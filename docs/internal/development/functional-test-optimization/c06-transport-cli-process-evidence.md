# C06 transport CLI-process characterization evidence

- Stories: `functional-test-optimization-c06-transport-cli-process-001` through
  `functional-test-optimization-c06-transport-cli-process-003`
- Parent behavior: `BEH-001` — CLI-process behavior remains at the boundary it
  actually proves while eligible `Process.Execute` scenarios are prepared for
  bounded reuse.
- Status: GATE-CHAR passed; the review correction below relocates built-binary
  witnesses to the integration lane and supersedes the pre-correction lane
  notes.
- Recorded at UTC: `2026-08-28T07:56:06.2303024Z`
- Recorded commit: `67710223e327d02c0de93a6ad826c754fe5c1702`
- Characterized package: `tests/functional/transport/cli/process`
- Current c06 lanes: `tests/functional/transport/cli/process` for
  `Process.Execute`/process-free cases and `tests/integration/transport/cli/process`
  for built-binary/OS-process cases.
- Top-level test inventory: 15
- Classification reconciliation: `15 = 12 isolated-with-reason + 2
  shareable-with-mock + 1 shareable/process-free`

This is a characterization artifact, not a source-scanning test or a runtime
contract. The rows below were reconciled against the pre-change test bodies and
the call paths in `internal/builtcliacceptance/harness.go`. The review-correction
section at the end is authoritative for current source placement and verification;
it does not relabel the historical in-process characterization as executable
evidence.

## Environment and GATE-CHAR result

- Go: `go version go1.25.0 windows/amd64`
- OS/architecture: `windows/amd64`
- Logical CPUs: `24`
- GOMOD: `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\functional-test-optimization-c06-transport-cli-process\go.mod`
- GOCACHE: `C:\Users\andre\AppData\Local\go-build`
- GOFLAGS: empty
- GOAMD64: `v1`
- Exact command: `go test -count=1 -timeout=10m -json ./tests/functional/transport/cli/process`
- Exit code: `0`
- Package result: `pass`, elapsed `19.22s`
- Top-level result: 12 pass, 0 fail, 3 explicit platform skips
- Terminal event detail: 25 passing events including subtests, 0 failing
  events, and 3 skipped top-level tests
- Unparsed JSON output lines: `0`
- Skipped tests:
  `TestCLIContextCancellationStopsExternalWork`,
  `TestCLIContextCancellationEmitsNoSuccessResult`,
  `TestBuiltCLIInterruptedResponseStreamExitCode`

The three skips are the explicit `runtime.GOOS == "windows"` branches for
Unix `/bin/sh` cancellation and `os.Interrupt` behavior. The Windows-specific
helper file compiled as part of this package. This run does not claim the Unix
properties.

## Boundary vocabulary

The current harness has three materially different boundaries:

| Label | Current implementation | What it proves |
| --- | --- | --- |
| `built-child` | `exec.Command` of a temporary binary built from `./cmd/factory` | Built-artifact argument parsing, OS streams, OS exit status, signal delivery, and descendant cleanup where the body asserts them. |
| `in-process Process.Execute` | `support.BuildProcess` or `builtcliacceptance.Command.Run` -> `root.BuildProcess` -> `Process.Execute` | Production root wiring, CLI parsing, public stream mapping, controlled edge behavior, and process lifecycle inside the test process. It is not executable/OS-process evidence. |
| `process-free` | Pure `parseContextCancellationProviderPID` calls | Parser acceptance/rejection only; it starts no root or OS process. |

`builtcliacceptance.Session.Run` is currently an in-process call despite the
helper and several test comments referring to a “built you CLI”. Its call path
is `Session.Run` -> `Session.run` -> `Command.Run` -> `root.BuildProcess` ->
`Process.Execute` (`internal/builtcliacceptance/harness.go:302-333,
132-172`). Only `TestBuiltCLIInterruptedResponseStreamExitCode` currently
builds and starts the `you` executable. The retained executable rows below are
classified by the property their bodies intend to protect; their current
fidelity gap is recorded rather than relabeled as executable evidence.

## Body-backed 15-test classification

“Application starts” means one current root invocation through
`Process.Execute`, or one started built CLI child. It excludes the Go compiler,
provider/mock-worker descendants, temporary TCP-port listener, and goroutines.
A failed initialization invocation still counts because it builds and executes
the application root. `t.Run` does not count by itself; each `Session.Run`,
`Command.Start`, or direct `Process.Execute` does.

| Test and source | Classification | Process property proved by the body | Current boundary and dependency fidelity | Cleanup owner | Starts Unix / Windows |
| --- | --- | --- | --- | --- | ---: |
| `TestACPServeCancellationPreservesContextCanceledIdentityThroughProcess` (`acp_shutdown_test.go:25`) | `isolated-with-reason` | A blocked ACP stdin read is unblocked by cancellation; `context.Canceled` is preserved by identity, stderr is exactly `Error: context canceled`, and stdout is empty. | `in-process Process.Execute` from a dedicated `support.BuildProcessWithContext`; local `io.Pipe`, temporary home/work directories, and dedicated lifecycle. | `support.CleanupProcess`; `t.Cleanup` closes both pipe ends; `t.TempDir`. | 1 / 1 |
| `TestCLIContextCancellationStopsExternalWork` (`context_cancellation_test.go:37`) | `isolated-with-reason` | Canceling the invocation stops the attributable external worker PID and the root command returns unsuccessfully. | `in-process Process.Execute` through `builtcliacceptance.Command.Start`; real Unix `/bin/sh`/`sleep` descendant and PID file, but not a built CLI child. Windows skips. | Command cancel/wait, explicit descendant termination fallback, bounded process-exit observation, `t.TempDir`. | 1 / 0 |
| `TestCLIContextCancellationEmitsNoSuccessResult` (`context_cancellation_test.go:121`) | `isolated-with-reason` | Cancellation produces `INVOCATION_CANCELED`, empty stdout, and no success primary result while external work is in flight. | `in-process Process.Execute` plus real Unix external worker/PID fixture; not a built CLI child. Windows skips. | Command cancel/wait, explicit descendant termination fallback, bounded process-exit observation, `t.TempDir`. | 1 / 0 |
| `TestContextCancellationPIDReadinessIgnoresPartialPublication` (`context_cancellation_test.go:321`) | `shareable` | Empty, whitespace, partial, and nonnumeric publications return `(0,false)`; a complete positive PID returns the exact PID and `true`. | `process-free` pure helper call; five table subcases and no process observation. | No process/resource cleanup; test-local values only. | 0 / 0 |
| `TestCLIWorkerFailureExitCode` (`exit_codes_test.go:28`) | `shareable-with-mock` | One controlled provider failure crosses `Process.Execute` as a typed error, empty stdout, and one sanitized `INVOCATION_RUNTIME_FAILURE` diagnostic with private detail removed. | `in-process Process.Execute` from `support.BuildProcess`; controlled `ProviderCommandRunner`, fresh scaffold, and captured inputs. | `support.CleanupProcess`; `support.FakeInputs`; `t.TempDir` fixture. | 1 / 1 |
| `TestBuiltCLIInterruptedResponseStreamExitCode` (`exit_codes_test.go:73`) | `isolated-with-reason` | A real Unix `os.Interrupt` maps to exit 130, reports canceled response-stream output, suppresses success markers, writes `INVOCATION_CANCELED`, and reaps the worker child. | `built-child`: one temporary `go build` artifact and one `exec.Command` child; controlled serialized mock-worker fixture and real Unix signal/process tree. Windows skips. | `command.Wait`/kill fallback, descendant termination and bounded join, `t.TempDir`, temporary binary removal. | 1 / 0 |
| `TestCLISuccessExitCode` (`exit_codes_test.go:185`) | `shareable-with-mock` | One controlled provider success returns nil, exactly the primary stdout, empty stderr, and one provider call through `Process.Execute`. | `in-process Process.Execute` from `support.BuildProcess`; controlled `ProviderCommandRunner`, fresh scaffold, and captured inputs. | `support.CleanupProcess`; `support.FakeInputs`; `t.TempDir` fixture. | 1 / 1 |
| `TestCLIHelpListsPublicCommandFamilies` (`help_and_version_test.go:56`) | `isolated-with-reason` | Bare root and explicit `--help` expose the same public command families, omit hidden/nested-only commands and activation noise, and leave the isolated filesystem untouched. | Current `Session.Run` is `in-process Process.Execute`; each subcase and comparison rerun receives a fresh home/work/log directory and reserved port. Required future fidelity is a built child. | `t.TempDir`, per-session temporary directories, reserved port close, and process close in `Command.Run`. | 4 / 4 |
| `TestCLISubcommandHelpUsesStableUsageAndExitZero` (`help_and_version_test.go:133`) | `isolated-with-reason` | `docs --help` and `docs -h` preserve usage, description, flags, empty stderr, exit zero, and no activation effects. | Current `Session.Run` is `in-process Process.Execute`; two fresh isolated sessions. Required future fidelity is a built child and alias parser. | `t.TempDir`, per-session temporary directories, reserved port close, and process close in `Command.Run`. | 2 / 2 |
| `TestCLIVersionWritesOneMachineReadableVersion` (`help_and_version_test.go:189`) | `isolated-with-reason` | `--version` writes one `dev` or semantic-version token, no stderr/help/lifecycle noise, exit zero, and no product filesystem effects. | Current `Session.Run` is `in-process Process.Execute`; fresh isolated environment. Required future fidelity is built-artifact version injection and OS stream/exit behavior. | `t.TempDir`, per-session temporary directories, reserved port close, and process close in `Command.Run`. | 1 / 1 |
| `TestCLISuccessWritesPrimaryResultOnlyToStdout` (`stdout_stderr_test.go:28`) | `isolated-with-reason` | A successful quiet named run writes only `mock worker accepted` to stdout, leaves stderr empty, and emits no lifecycle/diagnostic markers. | Current `Session.Run` is `in-process Process.Execute` with serialized mock-worker configuration; the body’s intended property is OS stdout/stderr and exit separation, which requires a built child. | Initializer `Session.Run`, `t.TempDir` fixture, per-session temporary directories, reserved port close, and process close in `Command.Run`. | 2 / 2 |
| `TestCLIFailureWritesDiagnosticToStderr` (`stdout_stderr_test.go:105`) | `isolated-with-reason` | A rejecting quiet named run exits unsuccessfully, writes actionable `INVOCATION_RUNTIME_FAILURE` diagnostics to stderr, leaves stdout empty, and avoids lifecycle leakage. | Current `Session.Run` is `in-process Process.Execute` with serialized mock-worker configuration; intended OS stream/exit property requires a built child. | Initializer `Session.Run`, `t.TempDir` fixture, per-session temporary directories, reserved port close, and process close in `Command.Run`. | 2 / 2 |
| `TestCLIQuietModeSuppressesNonResultNoise` (`stdout_stderr_test.go:199`) | `isolated-with-reason` | Three subcases establish the response-stream noise baseline, quiet success suppression, verbose-vs-quiet stderr contrast, and quiet failure script-safe stdout/error routing. | Current `Session.Run` is `in-process Process.Execute` for each of ten invocations with isolated sessions; intended process stream routing requires built children. | Initializer and run per subcase, `t.TempDir` fixtures, per-session temporary directories, reserved port close, and process close in `Command.Run`. | 10 / 10 |
| `TestCLIUnknownCommandWritesSafeCodedStderr` (`unknown_command_test.go:17`) | `isolated-with-reason` | Unknown root command produces safe Cobra text and help guidance, without internal envelopes, hidden suggestions, or activation output. | Current `Session.Run` is `in-process Process.Execute`; intended parser/stderr/exit evidence requires a built child. | `t.TempDir`, isolated environment, process close in `Command.Run`. | 1 / 1 |
| `TestCLIUnknownCommandReturnsUsageExitCode` (`unknown_command_test.go:69`) | `isolated-with-reason` | Unknown root command returns exit 1, empty stdout, no help/activation noise, and no Factory/config/server/worker side effect. | Current `Session.Run` is `in-process Process.Execute`; intended OS exit/environment evidence requires a built child. | `t.TempDir`, isolated environment, process close in `Command.Run`. | 1 / 1 |

The count is `12` retained isolated witnesses, `2` migration candidates
(`TestCLIWorkerFailureExitCode` and `TestCLISuccessExitCode`), and `1`
already process-free parser witness. No body-level classification blocker was
found.

## CASE-001 through CASE-028 reconciliation

The case rows below enumerate every planned assertion in the current matrix.
“Current evidence” describes what the present body actually reaches; “later
gate” identifies a property that GATE-CHAR intentionally does not claim.

| Case | Body-backed test/subcase | Current evidence and exact property | Classification / later gate |
| --- | --- | --- | --- |
| CASE-001 | `TestCLIHelpListsPublicCommandFamilies/bare_root` | Bare `Session.Run` root discovery exits successfully; public families and title appear, hidden families and activation markers do not, stderr is empty, and the isolated filesystem remains unchanged. | `isolated-with-reason`; current in-process parser, built-child fidelity later in GATE-FUNC. |
| CASE-002 | `TestCLIHelpListsPublicCommandFamilies/explicit_help` | `Session.Run("--help")` asserts the same public-family, output, stderr, and no-activation contract. | `isolated-with-reason`; built-child fidelity later in GATE-FUNC. |
| CASE-003 | `TestCLIHelpListsPublicCommandFamilies` comparison reruns | `runRootHelpListedCommands` invokes bare root and explicit help in two additional fresh sessions and compares parsed command sets as equal. | `isolated-with-reason`; fresh-process comparison later in GATE-FUNC. |
| CASE-004 | `TestCLISubcommandHelpUsesStableUsageAndExitZero/docs_help_flag` | `docs --help` asserts stable usage, description, flags, help text, exit success, empty stderr, and no activation effects. | `isolated-with-reason`; current in-process nested parser, built child later. |
| CASE-005 | `TestCLISubcommandHelpUsesStableUsageAndExitZero/docs_short_help` | `docs -h` asserts the same successful nested-help contract through the short alias. | `isolated-with-reason`; current in-process alias parser, built child later. |
| CASE-006 | `TestCLIVersionWritesOneMachineReadableVersion/version_flag` | `--version` asserts one `dev`/semantic-version stdout line, empty stderr, no help/lifecycle noise, exit success, and no filesystem activation. | `isolated-with-reason`; current in-process version path, built artifact later. |
| CASE-007 | Help, nested-help, version, and unknown-command fresh sessions | `ProcessEnvForIsolatedHome` redirects HOME/profile variables; assertions check no Factory directory or operator config after discovery/invalid-command runs. Each session has isolated home, work, and logs. | `isolated-with-reason`; current environment isolation is in-process, OS environment evidence later. |
| CASE-008 | `TestCLISuccessWritesPrimaryResultOnlyToStdout` | Initialized accepting named goal succeeds with exit 0, exact primary stdout `mock worker accepted`, empty stderr, and no lifecycle/diagnostic noise. | `isolated-with-reason`; current Process.Execute, built-child stream/exit later. |
| CASE-009 | `TestCLIFailureWritesDiagnosticToStderr` | Initialized rejecting named goal fails, has nonzero result, actionable `INVOCATION_RUNTIME_FAILURE` diagnostics on stderr, empty stdout, and no lifecycle leakage. | `isolated-with-reason`; current Process.Execute, built-child stream/exit later. |
| CASE-010 | `TestCLIQuietModeSuppressesNonResultNoise` / `success suppresses stdout lifecycle presentation`, response-stream run | Non-quiet response-stream success contains the primary-result separator and recognized human lifecycle noise, establishing the presentation baseline. | `isolated-with-reason`; current Process.Execute, built-child stream later. |
| CASE-011 | Same first quiet-mode subcase, quiet run | The equivalent quiet success returns exit 0, exact raw result, no separator/lifecycle markers, and empty stderr. | `isolated-with-reason`; current Process.Execute, built-child stream later. |
| CASE-012 | `success suppresses verbose stderr operator logs`, verbose run | `--verbose` succeeds, preserves the primary result as the stdout suffix, and provides the contrast for quiet suppression. | `isolated-with-reason`; current Process.Execute, built-child stream later. |
| CASE-013 | Same verbose subcase, quiet contrast | Fresh quiet success returns only the raw result and empty stderr after the verbose run. | `isolated-with-reason`; fresh-process stream isolation later. |
| CASE-014 | `failure keeps quiet stdout script-safe` | Quiet rejecting run is nonzero, stdout is empty, and stderr is actionable/nonempty. | `isolated-with-reason`; current Process.Execute, built-child stream/exit later. |
| CASE-015 | `TestCLIWorkerFailureExitCode` | Direct `Process.Execute` with one shaped provider failure returns an error, exactly one provider call, empty stdout, one coded runtime-failure diagnostic, and no private detail. | `shareable-with-mock`; migrated shared root in GATE-FUNC. |
| CASE-016 | `TestCLISuccessExitCode` | Direct `Process.Execute` with one shaped provider success returns nil, exactly one provider call, exact primary stdout, and empty stderr. | `shareable-with-mock`; migrated shared root in GATE-FUNC. |
| CASE-017 | No current direct body; planned failure-then-success composite of CASE-015/016 | Current tests build separate roots and therefore do not observe recovery on one shared root. The required later assertion is fresh inputs/routes, one call per scenario, exact outcomes, and no state bleed after failure. | `shareable-with-mock`; intentionally unproved until GATE-FUNC. |
| CASE-018 | `TestCLIContextCancellationStopsExternalWork` | After deterministic complete PID publication, cancellation causes an unsuccessful root command and the attributable Unix external worker is no longer running within the bounded guard. | `isolated-with-reason`; Windows explicit skip; Unix process-tree proof later. |
| CASE-019 | `TestCLIContextCancellationEmitsNoSuccessResult` | After the same deterministic readiness, cancellation produces `INVOCATION_CANCELED`, empty stdout, and no completed success result. | `isolated-with-reason`; Windows explicit skip; Unix cancellation proof later. |
| CASE-020 | `TestBuiltCLIInterruptedResponseStreamExitCode` | A real built child receives `os.Interrupt`, exits 130, reports CANCELED without success markers, writes `INVOCATION_CANCELED`, and the worker PID exits. | `isolated-with-reason`; Windows explicit skip; Unix signal proof present only on Unix. |
| CASE-021 | `TestACPServeCancellationPreservesContextCanceledIdentityThroughProcess` | The read-start signal fires before cancellation; direct `Process.Execute` returns `context.Canceled` by identity with exact stderr and empty stdout after blocked stdin cleanup. | `isolated-with-reason`; dedicated ACP lifecycle retained. |
| CASE-022 | `TestCLIUnknownCommandWritesSafeCodedStderr` | Unknown command stderr contains Cobra’s unknown-command/help guidance, no internal failure envelope, no hidden suggestions, and no activation surfaces. | `isolated-with-reason`; current Process.Execute, built parser later. |
| CASE-023 | `TestCLIUnknownCommandReturnsUsageExitCode` | Unknown command returns exit 1, empty stdout, no help/lifecycle noise, and no Factory/config/server/worker effects. | `isolated-with-reason`; current Process.Execute, built OS exit/environment later. |
| CASE-024 | `TestContextCancellationPIDReadinessIgnoresPartialPublication/empty publication` | Empty PID bytes return `(0,false)` without process observation. | `shareable`/process-free. |
| CASE-025 | `.../whitespace publication` | Whitespace-only PID bytes return `(0,false)`. | `shareable`/process-free. |
| CASE-026 | `.../partial publication` and `.../non numeric publication` | `12x` and `worker` each return `(0,false)`, preventing polling against a wrong PID. | `shareable`/process-free. |
| CASE-027 | `.../complete publication` | `12345\n` returns exact positive PID `12345` and `true`. | `shareable`/process-free. |
| CASE-028 | Windows build-tag path plus the full package matrix | Current Windows gate passes with exactly the three Unix-only skips; `context_cancellation_process_windows_test.go` compiles the Windows handle-based join/termination helpers. The current gate does not prove Unix signal/`/bin/sh` behavior or post-change cleanup/start counts. | Mixed platform case; complementary GATE-UNIX/GATE-WINDOWS and GATE-FUNC. |

## Characterization baseline application-start ledger

| Test | Windows starts | Unix starts | Accounting |
| --- | ---: | ---: | --- |
| ACP cancellation | 1 | 1 | One direct root process. |
| External-work cancellation | 0 | 1 | One `Command.Start`; Unix-only. |
| Cancellation no-success | 0 | 1 | One `Command.Start`; Unix-only. |
| PID readiness parser | 0 | 0 | Pure helper. |
| Worker failure | 1 | 1 | One direct root process. |
| Interrupted response stream | 0 | 1 | One built CLI child; Unix-only. Compiler and provider child excluded. |
| Worker success | 1 | 1 | One direct root process. |
| Root help families | 4 | 4 | Two table subcases plus two comparison reruns. |
| Subcommand help | 2 | 2 | `--help` and `-h`. |
| Version | 1 | 1 | One version invocation. |
| Success stdout | 2 | 2 | One initializer failure plus one named run. |
| Failure stderr | 2 | 2 | One initializer failure plus one named run. |
| Quiet mode | 10 | 10 | Four starts in each success subcase and two in failure subcase. |
| Unknown stderr | 1 | 1 | One invalid-command invocation. |
| Unknown exit code | 1 | 1 | One invalid-command invocation. |
| **Total** | **26** | **29** | Unix adds the three explicitly skipped Windows rows. |

The pre-change baseline is therefore `26` Windows and `29` Unix application
starts. The post-migration target is `25` Windows and `28` Unix after the two
worker outcome tests share one root and the parser remains at zero starts.
These are invocation ledger counts, not host process snapshots.

## Timing diagnostics and evidence limits

- This GATE-CHAR run measured `19.22s` at the Go package event on the current
  Windows host. It is one diagnostic sample, not a local threshold.
- The PRD records a prior compute-saturated Windows characterization at
  `27.464s`; that observation is retained for context and is not a delivery
  threshold.
- The PRD also records source-plan CI observations of `12.85–13.27s`; the
  referenced `docs/temp/functional-test-optimization.md` is not present in
  this checkout, so those values are not independently remeasured here.
- GATE-CHAR proves the current names, body-backed assertions, boundary
  classification, current-platform pass/skip result, and invocation ledger.
- It does not prove post-migration topology, the CASE-017 shared-root
  recovery, Unix-only cases on this Windows host, Windows runtime behavior
  beyond this pass/compile path, three-repeat stability, target start counts,
  PR package timing, clean-room validation, or absence of every transient
  resource after the process exits.

The characterization commit changed no shared support, production CLI
behavior, contracts, generated artifacts, baselines, CI workflows, adjacent
packages, or excluded cleanup surfaces.

## GATE-FUNC — story 002 pre-review implementation result

- Story: `functional-test-optimization-c06-transport-cli-process-002`
- Tested implementation commit: `c30105bb9`
- Platform: `windows/amd64` (`go1.25.0`)
- Exact command: `go test -count=1 -timeout=10m ./tests/functional/transport/cli/process`
- Result: `pass`, exit code `0`, package elapsed `20.118s`; 12 top-level tests
  passed and the three documented Unix-only tests skipped.
- Dependency fidelity: local-real production root and built `you` executable;
  controlled `ProviderCommandRunner` for the two eligible worker outcomes;
  real Windows child process, streams, environment, and process cleanup for
  retained executable witnesses.

### Topology and behavior result

- `TestCLIWorkerFailureExitCode` and `TestCLISuccessExitCode` now execute
  through one package-owned `support.ApplicationProcess`. Each call creates a
  fresh factory, `root.Input`, context, isolated HOME/profile environment,
  working directory, and captured streams. The synchronized route binds each
  fresh factory directory to its controlled failure or success result; each
  route observed exactly one provider call, and both existing typed
  `Process.Execute`/stdout/stderr assertions passed.
- The package builds one CLI artifact through a `sync.Once` cache and every
  retained executable/OS invocation creates a fresh `exec.Command` child.
  No live child, stream, environment, session, PID file, or route is shared.
- ACP cancellation still constructs a dedicated root and the PID-readiness
  table remains process-free. The Windows process helper file compiled; its
  Unix-only callers retained their explicit skips.
- Cleanup completed without timeout or teardown error: ordinary built-child
  calls joined through `exec.Cmd.Wait`, cancellation cases joined their root
  child and applied the existing attributable-descendant cleanup fallback,
  the shared root closed in `TestMain`, and the package artifact directory was
  removed after the test run.

### Post-change application-start ledger

The shared worker row represents two sequential `Process.Execute` calls on one
root construction. All other rows retain one fresh child/root invocation per
ledger entry. The parser remains at zero starts.

| Test or shared fixture | Windows starts | Unix starts | Result |
| --- | ---: | ---: | --- |
| ACP cancellation | 1 | 1 | Dedicated root retained. |
| External-work cancellation | 0 | 1 | Built child retained; Windows skip. |
| Cancellation no-success | 0 | 1 | Built child retained; Windows skip. |
| PID readiness parser | 0 | 0 | Process-free. |
| Shared worker outcomes (failure + success) | 1 | 1 | One root, two isolated inputs, two routed calls. |
| Interrupted response stream | 0 | 1 | Built child retained; Windows skip. |
| Root help families | 4 | 4 | Fresh built child per invocation. |
| Subcommand help | 2 | 2 | Fresh built child per invocation. |
| Version | 1 | 1 | Fresh built child. |
| Success stdout | 2 | 2 | Fresh built child per initialization/run. |
| Failure stderr | 2 | 2 | Fresh built child per initialization/run. |
| Quiet mode | 10 | 10 | Fresh built child per invocation. |
| Unknown stderr | 1 | 1 | Fresh built child. |
| Unknown exit code | 1 | 1 | Fresh built child. |
| **Total** | **25** | **28** | **2 migrated, 1 already process-free, 12 retained.** |

### Properties proved and remaining edges

This gate proves the complete applicable current-platform matrix, shared-root
failure-to-success recovery, one provider call per eligible scenario, fresh
input/resource isolation, built-artifact parser/stream/exit fidelity, and the
target current-platform ledger. It does not prove Unix signal and descendant
reaping, three-repeat stability, PR package timing, or independent clean-room
classification; those remain owned by GATE-REPEAT, GATE-UNIX/GATE-WINDOWS,
GATE-PR-PERF, and VAL-001.

## Story 003 — repeat and clean-room result

- Tested implementation commit: `ced9c72e4` (`test: reset shared CLI process
  fixture between repeats`), the conflict-free rebase of the same fix from
  `b8b2167dd` onto current `origin/main`.
- Local platform: `windows/amd64` (`go1.25.0`), 24 logical CPUs.
- GATE-REPEAT exact command: `go test -count=3 -timeout=30m
  ./tests/functional/transport/cli/process`.
- GATE-REPEAT result: **PASS**, exit code `0`, package elapsed `54.997s`; all
  three repetitions produced the same 12 top-level passes and three explicit
  Unix-only skips. The shared failure/success root was reused within each
  repetition and closed before the next repetition, preventing durable runtime
  state from crossing fresh fixtures. The compile-once built artifact and all
  child `Wait`/cancellation cleanup completed without teardown errors.
- Current-platform GATE-FUNC rerun exact command: `go test -count=1
  -timeout=10m ./tests/functional/transport/cli/process`.
- Current-platform result: **PASS**, exit code `0`, package elapsed `21.077s`;
  12 top-level tests passed and the three documented Unix-only tests skipped.
  This is local Windows evidence only; it does not claim Unix signal,
  cancellation, exit-130, or descendant-reaping behavior.
- One earlier post-commit single-run attempt exited `1` only while `TestMain`
  removed the temporary built binary (`access denied` on Windows); the
  immediate same-head rerun passed, and the three-repeat run also completed
  teardown successfully. This is retained as transient host cleanup
  contention, not a behavioral pass claim.
- GATE-WINDOWS local cleanup result: applicable child joins, stream closure,
  temporary fixture cleanup, Windows helper compilation, shared-root close,
  and package binary cleanup passed in the successful reruns. Required hosted
  platform evidence remains a PR CI responsibility.
- VAL-001 exact procedure: create a detached clean worktree at `b8b2167dd`
  (the pre-rebase implementation-equivalent fix),
  confirm `git status --short --untracked-files=all` is empty, run `go test
  -count=1 -timeout=10m ./tests/functional/transport/cli/process`, read every
  test body alongside the 15 classification rows and CASE-001 through
  CASE-028 rows, then confirm the clean worktree remains empty.
- VAL-001 result: **PASS**. The detached clean worktree package run passed on
  Windows in `23.207s`; the clean status was empty before and after the run;
  independent review matched all 15 top-level bodies to exactly one of 12
  `isolated-with-reason`, two `shareable-with-mock`, or one process-free
  `shareable` classification, and all 28 case rows were accounted for.
- Final handoff head: `bce9b48af` after the required rebase onto
  `origin/main` `93c7dd9f2`; the final-head repeat and single-run commands
  above passed before push.
- Remaining edges: required Unix/Windows hosted CI semantics and same-head
  Backend Functional Coverage timing are recorded in the PR conversation;
  this artifact does not claim those results before CI reports them.

## Review correction — current lane placement and lint result

The review identified that the pre-correction implementation placed built CLI
and OS-process witnesses under the functional package. The normative backend
testing rule requires those witnesses under the integration lane. This correction
preserves the 15 top-level names and CASE-001 through CASE-028 assertions while
moving only the tests whose bodies require a built executable, OS streams,
signals, or child-process cleanup.

### Current source ownership

The current body-to-lane reconciliation is:

| Lane | Tests | Boundary |
| --- | --- | --- |
| Functional | `TestACPServeCancellationPreservesContextCanceledIdentityThroughProcess`, `TestContextCancellationPIDReadinessIgnoresPartialPublication`, `TestCLIWorkerFailureExitCode`, `TestCLISuccessExitCode` | Dedicated or shared `root.BuildProcess` + `Process.Execute`, or pure parser; no executable construction or OS child. |
| Integration | `TestCLIContextCancellationStopsExternalWork`, `TestCLIContextCancellationEmitsNoSuccessResult`, `TestBuiltCLIInterruptedResponseStreamExitCode`, `TestCLIHelpListsPublicCommandFamilies`, `TestCLISubcommandHelpUsesStableUsageAndExitZero`, `TestCLIVersionWritesOneMachineReadableVersion`, `TestCLISuccessWritesPrimaryResultOnlyToStdout`, `TestCLIFailureWritesDiagnosticToStderr`, `TestCLIQuietModeSuppressesNonResultNoise`, `TestCLIUnknownCommandWritesSafeCodedStderr`, `TestCLIUnknownCommandReturnsUsageExitCode` | Existing integration package's compile-once artifact and fresh built children; real OS streams, exit status, signal, environment, and descendant cleanup where each body requires them. |

The two worker outcome tests still use one package-owned functional root with
fresh `root.Input` values and synchronized `ProviderCommandRunner` routes. ACP
shutdown remains a dedicated functional root, and the PID parser remains
process-free. The integration package reuses its existing `TestMain` artifact
fixture, but does not share a live child, stream, environment, session, PID
file, or scenario route between invocations. The three Unix-only cases retain
their explicit Windows skips, and the Windows helper files compile in the
integration package.

### Current verification

- Functional lane command: `go test -count=1 -timeout=10m
  ./tests/functional/transport/cli/process`; local Windows result: pass,
  exit code `0`, four top-level tests, and no platform skips. This proves the
  shared `Process.Execute` failure/success recovery, dedicated ACP cancellation,
  and process-free parser at the required functional boundary.
- Integration c06 focused command:
  `go test -count=1 -timeout=10m ./tests/integration/transport/cli/process
  -run '^(TestCLIContextCancellationStopsExternalWork|TestCLIContextCancellationEmitsNoSuccessResult|TestBuiltCLIInterruptedResponseStreamExitCode|TestCLIHelpListsPublicCommandFamilies|TestCLISubcommandHelpUsesStableUsageAndExitZero|TestCLIVersionWritesOneMachineReadableVersion|TestCLISuccessWritesPrimaryResultOnlyToStdout|TestCLIFailureWritesDiagnosticToStderr|TestCLIQuietModeSuppressesNonResultNoise|TestCLIUnknownCommandWritesSafeCodedStderr|TestCLIUnknownCommandReturnsUsageExitCode)$'`; local Windows result: pass, exit code `0`, elapsed `26.248s`. The three Unix-only tests are explicit skips on this host; their Unix signal/descendant properties remain unclaimed locally.
- Integration package command: `go test -count=1 -timeout=10m
  ./tests/integration/transport/cli/process`; local Windows result: pass,
  exit code `0`, elapsed `75.878s`, including the pre-existing integration
  matrix and all relocated c06 witnesses.
- Lint correction: `make backend-size` passed. The moved quiet-mode test is
  now below the 100-line function limit after helper extraction, so its stale
  functional exemption was removed; no new exemption was added.
- Current c06 application-start ledger remains `25` Windows / `28` Unix:
  `2` migrated worker scenarios share one functional root, `1` parser scenario
  remains process-free, and `12` retained executable/OS/lifecycle witnesses
  remain isolated in the two appropriate test lanes. These counts exclude the
  integration package's unrelated pre-existing tests and all compiler/provider
  descendants.

This correction proves lane-compliant current-platform behavior and preserves
the declared dependency fidelity. It does not prove Unix signal/descendant
reaping on this Windows host, three-repeat stability after relocation, same-head
PR package timing, or independent clean-room reconciliation; those remain
GATE-UNIX/GATE-WINDOWS, GATE-REPEAT, GATE-PR-PERF, and VAL-001 edges.
