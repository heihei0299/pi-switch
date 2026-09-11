import type { ProtocolApiCapability, ResponsesMode } from "../types";
import fallback from "./protocol-capabilities.json";

// The fallback is this JSON fixture, which a Go test keeps byte-identical to
// internal/protocol.Capabilities(). There is no second hand-written list, so the
// client cannot drift from the backend capability set.
export const FALLBACK_PROTOCOL_APIS = fallback as readonly ProtocolApiCapability[];

function resolved(caps: readonly ProtocolApiCapability[]): readonly ProtocolApiCapability[] {
  return caps.length > 0 ? caps : FALLBACK_PROTOCOL_APIS;
}

export function protocolCapabilities(caps: readonly ProtocolApiCapability[]): readonly ProtocolApiCapability[] {
  return resolved(caps);
}

export function protocolApiIds(caps: readonly ProtocolApiCapability[]): readonly string[] {
  return resolved(caps).map((c) => c.id);
}

export function protocolApi(
  caps: readonly ProtocolApiCapability[],
  id: string,
): ProtocolApiCapability | undefined {
  return resolved(caps).find((c) => c.id === id);
}

export function defaultProtocolApiId(caps: readonly ProtocolApiCapability[] = FALLBACK_PROTOCOL_APIS): string {
  return resolved(caps)[0]?.id ?? "openai-completions";
}

export function defaultResponsesModeFor(
  caps: readonly ProtocolApiCapability[],
  api: string,
): ResponsesMode {
  return protocolApi(caps, api)?.defaultMode ?? "auto";
}

export function allowedResponsesModes(
  caps: readonly ProtocolApiCapability[],
  api: string,
): readonly ResponsesMode[] {
  return protocolApi(caps, api)?.responsesModes ?? ["auto"];
}
