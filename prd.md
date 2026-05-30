# PRD: Command Wiring with Wire (S8)

---
author: Codex
last modified: 2026-05-31
status: draft
---

## Context

### Customer ask

Introduce optional compile-time dependency injection with **`google/wire`** at command entrypoints after service composition stabilizes (S6). Full upstream spec: [`tasks/prd-cmd-wire-composition.md`](../prd-cmd-wire-composition.md). Wave placement: backend simplification **S8** (optional, wave 4).

### Problem

`FactoryService` construction today flows through `service.BuildFactoryService` and is invoked from `pkg/cli/run` via a package-level `buildFactoryService` hook. As S6 injects collaborators (`factorysave`, `runtimebuild`, `factorysessions`, `localmodels`, `hostedworkers`), manual wiring at the process edge risks duplication, ordering mistakes, and drift between the service build path and what operators actually run via `you run`.

The repository standard places **process wiring in `cmd/`**, but `cmd/factory/main.go` is currently a one-line delegate to `pkg/cli.Execute`. Wire cannot live only in `cmd/` unless the binary registers the generated injector into the CLI run path (Go forbids `pkg/` importing `cmd/`).

### High-level solution

Add a **`cmd/factory/composition`** package (build tag `wireinject` in `wire.go`, checked-in `wire_gen.go`) that generates the `FactoryService` build function from plain constructors in `pkg/service` and related packages—**no `wire:` struct tags in `pkg/service` production code**.

`cmd/factory/main.go` registers the wire-generated builder with `pkg/cli/run` before `cli.Execute()`. `you run` continues to parse flags, build `FactoryServiceConfig`, start the API server, and run the factory with **unchanged CLI flags, defaults, and HTTP behavior**. Tests keep overriding `buildFactoryService` without invoking Wire.

**Gate:** Land only after [`prd-service-composition-seams.md`](../prd-service-composition-seams.md) (S6) stabilizes collaborator constructors. If `service.BuildFactoryService` already accepts an explicit `Deps` struct that is easy to read, this PRD may be deferred without blocking other work.

## Introduction

This PRD is an **optional maintainability** improvement. It does **not** change product APIs, OpenAPI contracts, dashboard UI, or factory execution semantics. Success means contributors can extend the service dependency graph by editing a localized provider set and regenerating code, while operators see the same `you` CLI and local API server behavior as before.

## Project-level acceptance criteria

- [ ] **AC-1:** Wire usage is confined to `cmd/factory/composition/**`; `pkg/service`, `pkg/api`, and `pkg/cli` contain no `wireinject` build tags and no `wire:` struct tags in production code.
- [ ] **AC-2:** `you run` (and `go build -o you ./cmd/factory`) preserves existing CLI flags, help text, default port/bind behavior, and startup outcomes covered by `pkg/cli/run` tests.
- [ ] **AC-3:** The wire-generated factory builder is registered from `cmd/factory/main.go` before `cli.Execute()`; `pkg/cli/run` tests can still replace `buildFactoryService` without running `go generate`.
- [ ] **AC-4:** Checked-in `wire_gen.go` is produced by `go generate` documented for maintainers; generated files are not hand-edited.
- [ ] **AC-5:** Post-S6 collaborator wiring (sessions, factory save, runtime build, local models, hosted workers, logger/config inputs) is expressed in one provider set; adding a new injected collaborator does not require parallel manual edits in multiple entrypoints.
- [ ] **AC-6:** Existing integration and functional tests that exercise `you run` and the local API server pass without assertion weakening.
- [ ] **AC-7 (quality gate):** `go build ./...`, repository lint/typecheck surfaces, and targeted tests for `cmd/factory/composition`, `pkg/cli/run`, and `pkg/service` pass for all changed behavior.

## Goals

- Generate `wire_gen.go` for the factory CLI composition root under `cmd/factory/composition`.
- Keep `pkg/service` constructors plain Go; Wire only calls them from `cmd/`.
- Register the generated injector at process startup so `you run` uses it by default.
- Preserve all CLI and local HTTP server observable behavior.
- Document the regenerate-and-commit workflow for contributors.

## User Stories

### cmd-wire-composition-001: Factory service builder registration seam

**Description:** As a maintainer, I want the CLI run path to accept a factory service builder registered from `cmd/factory/main` so compile-time DI can live under `cmd/` without `pkg` importing `cmd`.

**Acceptance Criteria:**

- [ ] `pkg/cli/run` exports a registration function (for example `SetBuildFactoryService`) that assigns the package-level `buildFactoryService` used by `Run`; when unset, behavior matches today's default (`service.BuildFactoryService`).
- [ ] `pkg/cli/run` tests that override `buildFactoryService` continue to pass without calling the registration function.
- [ ] Typecheck passes
- [ ] Tests pass

### cmd-wire-composition-002: Wire toolchain and composition package bootstrap

**Description:** As a contributor, I want a checked-in Wire-generated injector in `cmd/factory/composition` so the factory binary has a single generated composition root.

**Acceptance Criteria:**

- [ ] `github.com/google/wire` is available to the repo (module `require` and/or `tool` directive as appropriate); `cmd/factory/composition/wire.go` uses `//go:build wireinject` and `//go:generate` to produce `wire_gen.go`.
- [ ] `wire_gen.go` is committed and builds without the `wireinject` tag; it exposes a function that builds `*service.FactoryService` from `context.Context` and `*service.FactoryServiceConfig` by delegating to existing `service.BuildFactoryService` (initial bootstrap graph).
- [ ] `go build -o /dev/null ./cmd/factory` succeeds on a clean checkout after `go generate ./cmd/factory/composition/...`.
- [ ] Typecheck passes
- [ ] Tests pass

### cmd-wire-composition-003: Register wire-built builder in factory main

**Description:** As an operator, I want `you run` to use the wire-generated factory builder by default so runtime behavior stays the same while composition moves to `cmd/`.

**Acceptance Criteria:**

- [ ] `cmd/factory/main.go` registers the composition package's generated builder via `pkg/cli/run` before `cli.Execute()`.
- [ ] `go test ./pkg/cli/run/...` passes with no intentional changes to flag parsing, auto-port, dashboard URL, or API server startup tests.
- [ ] A focused composition test (in `cmd/factory/composition` or `pkg/cli/run`) asserts the registered builder returns a non-nil `*service.FactoryService` for a minimal valid `FactoryServiceConfig` fixture (or returns the same error family as the direct `service.BuildFactoryService` call for an invalid fixture).
- [ ] Typecheck passes
- [ ] Tests pass

### cmd-wire-composition-004: Explicit collaborator provider set (post-S6)

**Description:** As a maintainer, I want Wire providers for each `FactoryService` collaborator so dependency order is visible in one place after S6 extraction.

**Acceptance Criteria:**

- [ ] `cmd/factory/composition/wire.go` defines providers for the S6 collaborators (at minimum: logger/config inputs, `factorysessions` registry, `factorysave`, `runtimebuild`, `localmodels`, `hostedworkers`) and assembles `*service.FactoryService` without duplicating business logic from `pkg/service`.
- [ ] The wire-generated build path remains behaviorally equivalent to `service.BuildFactoryService` for the same `FactoryServiceConfig` on fixtures covered by existing `pkg/service` build tests (equivalence asserted in `cmd/factory/composition` or `pkg/service` test, not by file inventory).
- [ ] `go test ./pkg/service/...` and `go test ./pkg/cli/run/...` pass without weakening assertions.
- [ ] Typecheck passes
- [ ] Tests pass

### cmd-wire-composition-005: Document Wire regeneration workflow

**Description:** As a contributor, I want clear instructions for regenerating composition code after changing providers.

**Acceptance Criteria:**

- [ ] `docs/internal/development/` (or the development guide) documents: install Wire CLI, run `go generate ./cmd/factory/composition/...`, commit `wire_gen.go`, and never hand-edit generated files.
- [ ] The doc states Wire is limited to `cmd/factory/composition` and that `pkg/service` stays tag-free.
- [ ] Typecheck passes

### cmd-wire-composition-006: Optional CI guard for stale wire_gen

**Description:** As a maintainer, I want CI to fail when `wire_gen.go` is out of date so generated composition does not drift silently.

**Acceptance Criteria:**

- [ ] A CI or `make` target runs `go generate ./cmd/factory/composition/...` and fails if `git diff --exit-code` shows changes to `wire_gen.go` (or documents why this check is intentionally skipped).
- [ ] When enabled, the check passes on a clean tree after regeneration.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional Requirements

- **FR-1:** Wire usage is limited to `cmd/factory/composition/**`; no `wire:` struct tags in `pkg/service` production code.
- **FR-2:** Generated files (`wire_gen.go`) are produced only by `go generate` and must not be hand-edited.
- **FR-3:** CLI commands, flags, help strings, and default values remain unchanged from pre-Wire behavior.
- **FR-4:** Local API server startup (`api.NewServer` with `apisurface.APISurface`) continues to receive the same `FactoryService` implementation type as today; HTTP routes, status codes, and JSON shapes are unchanged.
- **FR-5:** `pkg/cli/run` tests and service tests may construct services manually or via `buildFactoryService` overrides without importing Wire.
- **FR-6:** Post-S6, new collaborators are added by extending the provider set and regenerating, not by copying constructor blocks into multiple mains.

## Non-Goals

- `uber/fx` or other runtime DI containers.
- Wiring the React dashboard or any `ui/` code.
- Refactoring `service.BuildFactoryService` collaborator extraction (S6 scope).
- Changing OpenAPI, HTTP routes, or CLI command surfaces.
- Replacing test harnesses (`testutil`, functional tests) with Wire.
- Mandatory CI wire check if it blocks contributor environments without Wire installed (story 006 may document skip).

## High-level technical design

### Composition boundary

```text
cmd/factory/main.go
  → composition.InitializeRun()  // wire_gen: register buildFactoryService
  → pkg/cli.Execute()
       → pkg/cli/run.Run
            → buildFactoryService(ctx, FactoryServiceConfig)
            → startAPIServer(apisurface.APISurface, port, logger)
```

Go import rule: `pkg/cli/run` cannot import `cmd/factory/composition`. Registration from `main` is required.

### Provider layering

1. **Bootstrap (story 002–003):** Single injector calling `service.BuildFactoryService`.
2. **Post-S6 (story 004):** Providers call exported constructors/`Deps` from `pkg/service`, `pkg/service/factorysave`, `pkg/service/runtimebuild`, `pkg/factorysessions`, `pkg/localmodels`, `pkg/hostedworkers`—mirroring order already enforced inside `BuildFactoryService`.

### Verification surfaces

| Behavior | Evidence |
|----------|----------|
| CLI run unchanged | `go test ./pkg/cli/run/...` |
| Service build unchanged | `go test ./pkg/service/...` (existing build tests) |
| Composition equivalence | New focused test in `cmd/factory/composition` |
| End-to-end | Existing functional/smoke paths that start `you run` (no new inventory tests) |

## Supporting technical considerations

- **Upstream:** [`prd-service-composition-seams.md`](../prd-service-composition-seams.md) (hard dependency for story 004).
- **Existing hook:** `pkg/cli/run` already uses `var buildFactoryService` for test doubles; registration formalizes the production injection point.
- **Binary layout:** Only `cmd/factory` exists today; other `cmd/*` tools are maint checks and do not need Wire unless a future binary builds `FactoryService`.
- **Tooling:** Follow repo patterns for `go tool` / `go generate` (see `pkg/api/server.go` codegen).

## Success metrics

- `cmd/factory/main.go` and `cmd/factory/composition` remain the only places that know the full collaborator graph.
- Adding a collaborator after S6 is a provider-set change plus `go generate`, not a hunt through CLI and service files.
- No operator-visible regression in `you run` startup, dashboard URL emission, or local API availability in existing tests.

## Open Questions

- None blocking planning. **Defer entire PRD** if S6 lands with a small, readable `Deps` struct and the team agrees manual wiring is sufficient.

## Related documents

- [`tasks/prd-cmd-wire-composition.md`](../prd-cmd-wire-composition.md) — upstream draft
- [`tasks/prd-service-composition-seams.md`](../prd-service-composition-seams.md) — S6 prerequisite
- [`tasks/dependence-graph-for-prds.md`](../dependence-graph-for-prds.md) — S6 → S8 ordering
