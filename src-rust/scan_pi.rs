//! Pi 会话供应商侧扫盘 — 纯文件发现与树解析。
//! 复用 cc-switch 的根发现、布局、校验、树遍历规则
//! （MAX_SESSION_BYTES 128MB / MAX_TREE_ENTRIES 500k / TITLE_MAX_LEN 60）
//! 3s 全量轮询，不依赖代理与 requests.log。

use std::collections::{HashMap, HashSet};
use std::path::{Path, PathBuf};

pub const MAX_SESSION_BYTES: u64 = 128 * 1024 * 1024;
pub const MAX_TREE_ENTRIES: usize = 500_000;
pub const TITLE_MAX_LEN: usize = 60;
/// 轮询间隔常量，首版固定 3s，不暴露为配置。
pub const SCAN_INTERVAL_SECS: u64 = 3;

// ─── SessionRoot ─────────────────────────────────────────────

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum SessionRoot {
    Available(PathBuf),
    RequiresProjectContext(String),
    Unavailable,
}

fn is_relative_path(s: &str) -> bool {
    let t = s.trim();
    if t.is_empty() {
        return false;
    }
    let p = Path::new(t);
    !p.is_absolute()
}

/// 纯函数：按优先级判定根。env > config > default（default 在此判为 Available）。
/// 相对路径 → RequiresProjectContext 并由调用方 warn。
pub fn resolve_session_root_with(
    env_val: Option<&str>,
    config_session_dir: Option<&str>,
) -> SessionRoot {
    if let Some(v) = env_val {
        let t = v.trim();
        if !t.is_empty() {
            if is_relative_path(t) {
                log::warn!("PI_CODING_AGENT_SESSION_DIR is relative, requires project context: {}", t);
                return SessionRoot::RequiresProjectContext(t.to_string());
            }
            return SessionRoot::Available(PathBuf::from(t));
        }
    }
    if let Some(v) = config_session_dir {
        let t = v.trim();
        if !t.is_empty() {
            if is_relative_path(t) {
                log::warn!("pi config sessionDir is relative, requires project context: {}", t);
                return SessionRoot::RequiresProjectContext(t.to_string());
            }
            return SessionRoot::Available(PathBuf::from(t));
        }
    }
    SessionRoot::Available(default_sessions_dir())
}

fn default_sessions_dir() -> PathBuf {
    dirs::home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".pi")
        .join("agent")
        .join("sessions")
}

/// 从真实环境与 pi 配置读取 sessionDir 的包装。
pub fn resolve_session_root() -> SessionRoot {
    let env_val = std::env::var("PI_CODING_AGENT_SESSION_DIR").ok();
    let cfg_val = pi_settings_session_dir();
    resolve_session_root_with(env_val.as_deref(), cfg_val.as_deref())
}

fn pi_settings_session_dir() -> Option<String> {
    let p = dirs::home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".pi")
        .join("agent")
        .join("settings.json");
    let text = std::fs::read_to_string(&p).ok()?;
    let v: serde_json::Value = serde_json::from_str(&text).ok()?;
    v.get("sessionDir")
        .and_then(|x| x.as_str())
        .map(|s| s.to_string())
}

// ─── Collect ─────────────────────────────────────────────────

fn is_valid_id(id: &str) -> bool {
    if id.is_empty() || id.len() > 64 {
        return false;
    }
    id.chars()
        .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_')
}

fn sanitize_title(raw: &str) -> String {
    let replaced: String = raw
        .chars()
        .map(|c| if c.is_control() { ' ' } else { c })
        .collect();
    let trimmed = replaced.trim();
    if trimmed.is_empty() {
        return String::new();
    }
    // collapse consecutive control-derived spaces: replace any run of whitespace? spec says控字符→空格，not collapse spaces generally.
    // We'll collapse multiple spaces that originated from controls: the above already maps each control to single space, keep as is.
    // Then truncate to TITLE_MAX_LEN chars (not bytes).
    let chars: Vec<char> = trimmed.chars().collect();
    if chars.len() > TITLE_MAX_LEN {
        chars[..TITLE_MAX_LEN].iter().collect()
    } else {
        trimmed.to_string()
    }
}

// 递归收集 *.jsonl，过滤大小 ≤128MB。其余校验（header / 条目数）在 parse 阶段完成，
// 但为满足 Seam B“非法头跳过”的期望，collect_valid_* 会预检首行。
fn collect_raw_jsonl(root: &Path) -> Vec<PathBuf> {
    let mut out = Vec::new();
    let mut stack = vec![root.to_path_buf()];
    while let Some(dir) = stack.pop() {
        let entries = match std::fs::read_dir(&dir) {
            Ok(r) => r,
            Err(e) => {
                log::warn!("scan_pi: cannot read dir {}: {}", dir.display(), e);
                continue;
            }
        };
        for ent in entries.flatten() {
            let p = ent.path();
            if p.is_dir() {
                stack.push(p);
            } else if p.is_file() {
                if p.extension().and_then(|s| s.to_str()) == Some("jsonl") {
                    // size check
                    if let Ok(meta) = std::fs::metadata(&p) {
                        if meta.len() > MAX_SESSION_BYTES {
                            log::warn!("scan_pi: skip oversized {} ({} bytes)", p.display(), meta.len());
                            continue;
                        }
                    }
                    out.push(p);
                }
            }
        }
    }
    out.sort();
    out
}

/// 返回通过首行校验的合法文件。对于不可读目录返回空并 warn。
pub fn collect_valid_session_files(root: &Path) -> Vec<PathBuf> {
    let candidates = collect_raw_jsonl(root);
    let mut valid = Vec::new();
    for p in candidates {
        if is_valid_session_file(&p) {
            valid.push(p);
        } else {
            log::warn!("scan_pi: skip invalid header {}", p.display());
        }
    }
    valid
}

fn is_valid_session_file(path: &Path) -> bool {
    let f = match std::fs::File::open(path) {
        Ok(f) => f,
        Err(_) => return false,
    };
    let mut reader = std::io::BufReader::new(f);
    let mut first = String::new();
    use std::io::BufRead;
    match reader.read_line(&mut first) {
        Ok(0) => return false,
        Ok(_) => {}
        Err(_) => return false,
    }
    let v: serde_json::Value = match serde_json::from_str(first.trim()) {
        Ok(v) => v,
        Err(_) => return false,
    };
    v.get("type").and_then(|x| x.as_str()) == Some("session")
}

// ─── Parse ───────────────────────────────────────────────────

#[derive(Debug, Clone)]
pub struct PiSession {
    pub id: String,
    pub title: String,
    pub last_active_at: Option<String>,
    pub model: Option<String>,
    pub prompt_tokens_hint: Option<u64>,
}

#[derive(Debug, Clone)]
struct RawEntry {
    id: String,
    parent_id: Option<String>,
    timestamp: Option<String>,
    typ: String,
    name: Option<String>,
    message_role: Option<String>,
    message_text: Option<String>,
    model_id: Option<String>,
    prompt_tokens: Option<u64>,
}

fn extract_text_from_message_value(msg: &serde_json::Value) -> Option<String> {
    let role = msg.get("role").and_then(|v| v.as_str())?;
    if role != "user" {
        return None;
    }
    let content = msg.get("content")?;
    if let Some(s) = content.as_str() {
        let t = s.trim();
        if t.is_empty() {
            return None;
        }
        return Some(t.to_string());
    }
    if let Some(arr) = content.as_array() {
        let mut out = String::new();
        for part in arr {
            if part.get("type").and_then(|v| v.as_str()) == Some("text") {
                if let Some(t) = part.get("text").and_then(|v| v.as_str()) {
                    if !out.is_empty() {
                        out.push(' ');
                    }
                    out.push_str(t);
                }
            }
        }
        let t = out.trim();
        if t.is_empty() {
            return None;
        }
        return Some(t.to_string());
    }
    None
}

fn parse_line_to_entry(line: &str) -> Option<RawEntry> {
    let v: serde_json::Value = serde_json::from_str(line).ok()?;
    let typ = v.get("type").and_then(|x| x.as_str())?.to_string();
    // id may be missing for legacy <2; treat missing as empty -> will be synthesized
    let id = v.get("id").and_then(|x| x.as_str()).unwrap_or("").to_string();
    let parent_id = v.get("parentId").and_then(|x| x.as_str()).map(|s| s.to_string());
    // parentId may be null -> None
    let parent_id = if parent_id.as_deref() == Some("") { None } else { parent_id };
    let timestamp = v.get("timestamp").and_then(|x| x.as_str()).map(|s| s.to_string());
    let name = if typ == "session_info" {
        v.get("name").and_then(|x| x.as_str()).map(|s| s.to_string())
    } else {
        None
    };
    let (message_role, message_text) = if typ == "message" {
        if let Some(msg) = v.get("message") {
            let role = msg.get("role").and_then(|x| x.as_str()).map(|s| s.to_string());
            let text = extract_text_from_message_value(msg);
            (role, text)
        } else {
            (None, None)
        }
    } else {
        (None, None)
    };
    let model_id = if typ == "model_change" {
        v.get("modelId").and_then(|x| x.as_str()).map(|s| s.to_string())
    } else if typ == "message" {
        v.get("message")
            .and_then(|m| m.get("model"))
            .and_then(|x| x.as_str())
            .map(|s| s.to_string())
            .or_else(|| v.get("modelId").and_then(|x| x.as_str()).map(|s| s.to_string()))
    } else {
        None
    };
    let prompt_tokens = if typ == "message" {
        v.get("message")
            .and_then(|m| m.get("usage"))
            .and_then(|u| u.get("input"))
            .and_then(|x| x.as_u64())
            .or_else(|| v.get("usage").and_then(|u| u.get("input")).and_then(|x| x.as_u64()))
            .or_else(|| v.get("message").and_then(|m| m.get("usage")).and_then(|u| u.get("prompt_tokens")).and_then(|x| x.as_u64()))
    } else {
        None
    };
    Some(RawEntry {
        id,
        parent_id,
        timestamp,
        typ,
        name,
        message_role,
        message_text,
        model_id,
        prompt_tokens,
    })
}

fn parse_timestamp_ms(s: &str) -> Option<i64> {
    // try RFC3339
    if let Ok(dt) = chrono::DateTime::parse_from_rfc3339(s) {
        return Some(dt.timestamp_millis());
    }
    // try epoch millis as decimal string?
    if let Ok(n) = s.parse::<i64>() {
        return Some(n);
    }
    None
}

/// 从 JSONL 内容解析单会话。返回 None 表示跳过（非法头/超限等）。
pub fn parse_session_str(content: &str) -> Option<PiSession> {
    let mut lines = content.lines();
    let header_line = lines.next()?.trim();
    if header_line.is_empty() {
        return None;
    }
    let header_v: serde_json::Value = serde_json::from_str(header_line).ok()?;
    if header_v.get("type").and_then(|x| x.as_str()) != Some("session") {
        return None;
    }
    let version = header_v.get("version").and_then(|x| x.as_u64()).unwrap_or(0);
    let header_id = header_v.get("id").and_then(|x| x.as_str())?.to_string();
    if !is_valid_id(&header_id) {
        return None;
    }
    let cwd = header_v
        .get("cwd")
        .and_then(|x| x.as_str())
        .unwrap_or("")
        .to_string();

    // collect raw entries lines, check count
    let rest: Vec<String> = lines.map(|l| l.to_string()).collect();
    if rest.len() > MAX_TREE_ENTRIES {
        log::warn!("scan_pi: skip too many entries {} > {}", rest.len(), MAX_TREE_ENTRIES);
        return None;
    }
    if rest.is_empty() {
        // no entries, title falls back to cwd basename
        let title = cwd_basename(&cwd);
        return Some(PiSession {
            id: header_id,
            title,
            last_active_at: header_v
                .get("timestamp")
                .and_then(|x| x.as_str())
                .map(|s| s.to_string()),
            model: None,
            prompt_tokens_hint: None,
        });
    }

    if version < 2 {
        return parse_legacy(header_id, cwd, header_v, rest);
    }
    parse_v2(header_id, cwd, header_v, rest)
}

fn cwd_basename(cwd: &str) -> String {
    let p = Path::new(cwd);
    p.file_name()
        .and_then(|s| s.to_str())
        .map(|s| sanitize_title(s))
        .filter(|s| !s.is_empty())
        .unwrap_or_else(|| {
            if cwd.is_empty() {
                "untitled".to_string()
            } else {
                sanitize_title(cwd)
            }
        })
}

fn parse_legacy(
    header_id: String,
    cwd: String,
    header_v: serde_json::Value,
    rest: Vec<String>,
) -> Option<PiSession> {
    // legacy: linear chain legacy-0, legacy-1 ...
    let mut entries: Vec<RawEntry> = Vec::new();
    for (idx, line) in rest.iter().enumerate() {
        let t = line.trim();
        if t.is_empty() {
            continue;
        }
        if let Some(mut e) = parse_line_to_entry(t) {
            // synthesize id/parent
            e.id = format!("legacy-{}", idx);
            e.parent_id = if idx == 0 {
                None
            } else {
                Some(format!("legacy-{}", idx - 1))
            };
            entries.push(e);
        }
    }
    // title extraction over linear order
    let title = extract_title(&entries, &cwd);
    let last_active_at = extract_last_active(&entries, &header_v);
    let model = extract_model(&entries);
    let prompt_tokens_hint = extract_prompt_hint(&entries);
    Some(PiSession {
        id: header_id,
        title,
        last_active_at,
        model,
        prompt_tokens_hint,
    })
}

fn parse_v2(
    header_id: String,
    cwd: String,
    header_v: serde_json::Value,
    rest: Vec<String>,
) -> Option<PiSession> {
    // Build map, dedup (first wins), validate ids
    let mut map: HashMap<String, RawEntry> = HashMap::new();
    let mut order: Vec<String> = Vec::new(); // insertion order of valid ids
    let mut last_valid_id: Option<String> = None;

    for line in rest.iter() {
        let t = line.trim();
        if t.is_empty() {
            continue;
        }
        let Some(e) = parse_line_to_entry(t) else {
            continue;
        };
        if e.id.is_empty() || !is_valid_id(&e.id) {
            continue;
        }
        if let Some(pid) = &e.parent_id {
            if !is_valid_id(pid) {
                // treat invalid parent as None (orphan handling will skip)
                // keep entry but parent becomes None
                // we mutate copy
                let mut ee = e.clone();
                ee.parent_id = None;
                if map.contains_key(&ee.id) {
                    continue;
                }
                order.push(ee.id.clone());
                last_valid_id = Some(ee.id.clone());
                map.insert(ee.id.clone(), ee);
                continue;
            }
        }
        if map.contains_key(&e.id) {
            // duplicate — first wins, skip
            continue;
        }
        order.push(e.id.clone());
        last_valid_id = Some(e.id.clone());
        map.insert(e.id.clone(), e);
    }

    // Determine latest_id: last valid entry id, fallback to header_id? but header not in tree
    let latest_id = match last_valid_id {
        Some(id) => id,
        None => {
            // no valid entries, fallback to header
            let title = cwd_basename(&cwd);
            return Some(PiSession {
                id: header_id.clone(),
                title,
                last_active_at: header_v
                    .get("timestamp")
                    .and_then(|x| x.as_str())
                    .map(|s| s.to_string()),
                model: None,
                prompt_tokens_hint: None,
            });
        }
    };

    // Backtrack active branch
    let mut active_ids: Vec<String> = Vec::new();
    let mut visited: HashSet<String> = HashSet::new();
    let mut cur = Some(latest_id.clone());
    while let Some(cid) = cur {
        if !visited.insert(cid.clone()) {
            // cycle detected
            log::warn!("scan_pi: cycle detected at {}", cid);
            break;
        }
        let entry = match map.get(&cid) {
            Some(e) => e,
            None => break, // orphan
        };
        active_ids.push(cid.clone());
        cur = entry.parent_id.clone();
        // if parent not in map, next loop will break as orphan
        if let Some(pid) = &cur {
            if !map.contains_key(pid) {
                // orphan parent, stop after current? spec says discard orphans: stop
                // do not include orphan itself beyond current
                break;
            }
        }
    }
    active_ids.reverse(); // root -> leaf order

    // Collect active entries in order
    let active_entries: Vec<RawEntry> = active_ids
        .iter()
        .filter_map(|id| map.get(id).cloned())
        .collect();

    if active_entries.is_empty() {
        let title = cwd_basename(&cwd);
        return Some(PiSession {
            id: header_id,
            title,
            last_active_at: header_v
                .get("timestamp")
                .and_then(|x| x.as_str())
                .map(|s| s.to_string()),
            model: None,
            prompt_tokens_hint: None,
        });
    }

    let title = extract_title(&active_entries, &cwd);
    let last_active_at = extract_last_active(&active_entries, &header_v);
    let model = extract_model(&active_entries);
    let prompt_tokens_hint = extract_prompt_hint(&active_entries);
    Some(PiSession {
        id: header_id,
        title,
        last_active_at,
        model,
        prompt_tokens_hint,
    })
}

fn extract_title(entries: &[RawEntry], cwd: &str) -> String {
    // priority: session_info.name > first user message > cwd basename
    // For active branch order root->leaf, we want latest session_info? Take first from leaf backwards for session_info.
    for e in entries.iter().rev() {
        if e.typ == "session_info" {
            if let Some(n) = &e.name {
                let t = n.trim();
                if !t.is_empty() {
                    return sanitize_title(t);
                }
            }
        }
    }
    // first user message: scan in chronological order (entries order is root->leaf, which approximates chronology)
    for e in entries.iter() {
        if e.typ == "message" && e.message_role.as_deref() == Some("user") {
            if let Some(txt) = &e.message_text {
                let s = sanitize_title(txt);
                if !s.is_empty() {
                    return s;
                }
            }
        }
    }
    cwd_basename(cwd)
}

fn extract_last_active(entries: &[RawEntry], header_v: &serde_json::Value) -> Option<String> {
    let mut best_ms: Option<i64> = None;
    let mut best_str: Option<String> = None;
    // consider header timestamp as candidate
    if let Some(ts) = header_v.get("timestamp").and_then(|x| x.as_str()) {
        if let Some(ms) = parse_timestamp_ms(ts) {
            best_ms = Some(ms);
            best_str = Some(ts.to_string());
        }
    }
    for e in entries {
        if let Some(ts) = &e.timestamp {
            if let Some(ms) = parse_timestamp_ms(ts) {
                if best_ms.map_or(true, |b| ms > b) {
                    best_ms = Some(ms);
                    best_str = Some(ts.clone());
                }
            }
        }
    }
    best_str
}

fn extract_model(entries: &[RawEntry]) -> Option<String> {
    // latest model_change or message model in active branch order (last wins)
    let mut model: Option<String> = None;
    for e in entries {
        if let Some(m) = &e.model_id {
            let t = m.trim();
            if !t.is_empty() {
                model = Some(t.to_string());
            }
        }
    }
    model
}

fn extract_prompt_hint(entries: &[RawEntry]) -> Option<u64> {
    // last usage input in active branch
    let mut hint: Option<u64> = None;
    for e in entries {
        if let Some(pt) = e.prompt_tokens {
            hint = Some(pt);
        }
    }
    hint
}

// ─── File-level helpers ──────────────────────────────────────

/// 解析单文件，处理大小与条目数超限。
pub fn parse_session_file(path: &Path) -> Option<PiSession> {
    let meta = std::fs::metadata(path).ok()?;
    if meta.len() > MAX_SESSION_BYTES {
        log::warn!("scan_pi: skip oversized {} ({} bytes)", path.display(), meta.len());
        return None;
    }
    let content = std::fs::read_to_string(path).ok()?;
    // quick entry count check without full parse
    let line_count = content.lines().count();
    if line_count > MAX_TREE_ENTRIES + 1 {
        log::warn!("scan_pi: skip too many entries {} ({})", path.display(), line_count);
        return None;
    }
    parse_session_str(&content)
}

/// 扫描根目录，返回 id -> PiSession 映射。失败仅 warn 返回空。
pub fn scan_sessions(root: &Path) -> HashMap<String, PiSession> {
    let files = collect_valid_session_files(root);
    let mut out = HashMap::new();
    for p in files {
        if let Some(sess) = parse_session_file(&p) {
            out.insert(sess.id.clone(), sess);
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;

    fn tmp_dir(name: &str) -> PathBuf {
        let base = std::env::temp_dir().join(format!(
            "pi-switch-scan-{}-{}",
            std::process::id(),
            name
        ));
        let _ = std::fs::remove_dir_all(&base);
        std::fs::create_dir_all(&base).unwrap();
        base
    }

    fn write_file(path: &Path, content: &str) {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).unwrap();
        }
        let mut f = std::fs::File::create(path).unwrap();
        f.write_all(content.as_bytes()).unwrap();
    }

    fn header(version: u32, id: &str, cwd: &str, ts: &str) -> String {
        format!(
            r#"{{"type":"session","version":{},"id":"{}","timestamp":"{}","cwd":"{}"}}"#,
            version, id, ts, cwd
        )
    }

    fn msg(id: &str, parent: Option<&str>, ts: &str, role: &str, text: &str) -> String {
        let p = parent.map(|x| format!(r#""{}""#, x)).unwrap_or("null".to_string());
        let text_json = serde_json::to_string(text).unwrap();
        format!(
            r#"{{"type":"message","id":"{}","parentId":{},"timestamp":"{}","message":{{"role":"{}","content":[{{"type":"text","text":{}}}]}}}}"#,
            id, p, ts, role, text_json
        )
    }

    fn session_info(id: &str, parent: Option<&str>, ts: &str, name: &str) -> String {
        let p = parent.map(|x| format!(r#""{}""#, x)).unwrap_or("null".to_string());
        format!(
            r#"{{"type":"session_info","id":"{}","parentId":{},"timestamp":"{}","name":"{}"}}"#,
            id, p, ts, name.replace('"', "\\\"")
        )
    }

    // ─── Seam A ──────────────────────────────────────────────
    #[test]
    fn resolve_env_absolute_wins() {
        let r = resolve_session_root_with(Some("/tmp/abs"), Some("/other/cfg"));
        assert_eq!(r, SessionRoot::Available(PathBuf::from("/tmp/abs")));
    }

    #[test]
    fn resolve_env_relative_requires_context() {
        let r = resolve_session_root_with(Some("relative/path"), None);
        assert_eq!(
            r,
            SessionRoot::RequiresProjectContext("relative/path".to_string())
        );
    }

    #[test]
    fn resolve_config_when_env_missing() {
        let r = resolve_session_root_with(None, Some("/cfg/abs"));
        assert_eq!(r, SessionRoot::Available(PathBuf::from("/cfg/abs")));
    }

    #[test]
    fn resolve_config_relative_requires_context() {
        let r = resolve_session_root_with(None, Some("rel/cfg"));
        assert_eq!(
            r,
            SessionRoot::RequiresProjectContext("rel/cfg".to_string())
        );
    }

    #[test]
    fn resolve_default_when_both_missing() {
        let r = resolve_session_root_with(None, None);
        match r {
            SessionRoot::Available(p) => {
                assert!(p.to_string_lossy().ends_with(".pi/agent/sessions"));
            }
            _ => panic!("expected Available"),
        }
    }

    #[test]
    fn resolve_env_wins_over_config() {
        let r = resolve_session_root_with(Some("/env"), Some("/cfg"));
        assert_eq!(r, SessionRoot::Available(PathBuf::from("/env")));
    }

    #[test]
    fn resolve_empty_env_falls_through() {
        let r = resolve_session_root_with(Some("  "), Some("/cfg/abs2"));
        assert_eq!(r, SessionRoot::Available(PathBuf::from("/cfg/abs2")));
    }

    // ─── Seam B ──────────────────────────────────────────────

    #[test]
    fn collect_flat_and_project_directories() {
        let root = tmp_dir("seam-b-flat");
        // Flat file
        let flat = root.join("a.jsonl");
        write_file(
            &flat,
            &format!("{}\n{}\n", header(3, "019f0000-0000-0000-0000-000000000001", "/tmp", "2026-08-01T00:00:00Z"), msg("m1", None, "2026-08-01T00:01:00Z", "user", "hello")),
        );
        // ProjectDirectories file
        let proj_file = root.join("--home-shial--").join("b.jsonl");
        write_file(
            &proj_file,
            &format!("{}\n{}\n", header(3, "019f0000-0000-0000-0000-000000000002", "/home/shial", "2026-08-01T00:00:00Z"), msg("m2", None, "2026-08-01T00:01:00Z", "user", "hi")),
        );
        // non-jsonl ignored
        write_file(&root.join("ignore.txt"), "not jsonl\n");
        let files = collect_valid_session_files(&root);
        assert_eq!(files.len(), 2);
        assert!(files.iter().any(|p| p.ends_with("a.jsonl")));
        assert!(files.iter().any(|p| p.ends_with("b.jsonl")));
        let _ = std::fs::remove_dir_all(&root);
    }

    #[test]
    fn collect_skips_invalid_header() {
        let root = tmp_dir("seam-b-header");
        let bad = root.join("bad.jsonl");
        write_file(&bad, "{\"type\":\"message\",\"id\":\"x\"}\n");
        let good = root.join("good.jsonl");
        write_file(
            &good,
            &format!("{}\n", header(3, "019f0000-0000-0000-0000-000000000003", "/tmp", "2026-08-01T00:00:00Z")),
        );
        let files = collect_valid_session_files(&root);
        assert_eq!(files.len(), 1);
        assert!(files[0].ends_with("good.jsonl"));
        let _ = std::fs::remove_dir_all(&root);
    }

    #[test]
    fn collect_skips_oversized_file() {
        let root = tmp_dir("seam-b-size");
        let p = root.join("big.jsonl");
        write_file(
            &p,
            &format!("{}\n", header(3, "019f0000-0000-0000-0000-000000000004", "/tmp", "2026-08-01T00:00:00Z")),
        );
        // sparse extend to >128M without writing all bytes
        let f = std::fs::OpenOptions::new().write(true).open(&p).unwrap();
        f.set_len(MAX_SESSION_BYTES + 1).unwrap();
        let files = collect_valid_session_files(&root);
        assert_eq!(files.len(), 0);
        let _ = std::fs::remove_dir_all(&root);
    }

    #[test]
    fn parse_skips_too_many_entries() {
        // Build content with >500k lines (use helper to avoid FS)
        let mut content = header(3, "019f0000-0000-0000-0000-000000000005", "/tmp", "2026-08-01T00:00:00Z");
        content.push('\n');
        // we don't actually create 500k lines (heavy), test the threshold logic directly by checking parse rejects
        // Instead craft content that claims to be large via line count helper: use actual large count but with minimal lines
        // We'll test the file-level check via set_len trick not applicable; test parse_session_str's entry count by building a string with 500001 lines efficiently
        // For speed, generate 500001 tiny lines
        let mut big = String::new();
        big.push_str(&header(3, "019f0000-0000-0000-0000-000000000006", "/tmp", "2026-08-01T00:00:00Z"));
        big.push('\n');
        for i in 0..MAX_TREE_ENTRIES {
            big.push_str(&format!("{{\"type\":\"message\",\"id\":\"id{:06}\",\"parentId\":null,\"timestamp\":\"2026-08-01T00:01:00Z\",\"message\":{{\"role\":\"user\",\"content\":[{{\"type\":\"text\",\"text\":\"hi\"}}]}}}}\n", i));
        }
        // Now big has 500001 lines total (header + 500000 entries) => exactly limit, should pass
        assert!(parse_session_str(&big).is_some());
        // Add one more to exceed
        big.push_str("{\"type\":\"message\",\"id\":\"overflow\",\"parentId\":null,\"timestamp\":\"2026-08-01T00:02:00Z\",\"message\":{\"role\":\"user\",\"content\":[{\"type\":\"text\",\"text\":\"hi\"}]}}");
        big.push('\n');
        assert!(parse_session_str(&big).is_none());
        let _ = content;
    }

    // ─── Seam C ──────────────────────────────────────────────

    #[test]
    fn active_branch_from_latest() {
        // Tree: A(null) -> B(A) -> D(B) ; A -> C(A) branch dead
        let h = header(3, "019f0000-0000-0000-0000-000000000010", "/proj", "2026-08-01T00:00:00Z");
        let a = r#"{"type":"message","id":"A","parentId":null,"timestamp":"2026-08-01T00:01:00Z","message":{"role":"user","content":[{"type":"text","text":"root"}]}}"#;
        let b = r#"{"type":"message","id":"B","parentId":"A","timestamp":"2026-08-01T00:02:00Z","message":{"role":"assistant","content":[{"type":"text","text":"b"}]}}"#;
        let c = r#"{"type":"message","id":"C","parentId":"A","timestamp":"2026-08-01T00:03:00Z","message":{"role":"assistant","content":[{"type":"text","text":"c"}]}}"#;
        let d = r#"{"type":"message","id":"D","parentId":"B","timestamp":"2026-08-01T00:04:00Z","message":{"role":"user","content":[{"type":"text","text":"leaf"}]}}"#;
        let content = format!("{}\n{}\n{}\n{}\n{}\n", h, a, b, c, d);
        let sess = parse_session_str(&content).unwrap();
        // title should be from first user message in active branch: A is user "root"
        assert_eq!(sess.title, "root");
        // last_active should be D's timestamp (max)
        assert_eq!(sess.last_active_at.as_deref(), Some("2026-08-01T00:04:00Z"));
    }

    #[test]
    fn duplicate_ids_first_wins() {
        let h = header(3, "019f0000-0000-0000-0000-000000000011", "/proj", "2026-08-01T00:00:00Z");
        let a1 = r#"{"type":"message","id":"dup","parentId":null,"timestamp":"2026-08-01T00:01:00Z","message":{"role":"user","content":[{"type":"text","text":"first"}]}}"#;
        let a2 = r#"{"type":"message","id":"dup","parentId":null,"timestamp":"2026-08-01T00:02:00Z","message":{"role":"user","content":[{"type":"text","text":"second"}]}}"#;
        let content = format!("{}\n{}\n{}\n", h, a1, a2);
        let sess = parse_session_str(&content).unwrap();
        // should pick first dup's text
        assert_eq!(sess.title, "first");
    }

    #[test]
    fn cycle_is_broken() {
        let h = header(3, "019f0000-0000-0000-0000-000000000012", "/proj", "2026-08-01T00:00:00Z");
        let a = r#"{"type":"message","id":"A","parentId":"B","timestamp":"2026-08-01T00:01:00Z","message":{"role":"user","content":[{"type":"text","text":"a"}]}}"#;
        let b = r#"{"type":"message","id":"B","parentId":"A","timestamp":"2026-08-01T00:02:00Z","message":{"role":"assistant","content":[{"type":"text","text":"b"}]}}"#;
        let content = format!("{}\n{}\n{}\n", h, a, b);
        // latest is B, chain B->A->B would cycle, should not infinite loop and should produce a title
        let sess = parse_session_str(&content).unwrap();
        assert!(!sess.title.is_empty());
    }

    #[test]
    fn legacy_version_uses_linear_chain() {
        let h = header(1, "019f0000-0000-0000-0000-000000000013", "/proj", "2026-08-01T00:00:00Z");
        let e1 = r#"{"type":"message","timestamp":"2026-08-01T00:01:00Z","message":{"role":"user","content":[{"type":"text","text":"legacy hello"}]}}"#;
        let e2 = r#"{"type":"message","timestamp":"2026-08-01T00:02:00Z","message":{"role":"assistant","content":[{"type":"text","text":"reply"}]}}"#;
        let content = format!("{}\n{}\n{}\n", h, e1, e2);
        let sess = parse_session_str(&content).unwrap();
        assert_eq!(sess.title, "legacy hello");
        assert_eq!(sess.id, "019f0000-0000-0000-0000-000000000013");
    }

    #[test]
    fn title_priority_session_info_over_user_message() {
        let h = header(3, "019f0000-0000-0000-0000-000000000014", "/proj/myapp", "2026-08-01T00:00:00Z");
        let si = session_info("S1", None, "2026-08-01T00:01:00Z", "explicit name");
        let um = msg("M1", Some("S1"), "2026-08-01T00:02:00Z", "user", "user text");
        let content = format!("{}\n{}\n{}\n", h, si, um);
        let sess = parse_session_str(&content).unwrap();
        assert_eq!(sess.title, "explicit name");
    }

    #[test]
    fn title_falls_back_to_cwd_basename() {
        let h = header(3, "019f0000-0000-0000-0000-000000000015", "/home/shial/myproj", "2026-08-01T00:00:00Z");
        let am = msg("A1", None, "2026-08-01T00:01:00Z", "assistant", "no user");
        let content = format!("{}\n{}\n", h, am);
        let sess = parse_session_str(&content).unwrap();
        assert_eq!(sess.title, "myproj");
    }

    #[test]
    fn title_truncation_and_sanitization() {
        let h = header(3, "019f0000-0000-0000-0000-000000000016", "/tmp", "2026-08-01T00:00:00Z");
        let long = "a".repeat(100);
        let with_ctrl = format!("{} \x01\x02 {}", long, "tail");
        let um = msg("M1", None, "2026-08-01T00:01:00Z", "user", &with_ctrl);
        let content = format!("{}\n{}\n", h, um);
        let sess = parse_session_str(&content).unwrap();
        // control chars become space, then truncated to 60
        assert_eq!(sess.title.len(), 60);
        assert!(!sess.title.contains('\x01'));
    }

    #[test]
    fn last_active_is_max_timestamp() {
        let h = header(3, "019f0000-0000-0000-0000-000000000017", "/tmp", "2026-08-01T00:00:00Z");
        let m1 = msg("A", None, "2026-08-01T00:10:00Z", "user", "hi");
        let m2 = msg("B", Some("A"), "2026-08-01T00:05:00Z", "assistant", "earlier");
        let m3 = msg("C", Some("B"), "2026-08-01T00:15:00Z", "user", "latest");
        let content = format!("{}\n{}\n{}\n{}\n", h, m1, m2, m3);
        let sess = parse_session_str(&content).unwrap();
        assert_eq!(sess.last_active_at.as_deref(), Some("2026-08-01T00:15:00Z"));
    }

    #[test]
    fn invalid_id_is_skipped() {
        let h = header(3, "019f0000-0000-0000-0000-000000000018", "/tmp", "2026-08-01T00:00:00Z");
        let bad = r#"{"type":"message","id":"bad id with space","parentId":null,"timestamp":"2026-08-01T00:01:00Z","message":{"role":"user","content":[{"type":"text","text":"bad"}]}}"#;
        let good = msg("GOOD", None, "2026-08-01T00:02:00Z", "user", "good");
        let content = format!("{}\n{}\n{}\n", h, bad, good);
        let sess = parse_session_str(&content).unwrap();
        assert_eq!(sess.title, "good");
    }

    #[test]
    fn scan_sessions_integration_flat_and_nested() {
        let root = tmp_dir("scan-integration");
        let h1 = header(3, "019f0000-0000-0000-0000-000000000020", "/tmp/proj", "2026-08-01T00:00:00Z");
        let m1 = msg("X", None, "2026-08-01T00:01:00Z", "user", "hello world");
        write_file(&root.join("s1.jsonl"), &format!("{}\n{}\n", h1, m1));
        let h2 = header(3, "019f0000-0000-0000-0000-000000000021", "/home/shial", "2026-08-01T00:00:00Z");
        let m2 = session_info("SI", None, "2026-08-01T00:02:00Z", "my session");
        write_file(
            &root.join("--nested--").join("s2.jsonl"),
            &format!("{}\n{}\n", h2, m2),
        );
        let map = scan_sessions(&root);
        assert_eq!(map.len(), 2);
        assert_eq!(map.get("019f0000-0000-0000-0000-000000000020").unwrap().title, "hello world");
        assert_eq!(map.get("019f0000-0000-0000-0000-000000000021").unwrap().title, "my session");
        let _ = std::fs::remove_dir_all(&root);
    }
}
