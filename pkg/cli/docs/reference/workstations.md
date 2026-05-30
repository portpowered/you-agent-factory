# Workstations Reference

Use this page when you need the current workstation authoring contract:
topology fields, scheduling kinds, runtime `type`, and outcome routing.

This is the canonical customer-facing guide for workstations. Keep workstation
kinds, route fields, runtime step behavior, prompt/runtime fields, and
workstation-scoped execution settings here. Keep worker backend fields in
[Workers](workers.md) and top-level `factory.json` work type and routing
context in `you docs config`.

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
`onFailure`. `CLASSIFIER_WORKSTATION` is the exception: it still uses a worker
and inputs, but successful routing goes through authored
`classificationRoutes` instead of normal success `outputs`.

The bound worker supplies the execution backend and shared system
instructions. The workstation supplies the step-specific prompt template,
execution limits, output schema, working directory, worktree path,
environment, and routing.

## Current Contract

- Use `behavior` for scheduling behavior: `STANDARD`, `REPEATER`, `CRON`, or
  `POLLER`.
- Use `type` for the runtime implementation: `MODEL_WORKSTATION`,
  `MODEL_INVOKE`, `CLASSIFIER_WORKSTATION`, or `LOGICAL_MOVE`.
- Use `worker` for the bound worker name. Omit it only for logical routing
  workstations such as `LOGICAL_MOVE`.
- Route accepted results through `outputs`, ordinary partial-progress results
  through `onContinue`, rejected results through `onRejection`, and failed or
  timed-out results through `onFailure`.
- Worker-backed workstations that omit `onFailure` still map an explicit
  failure lane to each emitted work type's `FAILED` state. This implicit
  expansion applies to `STANDARD`, `REPEATER`, `CRON`, `POLLER`,
  `MODEL_WORKSTATION`, `MODEL_INVOKE`, and `CLASSIFIER_WORKSTATION`.
  `LOGICAL_MOVE` stays explicit.
- `CLASSIFIER_WORKSTATION` returns one plain string label. Leading and trailing
  whitespace are trimmed before matching, matching stays exact and
  case-sensitive, and empty or non-string outputs fail instead of routing
  through success.
- Use workstation-level `guards` only for `VISIT_COUNT` gating. Use a guarded
  `LOGICAL_MOVE` workstation when you need an explicit loop-breaker route.

## `MODEL_INVOKE` Workstations

Use `MODEL_INVOKE` when a workstation should describe the requested behavior in
provider-agnostic terms such as `TTS`, `ASR`, `TRANSCRIBE`, `EMBED`, or
`CLASSIFY`.

```json
{
  "name": "speak",
  "type": "MODEL_INVOKE",
  "operation": "TTS",
  "worker": "tts-worker",
  "operationBindings": [
    {
      "slot": "text",
      "selector": {
        "label": "utterance",
        "type": "TEXT"
      }
    },
    {
      "slot": "voice",
      "defaultContent": [
        {
          "type": "JSON",
          "role": "voice",
          "json": { "name": "alloy" }
        }
      ]
    }
  ],
  "inputs": [{ "workType": "speech", "state": "init" }],
  "outputs": [{ "workType": "speech", "state": "complete" }],
  "onFailure": [{ "workType": "speech", "state": "failed" }]
}
```

Additional `MODEL_INVOKE` rules:

- `operation` must be uppercase.
- `worker` must reference a `MODEL_WORKER`.
- The worker must declare the same operation and a compatible input/output
  slot contract.
- Each `operationBindings[].slot` must match one declared worker input slot.
- Bindings resolve deterministically from matching runtime input, authored
  `config`, authored `defaultContent`, or omission for optional slots.

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
| `behavior` | No | Scheduling behavior. Use `STANDARD`, `REPEATER`, `CRON`, or `POLLER`. Defaults to `STANDARD`. |
| `type` | Runtime config | Runtime implementation. Use `MODEL_WORKSTATION` for worker dispatch, `CLASSIFIER_WORKSTATION` for single-label branch selection, or `LOGICAL_MOVE` for no-worker routing. |
| `worker` | Usually | Worker name from `workers[].name`. Required for model/script dispatch, cron workstations, and poller workstations. Omit only for logical routing workstations. |
| `inputs` | Usually | IO places that enable the workstation. Cron workstations may omit customer inputs but still consume internal time work. |
| `outputs` | Usually | IO places produced when the worker returns accepted. Cron workstations require at least one output. |
| `onContinue` | No | IO place produced when the worker reports ordinary partial progress and the work should iterate without being classified as rejection. |
| `onRejection` | No | IO place produced when the worker returns rejected. |
| `onFailure` | No | IO place produced when execution fails or times out. When omitted on a worker-backed workstation, config mapping adds explicit failure arcs to each emitted work type's `FAILED` state. |
| `resources` | No | Resource capacity consumed while the workstation runs. |
| `guards` | No | Workstation-level visit-count guards. Parent fan-in and same-name matching belong on per-input guards. |
| `cron` | Cron only | Trigger timing for `behavior: "CRON"`. |
| `operation` | `MODEL_INVOKE` only | Uppercase provider-agnostic operation such as `TTS`. |
| `operationBindings` | `MODEL_INVOKE` only | Deterministic slot bindings from runtime input content, static config content, defaults, or omission. |

## Implicit Failure Routing

Config mapping normalizes omitted `onFailure` routes for worker-backed
workstations into explicit Petri arcs. The runtime, topology projections, and
event-history structure all use that normalized graph.

- If a worker-backed workstation omits `onFailure`, each emitted work type gets
  a failure arc to its own `FAILED` place.
- Explicit authored `onFailure` routes win and are preserved unchanged.
- Accepted success routing stays explicit: use `outputs` for ordinary
  workstations and `classificationRoutes` for classifiers.
- Existing repeater rejection behavior stays explicit too; implicit failure
  routing does not add new rejection routes.
- `LOGICAL_MOVE` does not participate because it does not dispatch a worker and
  should keep every route authored explicitly.

Example:

```json
{
  "name": "poll-inbox",
  "behavior": "CRON",
  "type": "MODEL_WORKSTATION",
  "worker": "ingest",
  "cron": { "schedule": "*/5 * * * *" },
  "outputs": [{ "workType": "task", "state": "queued" }]
}
```

That config omits `onFailure`, but the mapped graph still includes an explicit
failure route equivalent to:

```json
{
  "onFailure": [{ "workType": "task", "state": "failed" }]
}
```

## `behavior` Versus `type`

`behavior` answers "when should this workstation run?"

- `STANDARD` is the default fire-once step.
- `REPEATER` re-runs after continue results until the work is accepted or
  fails.
- `CRON` runs on a schedule in service mode.
- `POLLER` keeps an external ingress loop active in service mode and submits
  new work through the bound worker.

`type` answers "what runtime implementation handles the step?"

- `MODEL_WORKSTATION` renders a prompt and dispatches to the bound worker.
- `MODEL_INVOKE` resolves slot bindings and dispatches one typed model
  operation to the bound worker.
- `CLASSIFIER_WORKSTATION` renders a prompt and expects one plain string label
  such as `approved`, `needs_review`, or `spam`.
- `LOGICAL_MOVE` moves tokens without invoking a worker.

Do not use `type` to express schedule semantics, and do not use `behavior` to
replace runtime implementation.

## Classifier Workstations

Use `CLASSIFIER_WORKSTATION` when one workstation should return exactly one
label and route through authored `classificationRoutes` instead of normal
success outputs:

```json
{
  "name": "triage",
  "type": "CLASSIFIER_WORKSTATION",
  "worker": "reviewer",
  "inputs": [{ "workType": "task", "state": "init" }],
  "classificationRoutes": [
    {
      "label": "approved",
      "outputs": [{ "workType": "task", "state": "complete" }]
    },
    {
      "label": "needs_review",
      "outputs": [{ "workType": "task", "state": "in-review" }]
    },
    {
      "label": "spam",
      "outputs": [{ "workType": "task", "state": "failed" }]
    }
  ],
  "onFailure": [{ "workType": "task", "state": "failed" }]
}
```

Use classifier routing when the workstation's job is "choose exactly one
authored branch label." Do not approximate that with normal `outputs`,
`onContinue`, or `onRejection`:

- Use `outputs` when accepted success always fans out to the same destinations.
- Use `onContinue` when the work made ordinary partial progress and should
  iterate again without being treated as a rejection.
- Use `onRejection` when the work was actually rejected or sent back.
- Use `CLASSIFIER_WORKSTATION` when success is one explicit label such as
  `approved`, `needs_changes`, or `spam` and that label alone decides which
  authored branch runs next.

Classifier authoring stays intentionally strict:

- `classificationRoutes` is required and must contain one or more entries.
- Every route label must be non-empty, unique, free of surrounding whitespace,
  and authored as plain text rather than JSON literal text such as
  `"approved"`, `123`, `true`, `null`, `{...}`, or `[...]`.
- Every route must declare one or more destination outputs.
- Classifier workstations must not also declare normal success `outputs`,
  `onContinue`, or `onRejection`.

The successful classifier contract is one plain string label. The runtime trims
surrounding whitespace before matching, preserves exact case-sensitive label
matching, and routes only the selected label's destinations. It never falls
back to a non-classification success path.

Invalid classifier results fail through the ordinary `FAILED` lane:

- Empty labels fail.
- Unknown labels fail.
- Non-string outputs fail.
- Parse failures, execution errors, and timeouts fail.

When `onFailure` is configured, those failures use it exactly as other
workstation failures do. When `onFailure` is omitted, classifier failures still
follow the same implicit failed-state routing as other worker-backed
workstations. Successful classifier dispatches preserve the selected label in
runtime evidence, replay, and projections; failed classifier attempts keep
ordinary failure details and do not invent a selected label.

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

When the step should ask for a generic model capability instead of rendering a
prompt-oriented workstation body, prefer `MODEL_INVOKE` plus
`operationBindings` over encoding provider-specific slot names in the submitted
payload.

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
| `type` | Runtime implementation. Use `MODEL_WORKSTATION` for prompt-rendered worker dispatch, `CLASSIFIER_WORKSTATION` for prompt-rendered single-label routing, or `LOGICAL_MOVE` for no-worker pass-through routing. |
| `runner` | Stable runner override for this workstation. Supported built-in IDs are `codex`, `gemini`, `kiro`, `cursor-cli`, and `opencode`. |
| `openCodeAgent` | Optional OpenCode agent profile override; when set, replaces the bound worker default for `opencode` dispatches. |
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

Runner selection resolves in this order: workstation `runner`, then factory
`runner`, then legacy worker `modelProvider`, then the default `codex`
runner. Selecting a built-in runner expects the corresponding local CLI and
auth/setup to already be available before execution starts.

`openCodeAgent` resolves workstation override first, then the bound worker
default, and applies only when the resolved runner is `opencode`. List agent
names with `opencode agent list` ([OpenCode CLI](https://opencode.ai/docs/cli/)).
See [`docs/reference/workstations.md`](../../../docs/reference/workstations.md)
for examples and factory-build validation.

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

See [Guards](guards.md) for guard types, attachment levels, and guarded
`LOGICAL_MOVE` loop breakers, and [Relationships](relationships.md) for
`PARENT_CHILD` batch relations that enable parent-aware input guards.

## Related

- [CLI reference landing page](README.md)
- [Package docs index](../README.md)
- [Workers reference](workers.md)
- `you docs config`
- `you docs work`
- [Templates](templates.md)
- [Guards](guards.md)
- [Relationships](relationships.md)
