# WP-08 WebUI retirement assessment — 2026-10-08

**Status: DEFERRED. No WebUI code, dependency, route, or workflow was removed.**

## Evidence

- WP-06 report: Gate C is not passed; Gateway reads real config/models and Stats uses the configured service with isolated fixture tests, but the JSON/Gateway conflict workflow, original Stats charts/failure details, Packages parity and physical GUI evidence remain incomplete.
- WP-07 report: Gate D is not passed; Linux tarball install/reinstall/uninstall, isolated data-preservation, hostile-archive rejection and six-target CLI identity checks now have local evidence, but the GitHub workflow has not run and Debian package-manager transactions, daemon lifecycle, cross-platform GUI and signing remain unverified.
- Current code still serves the embedded WebUI from `webui/embed.go` through `internal/server` and uses that router for the MyGo custom-scheme page. CLI `webui` and npm package `webui/dist` remain supported delivery paths.

## Decision

Do not retire React/Vite/WebView or its dependencies. Reopen only after Gate C and Gate D are passed with evidence, then write an ADR deciding whether CLI/npm continues to ship the browser UI. This is a deferral, not a permanent decision to keep or remove it.

## Verification

- `git diff --check`: PASS.
- No product code or build configuration changed; no product tests were run.
- The final full repository test suite remains deferred until the requested work packages are complete.
