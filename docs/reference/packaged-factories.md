# Packaged Factories

YOU ships **fourteen** first-party Factories under the `@you/` namespace. They
are authored in `packages/packaged-factories/factories/`, published through the
catalog manifest at `packages/packaged-factories/generated/manifest.json`, and
materialized lazily under `~/.you-agent-factory/factories` (Windows:
`%USERPROFILE%\.you-agent-factory\factories`) on first use. Materialized copies
remain editable.

List them at runtime with `you factory list`. Inspect one Factory's live CLI
boundary with `you run --named <name> --help` (shorthand: `you run -a <name>
--help`).

## Catalog

| Factory | Orchestrator | Description |
|---------|--------------|-------------|
| `@you/classify` | Graph | Classifies a request by complexity and routes it to the configured small, medium, or large model lane. |
| `@you/deep-research` | JavaScript | Breaks a research question into bounded specialist investigations and synthesizes their findings. |
| `@you/full-flow` | Graph | Plans parallel implementation waves in isolated worktrees, merges completed tasks, and replans until the project is complete. |
| `@you/fusion` | Graph | Produces an initial answer with one worker and refines it with a second worker. |
| `@you/goal` | Graph | Repeatedly works a goal until the executor reports completion or the Factory reaches a failure bound. |
| `@you/loop` | Graph | Runs the requested task at a duration interval such as `1h` for the lifetime of the Factory Session. |
| `@you/plan-execute` | Graph | Writes a Markdown and JSON PRD for a project request, then executes that plan in the current workspace. |
| `@you/plan-parallel` | Graph | Plans a dependency graph of Work, executes ready tasks concurrently, and merges the completed results. |
| `@you/quorum` | Graph | Runs independent assessments in parallel and merges them into one final answer. |
| `@you/review` | Graph | Produces candidate work and repeats independent review until approval or a bounded failure. |
| `@you/spawn` | JavaScript | Plans an exact number of independent tasks, runs them concurrently, and merges their results into one answer. |
| `@you/subagent` | Graph | Runs one bounded read-only subagent and returns its result. |
| `@you/tournament` | JavaScript | Runs candidates through bounded 1v1 matches, uses a judge to advance each winner, and returns the champion result. |
| `@you/tts` | Graph | Converts submitted text to audio with the packaged local text-to-speech model. |

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

1. `you factory list` — see the fourteen catalog entries.
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
