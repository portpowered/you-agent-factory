# Packaged Factory Families Implementation Plan

## Status

Proposed.

## Customer Ask

Ship the packaged Factory families described by `plan-readme.md` as ordinary
Factory graphs with discoverable descriptions, coherent invocation arguments,
and direct behavioral test coverage. In particular:

- `@you/plan-execute` must use the same PRD-to-worktree execution model as the
  checked-in `factory/factory.json`, not a two-prompt approximation.
- `@you/plan-parallel` must let a planner emit canonical Work and Work
  relationships, then let the Factory runtime schedule dependency-ready Work
  concurrently.
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
`@you/classify`. Those three names are not in the packaged catalog. Existing
catalog entries have uneven invocation signatures and several omit
descriptions, so users cannot reliably discover their purpose or role-specific
arguments. `@you/deep-research` is currently a JavaScript Factory even though
the intended packaged-family model is a normal Factory graph.

The public CLI also has one important contract boundary: `you run -a` already
means `you run --named`. Factory-specific inputs and role overrides should come
from the selected Factory's `invocationSignature`; they should not become new
global `you run` flags or separate top-level commands.

## Goals

- Publish all README-named packaged families as normal Factory graphs.
- Make `@you/plan-execute` reproduce the checked-in PRD, worktree, executor,
  review, CI, and merge lifecycle.
- Make `@you/plan-parallel` use canonical generated Work batches,
  `DEPENDS_ON`, parent-child lineage, runtime concurrency, and guarded fan-in.
- Make `@you/classify` route through `CLASSIFIER_WORKSTATION`.
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
- Do not count aliases as additional packaged families. After adding the three
  named missing families, the catalog contains ten Factories; documentation
  must use the exact catalog or define another real family before claiming
  `11+`.
- Do not redesign unrelated Factory Session, Provider Session, or dashboard
  state architecture.

## Product Decisions

### Packaged Factories remain data-defined Factory graphs

All ten packaged families use the canonical Factory definition model:
`workTypes`, `workers`, `workstations`, resources, guards, Work relationships,
and explicit invocation return policy. Packaged prompts and scripts remain
package-owned assets flattened by the existing catalog generator.

`@you/deep-research` will migrate from its JavaScript orchestrator to a Factory
graph. Its lead/planner workstation may emit a bounded specialist Work batch;
specialists run through ordinary Work dispatch and a guarded parent fan-in.

### Invocation syntax

The canonical examples use the existing named-Factory command:

```sh
you run -a @you/plan-execute --to "Implement the requested feature"
you run -a @you/plan-parallel --to "Implement the requested feature"
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
| `@you/plan-execute` | Writes a Markdown and JSON PRD, prepares an isolated worktree, and executes the PRD through implementation, review, CI, and merge. |
| `@you/plan-parallel` | Plans a dependency graph of Work, executes ready tasks concurrently, and merges the completed results. |
| `@you/quorum` | Runs independent assessments in parallel and merges them into one final answer. |
| `@you/review` | Produces candidate work and repeats independent review until approval or a bounded failure. |
| `@you/classify` | Classifies a request by complexity and routes it to the configured small, medium, or large model lane. |
| `@you/subagent` | Runs one bounded read-only subagent and returns its result. |
| `@you/tts` | Converts submitted text to audio with the packaged local text-to-speech model. |

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

## Family Designs

### `@you/plan-execute`: PRD-driven execution

This family follows the checked-in `factory/factory.json` lifecycle rather
than behaving like `@you/fusion`.

#### Observable workflow

1. The invocation creates one named idea/request Work item.
2. The planner reads repository standards and the customer request.
3. The planner writes `tasks/todo/<work-name>.md` and
   `tasks/todo/<work-name>.json` in the target repository.
4. The Markdown PRD describes context, goals, non-goals, technical design,
   vertically sliced user stories, behavioral acceptance criteria, test
   evidence, and the delivery loop.
5. The JSON PRD is a mechanistic implementation manifest containing project,
   branch name, description, context, project acceptance criteria, ordered
   stories, `passes: false`, and empty notes.
6. A packaged workspace setup script creates or reuses an isolated worktree,
   using the PRD/work item name as branch identity.
7. The setup step copies the two planned files into the worktree root as
   `prd.md` and `prd.json` and initializes `progress.txt` when absent.
8. The executor runs in that worktree, reads `prd.json`, `prd.md`, and
   `progress.txt`, and implements one highest-priority failing story per pass.
9. Each executor pass runs story-proportional verification, updates PRD state
   and progress, commits only reviewable product changes, pushes, and updates
   or opens the PR.
10. The reviewer checks the PRD acceptance criteria, diff, test evidence, and
    PR conversation feedback. Rejection routes back to the executor.
11. Guarded loop breakers bound executor and reviewer attempts.
12. The Factory reaches successful terminal Work only after all PRD stories
    pass, required CI is terminal and green, blocking feedback is addressed,
    conflicts are resolved, and the PR is merged.

#### Factory topology

```text
idea:init
  -> plan-prd
  -> idea:to-complete + plan:init

plan:init
  -> setup-workspace
  -> plan:complete + task:init

task:init
  -> process-prd-story (REPEATER)
  -> task:in-review + review:init

task:in-review + review:init
  -> review-prd
  -> task:to-complete + review:complete
  -> rejection returns task:init

idea:to-complete + task:to-complete, matched by Work name
  -> invocation:complete
```

The packaged definition should reuse the checked-in Factory's `SAME_NAME`
join, executor/reviewer visit-count loop breakers, isolated working directory,
and package-owned setup script. Package assets must include planner, executor,
reviewer, and workspace prompts/scripts; the packaged Factory cannot depend on
the repository's `factory/` directory being present.

Invocation parameters:

- `to`
- optional `branch-name`, validated as a safe git branch/worktree identity
- `planner-provider`, `planner-model`
- `executor-provider`, `executor-model`
- `reviewer-provider`, `reviewer-model`

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
- `@you/deep-research`: replace the JavaScript orchestrator with a graph in
  which a lead planner emits zero to the configured maximum specialist Work
  items, specialists run through an ordinary agent workstation, and guarded
  fan-in performs lead synthesis.

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

### PF-FAMILIES-004: Package the PRD planner and workspace handoff

Implement the first half of `@you/plan-execute`: PRD authoring and isolated
workspace preparation.

Acceptance criteria:

- The planner writes both `tasks/todo/<name>.md` and
  `tasks/todo/<name>.json` for the submitted request.
- The Markdown and JSON files contain the same project intent, ordered stories,
  behavioral acceptance criteria, test requirements, and delivery-loop
  completion rule.
- JSON story IDs are stable and sequential, priorities are ordered,
  `passes` starts false, and notes start empty.
- The workspace step creates or safely reuses the requested branch/worktree,
  copies the files to worktree-root `prd.md` and `prd.json`, and initializes
  `progress.txt` without overwriting existing progress.
- Unsafe branch names, missing/malformed PRD JSON, git failures, and copy
  failures route plan Work to a terminal failure with a useful diagnostic.
- Script unit tests use temporary git repositories and do not require network
  or GitHub access.
- A root-built functional test with an injected provider command runner proves
  planner-to-workspace handoff without touching the user's real worktrees.
- Typecheck and tests pass.

### PF-FAMILIES-005: Complete PRD execution, review, and merge lifecycle

Implement the executor/reviewer half of `@you/plan-execute` using the checked-in
Factory's one-story-per-pass and guarded review model.

Acceptance criteria:

- The executor reads `prd.json`, `prd.md`, and `progress.txt` from the prepared
  worktree and selects the highest-priority story with `passes: false`.
- One executor pass changes at most one unfinished story except for narrowly
  documented mergeability work after all stories pass.
- Successful passes run story-proportional tests, record progress, mark the
  story passed, and keep PRD/progress artifacts out of product-review commits.
- Reviewer rejection returns the same task to executor with actionable
  feedback; reviewer approval advances only when project criteria and required
  evidence pass.
- Executor and reviewer visit-count guards produce a terminal failed outcome
  instead of an infinite loop.
- The invocation cannot report success merely because a PR exists or is green;
  success requires all stories passed, required CI complete and passing,
  blocking feedback addressed, conflicts resolved, and the PR merged.
- Functional tests use injected provider/process edges and a fake GitHub/`gh`
  boundary; they cover happy path, one rejected review followed by approval,
  CI failure/continue, merge conflict/continue, retry exhaustion, and final
  merged completion.
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

### PF-FAMILIES-008: Convert `@you/deep-research` to a Factory graph

Replace JavaScript child orchestration with generated specialist Work and
guarded synthesis.

Acceptance criteria:

- A lead planning dispatch emits between zero and the configured maximum number
  of specialist Work items within the generated-Work ceiling.
- Specialist Work runs through ordinary Factory dispatch and can execute
  concurrently.
- Lead synthesis waits for every generated specialist to complete and returns
  the final research answer.
- Specialist failure routes the parent research Work to failure with durable
  dispatch and event evidence.
- Existing topic, depth, specialist cap, provider, model, and reasoning inputs
  retain their documented behavior or receive an explicit migration note.
- No JavaScript orchestrator or JavaScript-only session behavior remains in the
  packaged definition.
- Functional and replay tests replace the prior JavaScript-specific tests and
  prove equivalent customer-visible outcomes.
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

### PF-FAMILIES-010: Publish the complete documented catalog

Regenerate package artifacts and update customer documentation only after the
family behaviors are implemented and tested.

Acceptance criteria:

- The generated manifest contains exactly the supported authored catalog in
  stable lexical order with unique names, slugs, projects, descriptions,
  hashes, and examples.
- Generated JSON and YAML artifacts match their authored Factory sources and
  are not hand-edited.
- Root README and `you docs` examples use the implemented syntax and accurately
  distinguish plan-execute, plan-parallel, fusion, quorum, review, and classify.
- Documentation states the exact packaged count; it does not claim `11+` while
  only ten real families ship.
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

- Plan-execute: real temporary git worktree handoff, PRD file contents,
  one-story iteration, review rejection, CI continuation, merge continuation,
  and merged completion using fake provider and GitHub/process boundaries.
- Plan-parallel: independent readiness, dependency blocking, parallel dispatch
  capacity, guarded fan-in, child failure, invalid/cyclic/oversized batches,
  lineage, replay, and merge result.
- Deep research: generated specialist count, ordinary Factory dispatch,
  guarded synthesis, failure, and migration from JavaScript projections.
- Classify: each label, exactly-one-lane dispatch, malformed labels, and model
  selection.

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
7. PF-FAMILIES-008: deep-research graph conversion.
8. PF-FAMILIES-009: existing graph-family alignment.
9. PF-FAMILIES-010: final catalog and documentation publication.

Each story should be a vertically sliced, independently reviewable PR when
practical. Shared schema and generated-Work behavior land before Factories that
depend on them. Avoid combining unrelated cleanup with a family behavior.

## Project Acceptance Criteria

- All README-named families resolve from the packaged catalog and execute as
  normal Factory graphs.
- `@you/plan-execute` creates Markdown and JSON PRDs, prepares an isolated
  worktree, and executes/reviews the PRD through actual merge completion.
- `@you/plan-parallel` expresses planner output as canonical Work and
  relationships, uses runtime dependency scheduling and concurrency, and
  returns a guarded merged result.
- Every packaged Factory has a useful description visible through CLI, API,
  npm manifest data, and dashboard discovery.
- Every family has direct happy-path, failure-path, invocation, provider/model,
  and primary-result test evidence proportional to its risk.
- Authored definitions, generated JSON/YAML, manifest hashes, OpenAPI-derived
  artifacts, docs, and runtime behavior remain aligned.
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
