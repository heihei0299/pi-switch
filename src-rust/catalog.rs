use crate::config;
use crate::error::{AppError, Result};
use serde_json::Value;
use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime};

pub const CATALOG_URL: &str = "https://models.dev/api.json";
pub const CATALOG_TTL_SECS: u64 = 24 * 3600;

/// 将 preset 推断为模型目录的 provider key（复用 config 的映射，推断失败返回 None，跳过模型元数据 enrich）
pub fn infer_catalog_provider_key(preset: &str) -> Option<&'static str> {
    config::preset_to_models_dev_key(preset)
}

/// 解析 profile 对应的模型目录 provider key（模型元数据 enrich 用）。
/// 优先级：显式 modelsDevProvider > preset 推断；均无或推断失败则返回 None（跳过 enrich，不报错）
pub fn resolve_catalog_provider(profile: &config::ProviderProfile) -> Option<String> {
    config::resolve_models_dev_provider(profile)
}

/// 模型目录缓存路径：`~/.pi-switch/cache/models-dev.json`
pub fn catalog_cache_path() -> PathBuf {
    config::config_dir().join("cache").join("models-dev.json")
}

/// 判断缓存是否在 TTL 内（24h）
pub fn is_cache_fresh(path: &Path) -> bool {
    let Ok(meta) = std::fs::metadata(path) else {
        return false;
    };
    let Ok(modified) = meta.modified() else {
        return false;
    };
    let Ok(elapsed) = SystemTime::now().duration_since(modified) else {
        return false;
    };
    elapsed.as_secs() < CATALOG_TTL_SECS
}

/// 从缓存读取目录（不存在返回 None）
pub fn load_catalog_from_cache(path: &Path) -> Result<Option<Value>> {
    if !path.exists() {
        return Ok(None);
    }
    let text = std::fs::read_to_string(path).map_err(|e| AppError::io(path, e))?;
    let value: Value = serde_json::from_str(&text).map_err(|e| AppError::json(path, e))?;
    Ok(Some(value))
}

/// 原子写入缓存（创建父目录）
fn write_catalog_atomic(path: &Path, value: &Value) -> Result<()> {
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent).map_err(|e| AppError::io(parent, e))?;
    }
    let tmp = path.with_extension(format!("tmp-{}", std::process::id()));
    let json = serde_json::to_string(value).map_err(|e| AppError::json(path, e))?;
    std::fs::write(&tmp, json + "\n").map_err(|e| AppError::io(&tmp, e))?;
    std::fs::rename(&tmp, path).map_err(|e| AppError::io(path, e))?;
    Ok(())
}

/// 从网络拉取目录并原子写入缓存
pub async fn fetch_catalog_and_cache(path: &Path) -> Result<Value> {
    let value = fetch_catalog_from_network().await?;
    write_catalog_atomic(path, &value)?;
    Ok(value)
}

async fn fetch_catalog_from_network() -> Result<Value> {
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(10))
        .build()
        .map_err(|e| AppError::Message(format!("HTTP client error: {}", e)))?;
    let resp = client
        .get(CATALOG_URL)
        .send()
        .await
        .map_err(|e| AppError::Message(format!("模型目录拉取失败: {}", e)))?;
    let status = resp.status();
    if !status.is_success() {
        return Err(AppError::Message(format!(
            "模型目录拉取失败: HTTP {}",
            status.as_u16()
        )));
    }
    let value: Value = resp
        .json()
        .await
        .map_err(|e| AppError::Message(format!("模型目录解析失败: {}", e)))?;
    Ok(value)
}

/// 对外主入口：TTL 内直接读缓存，过期或无缓存时尝试网络，失败回退到过期缓存
pub async fn get_or_refresh_catalog() -> Result<Option<Value>> {
    let path = catalog_cache_path();
    let (value, _) = get_or_refresh_catalog_inner_with_warning(&path, fetch_catalog_from_network).await?;
    Ok(value)
}

/// 对外主入口（带 warning）：返回 (catalog, warning) 用于可观测性
pub async fn get_or_refresh_catalog_with_warning() -> Result<(Option<Value>, Option<String>)> {
    let path = catalog_cache_path();
    get_or_refresh_catalog_inner_with_warning(&path, fetch_catalog_from_network).await
}

/// 可注入 fetcher 的内部实现（测试用）
pub async fn get_or_refresh_catalog_inner<F, Fut>(path: &Path, fetcher: F) -> Result<Option<Value>>
where
    F: FnOnce() -> Fut,
    Fut: std::future::Future<Output = Result<Value>>,
{
    let (value, _) = get_or_refresh_catalog_inner_with_warning(path, fetcher).await?;
    Ok(value)
}

/// 可注入 fetcher 的内部实现（带 warning，测试与可观测性用）
pub async fn get_or_refresh_catalog_inner_with_warning<F, Fut>(path: &Path, fetcher: F) -> Result<(Option<Value>, Option<String>)>
where
    F: FnOnce() -> Fut,
    Fut: std::future::Future<Output = Result<Value>>,
{
    // 新鲜缓存直接返回
    if is_cache_fresh(path) {
        if let Some(cached) = load_catalog_from_cache(path)? {
            return Ok((Some(cached), None));
        }
    }

    // 缓存过期或不存在，尝试网络
    match fetcher().await {
        Ok(value) => {
            write_catalog_atomic(path, &value)?;
            Ok((Some(value), None))
        }
        Err(err) => {
            // 回退到过期缓存（若有）
            if path.exists() {
                if let Some(cached) = load_catalog_from_cache(path)? {
                    let warning = format!("模型目录拉取失败，回退到过期缓存: {}", err);
                    log::warn!("{}", warning);
                    return Ok((Some(cached), Some(warning)));
                }
            }
            let warning = format!("模型目录拉取失败且无可用缓存: {}", err);
            log::warn!("{}", warning);
            Ok((None, Some(warning)))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    use std::fs;
    use std::time::{Duration, SystemTime};

    fn temp_path(name: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("pi-switch-catalog-test-{}-{}", name, std::process::id()));
        let _ = fs::create_dir_all(&dir);
        dir.join("models-dev.json")
    }

    fn write_fixture(path: &Path, value: &Value) {
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent).unwrap();
        }
        fs::write(path, serde_json::to_string(value).unwrap() + "\n").unwrap();
    }

    fn set_mtime(path: &Path, ago: Duration) {
        let t = SystemTime::now() - ago;
        let ft = filetime::FileTime::from_system_time(t);
        filetime::set_file_mtime(path, ft).unwrap();
    }

    #[test]
    fn catalog_cache_path_is_under_config_dir() {
        let p = catalog_cache_path();
        assert!(p.ends_with("cache/models-dev.json"));
        assert!(p.to_string_lossy().contains(".pi-switch"));
    }

    #[test]
    fn is_cache_fresh_true_when_recent() {
        let path = temp_path("fresh");
        let _ = fs::remove_file(&path);
        write_fixture(&path, &json!({"a":1}));
        // mtime = now, should be fresh
        assert!(is_cache_fresh(&path));
        let _ = fs::remove_file(&path);
    }

    #[test]
    fn is_cache_fresh_false_when_stale() {
        let path = temp_path("stale");
        let _ = fs::remove_file(&path);
        write_fixture(&path, &json!({"a":1}));
        set_mtime(&path, Duration::from_secs(CATALOG_TTL_SECS + 10));
        assert!(!is_cache_fresh(&path));
        let _ = fs::remove_file(&path);
    }

    #[test]
    fn is_cache_fresh_false_when_missing() {
        let path = temp_path("missing");
        let _ = fs::remove_file(&path);
        assert!(!is_cache_fresh(&path));
    }

    #[test]
    fn load_catalog_from_cache_parses_fixture() {
        let path = temp_path("load");
        let _ = fs::remove_file(&path);
        let fixture = json!({"openai":{"id":"openai","models":{"gpt-4o":{"id":"gpt-4o","limit":{"context":128000,"output":16384}}}}});
        write_fixture(&path, &fixture);
        let loaded = load_catalog_from_cache(&path).unwrap().unwrap();
        assert_eq!(loaded["openai"]["id"], "openai");
        let _ = fs::remove_file(&path);
    }

    #[test]
    fn load_catalog_from_cache_none_when_missing() {
        let path = temp_path("load_missing");
        let _ = fs::remove_file(&path);
        assert!(load_catalog_from_cache(&path).unwrap().is_none());
    }

    #[tokio::test]
    async fn get_or_refresh_returns_fresh_cache_without_fetching() {
        let path = temp_path("fresh_no_fetch");
        let _ = fs::remove_file(&path);
        let fixture = json!({"openai":{"id":"openai"}});
        write_fixture(&path, &fixture);
        // fresh cache, fetcher would panic if called — proves no network request
        let result = get_or_refresh_catalog_inner(&path, || async {
            panic!("should not fetch when cache is fresh");
            #[allow(unreachable_code)]
            Ok::<Value, AppError>(json!({}))
        })
        .await
        .unwrap()
        .unwrap();
        assert_eq!(result["openai"]["id"], "openai");
        let _ = fs::remove_file(&path);
    }

    #[tokio::test]
    async fn get_or_refresh_fetches_when_stale_and_updates_cache() {
        let path = temp_path("stale_fetch");
        let _ = fs::remove_file(&path);
        let old = json!({"old":1});
        write_fixture(&path, &old);
        set_mtime(&path, Duration::from_secs(CATALOG_TTL_SECS + 10));
        let fresh = json!({"openai":{"id":"openai","models":{"gpt-4o":{"id":"gpt-4o"}}}});
        let fresh_clone = fresh.clone();
        let result = get_or_refresh_catalog_inner(&path, move || {
            let v = fresh_clone.clone();
            async move { Ok::<Value, AppError>(v) }
        })
        .await
        .unwrap()
        .unwrap();
        assert_eq!(result["openai"]["id"], "openai");
        // cache should be updated
        let cached = load_catalog_from_cache(&path).unwrap().unwrap();
        assert_eq!(cached["openai"]["id"], "openai");
        let _ = fs::remove_file(&path);
    }

    #[tokio::test]
    async fn get_or_refresh_fetches_when_no_cache() {
        let path = temp_path("no_cache_fetch");
        let _ = fs::remove_file(&path);
        let fresh = json!({"anthropic":{"id":"anthropic"}});
        let fresh_clone = fresh.clone();
        let result = get_or_refresh_catalog_inner(&path, move || {
            let v = fresh_clone.clone();
            async move { Ok::<Value, AppError>(v) }
        })
        .await
        .unwrap()
        .unwrap();
        assert_eq!(result["anthropic"]["id"], "anthropic");
        assert!(path.exists());
        let _ = fs::remove_file(&path);
    }

    #[tokio::test]
    async fn get_or_refresh_falls_back_to_stale_cache_on_fetch_failure() {
        let path = temp_path("fallback");
        let _ = fs::remove_file(&path);
        let stale = json!({"deepseek":{"id":"deepseek"}});
        write_fixture(&path, &stale);
        set_mtime(&path, Duration::from_secs(CATALOG_TTL_SECS + 10));
        let result = get_or_refresh_catalog_inner(&path, || async {
            Err::<Value, AppError>(AppError::Message("network down".into()))
        })
        .await
        .unwrap()
        .unwrap();
        assert_eq!(result["deepseek"]["id"], "deepseek");
        let _ = fs::remove_file(&path);
    }

    #[tokio::test]
    async fn get_or_refresh_returns_none_when_no_cache_and_fetch_fails() {
        let path = temp_path("no_cache_fail");
        let _ = fs::remove_file(&path);
        let result = get_or_refresh_catalog_inner(&path, || async {
            Err::<Value, AppError>(AppError::Message("network down".into()))
        })
        .await
        .unwrap();
        assert!(result.is_none());
    }

    #[tokio::test]
    async fn write_catalog_atomic_creates_parent_dirs() {
        let path = temp_path("atomic_dirs");
        let _ = fs::remove_file(&path);
        // ensure parent not exists
        if let Some(parent) = path.parent() {
            let _ = fs::remove_dir_all(parent);
        }
        let value = json!({"a":1});
        let v = value.clone();
        let result = get_or_refresh_catalog_inner(&path, move || {
            let v = v.clone();
            async move { Ok::<Value, AppError>(v) }
        })
        .await
        .unwrap()
        .unwrap();
        assert_eq!(result["a"], 1);
        assert!(path.exists());
        let _ = fs::remove_file(&path);
    }

    #[test]
    fn resolve_catalog_provider_prefers_explicit_over_preset() {
        let mut profile = config::ProviderProfile {
            preset: Some("anthropic".into()),
            models_dev_provider: Some("openai".into()),
            ..Default::default()
        };
        assert_eq!(
            resolve_catalog_provider(&profile).as_deref(),
            Some("openai")
        );
        profile.models_dev_provider = None;
        assert_eq!(
            resolve_catalog_provider(&profile).as_deref(),
            Some("anthropic")
        );
    }

    #[test]
    fn infer_catalog_provider_key_maps_known_presets() {
        assert_eq!(infer_catalog_provider_key("openai"), Some("openai"));
        assert_eq!(infer_catalog_provider_key("unknown"), None);
    }

    #[test]
    fn resolve_catalog_provider_none_when_no_mapping() {
        let profile = config::ProviderProfile {
            preset: Some("custom".into()),
            ..Default::default()
        };
        assert!(resolve_catalog_provider(&profile).is_none());
    }

    #[tokio::test]
    async fn get_or_refresh_with_warning_fresh_cache_no_warning() {
        let path = temp_path("warn_fresh");
        let _ = fs::remove_file(&path);
        let fixture = json!({"openai":{"id":"openai"}});
        write_fixture(&path, &fixture);
        // fresh cache should return cached without warning and without calling fetcher
        let (value, warning) = get_or_refresh_catalog_inner_with_warning(&path, || async {
            panic!("should not fetch when fresh");
            #[allow(unreachable_code)]
            Ok::<Value, AppError>(json!({}))
        })
        .await
        .unwrap();
        assert!(value.is_some());
        assert!(warning.is_none(), "24h 内命中缓存不应有 warning，无重复网络请求");
        assert_eq!(value.unwrap()["openai"]["id"], "openai");
        let _ = fs::remove_file(&path);
    }

    #[tokio::test]
    async fn get_or_refresh_with_warning_falls_back_with_warning() {
        let path = temp_path("warn_fallback");
        let _ = fs::remove_file(&path);
        let stale = json!({"deepseek":{"id":"deepseek"}});
        write_fixture(&path, &stale);
        set_mtime(&path, Duration::from_secs(CATALOG_TTL_SECS + 10));
        let (value, warning) = get_or_refresh_catalog_inner_with_warning(&path, || async {
            Err::<Value, AppError>(AppError::Message("network down".into()))
        })
        .await
        .unwrap();
        assert!(value.is_some());
        let w = warning.expect("过期或网络失败时应回退并报告 warning");
        assert!(w.contains("模型目录"), "warning 文案需使用模型目录术语");
        assert!(w.contains("回退到过期缓存"), "warning 需说明回退");
        assert_eq!(value.unwrap()["deepseek"]["id"], "deepseek");
        let _ = fs::remove_file(&path);
    }

    #[tokio::test]
    async fn get_or_refresh_with_warning_no_cache_and_fail_returns_warning() {
        let path = temp_path("warn_no_cache");
        let _ = fs::remove_file(&path);
        let (value, warning) = get_or_refresh_catalog_inner_with_warning(&path, || async {
            Err::<Value, AppError>(AppError::Message("network down".into()))
        })
        .await
        .unwrap();
        assert!(value.is_none(), "无缓存且失败时应返回 None，跳过 enrich");
        let w = warning.expect("应有 warning 提示失败原因");
        assert!(w.contains("模型目录"), "文案需使用模型目录");
        assert!(w.contains("无可用缓存"));
    }

    #[tokio::test]
    async fn get_or_refresh_with_warning_offline_has_cache_still_enriches() {
        // 24h 内缓存即使离线（fetcher 失败）也应返回缓存且无 warning（因为命中 fresh）
        let path = temp_path("warn_offline_fresh");
        let _ = fs::remove_file(&path);
        let fixture = json!({"openai":{"id":"openai","models":{"gpt-4o":{"id":"gpt-4o"}}}});
        write_fixture(&path, &fixture);
        // 不设过期，fresh
        let (value, warning) = get_or_refresh_catalog_inner_with_warning(&path, || async {
            Err::<Value, AppError>(AppError::Message("offline".into()))
        })
        .await
        .unwrap();
        assert!(value.is_some());
        assert!(warning.is_none(), "有 24h 内缓存时离线仍能完成 enrich，不应 warning");
        let _ = fs::remove_file(&path);
    }

    #[tokio::test]
    async fn get_or_refresh_with_warning_offline_stale_returns_stale_with_warning() {
        // 过期缓存 + 离线失败应回退到过期缓存并 warning
        let path = temp_path("warn_offline_stale");
        let _ = fs::remove_file(&path);
        let stale = json!({"openai":{"id":"openai"}});
        write_fixture(&path, &stale);
        set_mtime(&path, Duration::from_secs(CATALOG_TTL_SECS + 100));
        let (value, warning) = get_or_refresh_catalog_inner_with_warning(&path, || async {
            Err::<Value, AppError>(AppError::Message("offline".into()))
        })
        .await
        .unwrap();
        assert!(value.is_some());
        assert!(warning.is_some());
        assert!(warning.unwrap().contains("回退到过期缓存"));
        let _ = fs::remove_file(&path);
    }
}
