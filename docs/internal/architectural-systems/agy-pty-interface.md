# Go PTY/process interface proposal: Agy headless boundary

| Field | Value |
| --- | --- |
| Status | Accepted |
| Story | S16 / `stream-b06-agy-integration-decision-003` |
| Date | 2026-07-14 (UTC) |
| Companion ADR | `docs/architecture/agy-pty-boundary.md` |
| Threat review | `docs/architecture/agy-pty-threat-review.md` |
| Packages | `pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty` (policy and port), `pkg/platform/pty` (native adapter) |
| Gates | Story 17+ Agy execution |

This document proposes the mockable Go PTY/process seams for the minimal native
Agy headless boundary. It is specification only: **production Agy headless
execution remains out of scope until Story 17** consumes these interfaces. This
lane does not invoke or embed the upstream Python bridge and does not require an
installed Agy binary for hermetic unit tests.

## Package layout

| File / path | Responsibility |
| --- | --- |
| `types.go` | `PTYAllocator`, `PTYSession`, low-level `Host`, `ProcessLaunch`, `SessionConfig`, `SessionResult` |
| `limits.go` | `DefaultMaxCaptureBytes`, `MaxMaxCaptureBytes`, idle/hard timeout defaults |
| `argv.go` | Typed `BuildArgv`, `ValidateArgv`, `RejectShellWrapper` (T1) |
| `workspace.go` | `ResolveWorkspaceDir` containment normalization (T2) |
| `cleaner.go` | Pure `CleanTerminal` seam for Story 17/18 cleaning |
| `mock.go` | `MockAllocator`, `MockSession` for hermetic tests |
| `testdata/argv_fixtures.json` | Hermetic argv corpus |
| `testdata/workspace_fixtures.json` | Hermetic workspace path corpus |

Wire selects a policy-free native adapter under `pkg/platform/pty`:

| Build tag | Implementation |
| --- | --- |
| `windows` | `WindowsHost` — ConPTY handles and attached-process mechanics |
| `linux,darwin` | `POSIXHost` — `openpty` handles and attached-process mechanics |
| unsupported | `ErrUnsupportedPlatform` — fail closed, no pipe fallback |

## Core interfaces

### `PTYAllocator`

Allocates one platform PTY for a supervised Agy child:

```go
type PTYAllocator interface {
    Allocate(ctx context.Context, launch ProcessLaunch, cfg SessionConfig) (PTYSession, error)
}
```

- **Windows:** the injected `WindowsHost` creates a pseudo-console pair and
  attaches the child so TTY checks succeed (`agy-pty-boundary.md` §Windows).
- **POSIX:** the injected `POSIXHost` opens a master/slave pair and attaches the
  child to the slave TTY (`agy-pty-boundary.md` §POSIX).
- **Tests:** `MockAllocator` records `ProcessLaunch` and returns
  `MockSession` without real ConPTY/PTY syscalls.

### `PTYSession`

Mockable seam for bounded capture, timeout signaling, and cleanup:

```go
type PTYSession interface {
    Run(ctx context.Context) (SessionResult, error)
    Close() error
}
```

Story 17 `Run` responsibilities:

1. Spawn the child with typed `ProcessLaunch.Argv` (no shell wrapper).
2. Read PTY bytes into an internal buffer capped by `SessionConfig.MaxCaptureBytes`.
3. Enforce `IdleTimeout` (no new bytes) and `HardTimeout` (wall clock).
4. On exit, timeout, or cancel, terminate the supervised process tree through
   `pkg/services/workers/process` semantics, then close PTY handles.
5. Return `SessionResult` with raw bytes and `CleanTerminal` output.

`MockSession` lets unit tests assert launch metadata, config limits, and
predetermined capture without an Agy binary.

### `Host`

`Host` is the exact external-effect port owned beside the Workers PTY
algorithm. `Allocate` returns an opaque `HostPTY`; `Start` returns only a raw
reader and supervised `HostProcess`. Platform implements native handle and
process mechanics, while Workers retains argv validation, limit normalization,
capture, timeout, cleaning, result shaping, and cleanup ordering. Wire is the
only production selector of `platform/pty.NewHost`.

### `ProcessLaunch`

Typed subprocess description passed to allocation:

| Field | Rule |
| --- | --- |
| `Executable` | Resolved Agy binary path after `filepath.Clean` |
| `Argv` | Full argv slice from `BuildArgv`; passed to `exec.Command` directly |
| `WorkDir` | Output of `ResolveWorkspaceDir` |
| `Env` | Output of `commandenv.Build`; no parallel env builder |

### `SessionConfig`

| Field | Default | Ceiling |
| --- | --- | --- |
| `MaxCaptureBytes` | 4 MiB (`DefaultMaxCaptureBytes`) | 16 MiB (`MaxMaxCaptureBytes`) |
| `IdleTimeout` | 30s | Story 17 factory config |
| `HardTimeout` | 10m | Story 17 factory config |

## Pure seams (T1 / T2)

### Typed argv (`argv.go`)

`ArgvSpec` carries executable, subcommand, flags, and prompt as **separate
fields**. `BuildArgv` returns a `[]string` slice suitable for
`exec.Command(executable, args...)`.

Controls (threat review T1):

- Prompt is always a distinct argv element, including when it contains shell
  metacharacters (`;`, `|`, `` ` ``).
- `RejectShellWrapper` forbids `sh -c`, `cmd /C`, `powershell -Command`, and
  equivalent shell-string indirection.
- `ValidateArgv` is the single entry point Story 17 calls before spawn.

Hermetic corpus: `testdata/argv_fixtures.json`, loaded by `LoadArgvFixtures()`.

### Workspace paths (`workspace.go`)

`ResolveWorkspaceDir(factoryRoot, rawPath)`:

1. Applies `filepath.Clean` and `filepath.FromSlash`.
2. Rejects `..` traversal for relative inputs.
3. Joins relative paths under `factoryRoot`.
4. Verifies containment with `filepath.Rel` (same pattern as
   `pkg/config/portable_bundled_files.go`).

Controls (threat review T2):

- One normalized path is used for both `cmd.Dir` and argv path fields.
- Absolute paths outside `factoryRoot` are rejected.

Hermetic corpus: `testdata/workspace_fixtures.json`, loaded by
`LoadWorkspaceFixtures()`. Tests substitute `t.TempDir()` for
`FACTORY_ROOT` placeholders.

## Terminal cleaning seam

`CleanTerminal(raw []byte) string` is a pure function over captured PTY bytes.
Story 17 calls it before any public response emit. Story 18 may extend the
cleaning corpus for partial-timeout policy.

Current scope (proposal baseline):

- Strip ANSI CSI (`ESC [` …) and OSC (`ESC ]` … BEL) sequences.
- Collapse carriage-return repaint lines to the final visible segment.
- Drop blank lines after stripping.

Raw PTY bytes remain internal (T4, T10).

## Mock substitution matrix

| Seam | Production (Story 17) | Hermetic test |
| --- | --- | --- |
| Native PTY/process effect | Wire-selected `platform/pty.NewHost` | injected `Host` fake |
| Workers session policy | `agypty.Allocator` / real `PTYSession.Run` | `MockAllocator` / `MockSession` |
| Capture / timeout / cleanup | Real `PTYSession.Run` | `MockSession` with fixed `SessionResult` |
| argv construction | `BuildArgv` + `ValidateArgv` | `testdata/argv_fixtures.json` |
| Workspace attachment | `ResolveWorkspaceDir` | `testdata/workspace_fixtures.json` |
| Terminal cleaning | `CleanTerminal` | Direct unit tests on byte corpora |
| Process supervision | `pkg/services/workers/process.ExecCommandRunner` | Existing process package tests |

## Story 17 consumption checklist

Story 17 **must**:

- [ ] Implement `PTYAllocator` / `PTYSession` for Windows ConPTY and POSIX PTY.
- [ ] Call `BuildArgv`, `ValidateArgv`, and `ResolveWorkspaceDir` before spawn.
- [ ] Honor `DefaultMaxCaptureBytes` and timeout defaults unless config overrides
      within `MaxMaxCaptureBytes`.
- [ ] Reuse `pkg/services/workers/process` for process-tree cleanup (T7).
- [ ] Build environment through `commandenv.Build` (T3).
- [ ] Keep `MockAllocator` / fixture tests passing in CI without an Agy binary.

Story 17 **must not**:

- Invoke or embed the Python bridge (T9).
- Use shell-string command construction (T1).
- Fall back to pipe IO on unsupported platforms (T8).
- Publish raw PTY bytes as public response content (T4, T10).

## Explicit exclusions

Same exclusions as `agy-pty-boundary.md`:

- Production Agy execution in this lane (Story 001–003 are specification only).
- Python-bridge invocation or embedding.
- Shell-string command construction.
- Incremental native streaming of spinner/repaint frames.

## Related documents

| Document | Role |
| --- | --- |
| `agy-pty-boundary.md` | ADR — scope, platforms, gating |
| `agy-pty-threat-review.md` | T1–T10 controls and Story 17 security checklist |
| `pkg/services/workers/process/doc.go` | Supervision and cleanup escape documentation |
| `pkg/services/providers/internal/services/execution/internal/provider/commandenv/environment.go` | Environment merge policy |
| `pkg/services/workers/worktree/paths.go` | Relative name normalization reference |


