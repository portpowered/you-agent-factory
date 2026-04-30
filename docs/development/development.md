# Agent Factory Development Guide

This guide is the contributor guide for the Agent Factory repository rooted at this checkout. Read it before changing runtime behavior, dashboard assets, workflow fixtures, or maintainer documentation, then use the local [docs index](../README.md) and [standards index](../standards/STANDARDS.md) for the current shared guidance.

## Purpose

`agent-factory` is the Coloured Petri Net workflow engine for orchestrating AI agent and script work. It owns runtime scheduling, worker dispatch, failure and retry behavior, replay, the HTTP API, and the embedded dashboard shell.

## Local Architecture

- `cmd/factory/` is the CLI binary entrypoint.
- `pkg/cli/` owns Cobra routing, command-specific packages (`run`, `config`, `submit`, `default`, and `init`), and the CLI dashboard read models in `dashboard`.
- `pkg/factory/` owns runtime engine behavior, scheduling, markings, transitions, resources, and engine state snapshots.
- `pkg/service/` wires the runtime, configuration, API server, replay, logging, and worker construction.
- `pkg/api/` serves runtime HTTP endpoints and the embedded dashboard shell.
- `pkg/workers/` owns worker execution contracts, provider calls, script command execution, and work-scoped metadata.
- `pkg/replay/` owns record/replay artifact construction, side-effect matching, and deterministic replay behavior.
- `ui/` is the Vite dashboard source. `ui/dist/` contains committed embedded assets.
- `tests/functional_test/` contains workflow fixtures and smoke coverage.

## Development Commands

Run commands from the repository root.

```bash
make build
make generate-api
make api-smoke
make test
make test-full
make release-surface-smoke
make lint
make script-timeout-companion-smoke-100
make current-factory-watcher-switch-smoke
make fmt
make dashboard-verify
make ui-deps
make ui-build
make ui-test
make ui-storybook
make ui-test-storybook
```

Use `make dashboard-verify` for dashboard review readiness after UI source changes that affect embedded assets. It runs `ui-build`, `lint`, and the short Go test suite sequentially so Vite asset rotation does not race with Go embed scanning.

`make lint` runs `go vet ./...` and the pinned deadcode analyzer. The deadcode step writes a normalized current report to `bin/deadcode-current.txt` and compares it with `docs/development/deadcode-baseline.txt`. Review any drift before updating the baseline.

`make release-surface-smoke` is the standalone-release readiness check for customer-facing cleanup work. It reruns the product-agnostic functional smoke suite against the package README, checked-in starter, shipped examples and sample payloads, verifies generated `agent-factory init` output stays neutral, and finishes with `go run ./cmd/publicsurfacecheck` so release-facing Port OS coupling cannot silently return.

`make artifact-contract-closeout` is the root factory artifact contract closeout for this cleanup lane. Use it after touching the root starter inventory, release-surface smoke coverage, or the targeted `pkg/api`, `pkg/config`, `pkg/replay`, `tests/adhoc`, and `tests/functional_test` contract tests. It first proves the checked-in inventory doc still matches the enforced classifications, then reruns `make release-surface-smoke`, and finally reruns the targeted package bundle that stabilized this repository contract.

Use `make ui-storybook` followed by `make ui-test-storybook` when dashboard Storybook stories, play functions, runtime mocks, or the package-local Storybook runner change. `ui-storybook` builds `ui/storybook-static`; `ui-test-storybook` serves that static build on the dashboard-owned runner port and executes the dashboard Storybook interaction tests through the UI package's `test-storybook` script.

## API Contract Generation

The authored JSON API contract source is `api/openapi-main.yaml` plus referenced fragments such as `api/components/schemas/events/`. Keep the shared event envelope, context, enum, and helper schemas under `api/components/schemas/events/`, and add supported public event payload fragments under `api/components/schemas/events/payloads/`. The checked-in `api/openapi.yaml` remains the bundled published artifact consumed by code generation, tests, and downstream readers. This standardization pass preserves the existing runtime API behavior; future route removals, renamed fields, pagination redesigns, or response fields changes need separate PRD/story scope before editing handlers or generated contracts for that redesign.

From a clean checkout, use this workflow after editing `api/openapi-main.yaml` or a referenced fragment:

1. Validate the authored source tree from the repository root:

```bash
cd api && node run-quiet-api-command.js validate:main ../api/openapi-main.yaml
```

2. Rebundle the published contract from the authored source tree:

```bash
make bundle-api
```

This uses the repository-supported Redocly CLI from the root `api/` workspace and rewrites `api/openapi.yaml` from `api/openapi-main.yaml` without manual post-processing. The package-local targets intentionally execute from `api/` so Redocly picks up the repository-owned `api/redocly.yaml` configuration instead of falling back to CLI defaults.

3. Regenerate the checked-in Go server interface, model types, and UI OpenAPI types from the bundled contract:

```bash
make generate-api
```

`make generate-api` rebundles `api/openapi.yaml`, then runs `go generate -tags=interfaces ./pkg/api`, which uses `api/codegen_config/server.yaml` and writes `pkg/api/generated/server.gen.go`. It also runs the dashboard UI OpenAPI generator and writes `ui/src/api/generated/openapi.ts`.

4. Prove regeneration is stable when the authored sources are unchanged:

```bash
make api-smoke
```

`make api-smoke` validates `api/openapi-main.yaml`, runs `make generate-api` twice from the split-source tree, verifies `api/openapi.yaml`, `pkg/api/generated/server.gen.go`, and `ui/src/api/generated/openapi.ts` are clean with `git diff --exit-code`, runs the focused bundled event-contract completeness guard from `pkg/api/openapi_contract_test.go`, and then runs the generated-contract live API smoke test across supported work, status, and event routes without requiring live LLM provider credentials.

5. Run the focused API and package checks that cover the contract boundary:

```bash
go test ./pkg/api -count=1
make test
make lint
```

Review any generated diff together with the authored OpenAPI change. Do not hand-edit `api/openapi.yaml`, `pkg/api/generated/server.gen.go`, or `ui/src/api/generated/openapi.ts`; change `api/openapi-main.yaml` or a referenced fragment, then regenerate.

## Factory Sharing Contract

The canonical export/import sharing boundary is the generated OpenAPI
`NamedFactory` payload returned by `GET /factory/~current` and accepted by
`POST /factory`. PNG export and import reuse that exact payload through
`PortOSFactoryPngEnvelope`, which adds only `schemaVersion`.

Use [Named Factory API Contract Data Model](named-factory-api-contract-data-model.md)
as the detailed contract reference. Keep the dashboard on the authored API
boundary. Do not rebuild sharing payloads from `/events`, runtime-only
projections, or export-only field aliases.

## Package-Specific Verification

1. Run `make test` for normal Go changes; the short suite skips stress tests.
2. Run `make test-full` when changing scheduler behavior, retry logic, stress-sensitive runtime code, or failure cascades.
3. Run `make release-surface-smoke` after changing release-facing README content, shipped docs/examples, checked-in starter content, or `agent-factory init` defaults. It is the canonical smoke path for proving Agent Factory still reads as a standalone library across those public surfaces.
4. Run `make script-timeout-companion-smoke-100` after changing script timeout, requeue, command-runner, or companion smoke behavior. The target runs `TestIntegrationSmoke_ScriptTimeoutCompanionRequeuesBeforeLaterCompletion` 100 consecutive times through the real timeout/requeue/later-completion flow and fails on the first run that misses the direct timeout signal, retry dispatch, requeue mutation, or final completion.
5. Run `make current-factory-watcher-switch-smoke` after changing current-factory activation, watched-input listener ownership, or service-mode watcher handoff behavior. The target runs the focused named-factory smoke that proves watched input moves to the activated factory, the previous factory stops receiving watched work, and the handoff leaves only one completed dispatch for the new watched file.
6. Run `make dashboard-verify` after dashboard UI source changes or embedded asset changes.
7. Run `make ui-test` for focused dashboard UI behavior.
8. Run `make ui-storybook` when Storybook fixtures, visual states, or dashboard component stories change.
9. Run `make ui-test-storybook` after `make ui-storybook` when Storybook play functions, dashboard Storybook runtime mocks, or browser-backed interaction behavior change.
10. Run replay-focused smoke tests when changing `pkg/replay`, record/replay CLI flags, worker side-effect matching, or artifact promotion behavior.

### Cron Workstation Changes

Cron behavior crosses service tick production, Petri-net guards, dispatcher identity, event history, API read models, and dashboard projections. Keep [Workstation Kinds and Parameterized Fields](../workstations.md#cron-kind) as the canonical authoring and migration guide instead of duplicating the full cron model in local notes.

`TestCronWorkstations_ServiceModeSmoke_SubmitsInternalTimeWorkExpiresRetriesDispatchesAndFiltersViews` is the end-to-end integration smoke for the token-backed cron flow. It starts service mode, observes missing-input time work, verifies stale tick expiry and retry, submits the required input, proves normal worker dispatch/output, checks canonical cron metadata, and confirms normal API/dashboard projections hide internal time work.

Use these focused checks before the broader package gates when changing cron behavior:

```bash
go test ./pkg/config ./pkg/timework ./pkg/service ./pkg/factory/scheduler ./pkg/factory/subsystems ./pkg/factory/projections -count=1
make cron-time-work-smoke CRON_TIME_WORK_SMOKE_COUNT=1
make test-full GO_TEST_TIMEOUT=300s
```

Use the default `make cron-time-work-smoke` count when the change touches timing-sensitive cron scheduling, expiry, dispatcher, or projection behavior and needs repeated stability evidence.

Run `make ui-test` and `make ui-build` when dashboard or projection code changes. Run `make api-smoke` after editing `api/openapi-main.yaml`, any referenced OpenAPI fragment, or handler behavior.

## Functional Test Harness Guidance

Functional tests should use `testutil.WithFullWorkerPoolAndScriptWrap()` or
the current full-worker-pool equivalent whenever the behavior can be observed
through normal runtime dispatch. Mock at the outer provider, provider
command-runner, command-runner, or mock-worker command boundary instead of
replacing workstation execution with the synchronous/default harness path.

Use lower-level custom executors, synchronous/default execution, or async-only
harness seams only when the test intentionally verifies a lower-level contract
that the edge mocks cannot expose. Acceptable examples include pausing an
in-flight dispatch for dashboard or runtime snapshot inspection, asserting raw
dispatch fields before workstation resolution, or testing harness compatibility
itself. Document the reason near the test or in the inventory so reviewers can
see what behavior would be lost by migrating it.

Use [Functional Test Execution Mode Inventory](functional-test-execution-mode-inventory.md)
when migrating shortcut tests or reviewing exceptions. Keep
`docs/processes/AGENTS.md` linked to this section instead of duplicating the
full rule set for autonomous-agent instructions.

The functional-test package includes
`TestFunctionalTestsUseFullWorkerPoolHarnessOrDocumentException` as a
lightweight guardrail. New `testutil.NewServiceTestHarness(...)` calls should
include `testutil.WithFullWorkerPoolAndScriptWrap()`. If a shortcut is truly
needed, add a narrow entry to that test's exception map with the exact shortcut
count and the behavior that would be lost by migrating it.

Provider-error smoke tests that need to prove "requeue first, then fail after a
bounded retry budget" should mutate the copied fixture's `factory.json` with a
test-local guarded `LOGICAL_MOVE` workstation using a `visit_count` guard
instead of editing shared testdata. Keep the shared `worktree_passthrough`
fixture on its default infinite-retry shape and make the terminal-budget
behavior explicit inside the test that needs it.

## Local Gotchas

- Embedded dashboard builds are committed artifacts. Rebuild `ui/dist/` with `make ui-build` or `make dashboard-verify` after dashboard source changes.
- Do not run `ui-build` in parallel with Go vet, build, or test commands; Vite rotates hashed files under `ui/dist/assets`.
- Treat `factory.json` as a generated-schema boundary: normalize legacy key styles first, then decode through `pkg/api/generated.Factory` with unknown-field rejection enabled. Keep any compatibility exceptions explicit and narrow instead of falling back to permissive handwritten DTOs.
- Apply that same generated-schema boundary to replay and event-carried factory config: when `RUN_REQUEST.payload.factory` is decoded back from JSON, route the nested factory payload through `config.GeneratedFactoryFromOpenAPIJSON(...)` instead of relying on permissive struct unmarshalling.
- Browser-side PNG export should load the authored payload from `GET /factory/~current` and treat that canonical `NamedFactory` response as the only source of truth for embedded sharing metadata. The detailed boundary and wrapper shape are documented in [Named Factory API Contract Data Model](named-factory-api-contract-data-model.md).
- Browser-side sharing roundtrip coverage should exercise `writeFactoryExportPng(...)`, `readFactoryImportPng(...)`, and `useFactoryImportActivation(...)` together so tests prove the same canonical `NamedFactory` reaches `POST /factory` without dashboard-only reshaping.
- App-level browser sharing smokes should export through the real dashboard dialog, capture the downloaded PNG blob, drop that same file back through the graph viewport import entry, and assert the resulting `POST /factory` body matches the original `GET /factory/~current` `NamedFactory` payload exactly.
- Dashboard Storybook interaction tooling is package-local to `ui/`. Keep runner config, `storybook-static` serving assumptions, base-path behavior, and API mocks under `ui/` or `ui/.storybook` instead of importing website Storybook setup.
- Browser-side factory export should serialize the authored `NamedFactory` payload returned by `GET /factory/~current` and write it into one PNG `iTXt` metadata chunk through the additive `PortOSFactoryPngEnvelope` wrapper; do not create a parallel export-only DTO or mixed event-derived payload.
- Browser-side factory sharing metadata must reuse the public generated `NamedFactory` contract fields `name` and `factory`, with PNG-only concerns limited to additive wrapper fields such as `schemaVersion`; do not rename the canonical named-factory fields to export-only aliases like `factoryName`.
- If browser-side PNG metadata has already shipped under a given `schemaVersion`, keep import compatibility for those required fields under that same version; for example, `v1` import still needs to accept legacy `factoryName` even though fresh exports now write canonical `name`.
- Browser-side factory export canonicalization must normalize legacy guard enum spellings such as `visit_count`, `all_children_complete`, and `any_child_failed` to the public OpenAPI values before packaging metadata; key-only alias rewrites still leak non-canonical factory contracts into exported PNGs.
- Browser-side export canonicalization must also preserve the full generated same-name input-guard contract: normalize `same_name` to `SAME_NAME` and `match_input` to `matchInput` instead of rejecting valid current-factory payloads during PNG export.
- Browser-side export canonicalization must stay aligned with `pkg/config/public_factory_enums.go`: if the public factory boundary accepts aliases such as `default`, `CLAUDE`, `SCRIPT_WRAP`, or lowercase workstation `kind` values, the PNG export path must canonicalize those spellings before the strict generated `Factory` decode runs.
- Browser-side export dialogs must invalidate any in-flight PNG export attempt when the dialog closes so a late async rasterization or metadata-write completion cannot trigger a download after the user cancels or dismisses the flow.
- For Agent Factory boundary-cleanup work that narrows a customer-visible DTO or formatter seam, check in a field inventory under `docs/development/*-data-model.md` before removing the broad contract so later stories can distinguish render-owned fields from canonical passthrough and dead aggregate-only ballast.
- For browser-backed dashboard download stories, serve `ui/storybook-static` and scope the Vitest Storybook run with `--testNamePattern` when only one changed story needs proof. If the story or App-level test both decodes an uploaded image and downloads a blob, stub `createImageBitmap` or `OffscreenCanvas` on `globalThis` instead of `URL.createObjectURL` so the upload decode path does not consume the download stub.
- For package-local browser-visible narrow-width verification, wrap the dashboard story in a bounded container such as `width: 360px`, keep the same production component tree, and assert `document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1` in the story `play` function instead of relying on website-only viewport helpers.
- Dashboard typography roles live in `ui/src/components/dashboard/typography.ts` and `ui/src/styles.css`. Reuse those semantic page, section, body, and supporting text classes before adding new `text-[...]` literals to cards, drill-downs, or chart labels.
- Shared dashboard shell helpers also live in `ui/src/components/dashboard/typography.ts`: use `DASHBOARD_SUPPORTING_LABELS_CLASS` for repeated metadata-label containers and `DASHBOARD_WIDGET_SUBTITLE_CLASS` for repeated large widget value/subtitle text instead of rebuilding inline label/value typography bundles.
- Detail-card and trace-table typography should layer `DASHBOARD_BODY_TEXT_CLASS`, `DASHBOARD_SUPPORTING_TEXT_CLASS`, `DASHBOARD_BODY_CODE_CLASS`, and `DASHBOARD_SUPPORTING_CODE_CLASS` onto nested rows, captions, pills, and metadata rather than restyling repeated `dt`/`dd` or code shells with local `text-[...]` values.
- Workstation-request selection banners and unavailable-request status copy should use `DASHBOARD_SUPPORTING_TEXT_CLASS` rather than keeping separate `text-[0.78rem]` status literals in drill-down cards.
- Current-selection drill-down coverage should stay in the focused component test files such as `work-item-card.test.tsx`, `workstation-detail-card.test.tsx`, `workstation-request-detail.test.tsx`, `state-node-detail.test.tsx`, and `terminal-work-summary-detail.test.tsx`; do not revive broad duplicate umbrella suites like `detail-cards.test.tsx` after the component contracts have split.
- Nested workstation-request drill-down sections should use `DASHBOARD_SECTION_HEADING_CLASS` for subsection headings and `DASHBOARD_SUPPORTING_LABEL_CLASS` for prompt/response captions instead of bare `<h4>` elements or local `text-[...]` label spans.
- Trend summaries and chart-adjacent secondary copy should keep `dl` containers on `DASHBOARD_SUPPORTING_LABELS_CLASS`, put primary summary values on `DASHBOARD_WIDGET_SUBTITLE_CLASS`, and use `DASHBOARD_BODY_TEXT_CLASS` for nearby select controls or cause-list copy instead of descendant `text-[...]` literals.
- Keep the dashboard shell on `overflow-x-hidden` when the page includes the React Flow graph. The graph viewport intentionally contains off-screen nodes and labels for pan/zoom, and without shell-level horizontal clipping those internal transforms can widen the page at narrow widths even when the visible card layout is otherwise correct.
- Current-activity workstation icons should come from `ui/src/features/flowchart/workstation-icon-metadata.ts`; keep node rendering, legend entries, and regression fixtures on that shared metadata instead of maintaining separate workstation icon lists.
- Use `ui/src/components/dashboard/test-fixtures.ts` `workstationKindParityDashboardSnapshot` for browser-visible standard/repeater/cron icon checks instead of mutating `semanticWorkflowDashboardSnapshot` inline in stories or Vitest files.
- When Storybook and Vitest need the same dashboard parity assertions, export the scenario-specific expectation catalog from `ui/src/components/dashboard/test-fixtures.ts` and derive icon expectations from shared flowchart metadata instead of restating labels or icon kinds inline.
- The current-activity graph legend uses `DashboardFlowAxisLegend` and starts minimized by default; tests and stories that assert legend icons should expand it first with the shared `expandGraphLegend(...)` helper instead of assuming the legend panel is already rendered.
- Canonical runtime history is exposed through `GET /events`; new API and UI history consumers should replay factory events instead of depending on dashboard snapshot routes.
- Inference-event consumers should treat `FactoryEvent.context.dispatchId` as the canonical dispatch identity. Generated inference payloads no longer restate `dispatchId` or `transitionId`, so projections should recover the transition from the matching dispatch request and only keep a narrow legacy-payload fallback for older recorded fixtures.
- Compatibility dashboard projections should derive from `GetEngineStateSnapshot(...)` or canonical event world state instead of recombining primitive getters in handlers.
- Runtime log files are service-owned. Initialize file-backed structured logging through `pkg/service.BuildFactoryService(...)` and pass work identity through `workers.ExecutionMetadata`.
- Worktree-backed tests must locate the repository root by searching upward for `go.mod` instead of assuming fixed `../../..` traversal from package directories. Nested `.claude/worktrees/...` layouts break hard-coded relative root calculations.
- Keep behavior-oriented package tests on package-local or paired replay fixtures. Repository-root generated artifacts and dashboard fixture sweeps belong in release-surface smoke coverage instead of `pkg/api`, `pkg/config`, or `pkg/replay` behavior tests.
- Provider-error and lane-isolation smoke tests should use `pkg/testutil` harness helpers instead of open-coded fixture scaffolding and polling loops.
- Shared Codex, Cursor-family, and Claude provider-failure fixtures live in `pkg/workers/testdata/provider_error_corpus.json`; extend that corpus and load it through `workers.LoadProviderErrorCorpus()` before adding new inline raw provider payloads to worker or functional tests.
- Shared provider-error smoke scenarios should assert `CompletedDispatch.ProviderFailure` type and family from the corpus entry they use, so normalization and runtime routing stay aligned through the full worker-pool path instead of only through final token placement.
- When transcript-trimming or bounded-error-line tests need extra noise around a supported provider failure, start from the shared corpus entry and layer the unique transcript text around that corpus-derived `ERROR:` line instead of open-coding a fresh supported payload.
- When Codex or Cursor-family provider failures change classification, update both `pkg/workers/testdata/provider_error_corpus.json` and the shared `codexProviderBehavior` matcher needles in `pkg/workers/provider_behavior.go` so retryable temporary-server failures do not silently drift into throttle or unknown handling.
- Keep record/replay side effects behind existing worker interfaces. Replay mode should install replay-aware providers and command runners through service wiring, not through runtime-specific shortcuts.
- When retiring a public `Factory` config field from `api/openapi.yaml`, remove it from the generated/public `Factory` model, drop any orphaned OpenAPI component schemas that existed only for that field, reject the raw input at `FactoryConfigMapper.Expand` with migration guidance once the validation story lands, and migrate checked-in fixtures/examples to the supported replacement contract in the same change.
- Guarded loop breakers authored as `type: LOGICAL_MOVE` plus `visit_count` guards stay normal scheduler-dispatched workstations when docs or tests need dispatcher-visible execution history; reserve `TransitionExhaustion` for legacy or system circuit-breaker paths such as retired `exhaustion_rules` and time-expiry consumption.
- File watcher input handling should parse `FACTORY_REQUEST_BATCH` JSON as the only structured submit format, map public batch item `work_type_name` values into runtime work type IDs, fill missing batch item work types from the watched folder, wrap Markdown and non-batch JSON files as one-item `WorkRequest` batches with raw file content payloads, reject item work-type conflicts before submitting, and parse plus validate all preseed files before calling the factory so startup failures do not create partial work.
- Deadcode findings are baseline-managed through `docs/development/deadcode-baseline.txt`. Remove confirmed stale symbols first, then update the baseline only for accepted remaining library or test-helper debt.

## Extending the Type System

Adding a new workstation type requires two steps — no engine changes are needed:

1. **Implement the `WorkstationTypeStrategy` interface:**

```go
type MyCustomType struct{}

func (m *MyCustomType) Kind() config.WorkstationKind {
    return "my-custom"
}

func (m *MyCustomType) HandleResult(result WorkResult) PostResultAction {
    // Return ActionAdvance to route normally, or ActionRepeat to re-fire.
    if result.Outcome == OutcomeRejected {
        return ActionRepeat
    }
    return ActionAdvance
}
```

2. **Register with the type registry:**

```go
registry := workers.NewWorkstationTypeRegistry()
registry.Register(&MyCustomType{})
```

The config validator checks workstation scheduling values against a known set of kinds. To make a new kind available in `factory.json`, add the kind constant to `pkg/interfaces/factory_config.go` and update the validation in `pkg/config/config_validator.go`.


## Related Docs

- [Agent Factory README](../../README.md)
- [Internal Architecture](architecture.md)
- [API Inventory](api-inventory.md)
- [Dashboard UI Replay Testing](dashboard-ui-replay-testing.md)
- [Contract Guard Walker Inventory](contract-guard-walker-inventory.md)
- [Factory Config Schema Inventory And Enum Policy](factory-config-schema-inventory-and-enum-policy.md)
- [Factory Config Generated-Schema Boundary Inventory](factory-config-generated-schema-boundary-inventory.md)
- [Safe Diagnostics Contract Consolidation Data Model](safe-diagnostics-contract-consolidation-data-model.md)
- [Simple Dashboard World-View Seam Inventory](simple-dashboard-world-view-seam-inventory.md)
- [Simple Dashboard Render DTO Data Model](simple-dashboard-render-dto-data-model.md)
- [Simple Dashboard World-View Field Inventory](simple-dashboard-world-view-field-inventory.md)
- [World-View Contract Cleanup Data Model](world-view-contract-cleanup-data-model.md)
- [Live Dashboard](live-dashboard.md)
- [Record and Replay](record-replay.md)
- [Provider Error Corpus Audit](provider-error-corpus-audit.md)
- [Root Factory Artifact Contract Inventory](root-factory-artifact-contract-inventory.md)
- [Port OS Reference Inventory](portos-reference-inventory.md)
- [Dashboard UI Workflow Baseline](dashboard-ui-workflow-baseline.md)
- [Dashboard UI Bun Validation](dashboard-ui-bun-validation.md)
- [Agent Factory Intent](../intents/agent-factory.md)
- [Standards Index](../standards/STANDARDS.md)
