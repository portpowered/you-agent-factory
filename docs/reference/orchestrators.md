---
author: Agent Factory Team
last-modified: 2026-06-08
doc-id: agent-factory/guides/orchestrators
---

# Orchestrators and Factory Sessions

Use this guide when you need the canonical contract nouns for Petri factories,
JavaScript dynamic workflows, shared dispatches, artifacts, and factory events.
`FactorySession` is the canonical runtime object for every live orchestration.

For live session discovery and CLI routing, see `you docs sessions`. For
`factory.json` topology, see `you docs config`.

## Canonical Nouns

| Noun | Meaning |
|------|---------|
| `Factory` | Authored orchestration definition loaded by a live session. |
| `FactoryOrchestrator` | Authored orchestrator identity on a factory (`PETRI` or `JAVASCRIPT`). |
| `FactorySession` | Canonical runtime instance for one live orchestration on a host. |
| `Dispatch` | Shared execution request record for a Petri transition or JavaScript child task. |
| `FactoryArtifact` | Shared session-owned output record such as a result, finding, log, or checkpoint ref. |
| `FactoryEvent` | Canonical event-stream record for session lifecycle, dispatch lifecycle, marking, phase, checkpoint refs, and artifacts. |

## Accepted Dynamic Workflow Aliases

- **Dynamic workflow** is shorthand for a `JAVASCRIPT` orchestrator-backed factory.
- A dynamic workflow **run** is a live `FactorySession` with
  `runtime.orchestratorKind = JAVASCRIPT`.
- Do not introduce `DynamicWorkflowRun` as a separate canonical runtime noun in
  API, CLI, dashboard, or docs.

## FactoryOrchestrator

Every factory resolves to an orchestrator kind:

- `PETRI` — existing Petri graph factories. When no authored orchestrator block
  exists, compatibility defaulting projects `orchestrator.kind = PETRI`.
- `JAVASCRIPT` — JavaScript workflow factories with source identity, metadata,
  args schema, and default policy instead of required Petri graph fields.

## FactorySession Runtime Projections

`GET /factory-sessions` and `GET /factory-sessions/{session_id}` expose shared
runtime fields plus kind-specific projections:

| Shared runtime field | Meaning |
|----------------------|---------|
| `orchestratorKind` | `PETRI` or `JAVASCRIPT`. |
| `status` | Canonical session lifecycle status. |
| `progress` | Factory state, token categories, and in-flight dispatch counts. |
| `dispatches` | Shared dispatch projections across orchestrators. |
| `artifacts` | Shared artifact projections across orchestrators. |

Petri sessions add `runtime.petri.marking` and
`runtime.petri.enabledTransitions`. JavaScript sessions add phase, checkpoint
refs, script status, child dispatch counts, and result refs under
`runtime.javascript` without exposing raw VM checkpoint bodies.

## CLI Inspection

```bash
# List live factory sessions with orchestrator kind.
you session list

# Show one factory session projection.
you session show
you session show session-beta
you --json session show session-beta
```

Human `you session show` output uses `FactorySession` as the canonical runtime
noun. Dynamic workflow wording appears only as JavaScript shorthand.

## Dashboard Inspection

Add the **Factory session** dashboard widget to inspect orchestrator-aware
runtime state for the selected live session. Petri marking and topology remain
available through the workflow activity graph; JavaScript sessions show phase,
checkpoint refs, child dispatch counts, artifacts, warnings, and final or
partial result refs without raw checkpoint bodies.

## Related Topics

- `you docs mcp` — canonical `you mcp serve` install path and preview tool scope
- `you docs sessions` — session list, show, factory query, status API, and routing
- `you docs config` — `factory.json` topology and portability
- `you docs work` — submitted work and verification commands
