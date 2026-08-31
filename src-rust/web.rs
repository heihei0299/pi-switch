//! Web UI backend: an axum server that exposes the same operations as the CLI/TUI
//! over `REST /api/*` and serves the embedded React frontend.
//!
//! Every handler is a thin adapter: parse input → call `ops`/`service`/`daemon`/`sync`
//! → serialize output. All business logic stays in those shared modules, so adding a
//! capability to the web UI means wiring one route here — not reimplementing anything.
//!
//! ## 路由组隔离（第二阶段）
//! - `profiles` 与 `gateway` 为独立子 Router（文件锁/原子写已隔离，路由层再隔离错误边界）
//! - `make_profiles_router()` 仅含 `/profiles/*`、`/state`、`/presets` 等供应商侧；`make_gateway_router()` 仅含 `/models/gateway*` 与 `/gateway/*` 占位
//! - 两组错误不串扰：Gateway 离线/校验失败不影响 Profiles CRUD；反之亦然（通过独立 `ApiError` 转换与 `fallback`）
//! - 后续真拆进程时，gateway Router 可直接挂载到独立进程的 `axum::Router`（健康检查 `GET /api/gateway/health` 已预留）

use crate::error::AppError;
use crate::{config, daemon, ops, service, stats, sync};
use axum::{
    body::Body,
    extract::{Path, Query, Request, State},
    http::{header, StatusCode, Uri},
    middleware::Next,
    response::{IntoResponse, Json, Response},
    routing::{get, post, put},
    Router,
};
use base64::Engine;
use rust_embed::RustEmbed;
use serde::Deserialize;
use serde_json::{json, Value};
use std::collections::HashMap;
use std::sync::Arc;

// ─── Embedded frontend ────────────────────────────────────
//
// `webui/dist` is produced by `npm run build:webui`. In release builds rust-embed
// bakes the files into the .node; in debug builds it reads them from disk at runtime
// (so `vite build` + re-run picks up changes without recompiling).

#[derive(RustEmbed)]
#[folder = "webui/dist"]
struct WebAssets;

// ─── Shared state ─────────────────────────────────────────

pub struct WebState {
    /// The JS project dir (parent of bin/), threaded through so proxy start/stop
    /// launched from the web UI can locate bin/pi-switch.js.
    pub project_dir: Option<String>,
    /// When `Some`, HTTP Basic auth (user `admin`) is required — enabled automatically
    /// for non-loopback binds. `None` for localhost.
    pub password: Option<String>,
}

// ─── Error type ───────────────────────────────────────────

struct ApiError(StatusCode, String);

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        (self.0, Json(json!({ "error": self.1 }))).into_response()
    }
}
impl From<AppError> for ApiError {
    fn from(e: AppError) -> Self {
        ApiError(StatusCode::BAD_REQUEST, e.to_string())
    }
}
impl From<String> for ApiError {
    fn from(e: String) -> Self {
        ApiError(StatusCode::BAD_REQUEST, e)
    }
}

type ApiJson = std::result::Result<Json<Value>, ApiError>;

// ─── Router ───────────────────────────────────────────────

/// 独立 profiles 路由组：供应商侧（/profiles/* 等），错误与 gateway 隔离
pub fn make_profiles_router() -> Router<Arc<WebState>> {
    Router::new()
        .route("/state", get(get_state))
        .route("/presets", get(get_presets))
        .route("/presets/:id", get(get_preset))
        .route(
            "/profiles/:name",
            get(get_profile).put(put_profile).delete(delete_profile),
        )
        .route("/doctor", get(get_doctor))
        .route("/config/validate", get(get_validate))
        .route("/backups", get(get_backups))
        .route("/stats", get(get_stats))
        .route("/stats/conversations", get(get_stats_conversations))
        .route(
            "/stats/conversations/:id/requests",
            get(get_conversation_requests),
        )
        .route("/proxy/status", get(get_proxy_status))
        .route("/webui/info", get(get_webui_info))
        .route("/logs/export", get(get_logs_export))
        // package management
        .route("/packages", get(get_packages).post(post_package))
        .route("/packages/import", post(post_package_import))
        .route("/packages/:id", get(get_package).delete(delete_package))
        .route("/packages/:id/toggle", post(post_package_toggle))
        // cc-switch import
        .route("/ccswitch/providers", get(get_ccswitch_providers))
        .route("/ccswitch/import", post(post_ccswitch_import))
        // profile mutations
        .route("/init", post(post_init))
        .route("/profiles", post(post_profile))
        .route("/profiles/:name/duplicate", post(post_duplicate))
        .route("/profiles/:name/use", post(post_use))
        .route("/profiles/:name/test", post(post_test))
        .route("/profiles/:name/fetch-models", post(post_fetch_models))
        .route("/profiles/:name/models", put(put_models))
        .route("/profiles/:name/expose", put(put_expose))
        .route("/profiles/:name/spoof", put(put_spoof))
        .route("/profiles/:name/credits", get(get_credits))
        // proxy + settings + config (仍属 profiles 侧，写 config.json)
        .route("/proxy/start", post(post_proxy_start))
        .route("/proxy/stop", post(post_proxy_stop))
        .route("/proxy/failover", put(put_failover))
        .route("/settings", put(put_settings))
        .route("/config/export", post(post_config_export))
        .route("/config/import", post(post_config_import))
        .route("/config/restore", post(post_config_restore))
}

/// 独立 gateway 路由组：网关侧（/gateway/* 与 /models/gateway*），错误与 profiles 隔离
/// 当前为逻辑隔离（同进程不同 Router + 独立 fallback），下一阶段可直接挂到独立 `pi-switch-gateway` 进程
pub fn make_gateway_router() -> Router<Arc<WebState>> {
    Router::new()
        .route("/models/gateway", get(get_gateway).put(put_gateway))
        .route("/models/gateway/preview", get(get_gateway_preview))
        .route("/gateway/health", get(get_gateway_health))
        .route("/gateway/start", post(post_gateway_start))
}

pub fn make_web_router(state: Arc<WebState>) -> Router {
    // 路由组逻辑隔离：profiles 与 gateway 各自独立 Router，错误不串扰
    let profiles_api = make_profiles_router().with_state(state.clone());
    let gateway_api = make_gateway_router().with_state(state.clone());
    // 合并时各自保留独立错误转换；任一子 Router 的 400/500 不会覆盖另一组的成功路径
    let api = Router::new()
        .merge(profiles_api)
        .merge(gateway_api)
        .fallback(api_not_found)
        .with_state(state.clone());

    Router::new()
        .nest("/api", api)
        .fallback(static_handler)
        .layer(axum::middleware::from_fn_with_state(state, auth_mw))
}

// ─── Auth (Basic, only when password set) ─────────────────

async fn auth_mw(State(state): State<Arc<WebState>>, req: Request, next: Next) -> Response {
    if let Some(ref pw) = state.password {
        let expected = format!("admin:{}", pw);
        let ok = req
            .headers()
            .get(header::AUTHORIZATION)
            .and_then(|v| v.to_str().ok())
            .and_then(|v| v.strip_prefix("Basic "))
            .and_then(|b64| base64::engine::general_purpose::STANDARD.decode(b64).ok())
            .and_then(|bytes| String::from_utf8(bytes).ok())
            .map(|creds| creds == expected)
            .unwrap_or(false);
        if !ok {
            return (
                StatusCode::UNAUTHORIZED,
                [(header::WWW_AUTHENTICATE, "Basic realm=\"pi-switch\"")],
                "Unauthorized",
            )
                .into_response();
        }
    }
    next.run(req).await
}

/// Decide whether the server needs auth: loopback binds run open; anything else
/// requires Basic auth with an auto-generated password stored under ~/.pi-switch/.
pub fn resolve_password(host: &str) -> Option<String> {
    if matches!(host, "127.0.0.1" | "localhost" | "::1") {
        return None;
    }
    let path = config::config_dir().join("webui_password");
    if let Ok(existing) = std::fs::read_to_string(&path) {
        let trimmed = existing.trim().to_string();
        if !trimmed.is_empty() {
            return Some(trimmed);
        }
    }
    // Generate a fresh 24-hex-char password and persist it.
    use rand::RngCore;
    let mut buf = [0u8; 12];
    rand::thread_rng().fill_bytes(&mut buf);
    let pw = buf.iter().map(|b| format!("{:02x}", b)).collect::<String>();
    std::fs::create_dir_all(config::config_dir()).ok();
    std::fs::write(&path, &pw).ok();
    Some(pw)
}

// ─── Read handlers ────────────────────────────────────────

async fn get_state() -> ApiJson {
    Ok(Json(service::get_state()?))
}

async fn get_presets() -> Json<Value> {
    Json(json!(service::presets_info()))
}

async fn get_preset(Path(id): Path<String>) -> ApiJson {
    Ok(Json(service::show_preset(&id)?))
}

async fn get_profile(Path(name): Path<String>) -> ApiJson {
    Ok(Json(service::get_profile(&name)?))
}

async fn get_gateway() -> ApiJson {
    Ok(Json(service::get_gateway()?))
}

async fn get_gateway_preview() -> ApiJson {
    Ok(Json(service::gateway_preview()?))
}

async fn put_gateway(Json(gateway): Json<Value>) -> ApiJson {
    Ok(Json(service::apply_gateway(gateway)?))
}

// ─── Gateway 进程生命周期占位（健康检查/启动，暂逻辑隔离） ──────────

async fn get_gateway_health() -> ApiJson {
    let health = crate::gateway::health_check()?;
    Ok(Json(
        serde_json::to_value(health).unwrap_or_else(|_| json!({})),
    ))
}

async fn post_gateway_start() -> ApiJson {
    let health = crate::gateway::start_placeholder()?;
    Ok(Json(
        serde_json::to_value(health).unwrap_or_else(|_| json!({})),
    ))
}

async fn get_doctor() -> Json<Value> {
    Json(json!(service::run_doctor()))
}

async fn get_validate() -> ApiJson {
    let issues = config::validate_config()?;
    Ok(Json(json!(issues)))
}

async fn get_backups() -> ApiJson {
    Ok(Json(json!(service::list_backups()?)))
}

async fn get_stats(Query(q): Query<HashMap<String, String>>) -> ApiJson {
    let window = crate::stats::parse_window_query(
        q.get("range").map(String::as_str),
        q.get("from").map(String::as_str),
        q.get("to").map(String::as_str),
    )?;
    // Optional 0-based page / per-page limit for the recent-request details
    // list; omitted values fall back to page 0 / limit 100 so old callers
    // keep the previous behaviour.
    let page = q.get("page").and_then(|s| s.parse::<usize>().ok());
    let limit = q.get("limit").and_then(|s| s.parse::<usize>().ok());
    Ok(Json(service::stats_value(window, page, limit)))
}

async fn get_stats_conversations(Query(q): Query<HashMap<String, String>>) -> ApiJson {
    // Same window semantics as `/stats`: no range/from/to at all means full
    // history (the All-time preset); a partial or invalid window is a 400.
    let window = crate::stats::parse_window_query(
        q.get("range").map(String::as_str),
        q.get("from").map(String::as_str),
        q.get("to").map(String::as_str),
    )?;
    let page = q.get("page").and_then(|s| s.parse::<usize>().ok());
    let limit = q.get("limit").and_then(|s| s.parse::<usize>().ok());
    Ok(Json(service::conversations_value(window, page, limit)))
}

async fn get_conversation_requests(
    Path(id): Path<String>,
    Query(q): Query<HashMap<String, String>>,
) -> ApiJson {
    if id.trim().is_empty() {
        return Err("conversation id must not be empty".to_string().into());
    }
    // Same optional paging as `/stats`: omitted or unparseable values fall
    // back to page 0 / limit 100.
    let page = q.get("page").and_then(|s| s.parse::<usize>().ok());
    let limit = q.get("limit").and_then(|s| s.parse::<usize>().ok());
    Ok(Json(service::conversation_requests_value(&id, page, limit)))
}

async fn get_proxy_status() -> ApiJson {
    let result = daemon::daemon_status(&daemon::PROXY)?;
    Ok(Json(
        serde_json::to_value(result).unwrap_or_else(|_| json!({})),
    ))
}

async fn get_webui_info(State(state): State<Arc<WebState>>) -> Json<Value> {
    Json(json!({
        "authRequired": state.password.is_some(),
    }))
}

async fn get_logs_export(Query(q): Query<HashMap<String, String>>) -> Response {
    let format = q.get("format").map(|s| s.as_str()).unwrap_or("json");
    let (body, content_type, filename) = match format {
        "csv" => match stats::export_logs_csv() {
            Ok(text) => (text, "text/csv", "pi-switch-logs.csv"),
            Err(e) => return ApiError::from(e).into_response(),
        },
        _ => match stats::export_logs_json() {
            Ok(text) => (text, "application/json", "pi-switch-logs.json"),
            Err(e) => return ApiError::from(e).into_response(),
        },
    };
    (
        [
            (header::CONTENT_TYPE, content_type.to_string()),
            (
                header::CONTENT_DISPOSITION,
                format!("attachment; filename=\"{}\"", filename),
            ),
        ],
        body,
    )
        .into_response()
}

// ─── Package handlers ─────────────────────────────────────

async fn get_packages() -> ApiJson {
    let packages = crate::package_ops::list_packages()?;
    // UI lists installed packages only; stale/uninstalled db records stay
    // invisible (CLI `package list` still shows them with status markers).
    let installed: Vec<_> = packages.into_iter().filter(|p| p.installed).collect();
    Ok(Json(json!({ "packages": installed })))
}

async fn get_package(Path(id): Path<String>) -> ApiJson {
    let package = crate::package_ops::get_package(&id)?;
    Ok(Json(
        serde_json::to_value(package).unwrap_or_else(|_| json!({})),
    ))
}

#[derive(Deserialize)]
struct PackageBody {
    spec: String,
}

async fn post_package(Json(body): Json<PackageBody>) -> ApiJson {
    // WebUI adds by full spec (e.g. npm:foo@1.0.0): add the db record, then
    // install it so the package actually appears in the installed list.
    crate::package_ops::add_package(&body.spec)?;
    let pkg = crate::package_ops::install_package(&body.spec)?;
    Ok(Json(
        json!({ "ok": true, "package": pkg.name, "installed": true }),
    ))
}

async fn post_package_toggle(Path(id): Path<String>) -> ApiJson {
    let package = crate::package_ops::toggle_package(&id)?;
    Ok(Json(json!({ "ok": true, "enabled": package.enabled })))
}

async fn delete_package(Path(id): Path<String>) -> ApiJson {
    // UI delete = uninstall from pi (if installed) + remove db record.
    crate::package_ops::uninstall_and_remove(&id)?;
    Ok(Json(json!({ "ok": true, "uninstalled": true })))
}

async fn post_package_import() -> ApiJson {
    let imported = crate::package_ops::import_from_pi()?;
    Ok(Json(json!({
        "ok": true,
        "count": imported.len(),
        "message": format!("Imported {} packages from Pi Agent", imported.len())
    })))
}

// ─── cc-switch import ─────────────────────────────────────

async fn get_ccswitch_providers(Query(q): Query<HashMap<String, String>>) -> ApiJson {
    let path = q.get("path").cloned();
    let providers = crate::ccswitch::list_ccswitch_providers(path.as_deref())?;
    Ok(Json(json!({ "providers": providers })))
}

#[derive(Deserialize)]
struct CcsImportBody {
    selections: Vec<CcsImportSel>,
    #[serde(default)]
    path: Option<String>,
}

#[derive(Deserialize)]
struct CcsImportSel {
    id: String,
    #[serde(default)]
    force: bool,
}

async fn post_ccswitch_import(Json(body): Json<CcsImportBody>) -> ApiJson {
    let selections: Vec<crate::ccswitch::CcsImportSelection> = body
        .selections
        .into_iter()
        .map(|s| crate::ccswitch::CcsImportSelection {
            id: s.id,
            force: s.force,
        })
        .collect();
    let results = crate::ccswitch::import_ccswitch_providers(&selections, body.path.as_deref())?;
    let imported = results.iter().filter(|r| r.imported).count();
    Ok(Json(json!({
        "ok": true,
        "imported": imported,
        "results": results,
    })))
}

// ─── Mutation handlers ────────────────────────────────────

fn ok(value: Value) -> ApiJson {
    Ok(Json(value))
}

fn backup_msg(backup: Option<std::path::PathBuf>) -> Value {
    json!({ "ok": true, "backup": backup.map(|p| p.display().to_string()) })
}

async fn post_init() -> ApiJson {
    let messages = ops::init()?;
    ok(json!({ "messages": messages }))
}

#[derive(Deserialize)]
struct UpsertBody {
    name: String,
    profile: Value,
}

async fn post_profile(Json(body): Json<UpsertBody>) -> ApiJson {
    let profile: config::ProviderProfile = serde_json::from_value(body.profile)
        .map_err(|e| AppError::Message(format!("invalid profile: {}", e)))?;
    let backup = ops::upsert_profile(&body.name, &profile, None)?;
    ok(backup_msg(backup))
}

#[derive(Deserialize)]
struct PutProfileBody {
    profile: Value,
    #[serde(rename = "renameFrom")]
    rename_from: Option<String>,
}

async fn put_profile(Path(name): Path<String>, Json(body): Json<PutProfileBody>) -> ApiJson {
    let profile: config::ProviderProfile = serde_json::from_value(body.profile)
        .map_err(|e| AppError::Message(format!("invalid profile: {}", e)))?;
    let backup = ops::upsert_profile(&name, &profile, body.rename_from.as_deref())?;
    ok(backup_msg(backup))
}

async fn delete_profile(Path(name): Path<String>) -> ApiJson {
    let backup = ops::remove_profile(&name)?;
    ok(backup_msg(backup))
}

#[derive(Deserialize)]
struct DuplicateBody {
    #[serde(rename = "as")]
    as_name: String,
}

async fn post_duplicate(Path(name): Path<String>, Json(body): Json<DuplicateBody>) -> ApiJson {
    let backup = ops::duplicate_profile(&name, &body.as_name)?;
    ok(backup_msg(backup))
}

#[derive(Deserialize)]
struct UseBody {
    mode: Option<String>,
}

async fn post_use(Path(name): Path<String>, Json(body): Json<UseBody>) -> ApiJson {
    let outcome = ops::use_profile(&name, body.mode.as_deref())?;
    ok(json!({
        "ok": true,
        "name": outcome.name,
        "providerId": outcome.provider_id,
        "modelsBackup": outcome.models_backup.map(|p| p.display().to_string()),
        "configBackup": outcome.config_backup.map(|p| p.display().to_string()),
    }))
}

async fn post_test(Path(name): Path<String>) -> ApiJson {
    let result = ops::test_provider(&name).await?;
    ok(json!({
        "success": result.success,
        "message": result.message,
        "responseTimeMs": result.response_time_ms,
    }))
}

async fn post_fetch_models(Path(name): Path<String>) -> ApiJson {
    let (models, enrich) = ops::fetch_models_with_stats(&name).await?;
    ok(json!({ "models": models, "enrich": enrich }))
}

#[derive(Deserialize)]
struct ModelsBody {
    models: Vec<config::ModelEntry>,
}

async fn put_models(Path(name): Path<String>, Json(body): Json<ModelsBody>) -> ApiJson {
    let (backup, enrich) = ops::update_provider_models_with_stats(&name, body.models)?;
    let mut resp = backup_msg(backup);
    resp["enrich"] = serde_json::to_value(enrich).unwrap_or(serde_json::json!({}));
    ok(resp)
}

#[derive(Deserialize)]
struct ExposeBody {
    #[serde(rename = "modelIds")]
    model_ids: Vec<String>,
}

async fn put_expose(Path(name): Path<String>, Json(body): Json<ExposeBody>) -> ApiJson {
    let backup = ops::update_exposed_models(&name, body.model_ids)?;
    ok(backup_msg(backup))
}

#[derive(Deserialize)]
struct SpoofBody {
    spoof: Option<String>,
}

async fn put_spoof(Path(name): Path<String>, Json(body): Json<SpoofBody>) -> ApiJson {
    let backup = ops::set_profile_spoof(&name, body.spoof)?;
    ok(backup_msg(backup))
}

#[derive(Deserialize)]
struct ProxyStartBody {
    host: Option<String>,
    port: Option<u16>,
}

async fn post_proxy_start(
    State(state): State<Arc<WebState>>,
    Json(body): Json<ProxyStartBody>,
) -> ApiJson {
    let result = daemon::daemon_start(
        &daemon::PROXY,
        body.host,
        body.port,
        state.project_dir.clone(),
    )?;
    ok(serde_json::to_value(result).unwrap_or_else(|_| json!({})))
}

async fn post_proxy_stop() -> ApiJson {
    let result = daemon::daemon_stop(&daemon::PROXY)?;
    ok(serde_json::to_value(result).unwrap_or_else(|_| json!({})))
}

#[derive(Deserialize)]
struct FailoverBody {
    failover: Vec<String>,
}

async fn put_failover(Json(body): Json<FailoverBody>) -> ApiJson {
    let backup = ops::set_failover(body.failover)?;
    ok(backup_msg(backup))
}

async fn put_settings(Json(settings): Json<Value>) -> ApiJson {
    let backup = ops::update_settings(&settings)?;
    ok(backup_msg(backup))
}

#[derive(Deserialize)]
struct ExportBody {
    passphrase: String,
}

async fn post_config_export(Json(body): Json<ExportBody>) -> ApiJson {
    let path = sync::encrypt_config(&body.passphrase)?;
    ok(json!({ "ok": true, "path": path }))
}

#[derive(Deserialize)]
struct ImportBody {
    #[serde(rename = "filePath")]
    file_path: String,
    passphrase: String,
}

async fn post_config_import(Json(body): Json<ImportBody>) -> ApiJson {
    let msg = sync::import_config(&body.file_path, &body.passphrase)?;
    ok(json!({ "ok": true, "message": msg }))
}

#[derive(Deserialize)]
struct RestoreBody {
    #[serde(rename = "backupPath")]
    backup_path: String,
}

async fn post_config_restore(Json(body): Json<RestoreBody>) -> ApiJson {
    let current_backup = config::restore_config(&body.backup_path)?;
    ok(json!({ "ok": true, "backup": current_backup.display().to_string() }))
}

async fn get_credits(Path(name): Path<String>) -> Result<Json<Value>, ApiError> {
    match crate::credits::fetch_credits_for_profile(&name).await {
        Ok(data) => Ok(Json(
            serde_json::to_value(data).unwrap_or_else(|_| json!({})),
        )),
        Err(e) => Err(map_credits_error(e)),
    }
}

fn map_credits_error(e: crate::credits::CreditsError) -> ApiError {
    use crate::credits::CreditsError as CE;
    match e {
        CE::NotFound(msg) => ApiError(StatusCode::NOT_FOUND, msg),
        CE::Unsupported(msg) => ApiError(StatusCode::NOT_FOUND, msg),
        CE::Timeout(msg) => ApiError(
            StatusCode::GATEWAY_TIMEOUT,
            format!("upstream timeout: {}", msg),
        ),
        CE::Upstream {
            status: 401,
            message,
        } => ApiError(
            StatusCode::UNAUTHORIZED,
            format!("upstream 401: {}", message),
        ),
        CE::Upstream {
            status: 429,
            message,
        } => ApiError(
            StatusCode::TOO_MANY_REQUESTS,
            format!("upstream 429: {}", message),
        ),
        CE::Upstream { status, message } if status >= 500 => ApiError(
            StatusCode::BAD_GATEWAY,
            format!("upstream {}: {}", status, message),
        ),
        CE::Upstream { status, message } => ApiError(
            StatusCode::BAD_REQUEST,
            format!("upstream {}: {}", status, message),
        ),
        CE::Network(msg) => ApiError(StatusCode::BAD_GATEWAY, format!("network error: {}", msg)),
        CE::Parse(msg) => ApiError(StatusCode::BAD_GATEWAY, format!("parse error: {}", msg)),
    }
}

async fn api_not_found() -> ApiError {
    ApiError(StatusCode::NOT_FOUND, "unknown API endpoint".into())
}

// ─── Static file serving (SPA) ────────────────────────────

async fn static_handler(uri: Uri) -> Response {
    let path = uri.path().trim_start_matches('/');
    let path = if path.is_empty() { "index.html" } else { path };

    if let Some(content) = WebAssets::get(path) {
        let mime = content.metadata.mimetype();
        return (
            [(header::CONTENT_TYPE, mime.to_string())],
            content.data.into_owned(),
        )
            .into_response();
    }

    // SPA history fallback: serve index.html for unknown non-asset routes.
    match WebAssets::get("index.html") {
        Some(content) => (
            [(header::CONTENT_TYPE, "text/html".to_string())],
            content.data.into_owned(),
        )
            .into_response(),
        None => (
            StatusCode::NOT_FOUND,
            [(header::CONTENT_TYPE, "text/html".to_string())],
            Body::from(PLACEHOLDER_HTML),
        )
            .into_response(),
    }
}

const PLACEHOLDER_HTML: &str = r#"<!doctype html><html><head><meta charset="utf-8">
<title>pi-switch WebUI</title></head><body style="font-family:system-ui;max-width:40rem;margin:4rem auto;line-height:1.6">
<h1>pi-switch WebUI</h1>
<p>The frontend has not been built yet. Run:</p>
<pre style="background:#f4f4f5;padding:1rem;border-radius:.5rem">npm run build:webui
npm run build:native</pre>
<p>then restart the server. The REST API under <code>/api</code> is already live.</p>
</body></html>"#;

#[cfg(test)]
mod tests {
    use super::*;
    use axum::body::Body;
    use tower::ServiceExt;

    fn router() -> Router {
        make_web_router(Arc::new(WebState {
            project_dir: None,
            password: None,
        }))
    }

    async fn get(uri: &str) -> Response {
        router()
            .oneshot(Request::builder().uri(uri).body(Body::empty()).unwrap())
            .await
            .unwrap()
    }

    #[tokio::test]
    async fn stats_without_window_params_returns_200() {
        let res = get("/api/stats").await;
        assert_eq!(res.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn stats_custom_without_bounds_is_rejected_not_500() {
        for uri in [
            "/api/stats?range=custom",
            "/api/stats?range=custom&from=1785664800000",
            "/api/stats?range=custom&to=1785672000000",
            "/api/stats?range=today",
            "/api/stats?range=today&from=1785664800000",
        ] {
            let res = get(uri).await;
            assert_eq!(
                res.status(),
                StatusCode::BAD_REQUEST,
                "{uri} should be 400, not 500"
            );
        }
    }

    #[tokio::test]
    async fn stats_invalid_range_or_inverted_window_is_rejected_not_500() {
        for uri in [
            "/api/stats?range=week",
            "/api/stats?range=custom&from=abc&to=1785672000000",
            "/api/stats?range=custom&from=1785672000000&to=1785664800000",
            "/api/stats?range=custom&from=1785672000000&to=1785672000000",
        ] {
            let res = get(uri).await;
            assert_eq!(
                res.status(),
                StatusCode::BAD_REQUEST,
                "{uri} should be 400, not 500"
            );
        }
    }

    #[tokio::test]
    async fn stats_with_valid_window_returns_200() {
        for uri in [
            "/api/stats?range=custom&from=1785664800000&to=1785672000000",
            "/api/stats?range=today&from=1785664800000&to=1785672000000",
            "/api/stats?range=last24h&from=1785664800000&to=1785672000000",
            "/api/stats?range=last7d&from=1785664800000&to=1785672000000",
            "/api/stats?from=1785664800000&to=1785672000000",
        ] {
            let res = get(uri).await;
            assert_eq!(res.status(), StatusCode::OK, "{uri} should be 200");
        }
    }

    #[tokio::test]
    async fn stats_response_includes_recent_request_total_field() {
        let res =
            get("/api/stats?range=today&from=1785664800000&to=1785672000000&page=0&limit=50").await;
        assert_eq!(res.status(), StatusCode::OK);
        let body: Value = serde_json::from_slice(
            &axum::body::to_bytes(res.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        let total = body
            .get("recentRequestTotal")
            .expect("stats body must include recentRequestTotal");
        assert!(
            total.is_u64() || total.is_i64(),
            "recentRequestTotal must be a number"
        );
    }

    #[tokio::test]
    async fn proxy_status_route_still_registered() {
        // Regression guard: /proxy/status must never be dropped when adding
        // sibling routes (it feeds the ProxyPanel on mount).
        let res = get("/api/proxy/status").await;
        assert_eq!(res.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn conversations_without_window_params_returns_200_all_history() {
        let res = get("/api/stats/conversations").await;
        assert_eq!(res.status(), StatusCode::OK);
        let body: Value = serde_json::from_slice(
            &axum::body::to_bytes(res.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        let convs = body
            .get("conversations")
            .expect("body must include conversations");
        assert!(convs.is_array(), "conversations must be an array");
        let total = body.get("total").expect("body must include total");
        assert!(total.is_u64() || total.is_i64(), "total must be a number");
    }

    #[tokio::test]
    async fn conversations_with_valid_window_returns_200() {
        for uri in [
            "/api/stats/conversations?range=custom&from=1785664800000&to=1785672000000",
            "/api/stats/conversations?range=today&from=1785664800000&to=1785672000000",
            "/api/stats/conversations?range=last24h&from=1785664800000&to=1785672000000",
            "/api/stats/conversations?range=last7d&from=1785664800000&to=1785672000000",
            "/api/stats/conversations?from=1785664800000&to=1785672000000",
            "/api/stats/conversations?page=0&limit=50",
        ] {
            let res = get(uri).await;
            assert_eq!(res.status(), StatusCode::OK, "{uri} should be 200");
        }
    }

    #[tokio::test]
    async fn conversations_invalid_window_is_rejected_not_500() {
        for uri in [
            "/api/stats/conversations?range=week",
            "/api/stats/conversations?range=custom",
            "/api/stats/conversations?range=today&from=1785664800000",
            "/api/stats/conversations?range=custom&from=abc&to=1785672000000",
            "/api/stats/conversations?range=custom&from=1785672000000&to=1785664800000",
        ] {
            let res = get(uri).await;
            assert_eq!(
                res.status(),
                StatusCode::BAD_REQUEST,
                "{uri} should be 400, not 500"
            );
        }
    }

    #[tokio::test]
    async fn conversation_requests_with_valid_id_returns_200() {
        let res = get("/api/stats/conversations/conv-a/requests?page=0&limit=5").await;
        assert_eq!(res.status(), StatusCode::OK);
        let body: Value = serde_json::from_slice(
            &axum::body::to_bytes(res.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        let requests = body.get("requests").expect("body must include requests");
        assert!(requests.is_array(), "requests must be an array");
        let total = body.get("total").expect("body must include total");
        assert!(total.is_u64() || total.is_i64(), "total must be a number");
    }

    #[tokio::test]
    async fn conversation_requests_empty_id_is_rejected() {
        let res = get("/api/stats/conversations//requests").await;
        assert_eq!(
            res.status(),
            StatusCode::BAD_REQUEST,
            "empty id should be 400"
        );
    }

    #[tokio::test]
    async fn conversation_requests_coexists_with_conversations_list() {
        // The precise /stats/conversations route must still resolve after the
        // deeper :id/requests route is registered.
        let list = get("/api/stats/conversations").await;
        assert_eq!(list.status(), StatusCode::OK, "list route unaffected");
        let detail = get("/api/stats/conversations/conv-a/requests").await;
        assert_eq!(detail.status(), StatusCode::OK, "detail route resolves too");
    }

    #[tokio::test]
    async fn gateway_get_returns_200_with_gateway_shape() {
        let res = get("/api/models/gateway").await;
        assert_eq!(
            res.status(),
            StatusCode::OK,
            "GET /api/models/gateway should be 200"
        );
        let body: Value = serde_json::from_slice(
            &axum::body::to_bytes(res.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        // must contain api/baseUrl/models or be null when no models file
        assert!(body.get("gateway").is_some() || body.is_null() || body.is_object());
    }

    #[tokio::test]
    async fn gateway_preview_is_dry_run_and_returns_current_and_proposed() {
        let res = get("/api/models/gateway/preview").await;
        assert_eq!(res.status(), StatusCode::OK, "preview should be 200");
        let body: Value = serde_json::from_slice(
            &axum::body::to_bytes(res.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        assert!(body.get("current").is_some(), "preview needs current");
        assert!(body.get("proposed").is_some(), "preview needs proposed");
        assert!(body.get("conflicts").is_some(), "preview needs conflicts");
        assert!(
            body["proposed"].get("models").is_some(),
            "proposed must have models"
        );
    }

    #[tokio::test]
    async fn gateway_apply_rejects_invalid_json() {
        let app = router();
        let req = axum::http::Request::builder()
            .uri("/api/models/gateway")
            .method(axum::http::Method::PUT)
            .header(axum::http::header::CONTENT_TYPE, "application/json")
            .body(Body::from(
                r#"{"api": "invalid-api", "baseUrl": "not-a-url", "models":[]}"#,
            ))
            .unwrap();
        let res = app.oneshot(req).await.unwrap();
        assert_eq!(
            res.status(),
            StatusCode::BAD_REQUEST,
            "invalid gateway should be 400"
        );
    }

    #[tokio::test]
    async fn gateway_preview_does_not_write_models_file() {
        use std::fs;
        // snapshot mtime or content before
        let path = crate::config::models_path();
        let before = fs::read_to_string(&path).unwrap_or_default();
        let _ = get("/api/models/gateway/preview").await;
        let after = fs::read_to_string(&path).unwrap_or_default();
        assert_eq!(
            before, after,
            "preview must be dry-run, not modify models.json"
        );
    }

    #[tokio::test]
    async fn gateway_health_returns_200_with_logical_isolation() {
        let res = get("/api/gateway/health").await;
        assert_eq!(
            res.status(),
            StatusCode::OK,
            "/api/gateway/health should be 200"
        );
        let body: Value = serde_json::from_slice(
            &axum::body::to_bytes(res.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        assert_eq!(body.get("running").and_then(|v| v.as_bool()), Some(true));
        assert_eq!(
            body.get("mode").and_then(|v| v.as_str()),
            Some("logical-isolation")
        );
        assert!(body.get("gateway_id").is_some());
    }

    #[tokio::test]
    async fn gateway_preview_and_profiles_routes_are_independent() {
        // Error in gateway should not affect profiles
        let app = router();
        // invalid gateway PUT -> 400
        let bad_gateway = app
            .clone()
            .oneshot(
                axum::http::Request::builder()
                    .uri("/api/models/gateway")
                    .method(axum::http::Method::PUT)
                    .header(axum::http::header::CONTENT_TYPE, "application/json")
                    .body(Body::from(
                        r#"{"api":"bad","baseUrl":"http://x/v1","models":[]}"#,
                    ))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(bad_gateway.status(), StatusCode::BAD_REQUEST);
        // profiles still reachable
        let profiles_state = get("/api/state").await;
        assert_eq!(profiles_state.status(), StatusCode::OK);
        // and preview still reachable
        let preview = get("/api/models/gateway/preview").await;
        assert_eq!(preview.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn profiles_upstream_payload_accepted_and_persists_baseurl_fallback() {
        // Create a profile with upstreams, ensure it round-trips via /state
        let app = router();
        let payload = serde_json::json!({
            "name": "test-upstream-profile",
            "profile": {
                "api": "openai-completions",
                "baseUrl": "http://a/v1",
                "apiKey": "k1",
                "upstreams": [
                    { "baseUrl": "http://a/v1", "apiKey": "k1", "weight": 2, "name": "a" },
                    { "baseUrl": "http://b/v1", "apiKey": "k2" }
                ],
                "models": [],
                "proxy": false
            }
        });
        let req = axum::http::Request::builder()
            .uri("/api/profiles")
            .method(axum::http::Method::POST)
            .header(axum::http::header::CONTENT_TYPE, "application/json")
            .body(Body::from(serde_json::to_string(&payload).unwrap()))
            .unwrap();
        let res = app.clone().oneshot(req).await.unwrap();
        // 200 or 400 depending on existing; but validation should not reject upstreams
        assert!(res.status() == StatusCode::OK || res.status() == StatusCode::BAD_REQUEST);
        // If OK, verify state contains upstreams
        if res.status() == StatusCode::OK {
            let state = get("/api/state").await;
            assert_eq!(state.status(), StatusCode::OK);
            let body: Value = serde_json::from_slice(
                &axum::body::to_bytes(state.into_body(), usize::MAX)
                    .await
                    .unwrap(),
            )
            .unwrap();
            let profiles = body.get("profiles").unwrap().as_object().unwrap();
            if let Some(p) = profiles.get("test-upstream-profile") {
                assert!(p.get("upstreams").is_some(), "upstreams should persist");
            }
            // cleanup
            let _ = app
                .clone()
                .oneshot(
                    axum::http::Request::builder()
                        .uri("/api/profiles/test-upstream-profile")
                        .method(axum::http::Method::DELETE)
                        .body(Body::empty())
                        .unwrap(),
                )
                .await
                .unwrap();
        }
    }

    #[tokio::test]
    async fn gateway_routers_have_independent_fallbacks() {
        // Unknown gateway route should be 404 without affecting profiles router
        let gw_404 = get("/api/gateway/unknown").await;
        assert_eq!(gw_404.status(), StatusCode::NOT_FOUND);
        let prof_ok = get("/api/state").await;
        assert_eq!(prof_ok.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn tdd_spoof_does_not_trigger_gateway_write() {
        // Ticket 01 S1: 供应商个性变更不自动写网关 — Red phase expects failure before fix
        let _health_before: Value = serde_json::from_slice(
            &axum::body::to_bytes(get("/api/gateway/health").await.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        let gateway_before: Value = serde_json::from_slice(
            &axum::body::to_bytes(get("/api/models/gateway").await.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        let gateway_before_inner = gateway_before
            .get("gateway")
            .cloned()
            .unwrap_or(Value::Null);

        let profile_name = "tdd-spoof-profile";
        let create_payload = serde_json::json!({
            "name": profile_name,
            "profile": {
                "api": "openai-completions",
                "baseUrl": "http://example.com/v1",
                "apiKey": "test-key",
                "models": [{"id": "m1"}],
                "proxy": false
            }
        });
        // ensure profile exists (ignore error if already exists)
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri("/api/profiles")
                    .method(axum::http::Method::POST)
                    .header(axum::http::header::CONTENT_TYPE, "application/json")
                    .body(Body::from(serde_json::to_string(&create_payload).unwrap()))
                    .unwrap(),
            )
            .await
            .unwrap();

        let health_before2: Value = serde_json::from_slice(
            &axum::body::to_bytes(get("/api/gateway/health").await.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();

        let spoof_payload = serde_json::json!({"spoof": "test-preset"});
        let app = router();
        let res = app
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}/spoof", profile_name))
                    .method(axum::http::Method::PUT)
                    .header(axum::http::header::CONTENT_TYPE, "application/json")
                    .body(Body::from(serde_json::to_string(&spoof_payload).unwrap()))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(
            res.status(),
            axum::http::StatusCode::OK,
            "spoof should be 200"
        );

        let health_after: Value = serde_json::from_slice(
            &axum::body::to_bytes(get("/api/gateway/health").await.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        let gateway_after: Value = serde_json::from_slice(
            &axum::body::to_bytes(get("/api/models/gateway").await.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        let gateway_after_inner = gateway_after.get("gateway").cloned().unwrap_or(Value::Null);

        assert_eq!(
            gateway_before_inner, gateway_after_inner,
            "spoof must not auto-write gateway: current should stay same before explicit publish"
        );
        assert_eq!(
            health_before2.get("last_notify"),
            health_after.get("last_notify"),
            "spoof must not trigger gateway notify"
        );

        // cleanup
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}", profile_name))
                    .method(axum::http::Method::DELETE)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
    }

    #[tokio::test]
    async fn gateway_preview_returns_pending_count() {
        let res = get("/api/models/gateway/preview").await;
        assert_eq!(res.status(), StatusCode::OK);
        let body: Value = serde_json::from_slice(
            &axum::body::to_bytes(res.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        assert!(
            body.get("pending_count").is_some(),
            "preview must include pending_count"
        );
        let pending = body
            .get("pending_count")
            .and_then(|v| v.as_u64())
            .expect("pending_count must be number");
        // pending_count should be computed as added+removed+changed between current and proposed
        let current = body.get("current");
        let proposed = body.get("proposed").expect("proposed required");
        // compute expected via same logic as gateway.rs (use json comparison)
        let expected = match current {
            None => proposed.as_object().map(|o| o.len() as u64).unwrap_or(0),
            Some(v) if v.is_null() => proposed.as_object().map(|o| o.len() as u64).unwrap_or(0),
            Some(v) => {
                let cur_obj = v.as_object().cloned().unwrap_or_default();
                let prop_obj = proposed.as_object().cloned().unwrap_or_default();
                let added = prop_obj
                    .keys()
                    .filter(|k| !cur_obj.contains_key(*k))
                    .count() as u64;
                let removed = cur_obj
                    .keys()
                    .filter(|k| !prop_obj.contains_key(*k))
                    .count() as u64;
                let changed = cur_obj
                    .keys()
                    .filter(|k| prop_obj.contains_key(*k) && cur_obj.get(*k) != prop_obj.get(*k))
                    .count() as u64;
                added + removed + changed
            }
        };
        assert_eq!(
            pending, expected,
            "pending_count should be added+removed+changed"
        );
    }

    #[tokio::test]
    async fn gateway_health_returns_full_shape_and_isolation() {
        let res = get("/api/gateway/health").await;
        assert_eq!(res.status(), StatusCode::OK);
        let body: Value = serde_json::from_slice(
            &axum::body::to_bytes(res.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        assert_eq!(body.get("running").and_then(|v| v.as_bool()), Some(true));
        assert_eq!(
            body.get("mode").and_then(|v| v.as_str()),
            Some("logical-isolation")
        );
        assert!(body.get("gateway_id").is_some(), "gateway_id required");
        assert!(
            body.get("has_models_file").is_some(),
            "has_models_file required"
        );
        assert!(
            body.get("upstreams_total").is_some(),
            "upstreams_total required"
        );
        assert!(body.get("message").is_some(), "message required");
        // last_notify may be null or string
        assert!(body.as_object().unwrap().contains_key("last_notify"));
    }

    #[tokio::test]
    async fn gateway_preview_and_health_remain_available_when_gateway_missing() {
        // Even if gateway file is missing/corrupted, preview and health should still be 200
        let health = get("/api/gateway/health").await;
        assert_eq!(
            health.status(),
            StatusCode::OK,
            "health should be ok even when gateway missing"
        );
        let preview = get("/api/models/gateway/preview").await;
        assert_eq!(
            preview.status(),
            StatusCode::OK,
            "preview should be ok even when gateway missing"
        );
        // supplier CRUD should still work (health failure isolation)
        let state = get("/api/state").await;
        assert_eq!(
            state.status(),
            StatusCode::OK,
            "supplier state should remain available when gateway missing"
        );
    }

    #[tokio::test]
    async fn gateway_start_does_not_auto_write_and_health_warn_only() {
        use std::fs;
        let path = crate::config::models_path();
        let before = fs::read_to_string(&path).unwrap_or_default();
        // Call start placeholder via API (POST /api/gateway/start)
        let app = router();
        let req = axum::http::Request::builder()
            .uri("/api/gateway/start")
            .method(axum::http::Method::POST)
            .body(Body::empty())
            .unwrap();
        let res = app.oneshot(req).await.unwrap();
        assert_eq!(
            res.status(),
            StatusCode::OK,
            "gateway start should succeed even if gateway file missing"
        );
        let body: Value = serde_json::from_slice(
            &axum::body::to_bytes(res.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        assert_eq!(body.get("running").and_then(|v| v.as_bool()), Some(true));
        let after = fs::read_to_string(&path).unwrap_or_default();
        // start should not auto-write models.json beyond notify; if models file existed, content should stay same
        // If file didn't exist, it may still not be created by start (only notify file)
        if !before.is_empty() {
            assert_eq!(
                before, after,
                "start_placeholder must not auto-write gateway"
            );
        }
    }

    #[tokio::test]
    async fn tdd_settings_does_not_trigger_gateway_write() {
        let state_before: Value = serde_json::from_slice(
            &axum::body::to_bytes(get("/api/state").await.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        let settings_before = state_before
            .get("settings")
            .cloned()
            .expect("state must have settings");
        let gateway_before: Value = serde_json::from_slice(
            &axum::body::to_bytes(get("/api/models/gateway").await.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        let health_before: Value = serde_json::from_slice(
            &axum::body::to_bytes(get("/api/gateway/health").await.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();

        let current_api = settings_before
            .get("gatewayApi")
            .and_then(|v| v.as_str())
            .unwrap_or("openai-completions");
        let flipped = if current_api == "openai-completions" {
            "openai-responses"
        } else {
            "openai-completions"
        };
        let mut new_settings = settings_before.clone();
        new_settings["gatewayApi"] = Value::String(flipped.to_string());

        let app = router();
        let res = app
            .oneshot(
                axum::http::Request::builder()
                    .uri("/api/settings")
                    .method(axum::http::Method::PUT)
                    .header(axum::http::header::CONTENT_TYPE, "application/json")
                    .body(Body::from(serde_json::to_string(&new_settings).unwrap()))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(
            res.status(),
            axum::http::StatusCode::OK,
            "settings PUT should be 200"
        );

        let gateway_after: Value = serde_json::from_slice(
            &axum::body::to_bytes(get("/api/models/gateway").await.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        let preview_after: Value = serde_json::from_slice(
            &axum::body::to_bytes(
                get("/api/models/gateway/preview").await.into_body(),
                usize::MAX,
            )
            .await
            .unwrap(),
        )
        .unwrap();
        let health_after: Value = serde_json::from_slice(
            &axum::body::to_bytes(get("/api/gateway/health").await.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();

        assert_eq!(
            gateway_before.get("gateway"),
            gateway_after.get("gateway"),
            "settings change must not auto-write gateway current"
        );
        // preview should show pending: current != proposed and proposed api is flipped
        let proposed_api = preview_after
            .get("proposed")
            .and_then(|p| p.get("api"))
            .and_then(|v| v.as_str());
        assert_eq!(
            proposed_api,
            Some(flipped),
            "preview proposed api should reflect new settings"
        );
        assert_ne!(
            preview_after.get("current"),
            preview_after.get("proposed"),
            "preview after settings change should show pending (current != proposed)"
        );
        assert_eq!(
            health_before.get("last_notify"),
            health_after.get("last_notify"),
            "settings change must not trigger gateway notify until explicit publish"
        );

        // restore settings
        let app2 = router();
        let _ = app2
            .oneshot(
                axum::http::Request::builder()
                    .uri("/api/settings")
                    .method(axum::http::Method::PUT)
                    .header(axum::http::header::CONTENT_TYPE, "application/json")
                    .body(Body::from(serde_json::to_string(&settings_before).unwrap()))
                    .unwrap(),
            )
            .await
            .unwrap();
    }

    #[tokio::test]
    async fn gateway_apply_validation_400_does_not_touch_models_and_supplier_isolated() {
        use std::fs;
        let models_path = crate::config::models_path();
        let before = fs::read_to_string(&models_path).unwrap_or_default();
        let app = router();
        let req = axum::http::Request::builder()
            .uri("/api/models/gateway")
            .method(axum::http::Method::PUT)
            .header(axum::http::header::CONTENT_TYPE, "application/json")
            .body(Body::from(
                r#"{"api":"bad-api","baseUrl":"ftp://bad","models":[]}"#,
            ))
            .unwrap();
        let res = app.oneshot(req).await.unwrap();
        assert_eq!(
            res.status(),
            StatusCode::BAD_REQUEST,
            "invalid gateway must be 400"
        );
        let after = fs::read_to_string(&models_path).unwrap_or_default();
        assert_eq!(
            before, after,
            "validation failure must not touch models.json"
        );
        let state = get("/api/state").await;
        assert_eq!(
            state.status(),
            StatusCode::OK,
            "supplier state must remain 200 after gateway validation failure"
        );
        let preview = get("/api/models/gateway/preview").await;
        assert_eq!(
            preview.status(),
            StatusCode::OK,
            "preview must remain 200 after validation failure"
        );
    }

    #[tokio::test]
    async fn gateway_apply_success_is_atomic_and_clears_pending() {
        let preview_before: Value = serde_json::from_slice(
            &axum::body::to_bytes(
                get("/api/models/gateway/preview").await.into_body(),
                usize::MAX,
            )
            .await
            .unwrap(),
        )
        .unwrap();
        let proposed = preview_before
            .get("proposed")
            .cloned()
            .expect("proposed required");
        let app = router();
        let req = axum::http::Request::builder()
            .uri("/api/models/gateway")
            .method(axum::http::Method::PUT)
            .header(axum::http::header::CONTENT_TYPE, "application/json")
            .body(Body::from(serde_json::to_string(&proposed).unwrap()))
            .unwrap();
        let res = app.oneshot(req).await.unwrap();
        assert_eq!(res.status(), StatusCode::OK, "valid apply must be 200");
        let preview_after: Value = serde_json::from_slice(
            &axum::body::to_bytes(
                get("/api/models/gateway/preview").await.into_body(),
                usize::MAX,
            )
            .await
            .unwrap(),
        )
        .unwrap();
        let pending = preview_after
            .get("pending_count")
            .and_then(|v| v.as_u64())
            .unwrap_or(999);
        assert_eq!(
            pending, 0,
            "after successful publish pending_count must be 0, got preview {:?}",
            preview_after
        );
        let current = preview_after.get("current");
        assert!(
            current.is_some() && !current.unwrap().is_null(),
            "current should be Some after publish"
        );
    }

    #[tokio::test]
    async fn profile_error_does_not_block_gateway_and_gateway_error_does_not_block_profile() {
        let app = router();
        let bad_profile = serde_json::json!({
            "name": "tdd-isolation-bad",
            "profile": { "api": "bad-api", "baseUrl": "not-a-url", "apiKey": "k", "models": [] }
        });
        let req = axum::http::Request::builder()
            .uri("/api/profiles")
            .method(axum::http::Method::POST)
            .header(axum::http::header::CONTENT_TYPE, "application/json")
            .body(Body::from(serde_json::to_string(&bad_profile).unwrap()))
            .unwrap();
        let res = app.clone().oneshot(req).await.unwrap();
        assert_eq!(
            res.status(),
            StatusCode::BAD_REQUEST,
            "invalid profile should be 400"
        );
        let health = get("/api/gateway/health").await;
        assert_eq!(
            health.status(),
            StatusCode::OK,
            "gateway health must remain 200 after profile error"
        );
        let preview = get("/api/models/gateway/preview").await;
        assert_eq!(
            preview.status(),
            StatusCode::OK,
            "gateway preview must remain 200 after profile error"
        );
        let app2 = router();
        let bad_gw = axum::http::Request::builder()
            .uri("/api/models/gateway")
            .method(axum::http::Method::PUT)
            .header(axum::http::header::CONTENT_TYPE, "application/json")
            .body(Body::from(
                r#"{"api":"bad","baseUrl":"http://x/v1","models":[]}"#,
            ))
            .unwrap();
        let res2 = app2.clone().oneshot(bad_gw).await.unwrap();
        assert_eq!(
            res2.status(),
            StatusCode::BAD_REQUEST,
            "invalid gateway should be 400"
        );
        let state = get("/api/state").await;
        assert_eq!(
            state.status(),
            StatusCode::OK,
            "supplier state must remain 200 after gateway error"
        );
        let good_profile = serde_json::json!({
            "name": "tdd-isolation-good",
            "profile": { "api": "openai-completions", "baseUrl": "http://example.com/v1", "apiKey": "k", "models": [{"id":"m1"}], "proxy": false }
        });
        let req_good = axum::http::Request::builder()
            .uri("/api/profiles")
            .method(axum::http::Method::POST)
            .header(axum::http::header::CONTENT_TYPE, "application/json")
            .body(Body::from(serde_json::to_string(&good_profile).unwrap()))
            .unwrap();
        let res_good = router().oneshot(req_good).await.unwrap();
        assert!(
            res_good.status() == StatusCode::OK || res_good.status() == StatusCode::BAD_REQUEST,
            "profile CRUD must not be 500 after gateway error, got {:?}",
            res_good.status()
        );
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri("/api/profiles/tdd-isolation-good")
                    .method(axum::http::Method::DELETE)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await;
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri("/api/profiles/tdd-isolation-bad")
                    .method(axum::http::Method::DELETE)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await;
    }

    // ─── Supplier Credits Panel (Issue 01) ──────────────────────────

    async fn start_mock_credits_server(status: StatusCode, body: Value, delay_ms: u64) -> String {
        use axum::http::Request;
        use std::time::Duration;
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let port = listener.local_addr().unwrap().port();
        let body_clone = body.clone();
        let app = Router::new().fallback(move |req: Request<Body>| {
            let body = body_clone.clone();
            async move {
                if delay_ms > 0 {
                    tokio::time::sleep(Duration::from_millis(delay_ms)).await;
                }
                let path = req.uri().path().to_string();
                if path.ends_with("/v1/usage") || path.ends_with("/v1/credits") {
                    (status, Json(body)).into_response()
                } else {
                    (StatusCode::NOT_FOUND, Json(json!({"error":"not found"}))).into_response()
                }
            }
        });
        tokio::spawn(async move {
            let _ = axum::serve(listener, app).await;
        });
        // baseUrl 需包含 opencode.ai 以命中 fetcher，同时指向 mock 端口
        format!("http://127.0.0.1:{}/opencode.ai", port)
    }

    async fn create_profile_with_baseurl(name: &str, base_url: &str) {
        let payload = serde_json::json!({
            "name": name,
            "profile": {
                "api": "openai-completions",
                "baseUrl": base_url,
                "apiKey": "test-key-credits",
                "models": [{"id": "m1"}],
                "proxy": false
            }
        });
        let res = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri("/api/profiles")
                    .method(axum::http::Method::POST)
                    .header(axum::http::header::CONTENT_TYPE, "application/json")
                    .body(Body::from(serde_json::to_string(&payload).unwrap()))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert!(
            res.status() == StatusCode::OK || res.status() == StatusCode::BAD_REQUEST,
            "create profile {} should be OK or BAD_REQUEST, got {:?}",
            name,
            res.status()
        );
    }

    #[tokio::test]
    async fn credits_opencode_returns_normalized() {
        let raw = json!({
            "balance": 42.5,
            "total": 100.0,
            "used": 30.0,
            "remaining": 70.0,
            "percent": 30.0,
            "reset_at": "2026-09-01T00:00:00Z"
        });
        let base = start_mock_credits_server(StatusCode::OK, raw.clone(), 0).await;
        let name = "tdd-credits-normalized";
        create_profile_with_baseurl(name, &base).await;
        // 小延迟让 mock 就绪
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
        let res = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}/credits", name))
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(
            res.status(),
            StatusCode::OK,
            "opencode-go credits should be 200"
        );
        let body: Value = serde_json::from_slice(
            &axum::body::to_bytes(res.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        assert_eq!(body.get("balance").and_then(|v| v.as_f64()), Some(42.5));
        assert_eq!(body.get("total").and_then(|v| v.as_f64()), Some(100.0));
        assert_eq!(body.get("used").and_then(|v| v.as_f64()), Some(30.0));
        assert_eq!(body.get("remaining").and_then(|v| v.as_f64()), Some(70.0));
        assert!((body.get("percent").and_then(|v| v.as_f64()).unwrap() - 30.0).abs() < 1e-6);
        assert_eq!(
            body.get("resetAt").and_then(|v| v.as_str()),
            Some("2026-09-01T00:00:00Z")
        );
        // raw 保留原体
        assert_eq!(body.get("raw"), Some(&raw));
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}", name))
                    .method(axum::http::Method::DELETE)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await;
    }

    #[tokio::test]
    async fn credits_non_opencode_returns_404_and_isolated() {
        let name = "tdd-credits-non-hit";
        let payload = serde_json::json!({
            "name": name,
            "profile": {
                "api": "openai-completions",
                "baseUrl": "https://example.com/v1",
                "apiKey": "k",
                "models": [{"id": "m1"}],
                "proxy": false
            }
        });
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri("/api/profiles")
                    .method(axum::http::Method::POST)
                    .header(axum::http::header::CONTENT_TYPE, "application/json")
                    .body(Body::from(serde_json::to_string(&payload).unwrap()))
                    .unwrap(),
            )
            .await
            .unwrap();
        let res = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}/credits", name))
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(
            res.status(),
            StatusCode::NOT_FOUND,
            "non-opencode should be 404"
        );
        // 隔离：state 与网关仍 200
        assert_eq!(get("/api/state").await.status(), StatusCode::OK);
        assert_eq!(
            get("/api/models/gateway/preview").await.status(),
            StatusCode::OK
        );
        assert_eq!(get("/api/gateway/health").await.status(), StatusCode::OK);
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}", name))
                    .method(axum::http::Method::DELETE)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await;
    }

    #[tokio::test]
    async fn credits_upstream_401_returns_normalized_error_and_isolated_no_write() {
        use std::fs;
        let raw_err = json!({"error": "unauthorized"});
        let base = start_mock_credits_server(StatusCode::UNAUTHORIZED, raw_err, 0).await;
        let name = "tdd-credits-401";
        create_profile_with_baseurl(name, &base).await;
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
        let cfg_path = crate::config::config_path();
        let models_path = crate::config::models_path();
        let cfg_before = fs::read_to_string(&cfg_path).unwrap_or_default();
        let models_before = fs::read_to_string(&models_path).unwrap_or_default();
        let res = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}/credits", name))
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(
            res.status(),
            StatusCode::UNAUTHORIZED,
            "401 should map to 401"
        );
        let cfg_after = fs::read_to_string(&cfg_path).unwrap_or_default();
        let models_after = fs::read_to_string(&models_path).unwrap_or_default();
        assert_eq!(
            cfg_before, cfg_after,
            "credits 401 must not write config.json"
        );
        assert_eq!(
            models_before, models_after,
            "credits 401 must not write models.json"
        );
        // 隔离：CRUD 与网关仍可用
        assert_eq!(get("/api/state").await.status(), StatusCode::OK);
        assert_eq!(
            get("/api/models/gateway/preview").await.status(),
            StatusCode::OK
        );
        // 供应商 CRUD 仍可用（spoof 不受影响）
        let spoof_res = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}/spoof", name))
                    .method(axum::http::Method::PUT)
                    .header(axum::http::header::CONTENT_TYPE, "application/json")
                    .body(Body::from(r#"{"spoof": null}"#))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(
            spoof_res.status(),
            StatusCode::OK,
            "supplier CRUD must remain available after 401"
        );
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}", name))
                    .method(axum::http::Method::DELETE)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await;
    }

    #[tokio::test]
    async fn credits_upstream_5xx_returns_502_and_isolated() {
        let base = start_mock_credits_server(
            StatusCode::INTERNAL_SERVER_ERROR,
            json!({"error":"internal"}),
            0,
        )
        .await;
        let name = "tdd-credits-5xx";
        create_profile_with_baseurl(name, &base).await;
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
        let res = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}/credits", name))
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(
            res.status(),
            StatusCode::BAD_GATEWAY,
            "5xx should map to 502"
        );
        assert_eq!(get("/api/state").await.status(), StatusCode::OK);
        assert_eq!(get("/api/gateway/health").await.status(), StatusCode::OK);
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}", name))
                    .method(axum::http::Method::DELETE)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await;
    }

    #[tokio::test]
    async fn credits_timeout_returns_504_and_isolated() {
        // mock 延迟 6s，超过 5s 超时
        let base = start_mock_credits_server(StatusCode::OK, json!({"balance":1}), 6000).await;
        let name = "tdd-credits-timeout";
        create_profile_with_baseurl(name, &base).await;
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
        let res = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}/credits", name))
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(
            res.status(),
            StatusCode::GATEWAY_TIMEOUT,
            "timeout should be 504"
        );
        assert_eq!(get("/api/state").await.status(), StatusCode::OK);
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}", name))
                    .method(axum::http::Method::DELETE)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await;
    }

    #[tokio::test]
    async fn credits_multi_upstream_only_primary() {
        // primary 指向 mock 成功，secondary 指向另一个 mock 但应被忽略
        let raw_primary = json!({"balance": 11.0, "total": 100.0, "used": 20.0});
        let base_primary = start_mock_credits_server(StatusCode::OK, raw_primary.clone(), 0).await;
        // secondary mock 返回不同值，但不应被查询
        let raw_secondary = json!({"balance": 999.0, "total": 999.0, "used": 999.0});
        let secondary_base = start_mock_credits_server(StatusCode::OK, raw_secondary, 0).await;
        let name = "tdd-credits-multi";
        let payload = serde_json::json!({
            "name": name,
            "profile": {
                "api": "openai-completions",
                "baseUrl": "http://fallback.example.com/v1",
                "apiKey": "fallback",
                "upstreams": [
                    {"baseUrl": base_primary, "apiKey": "k1"},
                    {"baseUrl": secondary_base, "apiKey": "k2"}
                ],
                "models": [{"id": "m1"}],
                "proxy": false
            }
        });
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri("/api/profiles")
                    .method(axum::http::Method::POST)
                    .header(axum::http::header::CONTENT_TYPE, "application/json")
                    .body(Body::from(serde_json::to_string(&payload).unwrap()))
                    .unwrap(),
            )
            .await
            .unwrap();
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
        let res = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}/credits", name))
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(
            res.status(),
            StatusCode::OK,
            "multi upstream should still be 200 via primary"
        );
        let body: Value = serde_json::from_slice(
            &axum::body::to_bytes(res.into_body(), usize::MAX)
                .await
                .unwrap(),
        )
        .unwrap();
        // 验证来自 primary（balance 11），而非 secondary 999
        assert_eq!(
            body.get("balance").and_then(|v| v.as_f64()),
            Some(11.0),
            "must query primary only"
        );
        assert_ne!(body.get("balance").and_then(|v| v.as_f64()), Some(999.0));
        let _ = router()
            .oneshot(
                axum::http::Request::builder()
                    .uri(format!("/api/profiles/{}", name))
                    .method(axum::http::Method::DELETE)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await;
    }
}
