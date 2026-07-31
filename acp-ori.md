# Agent Client Protocol executor-provider plan

> Recovery note (2026-07-29): this is a reconstructed facsimile of the deleted
> `docs/temp/acp-ori.md`. The original was not found in reachable Git history or
> the current worktrees. This document restores the decisions and TODOs retained
> in the surrounding design discussion; it is not claimed to be byte-for-byte
> identical. Checkbox state represents the plan, not necessarily the state of
> the repository today.

## Summary

Add Agent Client Protocol (ACP) agents as customer-selectable executor
providers. A worker selects an ACP integration through its existing
`executorProvider`; the provider service launches the selected agent over OS
stdio and uses `github.com/coder/acp-go-sdk` for the production ACP client and
wire protocol.

This should be an additive extension of the existing provider and worker
execution flow. It should not rewrite worker execution, add ACP beneath the
`workers` package, or introduce a second public execution model.

The initial customer outcomes are:

- `you workers acp list` shows built-in and operator-added ACP integrations.
- `you workers acp add` adds or replaces a custom stdio integration.
- `you workers acp delete` removes a custom integration.
- A normal factory worker can select `cursor-acp`, `kiro-acp`, `opencode-acp`,
  or a custom integration with `executorProvider`.
- `you run` streams useful ACP progress and reaches the normal terminal result.
- Packaged factories and JavaScript `agent.run` use the same provider selection
  and response path.

## Decisions already made

- [x] The public worker field is `executorProvider`, not `protocol`.
- [x] ACP is an execution-provider concern; `modelProvider` remains separate.
- [x] Do not add ACP-specific `startupTimeout` or `turnTimeout` configuration in
  this phase. Existing invocation and worker timeout behavior remains the owner
  of execution limits.
- [x] Do not add a separate permission-policy configuration. ACP permission
  behavior is derived from the existing `skipPermissions` value.
- [x] Use provider identities that describe the executable ACP integration:
  `cursor-acp`, `kiro-acp`, and `opencode-acp`, rather than generic numbered
  presets.
- [x] P0 supports ACP over stdio only.
- [x] Production protocol encoding, decoding, request correlation, and ACP types
  come from `acp-go-sdk`; do not maintain a bespoke production codec.
- [x] Tests may use an independent raw JSON-RPC peer, but that peer must operate
  at the OS stdio boundary and must not use SDK types or SDK RPC helpers.
- [x] Do not extend `pkg/workers` for the ACP implementation. Providers are a
  customer-facing capability and are expected to be separable into their own
  service.
- [x] Follow the service/package shape described by the original
  `docs/temp/packaged-structures.md` (now represented by
  `docs/architecture/packaged-structure.md`): public service contracts at the
  service root, implementation beneath its private tree, service-owned
  transports, and composition through Wire.
- [x] Preserve the existing executor and script-wrap paths. ACP is another
  adapter selected through the provider registry, not a replacement for those
  paths.

## Scope

### In scope

- ACP provider catalog and built-in metadata.
- Operator configuration for custom stdio ACP integrations.
- CLI list/add/delete operations.
- Resolution of `executorProvider` from authored worker configuration through
  compiled execution state.
- ACP initialization, session creation, prompt execution, updates, terminal
  completion, cancellation, and error mapping.
- Provider-neutral progress and terminal output so the rest of the factory does
  not depend on ACP SDK types.
- Normal factory, packaged-factory, and JavaScript invocation paths.
- Raw stdio functional coverage and checked-in protocol goldens.
- Public provider documentation.

### Not in scope for P0

- ACP over HTTP, WebSocket, or an in-process transport.
- Persisted ACP conversation resume across separate `you run` invocations.
- A new permission-policy DSL.
- ACP-specific startup and turn timeout fields.
- Reimplementing JSON-RPC or ACP framing.
- Exposing `acp-go-sdk` request or response types outside the private adapter.
- Implementing ACP client filesystem or terminal capabilities unless a selected
  agent requires them for the initial supported path. Unsupported requests must
  fail explicitly and diagnostically.
- Moving or redesigning unrelated worker/provider code.

## Existing feature fit and gaps

| Existing `you` capability | ACP need | Plan / gap |
| --- | --- | --- |
| Worker `executorProvider` selection | Select a concrete ACP agent | Accept extensible catalog identities such as `cursor-acp`; retain existing compatibility values. |
| Provider registry/conductor | Route execution by provider identity | Register one integration per available ACP catalog entry. Route through the provider service. |
| Script/CLI subprocess execution | Launch a long-lived bidirectional peer | Reuse the injected process-launch edge, but use a bespoke ACP adapter because the interaction is request/notification RPC, not final-output parsing. |
| Provider-neutral execution request | ACP initialize/session/prompt inputs | Add a narrow mapping layer in the ACP adapter. Keep SDK types private. |
| Provider-neutral response/progress stream | ACP session updates | Map message, thought, tool, plan, usage, and session updates into existing progress/events. |
| Existing `skipPermissions` | ACP permission requests | Select the non-interactive approval response when true; otherwise follow the supported interactive/default behavior without a second policy field. |
| Existing worker/invocation timeout | Hung ACP process or turn | Apply existing cancellation/deadline behavior to the process and SDK calls. No ACP-specific timeout configuration. |
| Operator settings | Custom executable integrations | Add an ACP integration collection under worker/provider settings. |
| `you run` terminal renderer | Customer-visible proof | Exercise ACP primarily through `you run`; inspect event streams only for cases the terminal output cannot prove. |
| Factory events and session projection | ACP streaming lifecycle | Emit existing canonical response-event shapes; do not make ACP events canonical domain state. |

## Public configuration

### Worker selection

An authored agent worker selects an ACP integration by catalog identity:

```yaml
name: implementer
type: AGENT_WORKER
executorProvider: cursor-acp
skipPermissions: true
body: |
  Complete the requested change and run the relevant tests.
```

No `protocol`, `permissionPolicy`, `startupTimeout`, or `turnTimeout` fields are
added.

The same value must survive all supported authoring and invocation routes:

```text
authored worker.executorProvider
  -> Factory definition mapping/validation
  -> compiled runtime worker
  -> execution request
  -> Providers catalog/registry selection
  -> ACP integration adapter
```

JavaScript uses the same catalog identity:

```javascript
const result = await agent.run({
  prompt: "Add tests for IsEven and run them.",
  executorProvider: "cursor-acp",
});
```

### Operator settings

Custom integrations live in the normal operator configuration, not inside a
factory and not as files beneath `workers/`. The expected shape is:

```json
{
  "workers": {
    "acp": {
      "integrations": [
        {
          "id": "company-cursor",
          "name": "company-cursor",
          "transport": "stdio",
          "command": "cursor-agent acp"
        }
      ]
    }
  }
}
```

The configuration is stored with the existing operator settings (normally
`~/.you-agent-factory/config.json`). Command strings are parsed with the
existing shell-word parser and launched without introducing a shell-only
contract.

### Built-in integrations

| Identity | Default command |
| --- | --- |
| `cursor-acp` | `cursor-agent acp` |
| `kiro-acp` | `kiro-cli acp` |
| `opencode-acp` | `opencode acp` |

Built-ins remain catalog entries even when their executable is unavailable;
availability should be represented explicitly. Operator entries may add new
identities or override the executable configuration according to one documented
precedence rule. List output must not silently invent unrelated providers.

## Package and ownership shape

The plan intentionally does not place ACP under `pkg/workers`. The target shape
is a Providers service with private execution adapters, service-owned CLI
operations, and process effects supplied through composition:

```text
pkg/
  services/
    providers/
      service.go                 # public provider-neutral contract only
      internal/
        ...
          adapters/
            acp/                 # private SDK-backed adapter
      transports/
        cli/                     # service-owned list/add/delete behavior
      wire/                      # service construction bridge
  transports/
    cli/                         # command/flag composition only
  wire/                          # application composition and external edges

tests/
  functional/
    providers/
      acp/                       # customer-boundary ACP behavior
```

Exact private nesting may follow the Providers service's current execution
subservice, but ownership must remain `pkg/services/providers/.../adapters/acp`.
No other service imports that private adapter. `pkg/wire` selects construction;
the initializer only owns lifecycle.

## Executor-to-adapter mapping

ACP should use the same provider registry and conductor contract as other
executors, while using a bespoke adapter implementation like the script
executor does for its own protocol. Trying to treat ACP as a script-wrap output
decoder would lose bidirectional requests, session notifications, and
permission handling.

| Executor/request value | ACP adapter value | Notes |
| --- | --- | --- |
| `executorProvider` | catalog lookup and integration registration | Exact normalized identity; unknown and unavailable are distinct errors. |
| prompt/body | ACP prompt content block(s) | Preserve text and supported resource/content inputs. |
| working directory | ACP session `cwd` | Resolve through existing work/worktree behavior first. |
| environment | launched process environment | Preserve existing sanitization and override rules. |
| model | optional ACP model selection | Send only when the negotiated agent capability supports it. |
| `skipPermissions` | permission response selection | Reuse the existing boolean; no ACP policy object. |
| invocation context/deadline | SDK calls and subprocess cancellation | Terminate and join the child through the existing process edge. |
| provider progress destination | mapped ACP session updates | SDK values stop at the adapter boundary. |
| execution result | provider-neutral content, diagnostics, session ref | Session ref records provider identity, kind `acp`, and ACP session ID. |

The mapping flow is:

1. Factory definition mapping preserves the authored `executorProvider`.
2. Runtime dispatch builds the existing provider-neutral execution request.
3. The provider registry resolves the identity and selects the ACP integration.
4. The ACP adapter resolves and launches the configured command through the
   injected platform process-command factory.
5. `acp-go-sdk` owns JSON-RPC framing and the client connection.
6. The adapter negotiates protocol version and capabilities, creates a session,
   optionally applies supported session configuration, and sends the prompt.
7. Incoming updates are converted immediately into provider-neutral progress.
8. The adapter returns one terminal result or a typed failure and always reaps
   the subprocess.

## ACP lifecycle and behavior

### Required request sequence

1. Start the configured stdio process.
2. Connect the SDK client to process stdin/stdout.
3. Send `initialize` and validate the negotiated protocol version/capabilities.
4. Send `session/new` with working-directory and advertised client capability
   information.
5. Apply optional model/session configuration only when both requested and
   supported.
6. Send `session/prompt`.
7. Consume notifications and client-directed requests until the prompt reaches
   a terminal stop reason or fails.
8. Close the connection and deterministically terminate/join the process.

### Session updates to preserve

- Agent message chunks and final message content.
- Agent thought/reasoning chunks, without treating reasoning as final output.
- Tool-call creation and updates, including edit/file-diff details where ACP
  provides them.
- Plan updates.
- Usage and session metadata.
- ACP session identifier and stop reason.
- Agent-provided error data that is safe and useful to the customer.

Updates should use the existing response-stream and Factory Session contracts.
Consumers outside the adapter must not switch on SDK structs or raw ACP method
names.

### Permissions

- When `skipPermissions` is true, answer ACP permission requests through the
  supported non-interactive approval option.
- When it is false, use the current invocation permission behavior available to
  the caller. If no safe interactive path exists, return a clear actionable
  failure rather than hanging.
- Cancellation while a permission request is pending must unblock the SDK call
  and reap the process.
- Do not persist an invocation-only `--skip-permissions` override back into the
  factory definition.

### Failures and diagnostics

Distinguish at least:

- unknown provider identity;
- known built-in whose executable is unavailable;
- invalid custom command or unsupported transport;
- process startup failure;
- initialize/version/capability negotiation failure;
- authentication-required response;
- unsupported agent-to-client filesystem or terminal request;
- malformed or out-of-order RPC traffic;
- prompt/session error;
- context cancellation or existing execution timeout;
- unexpected process exit;
- success response followed by teardown noise.

Every path must produce one terminal provider outcome. Diagnostics should name
the selected provider and retain safe stderr/error context without corrupting
the JSON-RPC stdout channel.

## CLI behavior

```text
you workers acp list
you workers acp add --name company-cursor --transport stdio --argument "cursor-agent acp"
you workers acp delete --name company-cursor
```

- `list` includes built-ins and configured integrations, their execution kind,
  command/transport where appropriate, and availability.
- `add` validates a canonical identity, stdio transport, and non-empty command;
  adding the same custom identity follows one explicit replace/error rule.
- `delete` removes operator configuration only. It must not delete built-in
  metadata from the catalog.
- Deleting a custom provider causes later workers that still reference it to
  fail provider selection clearly.
- Mutations are atomic and preserve unrelated operator settings.

## Implementation stories and TODOs

### 1. Expose ACP integrations through the Providers catalog

- [ ] Define provider-neutral catalog metadata for ACP integrations.
- [ ] Add the three built-in identities and executable availability checks.
- [ ] Merge operator-defined integrations using documented precedence.
- [ ] Prove exact catalog membership so accidental extra providers fail tests.

Acceptance criteria:

- Customers can distinguish known/available, known/unavailable, and unknown.
- Built-ins and custom entries resolve to the same execution contract.
- Catalog tests compare the exact expected set, not only `contains` assertions.

### 2. Persist and manage custom stdio integrations

- [ ] Decode/encode the ACP integration collection in operator settings.
- [ ] Implement service-owned list/add/delete operations and thin CLI bindings.
- [ ] Preserve unrelated configuration and report invalid/duplicate/delete
  cases consistently.

Acceptance criteria:

- Add, list, replace/delete behavior works through the compiled `you` CLI.
- Delete followed by `you run` fails if the deleted identity is still selected.
- Config round trips without adding ACP-specific timeout or permission fields.

### 3. Route authored `executorProvider` into the ACP adapter

- [ ] Preserve extensible lowercase provider identities through authored,
  mapped, compiled, and invocation-time worker shapes.
- [ ] Register resolved ACP integrations with the existing provider conductor.
- [ ] Keep `SCRIPT_WRAP` and omitted-provider compatibility behavior intact.

Acceptance criteria:

- The same ACP identity works from a custom factory, packaged factory, and
  JavaScript `agent.run`.
- Unknown provider failure happens before a misleading execution attempt.

### 4. Implement the SDK-backed stdio adapter

- [ ] Launch via the injected process edge and use `acp-go-sdk` for production
  RPC.
- [ ] Implement initialize, new session, prompt, update, permission, terminal,
  cancellation, and cleanup behavior.
- [ ] Map SDK messages into provider-neutral requests, progress, results, and
  failures at the adapter boundary.
- [ ] Fail unsupported client capabilities explicitly.

Acceptance criteria:

- A real ACP executable can complete a prompt through `you run`.
- Streaming output remains ordered and results in exactly one terminal result.
- Cancellation and all error paths reap the child process.
- No handwritten production JSON-RPC codec exists.

### 5. Preserve response-stream and session semantics

- [ ] Map message, reasoning, plan, tool lifecycle, file edit, usage, and session
  metadata updates.
- [ ] Retain the ACP session ID in the provider-neutral session reference.
- [ ] Verify terminal output, diagnostics, and event-stream projections.

Acceptance criteria:

- Terminal users see useful streaming progress and final content.
- Event consumers receive stable internal event contracts rather than raw ACP.
- Tool/update ordering and terminal precedence are deterministic.

### 6. Document and validate the customer workflow

- [ ] Add the canonical public provider guide under `docs/reference/`.
- [ ] Document built-ins, custom add/remove, config location, worker selection,
  packaged factory use, JavaScript use, permissions, and troubleshooting.
- [ ] Run a manual README-to-terminal acceptance pass with Cursor ACP.

Acceptance criteria:

- A customer can install/select Cursor ACP and complete a small test-writing
  task by following repository docs alone.
- The guide demonstrates prebuilt, custom, deletion/failure, packaged-factory,
  and JavaScript paths.

## Test strategy

### Boundary principle

Most tests should execute the real application through `you run` and observe it
until terminal completion. Direct event-stream inspection is reserved for
ordering, event shape, session metadata, or diagnostics that the terminal
cannot prove.

The main functional peer is a separate OS process connected through stdin and
stdout. It speaks raw newline-delimited JSON-RPC using JSON literals/maps and
the standard JSON package only. It deliberately does **not** import
`acp-go-sdk`, reuse the production peer, or construct SDK types. This protects
the tests from sharing the same encoding assumptions as production while still
testing the SDK over a real wire boundary.

The test should inject only true external effects through `edges.Edges`:

- executable lookup when a deterministic fake command is needed;
- process command creation so the application launches the test peer;
- filesystem/config roots needed for isolation.

It should not inject the provider service, ACP adapter, registry, RPC codec, or
response mapper.

### Golden protocol corpus

Copy the relevant examples from
`coder/acp-go-sdk/testdata/json_golden` into checked-in testdata. Record the
upstream repository path and revision in a manifest/README. Keep upstream
payloads byte-oriented; add local expected response-stream goldens separately.

```text
tests/functional/providers/acp/testdata/json_golden/
  README.md
  manifest.json
  upstream/
    initialize_request.json
    initialize_response.json
    new_session_request.json
    new_session_response.json
    prompt_request.json
    request_permission_request.json
    request_permission_response_selected.json
    session_update_agent_message_chunk.json
    session_update_agent_thought_chunk.json
    session_update_plan.json
    session_update_tool_call_edit.json
    session_update_tool_call_update_more_fields.json
  expected/
    response_stream.ndjson
```

The raw peer may parameterize IDs and paths, but should compare normalized wire
messages against these fixtures. Updating the SDK does not automatically
rewrite goldens; drift requires an explicit fixture update and review.

### Functional test file shape

```text
tests/functional/providers/acp/
  functional_rpc_peer_test.go          # independent raw stdio peer process
  golden_rpc_peer_test.go              # wire request/response fixture matching
  golden_fixture_test.go               # provenance and fixture integrity
  basic_factory_run_test.go            # ordinary `you run` happy path
  catalog_cli_test.go                  # built-ins plus add/list/delete
  catalog_cli_negative_test.go         # invalid, duplicate, unknown, unavailable
  acp_provider_events_test.go           # stream mapping and ordering
  acp_error_test.go                     # startup/RPC/session/process errors
  javascript_factory_run_test.go       # `agent.run` selection
  mixed_provider_factory_test.go       # ACP and compatibility executor coexist
  run_failure_diagnostics_test.go      # terminal customer diagnostics
  run_parameters_content_test.go       # cwd/model/content mapping
  run_permissions_test.go              # skipPermissions and pending cancellation
  run_unsupported_capabilities_test.go # filesystem/terminal request failures
```

Add focused owner-local unit tests only where they prove mapping or cleanup more
directly than a functional test. Functional behavior remains under
`tests/functional/providers/acp`, not beside the private adapter.

### Required scenarios

- [ ] Built-in catalog has exactly `cursor-acp`, `kiro-acp`, and
  `opencode-acp` for the ACP subset, with correct commands and availability.
- [ ] Add a custom integration, list it, use it through `you run`, delete it,
  and prove subsequent selection fails.
- [ ] Initialize/new-session/prompt sequence matches the upstream-derived wire
  goldens over real OS stdio.
- [ ] Message/reasoning/tool/plan updates map to the response-stream golden.
- [ ] Permission request with `skipPermissions: true` selects the expected
  option; false/no-safe-input returns a deterministic result rather than hangs.
- [ ] Invalid version, malformed JSON-RPC, RPC error, auth-required, unsupported
  client method, early EOF, non-zero exit, and cancellation all terminate.
- [ ] Working directory, prompt content, model, environment, and session ID are
  mapped correctly.
- [ ] A custom factory can ask Cursor ACP to add very small tests and run them.
- [ ] A packaged/prebuilt factory can select the same provider.
- [ ] JavaScript `agent.run` can select the same provider.
- [ ] ACP and script-wrap/other providers can run in the same factory without
  cross-routing output.

### Production-default composition regression

Functional tests that always inject a process-command factory can accidentally
prove only the override graph. Add a separate regression that constructs the
real process with `root.BuildProcess(Edges{})` (or the minimum genuinely empty
production edge bag), places a deterministic raw ACP executable on `PATH`, and
runs through the public CLI/process boundary.

- [ ] Prove a nil external command-factory edge resolves to the production
  `exec.Command` implementation before the ACP adapter is constructed.
- [ ] Keep the adapter's nil dependency check fail-closed; composition, not the
  adapter, owns production defaults.
- [ ] Exercise the compiled binary at least once so test-only injected edges do
  not mask missing production wiring.
- [ ] Assert exact provider catalog output so an over-inclusive registry cannot
  pass through partial `contains` checks.

### Coverage target

Target roughly 80-85% focused coverage for the new adapter and CLI behavior,
but treat behavioral scenario coverage as the gate. Coverage must include
negative and cleanup paths, not merely line execution through SDK success.

## Verification and delivery

- [ ] Run focused ACP unit and functional packages.
- [ ] Run the repository's functional target with the ACP lane enabled.
- [ ] Run `make docs-reference-smoke` after public provider documentation.
- [ ] Run `make verify-fast`, then the broader PR verification appropriate to
  provider/runtime changes.
- [ ] Manually run the compiled `you` binary against Cursor ACP following the
  README/provider guide from a clean temporary factory.
- [ ] Confirm no generated OpenAPI artifacts require updates; if public contract
  fragments change, regenerate them rather than editing generated files.
- [ ] Continue through terminal green CI, address all blocking review feedback,
  resolve conflicts, and verify the PR is actually merged. An open or merely
  approved PR is not completion.

## Manual acceptance script

1. Build the real `you` CLI.
2. Run `you workers acp list` and verify the exact built-in identities and
   availability.
3. Configure a minimal custom factory whose worker uses `cursor-acp`.
4. Ask the agent to add simple tests (for example, `Add`, `Reverse`, or
   `IsEven`) and run them; inspect the streamed output and terminal result.
5. Add a custom ACP identity pointing at Cursor, run it, delete it, and verify a
   later run fails provider selection.
6. Select `cursor-acp` in a packaged/prebuilt factory and complete another
   minimal task.
7. Select `cursor-acp` from JavaScript `agent.run` and complete another minimal
   task.
8. Confirm each created test exists and the underlying language test command
   passes independently of the agent's textual claim.

## Open follow-ups

- Interactive permission UX when `skipPermissions` is false.
- Session resume/persistence and explicit session selection.
- Additional ACP transports after stdio behavior is stable.
- Client filesystem and terminal capabilities if supported agents require
  them.
- Richer structured tool/diff presentation without leaking ACP SDK contracts.
- Authentication discovery/setup UX for unavailable or unauthenticated agents.
- Versioned process manifests if arbitrary command strings become insufficient.

