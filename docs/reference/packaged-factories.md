# Packaged Factories

YOU ships **fifteen** first-party Factories under the `@you/` namespace. This
page is the canonical operator guide for that catalog. The authored sources
live in `packages/packaged-factories/factories/`; the published catalog is
described by `packages/packaged-factories/generated/manifest.json`.

## Discovery and first use

### Find the Factory name

Run `you factory list` to inspect the effective Factory catalog. First-party
packaged entries use the `@you/<name>` namespace; other entries can be
project-local or operator-installed Factories. The command is discovery only:
it does not run a Factory. Project-local named Factories take precedence when
their name conflicts with an operator-level entry.

Packaged sources are materialized lazily under
`~/.you-agent-factory/factories` (Windows:
`%USERPROFILE%\.you-agent-factory\factories`) when a named Factory needs a
local copy. Materialized copies remain editable. Listing the catalog and
asking for help are useful different operations: `you factory list` finds the
name, while `you run --named <factory> --help` resolves the selected Factory's
current invocation contract without running its work.

### Read the live invocation contract

Use the exact name from the list when asking for help:

```bash
you run --named @you/goal --help
```

The help output is authoritative for the selected revision. It shows the
Factory-defined positional or `--to` request binding, role-specific provider
and model flags, required values, defaults, and authored examples. Run-level
options such as `--json`, `--output`, `--record`, and `--no-record` remain
available on the same `you run` command, but they are not substitutes for
Factory-defined arguments.

When a worker role omits its provider or model override, the runtime resolves
the value through the operator defaults configured by `you init` and operator
settings. A role-specific flag shown by one Factory is not automatically
valid for another Factory; copy flags from that Factory's live help.

The help command itself is safe for checking a signature. An actual run can
start provider processes, contact a configured provider, create a Factory
Session, write recordings or artifacts, and—depending on the Factory—change
the selected workspace. Read each entry's prerequisite and side-effect notes
before replacing its help command with a work request.

### Verified discovery output

These commands were run against the current `you` binary while maintaining
this topic. The catalog output contained all fifteen `@you/*` names; the help
output below is the live boundary for `@you/goal`:

```text
Factory invocation help

Selected factory: goal (named factory @you/goal)

Usage:
  you run --named @you/goal <to> [--executor-model <value>] [--executor-provider <value>]

Factory-defined arguments:
  positional 1 <to> | --to <value>
    Goal to pursue until completion.
    Required.
    Reads from stdin when provided.
  --executor-model <value>
    Model for the goal executor; omitted values use operator defaults.
    Optional.
  --executor-provider <value>
    Model provider for the goal executor; omitted values use operator defaults.
    Optional.
```

The directory column in `you factory list` is operator-specific, so entries
should be identified by their exact Factory name rather than by an installed
path. Use `you docs packaged-factories` to reopen this guide from the CLI.

## Common entry contract

Every detailed Factory entry in this guide uses the same order so an operator
can compare workflows without guessing which facts are omitted:

1. **Purpose and suitable use** — what the Factory is for and what kind of
   request belongs there.
2. **Invocation signature** — the exact live request binding and
   Factory-defined flags; use `you run --named <factory> --help` when the
   installed Factory may differ from this page.
3. **Worker roles and provider/model overrides** — every role exposed by the
   signature, the supported override flags, and the operator-default behavior
   when a value is omitted.
4. **Prerequisites and side effects** — required provider or model readiness,
   workspace, session, recording, artifact, process, network, or
   long-running behavior that affects safe use.
5. **Expected output shape** — the primary returned result, any structured or
   human-readable selection, and important intermediate or terminal states.
6. **Worked invocation** — one copy-ready command using only flags accepted by
   the live signature.
7. **Observed output evidence** — output from that exact command or a direct
   behavioral assertion; model-generated prose is evidence of shape, not a
   deterministic golden.

The catalog below is the quick navigation index. Search for the exact
`@you/<name>` in this page, then use the matching detailed entry when choosing
among the bounded, planning, parallel, subagent, and local-media families.

## Output stability and evidence

Provider and model responses are generated content. Their prose, ordering of
optional wording, and byte representation are not guaranteed to be stable
across runs, model versions, or providers. Documentation therefore describes
the output shape—such as a primary result, a structured classification, a
candidate set, a synthesized answer, or an audio artifact—rather than
promising exact model text.

Remember: model-generated prose is not byte-stable.

For automation, use the run-level structured output mode when the selected
Factory supports it and assert the documented envelope, status, fields, or
artifact existence. For human use, the default output is intended to be
readable. A worked example's captured prose demonstrates a successful shape;
it is not a promise of deterministic content.

## Catalog

| Family | Factory | Orchestrator | Description |
|---------|---------|--------------|-------------|
| Planning and implementation | `@you/factory-builder` | Graph | Creates and installs one validated graph or JavaScript Factory from a customer request. Answers with usage guidance when the request is a question rather than a build request. |
| Bounded and iterative | `@you/classify` | Graph | Classifies a request by complexity and routes it to the configured small, medium, or large model lane. |
| Parallel investigation and selection | `@you/deep-research` | JavaScript | Breaks a research question into bounded specialist investigations and synthesizes their findings. |
| Planning and implementation | `@you/full-flow` | Graph | Plans parallel implementation waves in isolated worktrees, merges completed tasks, and replans until the project is complete. |
| Bounded and iterative | `@you/fusion` | Graph | Produces an initial answer with one worker and refines it with a second worker. |
| Bounded and iterative | `@you/goal` | Graph | Repeatedly works a goal until the executor reports completion or the Factory reaches a failure bound. |
| Bounded and iterative | `@you/loop` | Graph | Runs the requested task at a duration interval such as `1h` for the lifetime of the Factory Session. |
| Planning and implementation | `@you/plan-execute` | Graph | Writes a Markdown and JSON PRD for a project request, then executes that plan in the current workspace. |
| Planning and implementation | `@you/plan-parallel` | Graph | Plans a dependency graph of Work, executes ready tasks concurrently, and merges the completed results. |
| Parallel investigation and selection | `@you/quorum` | Graph | Runs independent assessments in parallel and merges them into one final answer. |
| Bounded and iterative | `@you/review` | Graph | Produces candidate work and repeats independent review until approval or a bounded failure. |
| Parallel investigation and selection | `@you/spawn` | JavaScript | Plans an exact number of independent tasks, runs them concurrently, and merges their results into one answer. |
| Single bounded call | `@you/subagent` | Graph | Runs one bounded read-only subagent and returns its result. |
| Parallel investigation and selection | `@you/tournament` | JavaScript | Runs candidates through bounded 1v1 matches, uses a judge to advance each winner, and returns the champion result. |
| Local media | `@you/tts` | Graph | Converts submitted text to audio with the packaged local text-to-speech model. |

Graph Factories use canonical Work, relationships, resources, guards, and
runtime scheduling. The JavaScript Factories (`@you/deep-research`,
`@you/spawn`, `@you/tournament`) use the policy-bounded JavaScript orchestrator
where invocation-shaped fan-out is part of their design.

## Detailed bounded and iterative entries

The five entries in this family all accept one request, but they do different
things with it: `goal` keeps working toward completion, `loop` schedules
recurring executions, `fusion` performs a draft/refine pair, `classify` routes
to one complexity lane, and `review` repeats produce/review attempts until an
independent reviewer approves. The worked commands below deliberately include
`--with-mock-workers --no-record --quiet` so they are deterministic smoke
invocations against the current binary. Remove the mock-worker flag for live
provider execution after configuring the providers and models described in
each entry.

### `@you/goal`

**Purpose and suitable use.** Use `@you/goal` for one objective that may need
several verified passes, such as an implementation or investigation that can
make observable progress between attempts. It persists the objective and each
pass's result, then stops when the executor returns the `accepted` decision or
when the bounded Factory failure path is reached. Choose it for a persistent
single objective, not for a one-shot answer or a timer-driven recurring check.

**Invocation signature.** The live signature is:

```text
you run --named @you/goal <to> [--executor-model <value>] [--executor-provider <value>]
```

`<to>` is required and can be supplied positionally, through `--to`, or via
stdin. `--executor-provider` and `--executor-model` are optional.

**Worker roles and provider/model overrides.** The Factory has one worker role,
`goal-executor`. Its provider and model are selected by
`--executor-provider` and `--executor-model`; omitting either value delegates
resolution to the operator defaults. These flags configure the executor only;
they do not create a second review or merge role.

**Prerequisites and side effects.** A usable configured provider/model is
required for a live pass. Each pass creates runtime/session activity and may
inspect or change the selected workspace when the objective calls for it. The
worker maintains a JSON progress file under
`.you-goals/<factory-session-id>/<work-id>.json`; it keeps the original
objective and records the iteration, status, last result, and update time. The
Factory permits up to twelve visits to its execute workstation before routing
the Work to failure. Normal runs record events unless a run-level option such
as `--no-record` is supplied. A failed or exhausted goal does not provide a
successful primary result.

**Expected output shape.** The caller receives the executor's final verified
result, not the internal progress JSON or a transcript of every attempt. Human
output is readable text; structured run output exposes a completed invocation
result with a primary text result and status, while the event stream contains
the intermediate dispatches and decisions. Generated prose is provider/model
content and is not byte-stable.

**Worked invocation.**

```bash
you run --named @you/goal --with-mock-workers --no-record --quiet --executor-provider CODEX --executor-model gpt-5 --to "Check the README headings"
```

**Observed output evidence.** The exact command above returned:

```text
mock worker accepted
```

The mock result demonstrates the successful primary-result shape; it is not a
claim that a live model will return that wording.

### `@you/loop`

**Purpose and suitable use.** Use `@you/loop` for a request that should be
executed repeatedly on a duration schedule, such as checking dependencies or
polling a bounded inbox. Each trigger creates one isolated execution of the
same request. Choose it when recurrence is the behavior; use `goal` when the
worker should keep refining one objective toward completion instead.

**Invocation signature.** The live signature is:

```text
you run --named @you/loop <to> [--every <value>] [--executor-model <value>] [--executor-provider <value>] [--max-consecutive-failures <number>] [--trigger-at-start <value>]
```

`<to>` is required and accepts positional, `--to`, or stdin input. `--every`
accepts a positive duration from `1s` through `168h` and defaults to `1h`.
`--trigger-at-start` defaults to `true`, so the first execution normally does
not wait for the first full interval. `--max-consecutive-failures` defaults to
`0`, which means that failures do not stop scheduling; set a positive bound
when an unattended loop must fail closed. The executor provider and model are
optional.

**Worker roles and provider/model overrides.** `loop-trigger` is the scheduler
role and has no provider/model override. `loop-executor` performs each request
once with no memory of earlier triggers; `--executor-provider` and
`--executor-model` configure that role, with omitted values using operator
defaults. The trigger role preserves the complete request and does not rewrite
it into a summary.

**Prerequisites and side effects.** A live loop needs a configured provider and
model for every execution. The Factory Session owns an active controller and
can therefore be long-running; the one-shot invocation returns the first
completed scheduled execution, while a durable/continuous session can keep
the controller available for later triggers. Executions are serialized through
one executor slot, may inspect or change the selected workspace, and can
create recordings and provider processes. With `--trigger-at-start false`, a
caller that is waiting for the first result may wait for the interval. A
positive consecutive-failure bound is the operator's stop condition.

**Expected output shape.** The primary result is one completed scheduled
execution, not a list of future results and not the controller's internal
state. Session events and recordings can show later triggers, skipped runs,
and failure counts. Human output is the selected execution's readable result;
structured output can be used to inspect status and event metadata. Model
prose is not deterministic across executions.

**Worked invocation.**

```bash
you run --named @you/loop --with-mock-workers --no-record --quiet --every 1h --trigger-at-start true --max-consecutive-failures 1 --executor-provider CODEX --executor-model gpt-5 --to "Check dependency updates"
```

**Observed output evidence.** The exact command above returned:

```text
mock worker accepted
```

The immediate trigger is why this smoke command returns without waiting an
hour; a live session still follows the configured schedule after that first
execution.

### `@you/fusion`

**Purpose and suitable use.** Use `@you/fusion` when one worker should create a
complete first draft and a separate worker should validate, repair, and polish
that draft into the caller's answer. It is a fixed two-stage refinement flow,
not a recurring loop, classifier, or fan-out/merge over several independent
branches.

**Invocation signature.** The live signature is:

```text
you run --named @you/fusion <to> [--first-effort <value>] [--first-model <value>] [--first-provider <value>] [--result-file <file-path>] [--second-effort <value>] [--second-model <value>] [--second-provider <value>]
```

`<to>` is required and accepts positional, `--to`, or stdin input.
`--first-provider`/`--first-model` configure the first pass;
`--second-provider`/`--second-model` configure the refiner. Both effort flags
accept `low`, `medium`, or `high` and default to `medium`. `--result-file` is
an optional file-path output hint.

**Worker roles and provider/model overrides.** `fusion-drafter` is the first
role and uses the `first-*` provider, model, and effort inputs.
`fusion-refiner` is the second role and uses the matching `second-*` inputs.
Provider/model values omitted for either role use operator defaults; the two
roles can intentionally use different providers or models.

**Prerequisites and side effects.** A live run needs both selected provider
routes and normally performs two model-backed dispatches in sequence. The
workers may inspect relevant workspace evidence and change the workspace when
the request asks for implementation work. The first draft is held as
intermediate Work until the refiner completes. When `--result-file` is
supplied, the output contract identifies the refined result as file-oriented
Markdown content; there is no fixed default output path. Normal runs can write
recordings, session state, provider artifacts, or workspace changes.

**Expected output shape.** The primary returned result is the second worker's
refined answer. The first worker's draft and the refiner's internal checking
are intermediate activity, not a second answer set. Without `--result-file`,
the result is human-readable text by default; with it, callers can treat the
refined content as the documented Markdown file-oriented result. Generated
prose is not byte-stable.

**Worked invocation.**

```bash
you run --named @you/fusion --with-mock-workers --no-record --quiet --first-provider CODEX --second-provider CODEX --to "Draft a release summary"
```

**Observed output evidence.** The exact command above returned:

```text
mock worker accepted
```

This is the final synthetic refiner result; the first-stage mock dispatch is
not printed by `--quiet`.

### `@you/classify`

**Purpose and suitable use.** Use `@you/classify` when the same request may be
small, medium, or large and you want a model to select the execution lane.
The classifier makes one complexity decision, then exactly one of the three
lane workers handles the request. It is a routed single result, not a quorum
or a parallel comparison of all three lanes.

**Invocation signature.** The live signature is:

```text
you run --named @you/classify <to> [--classifier-model <value>] [--classifier-provider <value>] [--large-model <value>] [--large-provider <value>] [--medium-model <value>] [--medium-provider <value>] [--small-model <value>] [--small-provider <value>]
```

`<to>` is required and accepts positional, `--to`, or stdin input. The
classifier and each of the small, medium, and large lanes have optional
provider and model overrides. Omitted values for any role use operator
defaults. A provider/model override for one lane does not change the other
lanes.

**Worker roles and provider/model overrides.** `complexity-classifier` uses
`--classifier-provider` and `--classifier-model` and must return exactly one of
the labels `small`, `medium`, or `large`. The selected role is one of
`small-executor`, `medium-executor`, or `large-executor`, configured by its
matching `--<lane>-provider` and `--<lane>-model` flags. Only the selected lane
is dispatched after classification.

**Prerequisites and side effects.** A live run needs a usable classifier route
and a usable provider/model for whichever lane is selected. The classifier
adds one model-backed step before the selected executor; an invalid label,
classifier failure, or selected-lane failure leaves the invocation without a
primary result. Lane workers can inspect or change the selected workspace
according to the request, and normal runs can create recordings, session
state, provider processes, and workspace artifacts. The checked-in mock config
used below is a documentation smoke fixture: it returns the valid `small`
label through the classifier protocol and leaves the selected executor on the
default accepted mock behavior.

**Expected output shape.** The caller receives the selected lane executor's
final result. The `small`/`medium`/`large` classification is an intermediate
routing decision, not the primary answer; use structured events or recording
inspection when the selected label itself matters. Human output is the final
lane result, and model-generated prose is not byte-stable.

**Worked invocation.**

```bash
you run --named @you/classify --with-mock-workers ./docs/examples/packaged-classify-mock-workers.json --no-record --quiet --classifier-provider CODEX --classifier-model gpt-5 --small-provider CODEX --small-model gpt-5 --medium-provider CODEX --medium-model gpt-5 --large-provider CODEX --large-model gpt-5 --to "Explain the failing test"
```

**Observed output evidence.** The exact command above returned:

```text
mock worker accepted
```

The fixture's classifier response is `small`; `--quiet` intentionally prints
only the selected executor's primary result.

### `@you/review`

**Purpose and suitable use.** Use `@you/review` when a request needs a
candidate produced by one worker and independently checked by another. A
review rejection sends the Work back to the writer with the rejection
feedback, so the next pass can revise the candidate. Choose it for
produce-and-review quality control, not for a one-pass answer or a simple
parallel opinion poll.

**Invocation signature.** The live signature is:

```text
you run --named @you/review <to> [--reviewer-model <value>] [--reviewer-provider <value>] [--writer-model <value>] [--writer-provider <value>]
```

`<to>` is required and accepts positional, `--to`, or stdin input. All four
provider/model flags are optional. The writer flags configure candidate
production, and the reviewer flags configure independent checking; omitted
values use operator defaults.

**Worker roles and provider/model overrides.** `review-work-executor` is the
writer and uses `--writer-provider`/`--writer-model`.
`review-work-reviewer` is the reviewer and uses
`--reviewer-provider`/`--reviewer-model`. The reviewer emits a decision
envelope rather than becoming the caller's final answer. The roles can use
different provider/model combinations.

**Prerequisites and side effects.** A live run needs both provider routes. The
writer may inspect or change the selected workspace when the request concerns
code or artifacts; the reviewer independently checks the request, candidate,
workspace evidence, compatibility, and verification. Rejections can cause
additional provider calls and longer execution. The packaged Factory permits
up to eight reviewer visits before its failure path; there is no user-facing
attempt flag in the live signature. Normal runs create session/recording
activity and can leave the workspace or other artifacts changed by an
accepted writer pass.

**Expected output shape.** The primary returned result is the complete writer
candidate from the pass that the reviewer accepts. Reviewer rationale and
rejected candidates are intermediate feedback, not additional primary results.
Human output is the approved candidate text; structured events expose the
review decisions and retry activity. If the reviewer never accepts or a
provider fails, the invocation returns a terminal failure instead of claiming
approval. Model prose is not byte-stable.

**Worked invocation.**

```bash
you run --named @you/review --with-mock-workers --no-record --quiet --reviewer-provider CODEX --reviewer-model gpt-5 --writer-provider CODEX --writer-model gpt-5 --to "Draft the release notes"
```

**Observed output evidence.** The exact command above returned:

```text
mock worker accepted
```

The empty mock configuration accepts the writer and reviewer decision
envelope, so this smoke run reaches the approved primary-result path without
calling a live provider.

## Representative invocations

```bash
you run --named @you/goal --to "Ship the login fix by Friday"
you run --named @you/plan-execute --to "Implement the requested feature"
you run --named @you/plan-parallel --to "Implement the requested feature"
you run --named @you/review --to "Draft the release notes"
you run --named @you/classify --to "Explain and fix the failing test"
you run --named @you/quorum --to "Compare the two proposed release plans."
you run --named @you/loop --every 1h --to "Check dependency updates"
you run --named @you/tournament --rounds 2 --to "Propose a launch strategy"
you run --named @you/full-flow --to "Complete the requested project"
you run --named @you/spawn --count 10 --to "Research the best places to travel"
you run --named @you/fusion --to "Draft a release summary"
you run --named @you/subagent --to "Summarize this release"
you run --named @you/deep-research --to "Compare event sourcing and state machines"
you run --named @you/tts --output primary --to "The release is ready."
```

Positional text is accepted wherever the Factory's `invocationSignature` binds
`POSITIONAL` for the primary request. Named `--to` is the portable form shown
above. Pipe stdin instead of supplying both positional text and stdin.

## Role-specific provider and model flags

Many packaged Factories expose per-role overrides. Common examples:

| Factory | Role flags |
|---------|------------|
| `@you/goal` | `--executor-provider`, `--executor-model` |
| `@you/plan-execute` | `--planner-provider`, `--planner-model`, `--executor-provider`, `--executor-model` |
| `@you/plan-parallel` | planner / executor / merge provider and model flags |
| `@you/review` | `--writer-provider`, `--writer-model`, `--reviewer-provider`, `--reviewer-model` |
| `@you/classify` | classifier / small / medium / large provider and model flags |
| `@you/quorum` | `--branch-provider`, `--branch-model`, `--merge-provider`, `--merge-model` |
| `@you/fusion` | `--first-provider`, `--second-provider` (and matching model flags) |
| `@you/tournament` | competitor and judge provider / model flags |
| `@you/spawn` | `--worker-provider`, `--worker-model`, plus `--count` |

Always confirm the live flag set with `you run --named <factory> --help`.
Omitted provider and model values fall back to operator defaults from
`you init` / operator settings.

Save-money style plan then execute:

```bash
you run --named @you/plan-execute \
  --planner-provider codex --planner-model gpt-5 \
  --executor-provider codex --executor-model gpt-5 \
  --to "Implement and verify the requested feature"
```

Parallel plan then execute:

```bash
you run --named @you/plan-parallel \
  --planner-provider codex --planner-model gpt-5 \
  --executor-provider cursor --executor-model composer-2.5 \
  --merge-provider codex --merge-model gpt-5 \
  --to "Implement and test the requested feature"
```

Adversarial review loop:

```bash
you run --named @you/review \
  --writer-provider codex --writer-model gpt-5 \
  --reviewer-provider cursor --reviewer-model composer-2.5 \
  --to "Draft the release notes"
```

Classify then route by complexity:

```bash
you run --named @you/classify \
  --classifier-model gpt-5 \
  --small-model gpt-5 \
  --medium-model gpt-5 \
  --large-model gpt-5 \
  --to "Build me this one hundred page spec, make no mistakes"
```

## Materialization and editing

1. `you factory list` — see the fifteen catalog entries.
2. `you run --named @you/goal --help` — materializes `@you/goal` without running
   work when you only need the generated help / local copy.
3. Edit the materialized Factory under
   `~/.you-agent-factory/factories/@you/goal` (or the Windows equivalent).
4. `you factory config validate ~/.you-agent-factory/factories/@you/goal`
5. Re-run with `you run --named @you/goal ...`

Project-local named Factories resolve before the operator-level packaged
catalog. Author custom Factories with graph config or JavaScript; see
[`authoring-factories.md`](./authoring-factories.md) and
[`javascript-workflows.md`](./javascript-workflows.md).

## Related

- Catalog manifest: `packages/packaged-factories/generated/manifest.json`
- Authored sources: `packages/packaged-factories/factories/<slug>/`
- Harness inventory: [`harnesses.md`](./harnesses.md)
- `you docs run` — invocation inputs, stdout modes, server/site flags
- `you docs authoring-factories` — authoring workflow and packaged examples
- `you docs javascript-workflows` — JavaScript orchestrator Factories
- `you docs models` — local TTS readiness for `@you/tts`
