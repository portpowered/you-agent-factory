---
author: ralph agent
last modified: 2026, april, 12
doc-id: AGF-DEV-005
---

# Dashboard UI Workflow Baseline

This baseline records the Agent Factory dashboard UI command surfaces before any additional Bun migration work. Use it to compare later validation and command-switch stories against the behavior that already exists in the repository.

## Current UI Commands

Run these commands from `libraries/agent-factory/ui`.

| Workflow | Current package script | Toolchain invoked |
| --- | --- | --- |
| TypeScript check | `bun run tsc` | `tsc -b --noEmit --pretty false` |
| Production build | `bun run build` | `tsc -b && vite build` |
| Vitest tests | `bun run test` | `vitest run` |
| Storybook static build | `bun run build-storybook` | `storybook build --loglevel warn` |
| Storybook interaction check | `bun run test-storybook` | Serves `storybook-static`, waits for `index.json`, then runs Storybook browser tests through Vitest |
| Dev server | `bun run dev` or package-manager equivalent | `vite` |

The UI package already declares `packageManager: bun@1.3.12` and has a checked-in `bun.lock`. The package scripts themselves call package-local CLIs and do not hard-code `npm` or `bun`; the package manager decides how those binaries are resolved.

## Repo Command Surfaces

Run these commands from `libraries/agent-factory`.

| Surface | Command | Current behavior |
| --- | --- | --- |
| `Makefile` | `make ui-build` | `cd ui && $(BUN) run build` |
| `Makefile` | `make ui-test` | `cd ui && $(BUN) run test` |
| `Makefile` | `make ui-storybook` | `cd ui && $(BUN) run build-storybook` |
| `Makefile` | `make ui-test-storybook` | `cd ui && $(BUN) run test-storybook` |
| `Makefile` | `make dashboard-verify` | Runs `ui-build`, then Go vet through `make lint`, then short Go tests through `make test` |
| `README.md` | Development commands | Points maintainers to `make dashboard-verify`, `make ui-build`, and `make ui-test` |
| `docs/live-dashboard.md` | Local frontend dev server | Historical baseline finding: before US-004 this guide still showed unsupported `npm install` and `npm run dev`; the canonical guide now uses Bun |
| `docs/live-dashboard.md` | Build and verification | Points maintainers to `make test`, `make ui-test`, and `make ui-build` |

No repo-root common command currently documents Agent Factory UI-specific build or test commands. The root common command guide only covers the root project, website, backend, API, CLI, and operations command surfaces.

## Embedded Asset Path

Production dashboard builds write assets to `libraries/agent-factory/ui/dist`. The Go embed path lives in `libraries/agent-factory/ui/embed.go` and embeds `dist` plus direct children with:

```go
//go:embed dist dist/*
```

The current production build emits `dist/index.html` and hashed assets under `dist/assets/`, which are visible to the `ui` Go package and the API server path that serves `/dashboard/ui`.

## Baseline Verification

Verified on 2026-04-12 from this worktree after `bun install --frozen-lockfile` in `libraries/agent-factory/ui`.

| Check | Result |
| --- | --- |
| `bun run tsc` | Passed |
| `bun run test` | Passed: 15 test files, 66 tests |
| `bun run build` | Passed and produced `ui/dist/index.html`, `ui/dist/assets/index-CiGZ_MhP.js`, and `ui/dist/assets/index-fX_CPwxP.css` |
| `bun run build-storybook` | Passed |
| `make ui-build` | Passed through the Bun-backed Make target |
| `make ui-test` | Passed through the Bun-backed Make target |
| `go test ./ui` | Passed; the Go embed package compiled against the generated `dist` path |
| `make lint` | Passed |
| `make test` | Passed |

On Windows, `make ui-build`, `make ui-test`, and `make test` printed intermittent `The process cannot access the file because it is being used by another process.` messages after successful command output while still returning exit code 0. Treat this as a follow-up observation rather than a failed baseline unless the process exits non-zero.
