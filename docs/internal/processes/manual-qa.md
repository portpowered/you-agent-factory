# Manual QA

This document records the repo-local browser verification flow for dashboard UI changes and the latest execution evidence.

## Browser Verification Flow

Run these commands from the repository root unless the step says otherwise.

1. `cd ui && bun install`
2. `cd ui && bun run build-storybook`
3. `cd ui && bun run test-storybook`
4. `cd ui && bun run storybook:responsive-check`
5. `cd ui && bun run build`
6. `cd ui && bun run preview`

`storybook:responsive-check` is self-contained against `storybook-static`: it reuses an already-running Storybook server on `127.0.0.1:6008` when present, or starts and stops a temporary static server automatically after `build-storybook`.

After the preview server starts, open `http://127.0.0.1:4173` in a browser and spot-check the changed dashboard flows at these viewport widths:

- Mobile: `390x844`
- Tablet: `768x1024`
- Desktop: `1440x900`

## Mock-Worker Runtime Verification

Use live mock-worker QA when a change depends on real factory routing,
runtime outcomes, or observable side effects that Storybook verification and
unit-level automated coverage cannot prove on their own. This is the shared
baseline for runtime-oriented checks; feature-specific mock-worker checklists
may add narrower steps, but they extend this process instead of replacing it.

Start from these existing command patterns:

- `you run --dir ./factory --with-mock-workers`
- `you run --dir ./factory --with-mock-workers ./mock-workers.json`

Use the public [Authoring Factories](../../reference/authoring-factories.md)
guide for command usage details, mock-worker setup expectations, and related
runtime options instead of duplicating that setup documentation here.

During live runs, verify these reusable runtime outcomes before closing the
feature-specific checklist:

- Routing reaches the expected workstation or terminal outcome for the scenario
  you triggered, and the visible status, logs, or emitted artifacts match that
  path instead of silently falling back to a different branch.
- Rejection and retry-loop paths behave as intended for the exercised case,
  including whether work requeues, stops, or surfaces an explicit rejected
  outcome when a mock worker returns that result.
- Failure handling stays observable to maintainers, including any surfaced
  errors, preserved failure state, or recovery path that the runtime exposes
  after a worker, script, or downstream step fails.
- Script side effects that matter to the scenario actually happen, such as
  expected files, outputs, or state transitions appearing once the run
  completes, instead of only assuming the script was invoked.

Feature-specific mock-worker checklists can add narrower scenarios or
domain-owned assertions, but they should extend this baseline instead of
replacing it. Keep branch-owned deep dives, such as the open-session checklist,
in their feature-specific docs rather than absorbing them into this generic
section.

When a live mock-worker run completes, append a `Latest Evidence` entry that
records:

- The exact command variant you ran, including whether the run used the default
  mock-worker set or an explicit `./mock-workers.json` file.
- The scenario or feature-specific checklist path you exercised, stated in
  maintainer-observable terms.
- The observed routing or terminal outcome that proved the scenario completed as
  intended.
- Any verified rejection, retry-loop, failure-handling, or script-side-effect
  observations when those paths were part of the exercised run.

## Dashboard UI Checklist

Use this checklist for the shadcn primitive migration lane and similar dashboard-control changes.

- Submit work card: request name, request text, and work type stay labeled, keyboard-focusable, and preserve disabled and busy states.
- Factory graph add flow: work type creation clearly asks for the first ordered state, work state creation clearly asks for its parent work type and state type, and field-level validation stays visible after failed submits.
- Factory graph save/discard flow: pending graph changes expose explicit save and discard actions, save opens a confirmation summary of created/deleted entities and changed edges, stale drafts show a newer-topology warning, and active work keeps save visibly blocked with a clear explanation.
- Factory graph connect flow: workstation transition anchors stay labeled as Success, Continue, Failure, and Reject, compatible state anchors remain keyboard-reachable in connect mode, and incompatible anchor pairings surface an explicit recovery notice instead of silently failing.
- Factory graph edge-delete flow: removing a route or resource/worker edge requires an explicit confirmation that names the affected route or availability link, and pending added or removed edges stay visually distinct before save.
- Factory graph removal flow: the delete tool exposes a destructive confirmation that names the entity and impact summary, ineligible removals show an explicit warning notice, and pending removals stay visibly marked in the graph before save.
- Export PNG flow: export trigger opens the dialog, validation text renders when input is invalid, export actions keep disabled and busy states, and a successful download leaves a visible success acknowledgment before dismissal.
- Import preview flow: preview image, dropped filename, cancel, activate, and close controls stay keyboard-reachable, and the dialog remains readable at mobile, tablet, and desktop viewport widths.
- Completed and failed work card: expand and collapse controls remain keyboard-operable and selected work rows still update the current-selection panel.
- Selected-work dispatch history: dispatch cards keep the work-facing title and `Started at` summary visible, omit request-count summary rows, and keep inference or script attempt details collapsed until expanded on both narrow and wide layouts.
- Trace drill-down card: selectable work-item controls still update the trace detail surface and dispatch grid.
- Trace/factory graph consolidation: factory and trace graph regions keep matching zoom-control chrome, trace relation and dispatch graphs stay visible after expanding work items, and both surfaces avoid horizontal overflow on mobile and desktop widths.
- Work outcome chart: loading, empty, error, and ready states render explicitly, and sparse series do not appear as fabricated zero-value lines.
- Trace dispatch grid: shared table and skeleton states render without layout breakage on narrow and wide viewports.
- Dashboard header: icon-only branding keeps the accessible `you-agent-factory` name, the timeline slider cluster stays keyboard-operable, and the header toolbar remains unclipped at mobile, tablet, and desktop widths with desktop controls ordered brand -> slider -> stream status -> export action.
- Dashboard shared shell: the header and representative grid-card shell use the same computed border, radius, background, and shadow at mobile, tablet, and desktop widths while header and card controls keep their accessible names.
- Current-selection prompt editor: the prompt variable help toggle stays keyboard-operable, available-variable examples remain readable in-context, invalid template references render inline squiggle diagnostics, and the editor stays unclipped at mobile, tablet, and desktop widths.
- Open factory session flow: the dialog stays folder-first, invalid folder states render inline recovery copy for missing/file/unreadable/no-runnable cases, `~` home-directory input resolves to the same ready state as the equivalent absolute folder, detected factory choices stay selectable after validation, and manual override validation blocks launch when the requested named factory is missing.

## Open Factory Session Mock-Worker Checklist

Use this checklist when the open-session flow changes and reviewers need proof against a live service instead of Storybook-only mocks.

1. Build the local CLI: `go build -o bin/you ./cmd/factory`.
2. Create a disposable factory root under the current user home directory so `~` expansion is observable. Copy the repo-owned `factory/` scaffold into the root plus named child folders such as `review/` and `plan/`, then create `empty/`, `unreadable/`, and `not-a-dir.txt` failure fixtures.
3. Start the live service with mock workers and a fixed port: `bin/you run --dir "$OPEN_SESSION_ROOT" --with-mock-workers --continuously --quiet --port 7545`.
4. In the dashboard at `http://127.0.0.1:7545/dashboard/ui`, open the session dialog and check the inline UI states:
   `missing/` path shows the missing-folder recovery copy.
   `not-a-dir.txt` shows the non-directory recovery copy.
   `unreadable/` shows the unreadable-directory recovery copy.
   `empty/` shows the no-runnable-factory recovery copy.
   `~/...` resolves to the same validated folder as the absolute home-directory path.
5. With the same live service, verify the launch-target contract over the same `/factory-sessions` API the UI calls:
   `POST /factory-sessions` with `{"folderPath":"$OPEN_SESSION_ROOT","validateOnly":true}` returns default plus named targets from that folder.
   `POST /factory-sessions` with `{"folderPath":"~/...","validateOnly":true}` returns the same resolved `folderPath` and targets.
   `POST /factory-sessions` with `{"folderPath":"$OPEN_SESSION_ROOT","validateOnly":true,"target":{"kind":"named","name":"review"}}` succeeds, while the same request with `name:"missing"` fails with `400`.
   `POST /factory-sessions` with `{"folderPath":"$OPEN_SESSION_ROOT","target":{"kind":"named","name":"review"}}` returns a non-default session whose `factoryDir` points at `$OPEN_SESSION_ROOT/review`.
   `GET /factory-sessions` after launch shows the newly opened named session and preserves its `target.kind:"named"` and `target.name:"review"` instead of falling back to `~default`.

## Latest Evidence

Date: `2026-05-19`

- `cd ui && bun run build-storybook` passed.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/factory-graph-editor/factory-graph-editor-controls.stories.tsx src/features/factory-graph-editor/factory-graph-editor-flow.stories.tsx` passed in a browser-backed runner, covering destructive confirmation, blocked-removal notice copy, and pending-removal graph rendering for the factory graph editor removal flow.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/factory-graph-editor/factory-graph-editor-controls.stories.tsx` passed in a browser-backed runner, covering pending save/discard actions, save confirmation summary copy, active-work blocking, and stale-definition warning states for the factory graph editor.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/factory-graph-editor/factory-graph-editor-flow.stories.tsx` passed in a browser-backed runner, covering distinct pending-added and pending-removed edge styling in the focused factory graph editor flow story.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/factory-graph-editor/factory-graph-editor-flow.stories.tsx` passed in a browser-backed runner, covering labeled Success/Continue/Failure/Reject workstation anchors and keyboard-reachable connection-source selection in the focused graph-editor flow story.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/factory-graph-editor/factory-graph-editor-add-dialog.stories.tsx` passed in a browser-backed runner, covering the differentiated work-type and work-state add dialogs plus field-level work-state validation copy.
- `cd ui && bun run build` passed.
- `cd ui && bun run storybook:responsive-check` passed against built Storybook `iframe.html` stories for `ExportFactoryDialog`, `DashboardImportPreviewDialog`, and the focused `DashboardHeader` verification story, confirming mobile (`390x844`), tablet (`768x1024`), and desktop (`1440x900`) dialog/header bounds, visible controls, keyboard timeline interactions, desktop toolbar ordering, and no horizontal overflow in headless Chromium.
- `make test` passed from the repository root while the refreshed Storybook wrapper waited for the built index to restabilize before launching the responsive browser check.

Date: `2026-05-20`

- `cd ui && bun run build-storybook` passed.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6013 bun x vitest run --config vitest.storybook.config.ts --project=storybook src/App.workstation-requests.stories.tsx` passed in a browser-backed runner, confirming current-selection request and response timestamps render as browser-local date/time text instead of raw UTC ISO strings, while the no-response and errored workstation-request stories keep explicit empty and error messaging visible.
- `cd ui && bun run test-storybook` passed in a browser-backed runner, including the selected-work dispatch history smoke story that proves compact summary fields, collapsed-by-default inference and script attempt disclosures, and successful expansion of both attempt paths.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6012 bun run test-storybook` passed in a browser-backed runner, including the submit-work `StableActionAlignment` story that proves the primary button keeps the same measured right edge across ready, submitting, success, error, and wrapped validation states.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6010 bun run test-storybook` passed in a browser-backed runner, including tagged header stories for icon-only branding, slider alignment, keyboard-driven return/export actions, and the dedicated responsive verification script.
- `cd ui && bun run build` passed.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6011 node scripts/verify-import-export-storybook-responsive.mjs` passed against built Storybook `iframe.html` stories for `ExportFactoryDialog`, `DashboardImportPreviewDialog`, the dashboard header verification story, and the shared shell verification story, confirming mobile (`390x844`), tablet (`768x1024`), and desktop (`1440x900`) dialog/header/card bounds, visible controls, keyboard timeline interactions, desktop toolbar ordering, matching computed header/card shell styles, and no horizontal overflow in headless Chromium.
- `cd ui && bun run tsc` passed.
- `cd ui && bunx vitest run src/features/current-selection/current-selection-widget.save.test.tsx scripts/verify-import-export-storybook-responsive.schedule.test.mjs scripts/verify-import-export-storybook-responsive.test.mjs scripts/dashboard-shell-storybook-responsive.test.mjs` passed.
- `cd ui && bun run build-storybook` passed.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6012 bun run test-storybook` passed, including the new current-selection prompt hinting Storybook interaction and responsive browser verification for prompt help, inline squiggle diagnostics, keyboard interaction, and mobile/tablet/desktop overflow checks.
Date: `2026-05-22`

- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/header/dashboard-session-tabs.stories.tsx` passed in a browser-backed runner, covering the visible session tab strip, the folder-first open-session dialog, the multi-target picker step, and activation of the newly opened session tab.
- `cd ui && bun run storybook:responsive-check` passed against built Storybook `iframe.html` stories for the dashboard session tabs in addition to the existing dialog and header checks, confirming the session tab strip, open-session trigger, target picker, and active-folder summary remain visible without horizontal overflow at mobile (`390x844`), tablet (`768x1024`), and desktop (`1440x900`) widths in headless Chromium.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/trace-drilldown/trace-grid-card.stories.tsx src/features/trace-drilldown/trace-graph-surfaces.stories.tsx src/features/workflow-activity/react-flow-current-activity-card.stories.tsx` passed in a browser-backed runner, covering the representative factory graph story plus the trace drill-down and standalone trace-graph surfaces that back the graph-consolidation regression lane.
- `cd ui && bun run storybook:responsive-check` passed against built Storybook `iframe.html` stories for the factory graph narrow-viewport story, the standalone trace graph surfaces story, localized widgets, header verification, shared shell verification, and import/export dialogs, confirming matching factory/trace graph control chrome plus no horizontal overflow at `390x844`, `768x1024`, and `1440x900`.
- `cd ui && bun x vitest run scripts/verify-import-export-storybook-responsive.test.mjs scripts/verify-import-export-storybook-responsive.schedule.test.mjs` passed after adding provider-session success coverage to the responsive verifier.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6013 bun run test-storybook` passed, including `you-agent-factory/Workflow Dashboard/ProviderSessionDetailVerification`, which selects provider session `019e44f4-580e-7f32-981e-1e54ec6907d6` and confirms the current-selection panel renders `Selected session details`, `Source file`, and `Token usage` instead of the missing-session state.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6014 bun run storybook:responsive-check` passed against the built Storybook `iframe.html` story `you-agent-factory/Current Selection/Provider Session Detail Panel/TimestampPrefixedSessionSuccess`, confirming the provider-session success panel stays visible without horizontal overflow at mobile (`390x844`), tablet (`768x1024`), and desktop (`1440x900`) widths while rendering the timestamp-prefixed source path `2026/05/20/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl`.
- `cd ui && bun run build-storybook` passed after the `you-agent-factory` Storybook slug updates.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6013 bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/header/components/dashboard-header.stories.tsx src/App.stories.tsx src/features/current-selection/provider-session-detail-panel.stories.tsx` passed in a browser-backed runner, covering the renamed `you-agent-factory/Dashboard/Dashboard Header`, `you-agent-factory/Workflow Dashboard`, and `you-agent-factory/Current Selection/Provider Session Detail Panel` stories, including the accessible `you-agent-factory` header branding and the provider-session success panel.
- `cd ui && AGENT_FACTORY_STORYBOOK_PORT=6014 bun run storybook:responsive-check` passed against built Storybook `iframe.html` stories for the renamed `you-agent-factory` export/import dialogs, dashboard header, session tabs, workflow dashboard verification stories, and `TimestampPrefixedSessionSuccess`, confirming visible controls, keyboard timeline interactions, and no horizontal overflow at mobile (`390x844`), tablet (`768x1024`), and desktop (`1440x900`) widths.

Date: `2026-05-23`

- `cd ui && bun run tsc` passed.
- `cd ui && bun run lint` passed after splitting the new graph-parity responsive assertions into `ui/scripts/verify-graph-parity-storybook-responsive.mjs`.
- `cd ui && bun x vitest run src/features/factory-graph-editor/components/factory-graph-editor-flow.test.tsx src/features/workflow-activity/react-flow-current-activity-card-editor-controllers.test.tsx src/features/workflow-activity/components/react-flow-current-activity-card.coverage.test.tsx src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx` passed, covering all four saved workstation outcome anchors plus draft connection creation through the shared editor controller path.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/factory-graph-editor/components/factory-graph-editor-flow.stories.tsx src/features/workflow-activity/components/react-flow-current-activity-card.stories.tsx` passed in a browser-backed runner, covering the focused editor parity stories and the observer current-activity card stories together.
- `cd ui && bun run build` passed.
- `cd ui && bun run build-storybook` passed.
- `cd ui && bun run storybook:responsive-check` passed after narrowing two brittle pre-existing responsive assertions in the shared dashboard session-tab/header verifiers and adding the new observer/editor graph parity stories, confirming visible graph controls and no horizontal overflow at mobile (`390x844`), tablet (`768x1024`), and desktop (`1440x900`) widths.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/header/dashboard-session-tabs.stories.tsx` passed in a browser-backed runner after aligning the story with the current folder-check and target-select flow, confirming the localized open-session labels, multi-target select control, launch summary, and newly opened session tab activation.
- `make typecheck` passed.
- `cd ui && bun x vitest run --config vitest.storybook.config.ts --project=storybook src/features/header/dashboard-session-tabs.stories.tsx` passed again while closing out manual verification, confirming the folder-first dialog, target-selection step, launch summary, and newly opened session tab activation still hold in the browser-backed Storybook runner.
- Live mock-worker verification passed against `bin/you run --dir /Users/abdifamily/open-session-qa-valid-98917 --with-mock-workers --continuously --quiet --port 7545` using the repo-owned `factory/` scaffold copied into a disposable home-directory root with named `review/` and `plan/` children plus `empty/`, `unreadable/`, and `not-a-dir.txt` failure fixtures.
- `curl http://127.0.0.1:7545/factory-sessions` initially returned only the default live session for `/Users/abdifamily/open-session-qa-valid-98917`, then returned both the default session and a named `review` session after launch.
- `curl -X POST http://127.0.0.1:7545/factory-sessions -H 'Content-Type: application/json' --data '{"folderPath":"/Users/abdifamily/open-session-qa-valid-98917","validateOnly":true}'` returned the expected `default`, `plan`, and `review` targets, and the same request with `folderPath:"~/open-session-qa-valid-98917"` returned the same resolved folder and targets.
- Failure-path verification passed over the live API: `missing/` returned `400` with a missing-folder error, `not-a-dir.txt` returned `400` with the non-directory error, `unreadable/` returned `400` with the unreadable-directory error, `empty/` returned `400` with the no-runnable-factory error, and `target.name:"missing"` returned `400` with `factory session target "missing" was not found`.
- Explicit named-target launch passed over the live API: `POST /factory-sessions` with `{"folderPath":"/Users/abdifamily/open-session-qa-valid-98917","target":{"kind":"named","name":"review"}}` returned a non-default session whose `factoryDir` was `/Users/abdifamily/open-session-qa-valid-98917/review`, confirming the launch path did not silently fall back to the default factory.
