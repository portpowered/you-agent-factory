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
