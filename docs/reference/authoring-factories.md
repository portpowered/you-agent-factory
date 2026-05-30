---
author: Agent Factory Team
last-modified: 2026-05-23
doc-id: agent-factory/authoring-factories
---

# Authoring Factories

Use this guide to create and run a current you-agent-factory workflow with the
public `factory.json` contract. Keep topology in `factory.json`, worker runtime
instructions in `workers/<name>/AGENTS.md`, and workstation prompts in
`workstations/<name>/AGENTS.md`.

Use this guide for workflow sequencing, runnable examples, and command order.
Use `you docs config` for the field-by-field `factory.json` topology reference,
`you docs work` for submitted-work contracts, [Workstations](workstations.md) for workstation
runtime fields, [Workers](workers.md) for worker backend fields, and
[Batch Inputs](batch-inputs.md) for the watched-file and API request shape.

## Recommended Layout

```text
factory/
  factory.json
  workers/
    executor/AGENTS.md
    reviewer/AGENTS.md
  workstations/
    execute-story/AGENTS.md
    review-story/AGENTS.md
  inputs/
    story/
      default/
```

`factory.json` owns the work graph: work types, states, workers, workstations,
resources, and routing. The split `AGENTS.md` files own prompt-heavy runtime
configuration that is easier to maintain outside JSON.

## Minimal Workflow

A minimal workflow needs one work type, one worker, and one workstation:

```json
{
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "processor" }
  ],
  "workstations": [
    {
      "name": "process-task",
      "worker": "processor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": { "workType": "task", "state": "failed" }
    }
  ]
}
```

At runtime:

1. A submitted `task` work item starts in `task:init`.
2. `process-task` is enabled when a token is present in that place.
3. Accepted work routes to `task:complete`.
4. Failed or timed-out work routes to `task:failed`.

Use `you docs config` for the canonical routing contract, including continue
and rejection routes.

## Build Your First Workflow

This walkthrough creates a two-stage execution and review loop with canonical
camelCase config fields.

### 1. Create `factory.json`

```json
{
  "id": "sample-service",
  "resources": [
    { "name": "agent-slot", "capacity": 1 }
  ],
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
      "onContinue": { "workType": "story", "state": "init" },
      "onFailure": { "workType": "story", "state": "failed" },
      "resources": [{ "name": "agent-slot", "capacity": 1 }]
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
      "name": "review-loop-breaker",
      "type": "LOGICAL_MOVE",
      "guards": [{ "type": "VISIT_COUNT", "workstation": "review-story", "maxVisits": 3 }],
      "inputs": [{ "workType": "story", "state": "init" }],
      "outputs": [{ "workType": "story", "state": "failed" }]
    }
  ]
}
```

This topology gives you one execution pass, one review pass, and an explicit
guarded loop breaker so a rejected story cannot cycle forever.

### Optional portability manifest

Add `supportingFiles` only when the workflow also needs declarative host-tool
checks or bundled helper files that should travel with the factory contract.
Use
`you docs config` (Portability Resource Manifest)
for the manifest fields and validation rules.

### 2. Create the split runtime definitions

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

You are a software engineer. Implement the requested story and run focused
verification before finishing.
```

`workers/reviewer/AGENTS.md`:

```yaml
---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
timeout: 30m
skipPermissions: true
---

You review the story implementation and return ACCEPTED only when the change is
ready.
```

`workstations/execute-story/AGENTS.md`:

```yaml
---
type: MODEL_WORKSTATION
limits:
  maxExecutionTime: 1h
---

Implement the story.

Story payload:
{{ (index .Inputs 0).Payload }}

Return CONTINUE when the story made ordinary partial progress but needs another
execution pass.
Return COMPLETE only when the story is ready to advance into review.
```

`workstations/review-story/AGENTS.md`:

```yaml
---
type: MODEL_WORKSTATION
limits:
  maxExecutionTime: 30m
---

Review the story implementation.

Story payload:
{{ (index .Inputs 0).Payload }}

Return ACCEPTED when the story is ready.
Return REJECTED with concrete feedback when another pass is needed.
```

### 3. Start the factory

For a portable `factory.json` and a single customer prompt, mark one work type
with `handlingBehavior: ["DEFAULT"]` in `factory.json` (see
`you docs config` (Default handling for one-shot CLI runs) and run:

```bash
you run --factory ./factory.json "Fix the lint issues"
```

The command resolves the factory root from the config file path, submits the
quoted prompt as raw text to the `DEFAULT` work type, and exits after batch idle
completion. You cannot combine `--factory` with `--dir` or `--work` on the same
invocation.

Use mock workers for the first routing check with the directory layout:

```bash
you run --dir ./factory --with-mock-workers
```

Or combine `--factory` with mock workers when testing a portable config:

```bash
you run --factory ./factory.json --with-mock-workers ./docs/examples/mock-workers.json "Fix the lint issues"
```

The command loads `factory.json`, resolves the split `AGENTS.md` files, starts
continuous mode, and exposes the dashboard and API on the configured port.

Live runs record a replay-compatible artifact by default. Use `--no-record`,
`--record <path>`, or `--replay <path>` when you need to override capture or
playback. Run `you docs record-replay` for generated paths, incompatible flag
combinations, sensitivity warnings, and copy-pasteable record and replay
examples.

Run `you docs mock-workers` for the `--with-mock-workers` JSON contract,
selection fields, and deterministic outcome examples beyond this quick start.

### 4. Submit work

Create a startup or watched-file request:

```json
{
  "requestId": "story-001",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "story-001",
      "workTypeName": "story",
      "payload": {
        "title": "Add review checklist"
      }
    }
  ]
}
```

Run it at startup:

```bash
you run --dir ./factory --with-mock-workers --work ./docs/examples/startup-work.json
```

Or drop the file under `factory/inputs/story/default/` while the factory is
already running.

The reusable startup work file
[`docs/examples/startup-work.json`](../examples/startup-work.json) uses the
same `FACTORY_REQUEST_BATCH` request shape with one `story` work item in the
`init` state and a concrete payload. The companion
[`docs/examples/README.md`](../examples/README.md) shows how to combine that
startup work, the mock-worker config, and replay commands with the checked-in
[`examples/write-code-review`](../../examples/write-code-review/factory.json)
factory.

## Author A Model-Operation TTS Factory

Use `MODEL_INVOKE` when the workstation should request a generic operation such
as `TTS` and let worker capability plus typed resources decide whether the
execution is local or cloud-backed.

### Shared workstation contract

This workstation stays the same for both local and cloud TTS:

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

### Local OMNIVOICE example

`factory.json`:

```json
{
  "workTypes": [
    {
      "name": "speech",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "resources": [
    {
      "name": "omnivoice-cache",
      "type": "MODEL",
      "capacity": 1,
      "model": "OMNIVOICE_Q4_K_M",
      "backend": "LLAMACPP",
      "loadPolicy": "ON_DEMAND"
    }
  ],
  "workers": [{ "name": "tts-worker" }],
  "workstations": [
    {
      "name": "speak",
      "type": "MODEL_INVOKE",
      "operation": "TTS",
      "worker": "tts-worker",
      "operationBindings": [
        {
          "slot": "text",
          "selector": { "type": "TEXT", "label": "utterance" }
        }
      ],
      "inputs": [{ "workType": "speech", "state": "init" }],
      "outputs": [{ "workType": "speech", "state": "complete" }],
      "onFailure": [{ "workType": "speech", "state": "failed" }]
    }
  ]
}
```

`workers/tts-worker/AGENTS.md`:

```yaml
---
type: MODEL_WORKER
model: OMNIVOICE_Q4_K_M
modelProvider: CODEX
modelLocality: LOCAL
resources:
  - name: omnivoice-cache
    capacity: 1
operations:
  - name: TTS
    inputs:
      - name: text
        required: true
        contentTypes:
          - TEXT
    outputs:
      - name: audio
        contentTypes:
          - AUDIO
---
Synthesize speech from the resolved utterance.
```

### Cloud-backed TTS example

Reuse the same workstation and change the resources plus worker:

```json
{
  "resources": [
    {
      "name": "cloud-tts-quota",
      "type": "PROVIDER_QUOTA",
      "capacity": 8,
      "provider": "CODEX",
      "model": "gpt-4o-mini-tts"
    },
    {
      "name": "cloud-tts-slot",
      "type": "INVOCATION_SLOT",
      "capacity": 2,
      "provider": "CODEX",
      "model": "gpt-4o-mini-tts"
    }
  ],
  "workers": [{ "name": "tts-worker" }]
}
```

```yaml
---
type: MODEL_WORKER
model: gpt-4o-mini-tts
modelProvider: CODEX
modelLocality: CLOUD
resources:
  - name: cloud-tts-quota
    capacity: 1
  - name: cloud-tts-slot
    capacity: 1
operations:
  - name: TTS
    inputs:
      - name: text
        required: true
        contentTypes:
          - TEXT
    outputs:
      - name: audio
        contentTypes:
          - AUDIO
---
Synthesize speech through the cloud-backed provider.
```

Compatibility stays stable because the workstation still asks for one `TTS`
operation with the same slot contract. Only the worker identity, locality, and
resource metadata change.

### Test And Inspect Without A Full Workflow

Use the `/models` surface while authoring:

```bash
you models list
you models inspect OMNIVOICE_Q4_K_M
you models pull OMNIVOICE_Q4_K_M
you models invoke OMNIVOICE_Q4_K_M --operation TTS --text "release notes" --output speech.wav
you models invoke OMNIVOICE_Q4_K_M --operation TTS --text "release notes" --json
```

Use the `--output` form when you want the streamed audio body written directly
to a file. Use `--json` when you want metadata plus canonical output content
references.

### Maintainer Validation

For real local OMNIVOICE coverage, run `make long-tests`. Set
`INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS=1`, ensure `omnivoice-llamacpp` is
installed, and optionally set `INFINITE_YOU_OMNIVOICE_COMMAND` or
`INFINITE_YOU_OMNIVOICE_CACHE_DIR` to reuse a custom backend or managed cache.

## Related Contract Detail

- `you docs config` owns work types, states,
  routing, resources, and portability fields.
- [Workstations](workstations.md) owns workstation kinds, runtime fields,
  route fields, and guards.
- [Workers](workers.md) owns worker types, backend fields, and worker
  `AGENTS.md` placement.
- [Author AGENTS.md](authoring-agents-md.md) owns split file shape, prompt
  placement, and authoring patterns.

## Failure Routing And Provider Behavior

For workflow design, add explicit failure, continue, and rejection destinations
to the topology so every outcome lands somewhere intentional. Use
`you docs config` for the
canonical routing contract, [Workstations](workstations.md) for route fields
and execution limits, and [Workers](workers.md) for worker backend behavior.

## When To Use Pollers

Use `POLLER` when the factory itself should own a long-lived ingress loop that
continuously creates ordinary submitted work from an external system.

Choose the workstation behavior this way:

- Use `STANDARD` for a normal dispatch stage.
- Use `REPEATER` when one work item should iterate until it is accepted or
  fails.
- Use `CRON` when service mode should create internal time-triggered work on a
  schedule.
- Use `POLLER` when service mode should keep an external integration alive,
  restart it with bounded backoff, and stop it cleanly on shutdown or
  replacement.

Choose the poller worker type this way:

- Use a `SCRIPT_WORKER` poller when you already have custom integration logic
  in a script.
- Use a `HOSTED_WORKER` poller when the repository already ships the provider
  integration, such as the built-in `LINEAR` poller.

Keep the exact contracts on the canonical owner pages:

- [Workstations](workstations.md) owns `behavior: "POLLER"` and lifecycle
  behavior.
- [Workers](workers.md) owns hosted `LINEAR` worker fields and `auth.secretRef`.
- [Batch Inputs](batch-inputs.md#poller-stdout-contract) owns the script
  poller stdout submission contract.

### Script Poller Example

`factory.json`:

```json
{
  "name": "github-intake",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "github-poller" }
  ],
  "workstations": [
    {
      "name": "github-intake",
      "behavior": "POLLER",
      "worker": "github-poller",
      "outputs": [{ "workType": "task", "state": "init" }],
      "onFailure": { "workType": "task", "state": "failed" }
    }
  ]
}
```

`workers/github-poller/AGENTS.md`:

```yaml
---
type: SCRIPT_WORKER
command: bash
args: ["scripts/poll-github.sh"]
timeout: 2m
---

Poll GitHub and emit one canonical batch payload on stdout per run.
```

### Hosted Linear Poller Example

`workers/linear-poller/AGENTS.md`:

```yaml
---
type: HOSTED_WORKER
provider: LINEAR
auth:
  secretRef: secrets/linear-api-key
linear:
  pollInterval: 2m
  teams: ["ENG"]
  states: ["unstarted", "started"]
  mapping:
    workType: task
    state: init
---

Repository-owned Linear poller.
```

Bound workstation:

```json
{
  "name": "linear-intake",
  "behavior": "POLLER",
  "worker": "linear-poller",
  "outputs": [{ "workType": "task", "state": "init" }],
  "onFailure": { "workType": "task", "state": "failed" }
}
```

V1 non-goals for poller authoring:

- Raw factory event emission from pollers.
- OAuth-based hosted auth flows.
- Advanced multi-instance poller coordination.

## Test Workflows With Mock Workers

Use mock workers when you want to verify routing, rejection loops, failure
paths, and script side effects without making live provider calls.

Run `you docs mock-workers` for the full JSON contract, selection fields,
`runType` values, and examples. For this review-loop walkthrough, start with:

```bash
you run --dir ./factory --with-mock-workers
```

To exercise the checked-in rejection example:

```bash
you run --dir ./factory --with-mock-workers ./docs/examples/mock-workers.json
```

Reusable inputs live under [`docs/examples/`](../examples/README.md), including
[`docs/examples/mock-workers.json`](../examples/mock-workers.json) and
[`docs/examples/startup-work.json`](../examples/startup-work.json). The
companion [`docs/examples/README.md`](../examples/README.md) shows how to combine
startup work, mock-worker config, and record/replay commands with the checked-in
[`examples/write-code-review`](../../examples/write-code-review/factory.json)
factory.

The checked-in
[`examples/write-code-review/factory.json`](../../examples/write-code-review/factory.json)
factory is a concrete starting point for adapting this command to a
review-loop workflow.

## Authoring Checklist

- Keep the public workflow contract in `factory.json`.
- Use camelCase factory-config fields such as `workTypes`, `resources`,
  `onFailure`, `onRejection`, and `maxVisits`.
- Use `supportingFiles` only for portability-only concerns such as
  validation-only PATH tools and explicitly bundled scripts or docs.
- Keep prompt-heavy worker and workstation runtime fields in split `AGENTS.md`
  files unless you intentionally need a single-file config.
- Add a guarded `LOGICAL_MOVE` workstation for repeater or review loops.
- Use [Batch Inputs](batch-inputs.md) for `FACTORY_REQUEST_BATCH`
  request files.
- Use [Workstations](workstations.md) for cron, prompt templates, timeouts, and
  workstation runtime field details.
- Use [Workers](workers.md) for worker backend field details.

## Related

- `you docs agents`
- [Mock workers](mock-workers.md)
- [Record and replay](record-replay.md)
- `you docs config`
- `you docs work`
- [Workstations](workstations.md)
- [Workers](workers.md)
- [Batch Inputs](batch-inputs.md)
- [Parent-Aware Fan-In](../internal/development/parent-aware-fan-in.md)
- [Workstation Guards And Guarded Loop Breakers](../internal/development/workstation-guards-and-guarded-loop-breakers.md)
- [Templates](templates.md)
- [README](../README.md)
