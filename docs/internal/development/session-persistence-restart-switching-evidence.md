# Session Persistence Restart and Switching Evidence

Manual scenario evidence for `session-persistence-hardening-and-observability-004`.
Recorded commands, backend identity values, observed dashboard reconnect behavior, and
network outcomes. No provider account names, tokens, prompts, or workspace secrets appear
in this evidence.

## Verification Command

From the repository root:

```bash
cd ui && npx vitest run integration/dashboard-session-recovery-manual-scenarios.integration.test.mjs --no-file-parallelism --maxWorkers 1
```

Companion automated recovery proofs from earlier stories:

```bash
cd ui && npx vitest run integration/dashboard-session-recovery.integration.test.mjs --no-file-parallelism --maxWorkers 1
```

## Harness Setup

- Dashboard preview: `startBrowserPreview()` serves `http://127.0.0.1:<previewPort>/dashboard/ui/`
- Mock API: `startFactoryApiServer({ apiPort: preview.apiPort })` on the paired preview API port
- IndexedDB store: `agentFactoryTimelineCheckpoints` / `checkpoints`
- Default logical session selector: `~default`
- Sanitized identity fields observed in network responses:
  - `backendScopeID` (derived from `folderPath::browser-integration` in the harness)
  - `factorySessionID`
  - `streamGenerationID` (runtime lifecycle `startedAt` / version physical timestamp)

## Scenario 1: Backend Restart With Preserved History

**Goal:** A backend restart with preserved durable history continues only when identity and
cursor validity still match.

| Field | Value |
| --- | --- |
| Command | Seed matching checkpoint, load dashboard, reload twice (simulated restart) |
| `backendScopeID` | `/replay/factory::browser-integration` |
| `factorySessionID` | `~default` |
| `streamGenerationID` | `2026-05-19T00:00:00Z` |
| Checkpoint cursor | `after_event_id=manual-restart-event-5`, `after_sequence=5` |

**Observed outcome:**

- First reload opens `GET /factory-sessions/~default/events?after_event_id=manual-restart-event-5&after_sequence=5`
- Second reload (simulated restart) reuses the same cursor parameters
- `GET /factory-sessions/~default` succeeds before stream open
- Dashboard does not surface offline or recovery chrome

## Scenario 2: Clean Restart / Cleared Stream Generation

**Goal:** A clean backend restart drops stream-derived dashboard state and reconnects without
stale timeline cursor data.

| Field | Value |
| --- | --- |
| Command | Seed stale-generation checkpoint, reload against current server identity |
| Stored `streamGenerationID` | `2026-01-01T00:00:00Z` (stale) |
| Current `streamGenerationID` | `2026-05-19T00:00:00Z` |
| Stale cursor | `after_event_id=stale-clean-restart-event-11` |

**Observed outcome:**

- Event stream reconnect omits `after_event_id` and `after_sequence`
- Stale cursor `stale-clean-restart-event-11` is not sent
- Dashboard opens a fresh replay from current server history instead of hydrating the stale tick

## Scenario 3: Logical Session Remap (`~default` → Resolved UUID)

**Goal:** A remapped logical session resolves to a new `factorySessionID` without reusing an
old cursor when stream identity no longer matches.

| Field | Value |
| --- | --- |
| Command | Seed checkpoint for prior resolved UUID, reload dashboard on `~default` |
| Stored `factorySessionID` | `019e0000-0000-7000-8000-000000000042` (prior live id) |
| Current `factorySessionID` | `~default` |
| Stale cursor | `after_event_id=remap-stale-event-3` |

**Observed outcome:**

- IndexedDB checkpoint for the prior UUID identity is not reused for the current `~default` stream identity
- Event stream opens at `/factory-sessions/~default/events` with no reconnect cursor
- Remap stale cursor `remap-stale-event-3` is not present in captured stream URLs

## Scenario 4: Backend Scope Switch (Local → Cloud)

**Goal:** Switching backend scope never sends a cursor, query cache, or session detail from the
previous scope.

| Field | Value |
| --- | --- |
| Command | Seed local-scope checkpoint, reload against cloud-scope server session |
| Prior `backendScopeID` | `/local/factory::browser-integration` |
| Current `backendScopeID` | `/cloud/factory::browser-integration` |
| Stale cursor | `after_event_id=local-scope-event-8` |

**Observed outcome:**

- Event stream reconnect omits reconnect cursor query parameters
- Local-scope cursor `local-scope-event-8` is not sent after the scope switch
- Dashboard loads against the cloud-scope `GET /factory-sessions/~default` identity

## Scenario 5: Multi-Tab Isolation With Shared Checkpoint Invalidation

**Goal:** Two tabs keep independent reload behavior while shared stream checkpoints are
invalidated when stream identity changes.

| Field | Value |
| --- | --- |
| Command | Tab 1 + Tab 2 same browser context; shared IndexedDB; identity change between reloads |
| Shared matching identity | `/replay/factory::browser-integration` / `~default` / `2026-05-19T00:00:00Z` |
| Invalidated identity | same scope + session, `streamGenerationID=2026-01-15T00:00:00Z` |

**Observed outcome:**

- Both tabs initially reuse shared checkpoint cursor `multi-tab-event-4`
- After seeding a stale-generation checkpoint and reloading both tabs:
  - Tab 1 reconnects without `after_event_id` / `after_sequence`
  - Tab 2 reconnects without `after_event_id` / `after_sequence`
- Stale shared cursor `multi-tab-stale-event-9` is not sent from either tab

## Latest Evidence

Date: `2026-07-01` (UTC)

- `cd ui && npx vitest run integration/dashboard-session-recovery-manual-scenarios.integration.test.mjs --no-file-parallelism --maxWorkers 1` — records the five restart/switching scenarios above with captured `EventSource` URLs and sanitized identity fields only.
