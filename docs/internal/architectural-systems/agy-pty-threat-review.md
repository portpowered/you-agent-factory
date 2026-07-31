# Threat review: Agy PTY boundary (Windows and POSIX)

| Field | Value |
| --- | --- |
| Status | Accepted |
| Story | S16 / `stream-b06-agy-integration-decision-002` |
| Date | 2026-07-14 (UTC) |
| Companion ADR | `docs/architecture/agy-pty-boundary.md` |
| Gates | Story 17+ Agy execution (`agy-pty-boundary.md` §Gating Story 17) |

This document is the security threat review for the minimal native Go Agy PTY
boundary approved in `agy-pty-boundary.md`. It constrains Story 17
implementation choices. This lane does **not** invoke or embed the upstream
Python bridge and does **not** add production Agy execution.

## Scope and trust boundaries

| Boundary | Trusted side | Untrusted side |
| --- | --- | --- |
| Factory configuration | Maintainer-authored factory config, worker definitions, orchestration policy | — |
| Work payload | Factory runtime validation and work-content policy | User-supplied prompt text passed through to the Agy CLI as an argv element or documented stdin attachment |
| Agy CLI child | Typed argv, supervised process group/job, bounded PTY capture | Agy binary behavior, terminal output content, auth prompts, and any child processes Agy spawns |
| Public response events | Cleaned final text after terminal cleaning (Story 17+) | Raw PTY bytes, ANSI/OSC sequences, spinner/repaint noise |

The PTY boundary runs inside the worker process. It must treat Agy argv fields,
workspace paths, inherited environment values, and captured terminal bytes as
**untrusted input** once they originate from work payloads or the child process.

## Threat catalog

| ID | Threat | Primary controls | Story 17 owner |
| --- | --- | --- | --- |
| T1 | Shell / argv injection via metacharacters or shell wrappers | Typed `[]string` argv only; no `sh -c`, `cmd /C`, or PowerShell `-Command` | Agy provider adapter + Story 003 fixtures |
| T2 | Workspace path traversal or attachment outside factory root | Normalize, clean, and validate paths before `Dir` or argv attachment; reject `..` escapes | Agy provider adapter + Story 003 fixtures |
| T3 | Environment inheritance leaking secrets or re-enabling interactive prompts | `commandenv.Build` precedence; explicit denylist for PTY-sensitive vars | Agy provider adapter |
| T4 | Terminal escape sequences in PTY output affecting downstream consumers | Bounded capture; terminal cleaning before any public payload; no raw PTY publish | PTY package + Story 18 cleaning corpus |
| T5 | Unbounded PTY capture exhausting memory or event buffers | Hard byte cap on internal capture buffer; truncate or fail closed per policy | PTY package |
| T6 | Hung or runaway child processes after idle/hard timeout | Idle and hard timers; supervised tree termination via `pkg/services/workers/process` | PTY package + process runner |
| T7 | Orphaned child processes after cancel, crash, or adapter failure | Job object (Windows) / process group (POSIX) cleanup; documented escape limits | `pkg/services/workers/process` |
| T8 | Unsupported platform silently using pipe IO | Fail closed with `UNSUPPORTED` or classified setup failure | PTY allocator |
| T9 | Python bridge invocation or embedding at runtime | ADR exclusion; no CPython dependency; hermetic tests without bridge | All layers |
| T10 | Publishing incremental spinner/repaint noise as response events | Final-only fidelity (`nativeStreaming=false`); internal PTY bytes only | Agy adapter kernel |

## T1 — Shell and argv injection

### Risk

Concatenating user or configuration values into a shell command string allows
shell metacharacter injection (`;`, `|`, `` ` ``, `$()`, newlines). Wrapping
argv in `/bin/sh -c`, `cmd.exe /C`, or `powershell -Command` reintroduces a
shell parser between the factory and the Agy binary.

### Required controls

1. **Typed argv only.** Build commands with `exec.Command(executable, args...)`
   where `executable` is the resolved Agy binary path and each `args[i]` is an
   independent string field. Never pass a single assembled command line to a
   shell interpreter.
2. **No shell indirection.** Forbidden patterns include:
   - `exec.Command("sh", "-c", joined)`
   - `exec.Command("bash", "-c", joined)`
   - `exec.Command("cmd", "/C", joined)`
   - `exec.Command("powershell", "-Command", joined)`
3. **Executable path resolution.** Resolve the configured Agy executable with
   `filepath.Clean` / `filepath.EvalSymlinks` where appropriate; reject directory
   paths and empty strings before spawn.
4. **Prompt and flag separation.** User prompt text must be a distinct argv
   element (or documented stdin attachment), not concatenated into a flag value
   that is later split on whitespace.
5. **Hermetic validation.** Story 003 fixtures must prove representative argv
   shapes are `[]string` slices with no shell wrapper (see
   `agy-pty-boundary.md` §Gating Story 17).

### Platform notes

| Platform | argv mechanism | Injection surface |
| --- | --- | --- |
| Windows | `CreateProcess` argv vector via Go `exec.Command` | Avoid `cmd.exe` parsing rules by not invoking `cmd.exe` |
| POSIX | `execve` argv array | Avoid `/bin/sh` by not invoking a shell |

## T2 — Workspace path attachment and normalization

### Risk

Attaching a workspace directory with `..` segments, absolute paths outside the
factory root, or symlink escapes can cause the Agy child to read or write files
outside the intended factory worktree.

### Required controls

1. **Normalize before use.** Apply `filepath.Clean` and `filepath.FromSlash` to
   configured workspace paths. Reject empty paths.
2. **Containment checks.** When the factory designates a workspace root, resolved
   paths must remain under that root (same pattern as
   `pkg/services/workers/worktree/paths.go` `cleanWorktreeName` — reject `..` traversal,
   reject absolute names where relative names are required).
3. **Process working directory.** Set `cmd.Dir` only after normalization and
   containment validation; never pass raw user strings.
4. **Argv path fields.** Any workspace path passed as an Agy flag value must be
   the same normalized absolute or root-relative path used for `cmd.Dir`, not a
   separate unvalidated copy.
5. **Hermetic fixtures.** Story 003 must include fixtures for valid paths,
   `..` rejection, and separator normalization on Windows and POSIX.

### Platform notes

| Platform | Consideration |
| --- | --- |
| Windows | `\\?\` prefixes, drive-relative paths, and case-insensitive comparison when checking root containment |
| POSIX | Symlink components under the workspace root; prefer `EvalSymlinks` on the resolved root before containment checks when feasible |

## T3 — Environment inheritance

### Risk

Inheriting the full process environment can propagate secrets (`AWS_*`,
`OPENAI_*`, provider tokens) to the Agy child, re-enable interactive prompts
(`GIT_TERMINAL_PROMPT`, `EDITOR`), or alter Agy behavior unpredictably.

### Required controls

1. **Centralized merge policy.** Use `pkg/services/providers/internal/services/execution/internal/provider/commandenv.Build`
   for provider subprocess environment assembly. Precedence: process environment,
   provider variables, then automation defaults (`GIT_TERMINAL_PROMPT=0`, etc.).
2. **No duplicate env builders.** The Agy adapter must not fork a parallel
   environment-merge path that bypasses `commandenv`.
3. **PTY-specific posture.** Story 17 must document any Agy-specific
   environment denylists or required vars (for example non-interactive flags)
   in the adapter; changes require threat-review update.
4. **No Python bridge env.** Do not set `PYTHONPATH`, `VIRTUAL_ENV`, or invoke
   `python -m agy_headless_bridge` to populate environment for the child.

## T4 — Terminal escape sequences

### Risk

PTY output can contain ANSI CSI/OSC sequences, title changes, alternate-screen
switches, or escape-driven repaints. Publishing raw bytes can corrupt dashboards,
log pipelines, or terminal emulators used by operators. Some sequences are
historically abused for social-engineering overlays (less relevant in headless
capture but still noise).

### Required controls

1. **Internal-only raw bytes.** PTY capture buffers are implementation-private
   until terminal cleaning runs.
2. **Cleaning before public emit.** Story 17 must not emit response events
   containing raw PTY bytes. Cleaned final text only (Story 18 may extend
   partial-timeout policy later).
3. **Cleaner scope.** Strip CSI/OSC, carriage-return repaint lines, spinner
   glyphs, and box-drawing noise per `agy-pty-boundary.md` maintained scope.
4. **No echo of control bytes in errors.** Failure messages must not include
   unfiltered capture snippets that contain escape sequences; truncate and strip
   when surfacing diagnostics.

## T5 — Output bounds

### Risk

A verbose or runaway TUI can produce unbounded PTY output, causing memory
pressure in the worker or oversized internal buffers.

### Required controls

1. **Hard byte cap.** Maintain a documented `maxCaptureBytes` on the internal
   PTY read buffer. Default and maximum values are fixed in Story 17
   implementation and recorded in the interface proposal (Story 003).
2. **Bounded reads.** Read loops must respect the cap; do not grow slices
   without limit.
3. **No unbounded public streaming.** Agy adapter declares final-only fidelity;
   incremental publish of PTY chunks is forbidden in Story 17.
4. **Policy on cap breach.** Classify as timeout or capacity failure per adapter
   kernel rules; do not publish partial raw capture.

## T6 — Timeouts

### Risk

Without idle and hard limits, a stuck Agy session holds PTY fds, worker slots,
and child processes indefinitely.

### Required controls

1. **Idle timeout.** Reset on new PTY bytes; cancel supervised tree on breach.
2. **Hard timeout.** Absolute wall-clock limit from spawn; cancel regardless of
   byte activity.
3. **Context propagation.** Timeouts must flow through `context.Context`
   cancellation into `pkg/services/workers/process` runner semantics.
4. **No partial public output on timeout in Story 17.** Partial-timeout publish
   is Story 18 scope only.

## T7 — Process-tree cleanup

### Risk

After success, failure, cancel, or timeout, remaining child processes can leak
workers, hold files, or continue consuming credentials.

### Required controls

1. **Reuse supervision semantics.** Align with `pkg/services/workers/process`:
   - **Windows:** job object with `KILL_ON_JOB_CLOSE`; terminate active
     processes before closing the handle.
   - **POSIX:** dedicated process group; SIGTERM, grace period, then SIGKILL.
2. **PTY fd lifecycle.** Close master/slave or ConPTY handles after cleanup
   starts; do not leak fds on error paths.
3. **Documented escape limits.** Cleanup targets the supervised group only.
   Detached children (`setsid`, `CREATE_BREAKAWAY_FROM_JOB`) may survive — same
   limits as `pkg/services/workers/process/doc.go`. Operators must not rely on cleanup
   to stop intentionally detached daemons.
4. **Cancel vs. exit ordering.** Capture exit metadata before cleanup mutates
   the process tree, consistent with existing runner behavior.

### Windows ConPTY-specific cleanup

| Step | Requirement |
| --- | --- |
| Cancel/timeout | Cancel context; signal job termination |
| ConPTY handles | Close pseudo-console and pipe handles after child termination attempt |
| Job object | Terminate active processes; close job handle to enforce `KILL_ON_JOB_CLOSE` |
| Failure mode | If ConPTY allocation fails at startup, fail closed before spawning Agy |

### POSIX PTY-specific cleanup

| Step | Requirement |
| --- | --- |
| Cancel/timeout | Cancel context; SIGTERM to process group, grace, SIGKILL |
| PTY fds | Close master and slave fds after child termination attempt |
| Controlling terminal | Follow platform PTY contract when setting controlling tty; avoid leaking session state to unrelated children |
| Failure mode | If `openpty` fails, fail closed with `UNSUPPORTED` or setup failure — no pipe fallback |

## T8 — Unsupported platforms

### Risk

Platforms without ConPTY or POSIX PTY APIs might tempt implementers to fall back
to pipe IO, defeating Agy TTY gating and hiding capability gaps.

### Required controls

1. **Fail closed.** Return explicit `UNSUPPORTED` capability or classified setup
   failure at allocation time.
2. **No silent pipe fallback.** Pipe redirection is not an approved degradation
   path (`agy-pty-boundary.md`).
3. **Build tags.** Platform files must compile only supported paths; unsupported
   OS builds return a typed error from allocator entry points.

## T9 — Python bridge and production execution exclusions

This threat review lane is specification only.

| Exclusion | Threat if violated |
| --- | --- |
| No Python bridge invocation | Supply-chain and runtime dependency on CPython; behavior drift from approved Go boundary |
| No Python embedding | Same as above; larger attack surface |
| No production Agy execution in S16 | Unreviewed execution code bypassing argv, bounds, and cleaning controls |

Story 17 may add execution only after this review, the ADR, and Story 003
interface fixtures are merged.

## T10 — Public response content policy

### Risk

Publishing raw terminal repaint or spinner frames as incremental response events
leaks noise, destabilizes client rendering, and bypasses cleaning.

### Required controls

1. **Final-only fidelity.** Agy adapter sets `nativeStreaming=false` per
   `stream-response-fix-plan.md` §8.6 (when present in tree).
2. **Cleaned text only.** Public `Content` fields contain terminal-cleaned text
   after Story 17 execution lands.
3. **Diagnostics.** Internal logs may record capture metrics (byte counts,
   duration) but must not log full prompt text or unfiltered PTY payloads at
   default verbosity.

## Windows vs POSIX security assumptions (summary)

| Topic | Windows (ConPTY) | POSIX (Linux/macOS) |
| --- | --- | --- |
| PTY allocation | Pseudo-console via ConPTY APIs | `openpty` master/slave pair |
| argv delivery | `CreateProcess` argv vector; no `cmd.exe` | `execve` argv array; no `/bin/sh` |
| Supervision | Job object | Process group |
| Cleanup | `TerminateJobObject`, close job handle | SIGTERM → grace → SIGKILL |
| Detached child escape | `CREATE_BREAKAWAY_FROM_JOB` | `setsid`, double-fork |
| Unsupported OS | Fail closed at allocator | Fail closed at allocator |
| Pipe fallback | **Forbidden** | **Forbidden** |

## Story 17 implementation checklist (security)

Story 17 **must** satisfy every item before merge:

- [ ] Spawn Agy with typed `[]string` argv and no shell wrapper (T1).
- [ ] Normalize and validate workspace paths before `cmd.Dir` and argv attachment (T2).
- [ ] Build environment through `commandenv.Build` (T3).
- [ ] Keep raw PTY bytes internal; emit cleaned final text only (T4, T10).
- [ ] Enforce `maxCaptureBytes` on capture loops (T5).
- [ ] Implement idle and hard timeouts with context cancellation (T6).
- [ ] Reuse `pkg/services/workers/process` supervision and cleanup semantics (T7).
- [ ] Fail closed on unsupported platforms; no pipe fallback (T8).
- [ ] Do not invoke or embed the Python bridge (T9).
- [ ] Prove argv and path fixtures from Story 003 in CI without an Agy binary.

## Review against backend standards

Checked against `docs/internal/standards/code/general-backend-standards.md`:

| Standard theme | How this review constrains implementation |
| --- | --- |
| Package boundaries | PTY allocation in dedicated `pkg/services/workers/` package; adapter owns argv/paths; `process` owns tree cleanup |
| Pure vs IO logic | Terminal cleaning and argv/path normalization are pure and unit-testable (Story 003) |
| Explicit timeouts and cancellation | T6 requires context-driven idle/hard limits |
| Observable failure handling | Unsupported platform, cap breach, and timeout failures are classified, not silent |
| Test evidence | Hermetic fixtures without live Agy or Python bridge |

## Related documents

| Document | Role |
| --- | --- |
| `docs/architecture/agy-pty-boundary.md` | ADR — scope, platforms, exclusions |
| Story 003 interface proposal (`agy-pty-interface.md`, `pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty`) | Mock seams, `maxCaptureBytes`, fixture locations |
| `pkg/services/workers/process/doc.go` | Supervision and cleanup escape documentation |
| `pkg/services/providers/internal/services/execution/internal/provider/commandenv/environment.go` | Environment merge policy |
| `pkg/services/workers/worktree/paths.go` | Path normalization reference for workspace containment |


