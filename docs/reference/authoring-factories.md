---
author: Agent Factory Team
last-modified: 2026-08-21
doc-id: agent-factory/authoring-factories
---

# Authoring Factories

Use this guide to create and run a current you-agent-factory workflow with the
public `factory.json` contract. Keep topology in `factory.json`, worker runtime
instructions in `workers/<name>/AGENTS.md`, and workstation prompts in
`workstations/<name>/AGENTS.md`.

Use this guide for workflow sequencing, runnable examples, and command order.
Use `you docs config` for the field-by-field
`factory.json` reference, `you docs workstations` for workstation
runtime fields, `you docs workers` for worker backend fields, and
`you docs batch-inputs` for the watched-file and API request shape.
Use `you docs factory-validation` for the required pre-run gate, its complete
static checks, and the worked unsupported-join failure. This page keeps the
end-to-end order without duplicating that validation reference.

For keeping a real pipeline alive across idle periods or recovering after a
process restart, use `you docs operations`.

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
  "name": "minimal-workflow",
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
      "onFailure": [{ "workType": "task", "state": "failed" }]
    }
  ]
}
```

At runtime:

1. A submitted `task` work item starts in `task:init`.
2. `process-task` is enabled when a token is present in that place.
3. Accepted work routes to `task:complete`.
4. Failed or timed-out work routes to `task:failed`.

Use `you docs config` for the
canonical routing contract, including continue and rejection routes.

## Declare Expected Artifacts

Work Types and Workstations can declare files that a dispatch is expected to
produce. The `pattern` is relative to the dispatch workspace and may be a
literal path, a glob, or a Go template. The replayable template vocabulary is
limited to `.Inputs` with the stable fields `Name`, `WorkID`, `WorkTypeID`,
`DataType`, `TraceID`, `ParentID`, `Project`, `Tags`, and `Payload`, plus
`.Context.Project` and `.Context.SessionID`. These fields are captured at
dispatch creation, so completion verification and historical Work reads use
the same values. Prompt-only fields such as `Relations`, `Content`,
`PreviousOutput`, `RejectionFeedback`, and `History` are not part of the
artifact contract. Host paths, environment variables, and Factory
documentation are intentionally not available to expected-artifact templates.
This complete JSON example can be saved as
`factory.json` and checked
with `you factory config validate ./factory.json`:

```json
{
  "name": "expected-artifacts",
  "workTypes": [
    {
      "name": "task",
      "handlingBehavior": ["DEFAULT"],
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ],
      "expectedArtifacts": [
        {
          "name": "task-report",
          "pattern": "reports/{{ (index .Inputs 0).Name }}.json",
          "nonEmpty": true
        }
      ]
    }
  ],
  "workers": [{ "name": "processor" }],
  "workstations": [
    {
      "name": "process-task",
      "worker": "processor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{ "workType": "task", "state": "failed" }],
      "expectedArtifacts": [
        {
          "name": "manifest",
          "pattern": "reports/{{ (index .Inputs 0).Name }}.manifest.json"
        }
      ]
    }
  ]
}
```

The same declarations are valid in `factory.yaml`:

```yaml
workTypes:
  - name: task
    expectedArtifacts:
      - name: task-report
        pattern: 'reports/{{ (index .Inputs 0).Name }}.json'
        nonEmpty: true
workstations:
  - name: process-task
    expectedArtifacts:
      - name: manifest
        pattern: 'reports/{{ (index .Inputs 0).Name }}.manifest.json'
```

Work Type declarations are inherited first, followed by Workstation
declarations. Exact duplicates are removed while preserving the first authored
position. `nonEmpty: true` requires every regular file matched by the pattern
to contain data. Empty names or patterns, invalid templates or globs, absolute
paths, paths containing `..`, or unsupported template fields such as
`.Inputs[0].Relations`, `.Inputs[0].History`, `.Context.WorkDir`,
`.Context.ArtifactDir`, `.Context.Env`, and `.Docs` are rejected with the
owning Work Type or Workstation in the validation diagnostic.
Omitting `expectedArtifacts` keeps the legacy behavior unchanged.

## Build Your First Workflow

This walkthrough creates a two-stage execution and review loop with canonical
camelCase config fields.

### 1. Create `factory.json`

```json
{
  "name": "sample-service",
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
      "onContinue": [{ "workType": "story", "state": "init" }],
      "onFailure": [{ "workType": "story", "state": "failed" }],
      "resources": [{ "name": "agent-slot", "capacity": 1 }]
    },
    {
      "name": "review-story",
      "worker": "reviewer",
      "inputs": [{ "workType": "story", "state": "in-review" }],
      "outputs": [{ "workType": "story", "state": "complete" }],
      "onRejection": [{ "workType": "story", "state": "init" }],
      "onFailure": [{ "workType": "story", "state": "failed" }],
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
`you docs config`
for the manifest fields and validation rules.

### 2. Create the split runtime definitions

`workers/executor/AGENTS.md`:

```yaml
---
type: AGENT_WORKER
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
type: AGENT_WORKER
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
type: AGENT_RUN
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
type: AGENT_RUN
limits:
  maxExecutionTime: 30m
---

Review the story implementation.

Story payload:
{{ (index .Inputs 0).Payload }}

Return ACCEPTED when the story is ready.
Return REJECTED with concrete feedback when another pass is needed.
```

### 3. Validate before the first run

Run the validate-only gate against the same authored file or directory that
the next run will use. Validation must pass after the Factory is authored and
immediately before its first execution. It does not start a Factory Session,
invoke a provider, or persist a named Factory.

For a portable file and the split directory layout, use the corresponding
source path:

```bash
# Portable JSON or YAML file.
you factory config validate ./factory.json

# Split Factory directory containing exactly one factory.json, factory.yaml,
# or factory.yml root.
you factory config validate ./factory
```

Against the current checked-in Factory, both forms print this current-binary
result:

```text
Factory validation passed.
Runtime taxonomy:
  worker processor: AGENT_WORKER
  worker workspace-setup: SCRIPT_WORKER
  worker planner: AGENT_WORKER
  worker ideafier: AGENT_WORKER
  workstation ideafy: AGENT_RUN (worker=ideafier)
  workstation plan: AGENT_RUN (worker=planner)
  workstation consume: LOGICAL_MOVE (worker=)
  workstation setup-workspace: SCRIPT_RUN (worker=workspace-setup)
  workstation process: AGENT_RUN (worker=processor)
  workstation review: AGENT_RUN (worker=planner)
  workstation executor-loop-breaker: LOGICAL_MOVE (worker=)
  workstation review-loop-breaker: LOGICAL_MOVE (worker=)
  workstation though-retrigger: LOGICAL_MOVE (worker=)
```

If validation fails, do not run or persist the Factory. Correct every blocking
finding and repeat the same command until it passes. For the field-level
checks, supported join arity, split-layout requirements, classifier routes,
and the real three-input failure, use `you docs factory-validation` rather
than duplicating those rules here.

Run the gate again after any topology, guard, route, schema, worker,
workstation, or split-layout prompt/configuration change, and do so before the
next run that relies on the change. A successful static check is not a promise
that providers, external resources, runtime paths, or execution will succeed.

### 4. Start the factory

Only after validation passes, for a portable JSON or YAML Factory and a single
customer prompt, mark one work type with `handlingBehavior: ["DEFAULT"]` (see
`you docs config`) and run:

```bash
you run --factory ./factory.json "Fix the lint issues"
you run --factory ./factory.yaml "Fix the lint issues"
```

The command preserves the explicitly selected file and resolves its asset
directory, submits the quoted prompt as raw text to the `DEFAULT` work type,
and exits after batch idle completion. `--factory` also accepts a directory
containing exactly one supported root. You cannot combine `--factory` with
`--dir` or `--work` on the same invocation.

Use mock workers for the first routing check with the directory layout:

```bash
you run --dir ./factory --with-mock-workers --work ./docs/examples/startup-work.json
```

Or combine `--factory` with mock workers when testing a portable config:

```bash
you run --factory ./factory.json --with-mock-workers ./docs/examples/mock-workers.json "Fix the lint issues"
```

The command loads the selected JSON or YAML definition, resolves the split
`AGENTS.md` files, and performs the one-shot invocation without starting a
listening server.

Live runs record a replay-compatible artifact by default. Use `--no-record`,
`--record <path>`, `--replay <path>`, or `--resume <recording>` when you need to
override capture, playback, or continuation. Resume writes a successor
recording by default. Use `--record <path>` to select its path. Run `you docs
record-replay` for generated paths, incompatible flag combinations, sensitivity
warnings, and copy-pasteable record, replay, and resume examples.

Run `you docs mock-workers` for the `--with-mock-workers` JSON contract,
selection fields, and deterministic outcome examples beyond this quick start.

## Run Named Factories From Anywhere

Persist reusable factories under a named-factory root when you want to run them
without locating `factory.json` manually:

```bash
you factory create my-team-review --from ./factory.json
```

After changing the source definition, replace the same persisted Factory
explicitly:

```bash
you factory update my-team-review --from ./factory.json
```

`create` refuses to overwrite an existing name; `update` requires that name to
already exist. Both commands validate the source before replacing durable
Factory files. Add `--set-current` only to `create` when the new project-local
Factory should also become the selected Current Factory.

By default persisted project factories live under `./factory`, and
`you run --named <name>` resolves that project-local root before checking the
global shared root at `~/.you-agent-factory/factories`.

```bash
you run --named my-team-review "Review the release notes"
```

This precedence is selection-only: the CLI chooses exactly one matching named
factory directory and never merges a project-local definition with a global
definition of the same canonical name.

The nineteen first-party packaged Factories also use the named-factory path.
`you factory list` is the discovery source for their descriptions and runnable
examples.

| Factory | Orchestrator | Use it for |
| --- | --- | --- |
| `@you/agy-clip-qa` | Graph | Gate a rendered clip against its shot specification with a structured pass-or-reroll result. |
| `@you/agy-cold-watch` | Graph | Review a completed cut from first principles, including visual chronology and audio. |
| `@you/classify` | Graph | Route a request to a small, medium, or large model lane by complexity. |
| `@you/deep-research` | JavaScript | Run bounded specialist investigations in parallel and synthesize their findings. |
| `@you/factory-builder` | Graph | Create and install one validated graph or JavaScript Factory from a request. |
| `@you/fix` | Graph | Plans and iterates a requested fix in an isolated named worktree, then repeats independent review until approval or bounded failure. |
| `@you/full-flow` | Graph | Plan implementation waves, work in isolated worktrees, merge, and replan until complete. |
| `@you/fusion` | Graph | Produce a draft with one worker and refine it with another. |
| `@you/goal` | Graph | Repeat bounded work on a goal until the executor reports completion. |
| `@you/loop` | Graph | Execute a request repeatedly at an invocation-supplied duration interval. |
| `@you/plan-execute` | Graph | Write matching Markdown and JSON PRDs, then execute and verify their stories in the current workspace. |
| `@you/plan-parallel` | Graph | Plan a Work dependency graph, execute ready tasks concurrently, and merge results. |
| `@you/quorum` | Graph | Run independent assessments concurrently and merge them. |
| `@you/ralph` | Graph | Plans a request, iterates through every incomplete plan story, and returns only after the durable plan is complete. |
| `@you/review` | Graph | Repeat writing and independent review until approval or exhaustion. |
| `@you/spawn` | JavaScript | Plan an exact number of tasks, execute them concurrently, and merge ordered results. |
| `@you/subagent` | Graph | Run one bounded read-only subagent and return its result. |
| `@you/tournament` | JavaScript | Compare candidates in judged 1v1 matches and return the champion. |
| `@you/tts` | Graph | Convert text to audio with the packaged local text-to-speech model. |

Representative invocations:

```bash
you run --named @you/plan-execute --to "Implement the requested feature"
you run --named @you/plan-parallel --to "Implement the requested feature"
you run --named @you/factory-builder --factory-name release-note-review --orchestrator graph --to "Review submitted release notes and return an approved summary."
you run --named @you/factory-builder --factory-name release-synthesis --orchestrator javascript --to "Run two independent analyses and return one synthesized result."
you run --named @you/loop --every 1h --to "Check dependency updates"
you run --named @you/tournament --rounds 3 --to "Propose a launch strategy"
you run --named @you/full-flow --to "Complete the requested project"
you run --named @you/spawn --count 10 --to "Research the best places to travel"
you run --named @you/goal "Ship the login fix by Friday"
you run --named @you/review "Draft the release notes"
you run --named @you/quorum "Compare the two proposed release plans."
you run --named @you/tts --output primary "Read the release summary."
```

Graph Factories use canonical Work, relationships, resources, guards, and
runtime scheduling. The three JavaScript Factories use the policy-bounded
workflow runtime where invocation-shaped fan-out is part of their design.
See `you run --named <factory> --help` for each Factory's arguments,
`you docs run` for invocation inputs and stdout result modes, `you docs
sessions` for stopped-run inspection and recovery, and `you docs models` for
TTS readiness, direct invocation, and audio or JSON result choices.

### Built-in `@you/factory-builder` validated creation

`@you/factory-builder` creates exactly one new named Factory from a request.
Set `--orchestrator graph` for a YAML graph Factory or `--orchestrator
javascript` for a JavaScript orchestrator. `--factory-name` optionally supplies
a new stable Factory name; when it is omitted, Builder derives one from the
request and fails safely if that name already exists. Optional
`--builder-provider` and `--builder-model` overrides follow the normal
operator-default precedence when they are omitted.

Before asking Builder to materialize a Factory, use the canonical public
guides for the requested form:

```bash
you docs agents
you docs authoring-factories
you docs config
you docs javascript-workflows
```

Builder stages its candidate beneath the current workspace, outside an
installed Factory root. It must first use the public validate-only command:

```bash
you factory config validate <staged-candidate>
```

Validation is required before persistence; success does not itself install the
candidate. Only after validation succeeds, Builder uses the ordinary named
Factory create command with the global operator-owned Factory root explicitly
selected. That prevents the command's project-local `./factory` default from
shadowing the shared named Factory:

```bash
you factory create <factory-name> --from <staged-candidate> --dir ~/.you-agent-factory/factories
```

That command owns the named-Factory destination. Builder does not copy staged
files into `./factory`, the operator-owned Factory root, a packaged Factory
directory, or another installed Factory. It also does not use `you factory
update`: a requested name that already exists remains unchanged. When
validation fails, Builder reports the safe diagnostic and a concrete correction
action, does not install the candidate, and does not start it.

The Builder result reports the canonical Factory name, requested orchestrator,
validation outcome, and installation outcome without exposing staging paths,
credentials, raw provider commands, or other unsafe host details. Inspect a
successful installation with `you factory list` or validate it again by name's
installed path before running it.

### Built-in `@you/review` approval gate

`@you/review` accepts one required request input:

```bash
you run --named @you/review "Draft the release notes"
```

It always runs `review-work-executor` and then the independent
`review-work-reviewer`. The reviewer returns either an accepted decision with
the candidate output or a rejected decision with actionable feedback. A
rejection returns the same request to the work stage with the prior candidate
and feedback; only a later approval becomes the invocation result. Work or
review failure ends at `reviewable-work:failed` and has no successful primary
result.

The materialized factory is editable at
`~/.you-agent-factory/factories/@you/review`. Its two workers accept
the standard agent-worker fields, including `modelProvider` (`CODEX` or
`CLAUDE`) and `model`, either in `factory.json` or their split `AGENTS.md`
front matter. Omit them to use normal operator defaults; `YOU_DEFAULT_WORKER_MODEL_PROVIDER`,
`YOU_DEFAULT_WORKER_MODEL`, and run-scoped `you run --provider` / `--model`
flags retain the documented `file < env < run flag`
precedence for omitted worker values. Configure both roles when they must use
the same provider/model, or configure them independently when the reviewer
needs a different model. Unsupported provider values are rejected by normal
factory validation:

```bash
you factory config validate ~/.you-agent-factory/factories/@you/review
```

You can customize the worker and workstation prompts, but preserve the review
workstation's `decision-envelope` format, `onRejection` route to
`reviewable-work:init`, and explicit `invocationReturn` for
`reviewable-work:approved`; those are the approval-only completion contract.
For a stopped run, inspect and recover it with `you docs sessions`; the package
uses the standard invocation recovery codes and adds no review-specific resume
command.

### Built-in `@you/goal` repeater

The shipped goal factory is deliberately minimal. It defines `goal:init`,
`goal:complete`, `goal:blocked`, and `goal:failed`, with one
`goal-executor` worker and one `execute-goal` `AGENT_RUN` workstation using
`REPEATER` behavior. The executor maintains an atomic JSON progress file under
`.you-goals/<session-id>/<work-id>.json` in the working directory and returns a
decision envelope. `needs_changes` routes back to `goal:init` for another pass,
`accepted` advances to `goal:complete`, and `blocked` leaves inspectable Work in
`goal:blocked`. Worker, malformed-envelope, or workstation failure routes to
`goal:failed`.

The factory's explicit `invocationReturn` selects `goal:complete`. The
executor's accepted envelope `output` is therefore returned as the invocation
`primaryResult` instead of echoing the submitted goal text. User or Factory
Session termination remains an external lifecycle control and is never inferred
from the executor's classifier decision.

The materialized built-in contains only these prompt-bearing entries:

```text
workers/goal-executor/AGENTS.md
workstations/execute-goal/AGENTS.md
```

Stopped runs use the shared invocation recovery codes and inspection flow,
including `INVOCATION_PAUSED`, `INVOCATION_INTERRUPTED`,
`INVOCATION_RUNTIME_FAILURE`, `INVOCATION_TIMED_OUT`, and
`INVOCATION_PRIMARY_RESULT_UNRESOLVED`. Customized materialized factories can
also surface `INVOCATION_BLOCKED` or `INVOCATION_NEEDS_HUMAN`. Run
`you docs sessions` for the inspect-first recovery steps; the built-in does not
add goal-specific inspect or resume commands.

Normal initializer startup materializes the built-in into
`~/.you-agent-factory/factories` before a named runtime opens. That keeps the
built-in editable: if you modify the installed
`workers/*/AGENTS.md`, `workstations/*/AGENTS.md`, or other split-layout files,
the next `you run --named @you/goal` invocation uses your edited version.

Use `you factory list` to inspect the effective catalog. It merges the
project-local `./factory` root, the global
`~/.you-agent-factory/factories` root, and packaged built-ins. Project-local
definitions override same-name global definitions, and global definitions
override same-name built-ins. Use `--dir` to substitute another project-local
root while retaining the global and packaged tiers:

```bash
you factory list
you factory list --dir ./alternate-factories
```

Packaged-only Factories remain visible without being installed and show `-`
for their Factory directory.

The retired `~/.you-agent-factory/you-agent-factories` root is not read or migrated during startup.
Move required Factory directories into
`~/.you-agent-factory/factories` before starting this version.

### 5. Submit work

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

Use `INFERENCE_RUN` when the workstation should request a generic operation
such as `TTS` and let worker capability plus typed resources decide whether the
execution is local or cloud-backed. Legacy `MODEL_INVOKE` remains accepted
during the migration window.

### Shared workstation contract

This workstation stays the same for both local and cloud TTS:

```json
{
  "name": "speak",
  "type": "INFERENCE_RUN",
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
  "name": "local-tts",
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
      "type": "INFERENCE_RUN",
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
type: INFERENCE_WORKER
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
type: INFERENCE_WORKER
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
references. Set `INFINITE_YOU_OMNIVOICE_CACHE_DIR` on the `you` process to
select a reusable managed-model cache root; when it is unset, the cache lives
under the current user's default model-cache directory.

### Maintainer Validation

For managed local-model runtime coverage, run
`make long-tests-managed-runtime`. This focused specialty lane exercises the
managed runtime adapter with deterministic local test doubles.

## Related Contract Detail

- `you docs config` owns work types, states,
  routing, resources, and portability fields.
- `you docs workstations` owns workstation kinds, runtime fields,
  route fields, and guards.
- `you docs workers` owns worker types, backend fields, and worker
  `AGENTS.md` placement.
- `docs/reference/authoring-agents-md.md` owns split file shape, prompt
  placement, and authoring patterns.

## Failure Routing And Provider Behavior

For workflow design, add explicit failure, continue, and rejection destinations
to the topology so every outcome lands somewhere intentional. Use
`you docs config` for the
canonical routing contract, `you docs workstations` for route fields
and execution limits, and `you docs workers` for worker backend behavior.

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
- Use a `POLLER_WORKER` poller when the repository already ships the provider
  integration, such as the built-in `LINEAR` poller. Legacy `HOSTED_WORKER`
  remains accepted during the migration window.

Keep the exact contracts on the canonical owner pages:

- `you docs workstations` owns `behavior: "POLLER"` and lifecycle
  behavior.
- `you docs workers` owns hosted `LINEAR` worker fields and `auth.secretRef`.
- `you docs batch-inputs` owns the script
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
      "onFailure": [{ "workType": "task", "state": "failed" }]
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

Before starting service mode, resolve the Linear API key for
`auth.secretRef: secrets/linear-api-key`. Either set
`INFINITE_YOU_SECRET_SECRETS_LINEAR_API_KEY` or create
`secrets/linear-api-key` beside `factory.json`. See `you docs workers` for the
full hosted-worker secret contract.

`workers/linear-poller/AGENTS.md`:

```yaml
---
type: POLLER_WORKER
provider: LINEAR
auth:
  secretRef: secrets/linear-api-key
linear:
  pollInterval: 2m
  teamIds: ["team-a"]
  stateIds: ["state-b"]
  mapping:
    workType: task
    state: init
  claim:
    assigneeField: assignee.email
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
  "onFailure": [{ "workType": "task", "state": "failed" }]
}
```

Keep hosted Linear config on the worker. The workstation only selects
`behavior: "POLLER"`, names the worker, and routes submitted work.

V1 non-goals for poller authoring:

- Raw factory event emission from pollers.
- OAuth-based hosted auth flows.
- Advanced multi-instance poller coordination.

## Test Workflows With Mock Workers

Use mock workers when you want to verify routing, rejection loops, failure
paths, and script side effects without making live provider calls.

Run `you docs mock-workers` for the full JSON contract, selection fields,
`runType` values, `unmatchedDispatchPolicy`, and script or mixed-mode examples.
For this review-loop walkthrough, start with:

```bash
you run --dir ./factory --with-mock-workers --work ./docs/examples/startup-work.json
```

To exercise the checked-in rejection example:

```bash
you run --dir ./factory --with-mock-workers ./docs/examples/mock-workers.json --work ./docs/examples/startup-work.json
```

Reusable inputs live under [`docs/examples/`](../examples/README.md), including
[`docs/examples/mock-workers.json`](../examples/mock-workers.json),
[`docs/examples/mock-workers-script.json`](../examples/mock-workers-script.json),
[`docs/examples/mock-workers-mixed.json`](../examples/mock-workers-mixed.json),
and [`docs/examples/startup-work.json`](../examples/startup-work.json). The
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
- Run `you factory config validate` on the authored file or directory after
  authoring and until it passes immediately before the first run.
- Repeat validation after topology, guard, route, schema, worker, workstation,
  or split-layout prompt/configuration changes and before the next dependent
  run.
- Use `you docs factory-validation` for the complete static-check list and
  unsupported-join correction example.
- Use camelCase factory-config fields such as `workTypes`, `resources`,
  `onFailure`, `onRejection`, and `maxVisits`.
- Use `supportingFiles` only for portability-only concerns such as
  validation-only PATH tools and explicitly bundled scripts or docs.
- Keep prompt-heavy worker and workstation runtime fields in split `AGENTS.md`
  files unless you intentionally need a single-file config.
- Add a guarded `LOGICAL_MOVE` workstation for repeater or review loops.
- Use `you docs batch-inputs` for `FACTORY_REQUEST_BATCH`
  request files.
- Use `you docs workstations` for cron, prompt templates, timeouts, and
  workstation runtime field details.
- Use `you docs workers` for worker backend field details.

## Related

- `you docs agents`
- `you docs mock-workers`
- `you docs record-replay`
- `you docs config`
- `you docs work`
- `you docs workstations`
- `you docs workers`
- `you docs batch-inputs`
- `you docs relationships`
- `you docs guards`
- `you docs templates`
- `docs/reference/README.md`
