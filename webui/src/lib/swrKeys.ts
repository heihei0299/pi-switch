export const SWR_KEY_PROFILES = ["profiles"] as const;
export const SWR_KEY_GATEWAY = ["gateway"] as const;

export function swrKeyStats(range: string, from: number | null, to: number | null): readonly [string, string] {
  return ["stats", `${range}:${from ?? ""}:${to ?? ""}`] as const;
}

export const PUT_MUTATE_KEYS = [SWR_KEY_PROFILES, SWR_KEY_GATEWAY] as const;
