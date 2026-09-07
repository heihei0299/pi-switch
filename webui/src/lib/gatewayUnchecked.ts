// 网关二次勾选的"排除记忆"：用户取消勾选的网关 id 持久化到 localStorage，
// 跨 load()/刷新/应用后依然保持不勾选；新出现的 id 默认勾选不受影响。
// 只记"不勾选"（默认勾选是零状态）， akun 切换浏览器则偏好不互通，可接受。

export const UNCHECKED_KEY = "pi-switch-gateway-unchecked";

function readRaw(): string[] {
  try {
    if (typeof window === "undefined" || !window.localStorage) return [];
    const raw = window.localStorage.getItem(UNCHECKED_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((v): v is string => typeof v === "string" && v.length > 0);
  } catch {
    return [];
  }
}

function writeRaw(ids: Iterable<string>): void {
  try {
    if (typeof window === "undefined" || !window.localStorage) return;
    window.localStorage.setItem(UNCHECKED_KEY, JSON.stringify([...new Set(ids)]));
  } catch {
    // 存储不可用（如隐私模式）则退化为会话内记忆，由 skipAutoCheck 兜底
  }
}

// 读取排除集；传入 knownIds 时顺带修剪已不存在的 id 并写回，防止无限增长。
export function loadUncheckedIds(knownIds?: Set<string>): Set<string> {
  const ids = new Set(readRaw());
  if (knownIds) {
    let pruned = false;
    for (const id of ids) {
      if (!knownIds.has(id)) {
        ids.delete(id);
        pruned = true;
      }
    }
    if (pruned) writeRaw(ids);
  }
  return ids;
}

export function addUncheckedId(id: string): void {
  if (!id) return;
  const ids = new Set(readRaw());
  if (!ids.has(id)) {
    ids.add(id);
    writeRaw(ids);
  }
}

export function removeUncheckedId(id: string): void {
  if (!id) return;
  const ids = new Set(readRaw());
  if (ids.delete(id)) writeRaw(ids);
}
