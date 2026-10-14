# Architecture V2 W08e — channel model-fetch persistence convergence — 2026-10-13

**Status:** Channel model-fetch slice passed focused local verification. W08 remains in progress. No push, merge, remote CI, or release was performed.

## Implementation

- Kept `FetchChannelModelEntries` as read-only fetch/enrichment. Added an application workflow that loads the selected config, performs the upstream request outside the write transaction, and merges through the provider usecase's no-backup update against the latest config state.
- Routed CLI `provider fetch-models --channel` and HTTP channel-directed fetch through that shared workflow. The CLI's no-channel list path remains read-only.
- Moved channel model merging into `internal/domain/provider`; existing IDs/metadata and exposure are preserved, and duplicate/blank fetched IDs are ignored.
- Removed the direct profile config-write helper and its dead persistence file/error paths. Config shape and API surface are unchanged.

## Verification

| Check | Result |
|---|---|
| Focused root tests: `./cmd/pi-switch ./internal/server ./internal/application ./internal/profile ./internal/domain/provider ./internal/usecase/providers` | PASS |
| Focused root `go vet` for the same packages | PASS |
| `go run -mod=readonly ./internal/archcheck` | PASS — clean |
| `gofmt -d` on changed Go files; `git diff --check` | PASS — no output |
| `internal/profile` direct config-write / old `FetchChannelModels` scan | PASS — no matches |
| HTTP integration test changing profile models/exposure during the upstream request | PASS — concurrent changes and fetched models are both retained |

## Remaining W08 work and limits

- Review remaining `internal/application` and Desktop wiring for business orchestration that can safely move to the established usecases; retain workflows that still own draft/redaction or UI error semantics.
- Run final root/Desktop tests and vet, Secrets Scan, local Root/Desktop CI-equivalent checks, CLI/API/Proxy smoke, and compatibility audit against the final HEAD.
- No remote GitHub CI was triggered. Arch+niri/Wayland, fcitx5, and hardware scaling remain unverified; Ubuntu/Xvfb is not a substitute.
