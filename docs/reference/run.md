---
author: Agent Factory Team
last-modified: 2026-07-13
doc-id: agent-factory/guides/run
---

# Run

Use `you run` to start a Factory or perform a one-shot Factory invocation. This
is the canonical packaged entry point for local, explicit-factory, batch,
continuous, and mock-worker run tasks.

## Choose a run shape

| Task | Run shape |
|------|-----------|
| Run the current `./factory` with initial Work | `you run --work <batch.json>` |
| Start a Factory directory | `you run --dir <factory-dir> --work <batch.json>` |
| Invoke one portable or named Factory | `you run --factory <factory.json> <text>` or `you run --named <name> <text>` |
| Keep a local Factory Session alive while idle | Add `--continuously` |
| Replace live worker dispatch with deterministic outcomes | Add `--with-mock-workers [config.json]` |

Run `you run --help` for the complete live flag boundary.

## Run the current Factory

From a project whose Factory is under `./factory`, submit explicit initial Work:

```bash
you run --work ./docs/examples/startup-work.json
```

This starts the local Factory, submits the batch, and exits after the Factory
becomes idle. To select another Factory directory explicitly:

```bash
you run --dir ./factory --work ./docs/examples/startup-work.json
```

`--work` accepts one `FACTORY_REQUEST_BATCH` JSON file. Use
`you docs batch-inputs` for its fields and watched-input alternatives.

## Invoke one Factory

Use `--factory` for a portable `factory.json` and supply one logical invocation
input as positional text:

```bash
you run --factory ./factory.json "Review the release notes"
```

Non-interactive stdin is the alternative input source:

```bash
printf '%s\n' 'Review the release notes' | you run --factory ./factory.json
```

Do not supply positional text and stdin together. Factories with an
`invocationSignature` may instead define named, file-path, repeated, or
defaulted arguments. Inspect their exact input boundary with
`you run --factory ./factory.json --help`.

Use `--named` for a persisted Factory and still provide its required input:

```bash
you run --named team-review "Review the release notes"
```

Project-local named Factories are resolved before operator-level named
Factories. Use `you run --named team-review --help` to inspect a named Factory's
signature.

For the supported complexity-routing packaged factory, initialize a new home
and invoke `@you/classifier` by name:

```bash
you config init
you run --named @you/classifier "Summarize the release notes."
```

It classifies the request as `small`, `medium`, or `large` and follows that
factory-defined `classificationRoutes` target. See `you docs authoring-factories`
for its baseline presets, customization, override precedence, and invalid-label
behavior.

## Batch and continuous operation

Batch startup uses `--work`; it is separate from one-shot positional or stdin
invocation input:

```bash
you run --dir ./factory --work ./batches/release.json
```

Keep the Factory Session available after it becomes idle when an external
system, watched inbox, CLI submitter, or API client will add more Work:

```bash
you run --dir ./factory --continuously --work ./batches/release.json
```

Cancel a continuous run with the normal process interrupt. Use `you docs work`
for live Work submission and `you docs sessions` for Factory Session
inspection.

## Mock workers

Add `--with-mock-workers` for deterministic accepted dispatches without live
provider calls. Keep explicit Work in the run:

```bash
you run --dir ./factory --with-mock-workers --work ./docs/examples/startup-work.json
```

An optional JSON path selects targeted accept, reject, or script outcomes:

```bash
you run --dir ./factory --with-mock-workers ./docs/examples/mock-workers.json --work ./docs/examples/startup-work.json
```

Use `you docs mock-workers` for the config contract and passthrough behavior.

## Invocation output

Supported one-shot `--factory` and `--named` invocations expose three stdout
modes. Use `you docs config` for `invocationReturn` and primary-result
selection policy.

### Primary-result mode (default)

Successful invocations write only the Factory's configured primary result to
stdout. Redirect it directly:

```bash
you run --factory ./factory.json "Summarize the changelog" > result.txt
```

### Human response-stream mode

Select `--output response-stream` to render live progress for people on the
terminal. The stream ends with the same primary result as primary-result mode:

```bash
you run --named team-review --output response-stream "Review the release notes"
```

### NDJSON automation mode

Add global `--json` with `--output response-stream` for newline-delimited
automation output. Each non-empty stdout line is one complete JSON record.
Streamed events use `recordType=response_event` with a nested public
`FactoryResponseEvent` that matches the session API contract. An available
invocation response ends with exactly one terminal `recordType=invocation_result`
record. That terminal record is always the final line, including when stdout is
slow. NDJSON mode does not emit retired private progress, compaction, gap, or
`primary_result` record shapes from earlier releases.

```bash
you --json run --factory ./factory.json --output response-stream "Summarize the changelog"
```

### Mode availability

`--output response-stream` is not available for `--work`, continuous, replay,
or other non-invocation run shapes. For primary-result automation without
response events, global `--json` preserves the invocation response contract:

```bash
you --json run --factory ./factory.json "Summarize the changelog"
```

Factory authoring and validation live under `you docs config`. Dynamic workflow
execution uses `you workflow` and `you docs javascript-workflows`.
