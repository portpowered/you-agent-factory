# Website Branding Rename Scope Inventory

This note records the remaining legacy-brand references intentionally left unchanged by the website-only rename to `You Agent Factory`.

## In Scope for This PRD

The completed stories changed customer-facing website surfaces under `ui/`, the checked-in fallback shell contract, and browser-backed website regression coverage that proves those surfaces.

## Intentionally Out of Scope

The following references still contain `infinite-you`, `Infinite You`, or `finite you`, but they were not changed in this PRD because they are not customer-facing website-shell branding:

| Path | Remaining reference | Why it stays unchanged in this PRD |
| --- | --- | --- |
| `go.mod` and Go import paths across `pkg/`, `cmd/`, `tests/`, and `internal/` | `github.com/portpowered/infinite-you` | Module and package import paths are backend or repository identity, not website-shell branding. Renaming them would be a repo-wide API and tooling migration. |
| `README.md`, release install links, and CLI docs or tests such as `pkg/cli/root.go` and `tests/functional/smoke/cli_docs_smoke_test.go` | `infinite-you` binary, docs command, and release URLs | These belong to CLI and release-distribution naming, not the in-browser website shell covered by this PRD. |
| `docs/comparatives/comparing-systems.md` | historical product comparisons that mention `infinite you` | This is long-form product documentation outside the website shell and would need a separate content review rather than a shell-branding rename. |
| `factory/logs/` and integration fixtures such as `ui/integration/fixtures/terminal-summary-regression-replay.jsonl` | captured worktree paths or historical runtime payloads containing `infinite-you` | These are recorded artifacts and fixture payloads. Rewriting them in this PRD would widen scope into fixture-history maintenance. |
| UI tests or data that use `infinite-you` as a session, slug, or machine identifier, such as `ui/src/features/header/components/dashboard-session-tabs.test.tsx` | non-visible identifier values | These are test-only identifiers rather than customer-facing branding strings. |

## Follow-up Guidance

If a future rename explicitly targets repository identity, CLI naming, docs content, or historical fixtures, treat that as a separate migration with its own audit. Do not fold those changes into website-shell branding work unless the PRD expands scope on purpose.
