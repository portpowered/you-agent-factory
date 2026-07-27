---
author: Agent Factory Team
last-modified: 2026-07-27
doc-id: agent-factory/guides/run
---

# Run

Use `you run` to start a Factory or perform a one-shot Factory invocation. This
is the canonical packaged entry point for local, explicit-factory, batch,
continuous, and mock-worker run tasks.

## Choose a run shape

| Task | Run shape |
|------|-----------|
| Run the exact Current Factory with initial Work | `you run --work <batch.json>` |
| Start a Factory directory | `you run --dir <factory-dir> --work <batch.json>` |
| Invoke one portable or named Factory | `you run --factory <factory-file-or-directory> <text>` or `you run --named <name> <text>` |
| Execute a JavaScript workflow | `you run --factory <workflow.js>` |
| Keep a local Factory Session alive while idle | Add `--continuously` |
| Replace live worker dispatch with deterministic outcomes | Add `--with-mock-workers [config.json]` |
| Serve an API only for the run lifetime | Add `--with-server` |
| Serve the embedded dashboard and open it once | Add `--with-site` |
| Serve the Current Factory continuously | `you server` |

Run `you run --help` for the complete live flag boundary.

## Run the current Factory

Without `--dir`, `--named`, or `--factory`, `you run` selects exactly
`<invocation-working-directory>/factory/factory.json`. It does not bootstrap a
missing Factory, follow `.current-factory`, or search another directory.

From a project with that exact file, submit explicit initial Work:

```bash
you run --work ./docs/examples/startup-work.json
```

This starts the Current Factory without an HTTP listener or browser, submits the
batch, and exits after the Factory becomes idle. Missing or invalid Current
Factory definitions fail before a Factory Session, worker, runtime host,
listener, or browser is activated.

To select another Factory directory explicitly:

```bash
you run --dir ./factory --work ./docs/examples/startup-work.json
```

`--work` accepts one `FACTORY_REQUEST_BATCH` JSON file. Use
`you docs batch-inputs` for its fields and watched-input alternatives.

## Invoke one Factory

Use `--factory` for an explicit `.json`, `.yaml`, or `.yml` document, or a
directory containing exactly one `factory.json`, `factory.yaml`, or
`factory.yml`, and supply one logical invocation input as positional text:

```bash
you run --factory ./factory.json "Review the release notes"
you run --factory ./factory.yaml "Review the release notes"
you run --factory ./factory "Review the release notes"
```

Non-interactive stdin is the alternative input source:

```bash
printf '%s\n' 'Review the release notes' | you run --factory ./factory.json
```

Do not supply positional text and stdin together. Factories with an
`invocationSignature` may instead define named, file-path, repeated, or
defaulted arguments. Inspect their exact input boundary with
`you run --factory ./factory.yaml --help`.

Use `--named` for a persisted Factory and still provide its required input:

```bash
you run --named team-review "Review the release notes"
```

## Built-in `@you/quorum`

`@you/quorum` is the supported named factory for one request that is evaluated
by two independent branch roles and then synthesized by a final merge role.
It has a logical split, branch A, branch B, and final merge. The complete
fan-out/fan-in workflow preserves the original request and both branch outputs;
the final merge is gated until both branches finish.

Packaged Factories are materialized automatically during normal runtime
initialization. Invoke one through the same named-factory path:

```bash
you run --named @you/quorum "Compare the two proposed release plans."
```

The branch workers use `branchProvider` and `branchModel`; the final merge
worker uses `mergeProvider` and `mergeModel`. Pass their supported CLI names as
`--branch-provider`, `--branch-model`, `--merge-provider`, and `--merge-model`.
Both roles default to `CODEX` with model `gpt-5`; provider overrides accept
`CODEX` or `CLAUDE`. Named CLI values take precedence over those packaged
parameter defaults without changing the fixed two-branch fan-in order.

```bash
you run --named @you/quorum \
  --branch-provider CODEX --branch-model gpt-5 \
  --merge-provider CLAUDE --merge-model claude-sonnet-4-20250514 \
  "Compare the two proposed release plans."
```

Project-local named Factories are resolved before operator-level named
Factories. Use `you run --named team-review --help` to inspect a named Factory's
signature.

## Run a JavaScript Factory Session

Passing a `.js`, `.mjs`, or `.cjs` workflow file to `--factory` selects the
standalone durable JavaScript execution path and waits for it to finish:

```bash
you run --factory ./factory.js
```

Child calls such as `agent.run` use the injected live provider execution edge.
Add `--with-mock-workers` to select deterministic fake child execution without
calling a provider:

```bash
you run --factory ./factory.js --with-mock-workers
```

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

## Server and site lifecycles

Ordinary `you run` shapes are serverless. This includes Current Factory batch
and continuous runs, named and portable one-shot invocation, replay,
mock-worker execution, JavaScript-orchestrated Factories, and raw `.js`, `.mjs`,
or `.cjs` workflows.

Add `--with-server` to serve the generated Current Factory API only for that run
without opening a browser:

```bash
you run --work ./docs/examples/startup-work.json --with-server
you run --factory ./workflow.js --with-server
```

The listener becomes ready before the run begins observable work. When a
one-shot run completes, or a continuous run is cancelled, the command cancels
and joins the listener and runtime before returning. Listener startup failure
cancels a waiting run and produces no browser effect.

Add `--with-site` to imply the same API server, serve the embedded dashboard,
and open it exactly once after listener and runtime readiness:

```bash
you run --named team-review --with-site "Review the release notes"
```

Use `you server` when the server itself is the continuous operation. It
validates the exact invocation-local `./factory/factory.json`, opens the
dashboard once after readiness, and runs until cancellation. It never
bootstraps a missing Current Factory:

```bash
you server
you --server http://127.0.0.1:7437 server
```

For `you server` and server-enabled runs, global `--server` supplies the
preferred local endpoint. The host must be `localhost` or a loopback IP. The
listener binds that exact host; it never binds a wildcard address. If the
preferred port is occupied, binding advances monotonically to the next
available port through `65535`, and startup output and browser presentation use
the actual bound URL. Invalid hosts, invalid or exhausted ports, and terminal
listener failures return `SERVER_BIND_FAILED`.

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

Add `--quiet` when the same terminal-only contract must also suppress operator
diagnostics. Quiet stdout is the raw primary result: it has no lifecycle text,
event records, JSON wrapper, or provider-session chunks. Live and `--replay`
invocations use the same quiet presentation rule. `--quiet` conflicts with
global `--json` and with explicit `--output`.

### Single-JSON automation mode

Add global `--json` without `--output response-stream` to write exactly one
`InvocationResponse` JSON object. Lifecycle records and provider-session chunks
are not included. Live and `--replay` invocations use the same single-response
presentation rule:

```bash
you --json run --factory ./factory.json "Summarize the changelog"
```

### Human Factory Event stream mode

Select `--output response-stream` to render the ordered canonical Factory Event
lifecycle for people on the terminal. The same consumer is used for live and
`--replay` invocations, and the stream ends with the same primary result as
primary-result mode:

```bash
you run --named team-review --output response-stream "Review the release notes"
```

Human lifecycle lines summarize Work acceptance, Factory Session start and
completion, workstation queue/start/outcome, inference start/outcome,
JavaScript phase and checkpoint changes, and final-output availability. They
retain canonical event order without printing provider tokens, deltas,
tool-call chunks, or provider-session chunks. Redirecting stdout preserves this
human presentation; terminal detection does not silently select another format.

### NDJSON automation mode

Add global `--json` with `--output response-stream` for newline-delimited
automation output. Each non-empty stdout line is one complete JSON record.
Streamed events use `recordType=factory_event` with a nested canonical
`FactoryEvent`, including its unchanged session sequence context. An available
invocation response ends with exactly one terminal `recordType=invocation_result`
record whose `response` field is the `InvocationResponse`. That terminal record
is always the final line, including when stdout is slow. Provider response,
diagnostic, Provider Session, delta, and tool-call fields are omitted from event
payloads at this presentation boundary. NDJSON mode does not emit retired
private progress, compaction, gap, or `primary_result` record shapes from
earlier releases. The CLI never emits a raw `FactoryResponseEvent` or a
`recordType=response_event` record.

```bash
you --json run --factory ./factory.json --output response-stream "Summarize the changelog"
```

### Invocation failures

Every one-shot invocation failure writes exactly one `ErrorResponse` JSON
object to stderr and exits unsuccessfully, in every output mode. Failures that
occur before a terminal response leave stdout empty. When the Factory Session
returns a failed `InvocationResponse`, single-JSON and NDJSON modes still write
that terminal response once to stdout; human stream mode writes its terminal
outcome, while quiet mode writes no terminal value. Provider response chunks
are never used as error output.

Current Factory selection reports `CURRENT_FACTORY_NOT_FOUND` or
`CURRENT_FACTORY_INVALID`. Output selection conflicts report
`INVOCATION_OUTPUT_CONFLICT`; unsupported output values or run shapes report
`INVOCATION_OUTPUT_UNSUPPORTED`. Invocation execution failures report
`RUN_INVOCATION_FAILED`, and local hosting failures report
`SERVER_BIND_FAILED`.

Successful commands exit `0`; usage and runtime failures exit `1`. Cancelling
`you run` or `you server` through the normal process interrupt exits `130`
after owned lifecycle resources have joined.

### Mode availability

`--output response-stream` is available for live and replayed one-shot Factory
invocations. It is not available for `--work`, continuous, or other
non-invocation run shapes. Use single-JSON mode when automation needs the
terminal invocation response without Factory Events.

Factory authoring and validation live under `you docs config`. JavaScript
orchestrator authoring uses `you docs javascript-workflows`; execution uses the
canonical Factory and Factory Session surfaces described above.
