---
author: Agent Factory Team
last-modified: 2026-07-12
doc-id: agent-factory/guides/javascript-workflows
---

# JavaScript Workflow Authoring

Use a JavaScript orchestrator when authored code should coordinate child work
inside a `FactorySession`. The script receives JSON-compatible inputs and a
small host API; it does not receive general-purpose machine access.

## Inputs and final output

- `args` is the JSON-compatible value supplied for this invocation.
- `meta` is host-resolved workflow metadata. Common fields include `name` and
  `description`; authored metadata can add other JSON-compatible fields.
- A top-level `return` supplies the successful final value.
- `workflow.final(value)` also supplies the final value and takes precedence
  when the script both calls it and returns another value.
- Final values must be JSON-compatible. A successful final is projected as the
  `FactorySession` result and a session-owned result `FactoryArtifact`, with
  lifecycle and artifact observations represented by `FactoryEvent` records.

## Supported globals and primitives

| Primitive | Supported signature | Observable outcome |
|-----------|---------------------|--------------------|
| `phase` | `phase(name: string): void` | Appends an ordered phase observation for the `FactorySession` and its `FactoryEvent` stream. |
| `log` | `log(message: string, fields?: object): void` | Appends a structured log observation. Fields must be JSON-compatible. |
| `workflow.log` | `workflow.log(message: string, fields?: object): void` | Same observable log contract as `log`. |
| `agent.run` | `await agent.run({prompt, label?, agentId?, preset?, modelProvider?, model?, reasoningEffort?, command?, sandbox?, writableRoots?, allowNetwork?, concurrency?, outputSchema?})` | Requests one child execution and resolves to its structured result. Its resolved worker selection and lifecycle are inspectable as a `Dispatch` and related `FactoryEvent` records. Explicit settings remain subject to policy. |
| `parallel` | `await parallel(items)` where each item is an `agent.run` request object or async function | Runs bounded child work and returns results in input order. Child work remains individually inspectable as `Dispatch` records. |
| `pipeline` | `await pipeline(items, worker, next?)` | Runs `worker(item, index)` and optional `next(workerResult, item, index)` for each item. Returns ordered per-item status and stage results. |
| `workflow.checkpoint` | `workflow.checkpoint({label: string, state?: object}): void` | Persists JSON-compatible application state as a checkpoint artifact/reference and appends a checkpoint observation. It does not snapshot the JavaScript VM. |
| `workflow.resumeState` | `workflow.resumeState(): object \| undefined` | Returns approved checkpoint state on a resume path and `undefined` on a fresh start. |
| `workflow.budget` | `workflow.budget(): object` | Returns the effective policy budget and appends a budget observation. It accepts no arguments. |
| `workflow.artifact` | `workflow.artifact({kind: string, label: string, content?: JSON, visibility?: string}): string` | Creates a session-owned `FactoryArtifact` and returns its stable `you-artifact://` reference. |
| `workflow.final` | `workflow.final(value: JSON): void` | Selects the final result and takes precedence over a top-level return. |

Promises returned by `agent.run`, `parallel`, and `pipeline` must be awaited or
otherwise resolved before the workflow completes.

## Host capability boundary

Workflow source is validated before execution and runs with only the globals
listed above plus ordinary JavaScript language facilities. Direct filesystem,
shell, process, module `import`/`require`, and network access is unavailable.
Child requests may ask for scoped capabilities such as a model, command,
writable roots, or network, but the host permits them only when effective
policy explicitly allows them. Use `agent.run` for host-mediated child work and
`workflow.artifact` for durable outputs.

## Runnable examples

The repository ships these executable examples under
`docs/examples/javascript-workflows/` and exercises those same files in
focused runtime tests.

### Simple final result

`simple-final.workflow.js` reads `args`, reports a phase, and returns JSON. The
terminal `FactorySession` exposes the result through its result
`FactoryArtifact`; phase, artifact, and lifecycle facts appear as
`FactoryEvent` records.

```javascript
phase("compose");
return { greeting: args.greeting, subject: args.subject };
```

### Ordered fan-out with synthesis

`ordered-fanout.workflow.js` calls `parallel` with reviewer requests, then
runs a synthesizer through `agent.run`. Each
reviewer and synthesizer is a `Dispatch`; completion timing does not change the
returned reviewer order, and the synthesis becomes the `FactorySession` result.

```javascript
const reviews = await parallel([
  { label: "review-alpha", prompt: "Review alpha" },
  { label: "review-beta", prompt: "Review beta" },
]);
const synthesis = await agent.run({
  label: "synthesize",
  prompt: "Synthesize the completed reviews",
});
return { reviews, synthesis };
```

### Checkpoint and resume

`checkpoint-resume.workflow.js` branches on `workflow.resumeState()`. A fresh
execution writes JSON application state; a resumed execution receives only
that approved state. The checkpoint reference is a `FactoryArtifact`, its
write is a `FactoryEvent`, and child work remains visible as `Dispatch`
records. Raw VM state is not captured.

## Equivalent CLI, API, and MCP execution

CLI, REST, and MCP are adapters over the same durable Factory Session execution
contract. Pick one `requestId` as the idempotency key, retain the returned
`sessionId`, and use that same stable Factory Session identifier for every
later status, result, dispatch, artifact, and event read. A synchronous start
waits for a terminal outcome (or its configured timeout); an asynchronous start
returns the same session concept immediately for polling. Terminal status and
result availability have the same meaning on every surface.

| Operation | CLI | REST API | MCP |
|-----------|-----|----------|-----|
| Validate or resolve source without starting a session | `you workflow validate --workflow review` | `POST /factories/preview` | `you.factory_session.validate_source` |
| Start and wait | `you --json workflow run --request-id req-review-1 --workflow review` | `POST /factory-sessions/sync` | `you.factory_session.start_sync` |
| Start for polling | `you --json workflow start --request-id req-review-1 --workflow review` | `POST /factory-sessions/async` | `you.factory_session.start_async` |
| Read status or final/partial result | `you --json workflow status SESSION_ID`; `you --json workflow result SESSION_ID` | `GET /factory-sessions/SESSION_ID`; `GET /factory-sessions/SESSION_ID/results` | `you.factory_session.get`; `you.factory_session.get_result` |
| Inspect child work and durable facts | `you --json workflow dispatches SESSION_ID`; `artifacts`; `events` | `GET /factory-sessions/SESSION_ID/dispatches`; `artifacts`; `events` | `you.factory_session.list_dispatches`; `list_artifacts`; `read_events` |

Start requests use the shared `FactorySessionExecutionRequest` shape: one source
selector, JSON-compatible `args`, requested policy where applicable, and the
stable request id. Responses expose the same `FactorySession`, result
availability, and inspection links rather than transport-specific workflow-run
resources. A completed result is the same session result whether read from the
sync response or fetched later; running sessions can report a not-ready final
result while their status, partial result, dispatches, artifacts, and events
remain inspectable.

`you mcp serve` is fixture-backed by default for deterministic offline contract
scenarios. Use `you mcp serve --runtime` for live JavaScript execution. Both
modes expose the same `you.factory_session.*` tool envelopes, but fixture-backed
calls return catalog scenarios while runtime-backed calls execute resolved
source. Workflow-named MCP tools such as `you.workflow.run`,
`you.workflow.status`, and `you.workflow.result` are compatibility aliases;
new integrations should use `you.factory_session.*`.

## Child worker presets

Reusable JavaScript child settings are operator-owned in
`~/.you-agent-factory/config.json`:

```json
{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  },
  "workerPresets": [
    {
      "id": "careful-review",
      "modelProvider": "codex",
      "model": "gpt-5-codex",
      "reasoningEffort": "high"
    }
  ]
}
```

Each preset supports a required, unique, non-empty `id` and required
`modelProvider`; `model` and `reasoningEffort` are optional. Supported reasoning
efforts are `minimal`, `low`, `medium`, and `high`. Reference a preset directly
with `agent.run({preset: "careful-review", ...})`, or assign it to a named
factory agent under `orchestrator.javascript.agents.<agentId>.preset` and call
`agent.run({agentId: "reviewer", ...})`.

Worker fields resolve independently. Highest precedence is an explicit
`agent.run` field, then the preset selected directly by that call, then the
named factory agent's preset, then that preset's fields, then operator
`defaults.workerModelProvider` and `defaults.workerModel`. The selected preset
id remains observable even when an explicit child field overrides one preset
value. The completed selection is canonicalized and checked against effective
policy before a `Dispatch` is created.

Unknown direct or named-agent presets are rejected with the preset id and its
selection source. Invalid preset definitions fail operator configuration load;
policy-denied resolved models or reasoning efforts fail before child dispatch.
Successful dispatch list/detail, event, and replay projections retain the
resolved preset id, provider, model, reasoning effort, runner, execution
provider, and provider-session reference when available. See `you docs config`
for the complete operator configuration contract.

## Inspection model

These examples execute as JavaScript-backed `FactorySession` resources—not as
a separate workflow-run resource. Inspect the session result, child `Dispatch`
records, session-owned `FactoryArtifact` summaries, and ordered `FactoryEvent`
history through normal Factory Session surfaces. These observations are not a
promise to expose raw JavaScript engine state or provider transcripts.

## Related topics

- `you docs orchestrators` — canonical Factory Session terminology
- `you docs sessions` — session discovery and inspection
- `you docs mcp-hosts` — fixture-backed and runtime-backed MCP setup
- `you docs config` — worker preset and operator-default configuration
- `you docs record-replay` — recording and replay modes
