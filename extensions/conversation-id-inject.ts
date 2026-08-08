import { readFileSync } from "node:fs";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { CONFIG_PATH } from "../src/core.js";

export type RequestHeaders = Record<string, string | null>;

/**
 * Pure injection logic: return a new headers object with `x-conversation-id`
 * set to the current session id, overriding any existing value. The caller's
 * headers object is never mutated.
 */
export function injectConversationId(
  headers: RequestHeaders,
  sessionId: string | undefined,
): RequestHeaders {
  if (sessionId == null || sessionId.trim() === "") {
    return { ...headers };
  }
  return { ...headers, "x-conversation-id": sessionId };
}

/**
 * Percent-encode every code point above Latin-1 (> 255) so the value stays
 * a valid ByteString for HTTP headers: undici's Headers rejects non-Latin1
 * characters with a TypeError and the request dies before being sent.
 * ASCII/Latin1 characters are kept as-is, so plain names round-trip
 * unchanged. The `u` flag matches astral characters (surrogate pairs) as
 * single code points so `encodeURIComponent` never sees an isolated surrogate.
 */
function encodeHeaderValue(value: string): string {
  return value.replace(/[^\x00-\xff]/gu, (ch) => encodeURIComponent(ch));
}

/**
 * Pure injection logic for the conversation display name: return a new
 * headers object with `x-conversation-name` set to the current session
 * name, overriding any existing value. Non-Latin1 characters are
 * percent-encoded so the header value stays HTTP-safe. Blank names are not
 * injected. The caller's headers object is never mutated.
 */
export function injectConversationName(
  headers: RequestHeaders,
  sessionName: string | undefined,
): RequestHeaders {
  if (sessionName == null || sessionName.trim() === "") {
    return { ...headers };
  }
  return { ...headers, "x-conversation-name": encodeHeaderValue(sessionName) };
}

/**
 * OpenCode attribution headers for a request: `x-opencode-session` carries
 * the conversation id (so an opencode-go upstream groups the request into
 * the pi session) and `x-opencode-client: "pi"` identifies the client.
 * pi core only injects these for opencode/opencode-go providers or
 * opencode.ai base URLs; requests via a pi-switch proxy never meet that
 * condition, so this fills the gap — idempotent with pi core's injection
 * (same values) for direct-to-opencode setups. Blank conversation ids are
 * not injected. The caller's headers object is never mutated.
 */
export function injectOpenCodeAttribution(
  headers: RequestHeaders,
  conversationId: string | undefined,
): RequestHeaders {
  if (conversationId == null || conversationId.trim() === "") {
    return { ...headers };
  }
  return {
    ...headers,
    "x-opencode-session": conversationId,
    "x-opencode-client": "pi",
  };
}

/**
 * Parse the `settings.injectOpenCodeAttribution` switch out of the raw
 * pi-switch config file content. Conservative default: anything other than
 * an explicit `false` (missing file, broken JSON, missing key, non-boolean
 * value) keeps the attribution headers injected, so a broken config never
 * changes the established behavior.
 */
export function parseOpencodeAttributionConfig(raw: string | undefined): boolean {
  if (raw == null) {
    return true;
  }
  try {
    const config = JSON.parse(raw) as { settings?: { injectOpenCodeAttribution?: unknown } } | null;
    return config?.settings?.injectOpenCodeAttribution !== false;
  } catch {
    return true;
  }
}

/**
 * Read the switch from ~/.pi-switch/config.json at extension load time.
 * Any read/parse failure falls back to the conservative default (true), so
 * a missing or broken config never changes the established behavior.
 */
export function loadOpencodeAttributionConfig(): boolean {
  try {
    return parseOpencodeAttributionConfig(readFileSync(CONFIG_PATH, "utf8"));
  } catch {
    return true;
  }
}

export type ProviderHeadersEvent = { headers: RequestHeaders };

/**
 * The minimal session-manager surface this extension reads. Defined as a
 * concrete shape (not a generic) so the wiring stays trivially typed while
 * remaining decoupled from the full pi ExtensionContext.
 */
/**
 * The minimal session-manager surface this extension reads. Defined as a
 * concrete shape (not a generic) so the wiring stays trivially typed while
 * remaining decoupled from the full pi ExtensionContext.
 */
export type SessionIdProvider = {
  sessionManager: {
    getSessionId(): string | undefined;
    getSessionName(): string | undefined;
    getEntries(): SessionEntry[];
  };
};

/**
 * Loose shape of a session entry (pi's `AgentMessage`). Only the fields this
 * extension reads are typed, so it stays decoupled from pi internals.
 */
export type SessionEntryContent = string | Array<{ type?: string; text?: string }>;
export type SessionEntry = {
  type?: string;
  // Legacy flat shape (extension tests / early callers).
  role?: string;
  content?: SessionEntryContent;
  // Real pi SessionManager entry shape: role/content live under `message`.
  message?: {
    role?: string;
    content?: SessionEntryContent;
  };
};

export const TITLE_MAX_LEN = 60;

/**
 * Extract the text of a session entry's content: plain strings pass through;
 * block arrays contribute every `text` block (image/thinking/toolCall blocks
 * are ignored).
 */
function textOf(content: SessionEntryContent | undefined): string {
  if (typeof content === "string") {
    return content;
  }
  if (Array.isArray(content)) {
    return content
      .filter(
        (b): b is { type: string; text: string } =>
          b?.type === "text" && typeof b.text === "string",
      )
      .map((b) => b.text)
      .join(" ");
  }
  return "";
}

/**
 * Text of the first non-empty user message in a session, or `undefined` when
 * there is none. The returned text is trimmed but not sanitized/truncated —
 * callers decide how to present it.
 */
/**
 * Text of the first non-empty user message in a session, or `undefined` when
 * there is none. The returned text is trimmed but not sanitized/truncated —
 * callers decide how to present it. Handles both the legacy flat entry
 * (`role`/`content` at the top level) and the real pi SessionManager entry
 * shape (`role`/`content` nested under `message`).
 */
/**
 * Text of the first non-empty user message in a session, or `undefined` when
 * there is none. The returned text is trimmed but not sanitized/truncated —
 * callers decide how to present it. Handles both the legacy flat entry
 * (`role`/`content` at the top level) and the real pi SessionManager entry
 * shape (`role`/`content` nested under `message`).
 *
 * Pi writes skill invocations into the session as user messages whose text
 * starts with the `<skill name="..."` tag (the whole SKILL.md body follows).
 * Those are system injections, not the user's own words, so they are skipped
 * when picking the title text.
 */
const SKILL_INJECTION_RE = /^\s*<skill\s+name=/;

export function firstUserMessageText(entries: SessionEntry[]): string | undefined {
  for (const entry of entries) {
    const role = entry.message?.role ?? entry.role;
    if (role !== "user") {
      continue;
    }
    const text = textOf(entry.message?.content ?? entry.content).trim();
    if (text && !SKILL_INJECTION_RE.test(text)) {
      return text;
    }
  }
  return undefined;
}

/**
 * Resolve the conversation display name: an explicit non-blank name wins;
 * otherwise fall back to the first user message as a readable title, with
 * control characters collapsed to spaces and the result truncated to
 * `TITLE_MAX_LEN`. Returns `undefined` when neither source yields text.
 */
export function resolveSessionName(
  name: string | undefined,
  entries: SessionEntry[],
): string | undefined {
  if (name != null && name.trim() !== "") {
    return name;
  }
  const title = firstUserMessageText(entries);
  if (!title) {
    return undefined;
  }
  const sanitized = title.replace(/[\x00-\x1f\x7f]+/g, " ").trim();
  if (!sanitized) {
    return undefined;
  }
  return sanitized.slice(0, TITLE_MAX_LEN);
}
export type SessionInfo = {
  id?: string;
  name?: string;
};

/**
 * The subagent-folding env surface this extension reads. Subagent processes
 * are spawned by pi-subagents with `PI_SUBAGENT_DEPTH >= 1` and inherit the
 * parent's `PI_PARENT_SESSION_ID`; the parent process advertises its session
 * id through that variable so child requests fold into the same conversation.
 */
export type RequestEnv = {
  PI_SUBAGENT_DEPTH?: string;
  PI_PARENT_SESSION_ID?: string;
  MAGIC_CONTEXT_PI_SUBAGENT?: string;
};

export type RequestInjection = {
  conversationId?: string;
  conversationName?: string;
};

/**
 * Decide what to inject for the current process: a subagent (depth > 0)
 * folds its requests into the parent conversation (parent id only, no name
 * so the aggregate label stays the parent's); the parent process injects its
 * own id and resolved name.
 */
export function resolveRequestInjection(
  ownId: string | undefined,
  ownName: string | undefined,
  env: RequestEnv,
): RequestInjection {
  const depth = Number.parseInt(env.PI_SUBAGENT_DEPTH ?? "0", 10) || 0;
  if (depth > 0) {
    return env.PI_PARENT_SESSION_ID
      ? { conversationId: env.PI_PARENT_SESSION_ID }
      : {};
  }
  // Magic Context 后台任务（dreamer 等）以 --no-session 子进程运行：session 只在
  // 内存、不落盘，pi 从未持久化它。注入其 id 会把后台任务伪造成独立会话（webui
  // 会话统计里出现 pi 无记录的幽灵会话），因此这类进程不携带任何会话标识——
  // 请求三源皆空，代理端按 ADR-0002 兜底归入 unlabeled。
  if (env.MAGIC_CONTEXT_PI_SUBAGENT === "1") {
    return {};
  }
  return {
    ...(ownId ? { conversationId: ownId } : {}),
    ...(ownName ? { conversationName: ownName } : {}),
  };
}

export type HandlerOptions = {
  /**
   * Whether the opencode attribution headers (`x-opencode-session` /
   * `x-opencode-client`) are injected. Defaults to true; set to false via
   * `settings.injectOpenCodeAttribution` in ~/.pi-switch/config.json to
   * keep requests free of these two headers.
   */
  injectOpenCodeAttribution?: boolean;
};

/**
 * Build the `before_provider_headers` handler: it merges the injected headers
 * back into the event's headers in place (pi's contract for this hook) while
 * the pure functions stay non-mutating. The session-info provider is injected
 * so the handler is testable without a live pi session. The parent process
 * additionally advertises its session id via `PI_PARENT_SESSION_ID` so spawned
 * subagents (which inherit the env) fold their requests into it.
 */
export function makeBeforeProviderHeadersHandler(
  getSession: (ctx: SessionIdProvider) => SessionInfo,
  options?: HandlerOptions,
): (event: ProviderHeadersEvent, ctx: SessionIdProvider) => void {
  return (event, ctx) => {
    const { id, name } = getSession(ctx);
    const sessionName = resolveSessionName(name, ctx.sessionManager.getEntries());
    const env = process.env as RequestEnv;
    const { conversationId, conversationName } = resolveRequestInjection(id, sessionName, env);
    const depth = Number.parseInt(env.PI_SUBAGENT_DEPTH ?? "0", 10) || 0;
    const isParent = depth === 0;
    if (isParent && conversationId) {
      process.env.PI_PARENT_SESSION_ID = conversationId;
    }
    // 后台 ephemeral 进程（如 Magic Context dreamer）：剥离 pi 核心注入的
    // x-opencode-session，否则代理端仍会把 in-memory session id 记为会话。
    // opencode 归因头同样不注入：子代理形态的 Magic Context 进程（depth≥1）
    // 会从父会话拿到 conversationId，注入会让刚剥离的头复活（含
    // x-opencode-client），后台请求仍不得携带任何会话标识。
    const isMagicContext = env.MAGIC_CONTEXT_PI_SUBAGENT === "1";
    if (isMagicContext) {
      delete event.headers["x-opencode-session"];
    }
    Object.assign(event.headers, injectConversationId(event.headers, conversationId));
    Object.assign(event.headers, injectConversationName(event.headers, conversationName));
    if (!isMagicContext && options?.injectOpenCodeAttribution !== false) {
      Object.assign(event.headers, injectOpenCodeAttribution(event.headers, conversationId));
    }
  };
}

export default function conversationIdInjectExtension(pi: ExtensionAPI): void {
  // 扩展加载时读取一次配置，重启 pi 生效（与 pi-switch “Restart pi to apply”惯例一致）
  const attributionEnabled = loadOpencodeAttributionConfig();
  pi.on(
    "before_provider_headers",
    makeBeforeProviderHeadersHandler(
      (ctx) => ({
        id: ctx.sessionManager.getSessionId(),
        name: ctx.sessionManager.getSessionName(),
      }),
      { injectOpenCodeAttribution: attributionEnabled },
    ),
  );
}
