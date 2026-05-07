# Unified Event Log Legacy Cleanup

Date: 2026-04-18
Scope: Agent Factory record/replay and public event model cleanup for `US-006`.

## Findings
- `ReplayArtifact` still exposed JSON-ignored `RecordedWorkRequest`, `RecordedSubmission`, `RecordedDispatch`, and `RecordedCompletion` storage paths after generated `FactoryEvent` artifacts became canonical.
- `pkg/interfaces/factory_events.go` still carried handwritten event envelope/context/type definitions beside generated OpenAPI types.
- Record/replay tests still asserted against replay-specific top-level arrays, which hid regressions in the generated event log boundary.

## Cleanup Applied
- Deleted the legacy replay storage model family and recorder append methods, leaving `RecordEvent` as the artifact append path.
- Removed handwritten factory event envelope/context/type declarations from interfaces and kept generated `pkg/api/generated.FactoryEvent` as the event contract.
- Added an AST guard test in `pkg/api` that rejects reintroduced deleted model names and type aliases to generated API types outside `pkg/api/generated`.
- Updated record/replay tests to decode `WORK_REQUEST`, `DISPATCH_CREATED`, and `DISPATCH_COMPLETED` events from `ReplayArtifact.Events`.

## Follow-Up Notes
- Runtime replay reducers may continue to use unexported internal structs derived from generated events; those are not persisted as JSON storage models.
- Future artifact assertions should count or decode generated events instead of checking replay-specific arrays.
