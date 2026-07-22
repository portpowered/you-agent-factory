---
author: Agent Factory Team
last-modified: 2026-07-13
doc-id: agent-factory/guides/javascript-workflows
---

# JavaScript Workflows

Use this guide to author or select a JavaScript workflow, validate it, start a
Factory Session, inspect its durable facts, and recover from supported failure
outcomes. A JavaScript orchestrator coordinates child work inside a Factory
Session. The script receives JSON-compatible inputs and a small host API; it
does not receive general-purpose machine access.

## Operator flow

### 1. Author or select source

Put reusable project workflows under `.claude/workflows/` and select them by
name. For a one-off source check, pass inline JavaScript directly:

```bash
curl -X POST http://localhost:7437/factories/preview \
  -H 'Content-Type: application/json' \
  -d '{"sourceKind":"INLINE_WORKFLOW","inlineSource":"phase(\"setup\");"}'
```

To validate a reusable project workflow named `release-train`, run from its
project root so ordered source lookup can find `.claude/workflows/`:

```bash
curl -X POST http://localhost:7437/factories/preview \
  -H 'Content-Type: application/json' \
  -d '{"sourceKind":"WORKFLOW_NAME","sourceValue":"release-train","projectRoot":"."}'
```

Validation resolves and checks source without creating a Factory Session.
Correct every blocking diagnostic before execution.

### 2. Start synchronously or asynchronously

Every start needs a stable request id and exactly one source selector. Supply
invocation data as a JSON object when the factory expects inputs.

Use `run` when the caller should wait for a terminal result or the configured
timeout. This copy-paste command exercises the published deterministic timeout
fixture and still returns its Factory Session id for inspection:

```bash
curl -X POST http://localhost:7437/factory-sessions/sync \
  -H 'Content-Type: application/json' \
  -d '{"requestId":"req-js-timeout-001","workflowName":"long-running-audit","args":{"scope":"release"},"waitTimeoutMillis":1000}'
```

Use `start` when the caller should receive the Factory Session id immediately
and poll it later:

```bash
curl -X POST http://localhost:7437/factory-sessions/async \
  -H 'Content-Type: application/json' \
  -d '{"requestId":"req-js-run-n-001","workflowName":"release-train","args":{"release":"2026.06"}}'
```

Retain the returned `sessionId`. JavaScript execution uses the same canonical
Factory Session API and MCP surfaces as other orchestrators.

These examples use the deterministic fake execution provider and published
fixture catalog selected by default. For live source resolution and JavaScript
execution, replace the fixture request/source values with your own and add
`--execution-provider javascript-runtime --project-root .`. Configure MCP with
runtime backing separately as described in `you docs mcp`.

### 3. Inspect the Factory Session

Use the exact returned id for every read. Status describes lifecycle and
progress; results expose final or partial Work output; dispatches describe
child work; artifacts are session-owned durable outputs; and events are the
ordered replay and reconnect history.

```bash
you --json workflow status SESSION_ID
you --json workflow result SESSION_ID --mode final
you --json workflow dispatches SESSION_ID
you --json workflow artifacts SESSION_ID
you --json workflow events SESSION_ID
```

For an in-progress Factory Session, use `result SESSION_ID --mode partial` and
poll `status` or `events` before retrying the final result. Use
`events SESSION_ID --after-event-id EVENT_ID` to continue after an acknowledged
event.

### 4. Recover

If validation fails, correct the source or selector and validate again. If a
child fails, inspect its Dispatch and the surrounding events before starting a
new request or using an explicitly supported resume path. If a result is
`NOT_READY`, keep the same Factory Session id and poll; do not create a new
request merely to read progress. The recovery table below maps stable signals
to the facts that remain inspectable.

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
| `agent.run` | `await agent.run({prompt, label?, preset?, modelProvider?, model?, reasoningEffort?})` | Requests one child execution and resolves to its structured result. Its resolved worker selection and lifecycle are inspectable as a `Dispatch` and related `FactoryEvent` records. Explicit settings remain subject to policy. |
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
Child requests may select a preset, model provider, model, and reasoning effort;
the host permits them only when effective policy allows them. Command, sandbox,
writable-root, network, concurrency, and output-schema fields are not supported
`agent.run` arguments. Use `agent.run` for host-mediated child work and
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
| Validate or resolve source without starting a session | — | `POST /factories/preview` | `you.factory_session.validate_source` |
| Start and wait | `you run --named FACTORY` for canonical named-Factory invocation | `POST /factory-sessions/sync` | `you.factory_session.start_sync` |
| Start for polling | — | `POST /factory-sessions/async` | `you.factory_session.start_async` |
| Read status or final/partial result | `you session show SESSION_ID` | `GET /factory-sessions/SESSION_ID`; `GET /factory-sessions/SESSION_ID/results` | `you.factory_session.get`; `you.factory_session.get_result` |
| Inspect child work and durable facts | `you session dispatches SESSION_ID` | `GET /factory-sessions/SESSION_ID/dispatches`; `artifacts`; `events` | `you.factory_session.list_dispatches`; `list_artifacts`; `read_events` |

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
source.

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
with `agent.run({preset: "careful-review", ...})`.

Worker fields resolve independently. Highest precedence is an explicit
`agent.run` field, then the preset selected by that call, then operator
`defaults.workerModelProvider` and `defaults.workerModel`. The selected preset
id remains observable even when an explicit child field overrides one preset
value. The completed selection is canonicalized and checked against effective
policy before a `Dispatch` is created.

Unknown presets are rejected with the preset id and its selection source.
Invalid preset definitions fail operator configuration load; policy-denied
resolved models or reasoning efforts fail before child dispatch.
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

## Stable failures and recovery

Treat validation diagnostics, result availability, and terminal failure facts
as the supported contract. Human-readable messages can add safe context, but
automation should prefer the stable code or status when one is listed. A
failure never makes raw provider transcripts, checkpoint bodies, or JavaScript
VM internals part of the inspection contract.

| Failure | Observable surface and stable signal | What remains inspectable | Recovery |
|---------|--------------------------------------|--------------------------|----------|
| Invalid source or primitive arguments | Validation/preview returns a `workflow.source.*` diagnostic such as `workflow.source.syntaxError` or `workflow.source.unsupportedPrimitive`; execution does not start. | No new `FactorySession`, `Dispatch`, `FactoryArtifact`, or `FactoryEvent`. | Correct the reported source location or primitive signature, then validate again. |
| Unsupported host access | Validation/preview returns `workflow.source.forbiddenHostAccess`. | No new session facts, because rejection precedes execution. | Replace filesystem, shell, process, import, or network access with a supported primitive or an explicitly permitted child request. |
| Unknown or policy-denied preset/child selection | Source resolution reports the unknown preset and selection source, or execution reports a bounded `policy denied` diagnostic before that child is dispatched. | The parent `FactorySession` and facts emitted before the denial remain inspectable; a rejected pre-dispatch child has no successful `Dispatch`. | Select a configured preset or change the requested child settings/effective operator policy. |
| Child execution failure | The child `Dispatch` is failed and the session result/status reports a failed or partial terminal outcome with safe failure detail. | The `FactorySession`, failed `Dispatch`, prior artifacts, and ordered events remain inspectable. | Inspect the dispatch failure class/detail, correct the child request or provider condition, and start or resume through a supported path. |
| Non-JSON checkpoint, artifact, log fields, or final value | Execution fails with a bounded `must be JSON-compatible` diagnostic. | The session, earlier dispatches/artifacts/events, and any approved checkpoint references remain inspectable; the rejected value is not promised as an artifact. | Convert the value to JSON data without functions, cycles, or host objects. |
| Result not ready | Result reads return `resultStatus: NOT_READY` with availability reason `RESULT_NOT_READY`; this is retryable while the session is running. | Status, partial result when present, dispatches, artifacts, and events remain inspectable. | Poll status/events and retry the result read after progress or a terminal transition. |
| Factory Session not found | CLI reports `SESSION_NOT_FOUND`; REST uses `NOT_FOUND`; MCP returns its Factory Session not-found error envelope. | Nothing is inspectable for that identifier. | Reuse the exact `sessionId` returned by start and confirm the same runtime/storage scope. |
| Recording flag conflict | `--record` with `--replay`, or `--no-record` with `--record`, is rejected before execution. | No new session or session-owned facts. | Choose one recording mode; see `you docs record-replay`. |
| Missing, malformed, or incompatible replay | Replay load fails before reconstruction; unsupported artifacts report `unsupported replay artifact schemaVersion`. | No reconstructed session is created from the rejected file; the file itself remains unchanged. | Use an artifact with the supported schema, regenerate it with a compatible `you` version, or run live to create a new recording. |

A successful final result has `resultStatus: FINAL`. A running session can be
`NOT_READY`; an interrupted or failed terminal session can expose `PARTIAL`,
`FAILED_WITH_PARTIAL`, `UNAVAILABLE`, or bounded failure detail depending on
what was durably emitted. Do not interpret partial data as a successful final.
Use the session lifecycle status together with result status and availability.

## Related topics

- `you docs orchestrators` — canonical Factory Session terminology
- `you docs sessions` — session discovery and inspection
- `you docs mcp` — fixture-backed and runtime-backed MCP host setup
- `you docs config` — worker preset and operator-default configuration
- `you docs record-replay` — recording and replay modes
