# Architecture V2 W13 — architecture guard and CI coverage — 2026-10-14

**Status:** W13 accepted locally. W14 was not started. No production behavior or persisted format changed; no push was performed.

## Scope and decision

The existing `internal/archcheck` already covered the dependency matrix in `SPEC-ARCHITECTURE-V2.md`. No additional production dependency rule was justified without changing the planned migration boundary, so W13 adds only the missing regression/CI coverage:

- A root-level production Go file importing the separate `desktop` module is rejected.
- Imports that exist only in `_test.go` fixtures are ignored, preserving the intentional compatibility-test references.
- Desktop CI now runs the checker’s negative/clean fixture tests explicitly. Root CI already runs these tests through `go test ./...` and runs the production-tree checker directly.

## Enforced dependency rules

`go run ./internal/archcheck` scans production `.go` files under `cmd/`, `internal/`, `legacy/`, `webui/`, and the repository root:

- `internal/domain/**` may import only the standard library or `internal/domain/**`.
- `internal/usecase/**` may import only the standard library, `internal/domain/**`, or `internal/usecase/**`.
- `internal/adapters/inbound/**` may import external packages, `internal/domain/**`, `internal/usecase/**`, and its inbound subtree; it may not import outbound or other internal packages.
- `internal/adapters/outbound/**` may import external packages, `internal/domain/**`, `internal/usecase/**`, and its outbound subtree; it may not import inbound or other internal packages. The existing `profilevalidation -> internal/profile` compatibility bridge remains the only explicit legacy exception.
- Go files directly under `internal/adapters/` are rejected; adapters must be under `inbound/` or `outbound/`.
- `internal/bootstrap/**` remains the composition-root exception for internal wiring.
- Production code in the root module, including root-level files, may not import the separate `github.com/heihei0299/pi-switch/desktop` module.
- `_test.go` files are excluded from production dependency checking.

The checker’s existing negative fixtures continue to cover third-party imports in domain/usecase, usecase-to-adapter imports, inbound/outbound crossing, legacy-package bypasses, misplaced adapters, and Desktop imports from bootstrap/command/WebUI. W13 adds the root-level Desktop-import and test-only-import cases.

## CI coverage

- **Root CI (`.github/workflows/ci.yml`)**: runs `go run ./internal/archcheck` before Go vet/tests; `go test ./...` executes the checker’s fixture tests.
- **Desktop CI (`.github/workflows/desktop.yml`)**: runs `go run ./internal/archcheck` and now explicitly runs `go test ./internal/archcheck -count=1` before Desktop test/vet.

No second checker or parallel dependency mechanism was introduced.

## Local verification

All checks used the existing lockfiles/modules with `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off` where applicable.

| Check | Result |
|---|---|
| `go test -mod=readonly ./internal/archcheck -count=1 -v` | PASS — all clean/negative fixtures, including the two W13 cases |
| `go run -mod=readonly ./internal/archcheck` | PASS — `architecture dependencies: clean` |
| `go test -mod=readonly ./...` | PASS |
| `go vet -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 go -C desktop test -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 go -C desktop vet -mod=readonly ./...` | PASS |
| `go -C desktop run -mod=readonly github.com/egoist/mygo/cmd/mygo vet .` | PASS |
| `git diff --check` | PASS |
| Workflow coverage audit with `rg` | PASS — Root and Desktop both invoke the checker; Desktop also runs checker fixtures |

## Changes

- `internal/archcheck/main_test.go`: two focused regression/negative fixtures.
- `.github/workflows/desktop.yml`: explicit architecture-checker fixture test step.
- This report.

## Remaining risk

Remote GitHub Actions was not executed, and the local environment does not prove GitHub runner behavior or every matrix runner. The checker remains a source-import rule; it does not prove runtime write behavior or replace Go’s compiler cycle detection. No W14 work was started.
