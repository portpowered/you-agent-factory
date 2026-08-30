# Validation report: BEH-001 — Fast compatible batch cold start

## Environment and artifact

- Validation source head: `1dc12f20f7ef812d7924e0e2e6d65d39ee4d585a`, rebased
  onto `origin/main` `a44ed015421b1ed42b919f1178531f38fd5087b5`. The final
  evidence commits are documentation only; no Go, functional-test, API,
  generated, or Wire source changed after this validation source head.
- Environment: Microsoft Windows 10.0.26200, `go1.25.0 windows/amd64`, Node
  `v22.12.0`, Bun `1.4.0`, PowerShell `7.6.5`, and 24 logical CPUs. Probe
  children used isolated `HOME`/`USERPROFILE` directories and
  `YOU_NO_BROWSER_OPEN=1`.
- Customer entry point: the compiled `./cmd/factory` binary, invoked with the
  exact one-Work batch command recorded below.
- Customer artifact: `.artifacts/batch-coldstart-probe/validation-9d1ffa7653-novcs.exe`,
  built from evidence head `9d1ffa765342fdef4b565825562ec71fface455e` with
  `go build -buildvcs=false -o ... ./cmd/factory`; SHA-256
  `BA6D1E721E97EBE98BA17C921DD659BC8710FBFD68CD5DC3413FC8612D887FFE`.
- Real and substituted dependencies: the real compiled CLI, Go runtime,
  filesystem, child-process creation, durable snapshot writer, and local
  lifecycle were exercised. The Work worker was the repository's accepting
  mock fixture. No network or paid provider call was used.
- Cost/call budget: one local build, three exact child invocations, focused
  package/race/direct-consumer gates, one serialized fresh functional lane, and
  the short Go/lint/package gates; zero provider calls and zero paid cost.
- Fixture hashes were unchanged: Factory
  `AD247CDAD555A29A73149E109B57AD3270FB1F705E3C805C9805B55493B9F978`, Work
  `CA3828653379C6BA4DA77FA2C887C03EBE65A846ADFB4FDAA2E7C2083D84DCC4`, and
  accepting mock workers
  `69A0CE360F450CF9E71D4C9935345D1B2CF6A734A4E79DAE534E836F6218D3C3`.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| GATE-CHAR-001 | PASS | The story-001 pre-change binary, exact fixture hashes, baseline median `1318.610 ms`, terminal Work, cleanup, and CPU-profile attribution remain in the tracked evidence ledger. | Future hosts and non-mock providers. |
| GATE-SPINE-001 | PASS | Changed CLI/process-lifecycle tests, direct initializer/application/Wire/runtime-hosting/recording/replay consumers, demanded-initializer failure, lifecycle order, same-preparation concurrency, reuse, recovery, and cleanup witnesses passed. | The broad unchanged ACP idle-stdin cancellation edge remains qualified below. |
| GATE-RACE-001 | PASS, qualified | The changed CLI and process-lifecycle race selectors passed five repetitions; the direct same-preparation demand witness proves one initializer call for concurrent first demand. | An unrelated broad ACP idle-stdin cancellation selector remains unproven. |
| GATE-PERF-001 | PASS | Three exact fresh child invocations from the final evidence head had median `350.666 ms`, `73.41%` below the `1318.610 ms` baseline, with exact output, exit, accepted terminal Work, exited children, and zero packaged-Factory files. | Other machines and later source drift. |
| GATE-SERVER-001 | PASS | Runtime-hosting, recording/replay, readiness/failure/cancellation, lifecycle, and server-preservation selectors passed; the serialized functional inventory also passed unchanged server/session/event paths. | No browser criterion applies to this backend/compiled-CLI change. |
| GATE-FUNC-001 | PASS, serialized | `FUNCTIONAL_DEFAULT_JOBS=1 make test-functional-fresh` exited `0` with isolated homes and `YOU_NO_BROWSER_OPEN=1`; the complete unchanged inventory passed, including packaged factories, server/session/event, recordings/process, and workers/mock. | The default parallel runner remains sensitive to shared staging ownership. |
| Quality gates | PASS, platform-qualified | `make test` exited `0`; package, boundary, structure, cycle, catalog, generation, package-maintainability, backend-size, and file-count checks passed. Native aggregate lint passed every target except the pre-existing Windows build-tag deadcode comparison (`3072` current findings versus `3074` baseline); no baseline or unrelated source was changed. | The shared deadcode baseline is not changed in this lane. |
| GATE-LOOP-001 | READY FOR HANDOFF | This report and the final ledger section identify the exact validated source/build, environment, customer journey, dependencies, costs, criteria, findings, and remaining remote-delivery edge. | The final push, PR check start, terminal CI, and merge are review-owned delivery edges. |
| No scope growth | PASS | No public contract, API, event, persisted-schema, pricing, `tests/functional` source, or Wire authorship changed. The review-requested synchronization fix moved the gate into `pkg/initializer` and retained CLI policy in the transport. | Review must preserve this boundary. |

## Customer journey

The compiled `cmd/factory` binary ran this exact command in three fresh,
isolated child fixture/home directories:

~~~text
run --work one-work.json --with-mock-workers=accept.json --no-record --quiet
~~~

The observed wall times were `461.502 ms`, `348.449 ms`, and `350.666 ms`;
the sorted median was `350.666 ms`. Against the characterized baseline of
`1318.610 ms`, the median is `967.944 ms` lower, or `73.41%` lower
(`0.2659x` the baseline). Every process exited `0`, reported
`HasExited=True`, wrote exact stdout `Batch completed successfully.\n`
(30 UTF-8 bytes), and wrote empty stderr.

Each durable snapshot contained Work `one-work` at
`prompt-task:complete`, state `complete`, outcome `ACCEPTED`, and
transition `process-prompt`. Snapshot hashes for runs 1–3 were
`A5AD7E388B2E57E964E315F41DBB91F1DA834873C7314D13930C83C50FB41B06`,
`67A7FC11E2FD496E4376FD4495B5778D1E9650C17013BE6FC6A9474CEC3B8F05`, and
`9C13B65C4868C2A60DEC1001D616A1A676EF7EC88F9AC43A9AFC1F2644EB47D4`.
Each isolated packaged-Factory root contained zero files.

The changed-package and direct-consumer commands exited successfully:

~~~text
go test ./pkg/transports/cli ./pkg/services/factory_sessions/internal/processlifecycle ./pkg/wire ./pkg/initializer/... -count=1
go test -race ./pkg/transports/cli -run '^TestRunCommand_ContinuouslyFlag$' -count=5 -timeout=10m
go test -race ./pkg/services/factory_sessions/internal/processlifecycle -run '^TestRuntimeLifecycleOwnsActivationAndReverseShutdown$' -count=5 -timeout=10m
go test ./pkg/services/factory_sessions/internal/runtimehosting ./pkg/services/factory_sessions/internal/execution/recordingreplay ./pkg/services/recordings/... -count=1 -timeout=10m
go test ./tests/functional/workers/mock -count=1 -timeout=10m
~~~

The full short suite exited `0` with 447 Go packages (one
platform-conditional skip) and 169 workflow tests (168 pass, one skip). The
serialized functional lane exited `0` without editing its inventory. The
package-boundary checker reported zero production or test-only violations,
including after the synchronization fix moved `PreparationGate` into the
initializer-owned lifecycle package. `make generate-wire` was not applicable:
no Wire authorship changed.

The first probe attempt copied only visible fixture files and correctly failed
closed with `CURRENT_FACTORY_NOT_FOUND`; it was discarded as a procedure
error. The authoritative probe copied the complete fixture tree, including
hidden and recursive files, and is the result above.

## Cross-task integration and usability

- Documentation discoverability: the measured result and historical baseline
  are recorded in the [cold-start evidence ledger](../c14-cli-batch-run-cold-start-latency-evidence.md).
- Permission and error behavior: demanded initialization, invalid input,
  dependency/start failure, capacity, timeout, cancellation, partial
  activation, and fresh-process recovery witnesses passed with attributable
  errors and no blind retry within one invocation.
- Persistence/reload behavior: every authoritative probe wrote the expected
  durable snapshot and accepted terminal token; recording/replay consumers
  passed.
- Accessibility/keyboard/responsive behavior: not applicable; this is a
  backend/compiled-CLI change with no UI surface.
- Operational signals: no listener was requested for the batch command, all
  child processes exited, no packaged-Factory files were created, and the
  full serialized functional inventory completed.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| VAL-001 | RESOLVED | Concurrent first demand on one startup-preparation callback. | One invocation performs one synchronized preparation. | The initializer-owned `PreparationGate` serialized the two demands; the existing CLI test witness observed exactly one initializer call and both callers succeeded. | `TestRunCommand_ContinuouslyFlag` focused/race coverage. |
| VAL-002 | QUALIFIED | Broad unchanged race selector for ACP idle-stdin cancellation. | The broad race lane is green. | The known ACP selector remains an unproven existing edge; changed CLI/process-lifecycle race selectors pass. | Focused race commands and prior direct selector reproduction. |
| VAL-003 | QUALIFIED ENVIRONMENT | Native Windows `make lint`. | The aggregate lint wrapper is green. | Every lint target passed except deadcode's Windows build-tag count (`3072`) versus the committed baseline (`3074`); no baseline or unrelated source was changed. | Native `make lint` output. |
| VAL-004 | DISCARDED PROCEDURE | Manual fixture copy omitted hidden/recursive entries. | The fixture contains the complete Factory tree. | The command failed closed with `CURRENT_FACTORY_NOT_FOUND`; the authoritative complete-tree rerun passed and only that rerun is used for product evidence. | Three-run probe procedure log. |

## Verdict

PASS for the local-real validation scope. The measured batch path exceeds the
required 50% improvement while preserving the exact customer result and the
server, recording/replay, failure, cancellation, concurrency, recovery,
cleanup, and functional behavior proved above. The final PR push and required
CI start are the remaining implementation handoff actions; terminal CI and
merge remain review-owned.
