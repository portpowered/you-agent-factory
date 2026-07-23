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

Implementation lives in `pkg/services/factory_sessions/internal/logicaltarget/`; the Factory
Session coordinator applies it when projecting live-session identities.

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

## Browser checkpoint ownership and durability

The backend Factory Event stream is the authoritative history for Factory
Session replay, lifecycle, Work, and terminal outcomes. The browser does not
create a second canonical Factory Session store. Its reconnect cursors,
timeline entries, materialized Work outcomes, and IndexedDB checkpoints are
derived caches that can always be rejected and rebuilt from backend-proven
identity and event history.

### Exact cache identity and isolation

Every timeline entry, reconnect cursor, materialized outcome accumulator, and
persisted checkpoint belongs to this exact four-part identity, in this order:

1. normalized backend scope (`backendScopeID`),
2. concrete resolved Factory Session UUID (`factorySessionID`),
3. logical session key (`logicalSessionKeyID`), and
4. stream generation (`streamGenerationID`).

All four values must be present and match before state can be restored or
continued. Supported selectors such as `~default` are input conveniences only:
preflight or another identity-bearing boundary resolves them to a concrete
UUID, and the alias is never written as durable checkpoint identity. Timeline
and materialized outcome state is held per exact identity, so switching Factory
Sessions or generations cannot reuse another entry's cursor, samples, or
accumulator. Retained materialized samples and compact text are bounded before
persistence; the checkpoint is a resume optimization, not an unbounded event
or Work-content archive.

### Preflight, hydration, and stale recovery

Sync preflight completes before checkpoint hydration or SSE connection. Only a
complete, reusable identity-and-cursor result can authorize hydration. A
rejected identity, remap, unsupported checkpoint, or stale cursor is discarded
for the affected identity. Stale-cursor recovery then opens the resolved
session stream without a cursor and rebuilds from canonical events; it does not
silently hydrate first and validate later.

### Durable replacement and lifecycle handoff

Checkpoint writes for an exact identity are serialized and admitted
monotonically. IndexedDB replacement reads the current record and conditionally
writes the newer checkpoint in one transaction; the replacement becomes
committed state only after that transaction completes. Concurrent browser
contexts follow the same durable ordering rule. An older or equal write cannot
replace newer committed progress, and an aborted, rejected, or failed
replacement leaves the last committed checkpoint intact.

Normal debounced persistence is supplemented by best-effort flush and handoff
when the active stream changes, the application unmounts, the page is hidden,
or `pagehide` fires. These lifecycle paths contain persistence failures and do
not make cancellation the sole cleanup or durability mechanism. A later stream
or generation remains authoritative even when lifecycle writes overlap.

### Bounded diagnostics and lazy Dispatch inspection

Session-persistence diagnostics are process-local, bounded outcome records.
They may retain an approved outcome, recovery action, mismatch class, and
one-way correlation token. They must not retain raw Factory events, Work
content, raw identities, checkpoint payloads, or unbounded console/network
history.

Ordinary Factory Session list/detail identity, timeline, and checkpoint state
remain free of raw Dispatch collections. Dispatch summaries and detail are
loaded lazily through the dedicated Factory Session Dispatch inspection reads
when a user opens that view; they are not added to the checkpoint to make
recovery appear complete.

## Verification surfaces

Use these focused surfaces when changing normalization, resolution, dashboard
remap, or default-alias behavior.

| Area | Primary packages / tests |
| --- | --- |
| Target normalization | `pkg/services/factory_sessions/internal/logicaltarget/normalize_test.go`, `pkg/services/factory_sessions/internal/logicaltarget/derive_key_test.go`, `pkg/services/factory_sessions/internal/logicaltarget/api_target_test.go` |
| Key derivation stability | `pkg/services/factory_sessions/internal/logicaltarget/derive_key_test.go`, `pkg/services/factory_sessions/internal/controlplane/read_test.go` |
| API resolution / sync preflight | `pkg/services/factory_sessions/internal/controlplane/sync_preflight_test.go`, `pkg/transports/http/contracttests/openapi_contract_surface_test.go` |
| Public contract fields | `api/components/schemas/api/FactorySessionLogicalTarget.yaml`, `api/components/schemas/api/FactorySessionStreamIdentity.yaml`, `api/components/schemas/api/FactorySessionSyncPreflightResponse.yaml`; regenerate with `make generate-api` |
| Dashboard preflight decisions | `ui/src/features/dashboard/lib/preflight/resolve-dashboard-checkpoint-preflight.test.ts`, `ui/src/features/dashboard/lib/preflight/dashboard-session-sync-preflight.test.ts` |
| Guarded checkpoint restore, remap, and recovery | `ui/src/features/dashboard/hooks/preflight/use-dashboard-checkpoint-preflight.test.tsx`, `ui/src/features/dashboard/hooks/useDashboardSnapshot.test.tsx` |
| Quiet reload and App reconnect ordering | `ui/src/App.session-stream.test.tsx`, `ui/src/testing/quiet-checkpoint-reload-test-utils.test.ts` |
| Four-part checkpoint identity and per-session timeline ownership | `ui/src/features/timeline/lib/stream-derived-cache-identity.test.ts`, `ui/src/features/timeline/state/checkpoint-persistence/identity/timelineCheckpointPersistence.identity.test.ts`, `ui/src/features/timeline/state/factoryTimelineStore.entries.test.ts` |
| Durable replacement, shared-context ordering, and failure preservation | `ui/src/features/timeline/state/checkpoint-persistence/ordering/timelineCheckpointPersistence.ordered-writes.test.ts`, `ui/src/features/timeline/state/checkpoint-persistence/ordering/timelineCheckpointPersistence.shared-contexts.test.ts`, `ui/src/features/timeline/state/checkpoint-persistence/timelineCheckpointPersistence.replacement-failure.test.ts` |
| Lifecycle flush and handoff | `ui/src/App.multi-session-checkpoint-debounce.test.tsx` |
| Bounded, redaction-safe diagnostics | `ui/src/features/dashboard/lib/session-persistence/diagnostics.test.ts` |
| Default alias to UUID runtime identity | `pkg/services/factory_sessions/registry_test.go`, dashboard preflight tests proving SSE opens on resolved UUID not `~default` |
| Maintainer file map | `docs/internal/processes/api-relevant-files.md` (logical session and sync-preflight entries) |

Focused backend verification:

```bash
go test ./pkg/services/factory_sessions/internal/logicaltarget/... ./pkg/services/factory_sessions/internal/controlplane ./pkg/services/factory_sessions/internal/runtimebinding -count=1
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
