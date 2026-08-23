---
author: Agent Factory Team
last-modified: 2026-08-21
doc-id: agent-factory/guides/run
---

# Run

Use `you run` to start a Factory or perform a one-shot Factory invocation. This
is the canonical packaged entry point for local, explicit-factory, batch,
continuous, and mock-worker run tasks.

For the operator runbook that distinguishes idle, finite-drained, and
restart-recovery states, use `you docs operations`.

Use `you docs providers` for worker/provider and model selection, provider
capabilities and limits, effort mapping, and the boundary between durable
Factory settings and one-shot overrides. This page owns run shapes and input
sources.

## Choose a run shape

| Task | Run shape |
|------|-----------|
| Run the exact Current Factory with initial Work | `you run --work <batch.json>` |
| Start a Factory directory | `you run --dir <factory-dir> --work <batch.json>` |
| Invoke one portable or named Factory | `you run --factory <factory-file-or-directory> <text>` or `you run --named <name> <text>` |
| Execute a JavaScript workflow | `you run --factory <workflow.js>` |
| Keep a local Factory Session alive while idle | Add `--continuously` |
| Replace live worker dispatch with deterministic outcomes | Add `--with-mock-workers [config.json]` |
| Run worker dispatches in an isolated Git checkout | Add `--worktree <name>` |
| Continue recoverable recorded Factory execution | `you run --resume <recording>` |
| Serve an API only for the run lifetime | Add `--with-server` |
| Serve the embedded dashboard and open it once | Add `--with-site` |
| Select an exact local listener | Add `--listen <host:port>` to `--with-server` or `--with-site` |
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

Intentional declarative-Factory invocation stdin for `--factory` and `--named`
is limited to 1,048,576 bytes (1 MiB), inclusive. Exactly 1,048,576 bytes is
accepted. A larger stream fails before Factory execution starts; use
`--to-file` when a prompt needs more space. This limit applies to the stdin
source only and does not change the Factory, HTTP, or replay contracts.

Do not supply positional text and stdin together. Factories with an
`invocationSignature` may instead define named, file-path, repeated, or
defaulted arguments. Inspect their exact input boundary with
`you run --factory ./factory.yaml --help`.

## Long prompts and explicit reasoning effort

For an applicable one-shot worker invocation, `--worker-reasoning-effort`
accepts the canonical authored-Worker values `minimal`, `low`, `medium`,
`high`, `xhigh`, and `max`. Case and surrounding whitespace are normalized.
An unsupported value fails before Factory Session activation or Provider
dispatch; a value that is canonical but unsupported by the selected Provider
continues through that Provider's existing validation.

The canonical effort vocabulary is therefore not a guarantee that every
provider/model accepts every value. See `you docs providers` before selecting
the provider-specific model or effort for a run.

Use `--to-file` when a prompt is long or multiline. It reads one regular file
with logically non-empty, valid UTF-8 content and sends its contents as one
primary prompt exactly as stored: spaces, Unicode, CRLF/LF line endings, blank
lines, and a trailing newline are preserved. The file path is the only prompt
input transported through the shell; the prompt itself is not split into
argv tokens.

`--to-file` is a one-shot source. It strictly conflicts with signature-defined
`--to`, positional invocation text, and supplied non-empty stdin; no source is
silently chosen by precedence. It is also rejected for `--work`,
`--continuously`, `--replay`, and JavaScript workflow invocations. Source,
path, encoding, and content failures happen before runtime or Provider
dispatch.

In PowerShell, create the UTF-8 file first and pass only its path. This example
uses a path containing spaces and ends with the independently runnable command
shape:

```powershell
$promptPath = Join-Path (Get-Location) "prompt files\release brief.txt"
$promptText = @"
Review the release notes and identify the highest-risk rollback step.
Keep the answer concise, but preserve the exact wording of the risk.
"@
[IO.File]::WriteAllText($promptPath, $promptText, [Text.UTF8Encoding]::new($false))

you run --provider codex --worker-reasoning-effort xhigh --to-file "prompt files\release brief.txt"
```

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

## Run in a Git worktree

Add `--worktree <name>` to create or reuse a checkout under the invocation
working directory's worktree parent and execute every worker dispatch from
that checkout:

```bash
you run --named @you/goal --worktree feature-login \
  --to "Fix the login bug and verify the focused tests"
```

The selection is run-scoped and does not modify the Factory definition. It is
provider-neutral: Codex, ACP, Claude, script, and other worker routes receive
the same materialized checkout as their working directory. Existing valid
checkouts are reused. A missing Git repository, an invalid relative worktree
name, a conflicting non-worktree path, or `git worktree add` failure stops the
dispatch with a worktree preparation error.

The name must be relative and cannot traverse outside the Factory's worktree
parent. When a run-level worktree is selected, it overrides workstation-level
`workingDirectory` and `worktree` templates for that run. Without the flag,
authored workstation behavior is unchanged.

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
you run --work ./docs/examples/startup-work.json --with-server --listen 127.0.0.1:7437
```

The listener becomes ready before the run begins observable work. When a
one-shot run completes, or a continuous run is cancelled, the command cancels
and joins the listener and runtime before returning. Listener startup failure
cancels a waiting run and produces no browser effect.

For a finite server-enabled run, an empty queue or a queue whose Work is all
terminal succeeds. If the runtime drains while `N` customer Work items remain
non-terminal, the run is unsuccessful, but it still joins the owned listener
and runtime before returning. Human CLI stderr contains exactly:

```text
Error: factory session drained with N non-terminal work items; run is incomplete
```

The failed run does not print a success or completion claim to stdout. Add
`--continuously` when an idle server-enabled run should remain live for later
Work instead of applying finite drain classification.

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
you server --listen 127.0.0.1:7437
```

Use `--listen <host:port>` when the listener must bind one exact local address.
The host must be `localhost` or a loopback IP, and the port must be a non-zero
TCP port. `--listen` takes precedence when both listener controls are present;
the `--server` value remains the HTTP API/remote endpoint and one migration
warning is written to stderr. Invalid, unavailable, or exhausted exact binds
return `SERVER_BIND_FAILED` before the run can proceed.

An explicit local `--server http://localhost:<port>` remains supported for
legacy scripts when `--listen` is absent. It binds the loopback host and can
advance monotonically through port `65535` on collisions, but prints one
actionable deprecation warning directing scripts to `--listen`. `--server` is
not a local listener selector for ordinary `you run`; ordinary runs remain
serverless, and `--listen` requires `--with-server` or `--with-site`.

## Capture local runtime memory diagnostics

`--pprof` is off by default. Enable it only for local diagnostics, and keep
the listener on a loopback host.

1. Start the local server:

   ```bash
   you server --pprof --listen 127.0.0.1:7437
   ```

   The server accepts local requests at `127.0.0.1:7437`. The pprof routes
   remain unavailable when `--pprof` is omitted.

2. Inspect the current runtime snapshot:

   ```bash
   curl -sS http://127.0.0.1:7437/debug/runtime
   ```

   The JSON includes `heapAllocBytes`, `heapInuseBytes`, `sysBytes`, `numGC`,
   `goroutines`, `processCommitBytes`, and `processCommitAvailable`.

3. Save a heap profile while the server is running:

   ```bash
   curl -sS http://127.0.0.1:7437/debug/pprof/heap -o heap.pprof
   ```

   The command creates a non-empty `heap.pprof` file.

4. Inspect the live heap profile with the Go tool:

   ```bash
   go tool pprof -top http://127.0.0.1:7437/debug/pprof/heap
   ```

   The command prints a table with allocation columns, including `flat` and
   `cum`. Use `go tool pprof -top heap.pprof` to inspect the saved copy.

On Windows, the working set can be trimmed without reducing committed process
memory. Compare `processCommitBytes` from `GET /debug/runtime` over time. Do
not use RSS or working set as the process-commit signal.

Use pprof only for local diagnostics. Never enable it on a non-loopback
listener. In PowerShell, use `curl.exe` when the `curl` alias is unavailable.

## Remote placement and local hosting

Use `--remote` when the operation should go through an already-running You
server. Global `--server` selects that server's HTTP API URI; it does not
choose a local bind for a listener-owning command:

```bash
you --remote --server <uri> run "Review the release notes"
```

Remote placement never starts a local runtime, listener, dashboard browser, or
recording for the `run` command. `--remote` conflicts with `--with-server` and
`--with-site`, because those flags ask the run to host its own local API or
dashboard. The conflict returns the stable `REMOTE_LOCAL_HOSTING_CONFLICT`
bad-request code. Remove `--remote` for local hosting, then use `--listen
<host:port>` when the local bind must be exact:

```bash
you run --with-server --listen 127.0.0.1:7437 "Review the release notes"
you run --with-site --listen 127.0.0.1:7437 "Review the release notes"
```

An explicit local `--server http://localhost:<port>` remains a warned legacy
compatibility form. Migrate it to `--listen <host:port>` for `you server` and
server-enabled local runs; use `--remote --server <uri>` when the endpoint is
already running.

## Invocation output

Supported one-shot `--factory` and `--named` invocations expose three stdout
modes. Use `you docs config` for `invocationReturn` and primary-result
selection policy.

### Primary-result mode

Select `--output primary` to write only the Factory's configured primary result
to stdout. Redirect it directly:

```bash
you run --factory ./factory.json --output primary "Summarize the changelog" > result.txt
```

Add `--quiet` when the same terminal-only contract must also suppress operator
diagnostics. Quiet stdout is the raw primary result: it has no lifecycle text,
event records, JSON wrapper, or provider-session chunks. Live and `--replay`
invocations use the same quiet presentation rule. `--quiet` conflicts with
global `--json` and with explicit `--output`.

### Single-JSON automation mode

Add global `--json` with `--output primary` to write exactly one
`InvocationResponse` JSON object. Lifecycle records and provider-session chunks
are not included. Live and `--replay` invocations use the same single-response
presentation rule:

```bash
you --json run --factory ./factory.json --output primary "Summarize the changelog"
```

### Human Factory Event stream mode (default)

One-shot text invocations render the ordered canonical Factory Event lifecycle
for people on the terminal by default. The same consumer is used for live and
`--replay` invocations, and the stream ends with the same primary result as
primary-result mode:

```bash
you run --named team-review "Review the release notes"
```

Human lifecycle lines summarize Work acceptance, Factory Session start and
completion, workstation queue/start/outcome, Worker Session association and
active-worker status, inference start/outcome, JavaScript phase and checkpoint
changes, and final-output availability. Workstation lines include the canonical
dispatch and Work IDs when the events provide them. They retain canonical event
order without printing provider tokens, deltas, tool-call chunks, or
provider-session chunks. Redirecting stdout preserves this human presentation;
terminal detection does not silently select another format. When the resolved
terminal policy and stderr TTY both permit interactive progress, active workers
also receive transient stderr-only spinner lines with stable, distinct colors.
Quiet, structured, or non-interactive output omits cursor controls and ANSI
sequences; progress never overwrites or contaminates the primary result on
stdout.

### NDJSON automation mode

Add global `--json` for newline-delimited
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
you --json run --factory ./factory.json "Summarize the changelog"
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
`RUN_INVOCATION_FAILED`. Local listener bind failures report
`SERVER_BIND_FAILED`; other service-mode startup failures detected before
listener readiness report `SERVER_START_FAILED` with a safe failure
classification.

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
