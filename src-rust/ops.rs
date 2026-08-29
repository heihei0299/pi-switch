use crate::config::{
    self, backup_config, load_config, provider_id_for, save_config, ProviderProfile,
};
use crate::error::{AppError, Result};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::path::PathBuf;

/// 模型目录 enrich 统计（用于可观测性：合并到 Fetch 成功 toast 与 web 响应）
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize, Default)]
pub struct EnrichStats {
    pub enriched: usize,
    pub skipped: usize,
    pub failed: usize,
    pub warning: Option<String>,
}


/// Create ~/.pi-switch/ + ~/.pi/agent/ and seed config.json / models.json if absent.
/// Shared by the CLI `init` (napi) and the web `POST /api/init`.
pub fn init() -> Result<Vec<String>> {
    let dir = config::config_dir();
    std::fs::create_dir_all(&dir).map_err(|e| AppError::io(&dir, e))?;
    let pi_dir = config::pi_dir();
    std::fs::create_dir_all(&pi_dir).map_err(|e| AppError::io(&pi_dir, e))?;

    let mut messages = Vec::new();
    let config_path = config::config_path();
    if !config_path.exists() {
        save_config(&config::PiSwitchConfig::default())?;
        messages.push(format!("Created {}", config_path.display()));
    } else {
        messages.push(format!("Already exists: {}", config_path.display()));
    }

    let models_path = config::models_path();
    if !models_path.exists() {
        let default_models = serde_json::json!({ "providers": {} });
        let tmp = config::config_dir().join("models.json.tmp");
        std::fs::write(
            &tmp,
            serde_json::to_string_pretty(&default_models).unwrap() + "\n",
        )
        .map_err(|e| AppError::io(&tmp, e))?;
        std::fs::rename(&tmp, &models_path).map_err(|e| AppError::io(&models_path, e))?;
        messages.push(format!("Created {}", models_path.display()));
    } else {
        messages.push(format!("Already exists: {}", models_path.display()));
    }

    Ok(messages)
}

/// Set the same-model failover chain. Validates every profile exists and is not a proxy
/// profile. Shared by the CLI (napi) and the web `PUT /api/proxy/failover`.
pub fn set_failover(failover_profiles: Vec<String>) -> Result<Option<PathBuf>> {
    let mut config = load_config()?;

    for name in &failover_profiles {
        if !config.profiles.contains_key(name) {
            return Err(AppError::Message(format!(
                "Profile '{}' does not exist",
                name
            )));
        }
        if let Some(profile_value) = config.profiles.get(name) {
            if let Ok(profile) = serde_json::from_value::<ProviderProfile>(profile_value.clone()) {
                if profile.proxy {
                    return Err(AppError::Message(format!(
                        "Cannot use proxy profile '{}' in failover chain",
                        name
                    )));
                }
            }
        }
    }

    let backup = backup_config("config")?;
    config.settings.proxy.failover = failover_profiles;
    save_config(&config)?;
    Ok(backup)
}

pub struct UseOutcome {
    pub name: String,
    pub provider_id: String,
    pub models_backup: Option<PathBuf>,
    pub config_backup: Option<PathBuf>,
}

#[allow(dead_code)]
fn normalize_models(profile: &mut serde_json::Value) {
    if let Some(models) = profile.get_mut("models").and_then(|v| v.as_array_mut()) {
        for m in models {
            if let Some(obj) = m.as_object_mut() {
                if obj
                    .get("contextWindow")
                    .or(obj.get("context_window"))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(0)
                    == 0
                {
                    obj.insert("contextWindow".into(), serde_json::json!(1000000));
                }
                if obj
                    .get("maxTokens")
                    .or(obj.get("max_tokens"))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(0)
                    == 0
                {
                    obj.insert("maxTokens".into(), serde_json::json!(128000));
                }
                if obj
                    .get("input")
                    .and_then(|v| v.as_array())
                    .map(|a| a.is_empty())
                    .unwrap_or(true)
                {
                    obj.insert("input".into(), serde_json::json!(["text"]));
                }
            }
        }
    }
}

fn write_models_atomic(models: &serde_json::Value) -> Result<()> {
    let models_path = config::models_path();
    let tmp = config::config_dir().join("models.json.tmp");
    let json = serde_json::to_string_pretty(models).map_err(|e| AppError::json(&tmp, e))?;
    std::fs::write(&tmp, json + "\n").map_err(|e| AppError::io(&tmp, e))?;
    std::fs::rename(&tmp, &models_path).map_err(|e| AppError::io(&models_path, e))?;
    Ok(())
}

pub fn update_exposed_models(name: &str, model_ids: Vec<String>) -> Result<Option<PathBuf>> {
    let mut config = load_config()?;
    let backup = backup_config("config")?;

    let profile_value = config
        .profiles
        .get_mut(name)
        .ok_or_else(|| AppError::Message(format!("unknown profile '{}'", name)))?;

    let mut profile: ProviderProfile = serde_json::from_value(profile_value.clone())
        .map_err(|e| AppError::Message(format!("invalid profile: {}", e)))?;

    profile.exposed_models = model_ids;
    profile.updated_at = Some(chrono::Utc::now().to_rfc3339());

    *profile_value =
        serde_json::to_value(&profile).map_err(|e| AppError::json(config::config_path(), e))?;

    save_config(&config)?;


    Ok(backup)
}

/// Set (or clear, with None) the per-profile disguise preset.
pub fn set_profile_spoof(name: &str, spoof: Option<String>) -> Result<Option<PathBuf>> {
    let mut config = load_config()?;
    let backup = backup_config("config")?;

    let profile_value = config
        .profiles
        .get_mut(name)
        .ok_or_else(|| AppError::Message(format!("unknown profile '{}'", name)))?;

    let mut profile: ProviderProfile = serde_json::from_value(profile_value.clone())
        .map_err(|e| AppError::Message(format!("invalid profile: {}", e)))?;

    profile.spoof = spoof;
    profile.updated_at = Some(chrono::Utc::now().to_rfc3339());

    *profile_value =
        serde_json::to_value(&profile).map_err(|e| AppError::json(config::config_path(), e))?;

    save_config(&config)?;
    sync_gateway_to_pi()?;
    Ok(backup)
}

pub fn update_provider_models(
    name: &str,
    models: Vec<config::ModelEntry>,
) -> Result<Option<PathBuf>> {
    let (backup, stats) = update_provider_models_with_stats(name, models)?;
    if let Some(w) = stats.warning {
        log::warn!("模型目录模型元数据 enrich 警告: {}", w);
    }
    if stats.failed > 0 {
        log::warn!("模型目录 enrich 失败 {} 条，跳过 {} 条，已 enrich {} 条", stats.failed, stats.skipped, stats.enriched);
    } else if stats.enriched > 0 || stats.skipped > 0 {
        log::info!("模型目录 enrich 完成: 已 enrich {} 条，跳过 {} 条（模型目录未覆盖）", stats.enriched, stats.skipped);
    }
    Ok(backup)
}

/// 带统计的 update_provider_models：返回 (backup, EnrichStats) 供可观测性展示
pub fn update_provider_models_with_stats(
    name: &str,
    models: Vec<config::ModelEntry>,
) -> Result<(Option<PathBuf>, EnrichStats)> {
    let mut config = load_config()?;
    let backup = backup_config("config")?;

    let profile_value = config
        .profiles
        .get_mut(name)
        .ok_or_else(|| AppError::Message(format!("unknown profile '{}'", name)))?;

    let mut profile: ProviderProfile = serde_json::from_value(profile_value.clone())
        .map_err(|e| AppError::Message(format!("invalid profile: {}", e)))?;

    // 模型目录 enrich：用本地缓存的模型元数据补齐 cost/limit/reasoning/input/name 等
    // - 仅 enrich 已有 models 中的 id，不自动新增目录独有模型
    // - 目录未命中或字段缺失时保留原值，私有模型无报错
    // - 手工 `provider models` 显式列表不受约束（仅在有 catalog 且有映射时 enrich）
    // 同步路径复用缓存文件（`load_catalog_from_cache`），异步 Fetch 链路复用 `get_or_refresh_catalog_with_warning`
    // 离线场景（有 24h 内缓存）仍能完成 enrich，无缓存且无网络时保留原值并提示失败原因（术语：模型目录 / 模型元数据）
    let (enriched, stats) = try_enrich_from_cache_sync_with_stats(&profile, models);
    profile.models = enriched;
    profile.updated_at = Some(chrono::Utc::now().to_rfc3339());

    *profile_value =
        serde_json::to_value(&profile).map_err(|e| AppError::json(config::config_path(), e))?;

    save_config(&config)?;


    Ok((backup, stats))
}

// ─── 模型目录 enrich（模型元数据） ──────────────────────────────────────────

/// 从模型目录 Value 中 enrich 模型列表（纯函数，可测试）
///
/// - 仅 enrich 已有 `models` 中的 id，目录有但本地无的不自动新增
/// - 未命中或字段缺失时保留本地原值，不清 cost/limit
/// - 按分字段策略覆盖：limit.context/output、cost.input/output/cache_read、reasoning、modalities.input 按目录覆盖，name 仅缺省时补齐
/// - extra/compat/headers 不覆盖（thinkingLevelMap 仅当本地为空时按 reasoning_options 派生，留空不设默认）；cost 仅外层 input/output/cache_read，忽略 tiers/context_over_200k
#[allow(dead_code)]
pub fn enrich_models_with_catalog(
    profile: &config::ProviderProfile,
    models: Vec<config::ModelEntry>,
    catalog: Option<&Value>,
) -> Vec<config::ModelEntry> {
    let (out, _) = enrich_models_with_catalog_with_stats(profile, models, catalog, None);
    out
}

/// 带统计的 enrich（纯函数，可测试）：返回 (enriched_models, stats)
///
/// - catalog=None 时视为模型目录不可用，failed = models.len()，warning 来自调用方（如网络失败原因）
/// - provider 映射失败或目录中无该 provider 时，skipped = models.len()
/// - 否则 enriched = 命中数，skipped = 未命中数，failed = 0
///   模型目录 id 的 provider 前缀剥离（如 deepseek/deepseek-v4-flash -> deepseek-v4-flash），用于全局回退的兼容匹配
fn normalized_suffix(id: &str) -> &str {
    match id.rfind('/') {
        Some(idx) => &id[idx + 1..],
        None => id,
    }
}

/// 在单 provider 的 models map 中查找（精确 + 前缀剥离 + 后缀扫描）
fn find_in_catalog_map<'a>(
    map: &'a serde_json::Map<String, Value>,
    id: &str,
) -> Option<&'a Value> {
    if let Some(v) = map.get(id) {
        return Some(v);
    }
    let suffix = normalized_suffix(id);
    if suffix != id {
        if let Some(v) = map.get(suffix) {
            return Some(v);
        }
    }
    // 兼容 catalog 中 key 也带前缀的情况（如 catalog 键为 provider/model，本地为 model）
    for (k, v) in map.iter() {
        if normalized_suffix(k) == suffix {
            return Some(v);
        }
        if k == id {
            return Some(v);
        }
    }
    None
}

/// 全目录全局搜索：遍历 catalog 全部 provider 的 models map，按 id 精确匹配（含 provider/model 前缀兼容）
fn find_global_catalog_model<'a>(catalog: &'a Value, id: &str) -> Option<&'a Value> {
    let obj = catalog.as_object()?;
    for (_prov_key, provider_entry) in obj.iter() {
        if let Some(map) = provider_entry.get("models").and_then(|v| v.as_object()) {
            if let Some(v) = find_in_catalog_map(map, id) {
                return Some(v);
            }
        }
    }
    None
}

pub fn enrich_models_with_catalog_with_stats(
    profile: &config::ProviderProfile,
    models: Vec<config::ModelEntry>,
    catalog: Option<&Value>,
    warning: Option<String>,
) -> (Vec<config::ModelEntry>, EnrichStats) {
    let total = models.len();
    if total == 0 {
        return (models, EnrichStats { enriched: 0, skipped: 0, failed: 0, warning });
    }
    let Some(catalog) = catalog else {
        let w = warning.unwrap_or_else(|| "模型目录不可用，跳过模型元数据 enrich".to_string());
        log::warn!("{}", w);
        return (models, EnrichStats { enriched: 0, skipped: 0, failed: total, warning: Some(w) });
    };
    // per-profile 优先：显式 modelsDevProvider > preset 推断
    let provider_key_opt = crate::catalog::resolve_catalog_provider(profile);
    let per_models_map_opt = provider_key_opt
        .as_ref()
        .and_then(|k| catalog.get(k))
        .and_then(|v| v.get("models"))
        .and_then(|v| v.as_object());
    let mut enriched = 0;
    let mut skipped = 0;
    let out: Vec<config::ModelEntry> = models
        .into_iter()
        .map(|m| {
            // 1) per-profile 精确查找（含前缀兼容）
            let mut hit: Option<&Value> = None;
            if let Some(map) = per_models_map_opt {
                hit = find_in_catalog_map(map, &m.id);
            }
            // 2) per-profile 未命中时全局回退（遍历全目录，含前缀兼容）
            if hit.is_none() {
                hit = find_global_catalog_model(catalog, &m.id);
            }
            if let Some(catalog_model) = hit {
                enriched += 1;
                enrich_single_entry(m, catalog_model)
            } else {
                skipped += 1;
                m
            }
        })
        .collect();
    (out, EnrichStats { enriched, skipped, failed: 0, warning })
}

/// 为上游模型列表（id 列表）计算 enrich 统计（不改变 id 列表，用于 Fetch toast）
pub fn enrich_stats_for_ids(
    profile: &config::ProviderProfile,
    ids: &[String],
    catalog: Option<&Value>,
    warning: Option<String>,
) -> EnrichStats {
    let total = ids.len();
    if total == 0 {
        return EnrichStats { enriched: 0, skipped: 0, failed: 0, warning };
    }
    let Some(catalog) = catalog else {
        let w = warning.unwrap_or_else(|| "模型目录不可用，跳过模型元数据 enrich".to_string());
        return EnrichStats { enriched: 0, skipped: 0, failed: total, warning: Some(w) };
    };
    let provider_key_opt = crate::catalog::resolve_catalog_provider(profile);
    let per_models_map_opt = provider_key_opt
        .as_ref()
        .and_then(|k| catalog.get(k))
        .and_then(|v| v.get("models"))
        .and_then(|v| v.as_object());
    let mut enriched = 0;
    let mut skipped = 0;
    for id in ids {
        let mut hit = false;
        if let Some(map) = per_models_map_opt {
            if find_in_catalog_map(map, id).is_some() {
                hit = true;
            }
        }
        if !hit && find_global_catalog_model(catalog, id).is_some() {
            hit = true;
        }
        if hit {
            enriched += 1;
        } else {
            skipped += 1;
        }
    }
    EnrichStats { enriched, skipped, failed: 0, warning }
}

/// 异步 enrich 入口：复用 `get_or_refresh_catalog()`，触发范围为 Fetch Models 与新增/编辑 profile 的自动发现
#[allow(dead_code)]
pub async fn enrich_models_from_catalog(
    profile: &config::ProviderProfile,
    models: Vec<config::ModelEntry>,
) -> Vec<config::ModelEntry> {
    let (out, _) = enrich_models_from_catalog_with_stats(profile, models).await;
    out
}

/// 异步 enrich 入口（带统计与 warning）：优先 fresh cache 直读，stale/missing 时调用 get_or_refresh_catalog_with_warning 尝试网络，失败回退到过期缓存并记录 warning，无缓存失败时 failed 统计
pub async fn enrich_models_from_catalog_with_stats(
    profile: &config::ProviderProfile,
    models: Vec<config::ModelEntry>,
) -> (Vec<config::ModelEntry>, EnrichStats) {
    let (catalog_opt, warning) = match crate::catalog::get_or_refresh_catalog_with_warning().await {
        Ok((v, w)) => (v, w),
        Err(e) => {
            let w = format!("模型目录获取失败，跳过模型元数据 enrich: {}", e);
            log::warn!("{}", w);
            (None, Some(w))
        }
    };
    enrich_models_with_catalog_with_stats(profile, models, catalog_opt.as_ref(), warning)
}

/// 从模型目录的 reasoning_options 派生 thinkingLevelMap（模型目录驱动，仅当本地为空时填充）
/// - reasoning != true 时不生成
/// - 无 effort 类型或 effort values 为空/仅含 none 时不生成（留空不设默认）
/// - off 始终映射为 null，其余 pi 7 级按强度最近匹配 catalog 可用值
fn build_thinking_level_map(catalog_model: &Value) -> Option<Value> {
    let reasoning = catalog_model.get("reasoning").and_then(|v| v.as_bool());
    if reasoning != Some(true) {
        return None;
    }
    // 收集 effort values，过滤 "none"
    let mut effort_values: Vec<String> = Vec::new();
    if let Some(opts) = catalog_model
        .get("reasoning_options")
        .and_then(|v| v.as_array())
    {
        for opt in opts {
            if opt.get("type").and_then(|v| v.as_str()) == Some("effort") {
                if let Some(vals) = opt.get("values").and_then(|v| v.as_array()) {
                    for v in vals {
                        if let Some(s) = v.as_str() {
                            if s != "none" && !s.trim().is_empty() && !effort_values.contains(&s.to_string()) {
                                effort_values.push(s.to_string());
                            }
                        }
                    }
                }
            }
        }
    }
    if effort_values.is_empty() {
        return None;
    }
    fn intensity(level: &str) -> i32 {
        match level {
            "minimal" => 1,
            "low" => 2,
            "medium" => 3,
            "high" => 4,
            "xhigh" => 5,
            "max" => 6,
            _ => 0,
        }
    }
    let pi_levels = ["off", "minimal", "low", "medium", "high", "xhigh", "max"];
    let mut map = serde_json::Map::new();
    for &pi_level in &pi_levels {
        if pi_level == "off" {
            map.insert("off".to_string(), Value::Null);
        } else if effort_values.contains(&pi_level.to_string()) {
            map.insert(pi_level.to_string(), Value::String(pi_level.to_string()));
        } else {
            let pi_int = intensity(pi_level);
            let mut best: Option<&String> = None;
            let mut best_dist = i32::MAX;
            let mut best_int = -1;
            for cand in &effort_values {
                let cand_int = intensity(cand);
                let dist = (cand_int - pi_int).abs();
                if dist < best_dist || (dist == best_dist && cand_int > best_int) {
                    best_dist = dist;
                    best_int = cand_int;
                    best = Some(cand);
                }
            }
            if let Some(b) = best {
                map.insert(pi_level.to_string(), Value::String(b.clone()));
            }
        }
    }
    Some(Value::Object(map))
}

fn is_thinking_level_map_empty(map: &Option<Value>) -> bool {
    match map {
        None => true,
        Some(v) => v.as_object().map(|o| o.is_empty()).unwrap_or(true),
    }
}


/// 解析单个 catalog model entry 并按分字段策略覆盖到 ModelEntry（保持 extra 等不动）
fn enrich_single_entry(mut entry: config::ModelEntry, catalog_model: &Value) -> config::ModelEntry {
    // limit.context -> contextWindow, limit.output -> maxTokens
    if let Some(limit) = catalog_model.get("limit").and_then(|v| v.as_object()) {
        if let Some(v) = limit
            .get("context")
            .and_then(|x| x.as_u64().or_else(|| x.as_f64().map(|f| f as u64)))
        {
            if v > 0 && v <= u32::MAX as u64 {
                entry.context_window = v as u32;
            }
        }
        if let Some(v) = limit
            .get("output")
            .and_then(|x| x.as_u64().or_else(|| x.as_f64().map(|f| f as u64)))
        {
            if v > 0 && v <= u32::MAX as u64 {
                entry.max_tokens = v as u32;
            }
        }
    }

    // cost.input/output/cache_read -> cost (仅外层，忽略 tiers/context_over_200k)
    if let Some(cost_obj) = catalog_model.get("cost").and_then(|v| v.as_object()) {
        let c_input = cost_obj
            .get("input")
            .and_then(|v| v.as_f64())
            .or_else(|| cost_obj.get("input").and_then(|v| v.as_i64().map(|i| i as f64)));
        let c_output = cost_obj
            .get("output")
            .and_then(|v| v.as_f64())
            .or_else(|| cost_obj.get("output").and_then(|v| v.as_i64().map(|i| i as f64)));
        // 兼容 snake 与 camel
        let c_cache_read = cost_obj
            .get("cache_read")
            .and_then(|v| v.as_f64())
            .or_else(|| cost_obj.get("cache_read").and_then(|v| v.as_i64().map(|i| i as f64)))
            .or_else(|| cost_obj.get("cacheRead").and_then(|v| v.as_f64()))
            .or_else(|| cost_obj.get("cacheRead").and_then(|v| v.as_i64().map(|i| i as f64)));

        if c_input.is_some() || c_output.is_some() || c_cache_read.is_some() {
            let mut cost = entry.cost.clone().unwrap_or_default();
            if let Some(v) = c_input {
                cost.input = v;
            }
            if let Some(v) = c_output {
                cost.output = v;
            }
            if let Some(v) = c_cache_read {
                cost.cache_read = v;
            }
            entry.cost = Some(cost);
        }
    }

    // reasoning -> reasoning
    if let Some(v) = catalog_model.get("reasoning").and_then(|x| x.as_bool()) {
        entry.reasoning = Some(v);
    }

    // modalities.input -> input（按目录覆盖，仅保留 text/image 以保持校验合法）
    if let Some(arr) = catalog_model
        .get("modalities")
        .and_then(|m| m.get("input"))
        .and_then(|v| v.as_array())
    {
        let mapped: Vec<String> = arr
            .iter()
            .filter_map(|v| v.as_str().map(|s| s.to_string()))
            .collect();
        if !mapped.is_empty() {
            // 过滤为仅 text/image，避免 pdf 等导致校验失败；若过滤后为空则保留原值
            let filtered: Vec<String> = mapped
                .into_iter()
                .filter(|s| s == "text" || s == "image")
                .collect();
            if !filtered.is_empty() {
                entry.input = filtered;
            }
        }
    }

    // name 仅缺省时补齐
    let need_name = entry
        .name
        .as_deref()
        .map(|s| s.trim().is_empty())
        .unwrap_or(true);
    if need_name {
        if let Some(name_str) = catalog_model.get("name").and_then(|v| v.as_str()) {
            if !name_str.trim().is_empty() {
                entry.name = Some(name_str.to_string());
            }
        }
    }

    // thinkingLevelMap：仅当本地为空时按模型目录派生（非空即手工，留空不设默认）
    if is_thinking_level_map_empty(&entry.thinking_level_map) {
        if let Some(derived) = build_thinking_level_map(catalog_model) {
            entry.thinking_level_map = Some(derived);
        }
    }

    entry
}

/// 同步 enrich 辅助：从本地缓存加载模型目录（若存在），否则跳过；供 `update_provider_models` 等同步路径复用
#[allow(dead_code)]
fn try_enrich_from_cache_sync(
    profile: &config::ProviderProfile,
    models: Vec<config::ModelEntry>,
) -> Vec<config::ModelEntry> {
    let (out, _) = try_enrich_from_cache_sync_with_stats(profile, models);
    out
}

/// 同步 enrich 辅助（带统计）：离线场景有 24h 内缓存仍能完成 enrich，无缓存时 failed 统计并提示
fn try_enrich_from_cache_sync_with_stats(
    profile: &config::ProviderProfile,
    models: Vec<config::ModelEntry>,
) -> (Vec<config::ModelEntry>, EnrichStats) {
    let path = crate::catalog::catalog_cache_path();
    let catalog_opt = crate::catalog::load_catalog_from_cache(&path).ok().flatten();
    let warning = if catalog_opt.is_none() && !path.exists() {
        Some("模型目录无可用缓存，跳过模型元数据 enrich".to_string())
    } else {
        None
    };
    // 若 catalog 存在但为 stale，仍直接使用（离线场景），与 catalog::get_or_refresh 语义一致：优先缓存
    enrich_models_with_catalog_with_stats(profile, models, catalog_opt.as_ref(), warning)
}

#[cfg(test)]
mod enrich_tests {
    use super::*;
    use crate::config::{ModelCost, ModelEntry, ProviderProfile};
    use serde_json::json;

    fn fixture_catalog() -> Value {
        json!({
            "openai": {
                "id": "openai",
                "models": {
                    "gpt-4o": {
                        "id": "gpt-4o",
                        "name": "GPT-4o",
                        "limit": {"context": 128000, "output": 16384},
                        "cost": {"input": 2.5, "output": 10, "cache_read": 1.25},
                        "reasoning": false,
                        "modalities": {"input": ["text", "image"], "output": ["text"]}
                    },
                    "o1": {
                        "id": "o1",
                        "name": "o1",
                        "limit": {"context": 200000, "output": 100000},
                        "cost": {"input": 15, "output": 60, "cache_read": 7.5, "tiers": [{"input":30, "output":120, "tier":{"type":"context","size":200000}}], "context_over_200k": {"input":30, "output":120}},
                        "reasoning": true,
                        "modalities": {"input": ["text"], "output": ["text"]}
                    }
                }
            },
            "anthropic": {
                "id": "anthropic",
                "models": {
                    "claude-3-5-sonnet": {
                        "id": "claude-3-5-sonnet",
                        "name": "Claude 3.5 Sonnet",
                        "limit": {"context": 200000, "output": 8192},
                        "cost": {"input": 3, "output": 15},
                        "reasoning": false,
                        "modalities": {"input": ["text", "image"], "output": ["text"]}
                    }
                }
            }
        })
    }

    fn profile_openai() -> ProviderProfile {
        ProviderProfile {
            preset: Some("openai".into()),
            ..Default::default()
        }
    }

    #[test]
    fn enrich_field_mapping_correctness() {
        let profile = profile_openai();
        let catalog = fixture_catalog();
        let models = vec![ModelEntry {
            id: "gpt-4o".into(),
            name: None,
            context_window: 1000,
            max_tokens: 100,
            cost: None,
            reasoning: None,
            input: vec!["text".into()],
            ..Default::default()
        }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        assert_eq!(out.len(), 1);
        let m = &out[0];
        assert_eq!(m.context_window, 128000, "limit.context -> contextWindow");
        assert_eq!(m.max_tokens, 16384, "limit.output -> maxTokens");
        let cost = m.cost.as_ref().expect("cost should be enriched");
        assert!((cost.input - 2.5).abs() < 1e-9);
        assert!((cost.output - 10.0).abs() < 1e-9);
        assert!((cost.cache_read - 1.25).abs() < 1e-9);
        assert_eq!(m.reasoning, Some(false));
        assert_eq!(m.input, vec!["text", "image"]);
        assert_eq!(m.name.as_deref(), Some("GPT-4o"));
    }

    #[test]
    fn enrich_name_retains_when_not_missing() {
        let profile = profile_openai();
        let catalog = fixture_catalog();
        let models = vec![ModelEntry {
            id: "gpt-4o".into(),
            name: Some("My Custom".into()),
            ..Default::default()
        }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        assert_eq!(out[0].name.as_deref(), Some("My Custom"), "手工 name 非空时不被目录覆盖");
        // name 缺省时应补齐
        let models2 = vec![ModelEntry { id: "gpt-4o".into(), name: None, ..Default::default() }];
        let out2 = enrich_models_with_catalog(&profile, models2, Some(&catalog));
        assert_eq!(out2[0].name.as_deref(), Some("GPT-4o"));
    }

    #[test]
    fn enrich_miss_retains_original() {
        let profile = profile_openai();
        let catalog = fixture_catalog();
        let original_cost = ModelCost { input: 99.0, output: 99.0, cache_read: 9.9, cache_write: 1.1, ..Default::default() };
        let models = vec![ModelEntry {
            id: "private-model".into(),
            context_window: 9999,
            max_tokens: 888,
            cost: Some(original_cost.clone()),
            reasoning: Some(true),
            input: vec!["text".into()],
            name: Some("Private".into()),
            ..Default::default()
        }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        let m = &out[0];
        assert_eq!(m.context_window, 9999);
        assert_eq!(m.max_tokens, 888);
        let c = m.cost.as_ref().unwrap();
        assert!((c.input - 99.0).abs() < 1e-9);
        assert_eq!(m.reasoning, Some(true));
        assert_eq!(m.name.as_deref(), Some("Private"));
    }

    #[test]
    fn enrich_not_auto_add_catalog_extra() {
        let profile = profile_openai();
        let catalog = fixture_catalog();
        let models = vec![ModelEntry { id: "gpt-4o".into(), ..Default::default() }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].id, "gpt-4o");
        // 目录中的 o1 不应自动新增
        assert!(!out.iter().any(|m| m.id == "o1"));
    }

    #[test]
    fn enrich_extra_fields_not_overwritten() {
        let profile = profile_openai();
        let catalog = fixture_catalog();
        let mut extra = serde_json::Map::new();
        extra.insert("futureField".into(), json!({"preserved": true}));
        let mut headers = serde_json::Map::new();
        headers.insert("x-custom".into(), json!("keep"));
        let models = vec![ModelEntry {
            id: "gpt-4o".into(),
            headers: Some(headers.clone()),
            compat: Some(json!({"supportsDeveloperRole": true})),
            thinking_level_map: Some(json!({"off": null})),
            extra: extra.clone(),
            cost: Some(ModelCost { input: 1.0, output: 1.0, cache_read: 0.1, cache_write: 0.2, tiers: vec![crate::config::ModelCostTier { input_tokens_above: 1.0, input: 2.0, output:2.0, cache_read:0.2, cache_write:0.2, ..Default::default() }], ..Default::default() }),
            ..Default::default()
        }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        let m = &out[0];
        assert_eq!(m.extra.get("futureField"), extra.get("futureField"));
        assert_eq!(m.headers, Some(headers));
        assert_eq!(m.compat, Some(json!({"supportsDeveloperRole": true})));
        assert!(m.thinking_level_map.is_some());
        // tiers 应保留原值，不被目录 tiers 覆盖
        let cost = m.cost.as_ref().unwrap();
        assert_eq!(cost.tiers.len(), 1);
        assert_eq!(cost.cache_write, 0.2);
        // 但 input/output/cache_read 应被覆盖
        assert!((cost.input - 2.5).abs() < 1e-9);
    }

    #[test]
    fn enrich_ignores_tiers_and_context_over_200k() {
        let profile = profile_openai();
        let catalog = fixture_catalog();
        let models = vec![ModelEntry { id: "o1".into(), ..Default::default() }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        let cost = out[0].cost.as_ref().unwrap();
        assert!((cost.input - 15.0).abs() < 1e-9);
        assert!((cost.output - 60.0).abs() < 1e-9);
        assert!((cost.cache_read - 7.5).abs() < 1e-9);
        assert!(cost.tiers.is_empty(), "tiers 不应被目录展开");
    }

    #[test]
    fn enrich_partial_cost_retains_missing_fields() {
        let profile = profile_openai();
        let mut catalog = fixture_catalog();
        // catalog 中 o1 仅提供 input/output，缺 cache_read，观察保留原 cache_read
        catalog["openai"]["models"]["o1"]["cost"] = json!({"input": 15, "output": 60});
        let models = vec![ModelEntry {
            id: "o1".into(),
            cost: Some(ModelCost { input: 1.0, output: 1.0, cache_read: 9.9, cache_write: 0.5, ..Default::default() }),
            ..Default::default()
        }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        let cost = out[0].cost.as_ref().unwrap();
        assert!((cost.input - 15.0).abs() < 1e-9);
        assert!((cost.output - 60.0).abs() < 1e-9);
        assert!((cost.cache_read - 9.9).abs() < 1e-9, "目录缺失 cache_read 时保留原值");
        assert!((cost.cache_write - 0.5).abs() < 1e-9);
    }

    #[test]
    fn enrich_respects_provider_mapping_and_skips_unknown_preset() {
        let mut profile = ProviderProfile { preset: Some("custom".into()), ..Default::default() };
        let catalog = fixture_catalog();
        let models = vec![ModelEntry { id: "gpt-4o".into(), context_window: 1000, ..Default::default() }];
        let out = enrich_models_with_catalog(&profile, models.clone(), Some(&catalog));
        assert_eq!(out[0].context_window, 128000, "未知 preset 现通过全局回退应 enrich（聚合 profile 修复）");

        profile.models_dev_provider = Some("openai".into());
        let out2 = enrich_models_with_catalog(&profile, models, Some(&catalog));
        assert_eq!(out2[0].context_window, 128000, "显式 modelsDevProvider 优先");
    }

    #[test]
    fn enrich_none_catalog_retains_original() {
        let profile = profile_openai();
        let models = vec![ModelEntry { id: "gpt-4o".into(), context_window: 123, ..Default::default() }];
        let out = enrich_models_with_catalog(&profile, models.clone(), None);
        assert_eq!(out[0].context_window, 123);
    }

    #[test]
    fn enrich_stats_counts_enriched_skipped_failed() {
        let profile = profile_openai();
        let catalog = fixture_catalog();
        let models = vec![
            ModelEntry { id: "gpt-4o".into(), ..Default::default() },
            ModelEntry { id: "private-model".into(), ..Default::default() },
            ModelEntry { id: "o1".into(), ..Default::default() },
        ];
        let (out, stats) = enrich_models_with_catalog_with_stats(&profile, models, Some(&catalog), None);
        assert_eq!(out.len(), 3);
        assert_eq!(stats.enriched, 2, "gpt-4o and o1 should be enriched");
        assert_eq!(stats.skipped, 1, "private-model 未在模型目录中");
        assert_eq!(stats.failed, 0);
        assert!(stats.warning.is_none());
    }

    #[test]
    fn enrich_stats_for_ids_counts() {
        let profile = profile_openai();
        let catalog = fixture_catalog();
        let ids = vec!["gpt-4o".to_string(), "unknown".to_string()];
        let stats = enrich_stats_for_ids(&profile, &ids, Some(&catalog), None);
        assert_eq!(stats.enriched, 1);
        assert_eq!(stats.skipped, 1);
        assert_eq!(stats.failed, 0);
    }

    #[test]
    fn enrich_stats_failed_when_no_catalog() {
        let profile = profile_openai();
        let models = vec![ModelEntry { id: "gpt-4o".into(), ..Default::default() }];
        let (out, stats) = enrich_models_with_catalog_with_stats(&profile, models.clone(), None, None);
        assert_eq!(out[0].id, "gpt-4o");
        assert_eq!(stats.failed, 1);
        assert_eq!(stats.enriched, 0);
        assert_eq!(stats.skipped, 0);
        assert!(stats.warning.is_some());
        assert!(stats.warning.as_ref().unwrap().contains("模型目录"), "文案需使用模型目录术语");
    }

    #[test]
    fn enrich_stats_skipped_when_provider_unmapped() {
        let profile = ProviderProfile { preset: Some("custom".into()), ..Default::default() };
        let catalog = fixture_catalog();
        // gpt-4o 存在于全局 openai 中，现应通过全局回退命中（聚合 profile 修复）
        let models = vec![ModelEntry { id: "gpt-4o".into(), ..Default::default() }];
        let (out, stats) = enrich_models_with_catalog_with_stats(&profile, models.clone(), Some(&catalog), None);
        assert_eq!(out[0].id, "gpt-4o");
        assert_eq!(stats.enriched, 1);
        assert_eq!(stats.skipped, 0);
        assert_eq!(stats.failed, 0);
        // 真正不存在于任何 provider 的 id 仍应 skipped
        let models2 = vec![ModelEntry { id: "private-unknown-xyz".into(), ..Default::default() }];
        let (out2, stats2) = enrich_models_with_catalog_with_stats(&profile, models2.clone(), Some(&catalog), None);
        assert_eq!(out2[0].id, "private-unknown-xyz");
        assert_eq!(stats2.skipped, 1);
        assert_eq!(stats2.enriched, 0);
    }

    #[test]
    fn enrich_stats_with_warning_propagates() {
        let profile = profile_openai();
        let catalog = fixture_catalog();
        let models = vec![ModelEntry { id: "gpt-4o".into(), ..Default::default() }];
        let warning = Some("模型目录拉取失败，回退到过期缓存: network down".to_string());
        let (_out, stats) = enrich_models_with_catalog_with_stats(&profile, models, Some(&catalog), warning.clone());
        assert_eq!(stats.enriched, 1);
        assert_eq!(stats.warning, warning);
    }

    #[test]
    fn enrich_stats_offline_with_catalog_still_enriches() {
        // 模拟离线但有缓存：catalog 为 Some 时仍能 enrich，即使网络失败（warning 存在）
        let profile = profile_openai();
        let catalog = fixture_catalog();
        let models = vec![ModelEntry { id: "gpt-4o".into(), context_window: 1000, ..Default::default() }];
        let warning = Some("模型目录拉取失败，回退到过期缓存: 离线".to_string());
        let (out, stats) = enrich_models_with_catalog_with_stats(&profile, models, Some(&catalog), warning);
        assert_eq!(out[0].context_window, 128000);
        assert_eq!(stats.enriched, 1);
        assert!(stats.warning.is_some());
        assert!(stats.warning.unwrap().contains("模型目录"));
    }
    #[test]
    fn enrich_global_fallback_oc_aggregation_cross_provider() {
        // oc 为聚合 profile：无 preset 且无 modelsDevProvider，应通过全目录全局搜索命中跨 provider 模型
        let catalog = json!({
            "openai": {
                "id": "openai",
                "models": {
                    "gpt-4o": {
                        "id": "gpt-4o",
                        "name": "GPT-4o",
                        "limit": {"context": 128000, "output": 16384},
                        "cost": {"input": 2.5, "output": 10, "cache_read": 1.25},
                        "reasoning": false,
                        "modalities": {"input": ["text", "image"], "output": ["text"]}
                    }
                }
            },
            "deepseek": {
                "id": "deepseek",
                "models": {
                    "deepseek-v4-flash": {
                        "id": "deepseek-v4-flash",
                        "name": "DeepSeek V4 Flash",
                        "limit": {"context": 64000, "output": 32000},
                        "cost": {"input": 0.5, "output": 1.5, "cache_read": 0.3},
                        "reasoning": false,
                        "modalities": {"input": ["text"], "output": ["text"]}
                    }
                }
            },
            "anthropic": {
                "id": "anthropic",
                "models": {
                    "claude-3-5-sonnet": {
                        "id": "claude-3-5-sonnet",
                        "name": "Claude 3.5 Sonnet",
                        "limit": {"context": 200000, "output": 8192},
                        "cost": {"input": 3, "output": 15},
                        "reasoning": false,
                        "modalities": {"input": ["text", "image"], "output": ["text"]}
                    }
                }
            }
        });
        let profile = ProviderProfile {
            preset: None,
            models_dev_provider: None,
            ..Default::default()
        };
        let models = vec![
            ModelEntry { id: "gpt-4o".into(), context_window: 1000, ..Default::default() },
            ModelEntry { id: "deepseek-v4-flash".into(), context_window: 1000, ..Default::default() },
        ];
        let (out, stats) = enrich_models_with_catalog_with_stats(&profile, models, Some(&catalog), None);
        assert_eq!(stats.enriched, 2, "聚合 profile 应通过全目录回退命中 gpt-4o 与 deepseek-v4-flash");
        assert_eq!(stats.skipped, 0);
        assert_eq!(stats.failed, 0);
        assert_eq!(out[0].context_window, 128000);
        assert_eq!(out[0].name.as_deref(), Some("GPT-4o"));
        assert_eq!(out[1].context_window, 64000);
        assert_eq!(out[1].name.as_deref(), Some("DeepSeek V4 Flash"));
    }

    #[test]
    fn enrich_global_fallback_prefix_compat() {
        // provider/model 前缀兼容：如 deepseek/deepseek-v4-flash vs deepseek-v4-flash
        let catalog = json!({
            "deepseek": {
                "id": "deepseek",
                "models": {
                    "deepseek-v4-flash": {
                        "id": "deepseek-v4-flash",
                        "name": "DeepSeek V4 Flash",
                        "limit": {"context": 64000, "output": 32000},
                        "cost": {"input": 0.5, "output": 1.5},
                        "reasoning": false,
                        "modalities": {"input": ["text"], "output": ["text"]}
                    }
                }
            }
        });
        let profile = ProviderProfile { preset: None, ..Default::default() };
        let models = vec![ModelEntry { id: "deepseek/deepseek-v4-flash".into(), context_window: 1000, ..Default::default() }];
        let (out, stats) = enrich_models_with_catalog_with_stats(&profile, models, Some(&catalog), None);
        assert_eq!(stats.enriched, 1, "provider/model 前缀应被剥离后命中");
        assert_eq!(stats.skipped, 0);
        assert_eq!(out[0].context_window, 64000);
        // 反向：本地无前缀，目录命中 plain 也应兼容（已在上一用例验证）
    }

    #[test]
    fn enrich_global_fallback_stats_for_ids() {
        let catalog = json!({
            "openai": { "id": "openai", "models": { "gpt-4o": {"id":"gpt-4o","limit":{"context":128000,"output":16384}} } },
            "deepseek": { "id": "deepseek", "models": { "deepseek-v4-flash": {"id":"deepseek-v4-flash","limit":{"context":64000,"output":32000}} } }
        });
        let profile = ProviderProfile { preset: Some("custom".into()), ..Default::default() };
        // custom preset 无映射，per-profile 会被 skipped，但全局应回退命中
        let ids = vec!["gpt-4o".to_string(), "deepseek-v4-flash".to_string(), "unknown-model".to_string()];
        let stats = enrich_stats_for_ids(&profile, &ids, Some(&catalog), None);
        assert_eq!(stats.enriched, 2, "enrich_stats_for_ids 也应全局回退");
        assert_eq!(stats.skipped, 1);
        assert_eq!(stats.failed, 0);
    }

    #[test]
    fn enrich_global_fallback_preserves_per_profile_priority() {
        // per-profile 命中时不应被全局覆盖，保持原 enriched 统计语义
        let catalog = json!({
            "openai": { "id": "openai", "models": { "gpt-4o": {"id":"gpt-4o","limit":{"context":128000,"output":16384},"cost":{"input":2.5,"output":10}} } },
            "deepseek": { "id": "deepseek", "models": { "gpt-4o": {"id":"gpt-4o","limit":{"context":99999,"output":99999},"cost":{"input":99,"output":99}} } }
        });
        let profile = ProviderProfile { preset: Some("openai".into()), ..Default::default() };
        let models = vec![ModelEntry { id: "gpt-4o".into(), context_window: 1000, ..Default::default() }];
        let (out, stats) = enrich_models_with_catalog_with_stats(&profile, models, Some(&catalog), None);
        assert_eq!(stats.enriched, 1);
        assert_eq!(out[0].context_window, 128000, "应优先命中 per-profile 的 openai，而非 deepseek 的全局条目");
    }

    #[test]
    fn enrich_global_fallback_still_skipped_when_absent() {
        let catalog = json!({
            "openai": { "id": "openai", "models": { "gpt-4o": {"id":"gpt-4o","limit":{"context":128000,"output":16384}} } }
        });
        let profile = ProviderProfile { preset: None, ..Default::default() };
        let models = vec![ModelEntry { id: "nonexistent-model-xyz".into(), context_window: 1000, ..Default::default() }];
        let (out, stats) = enrich_models_with_catalog_with_stats(&profile, models, Some(&catalog), None);
        assert_eq!(stats.enriched, 0);
        assert_eq!(stats.skipped, 1);
        assert_eq!(out[0].context_window, 1000, "真正缺失时仍跳过并保留原值");
    }

    #[test]
    fn enrich_stats_failed_no_cache_and_network_down() {
        let profile = profile_openai();
        let ids = vec!["gpt-4o".to_string(), "o1".to_string()];
        let warning = Some("模型目录拉取失败且无可用缓存: network down".to_string());
        let stats = enrich_stats_for_ids(&profile, &ids, None, warning.clone());
        assert_eq!(stats.failed, 2);
        assert_eq!(stats.enriched, 0);
        assert_eq!(stats.warning, warning);
    }
    // ── 思考等级 thinkingLevelMap enrich ──────────────────────────
    #[test]
    fn enrich_thinking_level_map_generated_when_empty_and_effort_present() {
        let profile = profile_openai();
        // 目录命中：reasoning=true + effort ["high","max"]（如 deepseek）
        let catalog = json!({
            "openai": {
                "id": "openai",
                "models": {
                    "deepseek-v4": {
                        "id": "deepseek-v4",
                        "reasoning": true,
                        "reasoning_options": [{"type": "effort", "values": ["high", "max"]}]
                    }
                }
            }
        });
        let models = vec![ModelEntry { id: "deepseek-v4".into(), thinking_level_map: None, ..Default::default() }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        let map = out[0].thinking_level_map.as_ref().expect("应生成 thinkingLevelMap");
        let obj = map.as_object().expect("map is object");
        // off 始终为 null
        assert_eq!(obj.get("off"), Some(&Value::Null));
        // high 精确命中
        assert_eq!(obj.get("high").and_then(|v| v.as_str()), Some("high"));
        // max 精确命中
        assert_eq!(obj.get("max").and_then(|v| v.as_str()), Some("max"));
        // xhigh 不在 ["high","max"] 中，应回退到 max（最近）
        assert_eq!(obj.get("xhigh").and_then(|v| v.as_str()), Some("max"));
        // minimal/low/medium 不在目录中，应回退到 high（最小可用）
        assert_eq!(obj.get("minimal").and_then(|v| v.as_str()), Some("high"));
        assert_eq!(obj.get("low").and_then(|v| v.as_str()), Some("high"));
        assert_eq!(obj.get("medium").and_then(|v| v.as_str()), Some("high"));
    }

    #[test]
    fn enrich_thinking_level_map_openai_four_levels() {
        let profile = profile_openai();
        let catalog = json!({
            "openai": {
                "id": "openai",
                "models": {
                    "gpt-5": {
                        "id": "gpt-5",
                        "reasoning": true,
                        "reasoning_options": [{"type": "effort", "values": ["minimal", "low", "medium", "high"]}]
                    }
                }
            }
        });
        let models = vec![ModelEntry { id: "gpt-5".into(), thinking_level_map: None, ..Default::default() }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        let map = out[0].thinking_level_map.as_ref().expect("应生成 map");
        let obj = map.as_object().unwrap();
        assert_eq!(obj.get("off"), Some(&Value::Null));
        assert_eq!(obj.get("minimal").and_then(|v| v.as_str()), Some("minimal"));
        assert_eq!(obj.get("low").and_then(|v| v.as_str()), Some("low"));
        assert_eq!(obj.get("medium").and_then(|v| v.as_str()), Some("medium"));
        assert_eq!(obj.get("high").and_then(|v| v.as_str()), Some("high"));
        // xhigh/max 不在目录，回退到 high
        assert_eq!(obj.get("xhigh").and_then(|v| v.as_str()), Some("high"));
        assert_eq!(obj.get("max").and_then(|v| v.as_str()), Some("high"));
    }

    #[test]
    fn enrich_thinking_level_map_with_none_value_filters() {
        let profile = profile_openai();
        // 含 none 的目录值（如 gpt-5.6）应过滤 none，off 仍为 null
        let catalog = json!({
            "openai": {
                "id": "openai",
                "models": {
                    "gpt-5.6": {
                        "id": "gpt-5.6",
                        "reasoning": true,
                        "reasoning_options": [{"type": "effort", "values": ["none", "low", "medium", "high", "xhigh", "max"]}]
                    }
                }
            }
        });
        let models = vec![ModelEntry { id: "gpt-5.6".into(), thinking_level_map: None, ..Default::default() }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        let map = out[0].thinking_level_map.as_ref().expect("应生成 map");
        let obj = map.as_object().unwrap();
        assert_eq!(obj.get("off"), Some(&Value::Null));
        // minimal 不在有效值，回退到 low
        assert_eq!(obj.get("minimal").and_then(|v| v.as_str()), Some("low"));
        assert_eq!(obj.get("low").and_then(|v| v.as_str()), Some("low"));
        assert_eq!(obj.get("xhigh").and_then(|v| v.as_str()), Some("xhigh"));
        assert_eq!(obj.get("max").and_then(|v| v.as_str()), Some("max"));
        // 不应出现 "none" 字符串
        for (_k, v) in obj.iter() {
            assert_ne!(v.as_str(), Some("none"), "map 不应包含 none 字符串");
        }
    }

    #[test]
    fn enrich_thinking_level_map_preserves_manual_value() {
        let profile = profile_openai();
        let catalog = json!({
            "openai": {
                "id": "openai",
                "models": {
                    "my-model": {
                        "id": "my-model",
                        "reasoning": true,
                        "reasoning_options": [{"type": "effort", "values": ["high", "max"]}]
                    }
                }
            }
        });
        let manual = json!({"off": null, "high": "high"});
        let models = vec![ModelEntry { id: "my-model".into(), thinking_level_map: Some(manual.clone()), ..Default::default() }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        // 非空即手工：应保留原值不覆盖
        assert_eq!(out[0].thinking_level_map, Some(manual));
    }

    #[test]
    fn enrich_thinking_level_map_remains_empty_when_no_effort() {
        let profile = profile_openai();
        // reasoning=true 但无 effort（仅 toggle 或空数组）→ 留空不设默认
        let catalog_toggle = json!({
            "openai": {
                "id": "openai",
                "models": {
                    "kimi-toggle": {
                        "id": "kimi-toggle",
                        "reasoning": true,
                        "reasoning_options": [{"type": "toggle"}]
                    }
                }
            }
        });
        let models = vec![ModelEntry { id: "kimi-toggle".into(), thinking_level_map: None, ..Default::default() }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog_toggle));
        assert!(out[0].thinking_level_map.is_none(), "toggle-only 不应生成 map，留空");

        // 空 reasoning_options
        let catalog_empty = json!({
            "openai": {
                "id": "openai",
                "models": {
                    "empty-opts": {
                        "id": "empty-opts",
                        "reasoning": true,
                        "reasoning_options": []
                    }
                }
            }
        });
        let models2 = vec![ModelEntry { id: "empty-opts".into(), thinking_level_map: None, ..Default::default() }];
        let out2 = enrich_models_with_catalog(&profile, models2, Some(&catalog_empty));
        assert!(out2[0].thinking_level_map.is_none(), "空数组不应生成 map");
    }

    #[test]
    fn enrich_thinking_level_map_remains_empty_when_reasoning_false() {
        let profile = profile_openai();
        let catalog = json!({
            "openai": {
                "id": "openai",
                "models": {
                    "gpt-4o": {
                        "id": "gpt-4o",
                        "reasoning": false
                    }
                }
            }
        });
        let models = vec![ModelEntry { id: "gpt-4o".into(), thinking_level_map: None, ..Default::default() }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        assert!(out[0].thinking_level_map.is_none(), "reasoning=false 不应生成 map");
        // 若原有空 object 也不覆盖？视为空，仍不生成
        let models2 = vec![ModelEntry { id: "gpt-4o".into(), thinking_level_map: Some(json!({})), ..Default::default() }];
        let out2 = enrich_models_with_catalog(&profile, models2, Some(&catalog));
        // 空对象视为手工？当前定义非空即手工，空对象应被视为可填充但因 reasoning=false 仍为 None/空 → 保持空
        // 实现可选择保留空对象或 None，此处断言不生成有效映射
        if let Some(v) = out2[0].thinking_level_map.as_ref() {
            assert!(v.as_object().map(|o| o.is_empty()).unwrap_or(true), "reasoning=false 时空对象不应被填充为有效 map");
        }
    }

    #[test]
    fn enrich_thinking_level_map_empty_object_treated_as_empty() {
        let profile = profile_openai();
        let catalog = json!({
            "openai": {
                "id": "openai",
                "models": {
                    "deepseek-v4": {
                        "id": "deepseek-v4",
                        "reasoning": true,
                        "reasoning_options": [{"type": "effort", "values": ["high", "max"]}]
                    }
                }
            }
        });
        // 空对象视为可填充（与 None 同等）
        let models = vec![ModelEntry { id: "deepseek-v4".into(), thinking_level_map: Some(json!({})), ..Default::default() }];
        let out = enrich_models_with_catalog(&profile, models, Some(&catalog));
        assert!(out[0].thinking_level_map.is_some(), "空对象应被填充");
        let obj = out[0].thinking_level_map.as_ref().unwrap().as_object().unwrap();
        assert_eq!(obj.get("off"), Some(&Value::Null));
    }


}

fn backup_models() -> Option<PathBuf> {
    let models_path = config::models_path();
    if !models_path.exists() {
        return None;
    }
    let ts = chrono::Utc::now().format("%Y-%m-%dT%H-%M-%S-%3fZ");
    let backup_path = config::backup_dir().join(format!("models-{}.json", ts));
    std::fs::create_dir_all(config::backup_dir()).ok();
    std::fs::copy(&models_path, &backup_path).ok()?;
    Some(backup_path)
}

pub fn use_profile(name: &str, mode: Option<&str>) -> Result<UseOutcome> {
    let mut config = load_config()?;

    let mode = mode
        .map(str::to_string)
        .unwrap_or_else(|| config.settings.write_mode.clone());
    let provider_id = provider_id_for(&config, name);

    let models_path = config::models_path();
    let models_backup = backup_models();

    // Handle exclusive mode
    if mode == "exclusive" {
        let mut models: serde_json::Value = if models_path.exists() {
            let text = std::fs::read_to_string(&models_path).unwrap_or_default();
            serde_json::from_str(&text).unwrap_or(serde_json::json!({ "providers": {} }))
        } else {
            serde_json::json!({ "providers": {} })
        };

        if let Some(providers) = models["providers"].as_object_mut() {
            let prefix = format!("{}-", config.settings.provider_prefix);
            providers.retain(|k, _| !k.starts_with(&prefix));
            write_models_atomic(&models)?;
        }
    }


    let config_backup = backup_config("config")?;

    config.current = Some(name.to_string());
    save_config(&config)?;

    Ok(UseOutcome {
        name: name.to_string(),
        provider_id,
        models_backup,
        config_backup,
    })
}

pub fn upsert_profile(
    name: &str,
    profile: &ProviderProfile,
    rename_from: Option<&str>,
) -> Result<Option<PathBuf>> {
    config::validate_provider_profile(name, profile).map_err(AppError::InvalidInput)?;

    // 模型目录 enrich：新增/编辑 profile 时若已携带 models，则尝试用缓存补齐元数据（离线有缓存仍 enrich）
    let mut profile_owned = profile.clone();
    if !profile_owned.models.is_empty() {
        let (enriched, stats) = try_enrich_from_cache_sync_with_stats(&profile_owned, profile_owned.models.clone());
        profile_owned.models = enriched;
        if let Some(w) = &stats.warning {
            log::warn!("模型目录 enrich 警告: {}", w);
        }
        if stats.failed > 0 {
            log::warn!("模型目录 enrich 失败 {} 条", stats.failed);
        } else if stats.enriched > 0 || stats.skipped > 0 {
            log::info!("模型目录 enrich: 已 enrich {} 条，跳过 {} 条（模型目录未覆盖）", stats.enriched, stats.skipped);
        }
    }

    let mut config = load_config()?;
    let backup = backup_config("config")?;

    if let Some(old) = rename_from {
        if old != name {
            config.profiles.remove(old);
            if config.current.as_deref() == Some(old) {
                config.current = Some(name.to_string());
            }
        }
    }

    config.profiles.insert(
        name.to_string(),
        serde_json::to_value(&profile_owned).map_err(|e| AppError::json(config::config_path(), e))?,
    );
    if config.current.is_none() {
        config.current = Some(name.to_string());
    }
    save_config(&config)?;


    Ok(backup)
}

pub fn remove_profile(name: &str) -> Result<Option<PathBuf>> {
    let mut config = load_config()?;
    if !config.profiles.contains_key(name) {
        return Err(AppError::Message(format!("unknown profile '{}'", name)));
    }

    let backup = backup_config("config")?;

    config.profiles.remove(name);
    if config.current.as_deref() == Some(name) {
        config.current = config.profiles.keys().next().cloned();
    }
    save_config(&config)?;


    Ok(backup)
}

pub fn duplicate_profile(src: &str, dst: &str) -> Result<Option<PathBuf>> {
    let mut config = load_config()?;
    let profile = config
        .profiles
        .get(src)
        .ok_or_else(|| AppError::Message(format!("unknown profile '{}'", src)))?
        .clone();
    if config.profiles.contains_key(dst) {
        return Err(AppError::Message(format!(
            "profile '{}' already exists",
            dst
        )));
    }

    let backup = backup_config("config")?;
    config.profiles.insert(dst.to_string(), profile);
    save_config(&config)?;

    Ok(backup)
}

// ─── Provider Testing ─────────────────────────────────────

#[derive(serde::Serialize)]
pub struct TestResult {
    pub success: bool,
    pub message: String,
    pub response_time_ms: Option<u64>,
}

pub async fn test_provider(name: &str) -> Result<TestResult> {
    let config = load_config()?;
    let profile_value = config
        .profiles
        .get(name)
        .ok_or_else(|| AppError::Message(format!("unknown profile '{}'", name)))?;

    let profile: ProviderProfile = serde_json::from_value(profile_value.clone())
        .map_err(|e| AppError::Message(format!("invalid profile: {}", e)))?;

    let start = std::time::Instant::now();

    // Build test request based on API type
    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(10))
        .build()
        .map_err(|e| AppError::Message(format!("HTTP client error: {}", e)))?;

    let test_body = match profile.api.as_str() {
        "openai-completions" => serde_json::json!({
            "model": profile.models.first().map(|m| &m.id).unwrap_or(&"gpt-3.5-turbo".to_string()),
            "messages": [{"role": "user", "content": "test"}],
            "max_tokens": 5
        }),
        "anthropic-messages" => serde_json::json!({
            "model": profile.models.first().map(|m| &m.id).unwrap_or(&"claude-3-haiku-20240307".to_string()),
            "messages": [{"role": "user", "content": "test"}],
            "max_tokens": 5
        }),
        _ => {
            return Ok(TestResult {
                success: false,
                message: format!("Unsupported API type: {}", profile.api),
                response_time_ms: None,
            });
        }
    };

    let url = format!(
        "{}/chat/completions",
        profile.primary_base_url().trim_end_matches('/')
    );
    let mut req = client.post(&url).json(&test_body);

    // Add authorization header
    if profile.api == "anthropic-messages" {
        req = req
            .header("x-api-key", profile.primary_api_key())
            .header("anthropic-version", "2023-06-01");
    } else {
        req = req.header("Authorization", format!("Bearer {}", profile.primary_api_key()));
    }

    match req.send().await {
        Ok(resp) => {
            let elapsed = start.elapsed().as_millis() as u64;
            let status = resp.status();

            if status.is_success() {
                Ok(TestResult {
                    success: true,
                    message: format!("✓ Connected successfully (HTTP {})", status.as_u16()),
                    response_time_ms: Some(elapsed),
                })
            } else {
                let error_text = resp.text().await.unwrap_or_else(|_| "Unknown error".into());
                Ok(TestResult {
                    success: false,
                    message: format!(
                        "✗ HTTP {} - {}",
                        status.as_u16(),
                        error_text.chars().take(100).collect::<String>()
                    ),
                    response_time_ms: Some(elapsed),
                })
            }
        }
        Err(e) => {
            let elapsed = start.elapsed().as_millis() as u64;
            Ok(TestResult {
                success: false,
                message: format!("✗ Connection failed: {}", e),
                response_time_ms: Some(elapsed),
            })
        }
    }
}

// ─── Fetch Models ─────────────────────────────────────────

async fn fetch_upstream_ids(profile: &ProviderProfile) -> Result<Vec<String>> {
    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(10))
        .build()
        .map_err(|e| AppError::Message(format!("HTTP client error: {}", e)))?;

    // 兼容多上游：取首个生效 upstream（向后兼容单 baseUrl）
    let api_key = crate::config::resolve_env(profile.primary_api_key());
    let base_url = profile.primary_base_url();
    // Build candidate URLs (try multiple common endpoints)
    let candidate_urls = build_model_fetch_urls(base_url, &profile.api);
    let mut last_error = String::from("No candidate URLs");

    for url in candidate_urls {
        let mut req = client.get(&url);

        // Set auth headers based on API type
        req = match profile.api.as_str() {
            "openai-completions" => req.header("Authorization", format!("Bearer {}", api_key)),
            "anthropic-messages" => req
                .header("x-api-key", &api_key)
                .header("anthropic-version", "2023-06-01"),
            _ => req.header("Authorization", format!("Bearer {}", api_key)),
        };

        match req.send().await {
            Ok(resp) => {
                let status = resp.status();
                if !status.is_success() {
                    last_error = format!("HTTP {} ({})", status.as_u16(), url);
                    // Skip 404/405 and try next URL
                    if status == reqwest::StatusCode::NOT_FOUND
                        || status == reqwest::StatusCode::METHOD_NOT_ALLOWED
                    {
                        continue;
                    }
                    return Err(AppError::Message(last_error));
                }

                match resp.json::<serde_json::Value>().await {
                    Ok(payload) => {
                        let models = parse_model_ids(&payload);
                        if models.is_empty() {
                            last_error = format!("No models found in response ({})", url);
                            continue;
                        }
                        return Ok(models);
                    }
                    Err(e) => {
                        last_error = format!("Invalid JSON ({}): {}", url, e);
                    }
                }
            }
            Err(e) => {
                last_error = format!("Request failed ({}): {}", url, e);
            }
        }
    }

    Err(AppError::Message(last_error))
}

pub async fn fetch_models(name: &str) -> Result<Vec<String>> {
    let config = load_config()?;
    let profile_value = config
        .profiles
        .get(name)
        .ok_or_else(|| AppError::Message(format!("unknown profile '{}'", name)))?;

    let profile: ProviderProfile = serde_json::from_value(profile_value.clone())
        .map_err(|e| AppError::Message(format!("invalid profile: {}", e)))?;

    fetch_upstream_ids(&profile).await
}

/// Fetch 上游模型列表并附带模型目录 enrich 统计（用于可观测性：24h 缓存命中、无重复网络请求；过期或网络失败时回退到过期缓存并报告 warning，无缓存且失败时跳过 enrich 并提示）
/// 术语统一使用“模型目录 / 模型元数据”，与“上游模型列表”区分
pub async fn fetch_models_with_stats(name: &str) -> Result<(Vec<String>, EnrichStats)> {
    let config = load_config()?;
    let profile_value = config
        .profiles
        .get(name)
        .ok_or_else(|| AppError::Message(format!("unknown profile '{}'", name)))?;

    let profile: ProviderProfile = serde_json::from_value(profile_value.clone())
        .map_err(|e| AppError::Message(format!("invalid profile: {}", e)))?;

    // 先拉取上游模型列表（上游仅返回 id 列表）
    let ids = fetch_upstream_ids(&profile).await?;

    // 再尝试获取模型目录（带 24h TTL 缓存与降级）
    let (catalog_opt, warning) = match crate::catalog::get_or_refresh_catalog_with_warning().await {
        Ok((v, w)) => (v, w),
        Err(e) => {
            let w = format!("模型目录获取失败，跳过模型元数据 enrich: {}", e);
            log::warn!("{}", w);
            (None, Some(w))
        }
    };

    let stats = enrich_stats_for_ids(&profile, &ids, catalog_opt.as_ref(), warning);

    // 可观测性日志（术语：模型目录 / 模型元数据 / 上游模型列表）
    if let Some(w) = &stats.warning {
        log::warn!("模型目录 enrich 警告: {}", w);
    }
    if stats.failed > 0 {
        log::warn!("模型目录 enrich 失败 {} 条（上游模型列表 {} 条），已回退或跳过", stats.failed, ids.len());
    } else {
        log::info!(
            "Fetch 上游模型列表 {} 条，模型目录 enrich: 已 enrich {} 条，跳过 {} 条（模型目录未覆盖）",
            ids.len(),
            stats.enriched,
            stats.skipped
        );
    }

    Ok((ids, stats))
}

// ─── Sync Gateway Provider to Pi Config ──────────────────
// 注意：已拆分至 `crate::gateway` 独立模块（文件锁/原子写隔离）。
// - Providers 侧仅操作 config.json（`config::save_config` 独立锁）
// - Gateway 侧仅操作 models.json（`gateway::write_models_atomic` 独立锁与 notify）
// - 互不阻塞，供应商离线/失败不影响 Gateway 读/预览/应用；错误边界已隔离
// - Gateway 进程间通过文件 `gateway.notify` 或未来 channel/ipc 通知

#[allow(unused_imports)]
pub use crate::gateway::{apply_gateway, get_gateway, preview_gateway, sync_gateway_to_pi, GatewayPreview};

/// Provider 独立 mod：仅负责供应商 profile 的 CRUD/校验/enrich，不触碰 Gateway 文件
/// 未来可整体移至 `crate::provider` 并独立进程化
pub mod provider {
    // 语义占位：当前实现仍在 `super::`，下一阶段抽离为独立文件 `provider.rs`
    // 供调用方以 `ops::provider::*` 明确边界；错误不穿透至 gateway
}

/// Gateway 独立 mod：Sync/Preview/Apply 已隔离至 `crate::gateway`
/// - 读写均在独立文件锁与原子写内完成（providers.json vs gateway.json 语义隔离：config.json vs models.json）
/// - 进程间通过文件 `gateway.notify` 通知，错误边界隔离（gateway 失败不阻断 providers）
pub mod gateway {
    #[allow(unused_imports)]
    pub use crate::gateway::{
        apply_gateway, get_gateway, notify_gateway_changed, preview_gateway, sync_gateway_to_pi,
        GatewayPreview,
    };
    /// 显式文件通知通道（轻量 mtime），下一阶段可替换为 channel/ipc；错误已隔离不阻断主流程
    #[allow(dead_code)]
    pub fn notify_via_file() -> crate::error::Result<()> {
        crate::gateway::notify_gateway_changed()
    }
}

// Build multiple candidate URLs to try (following cc-switch logic)
// Build multiple candidate URLs to try (following cc-switch logic)
pub fn build_model_fetch_urls(base_url: &str, api_type: &str) -> Vec<String> {
    let base = base_url.trim().trim_end_matches('/');
    if base.is_empty() {
        return Vec::new();
    }

    // If already ends with /models, use it directly
    if base.ends_with("/models") {
        return vec![base.to_string()];
    }

    let mut urls = Vec::new();
    let append_models = format!("{}/models", base);
    let has_version_suffix = base.ends_with("/v1") || base.ends_with("/v1beta");

    match api_type {
        "anthropic-messages" => {
            // Try /v1/models first for Anthropic-compatible endpoints
            if !has_version_suffix {
                urls.push(format!("{}/v1/models", base));
            } else {
                urls.push(append_models.clone());
            }

            // Try stripping known compatibility suffixes
            if let Some(stripped) = strip_compat_suffix(base) {
                let root = stripped.trim_end_matches('/');
                if !root.is_empty() && root.contains("://") {
                    urls.push(format!("{}/v1/models", root));
                    urls.push(format!("{}/models", root));
                }
            } else if !has_version_suffix {
                urls.push(append_models);
            }
        }
        _ => {
            // OpenAI and others: try /models, then /v1/models
            urls.push(append_models);
            if !has_version_suffix {
                urls.push(format!("{}/v1/models", base));
            }
        }
    }

    // Deduplicate
    let mut seen = std::collections::HashSet::new();
    urls.retain(|url| seen.insert(url.clone()));
    urls
}

// Strip known compatibility path suffixes (e.g., /api/anthropic, /claudecode)
fn strip_compat_suffix(base: &str) -> Option<&str> {
    const KNOWN_SUFFIXES: &[&str] = &[
        "/api/claudecode",
        "/api/anthropic",
        "/apps/anthropic",
        "/api/coding",
        "/claudecode",
        "/anthropic",
        "/step_plan",
        "/coding",
        "/claude",
    ];

    let lower = base.to_ascii_lowercase();
    KNOWN_SUFFIXES.iter().find_map(|suffix| {
        lower
            .ends_with(suffix)
            .then(|| &base[..base.len() - suffix.len()])
    })
}

// Parse model IDs from various response formats
pub fn parse_model_ids(payload: &serde_json::Value) -> Vec<String> {
    let mut out: Vec<String> = Vec::new();

    // Try OpenAI format: { "data": [{"id": "..."}, ...] }
    if let Some(data) = payload.get("data").and_then(|v| v.as_array()) {
        for item in data {
            if let Some(id) = item.get("id").and_then(|v| v.as_str()) {
                out.push(id.to_string());
            }
        }
    }

    // Try Google format: { "models": [{"name": "models/..."}, ...] }
    if out.is_empty() {
        if let Some(models) = payload.get("models").and_then(|v| v.as_array()) {
            for item in models {
                if let Some(name) = item.get("name").and_then(|v| v.as_str()) {
                    out.push(name.strip_prefix("models/").unwrap_or(name).to_string());
                }
            }
        }
    }

    // Try direct array: [{"id": "..."}, ...]
    if out.is_empty() {
        if let Some(arr) = payload.as_array() {
            for item in arr {
                if let Some(id) = item.get("id").and_then(|v| v.as_str()) {
                    out.push(id.to_string());
                }
            }
        }
    }

    // Deduplicate
    let mut seen = std::collections::HashSet::new();
    out.retain(|model| seen.insert(model.clone()));
    out
}

/// Deprecated: routing is now driven by the model name in the request body (gateway mode),
/// so there is no single "target". Kept for config back-compat — it records the field and
/// refreshes the gateway, but the value no longer affects routing.
pub fn set_proxy_target(target: Option<&str>) -> Result<()> {
    let mut config = load_config()?;

    if let Some(name) = target {
        if !config.profiles.contains_key(name) {
            return Err(AppError::Message(format!("Profile '{}' not found", name)));
        }
        config.settings.proxy.target = Some(name.to_string());
    } else {
        config.settings.proxy.target = None;
    }

    save_config(&config)?;
    sync_gateway_to_pi()?;
    Ok(())
}

/// Replace the whole `settings` object (providerPrefix / writeMode / language / proxy / web).
/// Front-ends read the current settings via `service::get_state`, edit, and send the full
/// object back. Re-syncs the gateway since prefix/host/port feed pi's models.json.
pub fn update_settings(new_settings: &serde_json::Value) -> Result<Option<PathBuf>> {
    let mut config = load_config()?;
    let settings: config::Settings = serde_json::from_value(new_settings.clone())
        .map_err(|e| AppError::Message(format!("invalid settings: {}", e)))?;

    let backup = backup_config("config")?;
    config.settings = settings;
    save_config(&config)?;
    sync_gateway_to_pi()?;
    Ok(backup)
}
