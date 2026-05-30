# Dashboard Session Scope Import Boundaries

This note defines when dashboard code may import `useDashboardSessionStore` /
`dashboardSessionStore` directly versus consuming session identity and API paths
through `useDashboardSession()`.

## Scope

The dashboard keeps **one Zustand store** for tab selection and per-session
pause flags (`ui/src/features/dashboard/state/dashboardSessionStore.ts`). The
dashboard shell projects that store into a **React context scope**
(`DashboardSessionProvider` + `buildSessionScope` in `ui/src/api/session-scope.ts`)
so feature hooks do not duplicate session normalization or path building.

New feature work under `DashboardSessionProvider` should treat
`useDashboardSession()` as the default seam for:

- normalized `sessionID` and raw `rawSessionID`
- canonical `factoryPath`, `workPath`, and `eventsPath`
- `isDefault` and `isPaused`

## Default rule

**Do not** import `useDashboardSessionStore` or
`../state/dashboardSessionStore` (or equivalent relative paths) from:

- feature hooks under `ui/src/features/**/hooks/`
- bento cards and dashboard widgets (`ui/src/features/bento/`, submit-work,
  export, import, current-factory-definition, workflow-activity, etc.)
- API client hooks that only need the active session for routing

Use `useDashboardSession()` instead. Wrap unit tests with
`DashboardSessionProvider` when exercising real tab-store updates, or
`DashboardSessionTestProvider` from `ui/src/testing/` when pinning session
identity without touching the store.

## Allowed direct store importers (production)

| Module | Role |
| --- | --- |
| `ui/src/features/dashboard/session/dashboard-session-provider.tsx` | Subscribes to `selectedSessionID` and `pausedSessionIDs`, supplies `buildSessionScope` via context. |
| `ui/src/features/header/hooks/use-dashboard-session-tabs-state.ts` | Session tab strip: reads active session, mutates selection and per-session pause through store actions. |
| `ui/src/features/dashboard/state/dashboardSessionStore.ts` | Store definition and `resetDashboardSessionStore` export. |

`DashboardSessionTabs` and related header components should keep using
`use-dashboard-session-tabs-state` rather than importing the store directly.

## Stream and pause lifecycle

The live factory event stream (`useDashboardSnapshot`) **does not** import the
store. It reads `rawSessionID`, `sessionID`, and `isPaused` from
`useDashboardSession()`. Header tab controls mutate pause/selection on the
store; the provider re-projects scope so the stream reconnects or shows paused
messaging without feature code calling `setSessionPaused` or
`setSelectedSessionID` outside the header seam.

## Allowed infrastructure importers (non-feature)

| Module | Role |
| --- | --- |
| `ui/.storybook/dashboard-story-runtime.tsx` | Resets session store between Storybook dashboard stories alongside timeline/selection stores. |
| `ui/src/testing/app-shell-test-utils.tsx` | Resets session store in full app-shell test lifecycle registration. |

These modules must not grow new business logic; they only reset or seed store
state for verification harnesses.

## Tests and stories

Tests and stories **may** import the store when the scenario explicitly exercises
tab switching, pause toggling, or store-driven provider updates. Prefer
`DashboardSessionTestProvider` / `renderWithDashboardSessionTest` when the test
only needs a fixed session identity or paths.

Examples that should keep the store:

- `dashboard-session-provider.test.tsx` — provider follows `setSelectedSessionID`
- `useDashboardSnapshot.test.tsx` — pause/resume and tab switch stream behavior
- `dashboard-session-tabs.test.tsx` — tab strip mutates selection

Examples that should prefer the test provider:

- export/submit-work hook tests scoped to `session-beta`
- prompt-template hook tests that only need a non-default session id

## Biome enforcement

`ui/biome.jsonc` enables `style/noRestrictedImports` with a pattern that blocks
imports of `dashboardSessionStore` / `dashboardSessionStore.ts` module paths.
Overrides turn the rule off for the allowed production modules, Storybook
runtime reset, app-shell test utilities, and all `*.test.*` / `*.stories.*`
files so tests can keep exercising the real tab store when needed.

When adding a new allowed production importer, update this document and the
Biome override list in the same change.

## Related seams

- Path helpers: `ui/src/api/session-routing.ts`
- Pure scope builder: `ui/src/api/session-scope.ts`
- Test session pin: `ui/src/testing/dashboard-session-test-provider.tsx`
- Process inventory row: `docs/internal/processes/development-guide-relevant-files.md` (factory-session workspace contract)
