# ACP part 2: restore the intended provider system

## Status and purpose

This document is the corrective implementation plan for the ACP work. It is
based on the original ACP proposal, the observed failures in `cursor-fail.md`,
and the current implementation. Where the current implementation conflicts
with this document, this document describes the intended target.

The first ACP implementation proved that `you` can communicate with an ACP
agent over real stdio, but it narrowed the design into a one-process-per-attempt
adapter and an ACP-specific catalog command. That does not fulfill the original
system intent.

The corrected system must provide:

- a persistent ACP process/connection owner beneath the Providers service;
- one Providers construction path, without invocation-time reinjection or root
  reconstruction;
- a unified `you workers list` catalog;
- packaged and operator-configured ACP integrations loaded through global
  configuration;
- a consistent provider-selection vocabulary;
- useful response-stream output and typed provider failures;
- functional proof through the public `you` process and raw OS stdio peers.

## Problem statement

Customers have existing agent harnesses, but many cannot currently be selected
and operated through `you`. ACP provides a shared protocol through which `you`
can initialize an agent, create or restore sessions, submit prompts, receive
structured progress, handle permissions, and manage session lifecycle.

Adding wire compatibility alone is insufficient. Customers must be able to
discover agents, configure them, select them consistently, run them through the
normal factory experience, understand failures, and eventually inspect and
operate their sessions.

## Priority boundaries

### P0: required corrective delivery

- Unified worker/provider discovery.
- Built-in and custom ACP integration configuration.
- Persistent ACP process reuse within a live `you` runtime.
- One Providers graph constructed once by Wire.
- Normal factory, packaged factory, JavaScript, and website compatibility.
- Correct `you run` provider/model flags and defaults.
- Typed ACP failures and useful response-stream output.
- Functional tests over real OS stdio.

### P1: preserve in the design, do not fake in P0

- Public worker-session list/run/resume/pause/close/delete/stream commands.
- Durable worker-session event storage.
- Cross-process session recovery and daemon IPC for separate `you` CLI
  invocations.
- Full ACP mode/configuration, elicitation, filesystem, and terminal support.

P1 being deferred does **not** permit P0 to use a one-shot ACP process. P0 must
establish the lifecycle-owning subservice and reuse a single live ACP process.
P1 extends that owner with durable and customer-controlled session operations.

## Non-negotiable system constraints

### 1. Provider terminology has one meaning at every boundary

For an ACP-backed worker:

- `executorProvider` selects the execution mechanism and is `ACP`.
- `modelProvider` selects the configured worker/provider integration, such as
  `cursor-acp` or `ac-cursor`.
- `model` optionally selects a model exposed by that integration.
- The `you run` spelling for `modelProvider` is `--provider`.
- The `you run` spelling for `model` is `--model`.

Example authored worker:

```yaml
name: implementer
type: AGENT_WORKER
executorProvider: ACP
modelProvider: cursor-acp
skipPermissions: true
body: |
  Complete the requested work and run the relevant tests.
```

Example override:

```text
you run --provider cursor-acp --model auto --skip-permissions \
  --named @you/goal "Add a unit test and run it."
```

The implementation must remove the present ambiguity in which
`cursor-acp` is sometimes supplied as an executor, sometimes as a model
provider, and sometimes through `--model-provider`. Compatibility mapping may
be used during migration, but all newly emitted config, help, examples, and API
responses must use the canonical meanings above.

### 2. ACP is a Providers-owned subservice, not a stateless decoder

ACP must live beneath the Providers service as a private subservice with a
single public-to-parent interface. The production SDK adapter remains private
to that subservice.

Target shape:

```text
pkg/services/providers/
  service.go
  internal/
    service/
      service.go
    services/
      builtins/
        service.go
        internal/service/
        wire/
      acp/
        service.go
        internal/service/
          service.go
          daemon.go
          connection.go
          sessions.go
          failures.go
          progress.go
        wire/
      catalog/
        ...
      execution/
        ...
  wire/
    wire.go
  transports/
    cli/
      ...
```

Exact implementation filenames may remain small and focused, but the ownership
rules are fixed:

- The Providers root exposes provider-neutral contracts only.
- The ACP subservice owns ACP process, connection, negotiated capabilities,
  session routing, and protocol mapping.
- `acp-go-sdk` types never escape the ACP subservice.
- The built-in integration catalog is a parallel service, not hard-coded as
  side effects in a root constructor.
- The execution service delegates ACP work to the already-constructed ACP
  subservice.
- Nothing beneath `pkg/services/workers` owns or constructs ACP.
- Top-level CLI packages bind flags and rendering; provider-specific operations
  belong to the Providers service transport.

The parent-private ACP contract should be provider-neutral in shape, for
example:

```go
type Service interface {
    Providers(context.Context) ([]Provider, error)
    Execute(context.Context, ExecuteRequest) (ExecuteResult, error)
}
```

Lifecycle may be exposed as an exact role to the initializer, but it must not
create a second product-service root.

### 3. A live runtime reuses one ACP process

The ACP subservice must lazily start and retain one stdio agent process per
effective ACP integration within a live `you` runtime.

- The first execution for an integration starts the process and initializes
  one SDK client-side connection.
- Later executions for that integration reuse the same process and connection.
- P0 may create a new ACP session for each execution, but must not create a new
  OS process for each execution.
- A connection-scoped mutex or queue serializes operations unless the selected
  agent explicitly advertises and proves safe concurrent behavior.
- Separate integrations never share a process or connection.
- A crashed process fails the active execution with a typed failure. A later
  execution may lazily create a replacement process.
- A prompt must not be silently replayed after an uncertain process failure;
  prompts are not assumed to be idempotent.
- Cancellation cancels the active ACP prompt first and preserves a healthy
  process when possible. Forced process-tree termination is a fallback.
- Config replacement or provider deletion drains and closes only the affected
  integration daemon.
- Application shutdown closes sessions/connections, terminates child process
  trees, waits for them, and leaves no orphan.

Persistence here means persistence for the lifetime of the constructed runtime.
Persistence across separate `you run` operating-system processes requires the
P1 daemon/IPC or server-hosted session work and must not be implied by P0 docs.

### 4. Providers is constructed once

There must be exactly one Providers construction path in the application graph.

- `pkg/wire` calls the Providers service-local Wire constructor once.
- All concrete dependencies are supplied to that call directly.
- The constructor returns an inert Providers root plus any exact lifecycle role
  needed by the initializer.
- Invocation code does not reconstruct Providers from options.
- CLI add/delete does not create a replacement Providers root to observe config.
- A `providers.Factory` callback must not be used as a service locator or
  reinjection mechanism.
- Configuration changes flow through an injected configuration read/watch
  contract and update the existing ACP/catalog subservices atomically.
- Production defaults such as `exec.Command` are resolved by composition before
  constructing the ACP subservice. The ACP service keeps fail-closed dependency
  validation.
- Tests may replace only external effects through `edges.Edges`; they may not
  replace the Providers root, ACP service, registry, or protocol mapper in a
  functional scenario.

### 5. Configuration and catalog data are not constructor literals

Global operator configuration owns the effective ACP integration set:

```json
{
  "defaults": {},
  "workers": {
    "acp": {
      "integrations": [
        {
          "name": "ac-cursor",
          "transport": "stdio",
          "command": "cursor-agent acp"
        }
      ]
    }
  }
}
```

- `name` is the customer-visible stable identity. A second generated public ID
  is not required.
- P0 accepts `stdio`; unsupported transports fail validation.
- `you init` materializes the packaged ACP defaults into global config when the
  ACP section is absent.
- Existing user entries are never overwritten by `you init` without an
  explicit migration/confirmation path.
- Built-in definitions originate from packaged catalog data owned by the
  built-ins service, not a large list inside a Wire constructor.
- The running Providers service lazily reads the effective config and refreshes
  only when config revision changes.
- Add/delete writes atomically and notifies or invalidates the existing service;
  it does not rebuild the application graph.
- Config round trips preserve unrelated settings and stable ordering.

### 6. The default ACP catalog is broad and reviewable

The packaged catalog must include the ACPX-derived integrations recorded in the
failure notes. Canonical worker identities use the `-acp` suffix so they do not
collide with existing script-wrap workers.

| Worker identity | Command |
| --- | --- |
| `pi-acp` | `npx pi-acp` |
| `openclaw-acp` | `openclaw acp` |
| `codex-acp` | `npx -y @agentclientprotocol/codex-acp` |
| `claude-acp` | `npx -y @agentclientprotocol/claude-agent-acp` |
| `gemini-acp` | `gemini --acp` |
| `cursor-acp` | `cursor-agent acp` |
| `copilot-acp` | `copilot --acp --stdio` |
| `droid-acp` | `droid exec --output-format acp` |
| `fast-agent-acp` | `uvx fast-agent-mcp acp` |
| `grok-build-acp` | `grok agent stdio` |
| `iflow-acp` | `iflow --experimental-acp` |
| `kilocode-acp` | `npx -y @kilocode/cli acp` |
| `kimi-acp` | `kimi acp` |
| `kiro-acp` | `kiro-cli-chat acp` |
| `mux-acp` | ACPX-owned Mux package/command, pinned after verification |
| `opencode-acp` | `npx -y opencode-ai acp` |
| `pool-acp` | `pool acp` |
| `qoder-acp` | `qodercli --acp` |
| `qwen-acp` | `qwen --acp` |
| `reasonix-acp` | `reasonix acp`, verified before packaging |
| `trae-acp` | `traecli acp serve` |
| `zeroclaw-acp` | `zeroclaw acp` |

`factory-droid` and `factorydroid` resolve as aliases of `droid-acp` without
creating duplicate catalog rows. Every packaged command and package version
must be verified against its upstream harness before release; uncertain entries
remain visible as unavailable with a useful prerequisite message rather than
being silently omitted.

### 7. Worker listing is unified

The customer discovery command is:

```text
you workers list
```

It lists all selectable worker/provider integrations, regardless of execution
mechanism:

```text
NAME         TYPE
codex        AGENT
claude       AGENT
cursor       AGENT
pi           AGENT
cursor-acp   AGENT-ACP
```

- `you workers acp add` and `you workers acp delete` remain ACP-specific
  configuration commands.
- `you workers acp list` must not be the only discovery surface. It may be
  removed, deprecated as an alias/filter, or retained only as an ACP-specific
  diagnostic after `you workers list` exists.
- List rows come from the live Providers/built-ins catalog, not a CLI-local
  table.
- The result contains the exact effective set with deterministic ordering.
- Each row can represent type, availability, source (`built-in` or `custom`),
  and a concise unavailable reason without hiding unavailable integrations.

### 8. ACP installation and deletion have stable outcomes

```text
you workers acp add --name ac-cursor --transport stdio \
  --argument "cursor-agent acp"
```

Success:

- exit code `0`;
- stdout contains `install succeeded`;
- the provider appears in `you workers list` without restarting/reconstructing
  the process graph;
- first use lazily starts its daemon.

Failure:

- a classified CLI exit code, defaulting to `1` when no more specific public
  code exists;
- one actionable failure message on stderr;
- no partial config mutation.

Deletion:

```text
you workers acp delete --name ac-cursor
```

- removes only the custom integration;
- drains and stops its live daemon;
- does not remove packaged built-in metadata;
- causes a later selection of the deleted custom identity to fail clearly.

### 9. `you init` exposes and writes the available choices

- Provider suggestions enumerate the effective worker-provider identities.
- Model configuration is optional; interactive init accepts a blank model.
- Init writes the default ACP integration section when it is absent.
- Init preserves existing custom integrations.
- Generated examples use `executorProvider: ACP` plus the chosen
  `modelProvider`.
- Init validates executable availability but does not require every packaged
  provider to be installed.

### 10. `you run` defaults optimize for customer diagnosis

- Default output is the response stream, not final-output-only mode.
- Packaged model-execution factories default to `skipPermissions: true` unless
  a factory explicitly requires interactive permissions.
- `--provider` and `--model` are run-level flags, not global default-worker
  flags.
- Remove `--runtime-log` and `--runtime-log-dir` from `you run`; logging paths
  derive from global configuration.
- `-v` diagnostics go to stderr or the configured log sink, never into stdout's
  response/event stream.
- Help content documents the common customer workflow first and avoids dumping
  internal runtime terminology.

Human response-stream rendering must:

- say `factory started`, not `Factory Session started`;
- show useful Work content/summary instead of only `work-1`;
- assign consistent colors to factory/work lifecycle lines;
- assign a stable per-workstation color to workstation and inference lines;
- remain machine-clean when JSON or JSON-stream output is requested;
- preserve explicit terminal status and error details.

### 11. Failures retain their cause through every layer

An ACP failure must not become `provider error: unknown` or a generic canceled
invocation unless cancellation was the actual cause.

The ACP subservice must classify at least:

- executable missing or process start failure;
- authentication required;
- incompatible protocol version;
- unsupported ACP capability or client request;
- invalid provider/model/session configuration;
- permission denied or non-interactive permission unavailable;
- session creation/load/resume failure;
- prompt RPC failure;
- malformed protocol traffic;
- unexpected process exit;
- user/context cancellation;
- configured execution timeout;
- internal dependency/composition failure.

The provider-neutral failure must carry a stable kind, safe customer message,
provider identity, operation, retryability where known, and safe stderr detail.
Factory response events and the terminal invocation outcome must preserve that
classification. Lifecycle bookkeeping errors may be included as diagnostics,
but must not replace the initiating ACP error.

### 12. Observability belongs at the ACP boundary

The ACP subservice logs structured lifecycle facts for:

- daemon lazy start, ready, reuse, drain, restart, and stop;
- provider identity and safe process metadata;
- initialize and negotiated capabilities;
- session creation/resolution;
- prompt start, progress kind, terminal stop reason, and elapsed time;
- cancellation and forced termination;
- classified failure kind and safe error detail.

Logs must never include secrets, raw environment values, or unredacted prompt
content. Protocol stdout remains exclusively ACP JSON-RPC. Human diagnostics and
verbose output use stderr/log sinks.

### 13. Website compatibility is P0

The embedded website must enumerate ACP entries as worker/provider type `ACP`
and must not reject, crash on, or silently rewrite the new type/provider
identity. No full worker-session UI is required for P0.

### 14. Production protocol code uses the SDK; wire tests remain independent

- Production uses `github.com/coder/acp-go-sdk` for ACP types, correlation,
  encoding, decoding, and client connection behavior.
- Production does not contain a handwritten JSON-RPC codec.
- Functional mocks use raw JSON at the OS stdin/stdout boundary and do not
  import SDK types.
- Upstream SDK JSON goldens remain checked-in test samples with provenance.
- The raw peer is a separate process and validates the actual wire exchange.

### 15. Integration behavior is proven at customer boundaries

- Do not add package-internal tests mislabeled as integration tests.
- Owner-local unit tests are allowed for pure validation, mapping, daemon state
  transitions, locking, and failure normalization.
- Cross-service composition, process reuse, CLI behavior, configuration reload,
  response events, and real stdio traffic belong in functional tests using
  `root.BuildProcess`, `Process.Execute`, the compiled CLI, public HTTP, or
  public MCP as appropriate.
- The primary ACP scenarios run through `you run` until terminal completion.
- Event-stream parsing is used only when terminal output cannot prove ordering
  or typed event shape.

## ACP capability mapping

| ACP capability | P0 `you` behavior | Gap / follow-up |
| --- | --- | --- |
| initialize and capability negotiation | Required | Persist negotiated facts on the live daemon. |
| `session/new` | Required | P0 may create a session per execution while reusing the process. |
| `session/prompt` and session updates | Required | Map all supported updates into provider-neutral progress/events. |
| `session/cancel` | Required | Attempt cooperative cancellation before force termination. |
| permissions | Required | Existing `skipPermissions`; no new P0 policy object. |
| session config/model/mode options | Model when advertised | General modes/options remain P1. |
| `session/list` | P1 | Back `you worker-sessions list`; capability-gated. |
| `session/load` / `session/resume` | P1 | Required for cross-turn/cross-process continuity. |
| session close | Internal cleanup in P0; public P1 | Do not equate close with delete. |
| session delete | P1 and capability-gated | Define soft/hard deletion expectations at the client boundary. |
| filesystem and terminal client methods | P1 | P0 fails unsupported requests explicitly. |
| elicitation | P1 | Requires a transport/customer interaction design. |
| durable event/history export | P1 | Store provider-neutral events, not SDK structs. |
| pause | P1 product operation | ACP has cancellation/close/resume primitives; define pause semantics rather than inventing a direct mapping. |

## Ordered implementation tasks

Each task is a vertical, reviewable behavior slice. File movement alone is not
completion.

### Task 1: lock the public provider vocabulary

- Change authored/runtime mapping so ACP workers canonically use
  `executorProvider: ACP` and `modelProvider: <integration>`.
- Add `you run --provider` and `--model` at run scope.
- Remove or migrate the global default-worker model/provider flags.
- Add an explicit compatibility path for existing factories that use
  `executorProvider: cursor-acp`, with a warning and deterministic normalized
  result.
- Update API descriptions, generated artifacts, validation, JavaScript input,
  examples, and help together.

Acceptance criteria:

- Custom factory, packaged factory, JavaScript, HTTP, and CLI select the same
  provider identity with the same meaning.
- New serialized config never emits `cursor-acp` as the execution mechanism.
- Ambiguous combinations fail with an actionable validation error.

### Task 2: package the complete built-in catalog

- Move built-in ACP definitions out of Wire literals into Providers-owned
  packaged data.
- Add every catalog entry in the required table, including aliases.
- Verify commands and versions against upstream harness documentation.
- Represent missing prerequisites as unavailable catalog facts.

Acceptance criteria:

- Catalog tests assert the exact identity/command/alias set.
- Uninstalled providers remain discoverable with an actionable reason.
- Adding a new packaged provider changes data plus focused validation, not the
  application constructor.

### Task 3: make global config the effective integration source

- Extend `you init` to materialize packaged ACP defaults when absent.
- Implement revision-aware lazy loading in the existing Providers service.
- Make add/delete atomic and notify/invalidate the live service.
- Preserve unrelated settings and custom overrides.

Acceptance criteria:

- Add/delete becomes visible without reconstructing Providers.
- Re-running init is idempotent and preserves customer entries.
- Malformed config leaves the previous valid live catalog intact and reports a
  typed configuration error.

### Task 4: construct Providers exactly once

- Replace factory/reconstruction callbacks with direct dependencies supplied to
  one service-local Wire constructor.
- Construct built-ins, ACP, catalog, and execution subservices once.
- Bind exact lifecycle roles to the initializer.
- Resolve production process-command and executable-lookup defaults before ACP
  construction.
- Move Providers/ACP construction baselines and ownership checks from broad root
  locations to their owning service packages where the architecture standards
  require them.

Acceptance criteria:

- A graph test proves exactly one Providers root construction.
- Invocation and CLI packages contain no Providers reconstruction path.
- `root.BuildProcess(Edges{})` receives a working production command factory.

### Task 5: implement the persistent ACP daemon subservice

- Create the parent-private ACP service and daemon pool.
- Start one process lazily per effective integration.
- Initialize one SDK connection and retain negotiated capabilities.
- Serialize/queue prompts on the connection.
- Reuse the process for later executions.
- Implement cancellation, crash invalidation, config drain, and application
  shutdown.
- Keep SDK conversion and progress mapping inside the subservice.

Acceptance criteria:

- Two sequential `you run` executions within one live runtime record one
  `cmd.Start`, one ACP initialize, and two prompt submissions through the same
  peer process.
- Concurrent submissions do not interleave invalid RPC traffic and obey the
  documented queue/serialization rule.
- A crash fails the active prompt and the next request starts exactly one
  replacement process.
- Shutdown and deletion leave no child process running.

### Task 6: deliver the unified worker catalog CLI

- Add the generated command contract and implementation for
  `you workers list`.
- Merge script-wrap/built-in and ACP integration descriptors through the
  Providers catalog.
- Render deterministic name/type/availability/source output.
- Decide and document whether `you workers acp list` is removed, deprecated, or
  retained as a filtered alias.

Acceptance criteria:

- The output contains both `AGENT` and `AGENT_you ACP` rows.
- Exact-set tests fail for missing or unexpected providers.
- List uses the live service and reflects add/delete immediately.

### Task 7: correct ACP add/delete behavior

- Align CLI stdout, stderr, and exit-code behavior with the customer contract.
- Validate name, stdio transport, command parsing, collisions, and built-in
  deletion rules.
- Drain live daemons on replacement/deletion.

Acceptance criteria:

- Successful add exits zero and prints `install succeeded`.
- Invalid add/delete leaves configuration and live catalog unchanged.
- Deleting an active custom integration safely closes its process and causes
  subsequent selection to fail clearly.

### Task 8: preserve typed failures from ACP to terminal

- Define the full ACP-to-provider failure mapping.
- Preserve it through execution, worker response events, workstation failure,
  Work terminal state, and invocation outcome.
- Ensure lifecycle-closure diagnostics do not overwrite the originating error.
- Add safe stderr details and structured ACP boundary logs.

Acceptance criteria:

- Authentication, missing executable, protocol mismatch, permission failure,
  prompt RPC failure, process exit, cancellation, and timeout are visibly
  distinct.
- The Cursor failure reproduced in `cursor-fail.md` never renders as
  `provider error: unknown`.
- A real cancellation is reported as cancellation; a provider failure is not.

### Task 9: repair `you run` customer defaults and rendering

- Make response-stream the default output.
- Move/rename provider and model flags.
- Remove runtime-log and runtime-metrics flags from `you run`.
- Route verbose diagnostics away from stdout.
- Default packaged model factories to skip permissions.
- Improve human labels, Work content, coloring, and terminal failure detail.
- Simplify run help around common workflows.

Acceptance criteria:

- Default terminal output is useful without extra flags.
- JSON/JSON-stream stdout remains stable and color-free.
- `-v` cannot corrupt stdout event streams.
- Packaged ACP execution does not hang waiting for an unavailable interactive
  permission path.

### Task 10: make init and website ACP-aware

- Enumerate worker-provider choices in init.
- Permit an omitted model.
- Write default ACP config without overwriting custom entries.
- Update generated website/API types and projections to accept and display ACP.

Acceptance criteria:

- A clean init produces a usable ACP-aware global config.
- The website lists ACP workers and does not reject their provider fields.
- No P1 session UI is required.

### Task 11: replace internal integration coverage with functional proof

- Retain owner-local unit tests for pure daemon state and mapping.
- Move cross-service/provider integration scenarios to
  `tests/functional/providers/acp`.
- Extend the independent raw stdio peer to remain alive for multiple prompts.
- Retain upstream-derived JSON goldens and provider-neutral response goldens.
- Exercise the real process graph and compiled CLI.

Required functional files/scenarios:

```text
tests/functional/providers/acp/
  daemon_reuse_test.go
  daemon_concurrency_test.go
  daemon_crash_recovery_test.go
  daemon_shutdown_test.go
  catalog_workers_list_test.go
  catalog_config_reload_test.go
  catalog_cli_negative_test.go
  run_typed_failures_test.go
  run_response_stream_test.go
  run_permissions_test.go
  run_parameters_content_test.go
  javascript_factory_run_test.go
  packaged_factory_run_test.go
  website_contract_test.go
  functional_rpc_peer_test.go
  golden_rpc_peer_test.go
```

Acceptance criteria:

- The daemon reuse test observes the same OS peer and connection across two
  executions, not merely the same mock object.
- Raw peers do not import `acp-go-sdk`.
- At least one test constructs `root.BuildProcess(Edges{})` and launches a peer
  found through `PATH`, proving production defaults.
- Tests primarily enter through `you run` and observe terminal completion.
- Focused ACP/CLI coverage remains at least 80-85%, including failure and
  cleanup paths.

### Task 12: validate with real harnesses and deliver through merge

- Run the README/provider workflow with Cursor ACP.
- Run representative packaged entries covering direct binaries, `npx`, and
  `uvx` launch forms.
- Validate custom add/run/delete and immediate catalog refresh.
- Validate packaged factory and JavaScript execution.
- Run focused tests, docs smoke, generated-contract verification,
  `make verify-fast`, and the broader provider/runtime PR tier.
- Continue through terminal green CI, blocking review feedback, conflict
  resolution, and actual PR merge.

Acceptance criteria:

- Customer-visible success is proven by created artifacts and independent test
  commands, not only agent prose.
- No required process is orphaned after success, failure, cancellation, delete,
  or shutdown.
- The work is not complete until the PR is merged.

## P1 worker-session contract to preserve

P0 architecture must leave room for these commands without replacing the ACP
subservice:

```text
you worker-sessions list
you worker-session run [flags] "prompt"
you worker-session resume <session-id>
you worker-session pause <session-id>
you worker-session delete <session-id>
you worker-session stream <session-id>
```

Candidate run flags:

```text
--cwd
--worktree
--permissions <approve-all|non-interactive|deny-all>
--policy
--async
--quiet
--json
--json-stream
```

Before P1 implementation, resolve:

- synchronous versus asynchronous run semantics;
- whether pause means cancel-and-resume, close-and-resume, or a distinct local
  queued state;
- agent capability behavior for list/load/resume/close/delete;
- session ownership and authorization;
- queueing and concurrency rules;
- retention and deletion semantics;
- server-hosted versus detached-daemon persistence;
- event storage and replay contracts.

Durable provider-neutral session events should live beneath:

```text
~/.you-agent-factory/worker-sessions/<year>/<month>/<day>/...
```

Do not persist `acp-go-sdk` structs as the durable format. Store canonical
provider/session events that can survive SDK and protocol-version changes.

## Out of scope

- The separate `you-agent-factory-docs` ACP page is delivered by its own
  project, although this repository must provide canonical reference material
  for it.
- A new P0 permission-policy DSL. P0 continues to use `skipPermissions`.
- ACP-specific startup and turn timeout configuration. Existing execution
  timeout/cancellation remains authoritative until session policy is designed.
- Handwritten production ACP codecs.
- Broad unrelated package cleanup beyond Providers/ACP construction ownership.

## Completion checklist

- [ ] ACP worker terminology is consistent across configuration and CLI.
- [ ] Full packaged ACP catalog is data-backed and visible.
- [ ] `you init` writes/preserves ACP defaults correctly.
- [ ] Providers is constructed once.
- [ ] ACP is a Providers-owned persistent subservice.
- [ ] Two executions reuse one real ACP process and SDK connection.
- [ ] `you workers list` is the primary unified discovery command.
- [ ] Add/delete refresh the live catalog without reinjection.
- [ ] Cursor failures retain an actionable typed cause.
- [ ] Response-stream is the useful default and verbose logs do not pollute it.
- [ ] Packaged factories default to the intended permission behavior.
- [ ] Website accepts and enumerates ACP workers.
- [ ] Cross-service behavior is proven through functional tests at real edges.
- [ ] Required generated artifacts and public reference docs are synchronized.
- [ ] CI and blocking review feedback are complete and the PR is merged.

