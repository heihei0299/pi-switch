import { resolvedUpstreams, type ProviderProfile } from "../types";

export function isCreditsSupported(profile: ProviderProfile): boolean {
  const ups = resolvedUpstreams(profile);
  const primaryBaseUrl = ups.length > 0 ? ups[0].baseUrl : profile.baseUrl;
  if (!primaryBaseUrl) return false;
  return primaryBaseUrl.toLowerCase().includes("opencode.ai");
}

export interface UsageWindow {
  percent: number;
  status: string;
  resetsAt?: string | null;
}

export interface GoUsage {
  rolling?: UsageWindow | null;
  weekly?: UsageWindow | null;
  monthly?: UsageWindow | null;
}

export interface NormalizedCredits {
  balance: number;
  used: number;
  total: number;
  remaining: number;
  percent: number;
  resetAt?: string | null;
  expiry?: string | null;
  usage?: GoUsage | null;
  raw?: unknown;
}
