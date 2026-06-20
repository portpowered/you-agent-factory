# Packaged Goal (`@you/goal`)

Use this guide when you want the first-party packaged goal factory: invoke it by
name, inspect the materialized factory on disk, and customize it like any other
named factory.

`you docs authoring-factories` owns the broader factory-authoring workflow.
`you docs config` and `you docs sessions` own the shared invocation input and
return-policy contract. This guide focuses on the `@you/goal` packaged factory
workflow.

The default `@you/goal` factory is a minimal goal-oriented example: one
`MODEL_WORKER` plus one `MODEL_WORKSTATION` that executes a single bounded task
through the shared model-worker runtime. Use it as a runnable starting point
before authoring a custom factory from scratch.

## Quick start

Run the packaged goal factory by its canonical name:

```bash
you run --named @you/goal
```

Pass one-shot invocation text when you want the factory to handle a single
prompt through the shared `handlingBehavior: ["DEFAULT"]` contract:

```bash
you run --named @you/goal "Ship the login bugfix"
```

Pipe stdin when no positional text is present:

```bash
echo "Ship the login bugfix" | you run --named @you/goal
```

Supplying both positional text and piped stdin for the same invocation is
rejected with `INVOCATION_INPUT_SOURCE_CONFLICT` before runtime work starts.

## What to expect after you run it

`you run --named @you/goal` resolves the named factory, materializes it on first
use if needed, then starts a normal factory session using the on-disk factory
layout.

Customers should expect:

- The CLI selects exactly one named-factory directory and loads the split
  `factory.json`, `workers/`, and `workstations/` layout from that directory.
- The default factory runs one `task` work type from `init` through
  `execute-goal` to `complete`, or to `failed` when the workstation reports a
  failure.
- Later invocations reuse the materialized on-disk copy instead of overwriting
  customer edits with pristine embedded content.

For broader session discovery, status inspection, and work submission after the
factory is running, use `you docs sessions`, `you docs work`, and
`you docs batch-inputs`.

## Where the factory materializes

`you run --named @you/goal` resolves named factories in this order:

1. Project-local `./factory`
2. Global shared root `~/.you-agent-factory/factories`
3. Built-in catalog materialization on first use

On first invocation, `@you/goal` materializes into the global root using the
normal named-factory persist pipeline. Scoped names are URL-encoded on disk:

```text
~/.you-agent-factory/factories/@you%2Fgoal/
```

Inspect the materialized factory:

```bash
you factory list --dir ~/.you-agent-factory/factories
```

The directory contains `factory.json`, split `workers/` and `workstations/`
files, and the default `goal-executor` worker plus `execute-goal` workstation
prompts needed for the packaged workflow.

## Customer edits affect the next run

Packaged factories stay editable after materialization. The CLI reuses the
on-disk directory on later invocations instead of overwriting customer changes
with pristine embedded content.

Edit distinguishing fields such as:

- `workers/goal-executor/AGENTS.md` prompt body
- `workstations/execute-goal/AGENTS.md` workstation prompt
- `factory.json` worker model or resource settings

The next `you run --named @you/goal` invocation loads the edited on-disk factory
immediately. No reinstall, cache clear, or special reload step is required.

If the materialized factory becomes invalid or incomplete, invocation fails
with a clear packaged-factory load error instead of silently falling back to
embedded behavior.

## Related topics

- `you docs authoring-factories` — named-factory resolution, factory layout,
  and when to start from a packaged example instead of authoring from scratch
- `you docs config` — invocation input sources and `invocationReturn` policy
- `you docs sessions` — session-scoped invocation API and runtime discovery
- `you docs workstations` — `MODEL_WORKSTATION` authoring fields
- `you docs workers` — `MODEL_WORKER` authoring fields
