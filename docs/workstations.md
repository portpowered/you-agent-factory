author: Agent Factory Team
---
last-modified: 2026-04-21
doc-id: agent-factory/workstations
---

# Workstations And Workers

Workstations are the dispatch steps in `factory.json`. Workers are the runtime
executors that workstations invoke. This guide explains how to configure
workstation topology, scheduling kinds, prompt/runtime fields, worker
definitions, cron triggers, and parent-aware fan-in.

Use [Factory JSON And Work Configuration](work.md) for the top-level
`factory.json` guide.

## Recommended Layout

Keep topology in `factory.json`, worker system instructions in
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
recorded configs. If a config embeds runtime definitions inline, keep the
bundle complete: every referenced worker and workstation must either have
inline runtime fields or a matching split `AGENTS.md` file on disk.

## Workstation Topology

A workstation entry wires input places to output places and names the worker to
execute:

```json
{
  "name": "review-story",
  "behavior": "STANDARD",
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
| `worker` | Usually | Worker name from `workers[].name`. Required for model/script dispatch and cron workstations. |
| `inputs` | Usually | IO places that enable the workstation. |
| `outputs` | Usually | IO places produced when the worker returns accepted. |
| `onContinue` | No | IO place produced when the worker reports ordinary partial progress and the work should iterate without being classified as rejection. |
| `onRejection` | No | IO place produced when the worker returns rejected. |
| `onFailure` | Recommended | IO place produced when execution fails or times out. |
| `resources` | No | Resource capacity consumed while the workstation runs. |
| `guards` | No | Workstation-level visit-count guard. |
| `CRON` | Cron only | Schedule for `behavior: "CRON"`. |

Use `behavior` for scheduling behavior. Use `type` only for the runtime
implementation, such as `MODEL_WORKSTATION` or `LOGICAL_MOVE`.

## Workstation Kinds

| Kind | Behavior |
|------|----------|
| `STANDARD` | Default fire-once scheduling. Inputs are consumed, the worker runs, and output routing follows the worker outcome. |
| `REPEATER` | Re-fires after continue results. Use for agent loops that should keep working until accepted or failed, while reserving rejection for true negative outcomes. |
| `CRON` | Creates internal time work in service mode and dispatches when the schedule and any configured inputs are ready. |

### Standard Kind

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

Omitting `behavior` has the same runtime behavior as `"behavior": "STANDARD"`.

### Repeater Kind

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

See [Workstation Guards And Guarded Loop Breakers](guides/workstation-guards-and-guarded-loop-breakers.md)
for the full comparison between a workstation-level guard and a guarded
loop-breaker route.

### Cron Kind

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

The `CRON` object supports:

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

## Runtime Workstation Fields

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

### Script-Backed Portability

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

### Automat-Inspired Portability Smoke

The canonical bounded portability smoke lives in
`tests/functional_test/testdata/automat_portability_smoke/`. It models one
realistic pre-dispatch `translate/automat` slice without productizing the full
workflow.

The smoke proves that the portable contract preserves and restores these
representative bundle assets:

- `scripts/prepare-automat-slice.ps1`
- `scripts/verify-external-tools.ps1`
- `docs/portable-workflow.md`
- `portable-dependencies.json`

The flattened portable form keeps those files in
`resourceManifest.bundledFiles` under canonical `factory/...` targets. Expand
then restores them into the runnable factory-relative layout:

- `factory/scripts/**` restores to `scripts/**`
- `factory/docs/**` restores to `docs/**`
- `factory/portable-dependencies.json` restores to `portable-dependencies.json`

The fixture intentionally keeps `mangaka.exe` and `magick` external. They must
stay declared in `resourceManifest.requiredTools`. The bundled
`portable-dependencies.json` mirrors that contract for the bounded fixture
scripts, and neither tool may appear as a bundled binary in the portable
output.

This smoke stops at bounded dispatch readiness. It proves that the expanded
layout can read the restored scripts, docs, and dependency contract through the
runtime command boundary, but it does not prove full translation, OCR, or image
processing output correctness.

Legacy workstation `timeout`, singular stop aliases, and retired workstation
resource aliases are migration-only inputs. Canonical docs, `config expand`,
`config flatten`, replay artifacts, and startup serialization emit
`limits.maxExecutionTime`, `stopWords`, and shared `resources[{name,capacity}]`.

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

## Worker AGENTS.md

A model worker `AGENTS.md` defines the system prompt plus the distinct
model-routing and executor-adapter settings:

```yaml
---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
timeout: 1h
skipPermissions: true
---

You are a software engineer. Follow the workstation instructions and keep
changes scoped to the requested work item.
```

A script worker runs a command:

```yaml
---
type: SCRIPT_WORKER
command: go
args: ["test", "./..."]
timeout: 10m
---

Runs the Go test suite.
```

Worker fields:

| Field | Applies to | Description |
|-------|------------|-------------|
| `type` | All | `MODEL_WORKER` or `SCRIPT_WORKER`. |
| `model` | Model | Provider model name. |
| `modelProvider` | Model | Model-provider identifier used for model routing and provider diagnostics. Built-in values are `CLAUDE` and `CODEX`. |
| `executorProvider` | Model | Executor adapter identifier used to select the worker execution wrapper. This is distinct from `modelProvider`. |
| `command` | Script | Executable name. |
| `args` | Script | Arguments. Values support template rendering. |
| `timeout` | All | Execution timeout. |
| `stopToken` | Model | Output marker used to classify accepted completion. |
| `skipPermissions` | Model | Provider-specific permission shortcut. |
| `resources` | All | Worker-level resource labels for provider/runtime integrations. |

The canonical source of truth for worker-contract values is the `Worker` schema
in [`api/openapi.yaml`](../api/openapi.yaml). Current built-in
`modelProvider` values are `CLAUDE` and `CODEX`, and the current public
`executorProvider` value is `SCRIPT_WRAP`.

## Templates

Workstation prompts, `workingDirectory`, `worktree`, `env` values, and script
worker `args` can use Go template syntax.

Common variables:

| Variable | Description |
|----------|-------------|
| `{{ (index .Inputs 0).WorkID }}` | Work ID of the first non-resource input token. |
| `{{ (index .Inputs 0).WorkTypeID }}` | Work type of the input token. |
| `{{ (index .Inputs 0).Payload }}` | Submitted payload as text. |
| `{{ index (index .Inputs 0).Tags "branch" }}` | Tag value from a submitted work item. |
| `{{ .Context.Project }}` | Explicit project context, first work-input `project` tag, or `default-project`. |
| `{{ .Context.WorkDir }}` | Current execution working directory. |
| `{{ .Context.ArtifactDir }}` | Artifact directory. |
| `{{ .Context.Env }}` | Context environment map. |

In JSON strings, escape quotes inside templates:

```json
{
  "workingDirectory": "{{ index (index .Inputs 0).Tags \"worktree\" }}",
  "env": {
    "BRANCH": "{{ index (index .Inputs 0).Tags \"branch\" }}"
  }
}
```

In Markdown `AGENTS.md`, use normal quotes:

```text
Branch: {{ index (index .Inputs 0).Tags "branch" }}
```

## Parent-Aware Fan-In

Use per-input guards when one parent work item spawns children and a later
workstation should wait for those children. Keep parent-aware fan-in on
`workstations[].inputs[].guards[]`; the old workstation-level `join` field is
retired.

See [Parent-Aware Fan-In](guides/parent-aware-fan-in.md) for the full
authoring guide, including `ALL_CHILDREN_COMPLETE`, `ANY_CHILD_FAILED`,
`parentInput`, and `spawnedBy`.

## Same-Name Input Guards

Use per-input guards for same-name joins when one workstation should consume
two normal inputs only if their authored work names match exactly. Keep this on
`workstations[].inputs[].guards[]`, attach `type: "SAME_NAME"` to one input,
and set `matchInput` to the peer input's `workType` name on the same
workstation.

If the names differ, the referenced input is missing, or either token does not
have a usable authored work name, the workstation stays disabled.

See [Workstation Guards And Guarded Loop Breakers](guides/workstation-guards-and-guarded-loop-breakers.md)
for the representative plan-item/task-item example and the comparison against
workstation-level guards.

## Workstation-Level Guards

Workstation-level guards support `VISIT_COUNT` and `MATCHES_FIELDS`. They gate
whether a workstation may fire; they do not create a failure or terminal route.
Prefer a guarded `LOGICAL_MOVE` workstation for common loop-breaking routes
because it states the source and target places explicitly.

See [Workstation Guards And Guarded Loop Breakers](guides/workstation-guards-and-guarded-loop-breakers.md)
for the full comparison.

## Complete Example

`factory.json`:

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
  "resources": [
    { "name": "agent-slot", "capacity": 2 }
  ],
  "workers": [
    { "name": "executor" },
    { "name": "reviewer" },
    { "name": "loop-breaker" }
  ],
  "workstations": [
    {
      "name": "execute-story",
      "behavior": "REPEATER",
      "worker": "executor",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "in-review" }],
      "onFailure": { "workType": "story", "state": "failed" },
      "resources": [{ "name": "agent-slot", "capacity": 1 }],
      "workingDirectory": "{{ index (index .Inputs 0).Tags \"worktree\" }}",
      "worktree": ".worktrees/{{ index (index .Inputs 0).Tags \"branch\" }}/{{ (index .Inputs 0).WorkID }}",
      "env": {
        "AGENT_FACTORY_BRANCH": "{{ index (index .Inputs 0).Tags \"branch\" }}",
        "AGENT_FACTORY_WORK_ID": "{{ (index .Inputs 0).WorkID }}"
      }
    },
    {
      "name": "review-story",
      "worker": "reviewer",
      "inputs": [{ "workType": "story", "state": "in-review" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "onRejection": { "workType": "story", "state": "init" },
      "onFailure": { "workType": "story", "state": "failed" },
      "resources": [{ "name": "agent-slot", "capacity": 1 }]
    },
    {
      "name": "executor-loop-breaker",
      "type": "LOGICAL_MOVE",
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "failed" }],
      "guards": [{ "type": "VISIT_COUNT", "workstation": "execute-story", "maxVisits": 50 }]
    }
  ]
}
```

`workstations/execute-story/AGENTS.md`:

```yaml
---
type: MODEL_WORKSTATION
limits:
  maxExecutionTime: 1h
stopWords:
  - "<result>ACCEPTED</result>"
---

Implement the story.

Story payload:
{{ (index .Inputs 0).Payload }}

Work ID: {{ (index .Inputs 0).WorkID }}
Branch: {{ index (index .Inputs 0).Tags "branch" }}
```

`workers/executor/AGENTS.md`:

```yaml
---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
timeout: 1h
skipPermissions: true
---

You are the implementation worker. Make the requested change, run focused
verification, and report the result.
```

## Authoring Checklist

- `behavior` is one of `STANDARD`, `REPEATER`, or `CRON`.
- Runtime workstation `type` is `MODEL_WORKSTATION` or `LOGICAL_MOVE`.
- Worker `type` is `MODEL_WORKER` or `SCRIPT_WORKER`.
- Every non-logical workstation names a declared worker.
- Every IO route references a real work type and state.
- Repeater loops have a guarded `LOGICAL_MOVE` loop breaker.
- Cron workstations have `worker`, `cron.schedule`, and `outputs`.
- Same-name matching uses per-input `guards` plus `matchInput`, not
  workstation-level `guards`.
- Parent fan-in uses per-input `guards`, not workstation-level `join`.
- JSON templates escape quotes; Markdown templates do not.
- New configs do not use retired `runtime_type`, `cron.interval`, `join`, or `worktree_cleanup` fields.

## Related

- [Factory JSON And Work Configuration](work.md)
- [Batch Inputs](guides/batch-inputs.md)
- [Parent-Aware Fan-In](guides/parent-aware-fan-in.md)
- [Workstation Guards And Guarded Loop Breakers](guides/workstation-guards-and-guarded-loop-breakers.md)
- [Prompt Template Variables](prompt-variables.md)
- [Author AGENTS.md](authoring-agents-md.md)
