# Packaged Goal (`@you/goal`)

Use this guide when you want the first-party packaged goal factory: invoke it by
name from the terminal, read the primary result on stdout, and avoid browser or
dashboard interaction for the standard batch success path.

`you docs sessions` and `you docs config` own the shared invocation input and
return-policy contract. This guide focuses on the `@you/goal` packaged factory
workflow.

The default `@you/goal` factory uses one `DEFAULT` handling work type and one
`MODEL_WORKSTATION` that executes the submitted goal text.

## Quick start

Run a goal from positional text:

```bash
you run --named @you/goal "Ship the login fix by Friday"
```

Pipe stdin when no positional text is present:

```bash
echo "Ship the login fix by Friday" | you run --named @you/goal
```

Supplying both positional text and piped stdin for the same invocation is
rejected with `INVOCATION_INPUT_SOURCE_CONFLICT` before runtime work starts.

## Batch success without browser or dashboard interaction

Normal `you run --named @you/goal` uses the default batch run mode (no
`--continuously`). For the standard success path you do not need to open the
operator dashboard or interact with a browser session:

- The CLI submits the goal through the real named packaged-factory path.
- The run completes when the submitted work reaches terminal success or the
  factory goes idle.
- The process exits after that terminal completion or idle instead of staying
  open for later operator submissions.
- Successful stdout carries the configured `primaryResult` from the shared
  invocation-return contract.

Use `--quiet` for scripted or CI-oriented runs when you want to suppress
dashboard startup output. That flag affects operator chatter only; it does not
change invocation input resolution or primary-result selection.

This guide documents the supported headless **operator-interaction** claim for
the normal batch success path. It does **not** promise that batch invocation
avoids binding a localhost API listener. Listener behavior belongs to the shared
`you run` service startup contract and may differ across run modes; see
`you docs sessions` for operator-oriented modes that keep a service alive for
later submissions.

## Default invocation result

Successful `@you/goal` invocations print the packaged factory's primary result
on stdout using the existing invocation-result contract. The default factory omits
an explicit `invocationReturn` override, so the runtime uses
`SUBMITTED_WORK_TERMINAL`: it follows the originally submitted work item until
terminal output, then returns that work content as `primaryResult`.

On the CLI, that successful `primaryResult` is written directly to stdout. You
do not need to reconstruct the answer from logs, event history, or dashboard
state.

The equivalent API path is `POST /factory-sessions/{session_id}/invocations`
with the same text input and return-policy semantics. Transport code adapts its
carrier; it does not invent separate primary-output rules.

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
files, and any supporting assets needed for the default goal runtime.

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

If the materialized factory becomes invalid or incomplete, invocation fails with a
clear packaged-factory load error instead of silently falling back to embedded
behavior.

## Related topics

- `you docs authoring-factories` — named-factory resolution and factory layout
- `you docs config` — invocation input sources and `invocationReturn` policy
- `you docs sessions` — session-scoped invocation API, dashboard URL, and run modes
- `you docs mock-workers` — deterministic local testing with `--with-mock-workers`
