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
| `agent.run` | `await agent.run({prompt, label?, model?, reasoningEffort?, command?, sandbox?, writableRoots?, allowNetwork?, concurrency?, outputSchema?})` | Requests one child execution and resolves to its structured result. Its lifecycle is inspectable as a `Dispatch` and related `FactoryEvent` records. Explicit settings remain subject to policy. |
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

## Inspection model

These examples execute as JavaScript-backed `FactorySession` resources—not as
a separate workflow-run resource. Inspect the session result, child `Dispatch`
records, session-owned `FactoryArtifact` summaries, and ordered `FactoryEvent`
history through normal Factory Session surfaces. These observations are not a
promise to expose raw JavaScript engine state or provider transcripts.

## Related topics

- `you docs orchestrators` — canonical Factory Session terminology
- `you docs sessions` — session discovery and inspection
- `you docs record-replay` — recording and replay modes
