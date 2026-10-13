# Architecture V2 W05a — Inference route selection — 2026-10-13

**Status:** W05a route-selection slice passed locally on top of W04 `9dd1eb2`. This does not complete W05; protocol planning, outbound coordination, response/usage handling, and streaming remain for subsequent W05 slices. No push or remote CI run.

## Implementation

- Moved globally unique bare-model resolution into `internal/usecase/inference.ResolveRoute`, using the domain config model and a typed result (`Candidates`, `Model`, `Channel`, `Ambiguous`).
- The `/v1` handler now maps that result to the existing 502 error envelopes. Slash-qualified model rejection, no-route behavior, current-profile independence, and ambiguity across suppliers or channels remain unchanged.
- Moved resolver unit coverage to the inference usecase seam. Existing server integration coverage still exercises the public ambiguous/no-route behavior.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (`desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root and `desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

## Remaining limits

- The rest of the inference pipeline is intentionally not claimed complete in W05a and remains required by the plan.
- No live upstream, user config, request history, manual Proxy smoke, or remote CI was used.
