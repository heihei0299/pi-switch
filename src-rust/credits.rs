//! 供应商余量代理与归一化抽象
//! - `CreditsFetcher` trait + 注册表，按供应商特征路由到具体实现
//! - `OpencodeGoFetcher`：仅查询主上游 `/v1/usage`（归一化 baseUrl 剥离尾部 `/v1`），超时 5s，归一化为前端固定结构（含 Go 三窗口用量）
//! - 预留 `CodexFetcher`：后续仅新增文件与注册一行，前端零改动

use crate::config::{self, ProviderProfile, Upstream};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::time::Duration;

// ─── 归一化结构 ─────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UsageWindow {
    pub percent: f64,
    pub status: String,
    #[serde(skip_serializing_if = "Option::is_none", rename = "resetsAt")]
    pub resets_at: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GoUsage {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rolling: Option<UsageWindow>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub weekly: Option<UsageWindow>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub monthly: Option<UsageWindow>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NormalizedCredits {
    pub balance: f64,
    pub used: f64,
    pub total: f64,
    pub remaining: f64,
    pub percent: f64,
    #[serde(skip_serializing_if = "Option::is_none", rename = "resetAt")]
    pub reset_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expiry: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub usage: Option<GoUsage>,
    pub raw: Value,
}
// ─── 错误 ───────────────────────────────────────────────────────

#[derive(Debug, Clone)]
pub enum CreditsError {
    NotFound(String),
    Unsupported(String),
    Timeout(String),
    Upstream { status: u16, message: String },
    Network(String),
    Parse(String),
}

impl std::fmt::Display for CreditsError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::NotFound(s) => write!(f, "{}", s),
            Self::Unsupported(s) => write!(f, "{}", s),
            Self::Timeout(s) => write!(f, "timeout: {}", s),
            Self::Upstream { status, message } => write!(f, "upstream {}: {}", status, message),
            Self::Network(s) => write!(f, "network: {}", s),
            Self::Parse(s) => write!(f, "parse: {}", s),
        }
    }
}
impl std::error::Error for CreditsError {}

// ─── 工具：字段抽取 ─────────────────────────────────────────────

fn get_f64(raw: &Value, keys: &[&str]) -> Option<f64> {
    for k in keys {
        if let Some(v) = raw.get(k) {
            if let Some(n) = v.as_f64() {
                return Some(n);
            }
            if let Some(n) = v.as_i64() {
                return Some(n as f64);
            }
            if let Some(n) = v.as_u64() {
                return Some(n as f64);
            }
            if let Some(s) = v.as_str() {
                if let Ok(n) = s.parse::<f64>() {
                    return Some(n);
                }
            }
        }
    }
    None
}

fn get_string(raw: &Value, keys: &[&str]) -> Option<String> {
    for k in keys {
        if let Some(v) = raw.get(k) {
            if let Some(s) = v.as_str() {
                if !s.trim().is_empty() {
                    return Some(s.to_string());
                }
            }
            // number as string?
            if let Some(n) = v.as_i64() {
                return Some(n.to_string());
            }
            if let Some(n) = v.as_f64() {
                return Some(n.to_string());
            }
        }
    }
    None
}

// ─── 归一化：opencode-go ───────────────────────────────────────

pub fn normalize_opencode_go(raw: Value) -> NormalizedCredits {
    // 优先处理 Go usage 形态：{ usage: { rolling, weekly, monthly } }
    if let Some(usage_val) = raw.get("usage").and_then(|v| v.as_object()) {
        let parse_window = |key: &str| -> Option<UsageWindow> {
            let w = usage_val.get(key)?.as_object()?;
            let percent = w.get("percent").and_then(|v| {
                if let Some(n) = v.as_f64() {
                    Some(n)
                } else if let Some(n) = v.as_i64() {
                    Some(n as f64)
                } else if let Some(s) = v.as_str() {
                    s.parse::<f64>().ok()
                } else {
                    None
                }
            })?;
            if !percent.is_finite() || !(0.0..=100.0).contains(&percent) {
                return None;
            }
            let status = w
                .get("status")
                .and_then(|v| v.as_str())
                .map(|s| s.to_string())
                .unwrap_or_else(|| "ok".to_string());
            let resets_at = w
                .get("resetsAt")
                .or_else(|| w.get("resets_at"))
                .or_else(|| w.get("resetAt"))
                .and_then(|v| v.as_str())
                .map(|s| s.to_string());
            Some(UsageWindow {
                percent: percent.clamp(0.0, 100.0),
                status,
                resets_at,
            })
        };
        let rolling = parse_window("rolling");
        let weekly = parse_window("weekly");
        let monthly = parse_window("monthly");
        if rolling.is_some() || weekly.is_some() || monthly.is_some() {
            let primary = rolling.as_ref().or(weekly.as_ref()).or(monthly.as_ref());
            let percent = primary.map(|w| w.percent).unwrap_or(0.0);
            let reset_at = primary.and_then(|w| w.resets_at.clone());
            let remaining = (100.0 - percent).max(0.0);
            return NormalizedCredits {
                balance: remaining,
                used: percent,
                total: 100.0,
                remaining,
                percent,
                reset_at: reset_at.clone(),
                expiry: reset_at,
                usage: Some(GoUsage {
                    rolling,
                    weekly,
                    monthly,
                }),
                raw: raw.clone(),
            };
        }
    }

    let balance_opt = get_f64(
        &raw,
        &[
            "balance",
            "remaining",
            "credits",
            "available",
            "creditBalance",
            "credit_balance",
        ],
    );
    let total_opt = get_f64(
        &raw,
        &[
            "total",
            "limit",
            "quota",
            "total_credits",
            "maxCredits",
            "totalCredits",
            "quotaTotal",
            "credit_total",
        ],
    );
    let used_opt = get_f64(
        &raw,
        &[
            "used",
            "used_credits",
            "consumed",
            "usedCredits",
            "usage",
            "consumedCredits",
        ],
    );
    let remaining_opt = get_f64(
        &raw,
        &["remaining", "balance", "available", "creditBalance"],
    );
    let percent_opt = get_f64(
        &raw,
        &[
            "percent",
            "usage_percent",
            "used_percent",
            "percentUsed",
            "usagePercent",
        ],
    );
    let reset_str = get_string(
        &raw,
        &[
            "reset_at",
            "resetAt",
            "expiry",
            "expires_at",
            "expireAt",
            "reset",
            "expiresAt",
            "reset_at_str",
            "expiryDate",
            "expiry_date",
        ],
    );

    // 计算 used
    let mut total = total_opt.unwrap_or(0.0);
    let used = if let Some(u) = used_opt {
        u
    } else if total > 0.0 {
        if let Some(b) = balance_opt {
            (total - b).max(0.0)
        } else if let Some(r) = remaining_opt {
            (total - r).max(0.0)
        } else {
            0.0
        }
    } else {
        0.0
    };

    // 计算 remaining
    let remaining = if let Some(r) = remaining_opt {
        r
    } else if total > 0.0 {
        (total - used).max(0.0)
    } else {
        balance_opt.unwrap_or(0.0)
    };

    // balance 优先 remaining/balance
    let balance = balance_opt.unwrap_or(remaining);

    // 如果 total 仍为 0，尝试由 used+remaining 推导
    if total == 0.0 && (used > 0.0 || remaining > 0.0) {
        total = used + remaining;
    }
    // 如果 used 仍为 0 且 total>0 且 balance 有值，修正
    if used_opt.is_none() && total > 0.0 && remaining > 0.0 && used == 0.0 {
        // 已在上面计算，这里保持
    }
    // percent
    let percent = if let Some(p) = percent_opt {
        p.clamp(0.0, 100.0)
    } else if total > 0.0 {
        (used / total * 100.0).clamp(0.0, 100.0)
    } else {
        0.0
    };

    // 防止 NaN
    let balance = if balance.is_finite() { balance } else { 0.0 };
    let used = if used.is_finite() { used } else { 0.0 };
    let total = if total.is_finite() { total } else { 0.0 };
    let remaining = if remaining.is_finite() {
        remaining
    } else {
        0.0
    };
    let percent = if percent.is_finite() { percent } else { 0.0 };

    NormalizedCredits {
        balance,
        used,
        total,
        remaining,
        percent,
        reset_at: reset_str.clone(),
        expiry: reset_str,
        usage: None,
        raw: raw.clone(),
    }
}

// ─── URL 归一化（Go 专有：剥离尾部 /v1，统一拼 /v1/usage） ─────────────

pub fn normalize_base_url(base: &str) -> String {
    let trimmed = base.trim().trim_end_matches('/');
    if trimmed.len() >= 3 && trimmed[trimmed.len() - 3..].eq_ignore_ascii_case("/v1") {
        trimmed[..trimmed.len() - 3]
            .trim_end_matches('/')
            .to_string()
    } else {
        trimmed.to_string()
    }
}

pub fn build_credits_url(base: &str) -> String {
    format!("{}/v1/usage", normalize_base_url(base))
}

// ─── Fetcher 抽象 ───────────────────────────────────────────────

#[allow(dead_code)]
pub trait CreditsFetcher: Send + Sync {
    #[allow(dead_code)]
    fn name(&self) -> &'static str;
    fn can_handle(&self, profile: &ProviderProfile) -> bool;
    fn fetch<'a>(
        &'a self,
        upstream: &'a Upstream,
    ) -> std::pin::Pin<
        Box<dyn std::future::Future<Output = Result<NormalizedCredits, CreditsError>> + Send + 'a>,
    >;
}

// ─── OpencodeGoFetcher ──────────────────────────────────────────

pub struct OpencodeGoFetcher;

impl CreditsFetcher for OpencodeGoFetcher {
    fn name(&self) -> &'static str {
        "opencode-go"
    }

    fn can_handle(&self, profile: &ProviderProfile) -> bool {
        let url = profile.primary_base_url();
        url.to_ascii_lowercase().contains("opencode.ai")
    }

    fn fetch<'a>(
        &'a self,
        upstream: &'a Upstream,
    ) -> std::pin::Pin<
        Box<dyn std::future::Future<Output = Result<NormalizedCredits, CreditsError>> + Send + 'a>,
    > {
        let base = upstream.base_url.clone();
        let key = upstream.api_key.clone();
        Box::pin(async move {
            let url = build_credits_url(&base);
            let client = reqwest::Client::builder()
                .timeout(Duration::from_secs(5))
                .build()
                .map_err(|e| CreditsError::Network(e.to_string()))?;
            let req = client
                .get(&url)
                .header("Authorization", format!("Bearer {}", key));
            let resp = req.send().await.map_err(|e| {
                if e.is_timeout() {
                    CreditsError::Timeout(e.to_string())
                } else {
                    CreditsError::Network(e.to_string())
                }
            })?;
            let status = resp.status();
            if !status.is_success() {
                let body = resp.text().await.unwrap_or_default();
                // 截断过长 body
                let msg = if body.len() > 500 {
                    body[..500].to_string()
                } else {
                    body
                };
                return Err(CreditsError::Upstream {
                    status: status.as_u16(),
                    message: msg,
                });
            }
            let raw: Value = resp
                .json()
                .await
                .map_err(|e| CreditsError::Parse(e.to_string()))?;
            Ok(normalize_opencode_go(raw))
        })
    }
}

// ─── 注册表 ─────────────────────────────────────────────────────

pub fn registry() -> Vec<Box<dyn CreditsFetcher>> {
    vec![Box::new(OpencodeGoFetcher)]
    // 后续 Codex 仅新增： vec![Box::new(OpencodeGoFetcher), Box::new(CodexFetcher)]
}

pub fn find_fetcher(profile: &ProviderProfile) -> Option<Box<dyn CreditsFetcher>> {
    registry().into_iter().find(|f| f.can_handle(profile))
}

// ─── 对外：按供应商名查询（仅查主上游，不扇出，不写盘） ────────

pub async fn fetch_credits_for_profile(name: &str) -> Result<NormalizedCredits, CreditsError> {
    let cfg = config::load_config().map_err(|e| CreditsError::Network(e.to_string()))?;
    let profile_value = cfg
        .profiles
        .get(name)
        .ok_or_else(|| CreditsError::NotFound(format!("unknown profile '{}'", name)))?;
    let profile: ProviderProfile = serde_json::from_value(profile_value.clone())
        .map_err(|e| CreditsError::Parse(e.to_string()))?;

    let fetcher = find_fetcher(&profile).ok_or_else(|| {
        CreditsError::Unsupported(format!("credits not supported for profile '{}'", name))
    })?;

    // 仅查询主上游
    let upstreams = profile.resolved_upstreams();
    let upstream = if !upstreams.is_empty() {
        upstreams[0].clone()
    } else if !profile.base_url.is_empty() || !profile.api_key.is_empty() {
        Upstream {
            base_url: profile.base_url.clone(),
            api_key: profile.api_key.clone(),
            headers: profile.headers.clone(),
            ..Default::default()
        }
    } else {
        return Err(CreditsError::Unsupported(format!(
            "no upstream for profile '{}'",
            name
        )));
    };

    if upstream.base_url.trim().is_empty() {
        return Err(CreditsError::Unsupported(format!(
            "no upstream baseUrl for profile '{}'",
            name
        )));
    }

    fetcher.fetch(&upstream).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn normalize_maps_standard_fields() {
        let raw = json!({
            "balance": 42.5,
            "total": 100.0,
            "used": 30.0,
            "remaining": 70.0,
            "percent": 30.0,
            "reset_at": "2026-09-01T00:00:00Z"
        });
        let n = normalize_opencode_go(raw.clone());
        assert_eq!(n.balance, 42.5);
        assert_eq!(n.total, 100.0);
        assert_eq!(n.used, 30.0);
        assert_eq!(n.remaining, 70.0);
        assert_eq!(n.percent, 30.0);
        assert_eq!(n.reset_at.as_deref(), Some("2026-09-01T00:00:00Z"));
        assert_eq!(n.expiry.as_deref(), Some("2026-09-01T00:00:00Z"));
        assert_eq!(n.raw, raw);
    }

    #[test]
    fn normalize_computes_used_from_total_minus_balance() {
        let raw = json!({"balance": 70.0, "total": 100.0});
        let n = normalize_opencode_go(raw);
        assert_eq!(n.used, 30.0);
        assert_eq!(n.remaining, 70.0);
        assert!((n.percent - 30.0).abs() < 1e-6);
    }

    #[test]
    fn normalize_computes_remaining_when_missing() {
        let raw = json!({"used": 25.0, "total": 100.0});
        let n = normalize_opencode_go(raw);
        assert_eq!(n.remaining, 75.0);
    }

    #[test]
    fn normalize_handles_alternative_keys() {
        let raw = json!({"creditBalance": 10, "quota": 50, "used_credits": 40, "expires_at": "2026-12-01"});
        let n = normalize_opencode_go(raw);
        assert_eq!(n.balance, 10.0);
        assert_eq!(n.total, 50.0);
        assert_eq!(n.used, 40.0);
        assert_eq!(n.expiry.as_deref(), Some("2026-12-01"));
    }

    #[test]
    fn normalize_retains_raw_unmodified() {
        let raw = json!({"foo": "bar", "balance": 1});
        let n = normalize_opencode_go(raw.clone());
        assert_eq!(n.raw["foo"], "bar");
    }

    #[test]
    fn opencode_fetcher_can_handle_detection() {
        let fetcher = OpencodeGoFetcher;
        let mut p = ProviderProfile {
            base_url: "https://api.opencode.ai/v1".into(),
            ..Default::default()
        };
        assert!(fetcher.can_handle(&p));
        p.base_url = "https://example.com/v1".into();
        assert!(!fetcher.can_handle(&p));
        // 多上游主上游命中
        p.upstreams = vec![
            Upstream {
                base_url: "https://api.opencode.ai/v1".into(),
                api_key: "k".into(),
                ..Default::default()
            },
            Upstream {
                base_url: "https://other.com/v1".into(),
                api_key: "k2".into(),
                ..Default::default()
            },
        ];
        assert!(fetcher.can_handle(&p));
        // 多上游主上游不命中
        p.upstreams[0].base_url = "https://other.com/v1".into();
        assert!(!fetcher.can_handle(&p));
    }

    #[test]
    fn registry_contains_opencode_and_find_works() {
        let mut p = ProviderProfile {
            base_url: "https://api.opencode.ai/v1".into(),
            ..Default::default()
        };
        assert!(find_fetcher(&p).is_some());
        assert_eq!(find_fetcher(&p).unwrap().name(), "opencode-go");
        p.base_url = "https://example.com/v1".into();
        assert!(find_fetcher(&p).is_none());
    }

    #[test]
    fn registry_reserved_for_codex_only_add_file_and_register() {
        // 验证扩展点：新增 CodexFetcher 只需新增实现并在 registry 中追加一行，接口不变
        let reg = registry();
        assert!(reg.iter().any(|f| f.name() == "opencode-go"));
        // 未来 Codex 接入时，registry 长度将为 2 且包含 codex，但当前仅 1
        assert_eq!(reg.len(), 1);
    }

    #[test]
    fn normalize_base_url_strips_trailing_v1() {
        assert_eq!(
            normalize_base_url("https://opencode.ai/zen/go/v1"),
            "https://opencode.ai/zen/go"
        );
        assert_eq!(
            normalize_base_url("https://opencode.ai/zen/go/v1/"),
            "https://opencode.ai/zen/go"
        );
        assert_eq!(
            normalize_base_url("https://opencode.ai/zen/go/V1"),
            "https://opencode.ai/zen/go"
        );
        assert_eq!(
            normalize_base_url("https://opencode.ai/zen/go"),
            "https://opencode.ai/zen/go"
        );
        assert_eq!(
            normalize_base_url("https://api.opencode.ai/v1"),
            "https://api.opencode.ai"
        );
        assert_eq!(
            normalize_base_url("https://opencode.ai/zen/v1"),
            "https://opencode.ai/zen"
        );
    }

    #[test]
    fn build_credits_url_normalizes_and_uses_usage_path() {
        assert_eq!(
            build_credits_url("https://opencode.ai/zen/go/v1"),
            "https://opencode.ai/zen/go/v1/usage"
        );
        assert_eq!(
            build_credits_url("https://opencode.ai/zen/go/v1/"),
            "https://opencode.ai/zen/go/v1/usage"
        );
        assert_eq!(
            build_credits_url("https://opencode.ai/zen/go"),
            "https://opencode.ai/zen/go/v1/usage"
        );
        assert_eq!(
            build_credits_url("https://opencode.ai/zen/go/v1/v1/credits"),
            "https://opencode.ai/zen/go/v1/v1/credits/v1/usage"
        );
    }

    #[test]
    fn normalize_go_usage_payload() {
        let raw = json!({
            "usage": {
                "rolling": {"status": "ok", "percent": 6, "resetsAt": "2026-08-30T23:53:51.013Z"},
                "weekly": {"status": "ok", "percent": 52, "resetsAt": "2026-08-31T00:00:00.013Z"},
                "monthly": {"status": "ok", "percent": 38, "resetsAt": "2026-09-19T08:19:27.013Z"}
            }
        });
        let n = normalize_opencode_go(raw.clone());
        let usage = n.usage.expect("usage should be parsed");
        assert_eq!(usage.rolling.as_ref().unwrap().percent, 6.0);
        assert_eq!(usage.weekly.as_ref().unwrap().percent, 52.0);
        assert_eq!(usage.monthly.as_ref().unwrap().percent, 38.0);
        assert_eq!(usage.rolling.as_ref().unwrap().status, "ok");
        assert_eq!(
            usage.rolling.as_ref().unwrap().resets_at.as_deref(),
            Some("2026-08-30T23:53:51.013Z")
        );
        assert_eq!(n.raw, raw);
        // 兼容字段：percent 取 rolling
        assert!((n.percent - 6.0).abs() < 1e-6);
        assert_eq!(n.reset_at.as_deref(), Some("2026-08-30T23:53:51.013Z"));
    }

    #[test]
    fn normalize_go_usage_rate_limited_status() {
        let raw = json!({
            "usage": {
                "rolling": {"status": "rate-limited", "percent": 100, "resetsAt": "2026-08-30T23:53:51.013Z"},
                "weekly": {"status": "ok", "percent": 52, "resetsAt": "2026-08-31T00:00:00.013Z"},
                "monthly": {"status": "ok", "percent": 38, "resetsAt": "2026-09-19T08:19:27.013Z"}
            }
        });
        let n = normalize_opencode_go(raw);
        assert_eq!(
            n.usage.as_ref().unwrap().rolling.as_ref().unwrap().status,
            "rate-limited"
        );
        assert_eq!(
            n.usage.as_ref().unwrap().rolling.as_ref().unwrap().percent,
            100.0
        );
    }
}
