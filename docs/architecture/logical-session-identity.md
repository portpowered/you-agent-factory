# Logical session identity and restart recovery

This document describes how Agent Factory derives stable logical session
identity from factory session targets, how clients remap persisted runtime
selectors after backend restart, and which client state survives a remap.

The customer-facing vocabulary remains `Factory Session`, `backendScopeID`,
`logicalSessionKeyID`, `factorySessionID`, and `streamGenerationID`. See
`docs/architecture/data-model.md` for the public resource model.

## Problem and model

A dashboard tab can persist a concrete `factorySessionID` and reconnect
cursors for timeline replay. After backend restart that UUID may be missing
or may refer to a different event history. User intent is better expressed as
**which factory session target** the tab opened, not which runtime allocation
happened to exist at save time.

Logical session identity closes that gap:

- **`backendScopeID`** scopes identity to one backend instance (local operator
  backend, cloud scope, or other deployment boundary).
- **`logicalSessionKeyID`** is a stable opaque id derived from a **normalized
  target reference** within that scope. It is **not** a separately allocated
  logical-session row and does not depend on `factorySessionID` allocation
  order across restarts.
- **`factorySessionID`** is the current live Factory Session UUID for that
  logical target within the active backend.
- **`streamGenerationID`** is the current live event-stream generation for
  that session. It changes when the backend replaces the session's event
  history boundary.

Clients resolve `backendScopeID + logicalSessionKeyID` through sync preflight
before opening SSE or restoring stream-derived caches.

## Deriving `logicalSessionKeyID`

Derivation is deterministic and pure:

1. Normalize the factory session target into a canonical reference (backend
   scope, workspace folder, and target kind-specific fields).
2. Build a stable signature from those canonical fields.
3. Hash the signature and expose an opaque `lsk-` prefixed id to API clients.

Equivalent normalized targets always produce the same `logicalSessionKeyID`.
Distinct backend scopes, workspace folders, named targets, or provider
boundaries produce distinct ids. The backend does not maintain a mutable
allocation table for logical keys.

Implementation lives in `pkg/factorysessions/logicaltarget/`; service wiring
derives keys in `pkg/service/runtime_sessions.go`
(`factorySessionLogicalSessionKeyID`).

## Target normalization (behavioral rules)

Normalization answers: "did the user mean the same factory session target?"
Invalid or ambiguous references return structured validation errors instead of
best-effort keys.

### Backend scope and workspace folder

Every logical target is scoped by:

- **`backendScopeID`**: trimmed non-empty backend scope string. Identities from
  one scope never resolve against another.
- **`folderPath`**: canonical absolute workspace path for the factory directory.
  Equivalent spellings normalize together: relative vs absolute paths, trailing
  separators, home-directory expansion, and symlink-resolved paths (for example
  macOS `/var` vs `/private/var`).

### Default target

Default-route aliases (`~default`, empty selector, explicit `default` kind)
normalize to one canonical **default** target within the same
`backendScopeID` and `folderPath`. A default target cannot also carry a named
target name; that combination is ambiguous and rejected.

The default session keeps the `~default` registry alias for CLI and API
compatibility, but runtime identity responses expose the allocated UUID
`factorySessionID`. Clients must not persist `~default` as a stream-derived
cache key once identity has been resolved.

### Named target

Named targets normalize through the same layout-segment rules used for on-disk
factory names. Equivalent user spellings that map to the same layout segment
(for example scoped names like `@you/goal`) share one canonical named target.
Distinct intended names remain distinct after normalization.

### Provider-backed target

Provider-backed targets include:

- provider id and kind (normalized to lowercase), and
- a stable provider workspace or account **boundary** string when available.

Boundary values must not embed secret material (tokens, API keys, passwords,
or similar markers). Provider-backed normalization is for identity boundaries,
not credential transport.

### Unsupported or invalid targets

Unsupported target kinds, missing required fields, ambiguous combinations, and
invalid `logicalSessionKeyID` hint formats return machine-readable validation
outcomes (`invalid_target_reference`, structured normalization errors) rather
than silently deriving a key.

## Resolving current runtime identity

`GET /factory-sessions/{session_id}/sync-preflight` is the primary resolution
surface for dashboard recovery. It accepts the persisted session selector plus
optional `backend_scope_id` and `logical_session_key_id` hints and returns:

- current `backendScopeID`, `logicalSessionKeyID`, `factorySessionID`,
  `streamGenerationID`,
- client-safe `normalizedTarget` metadata, and
- a structured `reasonCode` describing the outcome.

Resolution behavior:

| Outcome | Meaning |
| --- | --- |
| `ok` | The requested selector resolves to the current live session and stream generation; reconnect cursor may be reusable when preflight proves it still matches the stream. |
| `logical_session_remap` | The logical target still exists but the current `factorySessionID` (and usually `streamGenerationID`) differs from what the client persisted. The response points at the replacement session. |
| `cursor_stale` | The session and stream identity still match, but the persisted reconnect cursor no longer applies to the current stream generation. |
| `session_not_found` | No current live session matches the logical target within the scoped backend, or the backend scope hint does not match the active backend. |
| `invalid_target_reference` | The logical key or target reference is malformed or failed normalization. |

Wrong `backendScopeID` hints fail closed with `session_not_found` rather than
resolving across scopes.

Stream identity on session reads uses the same fields through
`FactorySessionStreamIdentity` on live session projections.

## Dashboard recovery: preserved vs dropped state

Before opening SSE with a persisted cursor, the dashboard calls sync preflight
with logical identity hints read from the persisted checkpoint.

`ui/src/features/dashboard/lib/preflight/resolve-dashboard-checkpoint-preflight.ts`
is the single production decision boundary for checkpoint reuse, logical
session remap, stale-cursor rejection, and non-recoverable recovery.
`ui/src/features/dashboard/hooks/preflight/use-dashboard-checkpoint-preflight.ts`
owns the guarded application of those decisions: checkpoint cleanup or
restore, selected-session remap, cache recovery, and publication of the
validated reconnect cursor. App event-stream work waits for that hook to
finish, so a reusable checkpoint is restored before a quiet stream opens and
the stream receives only the reconnect cursor approved by preflight.

### Preserved on logical remap

When identity resolves to a different `factorySessionID` or
`streamGenerationID`, the dashboard keeps **logical tab intent** and harmless
UI preferences that are not tied to the old stream, including:

- `backendScopeID` and `logicalSessionKeyID` for the tab's target,
- normalized target metadata returned by preflight,
- non-stream layout or navigation choices that do not assume the old event
  history.

### Dropped on `factorySessionID` or `streamGenerationID` change

When preflight reports `logical_session_remap`, `cursor_stale`, or a remap
detected by resolved UUID differing from the requested selector, the dashboard
clears stream-derived state for the old session:

- persisted reconnect cursor (`afterEventId` / `afterSequence`),
- IndexedDB timeline checkpoint for the stale stream identity,
- selected tick and replay-derived timeline state from the old stream,
- session-scoped React Query cache entries keyed to the superseded session or
  stream generation.

The tab reconnects against the resolved UUID `factorySessionID` and current
`streamGenerationID` without reusing stale cursors.

### Same-stream restore

When preflight returns `ok` with the same `factorySessionID` and
`streamGenerationID`, and `checkpointReusable` is true with a valid reconnect
cursor for that stream generation, the dashboard may restore the persisted
cursor and timeline checkpoint.

The existing sync-preflight response and reconnect-cursor fields are sufficient
for this recovery path, including a quiet stream with no new event. This
convergence does not add an OpenAPI field or explicit event-stream head marker.
Add a public head marker only if a separate quiet-stream experiment demonstrates
that the current contract cannot establish the required ordering or identity.

### Unresolved recovery

When preflight returns `session_not_found` or `invalid_target_reference`, the
dashboard surfaces an explicit recoverable stale-session state with an
accessible clean-replay action instead of entering an infinite reconnect loop.

## Verification surfaces

Use these focused surfaces when changing normalization, resolution, dashboard
remap, or default-alias behavior.

| Area | Primary packages / tests |
| --- | --- |
| Target normalization | `pkg/factorysessions/logicaltarget/normalize_test.go`, `derive_key_test.go`, `api_target_test.go` |
| Key derivation stability | `pkg/factorysessions/logicaltarget/derive_key_test.go`, `pkg/service/runtime_session_runtime_test.go` (logical key assertions) |
| API resolution / sync preflight | `pkg/service/runtime_session_runtime_test.go` (resolved, remapped, unresolved, wrong-scope, invalid-key, default alias cases), `pkg/api/contracttests/openapi_contract_surface_test.go` |
| Public contract fields | `api/components/schemas/api/FactorySessionLogicalTarget.yaml`, `FactorySessionStreamIdentity.yaml`, `FactorySessionSyncPreflight*.yaml`; regenerate with `make generate-api` |
| Dashboard preflight decisions | `ui/src/features/dashboard/lib/preflight/resolve-dashboard-checkpoint-preflight.test.ts`, `dashboard-session-sync-preflight.test.ts` |
| Guarded checkpoint restore, remap, and recovery | `ui/src/features/dashboard/hooks/preflight/use-dashboard-checkpoint-preflight.test.tsx`, `ui/src/features/dashboard/hooks/useDashboardSnapshot.test.tsx` |
| Quiet reload and App reconnect ordering | `ui/src/App.session-stream.test.tsx`, `ui/src/testing/quiet-checkpoint-reload-test-utils.test.ts` |
| Default alias to UUID runtime identity | `pkg/factorysessions/session_identity_test.go`, dashboard preflight tests proving SSE opens on resolved UUID not `~default` |
| Maintainer file map | `docs/internal/processes/api-relevant-files.md` (logical session and sync-preflight entries) |

Focused backend verification:

```bash
go test ./pkg/factorysessions/logicaltarget/... ./pkg/service/... -count=1
```

Focused frontend verification:

```bash
make ui-test
```

Broader gate when touching shared surfaces: `make verify-fast`.

## Related documentation

- `docs/architecture/data-model.md` — public Factory Session vocabulary
- `docs/architecture/architecture.md` — service/session boundaries and event stream
- `docs/internal/processes/api-relevant-files.md` — API and dashboard wiring map
