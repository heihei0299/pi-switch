import type { Upstream } from "../types";

export function mergePreviewHeaders(
  profileHeaders?: Record<string, string>,
  upstreamHeaders?: Record<string, string>,
): Record<string, string> {
  const out: Record<string, string> = {};
  if (profileHeaders) {
    for (const [k, v] of Object.entries(profileHeaders)) out[k] = v;
  }
  if (upstreamHeaders) {
    for (const [k, v] of Object.entries(upstreamHeaders)) out[k] = v;
  }
  return out;
}

export function previewHeadersForUpstream(
  profileHeaders: Record<string, string> | undefined,
  upstreams: Upstream[],
  index: number,
): Record<string, string> {
  const ups = upstreams[index];
  return mergePreviewHeaders(profileHeaders, ups?.headers as Record<string, string> | undefined);
}
