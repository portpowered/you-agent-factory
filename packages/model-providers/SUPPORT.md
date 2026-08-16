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
| `antigravity` | The Antigravity adapter and runner registration are bundled as a selectable built-in. | The provider resolves the external `agy` executable through the injected executable locator. | Runner policy declares prompt submission, tool execution, session resume, and working-directory support. | The adapter declares final-only message snapshots and no native streaming. |
| `claude` | `pkg/services/providers/internal/services/execution/internal/adapters/claude/` bundles the native adapter; it is not the default provider path, so its posture is `experimental`. | The Providers catalog maps the identity to the `claude` command. | `pkg/services/providers/internal/services/execution/internal/adapters/claude/` constructs prompt, session, worktree, tool-enabled, and working-directory execution. | The native Claude adapter and its tests cover message streaming, snapshots, correlated tool lifecycle/output, and stable item IDs. |
| `codex` | `pkg/services/providers/internal/services/execution/internal/adapters/codex/` bundles the native adapter; this is the sole `production` entry. | The Providers catalog and prerequisite checks map the identity to the `codex` command. | `pkg/services/workers/runner_policy_contracts.go` declares image, session, structured-output, working-directory, worktree, prompt, and tool capabilities. | The native Codex adapter and its tests cover snapshots, reasoning summaries, tool lifecycle/output, stable item IDs, file changes, plans, and usage. |

The manifests intentionally claim no credentials, environment values, endpoint
addresses, machine paths, pricing, current installation state, authentication
state, or live readiness.
