# Workstations Reference

Use this page when you need the current workstation authoring contract:
topology fields, scheduling kinds, runtime `type`, and outcome routing.

This is the canonical customer-facing guide for workstations. Keep workstation
kinds, route fields, runtime step behavior, prompt/runtime fields, and
workstation-scoped execution settings here. Keep worker backend fields in
[Workers](workers.md) and top-level `factory.json` work type and routing
context in [Factory JSON and work configuration](work.md).

## Split Layout And Ownership

Keep workflow topology in `factory.json`, worker system instructions in
`workers/<name>/AGENTS.md`, and workstation prompts in
`workstations/<name>/AGENTS.md`:

```text
factory/
  factory.json
  workers/
    executor/AGENTS.md
    reviewer/AGENTS.md
  workstations/
    execute-story/AGENTS.md
    review-story/AGENTS.md
  inputs/story/default/
```

Inline runtime fields are also supported in `factory.json` for single-file or
recorded configs. When a config embeds runtime definitions inline, keep the
bundle complete: every referenced worker and workstation must either have
inline runtime fields or a matching split `AGENTS.md` file on disk.

`factory.json` declares the workflow topology. Each model or script
workstation names a worker through `worker`, consumes one or more input places,
and routes outcomes through `outputs`, `onContinue`, `onRejection`, or
`onFailure`.

The bound worker supplies the execution backend and shared system
instructions. The workstation supplies the step-specific prompt template,
execution limits, output schema, working directory, worktree path,
environment, and routing.

## Current Contract

- Use `behavior` for scheduling behavior: `STANDARD`, `REPEATER`, or `CRON`.
- Use `type` for the runtime implementation: `MODEL_WORKSTATION` or
  `LOGICAL_MOVE`.
- Use `worker` for the bound worker name. Omit it only for logical routing
  workstations such as `LOGICAL_MOVE`.
- Route accepted results through `outputs`, ordinary partial-progress results
  through `onContinue`, rejected results through `onRejection`, and failed or
  timed-out results through `onFailure`.
- Use workstation-level `guards` only for `VISIT_COUNT` gating. Use a guarded
  `LOGICAL_MOVE` workstation when you need an explicit loop-breaker route.

## Topology Fields

A workstation entry wires input places to output places and names the worker to
execute:

```json
{
  "name": "review-story",
  "behavior": "STANDARD",
  "type": "MODEL_WORKSTATION",
  "worker": "reviewer",
  "inputs": [{ "workType": "story", "state": "in-review" }],
  "outputs": [{ "workType": "story", "state": "complete" }],
  "onRejection": { "workType": "story", "state": "init" },
  "onFailure": { "workType": "story", "state": "failed" }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Stable workstation name. This is also the transition ID in runtime events. |
| `behavior` | No | Scheduling behavior. Use `STANDARD`, `REPEATER`, or `CRON`. Defaults to `STANDARD`. |
| `type` | Runtime config | Runtime implementation. Use `MODEL_WORKSTATION` for worker dispatch or `LOGICAL_MOVE` for no-worker routing. |
| `worker` | Usually | Worker name from `workers[].name`. Required for model/script dispatch and cron workstations. Omit only for logical routing workstations. |
| `inputs` | Usually | IO places that enable the workstation. Cron workstations may omit customer inputs but still consume internal time work. |
| `outputs` | Usually | IO places produced when the worker returns accepted. Cron workstations require at least one output. |
| `onContinue` | No | IO place produced when the worker reports ordinary partial progress and the work should iterate without being classified as rejection. |
| `onRejection` | No | IO place produced when the worker returns rejected. |
| `onFailure` | Recommended | IO place produced when execution fails or times out. |
| `resources` | No | Resource capacity consumed while the workstation runs. |
| `guards` | No | Workstation-level visit-count guards. Parent fan-in and same-name matching belong on per-input guards. |
| `cron` | Cron only | Trigger timing for `behavior: "CRON"`. |

## `behavior` Versus `type`

`behavior` answers "when should this workstation run?"

- `STANDARD` is the default fire-once step.
- `REPEATER` re-runs after continue results until the work is accepted or
  fails.
- `CRON` runs on a schedule in service mode.

`type` answers "what runtime implementation handles the step?"

- `MODEL_WORKSTATION` renders a prompt and dispatches to the bound worker.
- `LOGICAL_MOVE` moves tokens without invoking a worker.

Do not use `type` to express schedule semantics, and do not use `behavior` to
replace runtime implementation.

## Minimal Standard Step

```json
{
  "name": "review-story",
  "behavior": "STANDARD",
  "type": "MODEL_WORKSTATION",
  "worker": "reviewer",
  "inputs": [{ "workType": "story", "state": "in-review" }],
  "outputs": [{ "workType": "story", "state": "complete" }],
  "onRejection": { "workType": "story", "state": "init" },
  "onFailure": { "workType": "story", "state": "failed" }
}
```

For a basic workflow step:

- `outputs` handles accepted completion.
- `onContinue` handles ordinary "keep iterating" routing when configured.
- `onRejection` handles true negative outcomes or review send-back.
- `onFailure` handles execution failure or timeout.

Use `REPEATER` when continue should keep the same workstation active instead of
routing to a different review state. Pair long-running review loops with a
guarded `LOGICAL_MOVE` loop breaker so repeated true rejection has an explicit
terminal path.

## When To Use Each Kind

- Use `STANDARD` for normal pipeline stages.
- Use `REPEATER` for iterative agent loops.
- Use `CRON` only when the step should submit scheduled time work in service
  mode; keep the schedule under `cron.schedule`.

### Standard Workstations

Use `STANDARD` for normal pipeline stages:

```json
{
  "name": "process",
  "behavior": "STANDARD",
  "worker": "processor",
  "inputs": [{ "workType": "task", "state": "init" }],
  "outputs": [{ "workType": "task", "state": "complete" }],
  "onFailure": { "workType": "task", "state": "failed" }
}
```

Omitting `behavior` has the same runtime behavior as
`"behavior": "STANDARD"`.

### Repeater Workstations

Use `REPEATER` when ordinary partial progress should continue iterating without
being treated as rejection:

```json
{
  "name": "execute-story",
  "behavior": "REPEATER",
  "worker": "executor",
  "inputs": [{ "workType": "story", "state": "init" }],
  "outputs": [{ "workType": "story", "state": "in-review" }],
  "onContinue": { "workType": "story", "state": "init" },
  "onFailure": { "workType": "story", "state": "failed" }
}
```

For execution-review loops, keep `onContinue` for "another executor pass is
needed" and reserve `onRejection` for true negative business or review results.

Pair repeaters with a guarded loop-breaker workstation:

```json
{
  "name": "executor-loop-breaker",
  "type": "LOGICAL_MOVE",
  "guards": [{ "type": "VISIT_COUNT", "workstation": "execute-story", "maxVisits": 50 }],
  "inputs": [{ "workType": "story", "state": "init" }],
  "outputs": [{ "workType": "story", "state": "failed" }]
}
```

## Minimal Workflow Shape

This abbreviated topology shows the relationship between workstation routing
and worker binding without restating each detailed field contract:

```json
{
  "workTypes": [
    {
      "name": "story",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "in-review", "type": "PROCESSING" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "executor" },
    { "name": "reviewer" }
  ],
  "workstations": [
    {
      "name": "execute-story",
      "behavior": "REPEATER",
      "worker": "executor",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "in-review" }],
      "onFailure": { "workType": "story", "state": "failed" }
    },
    {
      "name": "review-story",
      "worker": "reviewer",
      "inputs": [{ "workType": "story", "state": "in-review" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "onRejection": { "workType": "story", "state": "init" },
      "onFailure": { "workType": "story", "state": "failed" }
    }
  ]
}
```

With a split layout, `workers/executor/AGENTS.md` owns the executor backend
and system prompt, while `workstations/execute-story/AGENTS.md` owns the
step-specific prompt and execution settings. Use [Workers](workers.md) for the
worker contract and this page for the workstation contract.

## Cron Kind

### Cron Workstations

Use `CRON` when a workstation should run on a schedule while the factory is in
service mode:

```json
{
  "name": "daily-refresh",
  "behavior": "CRON",
  "worker": "refresh-worker",
  "cron": {
    "schedule": "*/5 * * * *",
    "jitter": "30s",
    "expiryWindow": "2m"
  },
  "outputs": [{ "workType": "refresh", "state": "ready" }]
}
```

Cron workstations require:

- `behavior: "CRON"`
- a `worker`
- a `cron.schedule`
- at least one `outputs` entry

The `cron` object supports:

| Field | Required | Description |
|-------|----------|-------------|
| `schedule` | Yes | Standard five-field cron expression such as `"*/5 * * * *"`. |
| `triggerAtStart` | No | When `true`, service startup submits one immediate time token and keeps the schedule active. |
| `jitter` | No | Non-negative Go duration. The runtime adds deterministic jitter up to this value. |
| `expiryWindow` | No | Positive Go duration after `due_at` before stale time work expires. |

Cron workstations create internal `__system_time` work. Public `/work`,
`/status`, and normal dashboard queue projections hide that internal time work,
while canonical events retain it for replay and diagnostics.

Do not use `cron.interval`; it is retired. Use `cron.schedule`.

## Runtime Fields

These fields can live inline on `workstations[]` or in the workstation
`AGENTS.md` frontmatter:

| Field | Description |
|-------|-------------|
| `type` | Runtime implementation. Use `MODEL_WORKSTATION` for prompt-rendered worker dispatch or `LOGICAL_MOVE` for no-worker pass-through routing. |
| `promptFile` | Path relative to the workstation directory. The file content becomes the prompt template. |
| `promptTemplate` | Inline prompt template. Usually generated by config flattening; split `AGENTS.md` body is easier to author by hand. |
| `outputSchema` | JSON schema string used to validate model output when configured. |
| `limits.maxExecutionTime` | Execution timeout such as `30m` or `1h`. Legacy top-level `timeout` is accepted only as a migration alias and normalized here. |
| `limits.maxRetries` | Per-workstation retry/failure limit used by the circuit breaker. |
| `stopWords` | Ordered stop markers for accept-or-fail output handling. When configured, matching output is accepted and missing markers follow the failure path. |
| `workingDirectory` | Go template resolved at dispatch time and passed as execution working directory. |
| `worktree` | Go template resolved at dispatch time and passed as CLI provider worktree path. |
| `env` | Environment variables passed to script or provider execution. Values are templates. |
| `copyReferencedScripts` | `config expand` portability flag. When `true`, expand copies supported relative script references from the bound `SCRIPT_WORKER` into the expanded layout. Omitted means `false`. |
| `body` | Inline markdown body used as prompt template when no prompt file or explicit prompt template is supplied. |

Do not author new configs with `runtime_type`; use `type`. Do not rely on
`worktree_cleanup`; that stale field is not part of the current public
workstation config.

## Workstation AGENTS.md

Use a workstation `AGENTS.md` for prompt-heavy model workstations:

```yaml
---
type: MODEL_WORKSTATION
limits:
  maxExecutionTime: 30m
stopWords:
  - "<result>ACCEPTED</result>"
---

Review the story implementation.

Story: {{ (index .Inputs 0).Payload }}
Work ID: {{ (index .Inputs 0).WorkID }}
Branch: {{ index (index .Inputs 0).Tags "branch" }}

Return ACCEPTED when the story is ready.
If the story is not ready, explain the issues without emitting the stop word so the failure path or retry policy can handle the next attempt.
```

Use `promptFile` when the prompt should live outside `AGENTS.md`:

```yaml
---
type: MODEL_WORKSTATION
promptFile: prompts/review.md
limits:
  maxExecutionTime: 20m
stopWords:
  - "<result>ACCEPTED</result>"
---

This body is ignored when `promptFile` is set.
```

For a logical transition that moves tokens without a model or script worker:

```yaml
---
type: LOGICAL_MOVE
---

No prompt is rendered for LOGICAL_MOVE.
```

When a workstation has no `type`, the runtime defaults to
`MODEL_WORKSTATION` if it has a worker and `LOGICAL_MOVE` if it has no worker.
Author the `type` explicitly when the distinction matters.

## Script-Backed Portability

When a workstation binds to a `SCRIPT_WORKER`, you can keep the workstation
definition inline in `factory.json` and still use the portability commands.
This is the supported contract for factories that do not want a split
`workstations/<name>/AGENTS.md` file.

Use `type: "MODEL_WORKSTATION"` as the minimal explicit inline runtime field
when the workstation is otherwise just topology plus execution context. That
inline runtime field is what makes `config flatten` preserve a standalone
workstation definition instead of failing as an incomplete split layout.

Set `copyReferencedScripts: true` on the workstation only when the expanded
layout should include the referenced script files. The current expand path
copies only supported relative paths rooted in the authored factory bundle:

- a relative `SCRIPT_WORKER.command`
- the first non-flag script argument for interpreter commands such as
  `python`, `powershell`, `bash`, `node`, or `bun`

Absolute paths and escaping `..` paths are rejected. Expand does not rewrite
them into a portable location.

Example:

```json
{
  "workers": [
    {
      "name": "workspace-setup",
      "type": "SCRIPT_WORKER",
      "command": "powershell",
      "args": ["-File", "scripts/setup-workspace.ps1"]
    }
  ],
  "workstations": [
    {
      "name": "setup-workspace",
      "type": "MODEL_WORKSTATION",
      "worker": "workspace-setup",
      "copyReferencedScripts": true,
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }]
    }
  ]
}
```

With this authored shape, `config flatten` succeeds without a split workstation
`AGENTS.md`, and `config expand` copies `scripts/setup-workspace.ps1` only when
`copyReferencedScripts` stays `true`.

## Template Fields

Workstation prompts, `workingDirectory`, `worktree`, and `env` values can use
Go template syntax. Use [Templates](templates.md) for the shared template
surface, complete variable inventory, and JSON-versus-Markdown quoting rules.

## Guards And Fan-In

Workstation-level guards support `VISIT_COUNT` and `MATCHES_FIELDS`. They gate
whether a workstation may fire; they do not create a failure or terminal route.
Prefer a guarded `LOGICAL_MOVE` workstation for common loop-breaking routes
because it states the source and target places explicitly.

Use per-input guards for same-name joins and parent-aware fan-in. Keep
same-name joins on `workstations[].inputs[].guards[]` with
`type: "SAME_NAME"` and `matchInput`; keep parent fan-in on per-input guards
such as `ALL_CHILDREN_COMPLETE` or `ANY_CHILD_FAILED`.

See [Workstation Guards And Guarded Loop Breakers](../internal/development/workstation-guards-and-guarded-loop-breakers.md)
and [Parent-Aware Fan-In](../internal/development/parent-aware-fan-in.md) for
the full guard authoring guides.

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Workers reference](workers.md)
- [Factory JSON and work configuration](work.md)
- [Templates](templates.md)
- [Workstation guards and guarded loop breakers](../internal/development/workstation-guards-and-guarded-loop-breakers.md)
- [Parent-aware fan-in](../internal/development/parent-aware-fan-in.md)
