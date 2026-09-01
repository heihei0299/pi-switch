import { mutate } from "swr";
import { SWR_KEY_GATEWAY, SWR_KEY_PROFILES } from "../lib/swrKeys";

export const SWR_KEYS = {
  profiles: SWR_KEY_PROFILES,
  gateway: SWR_KEY_GATEWAY,
  stats: (window: string) => ["stats", window] as const,
} as const;

export async function mutateAfterProfilePut(): Promise<void> {
  await Promise.all([mutate(SWR_KEY_PROFILES as unknown as string), mutate(SWR_KEY_GATEWAY as unknown as string)]);
}

export async function mutateAfterGatewayPublish(): Promise<void> {
  await Promise.all([mutate(SWR_KEY_PROFILES as unknown as string), mutate(SWR_KEY_GATEWAY as unknown as string)]);
}

export async function mutateAfterFailover(): Promise<void> {
  await Promise.all([mutate(SWR_KEY_PROFILES as unknown as string), mutate(SWR_KEY_GATEWAY as unknown as string)]);
}
