# Packaged Factory Families Implementation Plan

## Status

Implementation and feature-owned verification are complete. Repository-wide
lint baseline debt and the unavailable optional browser backend are recorded in
the completion audit rather than misrepresented as feature failures.

The 2026-07-30 implementation correction supersedes older plan-execute and
source-layout language later in this historical plan: graph families are
authored as `factory.yaml`; JavaScript families are self-contained `factory.js`
files with inert `@you-factory-meta` JSON comment headers for catalog metadata; and `@you/plan-execute` contains only
the planner and executor stages. It does not create a worktree or dispatch a
script, reviewer, CI, or merge stage. Every agent instruction is written for a
fresh worker with no assumed conversational or repository context.

## Completion Checklist

Every box remains unchecked until the implementation and the cited evidence
exist in the current tree. The plan is complete only when every box is checked.
TTS remains in automated regression coverage but is excluded from the manual
Cursor ACP and Codex multi-agent runs.

### Shared contracts and discovery

- [x] All fourteen authored packaged Factories have customer-facing
  descriptions, invocation help, and representative examples.
- [x] Generated JSON/YAML, manifest metadata and hashes, schemas, embedded
  assets, npm exports, CLI/API discovery, and dashboard discovery agree.
- [x] Text-oriented families accept positional input, stdin, and `--to` without
  shadowing static `you run` flags.
- [x] Provider/model overrides reach the intended role and omitted overrides
  retain operator-default resolution.
- [x] Generated Work, loop/review attempts, JavaScript child counts, schedule
  frequency, and concurrency are bounded with observable failure behavior.

### Packaged family behavior

- [x] `@you/deep-research` remains JavaScript and passes required/optional,
  parallel specialist, synthesis, policy, cancellation, and failure tests.
- [x] `@you/fusion` passes sequential draft/refine and primary-result tests.
- [x] `@you/goal` passes bounded repeat, completion, and exhaustion tests.
- [x] `@you/quorum` passes independent parallel branch and merge tests.
- [x] `@you/review` passes writer/reviewer rejection-loop, approval, and
  exhaustion tests.
- [x] `@you/subagent` passes bounded read-only invocation and result tests.
- [x] `@you/tts` passes its existing automated regression coverage; no manual
  ACP/Codex execution is required.
- [x] `@you/plan-execute` passes Markdown/JSON PRD creation, direct executor
  handoff, story execution, and exact two-dispatch tests without a script or
  reviewer stage.
- [x] `@you/plan-parallel` passes generated DAG, dependency readiness,
  concurrent execution, failure fan-in, replay, and merge-result tests.
- [x] `@you/classify` passes small/medium/large routing, invalid-label, and
  role-selection tests.
- [x] `@you/loop` accepts duration text such as `1h`, schedules repeated
  executions, prevents overlap, records failures/skips, and stops durably.
- [x] `@you/tournament` remains JavaScript, runs 1v1 candidate matches with a
  judge after each match, advances deterministic winners, and returns the
  champion.
- [x] `@you/full-flow` passes isolated parallel worktree, merge, dependency
  loopback, replanning, completion, failure-bound, and replay tests.
- [x] `@you/spawn` remains JavaScript, plans exactly `count` tasks, runs them in
  bounded parallelism, and merges all ordered results.

### Verification evidence

- [x] Focused unit, contract, projection/replay, CLI, API, UI, JavaScript
  runtime, packaged-catalog, and functional suites pass for changed owners.
- [x] Root-built functional tests use ACP command-runner mocks at the stdio
  layer and exercise Codex for families where multiple agents are appropriate.
- [x] Manual harness runs prove the new JavaScript and graph families work
  end-to-end with the installed Cursor ACP provider.
- [x] Manual harness runs prove applicable new and modified multi-agent
  families work end-to-end with Codex.
- [x] Existing modified families pass manual Cursor ACP/Codex regression runs,
  excluding TTS.
- [x] `make packaged-factory-catalog-check`, package verification,
  documentation smoke, `make verify-fast`, UI lint, and affected-owner Go vet
  pass; the independent repository-wide lint baseline is audited below.
- [x] The final completion audit records the commands and artifacts proving
  every checked item; no box is checked from intent or indirect evidence.

### Completion audit

Evidence recorded on 2026-07-29:

- `make verify-fast` passed, including TypeScript typecheck, MCP contract
  alignment, 390 UI test files with 3,161 passing tests, and the short Go unit
  lane.
- `go test ./tests/functional/factory/packaged/... -count=1` passed all 15
  packaged functional packages covering the fourteen families and their cross-
  surface/catalog contracts.
- `go test ./tests/functional/providers/acp -run TestPackaged -count=1` passed
  the root-built ACP stdio mocks for tournament and spawn; the packaged
  JavaScript runtime tests additionally prove exact call budgets, 1v1 judging,
  deterministic ordering, invalid-output rejection, and reducer failures.
- `make packaged-factory-catalog-check`,
  `make packaged-factory-package-verify`, and `make docs-reference-smoke`
  passed. Package verification reported 38 passes and one expected Windows
  symlink skip.
- `make ui-lint` passed, and `go vet` passed across every affected backend,
  runtime, transport, wiring, packaged-functional, ACP, and runtime-API owner.
- Focused loop scheduler tests prove fake-clock repetition, overlap skips,
  failure exhaustion, resumed sequence continuity, durable controller recovery,
  and no duplicate trigger-at-start. Plan-parallel and full-flow functional
  tests assert retained replay and generated Work relationships directly.
- Focused plan-execute tests create matching Markdown/JSON PRDs, dispatch only
  planner then executor, execute the plan in the current workspace, update
  story evidence, and prove both workers inherit operator provider/model
  defaults when role-specific configuration is omitted.
- Installed-provider manual runs used `cursor-agent 2026.07.23-e383d2b` and
  `codex-cli 0.144.1`. Cursor ACP completed deep-research, fusion, goal,
  quorum, review, subagent, classify, plan-parallel, spawn, tournament, and
  plan-execute. The live one-round tournament launched two independent Cursor
  competitors and a Cursor judge, then returned entrant 2 as the champion with
  the judge rationale in the structured primary result. The live plan-execute
  run used a disposable Git repository, wrote matching Markdown/JSON PRDs,
  created an isolated worktree, committed `result.txt` on
  `packaged-plan-audit`, reviewed it, and fast-forwarded disposable `main` to
  commit `b7148eb` with the exact requested content. Codex completed
  deep-research, fusion, goal, quorum, review, subagent, and spawn. TTS was
  excluded as planned. The Codex goal run exposed an omitted decision-marker
  failure; adding the customer payload and a mandatory final marker to the goal
  prompt made the rerun complete in one pass.
- `git diff --check` passed, and no manual `you` or `codex exec` process remained
  after the harness runs. Repository-local durable-session and PowerShell cache
  artifacts created during manual verification were removed.
- The real-backend browser integration was upgraded to proxy the actual
  `/packaged-factories` response and assert the deep-research description, and
  its module passes `node --check`. It was not executed because browser runtime
  discovery returned no available browser backends in this session.
- Repository-wide `make lint` remains red before reaching feature-owned size
  checks: repository `HEAD` contains six unreachable CLI test blocks and a
  stale provider inference fixture. A diagnostic continuation also found a
  broadly stale backend exemption budget (23 moved/unregistered targets).
  Those unrelated migration baselines were not hidden with new exemptions or
  folded into this feature; affected-owner UI lint and Go vet both pass.

## Customer Ask

Ship the packaged Factory families described by `plan-readme.md` as packaged
Factory definitions with discoverable descriptions, coherent invocation
arguments, and direct behavioral test coverage. Use the graph orchestrator for
stateful Work scheduling and the existing sandboxed JavaScript orchestrator for
families whose topology is intentionally dynamic. In particular:

- `@you/plan-execute` must use a two-stage PRD contract: one planner writes the
  Markdown and JSON PRDs and one fresh executor reads and implements them in the
  current workspace. It must not add script, review, CI, merge, or worktree
  stages.
- `@you/plan-parallel` must let a planner emit canonical Work and Work
  relationships, then let the Factory runtime schedule dependency-ready Work
  concurrently.
- `@you/loop` must execute a request at an invocation-supplied duration such as
  `1h`.
- `@you/tournament` and `@you/spawn` must use the existing JavaScript
  orchestrator for bounded dynamic fan-out and reduction.
- `@you/full-flow` must execute bounded parallel implementation waves in
  isolated worktrees, merge completed tasks, and loop back to a planner that
  decides whether the project is complete.
- Every packaged Factory must publish a useful customer-facing description so
  CLI, API, and dashboard discovery explain what the Factory does.

## Problem

The current packaged catalog contains seven Factories:

- `@you/deep-research`
- `@you/fusion`
- `@you/goal`
- `@you/quorum`
- `@you/review`
- `@you/subagent`
- `@you/tts`

The README proposal also names `@you/plan-execute`, `@you/plan-parallel`, and
`@you/classify`. This plan additionally defines `@you/loop`,
`@you/tournament`, `@you/full-flow`, and `@you/spawn`. Those seven names are not
in the packaged catalog. Existing
catalog entries have uneven invocation signatures and several omit
descriptions, so users cannot reliably discover their purpose or role-specific
arguments. `@you/deep-research` already exercises the JavaScript workflow
surface and must remain JavaScript rather than being converted to a graph.

The public CLI also has one important contract boundary: `you run -a` already
means `you run --named`. Factory-specific inputs and role overrides should come
from the selected Factory's `invocationSignature`; they should not become new
global `you run` flags or separate top-level commands.

## Goals

- Publish all named packaged families using the appropriate graph or
  JavaScript orchestrator.
- Make `@you/plan-execute` expose exactly the PRD planner and direct executor
  lifecycle, with the two PRD files as their durable handoff.
- Make `@you/plan-parallel` use canonical generated Work batches,
  `DEPENDS_ON`, parent-child lineage, runtime concurrency, and guarded fan-in.
- Make `@you/classify` route through `CLASSIFIER_WORKSTATION`.
- Add a durable duration-driven `@you/loop`, JavaScript `@you/tournament` and
  `@you/spawn` families, and a graph-based `@you/full-flow` implementation
  loop.
- Preserve the existing JavaScript implementation of `@you/deep-research`.
- Give every packaged Factory a concise localizable description and at least
  one runnable example where the Factory accepts invocation input.
- Give text-oriented Factories consistent positional, stdin, and `--to`
  invocation bindings.
- Support role-specific provider and model overrides without bypassing
  operator defaults when overrides are omitted.
- Bound generated Work, concurrency, retries, and review loops with explicit
  failure behavior.
- Keep authored sources, generated catalog artifacts, CLI help, API catalog,
  dashboard discovery, and packaged npm data aligned.

## Non-Goals

- Do not add `you plan-execute`, `you plan-parallel`, or other new top-level
  CLI commands.
- Do not add a second scheduler for `@you/plan-parallel`; the existing Factory
  runtime remains authoritative for Work readiness and concurrency.
- Do not execute model-generated Factory configuration or arbitrary code.
  Planners emit canonical `FACTORY_REQUEST_BATCH` data only.
- Do not special-case packaged family names inside the runtime when a reusable
  Factory or Work contract can express the behavior.
- Do not count aliases as additional packaged families. After adding the seven
  named missing families, the catalog contains fourteen Factories;
  documentation must state the exact generated catalog count.
- Do not convert `@you/deep-research`, `@you/tournament`, or `@you/spawn` into
  graph orchestration. They intentionally use the existing validated,
  policy-bounded JavaScript runtime.
- Do not redesign unrelated Factory Session, Provider Session, or dashboard
  state architecture.

## Product Decisions

### Packaged Factories use the orchestrator that matches their topology

Graph families use the canonical Factory definition model: `workTypes`,
`workers`, `workstations`, resources, guards, Work relationships, and explicit
invocation return policy. Packaged prompts and scripts remain package-owned
assets flattened by the existing catalog generator.

`@you/deep-research` keeps its existing JavaScript workflow. The new
`@you/tournament` and `@you/spawn` families also use JavaScript because their
fan-out arrays and reduction steps are invocation-shaped. Their scripts may
use only the validated `args`, `phase`, `agent.run`, `parallel`, `pipeline`, and
`workflow` surfaces and remain constrained by declared schemas and effective
policy. JavaScript children produce durable child-execution records and
artifacts; they do not bypass Factory Session lifecycle or policy enforcement.

### Invocation syntax

The canonical examples use the existing named-Factory command:

```sh
you run -a @you/plan-execute --to "Implement the requested feature"
you run -a @you/plan-parallel --to "Implement the requested feature"
you run -a @you/loop --every 1h --to "Check dependency updates"
you run -a @you/tournament --rounds 3 --to "Propose a launch strategy"
you run -a @you/full-flow --to "Complete the requested project"
you run -a @you/spawn --count 10 --to "research the best places to travel"
```

For text-oriented Factories, one logical input parameter supports:

- positional input;
- stdin input; and
- a Factory-defined named binding exposed as `--to`.

Role-specific options such as `--planner-provider` and `--executor-model` are
also Factory-defined invocation parameters. Static run flags such as `--output`
remain owned by `you run`; Factory parameters must not reuse those spellings.

### Descriptions are a required discovery behavior

Every authored packaged Factory must set `description` using the existing
localizable name/value contract. Descriptions should say what the Factory does,
not how its files are arranged.

Proposed canonical English descriptions:

| Factory | Description |
| --- | --- |
| `@you/deep-research` | Breaks a research question into bounded specialist investigations and synthesizes their findings. |
| `@you/fusion` | Produces an initial answer with one worker and refines it with a second worker. |
| `@you/goal` | Repeatedly works a goal until the executor reports completion or the Factory reaches a failure bound. |
| `@you/plan-execute` | Writes matching Markdown and JSON PRDs, then gives them to one fresh executor that implements the complete plan in the current workspace. |
| `@you/plan-parallel` | Plans a dependency graph of Work, executes ready tasks concurrently, and merges the completed results. |
| `@you/quorum` | Runs independent assessments in parallel and merges them into one final answer. |
| `@you/review` | Produces candidate work and repeats independent review until approval or a bounded failure. |
| `@you/classify` | Classifies a request by complexity and routes it to the configured small, medium, or large model lane. |
| `@you/subagent` | Runs one bounded read-only subagent and returns its result. |
| `@you/tts` | Converts submitted text to audio with the packaged local text-to-speech model. |
| `@you/loop` | Runs the requested task at a duration interval such as `1h` for the lifetime of the Factory Session. |
| `@you/tournament` | Runs candidates through bounded 1v1 matches, uses a judge to advance each winner, and returns the champion result. |
| `@you/full-flow` | Plans parallel implementation waves in isolated worktrees, merges completed tasks, and replans until the project is complete. |
| `@you/spawn` | Plans an exact number of independent tasks, runs them concurrently, and merges their results into one answer. |

Descriptions flow through the generated manifest and existing packaged catalog
API. CLI `factory list`, selected-Factory help, and dashboard inventory should
display the same description rather than maintaining parallel copy.

## Shared Technical Design

### Worker-generated Work graph contract

The runtime already recognizes an accepted worker output shaped as:

```json
{
  "request": {
    "type": "FACTORY_REQUEST_BATCH",
    "works": [
      {
        "name": "inspect-api",
        "workTypeName": "planned-task",
        "payload": "Inspect the API behavior"
      },
      {
        "name": "implement",
        "workTypeName": "planned-task",
        "payload": "Implement the requested behavior"
      }
    ],
    "relations": [
      {
        "type": "DEPENDS_ON",
        "sourceWorkName": "implement",
        "targetWorkName": "inspect-api",
        "requiredState": "complete"
      }
    ]
  }
}
```

The existing Work admission path validates the complete batch, rejects invalid
references and dependency cycles atomically, records canonical Work and
relationship events, and attaches generated children to their source Work.
The scheduler then prevents blocked Work from dispatching while allowing
independent ready Work to dispatch concurrently.

Before packaged planners depend on this behavior, add a generic declarative
ceiling such as `workstations[].limits.maxGeneratedWorkItems`. Enforcement must
happen before any generated child Work is admitted. This is a reusable
workstation behavior, not a packaged-name policy.

### Generated Work fan-in

Planner/source Work moves to a waiting processing state while generated child
Work runs. Completion uses the existing per-input guards:

- `ALL_CHILDREN_COMPLETE` enables the merge workstation only after the full
  generated child set completes.
- `ANY_CHILD_FAILED` routes the waiting parent to failure when a child fails.
- `spawnedBy` identifies the planner workstation that generated the child set.

The final invocation result explicitly targets the parent or result Work's
terminal state. It must not rely on the default submitted-Work result policy
when the graph returns separately generated Work.

### Provider and model resolution

Worker fields use invocation placeholders for role-specific overrides. An
omitted optional override must continue through the existing operator-default
resolution path. Tests must cover both omitted values and explicit values; the
implementation must not silently replace operator defaults with hard-coded
package defaults.

### Invocation-shaped JavaScript fan-out

`@you/deep-research`, `@you/tournament`, and `@you/spawn` keep dynamic child
orchestration inside the existing JavaScript runtime. Every numeric fan-out
argument is integer-validated by `argsSchema`, has a conservative package
ceiling, and is checked against effective `maxAgents` and concurrency policy
before the first child launches. A rejected request launches zero children.

The JavaScript runtime remains responsible for child execution records,
phase/log events, cancellation, deterministic output ordering, failure
propagation, and final result serialization. Package scripts cannot load
modules or access the host filesystem, process, or network directly. Tests use
an injected child executor and assert the exact child count, maximum observed
concurrency, reduction order, terminal result, and behavior when any child or
reducer fails.

### Invocation-scoped duration schedules

`@you/loop` resolves the invocation's `every` argument as a positive duration
instead of asking customers to write a cron expression. Initial syntax follows
`time.ParseDuration`, including values such as `30s`, `5m`, `1h`, and `1h30m`,
with documented minimum and maximum intervals. Duration validation happens
before the Factory Session registers its interval job. The normalized duration
is recorded in canonical session/event state so live projection and replay
explain when and why each trigger occurred.

Scheduled runs remain active until explicitly stopped or cancelled. The
initial version defaults to `SKIP_WHILE_RUNNING`: when a trigger fires while
the previous execution is active, it records a skipped trigger and does not
overlap executor dispatches. Tests use an injected clock/scheduler and never
sleep or wait for wall-clock time.

## Family Designs

### `@you/plan-execute`: PRD-driven execution

This family is deliberately a two-stage graph rather than a complete delivery
pipeline. The PRD files are the explicit handoff between agents that share no
conversation.

#### Observable workflow

1. The invocation creates one named `planned-request` Work item.
2. The planner begins with zero context, reads repository instructions,
   architecture, relevant code and tests, working-tree state, and the customer
   request.
3. The planner writes `tasks/todo/<work-name>.md` and
   `tasks/todo/<work-name>.json` in the target repository.
4. The Markdown PRD describes context, goals, non-goals, technical design,
   vertically sliced user stories, behavioral acceptance criteria, test
   evidence, and the delivery loop.
5. The JSON PRD is a mechanistic implementation manifest containing project,
   description, context, project acceptance criteria, ordered standalone
   stories, explicit tests, `passes: false`, and empty notes.
6. A different executor begins with zero context, reads repository instructions
   and both PRDs from `tasks/todo/`, validates that they agree, and implements
   the complete plan directly in the current workspace.
7. The executor runs story-proportional positive and failure-path verification,
   marks a story passed only with evidence in `notes`, and runs the relevant
   broad checks after every story passes.
8. The Factory reaches successful terminal Work after that executor completes.
   Review, CI, merge, script execution, and worktree lifecycle are outside this
   packaged Factory.

#### Factory topology

```text
planned-request:init
  -> plan-request
  -> planned-request:planned
  -> execute-plan
  -> planned-request:complete
```

The package owns verbose planner and executor prompts. There is no setup script,
reviewer prompt, isolated working directory, join, or retry loop.

Invocation parameters:

- `to`
- `planner-provider`, `planner-model`
- `executor-provider`, `executor-model`

`@you/fusion` remains the lighter draft/refine family and is not an alias for
this lifecycle.

### `@you/plan-parallel`: generated Work dependency graph

Work types:

- `parallel-plan`: `init`, `waiting`, `complete`, `failed`
- `planned-task`: `init`, `complete`, `failed`

Workstations:

1. `plan-parallel-work` consumes the submitted request, emits a bounded
   `FACTORY_REQUEST_BATCH` of `planned-task` Work plus `DEPENDS_ON` relations,
   and moves the original Work to `waiting` while preserving its input.
2. `execute-planned-task` consumes every ready `planned-task:init`. A shared
   executor resource bounds simultaneous dispatches; dependencies determine
   readiness.
3. `merge-plan-results` consumes the waiting parent and guarded completed child
   set, then produces the final parent result.
4. `fail-plan-from-child` consumes the waiting parent and a guarded failed
   child, then routes the parent to `failed`.

The planner emits data, not Factory configuration. The model may choose any
acyclic dependency shape within configured Work-count and execution bounds.

Invocation parameters:

- `to`
- `planner-provider`, `planner-model`
- `executor-provider`, `executor-model`
- `merge-provider`, `merge-model`, defaulting through the executor/operator
  resolution path when omitted

### `@you/classify`: model-lane routing

Use one `CLASSIFIER_WORKSTATION` with labels `small`, `medium`, and `large`.
Each classification route preserves the original request and enables exactly
one role-specific execution workstation. All successful lanes join the same
terminal Work state. An empty, JSON-encoded, or unknown classifier label routes
to failure without dispatching an execution worker.

Invocation parameters:

- `to`
- `classifier-provider`, `classifier-model`
- `small-provider`, `small-model`
- `medium-provider`, `medium-model`
- `large-provider`, `large-model`

### `@you/loop`: duration-driven execution

This graph turns one invocation into a long-lived scheduled Factory Session.
The submitted request remains controller Work while each interval tick creates a
distinct execution Work item with its scheduled time, actual trigger time, and
sequence number in canonical metadata.

```text
loop-controller:active
  <- invocation-scoped duration trigger
  -> scheduled-execution:init
  -> execute-request
  -> scheduled-execution:complete|failed
  -> wait for next trigger

session stop/cancel
  -> unregister schedule
  -> loop-controller:stopped
```

The same request payload is supplied on every execution. Individual failures
remain visible and do not silently disappear; default policy records the failed
execution and continues future ticks, while a configurable consecutive-failure
ceiling terminates the controller. Overlapping ticks use
`SKIP_WHILE_RUNNING` in the initial package. The Factory Session does not claim
a normal one-shot final result after the first tick.

Invocation parameters:

- `to`
- required `every`, using positive duration text such as `1h`
- optional `trigger-at-start`, default `false`
- optional `max-consecutive-failures`
- `executor-provider`, `executor-model`

### `@you/tournament`: JavaScript candidate tournament

This JavaScript workflow treats `rounds` as the depth of a single-elimination
bracket. It creates `2^rounds` initial candidate agents. Every match is 1v1:
one judge receives both candidates and the original request, selects exactly
one winner using stated criteria, and advances that winner unchanged with the
judge rationale attached. The final surviving candidate is the champion and
the invocation result.

```text
generate 2^rounds candidates in parallel
  -> pair candidates two at a time
  -> judge each 1v1 match in parallel
  -> advance winners
  -> repeat for rounds
  -> champion result
```

`rounds` is integer-bounded from 1 through 3 initially, producing at most 8
candidate calls and 7 judge calls. Effective policy must authorize the
derived total before execution begins. Candidate and match order remain stable
regardless of child completion order. A child or refiner failure fails the
workflow with round/match provenance; it does not advance a missing candidate
or allow the judge to introduce a third replacement answer.

Invocation parameters:

- `to`
- required `rounds`
- `competitor-provider`, `competitor-model`
- `judge-provider`, `judge-model`

### `@you/full-flow`: iterative parallel worktree delivery

This graph packages the meta-planner loop already demonstrated by
`factory/factory.json`, but makes every planned implementation task an isolated
worktree delivery unit. It differs from `@you/plan-parallel`: plan-parallel
executes one generated dependency DAG and synthesizes results, while full-flow
can generate several independent implementation waves and re-inspect the
repository after every merged wave.

```text
project-loop:init
  -> assess-project
     -> COMPLETE decision -> project:complete
     -> FACTORY_REQUEST_BATCH:
          task A -> setup worktree -> implement -> review -> CI -> merge
          task B -> setup worktree -> implement -> review -> CI -> merge
          task N -> setup worktree -> implement -> review -> CI -> merge
          loopback DEPENDS_ON every task reaching merged
  -> project-loop:init
```

The planner emits a bounded batch of task Work plus one loopback Work item. The
loopback has `DEPENDS_ON` relations to every task in that wave and cannot run
until each task is terminally merged. Each task receives a unique safe branch
and `.claude/worktrees/<task-name>` working directory, implements only its
assigned scope, runs proportional tests, handles review/CI feedback, rebases or
resolves conflicts against the evolving base, and performs the required merge.
The next planner pass reads current repository and Factory state rather than
assuming the previous plan is still valid.

The completion decision is a validated structured envelope, not an unparsed
prose claim. Explicit `max-cycles`, `max-tasks-per-cycle`, executor/reviewer
visit limits, and child-failure routing prevent an autonomous infinite loop.
Success requires the completion decision, no outstanding generated task or
loopback Work, and evidence that every task accepted in prior waves merged.

Invocation parameters:

- `to`
- optional `base-branch`
- optional `max-cycles` and `max-tasks-per-cycle`
- `planner-provider`, `planner-model`
- `executor-provider`, `executor-model`
- `reviewer-provider`, `reviewer-model`

### `@you/spawn`: JavaScript planned fan-out and merge

This JavaScript workflow first asks a top-level planner to produce exactly
`count` non-empty, distinct task descriptions for the request. It then launches
one child agent per task through `parallel(...)`, preserving task index and
label, and gives the ordered child results to a final top-level merger.

```text
request + count
  -> planner returns exactly count tasks
  -> count task agents run in parallel
  -> top-level merger receives every ordered result
  -> merged result
```

The initial `count` bound is 1 through 14 and must also fit effective
`maxAgents` and concurrency policy, including the planner and merger calls.
Malformed plans, the wrong task count, duplicate/empty tasks, policy overflow,
or any child failure fail before merge; no partial answer is presented as a
complete result.

Canonical example:

```sh
you run -a @you/spawn --count 10 --to "research the best places to travel"
```

Invocation parameters:

- `to`
- required `count`
- `planner-provider`, `planner-model`
- `worker-provider`, `worker-model`
- `merge-provider`, `merge-model`

### Existing family alignment

- `@you/review`: add `--to`, writer/reviewer provider and model arguments, and
  a guarded review-loop terminal failure.
- `@you/goal`: add a complete invocation signature, `--to`, executor provider
  and model arguments, description, examples, and an explicit bounded failure
  story without changing its goal-oriented repeater behavior.
- `@you/quorum`: retain fixed parallel branches and merge; add `--to` while
  preserving existing branch/merge overrides.
- `@you/fusion`: retain sequential draft/refine behavior; rename the
  Factory-defined CLI output-path spelling to a non-conflicting name such as
  `--result-file` while keeping API argument identity deliberate and tested.
- `@you/subagent`: add description, `--to`, examples, and optional worker
  provider/model selection consistent with its read-only policy.
- `@you/tts`: add description and a required text invocation signature with
  positional, stdin, and `--to` bindings; retain its local-model readiness and
  failure projections.
- `@you/deep-research`: keep the existing JavaScript orchestrator, add the
  common `--to` binding and description, and retain its policy-bounded parallel
  specialists followed by lead synthesis.

## Implementation Stories

### PF-FAMILIES-001: Require useful packaged Factory descriptions

Add localizable descriptions and representative examples to all authored
packaged Factories, then project them through the generated manifest and
existing discovery surfaces.

Acceptance criteria:

- CLI `factory list` shows a non-empty purpose description for every packaged
  Factory.
- `GET /packaged-factories` returns the same descriptions for all entries.
- The dashboard inventory and detail surfaces render the API-provided
  description without hard-coded per-family copy.
- Missing description on an authored packaged Factory is rejected by the
  packaged catalog validation path, or an equivalent catalog-level invariant
  prevents publishing an undescribed entry.
- CLI, API, and UI behavioral tests cover at least one entry and a catalog-wide
  completeness assertion at the public catalog boundary.
- Typecheck and tests pass.

### PF-FAMILIES-002: Normalize packaged invocation arguments and help

Define the common text input bindings, role override conventions, examples,
and reserved run-flag collision behavior.

Acceptance criteria:

- Each text-oriented family accepts positional text, piped stdin, and `--to`.
- `you run -a <factory> --help` shows Factory-specific arguments,
  descriptions, defaults, aliases, and examples.
- Omitted role overrides use operator defaults; explicit provider/model values
  reach the intended worker only.
- A Factory parameter cannot silently shadow a static `you run` flag;
  `@you/fusion` has a working non-conflicting result-file spelling.
- Focused parser/composition tests cover required input, conflicting input
  sources, unknown named arguments, aliases, and repeated process execution.
- Typecheck and tests pass.

### PF-FAMILIES-003: Bound worker-generated Work batches

Add a reusable generated-Work ceiling to workstation limits and enforce it in
the canonical worker-output-to-Work admission path.

Acceptance criteria:

- A valid worker-emitted `FACTORY_REQUEST_BATCH` creates canonical Work and
  relationship events through the existing Work service.
- Invalid references, cycles, unsupported Work types, empty batches, and
  oversized batches fail atomically before any child Work dispatch.
- Generated Work retains request, trace, parent-child, and source-dispatch
  lineage through live projections and replay.
- Cancellation before admission creates no partial batch.
- Authored Factory schema, OpenAPI, Go mappings, TypeScript types, generated
  artifacts, and contract tests remain aligned if the public limits contract
  changes.
- Unit, projection/replay, API contract, and focused functional tests pass.

### PF-FAMILIES-004: Package the standalone PRD planner

Implement the first stage of `@you/plan-execute`: evidence-based PRD authoring
for a future executor with zero shared context.

Acceptance criteria:

- The planner writes both `tasks/todo/<name>.md` and
  `tasks/todo/<name>.json` for the submitted request.
- The Markdown and JSON files contain the same project intent, ordered stories,
  behavioral acceptance criteria, test requirements, and delivery-loop
  completion rule.
- JSON story IDs are stable and sequential, priorities are ordered,
  `passes` starts false, and notes start empty.
- Every story is standalone and includes context, boundaries, behavioral
  acceptance criteria, failure cases, and explicit tests.
- Missing, malformed, or inconsistent PRD files prevent successful planner
  completion with a useful diagnostic.
- A root-built functional test with an injected provider command runner proves
  the planner creates both files without touching Git branches or worktrees.
- Typecheck and tests pass.

### PF-FAMILIES-005: Execute the complete PRD directly

Implement the second and final stage of `@you/plan-execute` as one executor that
works in the invocation's current workspace.

Acceptance criteria:

- The executor reads the matching Markdown and JSON PRDs from `tasks/todo/`,
  re-reads repository instructions and current state, and validates the handoff
  before editing.
- It implements every story in priority order, preserves unrelated work, and
  records test evidence before setting `passes: true`.
- It performs the relevant broad verification and returns a self-contained
  delivery summary.
- The graph contains exactly two AGENT_RUN workstations and two AGENT_WORKER
  definitions: planner and executor. It contains no script worker, reviewer,
  worktree, CI, merge, or loop-breaker stage.
- Root-built functional coverage uses a provider command-runner edge mock,
  asserts dispatch order `planner,executor`, proves the durable PRD handoff, and
  proves both roles inherit operator provider/model defaults without role args.
- Typecheck and tests pass.

### PF-FAMILIES-006: Implement dependency-aware `@you/plan-parallel`

Add the planner, generated task, executor, merge, and failure graph.

Acceptance criteria:

- Planner output creates only the configured `planned-task` Work type and
  preserves the original request on the waiting parent.
- Independent generated tasks become dispatchable concurrently, subject to the
  configured executor resource capacity.
- A task with `DEPENDS_ON` cannot dispatch before every required prerequisite
  reaches its required state.
- The merge workstation cannot dispatch until `ALL_CHILDREN_COMPLETE` matches
  the planner-generated child set.
- One failed child enables `ANY_CHILD_FAILED` and prevents a successful merge.
- Malformed, cyclic, unknown-type, and oversized plans create no partial child
  execution.
- The primary result is the merge result, not the raw submitted request or
  planner batch JSON.
- Functional tests assert dispatch/event ordering, dependency blocking,
  observable parallel readiness, fan-in, failure routing, role override
  selection, and replay-equivalent Work relationships.
- Typecheck and tests pass.

### PF-FAMILIES-007: Implement `@you/classify`

Add the classifier and three execution lanes.

Acceptance criteria:

- Classifier output `small`, `medium`, or `large` dispatches exactly one matching
  execution worker with the original request.
- Explicit classifier and lane provider/model overrides reach only their
  intended dispatches; omitted values use operator defaults.
- Empty, JSON-string, or unknown labels fail without running an execution lane.
- The selected lane's terminal output is the invocation primary result.
- CLI, API invocation, classifier routing, failure, and provider/model
  selection tests pass.
- Typecheck and tests pass.

### PF-FAMILIES-008: Preserve and harden JavaScript `@you/deep-research`

Keep the existing JavaScript workflow while bringing its description,
invocation, policy, failure, and result behavior into the shared packaged
catalog contract.

Acceptance criteria:

- The authored definition remains `orchestrator.kind: JAVASCRIPT` and continues
  to resolve its package-owned workflow source.
- The script launches between zero and the configured maximum number of
  specialists within effective agent and concurrency policy.
- Lead synthesis waits for all requested specialists and returns the final
  research answer in the documented result shape.
- Specialist or synthesis failure terminates the workflow with durable child,
  phase, and failure evidence.
- Existing topic, depth, specialist cap, provider, model, and reasoning inputs
  retain their documented behavior; `topic` also receives the common `--to`
  binding without removing positional or stdin input.
- JavaScript validation, runtime, cancellation, policy-limit, child ordering,
  and final-result tests preserve existing customer-visible behavior.
- Typecheck and tests pass.

### PF-FAMILIES-009: Align existing graph families

Complete descriptions, signatures, bounded loops, role overrides, examples,
and explicit return policies for goal, review, quorum, fusion, subagent, and
TTS without changing their distinct purposes.

Acceptance criteria:

- Each family satisfies the shared description and invocation contract.
- Goal and review cannot repeat without a configured terminal bound.
- Quorum still runs both independent branches before merge.
- Fusion still runs draft then refinement and exposes a usable result-file
  argument without colliding with run output mode.
- Subagent remains one-pass and read-only.
- TTS still selects the packaged local model, returns audio content, and
  preserves readiness/failure observability.
- Focused functional tests cover happy path, required input, explicit role
  overrides, terminal worker failure, and primary-result selection for every
  family.
- Typecheck and tests pass.

### PF-FAMILIES-010: Implement duration-scheduled `@you/loop`

Add invocation-scoped duration resolution and the long-lived execution graph.

Acceptance criteria:

- `you run -a @you/loop --every 1h --to <request>` parses and normalizes the
  positive duration before registering or dispatching anything.
- Each trigger creates a distinct execution Work item carrying nominal and
  actual trigger metadata and runs the original request exactly once.
- `trigger-at-start` produces one immediate execution only when enabled.
- A tick received during an active execution records a skipped trigger under
  `SKIP_WHILE_RUNNING` and does not overlap provider dispatches.
- Execution failure is projected and counted; later ticks continue until the
  configured consecutive-failure ceiling terminates the controller.
- Stop/cancel unregisters the schedule and prevents future executions, while
  live projection and replay agree on completed, failed, and skipped ticks.
- Unit and root-built functional tests use injected clocks/schedulers with no
  sleeps and cover invalid duration text, exact trigger counts, overlap, recovery after
  failure, failure exhaustion, cancellation, and restart/replay behavior.
- Typecheck and tests pass.

### PF-FAMILIES-011: Implement JavaScript `@you/tournament`

Add the bounded candidate bracket and 1v1 judge workflow.

Acceptance criteria:

- `rounds` accepts integers 1 through 3 and rejects invalid or unaffordable
  derived call counts before the first child launches.
- Round one launches exactly `2^rounds` candidates; each bracket level pairs
  candidates into 1v1 matches and launches exactly one judge per match.
- Matches at the same bracket level can execute concurrently within effective
  concurrency policy, and all children receive the original request plus their
  stable bracket identity.
- The judge must select candidate A or B and attach rationale; that selected
  candidate advances in deterministic bracket order and the sole champion
  becomes the invocation result.
- Candidate/judge failure or an invalid judge selection terminates with exact
  round and match provenance;
  cancellation prevents new matches from launching.
- JavaScript validation and injected-child functional tests cover rounds 1,
  2, and the maximum, derived call counts, ordering under out-of-order child
  completion, policy rejection, child/judge failure, invalid judge output,
  cancellation, role
  overrides, and final serialization.
- Typecheck and tests pass.

### PF-FAMILIES-012: Implement iterative worktree `@you/full-flow`

Package the planner, per-task worktree delivery pipeline, merge gate, and
dependency-backed loopback.

Acceptance criteria:

- Each non-complete planner pass emits one valid bounded task batch and one
  loopback Work item depending on every task's merged state.
- Independent tasks prepare unique safe worktrees and can implement
  concurrently without sharing a working directory.
- Every task passes implementation, review, required tests/CI, conflict
  handling, and actual merge before satisfying its loopback dependency.
- The loopback cannot dispatch early; after it dispatches, the planner inspects
  current repository and Factory state and may produce another wave.
- A validated complete decision succeeds only with no outstanding task or
  loopback Work and merged evidence for all accepted tasks.
- Invalid/oversized batches, unsafe names, task failure, review exhaustion,
  merge failure, and `max-cycles` exhaustion end in an explainable terminal
  failure rather than an orphaned worktree or infinite loop.
- Temporary-repository and root-built functional tests cover two concurrent
  tasks, distinct worktrees, merge ordering, a merge conflict repaired by the
  task, loopback blocking, two planning waves, final completion, child failure,
  and cycle exhaustion using injected provider/GitHub/process boundaries.
- Replay reconstructs every wave, task relationship, worktree identity, merge,
  and completion decision.
- Typecheck and tests pass.

### PF-FAMILIES-013: Implement JavaScript `@you/spawn`

Add exact-count task planning, parallel child execution, and top-level merge.

Acceptance criteria:

- `you run -a @you/spawn --count 10 --to "research the best places to travel"`
  reaches the JavaScript workflow with typed `count` and request values.
- `count` accepts integers 1 through 14 and the total planner, child, and merger
  calls must fit effective policy before child work begins.
- The planner must return exactly `count` distinct non-empty task descriptions;
  malformed output launches zero task children.
- Exactly one child runs for each planned task, children can run concurrently
  within policy, and ordered labeled results reach one top-level merger.
- The merger output is the invocation result. Child or merger failure cannot
  return a partial result as success.
- JavaScript validation and injected-child functional tests cover counts 1,
  10, and the maximum, wrong/duplicate task plans, exact child count,
  concurrency ceiling, out-of-order completion, cancellation, child/merge
  failure, role overrides, and final serialization.
- Typecheck and tests pass.

### PF-FAMILIES-014: Publish the complete documented catalog

Regenerate package artifacts and update customer documentation only after the
family behaviors are implemented and tested.

Acceptance criteria:

- The generated manifest contains exactly the supported authored catalog in
  stable lexical order with unique names, slugs, projects, descriptions,
  hashes, and examples.
- Generated JSON and YAML artifacts match their authored Factory sources and
  are not hand-edited.
- Root README and `you docs` examples use the implemented syntax and accurately
  distinguish plan-execute, plan-parallel, full-flow, loop, tournament, spawn,
  fusion, quorum, review, and classify.
- Documentation states the exact packaged count of fourteen and distinguishes
  graph-orchestrated families from JavaScript-orchestrated families.
- A clean package consumer can resolve the manifest and every advertised
  Factory JSON/YAML export.
- Docs smoke, package verification, typecheck, and tests pass.

## Test Strategy

### Unit and contract tests

- Factory validation: descriptions, invocation signature bindings, role
  placeholders, generated-Work limit, topology, guards, and return targets.
- Work admission: worker-emitted batch decoding, atomic validation,
  dependencies, parent lineage, cycles, oversized batches, cancellation, and
  deterministic request identity.
- CLI composition: `-a`, `--to`, positional/stdin parity, Factory-defined
  flags, reserved-name collisions, help, examples, and repeated execution.
- Workspace setup: temporary repositories, new/reused worktrees, PRD copy,
  progress preservation, unsafe branch names, and git/copy failures.
- Scheduled execution: invocation-resolved duration validation, registration,
  nominal/actual trigger metadata, overlap policy, failure counters,
  cancellation, and replay using injected clocks.
- JavaScript workflows: args-schema integer bounds, derived policy budgets,
  exact child counts, stable parallel result ordering, phase/failure records,
  cancellation, and terminal serialization for deep-research, tournament, and
  spawn.
- Mapping/contracts: authored schema, OpenAPI mappings, generated Go/TypeScript
  types, JSON/YAML parity, and manifest metadata.

### Functional tests

Functional application tests should use `root.BuildProcess` and
`Process.Execute` by default. Ordinary customer paths should enter through the
CLI. HTTP tests are appropriate for the packaged catalog contract and explicit
CLI/API invocation parity cells. External effects must be replaced through
`edges.Edges`, with `ProviderCommandRunner` and process-command edge mocks
preferred. Do not use `--with-mock-workers` outside the workers/mock feature.

Each packaged family receives focused coverage for:

- required-input success;
- missing/invalid input rejection before provider dispatch;
- explicit provider/model role overrides;
- omitted overrides using operator defaults;
- correct workstation dispatch sequence or routing;
- partial and terminal worker failures;
- bounded retry/review exhaustion;
- explicit primary-result selection; and
- canonical events/projections sufficient to explain the outcome.

Additional high-risk cells:

- Plan-execute: matching PRD file contents, exact planner-to-executor handoff,
  executor story completion/evidence, no extra dispatch stage, and operator
  provider/model inheritance using the command-runner boundary.
- Plan-parallel: independent readiness, dependency blocking, parallel dispatch
  capacity, guarded fan-in, child failure, invalid/cyclic/oversized batches,
  lineage, replay, and merge result.
- Deep research: preserved JavaScript source resolution, bounded specialist
  count, parallel child execution, synthesis, policy rejection, failure, and
  final JavaScript result projections.
- Classify: each label, exactly-one-lane dispatch, malformed labels, and model
  selection.
- Loop: exact fake-clock trigger counts, trigger-at-start, non-overlap, skipped
  tick evidence, executor failure and recovery, failure exhaustion,
  stop/cancel, durable restart, and replay without timing sleeps.
- Tournament: bracket sizes and calls for several round counts, parallel match
  execution, deterministic advancement, policy overflow, failure provenance,
  cancellation, and champion result.
- Full-flow: isolated concurrent worktrees, merge/CI/review gates, dependency
  loopback, repository reinspection across multiple waves, completion
  validation, failure bounds, and replay.
- Spawn: exact requested task count, plan validation, parallel children,
  deterministic merge input, policy and concurrency bounds, cancellation,
  failures, and merged result.

### UI verification

The dashboard's canonical data remains the packaged catalog API response. UI
projection tests should cover loading, empty, error, and populated inventory
states, with the populated state rendering descriptions. A browser integration
test should verify the hosted dashboard route displays the description returned
by a real backend catalog response. No per-family descriptions should live in
component state or a UI-only catalog.

### Focused commands

Use the narrowest relevant commands during each story, then run the shared
package and PR gates before merge:

```sh
make packaged-factory-catalog-generate
make packaged-factory-catalog-check
make packaged-factory-package-verify
make docs-reference-smoke
make api-smoke                 # when the public limits/schema contract changes
make ui-test                   # when packaged inventory presentation changes
make ui-lint                   # when packaged inventory presentation changes
make verify-fast
make lint
```

Run the focused Go packages for the family and changed owner before the broad
targets. Higher-risk Work admission, replay, Factory Session, or public API
changes should also run the relevant `make verify-pr` or functional tier.

## Delivery Sequence

1. PF-FAMILIES-001: descriptions and discovery.
2. PF-FAMILIES-002: invocation conventions.
3. PF-FAMILIES-003: bounded generated Work contract.
4. PF-FAMILIES-004 and PF-FAMILIES-005: complete `@you/plan-execute`.
5. PF-FAMILIES-006: `@you/plan-parallel`.
6. PF-FAMILIES-007: `@you/classify`.
7. PF-FAMILIES-008: preserve and harden JavaScript deep-research.
8. PF-FAMILIES-009: existing graph-family alignment.
9. PF-FAMILIES-010: invocation-scoped duration scheduling and `@you/loop`.
10. PF-FAMILIES-011: JavaScript `@you/tournament`.
11. PF-FAMILIES-012: `@you/full-flow`, after generated Work and worktree
    delivery foundations are stable.
12. PF-FAMILIES-013: JavaScript `@you/spawn`.
13. PF-FAMILIES-014: final catalog and documentation publication.

Each story should be a vertically sliced, independently reviewable PR when
practical. Shared schema and generated-Work behavior land before Factories that
depend on them. Avoid combining unrelated cleanup with a family behavior.

## Project Acceptance Criteria

- All fourteen planned families resolve from the packaged catalog and execute
  through their documented graph or JavaScript orchestrator.
- `@you/plan-execute` creates Markdown and JSON PRDs and passes them directly to
  one executor in the current workspace, with no script, worktree, reviewer,
  CI, or merge stage.
- `@you/plan-parallel` expresses planner output as canonical Work and
  relationships, uses runtime dependency scheduling and concurrency, and
  returns a guarded merged result.
- `@you/loop` runs the submitted request on its validated invocation-supplied
  duration interval until stopped, with durable execution and skipped-trigger
  evidence.
- `@you/tournament` executes a policy-bounded JavaScript bracket and returns its
  final refined champion.
- `@you/full-flow` runs isolated parallel implementation worktrees through
  merge, then dependency-loopbacks until a validated completion decision.
- `@you/spawn` plans exactly the requested number of tasks, runs them through
  bounded JavaScript parallelism, and returns the top-level merged result.
- `@you/deep-research` remains JavaScript and retains its documented behavior.
- Every packaged Factory has a useful description visible through CLI, API,
  npm manifest data, and dashboard discovery.
- Every family has direct happy-path, failure-path, invocation, provider/model,
  and primary-result test evidence proportional to its risk.
- Graph families use readable authored `factory.yaml` definitions. JavaScript
  families use one self-contained authored `factory.js` each and no sidecar
  `factory.json`; generated portable JSON/YAML, manifest hashes,
  OpenAPI-derived artifacts, docs, and runtime behavior remain aligned.
- Required typecheck, lint, unit, functional, contract, package, documentation,
  and browser checks pass for their affected surfaces.
- Delivery continues through blocking review feedback, required CI, conflict
  resolution, and actual PR merge; an opened or green PR is not complete.

## Delivery Loop

For every implementation PR, continue the implementation and review loop until
required CI is terminal and passing, every blocking PR conversation item is
explicitly addressed, merge conflicts and shared-file drift are resolved, and
the PR is merged. A PR that is only opened, approved, green, or ready to merge
does not complete its story.
