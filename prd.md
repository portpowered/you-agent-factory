# PRD: Session Factory Save Modes (S1 Recovery)

## Introduction

**Customer ask:** Retry implementation of **Session factory save modes (S1 recovery)** after a network outage interrupted the original program batch. Deliver the approved S1 behavior so customers can submit factory definitions through one session-scoped save endpoint with explicit save modes.

**Concrete problem:** The runtime still exposes two overlapping submission paths: `POST /factories` (create named factory + activate under the service root) and `PUT /factory-sessions/{session_id}/factory` (replace the current factory for one live session). They differ in validation, concurrency, persistence roots, and activation. Dashboard import and editor saves cannot express “replace what I am editing” vs “create/upsert a named factory and make it current for this session” through one predictable contract. A prior autonomous batch did not land; this recovery batch starts from the current tree with **no save-mode implementation present**.

**High-level solution:** Unify factory submission behind `PUT /factory-sessions/{session_id}/factory` with a capital-cased **`mode`** enum (`REPLACE_CURRENT` default, `UPSERT_NAMED_AND_ACTIVATE` optional). Request body is `{ mode?, factory }` where `factory.version` lives inside `Factory`. Remove `POST /factories`. Backend implements one **scope → validate → persist → activate → readback** pipeline with mode handlers, always activating via session-scoped `replaceSessionRuntime`. Dashboard editor uses `REPLACE_CURRENT` only; import confirm dialog chooses mode per operator intent.

**Authoritative spec:** [`tasks/prd-session-factory-save-modes.md`](../prd-session-factory-save-modes.md) — when this recovery plan and that document differ, the approved PRD wins.

**Recovery note:** If a legacy Ralph/program token still references the aborted batch, operators may manually retire it; this recovery work item uses fresh story ids and does not assume partial commits from the failed run.

## Project-level acceptance criteria

- [ ] `PUT /factory-sessions/{session_id}/factory` is the **only** HTTP API for submitting a full factory definition to a live session, with `mode` + `factory` body and default `REPLACE_CURRENT`.
- [ ] `POST /factories` is removed from OpenAPI, generated Go/TS clients, UI, CLI live-server paths, and functional tests.
- [ ] Both save modes persist under the correct session root and swap only the targeted session runtime when idle; multi-session tests prove isolation.
- [ ] Dashboard editor Save and import confirm flows use session PUT with correct mode selection; no `POST /factories` callers remain.
- [ ] Version rules per mode match the approved PRD (`REPLACE_CURRENT` requires version; UPSERT create may omit version; UPSERT replace detects stale on-disk version).
- [ ] Existing error families remain mappable (`FACTORY_NOT_IDLE`, `STALE_FACTORY_VERSION`, `INVALID_FACTORY`, etc.).
- [ ] Quality gate: repository typecheck, lint, and targeted tests for changed behavior pass.

## Goals

- Route all live factory definition submission through session PUT with explicit save modes.
- Remove `POST /factories` and parallel backend save trees (`CreateNamedFactory`, unscoped `SaveCurrentFactory` on HTTP paths).
- Consolidate backend saves into one readable `SaveFactoryForSession` pipeline with two mode handlers and one session activation path.
- Keep graph/editor Save on `REPLACE_CURRENT` only.
- Give dashboard import a confirm dialog: replace current vs create new named (with suffixed name allocation on conflict).
- Preserve CLI offline filesystem save behavior; live CLI uses session PUT with default mode and `factory.version` on the body.

## User Stories

### session-factory-save-modes-recovery-001: Session factory PUT contract and client regeneration

**Description:** As an API consumer, I need OpenAPI and generated clients to describe save modes on session PUT so all callers share one typed contract.

**Acceptance Criteria:**

- [ ] `PUT /factory-sessions/{session_id}/factory` request body schema includes optional `mode` enum (`REPLACE_CURRENT` | `UPSERT_NAMED_AND_ACTIVATE`) and required `factory` (`Factory` with embedded `version` when applicable); omitted `mode` means `REPLACE_CURRENT`.
- [ ] `POST /factories` is removed from `api/openapi.yaml`; contract test forbids `paths./factories.post`.
- [ ] Bundled spec and generated Go (`pkg/api/generated`, `pkg/generatedclient`) and TS (`ui/src/api/generated`) clients are regenerated and compile.
- [ ] Typecheck passes
- [ ] Tests pass (OpenAPI contract / regeneration smoke as applicable)

### session-factory-save-modes-recovery-002: Unified backend save pipeline and mode handlers

**Description:** As a maintainer, I need one orchestrated session save pipeline so session-scoped saves never fork through global activation paths.

**Acceptance Criteria:**

- [ ] `FactoryService` exposes a single entrypoint (e.g. `SaveFactoryForSession(ctx, sessionID, mode, factory)`) implementing: resolve scope → validate → persist (mode handler) → activate session runtime → readback.
- [ ] `REPLACE_CURRENT` requires `factory.name` to match session current name; overwrites current slot (default root or named); stale/missing `factory.version` → `STALE_FACTORY_VERSION` when replacing a versioned definition.
- [ ] `UPSERT_NAMED_AND_ACTIVATE` persists under session root for `factory.name` (create or replace), updates current-factory pointer, activates via `replaceSessionRuntime` for that session only.
- [ ] Activation always uses `requireIdleRuntimeForSession` + `replaceSessionRuntime`; session saves do not call `activateReplacementRuntime` or infer session from global run state.
- [ ] HTTP handler for session PUT decodes `{ mode, factory }` and calls only the unified entrypoint; `POST /factories` handler removed.
- [ ] `CreateNamedFactory`, unscoped HTTP `SaveCurrentFactory`, and parallel save trees removed or inlined into the pipeline.
- [ ] Service test demonstrates saving session B does not mutate session A (extend or mirror `SaveCurrentFactoryForSession_ReplacesOnlyTargetedSession` for both modes).
- [ ] Typecheck passes
- [ ] Tests pass

### session-factory-save-modes-recovery-003: Version rules and error mapping per mode

**Description:** As a client author, I need deterministic version requirements and response versioning for each save mode.

**Acceptance Criteria:**

- [ ] `REPLACE_CURRENT` rejects when `factory.version` is missing or stale relative to the current definition for that session.
- [ ] `UPSERT_NAMED_AND_ACTIVATE` on create (name absent under session root): `factory.version` may be omitted; response returns server-minted initial version inside `factory`.
- [ ] `UPSERT_NAMED_AND_ACTIVATE` on replace of existing named factory: stale detection uses on-disk version for that name; success returns incremented `factory.version`.
- [ ] `UPSERT_NAMED_AND_ACTIVATE` does not return `FACTORY_ALREADY_EXISTS`; it replaces when idle.
- [ ] `FACTORY_NOT_IDLE` and `INVALID_FACTORY` still surface for unsafe or invalid payloads in either mode.
- [ ] Typecheck passes
- [ ] Tests pass

### session-factory-save-modes-recovery-004: Dashboard editor save uses REPLACE_CURRENT

**Description:** As a factory editor, I want Save to update the factory I am editing in the active tab without renaming or switching named factories.

**Acceptance Criteria:**

- [ ] `saveCurrentFactoryDocument` / editor save hooks send session PUT with `mode: "REPLACE_CURRENT"` (or omitted mode) and include `version` on the `factory` payload from the latest GET.
- [ ] No editor save path calls `UPSERT_NAMED_AND_ACTIVATE` or `POST /factories`.
- [ ] Stale-version and not-idle errors present the same operator-facing messages as before (mapped from existing error codes).
- [ ] Typecheck passes
- [ ] Tests pass (API module / hook unit tests)
- [ ] Verify in browser: edit graph, Save succeeds; forced stale version shows existing stale error UX

### session-factory-save-modes-recovery-005: Dashboard import confirm dialog and mode selection

**Description:** As a dashboard operator, I want import to offer replace-current vs create-new-named with safe naming when a conflict exists.

**Acceptance Criteria:**

- [ ] Import activation uses session-scoped PUT only (no `POST /factories`).
- [ ] Confirm dialog on import for any session tab with default **Replace current factory**:
  - Replace → `REPLACE_CURRENT` with `factory.name` from session GET (PNG embedded name ignored); includes `factory.version` from GET.
  - Create new named → `UPSERT_NAMED_AND_ACTIVATE` with name from PNG metadata (validated writable segment).
- [ ] Create path: if preferred name exists under session root, client picks first free suffixed name (`name`, `name-2`, `name-3`, …) and shows resolved name in dialog before PUT.
- [ ] Create path: omit `factory.version` when target name is new; include version when upserting an existing name the operator chose.
- [ ] Replace path never changes the session’s active factory name (including `UNDEFINED` default root overwrite).
- [ ] Typecheck passes
- [ ] Tests pass (dialog choice, suffix allocation, PUT body/mode per session; at least one non-default session tab scenario)
- [ ] Verify in browser: import on default and non-default tabs; both dialog choices reach success or expected error states

### session-factory-save-modes-recovery-006: Remove legacy create-factory callers in tests and UI

**Description:** As a maintainer, I want no remaining dependencies on `POST /factories` after migration.

**Acceptance Criteria:**

- [ ] `ui/src/api/named-factory/api.ts` removes `createFactory` POST usage; named create flows use session PUT with `UPSERT_NAMED_AND_ACTIVATE`.
- [ ] Functional tests in `tests/functional/runtime_api/factory_transformation/api_named_factory_test.go` and `tests/functional/bootstrap_portability/` use session PUT with explicit `mode`.
- [ ] Generated client no longer exposes `CreateFactory` for `/factories` POST.
- [ ] Typecheck passes
- [ ] Tests pass

### session-factory-save-modes-recovery-007: Live CLI uses session PUT with REPLACE_CURRENT

**Description:** As a CLI user, I want live `you factory save` to use the unified session PUT while offline filesystem save stays unchanged.

**Acceptance Criteria:**

- [ ] Live `you factory save` (no name argument) PUTs `{ factory }` (default `REPLACE_CURRENT`) to `sessionpath.CurrentFactoryPath` with `version` echoed from prior GET inside `factory`.
- [ ] Offline `SaveFromFile` / `WriteCurrentFactoryPointer` behavior unchanged.
- [ ] CLI help/examples remain accurate for live vs offline save.
- [ ] Typecheck passes
- [ ] Tests pass (`pkg/cli` factory save tests)

## Functional Requirements

- FR-1: Session PUT is the sole HTTP submit path for full factory definitions.
- FR-2: Body shape `{ mode?, factory }`; enum values SCREAMING_SNAKE; default `REPLACE_CURRENT`.
- FR-3: `REPLACE_CURRENT` requires name match to session current; does not change active factory name.
- FR-4: `UPSERT_NAMED_AND_ACTIVATE` persists under `SessionFactoryRootDir`, sets current-factory pointer, activates in-session only.
- FR-5: All modes require idle targeted session before persist/activate.
- FR-6: Shared validation runs before disk write (topology + normalization; may delegate to validation consolidation PRD when available).
- FR-7: `POST /factories` hard-removed (no deprecation shim).
- FR-8: Editor saves `REPLACE_CURRENT` only.
- FR-9: Import dialog per approved PRD US-005 decisions (default replace; suffixed names on conflict).
- FR-10: Error responses remain compatible with existing dashboard mapping where codes overlap.

## Non-Goals

- `ACTIVATE_ONLY` mode (pointer switch without new definition).
- Changing `POST /factory-sessions` open-session semantics.
- Offline CLI named-factory create gaining HTTP UPSERT in this recovery batch.
- Broad handler-file splits (`prd-api-handlers-decomposition`) or service composition refactors unless required for save pipeline extraction.
- Replacing `REPLACE_CURRENT` to change active factory name (use UPSERT instead).

## High-level technical design

### Backend pipeline (required shape)

```text
SaveFactoryForSession(ctx, sessionID, mode, factory)
  → resolveSessionFactoryScope
  → validateFactorySave
  → persistFactorySave        // mode handler
  → activatePersistedFactory  // replaceSessionRuntime only
  → readbackFactory
```

**Scope object** (resolve once): `SessionID`, `Session`, `RootDir`, `Current`, `CurrentName`, `IsDefaultRoot`.

**Mode handlers:**

| Mode | Persist |
|------|---------|
| `REPLACE_CURRENT` | Overwrite current slot (root `factory.json` or `ReplaceNamedFactory` for current name) |
| `UPSERT_NAMED_AND_ACTIVATE` | `PersistNamedFactory` or `ReplaceNamedFactory` under session root; set current-factory pointer |

Suggested package: `pkg/service/factorysave/` with `save.go`, `scope.go`, `validate.go`, `persist.go`, `modes.go`, `activate.go`, `readback.go`. `FactoryService` stays thin.

### Incremental refactor order

1. Extract scope + readback (no behavior change).
2. Route existing session save through shared validate + activate.
3. Add mode handlers + HTTP body decode; wire PUT.
4. Delete `CreateNamedFactory`, unscoped save HTTP paths, save-path `activateReplacementRuntime`.

### UI transport

- Extend `saveCurrentFactoryDocument` (or successor) to send `{ mode, factory }`.
- Import: `activateImportedFactoryForSession` and confirm UI choose mode; remove `/factories` POST constant.

### Dependencies

- **Soft:** [`tasks/prd-factory-validation-consolidation.md`](../prd-factory-validation-consolidation.md) — do not block; interim topology validation acceptable.
- **Downstream:** S7 UI client PRD may further consolidate transport after S1 lands.
- **Do not** add new `POST /factories` callers during implementation.

## Supporting technical and UX considerations

- Reuse disk helpers: `marshalPersistedFactoryPayload`, `replaceDefaultFactoryDefinition`, `factoryconfig.PersistNamedFactory`, `ReplaceNamedFactory`, `buildSessionEditableFactoryReplacement`, `replaceSessionRuntime`.
- UI `SaveCurrentFactoryInput.baseVersion` may remain an adapter writing into `factory.version` on the wire.
- Contract tests: drop `/factories` post; assert save mode on session PUT.
- Accessible import dialog: radio or equivalent with visible labels, keyboard operable, focus trap in modal.
- Loading/error states on import confirm should not double-submit PUT.

## Success metrics

- Zero `POST /factories` references in production UI/CLI paths and functional tests after closeout.
- Multi-session functional tests pass for both modes without cross-session disk or runtime mutation.
- Editor save and import share one client shape (`mode` + `factory`).
- Invalid payloads rejected consistently regardless of mode.

## Open questions

None — decisions are recorded in the approved S1 PRD (2026-05-30).
