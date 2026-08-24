# Packaged Factories

YOU ships **seventeen** first-party Factories under the `@you/` namespace. This
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
this topic. The catalog output contained all seventeen `@you/*` names; the help
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
among the bounded, planning, parallel, subagent, local-media, and media-review families.

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
| Media review | `@you/agy-clip-qa` | Graph | Gates a rendered clip against its shot specification with ANTIGRAVITY and returns a schema-validated pass-or-reroll verdict. |
| Media review | `@you/agy-cold-watch` | Graph | Reviews a completed cut from first principles with ANTIGRAVITY, including chronology, temporal defects, audio, observed speech, and a pass-or-reroll recommendation. |

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

## Detailed planning and implementation entries

These four Factories are all planning-oriented, but their boundaries are
different. `factory-builder` authors and installs a reusable Factory;
`plan-execute` creates a PRD and implements it in the current workspace;
`plan-parallel` runs a dependency-aware task graph in one repository and
synthesizes the results; `full-flow` adds isolated Git worktrees, review, CI,
local merges, and bounded replanning. Choose the smallest boundary that fits
the job.

The worked commands below were run against the current binary. The first
three use `--with-mock-workers` to prove the invocation and terminal output
envelope without contacting a live model. Generic mock acceptance does not
simulate a builder's file tools or a planner's valid batch JSON; the normal
provider-backed behavior and artifact guarantees are described separately in
each entry. The `full-flow` smoke uses a controlled protocol-valid planner
response and a bounded cycle limit to reach its successful terminal result,
while the repository's functional coverage uses protocol-valid provider
responses for the complete worktree/review/merge loop. An intentionally
out-of-range `--max-cycles 9` invocation is retained separately as an
additional argument-validation check.

### `@you/factory-builder`

**Purpose and suitable use.** Use `@you/factory-builder` when a customer
request should become a new reusable named Factory. It can author either a
graph/YAML Factory or a JavaScript Factory, and it answers usage guidance when
the request is blank or a question rather than a build request. It is not a
general-purpose task executor and should not be used to modify an existing
Factory in place.

**Invocation signature.** The live signature is:

```text
you run --named @you/factory-builder [<to>] [--builder-model <value>] [--builder-provider <value>] [--factory-name <value>] [--orchestrator <value>]
```

`<to>` is optional and accepts positional, `--to`, or stdin input.
`--orchestrator` defaults to `graph` and accepts `graph` or `javascript`.
`--factory-name` is optional; the builder derives a name when it is omitted.

**Worker roles and provider/model overrides.** `builder-router` decides
whether the request is a build or help request, `builder-helper` supplies
the authored guidance for the help path, and `factory-builder` authors the
candidate with its agent tools. `--builder-provider` and `--builder-model`
apply to all three roles; omitted values use operator defaults.

**Prerequisites and side effects.** A live build needs a configured provider
and model and a workspace in which the builder can stage a new
factory-name-scoped directory. The builder reads the Factory authoring and
JavaScript workflow docs, validates the staged candidate with
`you factory config validate`, then installs it through
`you factory create <name> --from <stage> --dir ~/.you-agent-factory/factories`.
It does not use `./factory`, the packaged-factory source tree, an existing
Factory directory, or an existing name as a place to overwrite. A validation
failure or name collision leaves the candidate uninstalled. A successful run
changes the operator's local Factory catalog and may leave the requested
workspace artifacts.

**Expected output shape.** A successful build returns the canonical Factory
name, the selected orchestrator form, validation success, and confirmation
that the named-Factory create command installed it. The help path returns
usage guidance without creating or installing anything. A failed validation
returns a terminal failure and explicitly says that installation did not
occur. The exact explanatory prose is model-generated.

**Worked invocation.**

```bash
you run --named @you/factory-builder --with-mock-workers --no-record --quiet --factory-name docs-release-review --orchestrator graph --to "Create a Factory that reviews release notes and returns an approval summary."
```

**Observed output evidence.** The exact command above returned:

```text
mock worker accepted
```

That synthetic result only proves the current CLI can select and terminate
the entry. The repository's provider-backed functional coverage separately
asserts graph creation, JavaScript creation, validation, installation, and
the no-install-on-invalid-candidate failure path.

### `@you/plan-execute`

**Purpose and suitable use.** Use `@you/plan-execute` when one project should
first be expressed as a durable PRD and then implemented by an executor in
the current workspace. It is sequential and workspace-local: it does not
create task worktrees, merge branches, open a pull request, or perform remote
review.

**Invocation signature.** The live signature is:

```text
you run --named @you/plan-execute <to> [--executor-model <value>] [--executor-provider <value>] [--planner-model <value>] [--planner-provider <value>]
```

`<to>` is required and accepts positional, `--to`, or stdin input. The
planner and executor provider/model overrides are independent; omitted values
use operator defaults.

**Worker roles and provider/model overrides.** `prd-planner` investigates the
request and writes both `tasks/todo/<workname>.md` and
`tasks/todo/<workname>.json`, then emits `<COMPLETE>`. `prd-executor` reads
that plan, works in the current workspace, verifies the result, and emits
`<COMPLETE>`. `--planner-provider`/`--planner-model` configure the first role;
`--executor-provider`/`--executor-model` configure the second.

**Prerequisites and side effects.** The current workspace must be writable and
the selected provider/model must be usable for both roles. The planner's
Markdown and JSON files are durable workspace artifacts. The executor may
edit any files required by the request and run its checks. Normal runs create
session/recording activity and provider artifacts; `--no-record` suppresses
the normal recording side effect. The Factory does not itself open a PR or
merge a remote branch.

**Expected output shape.** The terminal result is the executor's completed
implementation report, not a separate planner answer. The durable PRD files
remain in `tasks/todo/`, and structured output exposes the completed status
and primary result. If planning or execution fails, the invocation is
terminally failed rather than reported as implemented. Model prose and the
generated work-name suffix are not byte-stable.

**Worked invocation.**

```bash
you run --named @you/plan-execute --with-mock-workers --no-record --quiet --planner-provider CODEX --planner-model gpt-5 --executor-provider CODEX --executor-model gpt-5 --to "Create a one-line README note and verify it."
```

**Observed output evidence.** The exact command above returned:

```text
mock worker accepted
<COMPLETE>
```

The first line is the synthetic planner result and the stop token marks the
synthetic executor's terminal result. A live provider run is the path that
creates and executes the PRD artifacts described above.

### `@you/plan-parallel`

**Purpose and suitable use.** Use `@you/plan-parallel` when a request can be
decomposed into independent or dependency-ordered implementation tasks and
the final answer should synthesize all completed task reports. It keeps the
tasks in the same repository and is a good fit for parallel investigation or
changes that do not need per-task Git worktrees. Use `full-flow` when isolated
branches, review, CI, and local merging are part of the requirement.

**Invocation signature.** The live signature is:

```text
you run --named @you/plan-parallel <to> [--executor-model <value>] [--executor-provider <value>] [--executor-reasoning-effort <value>] [--merge-model <value>] [--merge-provider <value>] [--planner-model <value>] [--planner-provider <value>]
```

`<to>` is required and accepts positional, `--to`, or stdin input.
`--executor-reasoning-effort` defaults to `xhigh`. Planner, executor, and
merger provider/model overrides are independent and omitted values use
operator defaults.

**Worker roles and provider/model overrides.** `parallel-planner` must emit a
raw `FACTORY_REQUEST_BATCH` containing 1–12 self-contained `planned-task`
items with acyclic optional `DEPENDS_ON` relations. It must not create a
catch-all synthesis task. `parallel-executor` runs each ready task with the
selected executor provider/model and reasoning effort. `parallel-merger`
receives every completed child and produces the final synthesis with the
selected merge provider/model.

**Prerequisites and side effects.** A live run needs a writable repository and
usable planner, executor, and merger routes. Up to eight task executions may
consume the `parallel-executor-slots` resource concurrently. All agents work
in the same repository; there is no authored worktree or Git-branch merge
step, so independent tasks should avoid overlapping files. A child failure
fails the parent and prevents final synthesis. Normal runs can change the
workspace and create recordings, provider sessions, and artifacts.

**Expected output shape.** The terminal result is one merger-produced,
customer-facing synthesis that incorporates the completed task reports. The
planner's DAG and individual executor reports are intermediate Work and can
be inspected through runtime events, but they are not printed as separate
primary answers. Planner validation, unsupported reasoning effort, or child
failure produces a terminal failure. Generated task names and prose are not
stable.

**Worked invocation.**

```bash
you run --named @you/plan-parallel --with-mock-workers --no-record --quiet --planner-provider CODEX --planner-model gpt-5 --executor-provider CODEX --executor-model gpt-5 --merge-provider CODEX --merge-model gpt-5 --to "Create a one-line README note and verify it."
```

**Observed output evidence.** The exact command above returned:

```text
mock worker accepted
```

This is the generic mock's terminal merger text. It does not claim that the
mock invented a valid task DAG; provider-backed functional coverage asserts
the bounded planner batch, dependency-aware dispatch, concurrency, merger,
and failure paths.

### `@you/full-flow`

**Purpose and suitable use.** Use `@you/full-flow` for a project that needs
bounded parallel implementation waves with independent worktrees, review,
CI, local branch merges, and replanning after each wave. It is the most
operationally involved planning entry in this catalog. Use it when the
repository itself is the delivery target and the request benefits from
separate task branches.

**Invocation signature.** The live signature is:

```text
you run --named @you/full-flow <to> [--base-branch <value>] [--executor-model <value>] [--executor-provider <value>] [--max-cycles <number>] [--max-tasks-per-cycle <number>] [--planner-model <value>] [--planner-provider <value>] [--reviewer-model <value>] [--reviewer-provider <value>]
```

`<to>` is required and accepts positional, `--to`, or stdin input.
`--base-branch` defaults to `main`; `--max-cycles` and
`--max-tasks-per-cycle` each accept 1–8 and default to 4. Planner, executor,
and reviewer provider/model overrides are optional and independently resolve
to operator defaults when omitted.

**Worker roles and provider/model overrides.** `full-flow-planner` produces a
bounded raw batch of `delivery-task` Work plus one `cycle-control` Work.
`task-worktree-setup` is a packaged script that creates or reuses a safe task
worktree from the selected base branch. `full-flow-executor` implements one
task, `full-flow-reviewer` independently reviews it, `full-flow-ci` verifies
it, and `full-flow-merger` merges the verified task branch. The
`cycle-decider` script routes the planner's exact `continue` or `complete`
payload. Planner flags configure only the planner; executor flags configure
implementation, CI, and merge; reviewer flags configure review.

**Prerequisites and side effects.** The request must run from a Git
repository with the selected base branch available. Each wave can create
`.claude/worktrees/<task>` and a matching task branch, and the merger performs
an actual local Git merge on the base branch. Up to eight task slots can run
in parallel, while planning and merging each have a single slot. Review,
CI, and merge rejection/continue outcomes route a task back for repair; the
implementation, review, and cycle loop breakers keep retries finite. A child
failure or exhausted cycle bound fails the project. This Factory completes
local repository work; it does not itself open a remote pull request.

**Expected output shape.** A successful invocation returns the completed
project result after all required task branches are verified and merged. The
event stream exposes planning waves, worktree setup, implementation/review/CI
dispatches, merges, and any replan. A failed provider, task, or finite bound
returns a terminal failure rather than claiming completion. Model-generated
task descriptions and the final prose are not byte-stable.

**Worked invocation.** This bounded terminal smoke was executed against the
current binary in an isolated Git repository with isolated `HOME` and
`USERPROFILE` values. A deterministic local `codex` executable was first on
`PATH` and returned a protocol-valid planner response; because that response
marked the bounded request complete, no implementation, review, or merge
provider call was needed:

```bash
you run --named @you/full-flow --planner-provider codex --executor-provider codex --reviewer-provider codex --no-record --quiet --base-branch main --max-cycles 1 --max-tasks-per-cycle 1 --to "Create a one-line README note and verify it."
```

The command exited 0 and returned:

```text
Create a one-line README note and verify it.
```

This smoke uses a planner response that says the bounded request is already
complete, so it does not create an implementation task. For the complete
orchestration path, the current repository functional test
uses protocol-valid planner, executor, reviewer, CI, and merger responses and
asserts two parallel task worktrees, local merges, a completion replan, and
the bounded failure paths. Do not substitute the empty generic mock for that
coverage: it can accept a planner dispatch without authoring the required
batch protocol.

**Additional bound check.** The public guard also rejects a value above the
`max-cycles` limit before provider work starts:

```bash
you run --named @you/full-flow --no-record --quiet --max-cycles 9 --max-tasks-per-cycle 1 --to "Create a one-line README note and verify it."
```

```text
{"code":"INVOCATION_ARGUMENT_STRING_VALIDATION_MISMATCH","family":"BAD_REQUEST","message":"parameter \"maxCycles\" value \"9\" is not one of the declared choices (factory \"@you/full-flow\")"}
```

## Detailed single bounded-call entry

### `@you/subagent`

**Purpose and suitable use.** Use `@you/subagent` for one bounded, read-only
worker task that should inspect the submitted request and return a
self-contained answer in one pass. It is useful for a focused summary,
explanation, or repository inspection when one worker is enough. It does not
recursively orchestrate other Factories, write to the selected workspace, or
represent an ongoing conversational agent session; use `goal`, `review`, or a
planning Factory when the work needs iteration, implementation, or orchestration.

**Invocation signature.** The live signature is:

```text
you run --named @you/subagent <to> [--worker-model <value>] [--worker-provider <value>] [--worker-reasoning-effort <value>]
```

`<to>` is required and accepts positional, `--to`, or stdin input. The
optional `--worker-provider` and `--worker-model` flags select the one
subagent worker's provider and model; `--worker-reasoning-effort` is an
optional reasoning setting for that worker. Run-level flags such as `--json`,
`--output`, `--record`, and `--no-record` remain available. `--quiet` cannot be
combined with `--json` or `--output`.

**Worker roles and provider/model overrides.** The Factory exposes exactly one
role, `subagent-worker`, an agent worker with a bounded in-process `READ_ONLY`
tool policy. That policy is not a provider-wide guarantee about shell,
filesystem, or network behavior. The three `worker-*` flags apply to that role
and there are no separate planner,
reviewer, merger, or child-agent overrides. Omit provider or model values to
use operator defaults. Omit reasoning effort to preserve the selected
provider's default reasoning setting. These are the only Factory-defined
provider, model, and reasoning inputs shown by the live help output.

**Prerequisites and side effects.** A live run needs a configured provider and
model route for `subagent-worker`; the worker can inspect the request and its
read-only workspace context but is not an implementation path for workspace
changes. One invocation creates runtime session, Work, dispatch, and provider
session activity, and normal recording/artifact behavior applies unless a
run-level option such as `--no-record` is supplied. The worker's answer is
bounded to one pass; a provider or child failure is terminal and does not
produce a success-shaped primary result. The runtime session and its metadata
are not an ongoing agent conversation for the caller.

**Expected output shape.** The default output is the worker's human-readable
primary answer. With `--json --output primary`, the normal invocation envelope
contains one text block in `primaryResult`, plus runtime metadata such as
`status`, `requestId`, and `traceId`; those metadata fields are not part of the
worker's answer. Generated answer prose is not byte-stable. A deterministic
mock-worker smoke can prove the binding and primary-result path, but it does
not represent the quality or content of a configured provider response.

**Worked invocation.** This exact structured smoke was executed against the
current binary with isolated operator state and a built-in accepting mock
worker. The provider/model/reasoning flags exercise the live optional bindings
without starting a provider process:

```bash
you run --named @you/subagent --with-mock-workers --no-record --json --output primary --worker-provider CODEX --worker-model gpt-5 --worker-reasoning-effort medium --to "Summarize the release checklist in three bullets."
```

**Observed output evidence.** The exact command completed with this result;
the request and trace identifiers are opaque values generated for that run:

```text
{"primaryResult":[{"text":"mock worker accepted","type":"text"}],"requestId":"request-4d99f48a-8bc2-41a9-91d8-75db575c456e","status":"COMPLETED","traceId":"trace-request-4d99f48a-8bc2-41a9-91d8-75db575c456e"}
```

The existing behavioral assertion
`TestPackagedSubagentReturnsChildResult` additionally proves through the
public CLI that the child's normalized primary result is returned rather than
the submitted request text, and that a hermetic named invocation succeeds
without starting an HTTP listener. Remove `--with-mock-workers` for a real
provider-backed answer after configuring the route described above.

## Detailed parallel investigation and selection entries

These four Factories all fan work out before returning one customer-facing
answer, but their selection policies differ. `deep-research` gives a lead
researcher bounded specialist evidence, `spawn` merges an exact task count,
`quorum` reconciles two independent Graph branches, and `tournament` keeps one
winner from a bounded 1v1 bracket. The JavaScript entries use
`defaultPolicy.allowedPermissions` to select whether child providers launch
with `DEFAULT` or `SKIP_PERMISSIONS`; that allowlist is not a provider-wide
filesystem, shell, or network isolation promise. Quorum uses canonical Graph
Work and requires both branch Works to complete before its merge worker can
run.

### `@you/deep-research`

**Purpose and suitable use.** Use `@you/deep-research` for a research question
where a lead answer benefits from separate technical and practical/trade-off
investigations. It is appropriate for bounded evidence gathering and
synthesis, not for an ongoing research session, network-connected browsing,
or a write-oriented implementation task. Short topics can stay lead-only;
longer topics can receive up to two specialist investigations.

**Invocation signature.** The live signature from an isolated current-binary
help run is:

```text
you run --named @you/deep-research <to> [--max-subagents <value>] [--research-model <value>] [--research-model-provider <value>] [--reasoning-effort <value>] [--research-depth <value>]
```

`<to>` is required and accepts positional, `--to`, or stdin input.
`--research-depth` is 1–3 and defaults to 2. `--max-subagents` is 0–2 and
defaults to 2. `--reasoning-effort` currently accepts `medium` and defaults to
that value. The research model and provider flags are optional.

**Worker roles and provider/model overrides.** A long topic can dispatch
`research-specialist-technical` and `research-specialist-tradeoffs` in
parallel, followed by `lead-research-synthesis`. The
`--research-model-provider` and `--research-model` values are passed to the
specialist and lead research calls; `--reasoning-effort` is passed to each
call as well. Omitted values use operator defaults. Setting
`--max-subagents 0`, or submitting a short topic, leaves only the lead
synthesis call.

**Prerequisites and side effects.** A live run needs a configured provider and
model. Its `defaultPolicy.allowedPermissions` authorizes `SKIP_PERMISSIONS` for
the provider-launched child calls. The workflow is bounded to a maximum of five
agent calls and at most two concurrent calls; it does not grant the JavaScript
workflow direct filesystem, shell, or network access. This Factory does not
create a worktree or promise workspace changes. The run still creates
Factory Session/provider activity and normally records events and artifacts;
`--no-record` suppresses the normal recording side effect. A provider failure
after the bounded specialist/lead work fails the invocation.

**Expected output shape.** The primary result is a JSON object containing the
submitted `topic`, `role: "lead-researcher"`, the selected `researchDepth` and
`maxSubagents`, an `execution` selection, and `synthesis`. `synthesis` contains
the lead result plus specialist status records; specialist prose is
intermediate evidence, not a second set of primary answers. Use `--json` when
consuming this structured primary result because default human output cannot
render the object as plain text. Model prose and status diagnostics are not
byte-stable.

**Worked invocation.** This exact structured smoke run used the current
binary with the deterministic mock runner:

```bash
you run --named @you/deep-research --with-mock-workers --no-record --json --max-subagents 2 --research-model-provider CODEX --research-model gpt-5 --research-depth 2 --reasoning-effort medium --to "Compare event sourcing and state machines for workflow orchestration"
```

**Observed output evidence.** The final `invocation_result` from that exact
command was `COMPLETED` and included this primary-result shape (the mock's
prompt-derived lead text is intentionally not reproduced as a stable golden):

```text
primaryResult[0].type = JSON
primaryResult[0].json.topic = "Compare event sourcing and state machines for workflow orchestration"
primaryResult[0].json.role = "lead-researcher"
primaryResult[0].json.researchDepth = 2
primaryResult[0].json.maxSubagents = 2
primaryResult[0].json.execution = { modelProvider: "CODEX", model: "gpt-5", reasoningEffort: "medium" }
primaryResult[0].json.synthesis.specialistStatuses = [technical: COMPLETED, tradeoffs: COMPLETED]
response.status = "COMPLETED"
```

### `@you/spawn`

**Purpose and suitable use.** Use `@you/spawn` when one bounded request can
be decomposed into an exact number of independent tasks and a final merger
should reconcile every result. It is useful for bounded parallel investigation
or comparison. It does not create task worktrees or expose each child as a
separate primary answer; use `plan-parallel` or `full-flow` when implementation
ownership, Git isolation, or review/merge behavior is required.

**Invocation signature.** The live signature is:

```text
you run --named @you/spawn <to> [--count <number>] [--worker-provider <value>] [--worker-model <value>] [--model-provider <value>]
```

`<to>` is required and accepts positional, `--to`, or stdin input.
`--count` is optional, defaults to 3, and accepts 1–14. The upper bound is
also the workflow budget: the Factory needs `count + 2` agent calls for one
planner, all child tasks, and one merger.

**Worker roles and provider/model overrides.** `spawn-planner` must return
exactly `count` distinct, non-empty task strings. The resulting
`spawn-task-1` through `spawn-task-N` workers run concurrently, followed by
`spawn-merger`. `--worker-provider` selects the executor provider used by the
planner, tasks, and merger; `--model-provider` selects their model provider;
`--worker-model` selects their model. Omitted values use operator defaults for
all three stages. These flags are intentionally shared: the live signature
does not expose separate planner, child, or merger overrides.

**Prerequisites and side effects.** A live run needs usable planner, task, and
merger provider/model routes. Its `defaultPolicy.allowedPermissions` authorizes
`SKIP_PERMISSIONS` for the provider-launched child calls. The workflow is
bounded to up to 16 agent calls and at most eight concurrent child calls; it
does not grant the JavaScript workflow direct filesystem, shell, or network
access. All children run in the same repository context rather than authored
worktrees, so this is not a safe substitute for an implementation
workflow with branch isolation. The session can still create provider-session,
recording, and artifact activity; `--no-record` suppresses normal recording.
If the planner does not return the exact JSON array contract, a child fails, or
the merger returns empty text, the parent fails without a merged primary result.

**Expected output shape.** A successful invocation returns one human-readable
text result from `spawn-merger`. The planner array and each ordered child
finding are intermediate workflow data; they are not printed as separate
primary results. Structured `--json` output wraps that one merged text in the
normal invocation result. Generated task strings, findings, and synthesis are
not deterministic prose.

**Worked invocation.** This successful run was executed against the current
binary with isolated `HOME` and `USERPROFILE` values and a deterministic local
`codex` executable first on `PATH`. The executable returned a valid planner
array, child answers, and merger response:

```bash
you run --named @you/spawn --model-provider codex --no-record --quiet --count 2 --to "Research the two strongest options for a release checklist."
```

The command exited 0 and returned:

```text
merged travel answer
```

The provider-command functional assertion
`TestPackagedSpawnPlansExactCountRunsChildrenAndMergesThroughCodexCommandRunner`
supplies a two-task planner array, two child results, and a merger result, then
asserts that the primary text is exactly `merged travel answer`.

**Additional bound check.** The live schema rejects a count above the Factory's
1–14 limit before provider work starts:

```bash
you run --named @you/spawn --no-record --quiet --count 15 --to "Research the two strongest options for a release checklist."
```

```text
{"code":"RUN_INVOCATION_FAILED","family":"INTERNAL_SERVER_ERROR","message":"workflow args do not satisfy argsSchema: ... at '/count': maximum: got 15, want 14"}
```

Do not use the empty `--with-mock-workers` configuration as success evidence
for this Factory: its synthetic text is not the planner's required JSON array.

### `@you/quorum`

**Purpose and suitable use.** Use `@you/quorum` when two independent answers
should challenge one another before a final merger reconciles their evidence.
It is a good fit for risk review, competing release plans, and decisions where
preserving disagreement matters. It always creates exactly two branches; use
`spawn` when the task count should be caller-controlled or `tournament` when a
judge should eliminate candidates pair by pair.

**Invocation signature.** The live signature is:

```text
you run --named @you/quorum <to> [--branch-model <value>] [--branch-provider <value>] [--merge-model <value>] [--merge-provider <value>]
```

`<to>` is required and accepts positional, `--to`, or stdin input. The branch
flags also have aliases `--bm` and `--bp`; the merge flags also have aliases
`--mm` and `--mp`. All four override values are optional and fall back to
operator defaults when omitted.

**Worker roles and provider/model overrides.** `quorum-branch-a` and
`quorum-branch-b` run independently with the shared branch provider/model
selection. `quorum-merge` receives the original request plus both completed
branch payloads and uses the independent merge provider/model selection.
`--branch-provider`/`--branch-model` do not configure the merger, and
`--merge-provider`/`--merge-model` do not configure either branch.

**Prerequisites and side effects.** A live run needs configured routes for two
branch workers and one merger. The Graph workflow creates the original task,
two branch Works, and a merge Work; the merge is enabled only after both branch
Works reach `complete`. It runs in the selected workspace without a packaged
worktree isolation step, so provider workers may inspect or change that
workspace according to their worker capabilities. Normal runs create session,
recording, Work, dispatch, and provider-session activity; use `--no-record` to
avoid the normal recording side effect. A branch or merge failure routes to a
failed terminal path instead of returning a partial consensus.

**Expected output shape.** The explicit invocation return is the single text
result from `quorum-merge`. Branch A and branch B proposals, their rationale,
and the Graph Work/dispatch events are intermediate evidence. Human-readable
default output is the merger text; structured output contains one primary text
part and the normal session status. The merger should preserve a consequential
disagreement rather than treating two matching answers as proof, and its model
prose is not byte-stable.

**Worked invocation.** The exact current-binary smoke used both branch and
merge overrides and deterministic mock workers:

```bash
you run --named @you/quorum --with-mock-workers --no-record --quiet --branch-provider CODEX --branch-model gpt-5 --merge-provider CODEX --merge-model gpt-5 --to "Compare the two proposed release plans."
```

**Observed output evidence.** That exact command completed and returned:

```text
mock worker accepted
```

The generic mock proves the Graph branch/merge path and primary text
selection; it does not claim that the mock produced meaningful independent
assessments. Use a configured provider for customer research.

### `@you/tournament`

**Purpose and suitable use.** Use `@you/tournament` when multiple independent
candidate answers should compete in judged 1v1 matches and only the champion
should be returned. It is appropriate for bounded competitive drafting or
selection. Use `quorum` when both independent assessments must inform one
reconciler, and use `spawn` when every branch result should contribute to the
final synthesis instead of being eliminated.

**Invocation signature.** The live signature is:

```text
you run --named @you/tournament <to> [--competitor-provider <value>] [--judge-provider <value>] [--judge-model <value>] [--judge-model-provider <value>] [--competitor-model <value>] [--competitor-model-provider <value>] [--rounds <number>]
```

`<to>` is required and accepts positional, `--to`, or stdin input.
`--rounds` is optional, defaults to 2, and accepts 1–3. A round value of `R`
generates `2^R` competitors and requires `2^(R+1)-1` provider calls, so the
three allowed values require 3, 7, or 15 calls respectively.

**Worker roles and provider/model overrides.** The JavaScript workflow creates
`tournament-competitor-1` through `tournament-competitor-N` for independent
candidate generation, then `tournament-judge-rN-mM` workers for each match.
`--competitor-provider`, `--competitor-model-provider`, and
`--competitor-model` configure candidate workers. `--judge-provider`,
`--judge-model-provider`, and `--judge-model` configure judges. If a judge
override is omitted, the workflow first falls back to the corresponding
competitor selection and then to operator defaults. Every judge must return
JSON selecting exactly `A` or `B` and a non-empty rationale.

**Prerequisites and side effects.** A live run needs usable competitor and
judge routes. Its `defaultPolicy.allowedPermissions` authorizes
`SKIP_PERMISSIONS` for the provider-launched competitor and judge calls. The
workflow is bounded to a maximum of 15 agent calls and at most eight concurrent
calls; it does not grant the JavaScript workflow direct filesystem, shell, or
network access. Candidate generation and matches are bounded by `rounds`; the
Factory does not create worktrees or a persistent candidate store. Session,
provider-session, recording, and artifact activity still occurs unless
`--no-record` is supplied. Invalid judge JSON, a failed candidate or judge, an
empty champion, or an out-of-range round is terminal failure.

**Expected output shape.** The primary result is one text string consisting of
the champion's candidate answer followed by a `Tournament decision trail:`
with the judge rationale for the matches that champion won. Losing candidates
and non-winning rationales are intermediate workflow data, not additional
primary results. The judge's JSON is an internal protocol and is not the
customer-facing output. Candidate and judge prose is not byte-stable.

**Worked invocation.** This successful one-round bracket was executed against
the current binary with isolated `HOME` and `USERPROFILE` values and a
deterministic local `codex` executable first on `PATH`. It returned valid
candidate answers and judge JSON:

```bash
you run --named @you/tournament --competitor-model-provider codex --judge-model-provider codex --no-record --quiet --rounds 1 --to "Propose a launch strategy"
```

The command exited 0 and returned:

```text
merged travel answer

Tournament decision trail:
Round 1, match 1: more complete
```

The provider-command functional assertion
`TestPackagedTournamentRunsOneOnOneBracketThroughCodexCommandRunner` supplies
the same two-candidate bracket and valid judge decision, then asserts a single
champion text followed by `Tournament decision trail:` and the judge rationale.

**Additional bound check.** The live schema rejects a round value above the
1–3 limit before provider work starts:

```bash
you run --named @you/tournament --no-record --quiet --rounds 4 --to "Propose a launch strategy"
```

```text
{"code":"RUN_INVOCATION_FAILED","family":"INTERNAL_SERVER_ERROR","message":"workflow args do not satisfy argsSchema: ... at '/rounds': maximum: got 4, want 3"}
```

Do not use the empty `--with-mock-workers` configuration as success evidence:
its generic text is not valid judge JSON.

## Detailed local media entry

### `@you/tts`

**Purpose and suitable use.** Use `@you/tts` to turn one submitted text
utterance into one local audio artifact. It is suitable for short spoken
announcements, release summaries, and other text that should be read in its
submitted order. It is a single inference run, not an agent loop, provider
fan-out, or translation step; the packaged worker is instructed to preserve
the bound wording and return one audio result.

**Invocation signature.** The live signature is:

```text
you run --named @you/tts <to>
```

The required `<to>` value binds the `text` input and can be supplied as the
positional argument shown above, with `--to <value>`, or through stdin. The
current help output identifies no Factory-defined voice, format,
provider, or model flags. Run-level output and recording flags remain
available, but they do not select a different TTS backend.

**Worker roles and provider/model overrides.** The Factory has one inference
worker role, `tts-executor`, selecting the built-in `tts` model backed by
VibeVoice-7B through the Models-owned generic `InvokeModel` path and its `TTS`
operation. The operation accepts one required `TEXT` slot and produces one
`AUDIO` slot. The packaged Factory does not define a separate TTS codec or an
OmniVoice-specific execution route; Models owns the generic managed backend
process. There is no general inference-provider or model override in the live
`@you/tts` signature: `modelProvider: CODEX` in the packaged worker metadata
preserves the existing local worker contract; it is not a supported
`--provider` or `--model` option.

**Prerequisites and side effects.** The built-in `tts` model must be installed
and report `READY` before the run can synthesize audio. Use [`you docs
models`](./models.md) for the canonical list, inspect, pull, and
runtime-readiness workflow; in particular, pull the model when inspect reports
`MISSING` and wait through `LOADING` until it is ready. The `localai-vibevoice`
backend may load on demand, so the first run can take longer than later runs.
The invocation creates a Factory Session and dispatches one local model Work;
normal recording and runtime artifact activity applies unless a run-level
option such as `--no-record` is supplied. It writes audio to a
runtime-generated artifact and may leave the managed model cache and
session/runtime diagnostics in the configured operator state. Model-not-ready
or synthesis failures are terminal failures and do not imply that an audio
artifact was produced.

**Expected output shape.** With `--output primary`, the caller receives one
JSON metadata object rather than raw audio bytes on stdout. The current
primary-result contract includes an opaque `artifactPath`, the reported
`mediaType` (the local runtime currently reports `audio/wav`), the
`tts/LOCALAI-VIBEVOICE` `backend`, and a `traceId`. Treat
`artifactPath` as runtime-generated: do not construct or promise a fixed
directory or filename, and inspect the returned path or artifact record to
locate the audio. The generated audio content is not byte-stable. A failed
run returns a TTS generation/model-readiness error without success-shaped
metadata.

**Worked invocation.** After the built-in `tts` model reports `READY`, run this
exact command from a blank directory:

```bash
you run --named @you/tts --no-record --output primary --to "The release is ready."
```

The focused packaged-factory tests verify the customer-facing input and
artifact metadata contract with injected effects. Real VibeVoice asset
download and backend conformance are separate model-asset verification scope;
this entry documents the Factory's built-in model selection and generic
invocation path only.

## Detailed media-review entries

`@you/agy-clip-qa` and `@you/agy-cold-watch` are first-party graph Factories
for production media review. Both use the existing `ANTIGRAVITY` provider with
the pinned `gemini-3.6-flash-high` model and an `8m` provider timeout. The
current help output exposes no provider, model, effort, or timeout override for
either role; those are safe role defaults, not hidden invocation inputs.

The provider receives the existing workspace as AGY's `--add-dir` path and the
media path exactly as supplied. The Factory wrapper never decodes, uploads,
copies, probes, extracts frames from, or extracts audio from media. Run from a
workspace that contains the file: `.\rendered\clip.mp4` is relative to that
workspace, and an absolute path such as
`C:\production\job-42\rendered\clip.mp4` is valid only when it resolves inside
the same directory exposed to AGY. A missing, unreadable, or inaccessible path
is an execution failure; it is never a successful review verdict, even when
AGY exits zero and reports provider status `SUCCESS`.

### `@you/agy-clip-qa`

**Purpose and suitable use.** Use this gate immediately after a rendered clip
is created and before it is accepted into a cut. It compares the complete
clip—including audio—with exactly one shot specification. The clip path and
shot specification are its only creative inputs; it does not receive a brief,
upstream status, filename-based intent, or prior verdict.

**Invocation signature.** The live signature is:

```text
you run --named @you/agy-clip-qa <clip-path> --shot-specification <value>
```

Both creative inputs are required. `<clip-path>` can be positional as shown or
bound with `--clip-path`; `--shot-specification` can be supplied as a named
value or read from stdin. There are exactly two creative parameters.

**Worker roles and provider/model overrides.** The one worker role is
`agy-clip-qa-gate`. It uses `ANTIGRAVITY`, `gemini-3.6-flash-high`, and an `8m`
timeout. No role-specific provider or model flags are exposed; do not invent
`--provider`, `--model`, or timeout flags for this Factory.

**Expected output shape.** The primary result is one JSON object with every
field required:

- `action_completed`: boolean.
- `spec_deviations`: string array; include each material mismatch.
- `temporal_artifacts`: string array; include each reroll-worthy temporal or
  transient defect.
- `audio_content`: one of `speech`, `music`, `noise`, `silence`, or `mixed`.
- `unexpected_speech`: boolean.
- `verdict`: exactly `pass` or `reroll`.
- `confidence`: a number from `0` through `1`.

`pass` means the action completes, both reason arrays are empty, and disallowed
speech is absent. `reroll` means the clip was successfully inspected but is
unacceptable; the arrays must record the reasons. A provider/process failure,
malformed or schema-invalid response, or media-access refusal fails Work and
returns no production verdict.

**Worked invocation.** This PowerShell-safe command uses only flags in the
live help output:

```powershell
$clipPath = (Resolve-Path '.\rendered\SH080.mp4').Path
$shotSpecification = 'A silver-haired woman points at a bright star; no speech is audible.'
$qaJson = you run --named @you/agy-clip-qa --clip-path $clipPath --shot-specification $shotSpecification --output primary --no-record
if ($LASTEXITCODE -ne 0) { throw "clip-QA execution failed with exit code $LASTEXITCODE" }
$qa = $qaJson | ConvertFrom-Json
switch ($qa.verdict) {
  'pass'   { Write-Output 'CLIP_PASS'; break }
  'reroll' { Write-Output 'CLIP_REROLL'; break }
  default  { throw "clip-QA returned an invalid verdict: $($qa.verdict)" }
}
```

The command's `pass` and `reroll` branches are successful inspection outcomes;
the non-zero process branch is a distinct failure route. The offline
behavioral test replays `agy-trace-clipqa-schema.stream.jsonl` for the real
structured pass and also proves schema-invalid, provider-failure, and
missing-file paths fail before a verdict is accepted.

### `@you/agy-cold-watch`

**Purpose and suitable use.** Use this reviewer after mechanical checks have
passed and the completed cut has been assembled. It watches the entire cut
from first principles so an operator can see what the artifact actually
communicates, including motion/transient defects and soundtrack content. It
must not receive or seek a creative brief, shot specification, expected beat,
upstream status, filename intent, or prior verdict.

**Invocation signature.** The live signature is:

```text
you run --named @you/agy-cold-watch <cut-path>
```

`<cut-path>` is the one required creative input. The help output exposes it as
positional input and as `--cut-path`; no other creative or role-specific flags
are available.

**Worker roles and provider/model overrides.** The one worker role is
`agy-cold-watch-reviewer`. It uses `ANTIGRAVITY`, `gemini-3.6-flash-high`, and
an `8m` timeout. No provider, model, effort, or timeout override is exposed by
the invocation signature.

**Expected output shape.** The primary result is one Markdown observation
report with all of these sections present, including explicit empty sections:

1. Chronological events, with timestamps where observable.
2. Timestamped temporal or transient defects, or `None observed`.
3. Audio content and timestamped audio defects, or `None observed`.
4. Observed speech, including an intelligible transcription, or `None
   observed`.
5. An overall recommendation of exactly `pass` or `reroll` based only on the
   artifact.

A file-access refusal is an execution failure, not a recommendation. The
accepted report must contain an explicit speech audit so an exit-zero AGY
refusal cannot become a false success.

**Worked invocation.** This command preserves an absolute path while keeping
the PowerShell invocation safe for paths with spaces:

```powershell
$completedCut = (Resolve-Path '.\assembled\completed-cut.mp4').Path
$report = you run --named @you/agy-cold-watch --cut-path $completedCut --output primary --no-record
if ($LASTEXITCODE -ne 0) { throw "cold-watch execution failed with exit code $LASTEXITCODE" }
$recommendationMatch = [regex]::Match($report, '(?im)^[ \t]*##[ \t]+Overall recommendation[ \t]*\r?\n(?:[ \t]*\r?\n)*[ \t]*Recommendation:[ \t]*(?:\*\*)?(pass|reroll)(?:\*\*)?[ \t]*\r?$')
if (-not $recommendationMatch.Success) { throw 'cold-watch report has no valid overall recommendation' }
switch ($recommendationMatch.Groups[1].Value.ToLowerInvariant()) {
  'pass'   { Write-Output 'COLD_WATCH_PASS'; break }
  'reroll' { Write-Output 'COLD_WATCH_REROLL'; break }
}
```

The report parser must treat a non-zero execution result or a missing
recommendation as `failed`, never as `reroll`. The offline behavioral test
replays both the real video/audio observations and the missing-file refusal,
including the case where AGY reports provider `SUCCESS` while declining the
file.

See the repository-owned [AGY production review composition example](../examples/agy-production-review.md)
for the complete clip-creation, mechanical-check, assembly, and routing
sequence. The fully offline end-to-end command is:

```bash
go test ./tests/functional/providers/agy -run '^TestAgyProductionReviewRolesThroughRootBuildProcess$' -count=1
```

This test constructs through `root.BuildProcess`, executes through
`Process.Execute`, replaces only the `ProviderCommandRunner` edge, and replays
recorded AGY traces. The existing operator-gated B1 live smoke remains the
only live AGY check:

```powershell
$env:YOU_AGY_LIVE_SMOKE = '1'
go test ./tests/functional/providers/agy/... -run '^TestAgyLiveSmoke$' -count=1
```

Do not enable that live smoke in ordinary CI.

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
you run --named @you/tts --no-record --output primary --to "The release is ready."
```

Positional text is accepted wherever the Factory's `invocationSignature` binds
`POSITIONAL` for the primary request. Named `--to` is the portable form shown
above. Pipe stdin instead of supplying both positional text and stdin.

## Role-specific provider and model flags

Many packaged Factories expose per-role overrides. Common examples:

| Factory | Role flags |
|---------|------------|
| `@you/factory-builder` | `--builder-provider`, `--builder-model` (shared by router, help, and builder roles) |
| `@you/goal` | `--executor-provider`, `--executor-model` |
| `@you/plan-execute` | `--planner-provider`, `--planner-model`, `--executor-provider`, `--executor-model` |
| `@you/plan-parallel` | planner / executor / merge provider and model flags |
| `@you/full-flow` | planner / executor (implementation, CI, merge) / reviewer provider and model flags |
| `@you/review` | `--writer-provider`, `--writer-model`, `--reviewer-provider`, `--reviewer-model` |
| `@you/classify` | classifier / small / medium / large provider and model flags |
| `@you/quorum` | `--branch-provider`, `--branch-model`, `--merge-provider`, `--merge-model` |
| `@you/fusion` | `--first-provider`, `--second-provider` (and matching model flags) |
| `@you/deep-research` | `--research-model-provider`, `--research-model`, `--reasoning-effort`, `--max-subagents`, `--research-depth` |
| `@you/tournament` | competitor and judge provider / model flags |
| `@you/spawn` | `--worker-provider`, `--worker-model`, `--model-provider`, plus `--count` |

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

1. `you factory list` — see the seventeen catalog entries.
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
