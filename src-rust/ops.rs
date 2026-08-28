use crate::config::{
    self, backup_config, load_config, provider_id_for, save_config, ProviderProfile,
};
use crate::error::{AppError, Result};
use std::path::PathBuf;

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
    sync_gateway_to_pi()?;
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

    // Refresh the single gateway provider in pi's models.json
    sync_gateway_to_pi()?;

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
    let mut config = load_config()?;
    let backup = backup_config("config")?;

    let profile_value = config
        .profiles
        .get_mut(name)
        .ok_or_else(|| AppError::Message(format!("unknown profile '{}'", name)))?;

    let mut profile: ProviderProfile = serde_json::from_value(profile_value.clone())
        .map_err(|e| AppError::Message(format!("invalid profile: {}", e)))?;

    profile.models = models;
    profile.updated_at = Some(chrono::Utc::now().to_rfc3339());

    *profile_value =
        serde_json::to_value(&profile).map_err(|e| AppError::json(config::config_path(), e))?;

    save_config(&config)?;

    // Refresh the gateway so model metadata in pi's models.json stays current
    sync_gateway_to_pi()?;

    Ok(backup)
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

    // Sync the gateway provider to pi config
    sync_gateway_to_pi()?;

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
        serde_json::to_value(profile).map_err(|e| AppError::json(config::config_path(), e))?,
    );
    if config.current.is_none() {
        config.current = Some(name.to_string());
    }
    save_config(&config)?;

    // Keep pi's gateway model list in sync with the profiles
    sync_gateway_to_pi()?;

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

    // Rebuild the gateway provider without the removed profile's models
    sync_gateway_to_pi()?;

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
        profile.base_url.trim_end_matches('/')
    );
    let mut req = client.post(&url).json(&test_body);

    // Add authorization header
    if profile.api == "anthropic-messages" {
        req = req
            .header("x-api-key", &profile.api_key)
            .header("anthropic-version", "2023-06-01");
    } else {
        req = req.header("Authorization", format!("Bearer {}", profile.api_key));
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

pub async fn fetch_models(name: &str) -> Result<Vec<String>> {
    let config = load_config()?;
    let profile_value = config
        .profiles
        .get(name)
        .ok_or_else(|| AppError::Message(format!("unknown profile '{}'", name)))?;

    let profile: ProviderProfile = serde_json::from_value(profile_value.clone())
        .map_err(|e| AppError::Message(format!("invalid profile: {}", e)))?;

    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(10))
        .build()
        .map_err(|e| AppError::Message(format!("HTTP client error: {}", e)))?;

    let api_key = crate::config::resolve_env(&profile.api_key);

    // Build candidate URLs (try multiple common endpoints)
    let candidate_urls = build_model_fetch_urls(&profile.base_url, &profile.api);
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

// ─── Sync Gateway Provider to Pi Config ──────────────────

/// Write a single "gateway" provider into pi's models.json. It advertises every non-proxy
/// profile's exposedModels as `profile/realModelId`, all pointing at the local proxy, so pi
/// sees one provider and the proxy routes by the model name in the request body.
///
/// This is the only pi provider pi-switch manages. Any legacy per-profile `{prefix}-*` entries
/// (from the old routing model) are removed; foreign providers are left untouched.
pub fn sync_gateway_to_pi() -> Result<()> {
    let config = load_config()?;
    let mut models = load_models_value()?;
    sync_gateway_with_current(&config, &mut models)?;
    write_models_atomic(&models)
}
// ─── Gateway pre-edit (preview / apply) ─────────────────

#[derive(Debug, Clone, serde::Serialize)]
pub struct GatewayPreview {
    pub current: Option<serde_json::Value>,
    pub proposed: serde_json::Value,
    pub conflicts: Vec<String>,
}

fn load_models_value() -> Result<serde_json::Value> {
    let models_path = config::models_path();
    if models_path.exists() {
        let text =
            std::fs::read_to_string(&models_path).map_err(|e| AppError::io(&models_path, e))?;
        Ok(serde_json::from_str::<serde_json::Value>(&text).unwrap_or(serde_json::json!({ "providers": {} })))
    } else {
        Ok(serde_json::json!({ "providers": {} }))
    }
}

fn build_proposed_gateway_entry(config: &config::PiSwitchConfig) -> serde_json::Value {
    let mut gateway_models: Vec<serde_json::Value> = Vec::new();
    for (name, profile_value) in &config.profiles {
        let profile: ProviderProfile = match serde_json::from_value(profile_value.clone()) {
            Ok(p) => p,
            Err(_) => continue,
        };
        if profile.proxy {
            continue;
        }
        for real_id in &profile.exposed_models {
            let mut entry = profile
                .models
                .iter()
                .find(|m| &m.id == real_id)
                .cloned()
                .unwrap_or_else(|| config::ModelEntry {
                    id: real_id.clone(),
                    ..Default::default()
                });
            entry.id = format!("{}/{}", name, real_id);
            if let Ok(v) = serde_json::to_value(&entry) {
                gateway_models.push(v);
            }
        }
    }
    let host = &config.settings.proxy.host;
    let port = config.settings.proxy.port;
    serde_json::json!({
        "api": "openai-completions",
        "baseUrl": format!("http://{}:{}/v1", host, port),
        "apiKey": "pi-switch-proxy",
        "models": gateway_models,
        "proxy": false,
    })
}

fn merge_gateway_extra(current: &serde_json::Value, proposed: &mut serde_json::Value) {
    // Preserve any top-level keys present in current but not in proposed (handwritten extra)
    if let (Some(cur_obj), Some(prop_obj)) = (current.as_object(), proposed.as_object_mut()) {
        for (k, v) in cur_obj {
            if !prop_obj.contains_key(k) {
                prop_obj.insert(k.clone(), v.clone());
            }
        }
        // For models array, preserve per-model extra fields when ids match:
        // if current model has keys not in proposed model, copy them (handwritten model extra)
        if let (Some(cur_models), Some(prop_models)) = (
            cur_obj.get("models").and_then(|v| v.as_array()),
            prop_obj.get_mut("models").and_then(|v| v.as_array_mut()),
        ) {
            let cur_by_id: std::collections::HashMap<String, &serde_json::Value> = cur_models
                .iter()
                .filter_map(|m| m.get("id").and_then(|id| id.as_str()).map(|id| (id.to_string(), m)))
                .collect();
            for prop_model in prop_models.iter_mut() {
                if let Some(id) = prop_model.get("id").and_then(|id| id.as_str()) {
                    if let Some(cur_model) = cur_by_id.get(id) {
                        if let (Some(cur_mo), Some(prop_mo)) =
                            (cur_model.as_object(), prop_model.as_object_mut())
                        {
                            for (k, v) in cur_mo {
                                if !prop_mo.contains_key(k) {
                                    prop_mo.insert(k.clone(), v.clone());
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

fn compute_gateway_conflicts(current: &serde_json::Value, proposed: &serde_json::Value) -> Vec<String> {
    let mut conflicts = Vec::new();
    let generated_keys = ["api", "baseUrl", "apiKey", "models", "proxy"];
    if let (Some(cur_obj), Some(prop_obj)) = (current.as_object(), proposed.as_object()) {
        for key in generated_keys {
            if let (Some(cur_val), Some(prop_val)) = (cur_obj.get(key), prop_obj.get(key)) {
                if cur_val != prop_val {
                    conflicts.push(key.to_string());
                }
            }
        }
        // Extra keys that would have been overwritten if they existed in proposed — but our merge preserves them,
        // so no conflict for extra keys. However we still report if a preserved extra's value differs? That case
        // doesn't happen because we only preserve missing keys. So conflicts are only generated keys.
    }
    conflicts
}

pub fn get_gateway() -> Result<Option<serde_json::Value>> {
    let config = load_config()?;
    let models = load_models_value()?;
    let gateway_id = config.settings.provider_prefix.clone();
    let entry = models
        .get("providers")
        .and_then(|p| p.get(&gateway_id))
        .cloned();
    Ok(entry)
}

pub fn preview_gateway() -> Result<GatewayPreview> {
    let config = load_config()?;
    let models = load_models_value()?;
    let gateway_id = config.settings.provider_prefix.clone();
    let current = models
        .get("providers")
        .and_then(|p| p.get(&gateway_id))
        .cloned();

    let mut proposed = build_proposed_gateway_entry(&config);
    let conflicts = if let Some(ref cur) = current {
        let c = compute_gateway_conflicts(cur, &proposed);
        merge_gateway_extra(cur, &mut proposed);
        c
    } else {
        Vec::new()
    };

    Ok(GatewayPreview {
        current,
        proposed,
        conflicts,
    })
}

fn validate_gateway_value(value: &serde_json::Value) -> std::result::Result<(), String> {
    let obj = value.as_object().ok_or("gateway must be an object")?;
    let api = obj.get("api").and_then(|v| v.as_str()).unwrap_or("");
    if api.is_empty() {
        return Err("gateway.api is required".into());
    }
    if !crate::config::SUPPORTED_APIS.contains(&api) {
        return Err(format!("gateway.api is not supported: {}", api));
    }
    let base_url = obj.get("baseUrl").and_then(|v| v.as_str()).unwrap_or("");
    if base_url.is_empty() {
        return Err("gateway.baseUrl is required".into());
    }
    if !base_url.starts_with("http://") && !base_url.starts_with("https://") {
        return Err("gateway.baseUrl must start with http:// or https://".into());
    }
    let models = obj.get("models").and_then(|v| v.as_array()).ok_or("gateway.models must be an array")?;
    for (i, m) in models.iter().enumerate() {
        let mid = m.get("id").and_then(|v| v.as_str()).unwrap_or("");
        if mid.trim().is_empty() {
            return Err(format!("gateway.models[{}].id must not be empty", i));
        }
    }
    Ok(())
}

pub fn apply_gateway(edited: serde_json::Value) -> Result<()> {
    validate_gateway_value(&edited).map_err(AppError::Message)?;
    let config = load_config()?;
    let mut models = load_models_value()?;
    let providers = models["providers"]
        .as_object_mut()
        .ok_or_else(|| AppError::Message("invalid models.json".into()))?;
    let gateway_id = config.settings.provider_prefix.clone();
    // backup models.json before write
    let _ = backup_models();
    providers.insert(gateway_id, edited);
    write_models_atomic(&models)
}

// Helper for sync_gateway_to_pi to reuse preview merging
fn sync_gateway_with_current(config: &config::PiSwitchConfig, models: &mut serde_json::Value) -> Result<()> {
    let gateway_id = config.settings.provider_prefix.clone();
    let current = models
        .get("providers")
        .and_then(|p| p.get(&gateway_id))
        .cloned();
    let mut proposed = build_proposed_gateway_entry(config);
    if let Some(ref cur) = current {
        // compute conflicts is not needed for sync, but we merge
        merge_gateway_extra(cur, &mut proposed);
    }
    let providers = models["providers"]
        .as_object_mut()
        .ok_or_else(|| AppError::Message("invalid models.json".into()))?;
    let legacy_prefix = format!("{}-", gateway_id);
    providers.retain(|k, _| k != &gateway_id && !k.starts_with(&legacy_prefix));
    providers.insert(gateway_id, proposed);
    Ok(())
}


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
