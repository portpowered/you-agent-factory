# ADR: Minimal native Go Agy PTY boundary

| Field | Value |
| --- | --- |
| Status | Accepted |
| Story | S16 / `stream-b06-agy-integration-decision-001` |
| Date | 2026-07-14 (UTC) |
| Gates | Story 17+ Agy execution (`stream-response-fix-plan.md` §8.6, Story 17) |

This architecture decision record fixes the native Go boundary for Agy headless
provider support. It is the authoritative scope contract for later execution
work. Customer-facing vocabulary remains `Provider Session` and response events;
see `docs/architecture/data-model.md`.

## Context

Agy (Antigravity CLI) gates useful output on TTY detection. Ordinary piped
subprocess IO is insufficient for headless factory execution. The upstream
reference implementation [`rhishi99/agy-headless-bridge`](https://github.com/rhishi99/agy-headless-bridge)
is a Python package that allocates a PTY (Windows ConPTY or POSIX `pty`),
captures bounded raw terminal output, cleans ANSI/TUI noise, enforces idle and
hard timeouts, and tears down the process tree.

The stream-response program (`docs/temp/projects/stream-responses/stream-response-fix-plan.md`
§8.6) requires a **minimum native Go port** of that behavior for final-only Agy
provider integration. Maintainers need a fixed build-vs-adapt decision,
supported-platform scope, license posture, and explicit exclusions before
Story 17 lands production execution code.

## Decision

**Build:** infinite-you owns a minimal native Go implementation of the Agy
headless PTY boundary inside the worker/provider stack.

**Do not:**

- vendor or submodule the full upstream repository,
- invoke the upstream Python package at runtime,
- embed CPython or ship the Python bridge as a runtime dependency, or
- shell out to a shell interpreter with a concatenated command string.

The Go boundary adapts **only** the behaviors listed under [Maintained behavior
scope](#maintained-behavior-scope). Upstream Python sources are reference
material for behavioral equivalence, not a code-import source of truth.

Implementation ownership:

| Concern | Owner |
| --- | --- |
| PTY allocation, capture, cleaning, timeout, cleanup | new Agy-owned Go package under `pkg/services/workers/` (exact path chosen in Story 17; interface proposal in Story 003) |
| Typed argv construction and workspace attachment | Agy provider adapter (Story 17) |
| Process-tree supervision semantics | align with `pkg/services/workers/process` job-object (Windows) and process-group (POSIX) patterns |
| Final-only response events and failure classification | `pkg/services/workers/provider` adapter kernel (Story 17+) |

## Maintained behavior scope

The approved native Go port maintains **only** these headless-bridge behaviors:

1. **Binary discovery** — resolve the configured Agy executable path; surface
   distinct missing-executable failures (execution detail in Story 17).
2. **PTY allocation** — allocate an interactive terminal for the child process:
   - **Windows:** ConPTY pseudo-console pair (not plain pipe redirection).
   - **POSIX:** openpty/master-slave pair with the child attached to the slave
     TTY (not plain pipe redirection).
3. **Bounded capture** — read raw PTY output into an internal buffer with a
   documented maximum byte bound; never publish unbounded terminal streams as
   public response events.
4. **Terminal cleaning** — pure functions that strip ANSI CSI/OSC sequences,
   carriage-return repaint lines, spinner glyphs, and box-drawing noise from
   captured bytes before any public message payload (cleaner corpus in Story
   18).
5. **Idle and hard timeout** — enforce configured idle (no new PTY bytes) and
   overall hard limits; cancel the supervised process tree on breach.
6. **Process cleanup** — on success, failure, cancel, or timeout, terminate the
   supervised child process tree using the same ownership model as
   `pkg/services/workers/process` (Windows job object with `KILL_ON_JOB_CLOSE`, POSIX
   process-group SIGTERM then SIGKILL grace).

**Out of scope for this boundary** (upstream or adjacent behaviors that must
not be imported as part of the minimal port):

- MCP server wiring, Gemini model routing, or Antigravity auth flows beyond
  what the Agy CLI already exposes through argv and environment,
- Python packaging, `pip install`, or PyPI distribution mechanics,
- Interactive resize/input forwarding beyond what headless execution requires,
- Publishing spinner/repaint chunks as incremental response events (Agy remains
  `nativeStreaming=false`, final-only fidelity),
- Partial-timeout public output policy (Story 18).

## Supported operating systems

Supported platforms match the repository's backend matrix: **Linux, macOS, and
Windows** on amd64 and arm64 where the factory already builds and tests.

| Platform | PTY mechanism | Support posture |
| --- | --- | --- |
| Windows | ConPTY via Go `x/sys/windows` or a maintained ConPTY helper | **Supported** — primary reference platform for upstream headless bridge |
| Linux | POSIX `openpty` / `pty` master-slave | **Supported** — same class as existing `pkg/services/workers/process` POSIX builds |
| macOS | POSIX `openpty` / `pty` master-slave | **Supported** — same class as Linux POSIX path |

**Unsupported platforms** (BSD variants, 32-bit targets, or OS builds without
ConPTY/POSIX PTY APIs) must fail closed with an explicit `UNSUPPORTED`
capability or classified setup failure. They must not silently fall back to pipe
IO for Agy execution.

Upstream marks POSIX as beta; infinite-you still documents POSIX as supported
because the factory already ships POSIX worker process supervision. Story 17
must prove POSIX PTY allocation with mocked and targeted integration tests
before release.

## Windows ConPTY vs POSIX PTY responsibilities

### Windows (ConPTY)

- Create a pseudo-console and attach the Agy child to it so `isatty` checks
  succeed.
- Read stdout-equivalent PTY output from the pseudo-console input pipe (ConPTY
  host side).
- Supervise the child through a Windows job object consistent with
  `pkg/services/workers/process/command_process_windows.go`.
- On cleanup, terminate active job processes before closing the job handle.
- Security assumption: argv is passed as a Windows command line built from typed
  `[]string` fields (`exec.Command(name, arg...)`), never a shell string passed
  to `cmd.exe` or PowerShell.

### POSIX (Linux and macOS)

- Allocate master/slave pair; set slave as controlling terminal for the child
  session when required by the platform PTY contract.
- Read captured bytes from the master fd with non-blocking or polled reads
  bounded by the capture limit.
- Supervise the child through a dedicated process group consistent with
  `pkg/services/workers/process/command_process_unix.go`.
- On cleanup, signal the process group (SIGTERM, grace, SIGKILL) before closing
  PTY fds.
- Security assumption: argv is passed as `exec.Command(name, arg...)` with no
  `/bin/sh -c` wrapper.

Detailed threat controls (argv injection, workspace paths, environment
inheritance, escape sequences, bounds, timeouts, cleanup edge cases) are
specified in `docs/architecture/agy-pty-threat-review.md` (Story 002). Mock
seams and interface types are specified in the Go PTY/process interface proposal
(Story 003).

## Upstream license and required notices

| Item | Finding |
| --- | --- |
| Upstream repository | `https://github.com/rhishi99/agy-headless-bridge` |
| Upstream license | **MIT License** (Copyright (c) 2026 agy-headless-bridge contributors) |
| Compatibility with infinite-you | **Compatible** — MIT permits use, modification, and distribution in this repository subject to notice preservation |
| Required notices | Retain the upstream MIT copyright and permission notice in `NOTICE` or `THIRD_PARTY_NOTICES` (or equivalent repository notices location) when Story 17 lands adapted behavior; cite upstream as the behavioral reference for the minimal port |
| Upgrade ownership | infinite-you maintainers own the Go implementation lifecycle, security fixes, and platform updates; upstream version bumps inform behavioral parity reviews but do not auto-import code |

No copyleft obligations apply to the adapted minimal port. Do not remove upstream
attribution when documenting adapted algorithms or test corpora derived from
upstream examples.

## Explicit exclusions

This lane and the approved boundary **exclude**:

| Exclusion | Rationale |
| --- | --- |
| Production Agy execution | Story 001–003 are specification only; Story 17+ implements execution |
| Python-bridge invocation or embedding | Runtime must not depend on CPython or `agy-headless-bridge` PyPI package |
| Shell-string command construction | Commands are `[]string` argv only; no `sh -c`, `cmd /C`, or PowerShell `-Command` string assembly |
| Unrelated upstream behavior | MCP tooling, packaging scripts, CLI UX unrelated to PTY capture/cleaning, and full-repo vendoring stay out of scope |
| Incremental native streaming | Agy adapter declares final-only fidelity; PTY bytes are internal until cleaned final text is emitted |
| Pipe-only fallback for Agy | Defeats TTY gating; classified as unsupported setup, not a silent fallback |

## Gating Story 17 and later work

Story 17 (**Execute Agy headlessly through the approved PTY/cleaning boundary**)
and Story 18 (**Preserve bounded partial output on timeout**) **must not** merge
until all of the following are true:

1. This ADR is merged and marked Accepted.
2. The Windows/POSIX threat review (Story 002) is published and reviewed
   against `docs/internal/standards/code/general-backend-standards.md`.
3. The Go PTY/process interface proposal with hermetic argv and path fixtures
   (Story 003) is published and tests pass without an installed Agy binary or
   Python bridge.
4. Batch B06 loopback (`stream-b06-boundary-loopback`) accepts both the Pi
   readiness gate and this Agy decision gate.

Story 17 implementation checklist (derived from this ADR):

- [ ] Implement only the [maintained behavior scope](#maintained-behavior-scope).
- [ ] Use typed argv and documented workspace-path normalization rules from the
  Story 003 interface proposal.
- [ ] Allocate ConPTY on Windows and POSIX PTY on Linux/macOS — no pipe
  fallback.
- [ ] Reuse or extend `pkg/services/workers/process` supervision semantics; do not
  duplicate ad hoc kill logic in the adapter.
- [ ] Add `NOTICE` / third-party attribution for upstream MIT material when code
  lands.
- [ ] Emit final-only response events per `stream-response-fix-plan.md` §8.6.

Story 18 may add partial-timeout public output only after Story 17 final-only
execution is approved.

## Consequences

### Positive

- Maintainers can implement and review Agy execution without reading Python
  sources or debating scope ad hoc.
- Windows and POSIX responsibilities are fixed before code lands.
- License posture is explicit before attribution-bearing code merges.
- Mock seams (Story 003) can be tested hermetically in CI without external
  binaries.

### Negative / costs

- infinite-you maintains PTY platform code and must track OS API changes
  (ConPTY availability, POSIX `openpty` behavior).
- Behavioral parity with upstream is a maintainer responsibility, not a
  package upgrade.
- POSIX PTY support requires explicit test investment even though upstream
  labels POSIX beta.

## Related documents

| Document | Story | Status |
| --- | --- | --- |
| Windows/POSIX threat review (`agy-pty-threat-review.md`) | `stream-b06-agy-integration-decision-002` | Accepted |
| Go PTY/process interface proposal and fixtures (`agy-pty-interface.md`) | `stream-b06-agy-integration-decision-003` | Accepted |
| Stream-response program plan | `docs/temp/projects/stream-responses/stream-response-fix-plan.md` §8.6 | Reference |
| Worker process supervision | `pkg/services/workers/process/doc.go` | Existing |
| Provider subprocess environment policy | `pkg/services/workers/provider/commandenv/environment.go` | Existing |
