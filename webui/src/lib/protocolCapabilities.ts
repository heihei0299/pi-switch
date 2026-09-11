import type { ProtocolApiCapability, ResponsesMode } from "../types";

// Mirrors internal/protocol.Capabilities(). It is only a fallback for the window
// before GET /api/state answers (and for tests); App seeds the real value from
// the backend so the api list and responsesMode rule have one source.
const FALLBACK: readonly ProtocolApiCapability[] = [
  { id: "openai-completions", label: "OpenAI Chat Completions", defaultMode: "convert", responsesModes: ["auto", "convert"], canProxy: true, canGateway: true },
  { id: "openai-responses", label: "OpenAI Responses", defaultMode: "passthrough", responsesModes: ["auto", "passthrough"], canProxy: true, canGateway: true },
  { id: "anthropic-messages", label: "Anthropic Messages", defaultMode: "auto", responsesModes: ["auto"], canProxy: true, canGateway: false },
  { id: "google-generative-ai", label: "Google Gemini", defaultMode: "auto", responsesModes: ["auto"], canProxy: false, canGateway: false },
];

let capabilities: readonly ProtocolApiCapability[] = FALLBACK;

/** Replace the capability set with the backend's; empty input keeps the fallback. */
export function setProtocolCapabilities(next: readonly ProtocolApiCapability[] | undefined): void {
  capabilities = next && next.length > 0 ? next : FALLBACK;
}

export function protocolCapabilities(): readonly ProtocolApiCapability[] {
  return capabilities;
}

export function protocolApiIds(): readonly string[] {
  return capabilities.map((c) => c.id);
}

export function protocolApi(id: string): ProtocolApiCapability | undefined {
  return capabilities.find((c) => c.id === id);
}

export function defaultProtocolApiId(): string {
  return capabilities[0]?.id ?? "openai-completions";
}

export function defaultResponsesModeFor(api: string): ResponsesMode {
  return protocolApi(api)?.defaultMode ?? "auto";
}

export function allowedResponsesModes(api: string): readonly ResponsesMode[] {
  return protocolApi(api)?.responsesModes ?? ["auto"];
}
