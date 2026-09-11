import type { ProtocolApiCapability, ProviderProfile, ResponsesMode } from "../types";
import { allowedResponsesModes, defaultResponsesModeFor } from "./protocolCapabilities";

export function effectiveResponsesMode(
  profile: Pick<ProviderProfile, "api" | "responsesMode">,
  caps: readonly ProtocolApiCapability[],
): ResponsesMode {
  if (profile.responsesMode !== "auto" && profile.responsesMode) return profile.responsesMode;
  return defaultResponsesModeFor(caps, profile.api);
}

// The allowed set comes from the backend capability; only the operator-facing
// wording stays here (keyed by mode so i18n keeps working).
export function responsesModeError(
  api: string,
  mode: ResponsesMode,
  caps: readonly ProtocolApiCapability[],
): string | null {
  if (allowedResponsesModes(caps, api).includes(mode)) return null;
  if (mode === "passthrough") return "passthrough requires openai-responses";
  if (mode === "convert") return "convert requires openai-completions";
  return `invalid responsesMode ${mode}`;
}
