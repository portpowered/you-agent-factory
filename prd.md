# PRD: Model Provider And Runner Grammar Convergence

## Introduction

The factory grammar currently exposes both `modelProvider` and `runner` for what customers experience as one choice: the execution family used for model-backed work, such as Codex, Gemini, Kiro, Cursor CLI, OpenCode, or Claude-backed execution. The duplicate vocabulary creates confusing precedence behavior, duplicate validation rules, and noisy API, docs, event, and dashboard surfaces.

This project makes `modelProvider` the canonical public and internal term for execution-family selection. New factory and workstation configuration must use `modelProvider`, the authored `runner` vocabulary must be retired, runtime dispatch metadata must report provider terminology, and generated contracts, docs, examples, events, replay behavior, and dashboard projections must remain aligned. The `modelProvider` grammar also gains a symbolic `DEFAULT` value that defers to the next configured provider in the finalized precedence chain and must resolve to a concrete provider before execution.

## Context

### Customer ask

Converge the public and internal grammar on `modelProvider`, delete the `runner` configuration vocabulary and associated runtime selection concepts, add a public `DEFAULT` provider value, preserve clear migration behavior for old configs and replay artifacts, and align OpenAPI, generated code, runtime selection, events, docs, examples, validation, replay compatibility, and dashboard/API projections.

### Problem

- Customers see both `modelProvider` and `runner` even though both describe execution-family selection.
- Factory-level and workstation-level `runner` fields create unclear precedence against worker `modelProvider`.
- API clients, dashboard projections, events, docs, and examples expose runner-specific names that customers must learn even when selecting a model provider.
- Existing configs and replay artifacts may contain old `runner` or `runnerId` fields, so the migration needs deterministic failure or compatibility behavior.
- Provider capability and prerequisite checks currently depend on runner-oriented identity, which risks behavior regressions if the vocabulary is renamed without collapsing the selection path.

### Solution

Define `modelProvider` as the only canonical execution-family selector at factory, workstation, worker, runtime dispatch, event, API, and dashboard projection boundaries. Accept `DEFAULT` as a symbolic public `modelProvider` value. Resolve dispatch provider selection in this order: concrete workstation provider, concrete factory provider, concrete worker provider, then operator default. A missing value or `DEFAULT` at a scope defers to the next step; execution, provider diagnostics, prerequisite checks, and capability checks receive only a concrete resolved provider.

New public config rejects `runner` with field-specific migration guidance and never persists or emits `runner`. Old event/replay artifacts with `runnerId` are supported through an explicit replay compatibility adapter that maps legacy metadata into the new provider metadata for historical inspection; new events and API responses use `modelProvider` only.

## Project-Level Acceptance Criteria

- [ ] **PAC-1:** New authored factory and workstation configuration accepts `modelProvider`, accepts `DEFAULT`, rejects `runner` with exact field-path guidance, and never emits `runner` during flatten, expand, save, export, or API responses.
- [ ] **PAC-2:** Runtime dispatch provider selection follows the finalized precedence order: concrete workstation provider, concrete factory provider, concrete worker provider, then operator default; `DEFAULT` and absent values defer and are resolved before execution.
- [ ] **PAC-3:** Provider prerequisite, capability, OpenCode-agent, command construction, and dispatch lifecycle behavior remain at least as strict as the current runner-backed behavior.
- [ ] **PAC-4:** Public OpenAPI schemas, generated Go/TypeScript clients, dispatch event metadata, replay projections, factory world views, and dashboard/provider summaries expose `modelProvider` terminology instead of runner terminology for execution family.
- [ ] **PAC-5:** Existing examples, fixtures, packaged reference docs, CLI help/docs, and sparse architecture notes teach `modelProvider` as canonical and mention `runner` only in migration guidance.
- [ ] **PAC-6:** Old replay artifacts containing `runnerId` replay through a documented compatibility adapter, while old authored configs containing `runner` fail with actionable errors unless an explicit migration command is added later.
- [ ] **PAC-7 (quality gate):** `make generate-api`, focused Go/API/replay/config/runtime tests, docs smoke where relevant, UI typecheck/build for generated TypeScript changes, lint, and the narrowest applicable test suites pass.

## Goals

- Make `modelProvider` the single canonical term for model execution-family selection.
- Remove authored `runner` fields from factory and workstation config.
- Add `DEFAULT` as a symbolic public `modelProvider` value and resolve it to a concrete provider before execution.
- Align generated OpenAPI contracts, handwritten Go types, UI generated clients, events, replay, projections, docs, examples, and CLI language.
- Preserve deterministic migration behavior for old configs and historical replay artifacts.
- Keep provider capability, prerequisite, and OpenCode-agent validation behavior intact.

## User Stories

### prd-model-provider-runner-convergence-001: Accept factory-level model provider defaults
**Description:** As a factory author, I want factory-level execution-family defaults to use `modelProvider` so factory config has one canonical provider concept.

**Acceptance Criteria:**
- [ ] A factory config can set top-level `modelProvider` to a concrete provider value and workers/workstations without a concrete provider resolve through that default.
- [ ] A factory config can set top-level `modelProvider: DEFAULT`, and dispatch resolution defers to worker provider and then operator default.
- [ ] Factory config decode rejects top-level `runner` with a clear field-path message such as `factory.runner is retired; use factory.modelProvider`.
- [ ] Factory config flatten, expand, save, and export output include `modelProvider` when needed and never include `runner`.
- [ ] Existing factory examples and config fixtures that previously used factory-level `runner` are migrated to `modelProvider`.
- [ ] Tests cover factory-level concrete provider, `DEFAULT`, absent provider fallback, rejected `runner`, and no `runner` in emitted config.
- [ ] Typecheck passes
- [ ] Tests pass

### prd-model-provider-runner-convergence-002: Accept workstation-level model provider overrides
**Description:** As a factory author, I want workstation-level execution overrides to use `modelProvider` so workstation grammar matches factory and worker grammar.

**Acceptance Criteria:**
- [ ] A workstation config can set `modelProvider` to a concrete provider value, and dispatches from that workstation use it ahead of factory, worker, and operator defaults.
- [ ] A workstation config can set `modelProvider: DEFAULT`, and dispatch resolution defers to concrete factory provider, then concrete worker provider, then operator default.
- [ ] Workstation config decode rejects `runner` with a clear field-path message such as `factory.workstations[id].runner is retired; use factory.workstations[id].modelProvider`.
- [ ] Workstation OpenCode-agent validation uses the effective resolved provider and accepts OpenCode-agent settings only when the resolved provider is OpenCode.
- [ ] Existing workstation examples and fixtures use `modelProvider` and no longer use `runner`.
- [ ] Tests cover concrete workstation override, workstation `DEFAULT`, factory fallback, worker fallback, operator fallback, rejected workstation `runner`, and OpenCode-agent validation.
- [ ] Typecheck passes
- [ ] Tests pass

### prd-model-provider-runner-convergence-003: Resolve runtime provider selection directly
**Description:** As a maintainer, I want runtime selection and validation to resolve model providers directly so execution behavior no longer depends on runner aliases.

**Acceptance Criteria:**
- [ ] Provider selection diagnostics report the selected concrete provider and whether the value came from workstation, factory, worker, operator default, or symbolic `DEFAULT` deferral.
- [ ] Runtime dispatch, provider command construction, provider prerequisite checks, and provider diagnostics receive a concrete `modelProvider` value before execution starts.
- [ ] Capability checks for image input, worktree support, working directory support, structured output, and session resume match the current behavior for each provider.
- [ ] Built-in provider prerequisite checks map from canonical provider values rather than runner IDs.
- [ ] Runner-oriented selection concepts are absent from active config/runtime selection paths except for an intentionally documented compatibility adapter.
- [ ] Unit tests cover provider precedence, diagnostic source metadata, capability checks, prerequisite checks, and concrete-provider enforcement before execution.
- [ ] Typecheck passes
- [ ] Tests pass

### prd-model-provider-runner-convergence-004: Publish provider terminology in API contracts and generated clients
**Description:** As an API client maintainer, I want the public contract to expose model-provider terminology so clients do not depend on runner vocabulary.

**Acceptance Criteria:**
- [ ] Authored OpenAPI schemas for factory, workstation, dispatch request metadata, dispatch summaries, world views, and event metadata expose `modelProvider` where the value describes execution family.
- [ ] Authored public OpenAPI schemas remove `runner`, `runnerId`, `runnerSelectionSource`, and `RunnerID` where they only represented model provider selection.
- [ ] The bundled OpenAPI contract and generated Go and TypeScript clients are regenerated from the authored contract.
- [ ] Contract tests prove generated public factory and workstation shapes do not include authored `runner` fields.
- [ ] API boundary tests prove dispatch metadata contains provider/model metadata using `modelProvider` terminology.
- [ ] Existing UI API adapters compile against the regenerated TypeScript types without reintroducing runner fields.
- [ ] Typecheck passes
- [ ] Tests pass

### prd-model-provider-runner-convergence-005: Emit and replay provider-based dispatch metadata
**Description:** As a dashboard and replay consumer, I want dispatch events and projections to report the selected model provider so historical and live views use one vocabulary.

**Acceptance Criteria:**
- [ ] New dispatch events record `modelProvider` and provider-selection source metadata instead of `runnerId` and runner-selection source metadata.
- [ ] Factory world views, dispatch summaries, and replay projections expose provider/model metadata and do not expose runner-specific config fields.
- [ ] Old replay artifacts with `runnerId` replay through an explicit compatibility adapter that maps legacy values to `modelProvider` for historical inspection.
- [ ] Replay of unsupported legacy provider values fails with a documented unsupported-version or unknown-provider error that names the legacy field.
- [ ] Dashboard runtime projections continue to show provider and model metadata for live and replayed dispatches.
- [ ] Focused replay and projection tests cover new provider metadata, legacy `runnerId` compatibility, unknown legacy value failure, and dashboard projection output.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill: run or replay a dispatch and confirm the dashboard shows provider/model metadata without runner labels.

### prd-model-provider-runner-convergence-006: Provide deterministic migration guidance
**Description:** As an operator with existing configs or artifacts, I want old runner usage to produce predictable outcomes so I know whether to edit config or rely on replay compatibility.

**Acceptance Criteria:**
- [ ] Public config validation rejects old `runner` fields at factory and workstation paths with replacement guidance and does not silently migrate or persist them.
- [ ] CLI validation and API validation return actionable errors for retired `runner` fields using the same field-path language.
- [ ] Release notes and packaged docs state that old authored configs require manual replacement of `runner` with `modelProvider`.
- [ ] Replay compatibility docs state that old event metadata with `runnerId` is supported only through the replay adapter and new events use `modelProvider`.
- [ ] If a migration command or helper is introduced, its output emits only `modelProvider`; otherwise the docs explicitly state no automatic config migration command exists.
- [ ] Tests cover CLI/API validation error shape for old config paths and replay compatibility behavior for old event metadata.
- [ ] Typecheck passes
- [ ] Tests pass

### prd-model-provider-runner-convergence-007: Update shipped docs, examples, and help text
**Description:** As a user, I want shipped docs, examples, and CLI help to use one term so I do not learn obsolete runner vocabulary.

**Acceptance Criteria:**
- [ ] Packaged reference docs for config, workers, workstations, models, and invocation describe `modelProvider` as the execution-family selector and document `DEFAULT` resolution.
- [ ] Workstation docs describe provider override behavior and the finalized precedence order without recommending `runner`.
- [ ] CLI help and docs remove runner terminology except for migration notes that point to `modelProvider`.
- [ ] Architecture docs receive a sparse note that `modelProvider` is the canonical execution-family selector and `model` remains separate model-name selection.
- [ ] Examples and functional fixtures use `modelProvider`, including at least one factory-level default and one workstation-level override.
- [ ] Docs/reference smoke or equivalent packaged-doc verification passes where existing docs tests cover these topics.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional Requirements

- **FR-1:** `modelProvider` must be the canonical user-facing field for execution-family selection at every config level that supports provider selection.
- **FR-2:** Public `modelProvider` values must include the concrete supported providers and symbolic `DEFAULT`.
- **FR-3:** `DEFAULT` must be symbolic and must not be treated as a provider command alias.
- **FR-4:** Dispatch provider resolution must choose the first concrete value in this order: workstation, factory, worker, operator default.
- **FR-5:** A missing provider value or `DEFAULT` at any scope must defer to the next resolution step.
- **FR-6:** Provider resolution must produce a concrete provider before command execution, capability validation, prerequisite checks, OpenCode-agent validation, and diagnostics that require a concrete provider.
- **FR-7:** Public config decode must reject retired `runner` fields at factory and workstation boundaries with actionable field-path replacement guidance.
- **FR-8:** Config flatten, expand, save, export, API responses, and new event output must not emit `runner`, `runnerId`, or runner-selection source fields for provider selection.
- **FR-9:** OpenAPI authored sources and generated Go/TypeScript clients must expose provider terminology consistently.
- **FR-10:** Dispatch metadata, world views, replay projections, and dashboard projections must report selected provider/model metadata without requiring consumers to understand runner aliases.
- **FR-11:** Provider capability and prerequisite checks must remain at least as strict as current runner-backed checks.
- **FR-12:** OpenCode-agent configuration must remain valid only when the effective resolved provider is OpenCode.
- **FR-13:** Old replay artifacts with `runnerId` must either replay through the compatibility adapter or fail with a documented unsupported-version/unknown-provider error.
- **FR-14:** Docs, examples, fixtures, CLI help, and packaged reference topics must teach `modelProvider` and mention `runner` only as retired migration vocabulary.

## Non-Goals

- Do not change provider command implementations themselves.
- Do not remove or rename model name selection; `model` remains distinct from `modelProvider`.
- Do not introduce new dashboard configuration editing beyond changes required to keep existing UI projections compiling and displaying provider metadata.
- Do not silently persist old `runner` fields in new configs.
- Do not add unrelated dynamic workflow policy, hosted runtime, model installation, or provider command behavior.
- Do not require tests that merely inventory source files or generated asset internals unless that structure is the product behavior under test.

## High-Level Technical Design

### Public contract and config grammar

Author OpenAPI changes in the component fragments and main entrypoint, then regenerate bundled OpenAPI and generated clients. Config loading should normalize public enum values into the internal provider representation and include `DEFAULT` as a symbolic value. Strict raw-input decoding should detect retired `runner` fields before they disappear into typed structs so errors can name the exact field path and replacement.

### Provider resolution model

Provider resolution is an explicit runtime operation that accepts workstation, factory, worker, and operator-default candidates. It records source metadata for diagnostics while returning a concrete provider for execution. `DEFAULT` and absent values are deferrals, not concrete choices. The resolved concrete provider owns prerequisite checks, capability checks, OpenCode-agent validation, provider command selection, dispatch metadata, and event emission.

### Events, replay, and projections

New events and projections use `modelProvider` and provider-selection source names. Replay has a deliberately small compatibility adapter for historical event metadata with `runnerId`; the adapter maps known legacy values to provider values for historical inspection and rejects unknown legacy values with a documented error. Compatibility is replay-only and does not permit new config or new event output to emit runner fields.

### Dashboard and generated TypeScript

The frontend consumes regenerated OpenAPI types. Dashboard projections should treat provider/model metadata as typed API data and render existing provider/model displays without runner labels. Loading, empty, error, and success states should follow existing dashboard projection behavior; no new configuration editor is in scope.

### Verification surfaces

Use focused config tests for decode, normalization, rejection, flatten, and expand behavior; runtime tests for precedence and concrete-provider enforcement; API contract tests for generated shapes and response metadata; replay/projection tests for new and legacy event metadata; functional smoke for factory default and workstation override dispatch; docs smoke for packaged reference topics; and UI typecheck/build plus browser verification for visible dashboard metadata changes.

## Supporting Technical And UX Considerations

- Public enum spelling should be canonical and documented before implementation. Public values should include `DEFAULT` and concrete provider values such as `CODEX`, `CLAUDE`, `GEMINI`, `KIRO_CLI`, `CURSOR_CLI`, and `OPENCODE`, with normalization to internal command values where needed.
- Diagnostics should distinguish an explicit concrete provider from a concrete provider reached after one or more `DEFAULT` deferrals.
- Error messages should be short, field-specific, and consistent across CLI and API validation.
- Generated artifacts must be updated from source contracts, not hand-edited.
- Public docs should avoid the internal Petri-net vocabulary and should describe provider selection in customer-facing factory terms.

## Success Metrics

- Searching shipped customer-facing config docs for `runner` guidance returns only migration notes.
- New factory configs can express execution-family defaults and overrides entirely with `modelProvider`.
- Dispatch metadata exposed to API and dashboard consumers uses model-provider terminology.
- Old `runner` config paths fail deterministically with field-specific replacement guidance.
- Old replay artifacts with known `runnerId` metadata remain inspectable through the compatibility adapter.
- Provider capability, prerequisite, and OpenCode-agent validation regressions are caught by focused tests before merge.

## Open Questions

- None. This PRD resolves the migration policy as strict rejection for new authored configs and replay-only compatibility for old event metadata.
