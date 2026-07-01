# Packaged Fusion (`@you/fusion`)

Use this guide when you want the first-party packaged fusion factory: inspect
its callable arguments, invoke it by name, and customize the materialized
factory like any other named factory.

`you docs config` owns the shared `invocationSignature` contract. `you docs sessions`
owns the session-scoped invocation API. This guide focuses on the packaged
`@you/fusion` workflow and examples.

The default `@you/fusion` factory runs a two-stage model flow: a first pass
drafts the answer and a second pass refines it. The packaged factory declares a
first-class `invocationSignature`, so CLI help, API args, docs, and runtime
normalization use the same argument schema.

## Quick start

Inspect the factory-defined invocation arguments:

```bash
you run --named @you/fusion --help
```

Run it with positional input and named overrides:

```bash
you run --named @you/fusion "Draft a release summary" \
  --first-provider=CLAUDE \
  --second-provider=CODEX \
  --output fusion-summary.md
```

Pipe stdin when you want `input` routed from stdin instead of a positional
argument:

```bash
printf '%s\n' "Draft a release summary" | \
  you run --named @you/fusion --first-model claude-sonnet-4-20250514 --second-model gpt-5
```

Supplying both positional input and piped stdin for the same invocation is
rejected with `INVOCATION_INPUT_SOURCE_CONFLICT` before runtime work starts.

## Declared arguments

`@you/fusion` publishes these invocation parameters through the shared
`invocationSignature` schema:

| Argument | Description |
|----------|-------------|
| `input` | Required request text. Accepts positional input or stdin. |
| `output` | Optional file-path hint for file-oriented callers. |
| `firstProvider`, `secondProvider` | Optional provider overrides for the first and second fusion passes. |
| `firstModel`, `secondModel` | Optional model overrides for the first and second fusion passes. |
| `firstEffort`, `secondEffort` | Optional reasoning-effort hints for the first and second fusion passes. Accepted values are `low`, `medium`, and `high`; both default to `medium`. |

The packaged signature also declares:

- Short aliases such as `--o`, `--p1`, `--p2`, `--m1`, `--m2`, `--e1`, and `--e2`
- A file-oriented output contract keyed by `output`
- Canonical CLI examples that also appear in factory-aware help

## Output behavior

The packaged signature declares a `FILE` output contract for markdown content.
When you supply `--output`, callers can treat the refined answer as
file-oriented markdown content and reuse the selected path as the authored
output hint.

The shared invocation result still follows the active `invocationReturn` policy.
Use `you docs sessions` for the API response shape and `you docs config` for the
return-policy contract.

## Where the factory materializes

`you run --named @you/fusion` resolves named factories in this order:

1. Project-local `./factory`
2. Global shared root `~/.you-agent-factory/factories`
3. Built-in catalog materialization on first use

On first invocation, `@you/fusion` materializes into the global root using the
normal named-factory persist pipeline. Scoped names are URL-encoded on disk:

```text
~/.you-agent-factory/factories/@you%2Ffusion/
```

Inspect the materialized factory:

```bash
you factory list --dir ~/.you-agent-factory/factories
```

The materialized `factory.json` keeps the same `invocationSignature` used by
CLI help and API normalization, so customer edits to the packaged signature or
runtime fields affect the next run.

## Customer edits affect the next run

Packaged factories stay editable after materialization. The CLI reuses the
on-disk directory on later invocations instead of overwriting customer changes
with pristine embedded content.

Edit distinguishing fields such as:

- `factory.json` invocation examples, defaults, or output hints
- `factory.json` worker `modelProvider` or `model` interpolation targets
- `workers/` and `workstations/` runtime instructions

The next `you run --named @you/fusion` invocation loads the edited on-disk
factory immediately. No reinstall, cache clear, or special reload step is
required.

## Related topics

- `you docs config` — `invocationSignature`, compatibility behavior, and `invocationReturn`
- `you docs sessions` — `InvocationRequest.args` and session invocation responses
- `you docs authoring-factories` — named-factory resolution and packaged-factory workflows
- `you docs workers` — authored worker model settings
- `you docs workstations` — workstation runtime settings
