# Provider catalog support evidence

The provider manifests are publication metadata, not live readiness results.
`technicalSupportLevel` records this repository's integration posture;
`implementationAvailability` records whether integration code ships here; and
capabilities record only the maximum behavior demonstrated by the cited code
or tests. An executable name is a credential-free discovery prerequisite, not
evidence that the executable is installed.

## Posture policy

- `production` is reserved for the default, built-in provider path with direct
  runner policy and structured adapter coverage.
- `experimental` identifies implemented provider paths that have focused
  behavior or adapter tests but are not the default production path.
- `not-supported` identifies an implementation that is present but is not in
  the selectable built-in or CLI-provider availability catalogs.
- `bundled` means the integration code is included in this repository. It does
  not mean its external executable is installed, authenticated, or ready.

## Evidence by provider

| Provider | Support and availability evidence | Discovery evidence | Execution-capability evidence | Response-fidelity evidence |
| --- | --- | --- | --- | --- |
| `agy` | The adapter is bundled under `pkg/services/workers/provider/agy`, but Agy is absent from `builtInRunnerStatus` and `registeredCLIProviders`; it is therefore conservatively `not-supported` rather than described as runnable. | `pkg/services/workers/provider/agy/adapter.go` defaults to the `agy` executable and resolves it through an injected executable locator. | `pkg/services/workers/runner_policy.go` declares prompt submission, tool execution, session resume, and working-directory support for `RunnerIDAgy`. | `pkg/services/workers/provider/agy/adapter.go` declares final-only message snapshots and no native streaming. |
| `claude` | `pkg/services/workers/provider/structured/executor.go` bundles the adapter and `pkg/services/workers/cliprovider/cli_provider_registry.go` registers the CLI, but it is not the default provider path, so its posture is `experimental`. | The CLI registry maps the identity to the `claude` command. | `pkg/services/workers/provider/claude/adapter.go` constructs prompt, session, worktree, tool-enabled, and working-directory execution. | `pkg/services/workers/provider/claude/adapter.go` declares native message streaming, snapshots, correlated tool lifecycle/output, and stable item IDs; adapter tests exercise those declarations. |
| `codex` | `pkg/services/workers/runner_policy.go` makes Codex the default runner and `pkg/services/workers/provider/structured/executor.go` bundles its structured adapter; this is the sole `production` entry. | The CLI registry and runner prerequisite checks map the provider to the `codex` command. | `pkg/services/workers/runner_policy.go` declares image, session, structured-output, working-directory, worktree, prompt, and tool capabilities. | `pkg/services/workers/provider/codex/adapter.go` declares native snapshots, reasoning summaries, tool lifecycle/output, and stable item IDs. |
| `cursor` | `pkg/services/workers/runner_registry.go` and the CLI-provider registry include Cursor, while `pkg/services/workers/provider/cursor` supplies parsing and failure behavior; the non-default path is `experimental`. | `pkg/services/models/provider_contract.go` and the CLI registry map canonical publication ID `cursor` to the runtime executable identity `agent`. | `pkg/services/workers/runner_policy.go` declares prompt, tool, session-resume, and working-directory support. | Cursor response-decoder and stream tests under `pkg/services/workers/provider/cursor` demonstrate streamed snapshots, correlated tool output, usage, and stable response-item identity. |
| `gemini` | Gemini is a selectable built-in and CLI provider with focused behavior/failure tests, but is not the default provider path; it is `experimental`. | The CLI registry and runner prerequisite checks map the provider to the `gemini` command. | `pkg/services/workers/runner_policy.go` declares only the prompt and tool baseline; `provider_behavior.go` explicitly rejects the optional capabilities published as false. | The legacy result path produces an authoritative final message only; no structured streaming capability is claimed. |
| `kiro` | Kiro is a selectable built-in and CLI provider with focused behavior/failure tests, but is not the default provider path; it is `experimental`. | `pkg/services/models/provider_contract.go` and the CLI registry map canonical publication ID `kiro` to executable identity `kiro-cli`. | `pkg/services/workers/runner_policy.go` declares prompt, tool, and session-resume support and rejects the other optional capabilities. | The legacy result path produces an authoritative final message only; no structured streaming capability is claimed. |
| `opencode` | OpenCode is a selectable built-in and CLI provider with a bundled negotiated adapter, but is not the default provider path; it is `experimental`. | `pkg/services/workers/provider/adapter/opencode/discovery.go` resolves and fingerprints the `opencode` executable before bounded capability discovery. | `pkg/services/workers/runner_policy.go` declares prompt, tool, session-resume, and working-directory support. | `pkg/services/workers/provider/adapter/opencode/capability.go` proves that the maximum structured mode provides native message snapshots and stable item IDs, while unsupported installations downgrade to final-only output. |
| `pi` | Pi is a selectable built-in and CLI provider with a bundled structured adapter, but is not the default provider path; it is `experimental`. | The CLI registry and adapter map the provider to the `pi` command. | `pkg/services/workers/runner_policy.go` declares prompt, tool, session-resume, structured-output, and working-directory support. | `pkg/services/workers/provider/pi/adapter.go` declares native message deltas/snapshots, reasoning summaries, tool lifecycle/output, and stable item IDs; focused adapter tests exercise the profile. |

The manifests intentionally claim no credentials, environment values, endpoint
addresses, machine paths, pricing, current installation state, authentication
state, or live readiness.
