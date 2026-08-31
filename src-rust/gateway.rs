//! Gateway 独立进程/插件第一阶段：存储与逻辑隔离 → 第二阶段 Web 路由与进程占位
//!
//! - Providers 仅写 `config.json`（由 `config::save_config` 负责，原子写 + 独立锁）
//! - Gateway 显式通过 `PUT /models/gateway`（`apply_gateway`）写 `models.json`，独立文件锁与原子写
//! - 两者文件级隔离：`providers.json` 语义即 `config.json` 的 profiles 段，`gateway.json` 语义即 `models.json` 的 gateway provider；
//!   互不阻塞，供应商离线（fetch 失败）不影响 Gateway 读取/预览/应用
//! - Gateway 进程通过文件通知（`gateway.notify`）或 channel 隔离错误边界，下一阶段可接入独立进程
//!
//! ## 进程生命周期占位（第二阶段，仅接口预留，未真拆进程）
//! - `gateway::lifecycle::GatewayHealth` / `health_check()` / `start_placeholder()` 为独立进程预留的启动与健康检查接口（当前为逻辑隔离，复用文件通知）
//! - 后续真拆路径见 `docs/agents/gateway-process.md`（本文件末尾注释）或 `.scratch/gateway-sep-full-webui/process.md`：
//!   1. 将 `gateway::write_models_atomic` 与 `daemon` 职责合并为独立 `pi-switch-gateway` 二进制
//!   2. Web 侧 `/api/gateway/health` → `gateway::health_check`（当前占位返回基于 models.json + notify 的合成状态）
//!   3. 用 IPC/channel 替换 `gateway.notify` 文件，providers 侧仅发通知，不阻塞
//!   4. 独立进程内监听文件/ipc 触发 `sync_gateway_with_current` 或热加载，错误边界保持 providers 侧可用

use crate::config::{self, ProviderProfile};
use crate::error::{AppError, Result};
use std::path::PathBuf;

// ─── 文件路径与独立锁说明 ──────────────────────────────────────────────
// providers 侧：config::config_path()  -> ~/.pi-switch/config.json
// gateway 侧：  config::models_path()  -> ~/.pi/agent/models.json
// 两文件各自原子写（tmp + rename），无交叉锁；未来可升级为 advisory file lock（fs2 / flock）
fn gateway_models_path() -> PathBuf {
    config::models_path()
}

fn gateway_notify_path() -> PathBuf {
    config::config_dir().join("gateway.notify")
}

/// 通知 Gateway 进程：通过文件 mtime 触发（下一阶段可替换为 channel/ipc）
pub fn notify_gateway_changed() -> Result<()> {
    let p = gateway_notify_path();
    // 轻量通知：写时间戳文件，不阻塞 providers 流程
    let ts = chrono::Utc::now().to_rfc3339();
    let dir = config::config_dir();
    let _ = std::fs::create_dir_all(&dir);
    let _ = std::fs::write(&p, ts);
    Ok(())
}

// ─── 原子写与备份（Gateway 独立） ─────────────────────────────────────

fn write_models_atomic(models: &serde_json::Value) -> Result<()> {
    let models_path = gateway_models_path();
    // 独立 tmp 命名，避免与 providers 的 config.json.tmp 冲突
    let tmp = config::config_dir().join(format!("models.json.tmp-{}", std::process::id()));
    let json = serde_json::to_string_pretty(models).map_err(|e| AppError::json(&tmp, e))?;
    // 确保父目录存在
    if let Some(parent) = models_path.parent() {
        std::fs::create_dir_all(parent).map_err(|e| AppError::io(parent, e))?;
    }
    std::fs::write(&tmp, json + "\n").map_err(|e| AppError::io(&tmp, e))?;
    std::fs::rename(&tmp, &models_path).map_err(|e| AppError::io(&models_path, e))?;
    // 通知独立进程（文件通知，错误隔离）
    let _ = notify_gateway_changed();
    Ok(())
}

fn backup_models() -> Option<PathBuf> {
    let models_path = gateway_models_path();
    if !models_path.exists() {
        return None;
    }
    let ts = chrono::Utc::now().format("%Y-%m-%dT%H-%M-%S-%3fZ");
    let backup_path = config::backup_dir().join(format!("models-{}.json", ts));
    std::fs::create_dir_all(config::backup_dir()).ok();
    std::fs::copy(&models_path, &backup_path).ok()?;
    Some(backup_path)
}

fn load_models_value() -> Result<serde_json::Value> {
    let models_path = gateway_models_path();
    if models_path.exists() {
        let text =
            std::fs::read_to_string(&models_path).map_err(|e| AppError::io(&models_path, e))?;
        Ok(serde_json::from_str::<serde_json::Value>(&text)
            .unwrap_or(serde_json::json!({ "providers": {} })))
    } else {
        Ok(serde_json::json!({ "providers": {} }))
    }
}

// ─── Gateway 预览/合并/冲突 ───────────────────────────────────────────

#[derive(Debug, Clone, serde::Serialize)]
pub struct GatewayPreview {
    pub current: Option<serde_json::Value>,
    pub proposed: serde_json::Value,
    pub conflicts: Vec<String>,
    pub pending_count: usize,
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
    let host = config::normalize_gateway_host(&config.settings.proxy.host);
    let port = config.settings.proxy.port;
    serde_json::json!({
        "api": config.settings.gateway_api.clone(),
        "baseUrl": format!("http://{}:{}/v1", host, port),
        "apiKey": "pi-switch-proxy",
        "models": gateway_models,
        "proxy": false,
    })
}

fn merge_gateway_extra(current: &serde_json::Value, proposed: &mut serde_json::Value) {
    if let (Some(cur_obj), Some(prop_obj)) = (current.as_object(), proposed.as_object_mut()) {
        for (k, v) in cur_obj {
            if !prop_obj.contains_key(k) {
                prop_obj.insert(k.clone(), v.clone());
            }
        }
        if let (Some(cur_models), Some(prop_models)) = (
            cur_obj.get("models").and_then(|v| v.as_array()),
            prop_obj.get_mut("models").and_then(|v| v.as_array_mut()),
        ) {
            let cur_by_id: std::collections::HashMap<String, &serde_json::Value> = cur_models
                .iter()
                .filter_map(|m| {
                    m.get("id")
                        .and_then(|id| id.as_str())
                        .map(|id| (id.to_string(), m))
                })
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

fn normalize_base_url_for_compare(url: &str) -> String {
    // 归一化监听地址：0.0.0.0/[::] -> 127.0.0.1，保证 preview 不因监听地址误报冲突
    url.replace("://0.0.0.0:", "://127.0.0.1:")
        .replace("://[::]:", "://127.0.0.1:")
}

fn compute_pending_count(
    current: Option<&serde_json::Value>,
    proposed: &serde_json::Value,
) -> usize {
    let Some(cur) = current else {
        return proposed.as_object().map(|o| o.len()).unwrap_or(0);
    };
    let Some(cur_obj) = cur.as_object() else {
        return proposed.as_object().map(|o| o.len()).unwrap_or(0);
    };
    let Some(prop_obj) = proposed.as_object() else {
        return cur_obj.len();
    };
    let added = prop_obj
        .keys()
        .filter(|k| !cur_obj.contains_key(*k))
        .count();
    let removed = cur_obj
        .keys()
        .filter(|k| !prop_obj.contains_key(*k))
        .count();
    let changed = cur_obj
        .keys()
        .filter(|k| prop_obj.contains_key(*k) && cur_obj.get(*k) != prop_obj.get(*k))
        .count();
    added + removed + changed
}

fn compute_gateway_conflicts(
    current: &serde_json::Value,
    proposed: &serde_json::Value,
) -> Vec<String> {
    let mut conflicts = Vec::new();
    let generated_keys = ["api", "baseUrl", "apiKey", "models", "proxy"];
    if let (Some(cur_obj), Some(prop_obj)) = (current.as_object(), proposed.as_object()) {
        for key in generated_keys {
            if let (Some(cur_val), Some(prop_val)) = (cur_obj.get(key), prop_obj.get(key)) {
                let is_equal = if key == "baseUrl" {
                    let cur_s = cur_val.as_str().unwrap_or("");
                    let prop_s = prop_val.as_str().unwrap_or("");
                    normalize_base_url_for_compare(cur_s) == normalize_base_url_for_compare(prop_s)
                } else {
                    cur_val == prop_val
                };
                if !is_equal {
                    conflicts.push(key.to_string());
                }
            }
        }
    }
    conflicts
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
    let models = obj
        .get("models")
        .and_then(|v| v.as_array())
        .ok_or("gateway.models must be an array")?;
    for (i, m) in models.iter().enumerate() {
        let mid = m.get("id").and_then(|v| v.as_str()).unwrap_or("");
        if mid.trim().is_empty() {
            return Err(format!("gateway.models[{i}].id must not be empty"));
        }
    }
    Ok(())
}

// ─── 供 ops / service / web 调用的独立接口（错误边界隔离） ─────────────

/// 读取当前 gateway（仅读 models.json，不触碰 providers）
pub fn get_gateway() -> Result<Option<serde_json::Value>> {
    let config = config::load_config()?;
    let models = load_models_value()?;
    let gateway_id = config.settings.provider_prefix.clone();
    let entry = models
        .get("providers")
        .and_then(|p| p.get(&gateway_id))
        .cloned();
    Ok(entry)
}

/// 预览 gateway（干跑，不写盘）
pub fn preview_gateway() -> Result<GatewayPreview> {
    let config = config::load_config()?;
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
    let pending_count = compute_pending_count(current.as_ref(), &proposed);

    Ok(GatewayPreview {
        current,
        proposed,
        conflicts,
        pending_count,
    })
}

/// 显式应用 gateway（PUT /models/gateway），独立原子写，隔离错误
pub fn apply_gateway(edited: serde_json::Value) -> Result<()> {
    validate_gateway_value(&edited).map_err(AppError::Message)?;
    let config = config::load_config()?;
    let mut models = load_models_value()?;
    let providers = models["providers"]
        .as_object_mut()
        .ok_or_else(|| AppError::Message("invalid models.json".into()))?;
    let gateway_id = config.settings.provider_prefix.clone();
    let _ = backup_models();
    providers.insert(gateway_id, edited);
    write_models_atomic(&models)
}

/// 将当前 profiles/settings 合并为 gateway 并写入 models.json（供启动/设置变更时调用）
/// 保留供手动调用与未来独立进程 IPC 替换 `gateway.notify` 后使用
#[allow(dead_code)]
pub fn sync_gateway_to_pi() -> Result<()> {
    let config = config::load_config()?;
    let mut models = load_models_value()?;
    sync_gateway_with_current(&config, &mut models)?;
    write_models_atomic(&models)
}

#[allow(dead_code)]
fn sync_gateway_with_current(
    config: &config::PiSwitchConfig,
    models: &mut serde_json::Value,
) -> Result<()> {
    let gateway_id = config.settings.provider_prefix.clone();
    let current = models
        .get("providers")
        .and_then(|p| p.get(&gateway_id))
        .cloned();
    let mut proposed = build_proposed_gateway_entry(config);
    if let Some(ref cur) = current {
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

// ─── 进程生命周期占位（独立 gateway 进程，暂逻辑隔离） ─────────────

/// 独立 gateway 进程健康状态（占位：当前基于文件/内存合成，未真拆进程）
#[derive(Debug, Clone, serde::Serialize)]
pub struct GatewayHealth {
    pub running: bool,
    pub mode: String,
    pub gateway_id: String,
    pub has_models_file: bool,
    pub last_notify: Option<String>,
    pub upstreams_total: usize,
    pub message: String,
}

/// 健康检查占位：检查 models.json 与 notify 文件，返回合成状态
/// 后续真拆进程后，此函数将改为通过 IPC 向独立进程询问健康度
pub fn health_check() -> Result<GatewayHealth> {
    let config = config::load_config().unwrap_or_default();
    let models_path = gateway_models_path();
    let has_models_file = models_path.exists();
    let last_notify = std::fs::read_to_string(gateway_notify_path())
        .ok()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty());
    // 统计生效上游总数（用于后续多上游独立调度健康度）
    let mut upstreams_total = 0usize;
    for (_name, pv) in &config.profiles {
        if let Ok(profile) = serde_json::from_value::<ProviderProfile>(pv.clone()) {
            upstreams_total += profile.resolved_upstreams().len();
        }
    }
    Ok(GatewayHealth {
        running: true,
        mode: "logical-isolation".into(),
        gateway_id: config.settings.provider_prefix.clone(),
        has_models_file,
        last_notify,
        upstreams_total,
        message: "gateway logical isolation active; process placeholder (see gateway.rs docs for true split path)".into(),
    })
}

/// 启动占位：未来独立进程的启动入口（当前仅确保目录与 notify 文件，返回健康状态）
pub fn start_placeholder() -> Result<GatewayHealth> {
    let _ = std::fs::create_dir_all(config::config_dir());
    let _ = notify_gateway_changed();
    health_check()
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn gateway_validate_rejects_bad_api() {
        let v = json!({"api":"bad","baseUrl":"http://x/v1","models":[]});
        assert!(validate_gateway_value(&v).is_err());
    }

    #[test]
    fn gateway_notify_is_isolated_error() {
        // notify 不应因文件系统错误而阻断
        let _ = notify_gateway_changed();
    }

    #[test]
    fn gateway_preview_is_deterministic_and_dry_run() {
        // preview 为干跑：两次调用应返回一致的 proposed（不写盘）
        let a = preview_gateway();
        let b = preview_gateway();
        assert!(a.is_ok() && b.is_ok());
        let a = a.unwrap();
        let b = b.unwrap();
        assert_eq!(a.proposed, b.proposed);
    }

    #[test]
    fn gateway_apply_rejects_invalid_base_url() {
        let v = json!({"api":"openai-completions","baseUrl":"ftp://bad","models":[]});
        assert!(validate_gateway_value(&v).is_err());
    }

    #[test]
    fn gateway_health_returns_running_and_mode() {
        let h = health_check().expect("health should succeed");
        assert!(h.running);
        assert!(!h.gateway_id.is_empty());
        assert_eq!(h.mode, "logical-isolation");
    }

    #[test]
    fn gateway_merge_extra_preserves_unknown_fields() {
        let cur = json!({"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","models":[{"id":"p/m1","custom":"keep"}],"proxy":false,"extraKept":1});
        let mut prop = json!({"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","models":[{"id":"p/m1","input":["text"]}],"proxy":false});
        merge_gateway_extra(&cur, &mut prop);
        assert_eq!(prop["extraKept"], 1);
        assert_eq!(prop["models"][0]["custom"], "keep");
    }

    #[test]
    fn gateway_preview_pending_count_reflects_diff() {
        // pending_count 应该按新增/移除/变更计算（顶层 keys）
        let cur =
            json!({"api":"openai-completions","baseUrl":"http://a/v1","models":[],"proxy":false});
        let prop = json!({"api":"openai-completions","baseUrl":"http://b/v1","models":[],"proxy":false,"extra":1});
        // added=1 (extra), changed=1 (baseUrl) => pending 2
        assert_eq!(compute_pending_count(Some(&cur), &prop), 2);
        // missing current => pending = keys in proposed
        assert_eq!(compute_pending_count(None, &prop), 5);
        // identical => 0
        assert_eq!(compute_pending_count(Some(&cur), &cur), 0);
    }

    #[test]
    fn gateway_preview_returns_pending_count() {
        let preview = preview_gateway().expect("preview should succeed");
        // pending_count should equal diff of current vs proposed
        let expected = compute_pending_count(preview.current.as_ref(), &preview.proposed);
        assert_eq!(
            preview.pending_count, expected,
            "pending_count must reflect added/removed/changed"
        );
    }

    #[test]
    fn gateway_health_returns_full_shape() {
        let h = health_check().expect("health should succeed");
        assert!(h.running);
        assert_eq!(h.mode, "logical-isolation");
        assert!(!h.gateway_id.is_empty());
        assert!(!h.message.is_empty());
        // has_models_file is bool, last_notify is Option, upstreams_total is count
        let _ = h.has_models_file;
        let _ = h.last_notify.clone();
        let _ = h.upstreams_total;
    }

    #[test]
    fn gateway_start_placeholder_does_not_auto_write_models() {
        use std::fs;
        let path = crate::config::models_path();
        let before = fs::read_to_string(&path).unwrap_or_default();
        let h = start_placeholder().expect("start_placeholder should succeed");
        assert!(h.running);
        let after = fs::read_to_string(&path).unwrap_or_default();
        // start_placeholder should not modify models.json content (only touch notify)
        assert_eq!(
            before, after,
            "start_placeholder must not auto-write gateway"
        );
    }

    #[test]
    fn gateway_health_and_preview_available_when_models_missing() {
        // Even if models file is corrupted or missing, health and preview should still be Ok
        let h = health_check();
        assert!(
            h.is_ok(),
            "health should be ok even when gateway file missing/corrupted"
        );
        let p = preview_gateway();
        assert!(
            p.is_ok(),
            "preview should be ok even when gateway file missing"
        );
    }

    #[test]
    fn upstream_fallback_single_to_resolved() {
        // 单字段回退：未配置 upstreams 时 resolved_upstreams 应回退到 baseUrl/apiKey
        let mut profile = crate::config::ProviderProfile {
            base_url: "https://api.example.com/v1".into(),
            api_key: "sk-test".into(),
            ..Default::default()
        };
        assert!(!profile.has_upstreams());
        let resolved = profile.resolved_upstreams();
        assert_eq!(resolved.len(), 1);
        assert_eq!(resolved[0].base_url, "https://api.example.com/v1");
        // 配置多上游后，has_upstreams true 且 resolved 直接返回 upstreams
        profile.upstreams = vec![
            crate::config::Upstream {
                base_url: "http://a/v1".into(),
                api_key: "k1".into(),
                weight: Some(2),
                name: Some("a".into()),
                ..Default::default()
            },
            crate::config::Upstream {
                base_url: "http://b/v1".into(),
                api_key: "k2".into(),
                ..Default::default()
            },
        ];
        assert!(profile.has_upstreams());
        let resolved2 = profile.resolved_upstreams();
        assert_eq!(resolved2.len(), 2);
        assert_eq!(resolved2[0].weight, Some(2));
        assert_eq!(resolved2[1].base_url, "http://b/v1");
        // 单字段 baseUrl 在多上游下应被 primary_* 覆盖
        assert_eq!(profile.primary_base_url(), "http://a/v1");
        assert_eq!(profile.primary_api_key(), "k1");
    }

    #[test]
    fn gateway_merge_extra_does_not_elevate_generated_keys() {
        let cur = json!({"api":"openai-completions","baseUrl":"http://old/v1","models":[],"proxy":false,"extraKept":1});
        let mut prop = json!({"api":"openai-responses","baseUrl":"http://127.0.0.1:43112/v1","models":[],"proxy":false});
        merge_gateway_extra(&cur, &mut prop);
        assert_eq!(
            prop["api"], "openai-responses",
            "proposed api must not be overwritten by current"
        );
        assert_eq!(prop["baseUrl"], "http://127.0.0.1:43112/v1");
        assert_eq!(prop["extraKept"], 1);
        assert_eq!(prop["proxy"], false);
    }

    #[test]
    fn gateway_apply_is_atomic_and_notifies() {
        use std::fs;
        let path = crate::config::models_path();
        let notify_path = crate::config::config_dir().join("gateway.notify");
        let _before_content = fs::read_to_string(&path).unwrap_or_default();
        let preview = preview_gateway().expect("preview should succeed");
        let edited = preview.proposed.clone();
        let res = apply_gateway(edited.clone());
        assert!(
            res.is_ok(),
            "apply with valid proposed should succeed: {:?}",
            res
        );
        let after_content = fs::read_to_string(&path).unwrap_or_default();
        assert!(
            !after_content.is_empty(),
            "models.json should not be empty after apply"
        );
        let after_notify = fs::read_to_string(&notify_path).unwrap_or_default();
        assert!(
            !after_notify.is_empty(),
            "notify file should exist after apply"
        );
        let preview2 = preview_gateway().expect("preview after apply should succeed");
        assert_eq!(preview2.pending_count, 0, "after apply pending should be 0");
    }
}
