# Cleanup Idea: Extract Shared App Dashboard Test Harness

## Why this cleanup exists

The dashboard App-level test suite is now split across:

- `ui/src/App.test.tsx`
- `ui/src/App.current-selection.test.tsx`

That split is useful, but both files still carry parallel setup code for the
same browser-backed App harness:

- `MockEventSource`
- `RenderAppOptions`
- `timelineSnapshot`
- `seedTimelineSnapshot`
- `seedTimelineSnapshots`
- `renderApp`
- `requireValue`
- `removeTraceIDsFromSnapshot` and related work-item trace stripping helpers
- repeated QueryClient cleanup, store resets, local-storage cleanup, fetch
  stubbing, EventSource stubbing, and dashboard browser shims

This duplication makes future App-level dashboard regression tests more
expensive to maintain because store reset behavior, stream seeding, mocked
fetch behavior, and timeline fixture setup can drift between the general App
suite and the current-selection suite.

## Requested change

Extract the shared App dashboard test harness into one test helper module and
use it from both App-level test files.

Keep the cleanup narrow:

- do not change App runtime code
- do not change user-visible dashboard behavior
- do not change which App-level behaviors are covered
- do not merge the two test files back together
- do not broaden this into a fixture rewrite for every dashboard feature test
- do not replace rendered UI assertions with source-layout, route-inventory, or
  helper-location assertions

Suggested shape:

- Add a helper near the App tests, for example
  `ui/src/testing/app-dashboard-test-harness.tsx` or
  `ui/src/App.test-harness.tsx`.
- Move only genuinely shared App harness code there:
  - EventSource mock
  - render helper and React Query client lifecycle support
  - timeline snapshot seeding helpers
  - reusable store/browser cleanup setup helpers
  - small shared assertion utilities such as `requireValue`
  - trace-ID stripping helpers if both suites still need them
- Keep test-specific fixtures and assertions in their owning test files.
- Keep current-selection-specific helpers such as dispatch-card expansion in
  `App.current-selection.test.tsx` unless they are already used by both suites.

## Relevant files

- `ui/src/App.test.tsx`
- `ui/src/App.current-selection.test.tsx`
- `ui/src/features/timeline/state/factoryTimelineStore.ts`
- `ui/src/components/dashboard/test-browser-shims.ts`
- `docs/internal/processes/development-guide-relevant-files.md`

## Acceptance criteria

- The shared App render, timeline seeding, EventSource mock, and repeated cleanup
  setup no longer have two independent implementations across
  `App.test.tsx` and `App.current-selection.test.tsx`.
- The two App-level test files remain behavior-focused and retain their current
  user-visible dashboard/current-selection assertions.
- The helper module does not become a generic dashboard testing framework; it
  should expose only the App-level harness pieces currently duplicated by these
  two files.
- Existing App and current-selection tests continue to pass.
- Run the focused UI test command for these files, for example
  `bun test src/App.test.tsx src/App.current-selection.test.tsx` from `ui/`, or
  the repository's current equivalent.

## Review guidance

Prefer reviewing the diff by confirming duplicated setup disappeared and the
same rendered App behaviors are still asserted. The regression risk is changed
App test setup semantics, especially timeline world-view seeding, EventSource
stream delivery, React Query cleanup, and store reset behavior.
