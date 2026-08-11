# Dashboard Fixture Catalog

This catalog provides the canonical graph fixtures for dashboard layout tests, app rendering tests, Storybook stories, and future browser smoke coverage.

## Topologies

- `oneNodeDashboardTopology`: a single intake workstation with no edges.
- `mediumBranchingDashboardTopology`: a five-workstation workflow with branch, join, retry, and failed-state paths.
- `twentyNodeDashboardTopology`: a representative 20-workstation workflow that exercises multi-column layout.
- `dashboardTopologyFixtures`: a named collection of the exported topology fixtures.

## Runtime Semantics

- `activeWorkRuntimeOverlay`: marks a workstation and work item as active.
- `retryAttemptRuntimeOverlay`: adds a provider session with a retry outcome.
- `failedOutcomeRuntimeOverlay`: adds failed session metrics and a failed provider outcome.
- `rejectedOutcomeRuntimeOverlay`: adds a rejected provider outcome.
- `dashboardRuntimeOverlays`: a named collection of the exported runtime overlays.
- `buildDashboardSnapshotFixture(...)`: composes a topology with one or more overlays without mutating the base topology.
- `dashboardSemanticSnapshotFixtures`: ready-made snapshots for active, retry, failed, and rejected runtime states.

## Event Replay Fixtures

- `failureAnalysisTimelineEvents`: a streamed event sequence with queued work, a consumed in-flight item, and a failed `DISPATCH_RESPONSE` carrying `failureReason` and `failureMessage`.
- `resourceCountBackendWorldViewCountsByTick`: backend `BuildFactoryWorldView(...)` expected resource counts for the resource-count smoke ticks.
- `resourceCountTimelineEvents`: a streamed event sequence with configured resource capacity, active dispatch resource consumption, and completed dispatch resource release.
- `runtimeDetailsTimelineEvents`: a streamed workstation-request-selection sequence with one pending request, one successful response, and one failed response, including inference events and safe diagnostics.
- `runtimeDetailsBackendWorkstationRequestsByDispatchID`: checked-in backend `BuildFactoryWorldWorkstationRequestProjectionSlice(...)` expectations for the runtime-details smoke dispatches.
- `scriptDashboardIntegrationTimelineEvents`: a mixed streamed event sequence with script success, script failure, and inference success dispatches for the dashboard integration smoke path.
- `scriptDashboardIntegrationBackendWorkstationRequestsByDispatchID`: checked-in backend `BuildFactoryWorldWorkstationRequestProjectionSlice(...)` expectations for the mixed script-and-inference smoke dispatches.

## Dashboard Regression Fixture

- `dashboardRegressionFixture`: one identity-keyed scenario for dashboard
  session discovery, out-of-order list refreshes, chart states/remounts,
  two-session submit races, and Open/New Factory outcomes.
- `createDashboardRegressionFixture()`: creates a fresh controller with
  observable promise controls for list, submit, and Factory-operation
  completion. It never uses wall-clock delays.
- `canonicalizeDashboardRegressionSessions(...)`: fixture-local expected-row
  normalization that drops selector aliases and duplicate canonical IDs.

The regression fixture keeps `~default` in selector state and uses the resolved
UUID for durable rows and operation ownership. Its submit, chart, and Factory
journey records are keyed by immutable IDs so component and browser scenarios
can advance the same race without relying on labels or array positions.

## Workstation-Request Fixtures

- `buildDashboardInferenceAttemptFixture(...)`: creates a typed inference-attempt projection for request-detail coverage.
- `buildDashboardWorkstationRequestFixture(...)`: creates a typed workstation-request projection with one canonical review work item.
- `dashboardWorkstationRequestFixtures.ready`: a response-ready review request with request metadata, response metadata, and retry inference attempts.
- `dashboardWorkstationRequestFixtures.noResponse`: a request-only review request that renders pending response copy.
- `dashboardWorkstationRequestFixtures.errored`: an errored review request with projected failure details.

Reuse this catalog when a test, story, or smoke check needs one of the accepted dashboard graph shapes or semantic runtime states. Keep a local fixture only when the scenario intentionally differs from these canonical examples, such as malformed API responses, omitted fields, or a graph shape built for a specific edge case.

Import fixture exports directly from `src/dashboard/fixtures` or from `src/dashboard/test-fixtures` in Storybook and Vitest files. Do not export these fixtures through `src/dashboard/index.ts`; the dashboard barrel is the production runtime API surface.
