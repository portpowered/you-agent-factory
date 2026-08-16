# Observability Plane Packet Specification

Status: Story 001 baseline; current-tree inventory complete. Later stories add
the target contracts and post-BTRC-P6 packets to this same specification.

This document is the authoritative starting point for a backend logs, metrics,
and traces plane. This iteration is documentation-only. It does not add
production instrumentation, dependencies, exporters, baselines, composition
changes, or resumption behavior.

## Audit authority and reproducibility

The audit was performed on 2026-08-16 at commit
`424e314f5c1c7b1e6c4ae361e1b02428769e41cb` (`424e314f5`), the then-current
`origin/main` and `HEAD`. The commit timestamp is
`2026-08-16T11:56:10-07:00`. Current-tree symbols, tests, and counts below are
authoritative only for that snapshot.

The work item supplied historical operational context dated 2026-08-16, but
did not supply an incident timestamp. A later checkout does not change that
historical claim; it does make the present-state counts stale. If the branch
moves, rerun the commands below and update the audited commit/date before
using a count as current evidence.

The audit used `rg` over Go source. “All” includes `_test.go`; “production” is
the same search with `_test.go` excluded. Import counts count files containing
the direct import path, not import statements, logger calls, or emitted
records.

```powershell
# Direct platform-surface imports in cmd/, internal/, and pkg/.
foreach ($surface in @(
  'logging',
  'metrics',
  'rollingfile',
  'runtimeartifact'
)) {
  $import = '"github.com/portpowered/infinite-you/pkg/platform/' + $surface + '"'
  $all = rg -l $import --glob '*.go' cmd internal pkg
  $production = rg -l $import --glob '*.go' --glob '!**/*_test.go' cmd internal pkg
  "pkg/platform/${surface}: all=$(@($all).Count); production=$(@($production).Count)"
}

# Service logging consumers and their family counts.
$loggingImport = '"github.com/portpowered/infinite-you/pkg/platform/logging"'
$all = rg -l $loggingImport --glob '*.go' pkg/services
$production = rg -l $loggingImport --glob '*.go' --glob '!**/*_test.go' pkg/services
"service logging consumers: all=$(@($all).Count); production=$(@($production).Count)"

# Confirm whether an instrumentation tracer exists in non-test Go source.
rg -n -i '\b(opentelemetry|open-telemetry|otel|tracerprovider|newtracer|startspan|spanid|span_id)\b' `
  --glob '*.go' --glob '!**/*_test.go' cmd internal pkg tests
```

The observed surface counts were:

| Direct import surface | All Go files | Production Go files | Counting scope |
| --- | ---: | ---: | --- |
| `pkg/platform/logging` | 180 | 82 | `cmd/`, `internal/`, `pkg/`; exact quoted import path |
| `pkg/platform/metrics` | 13 | 3 | `cmd/`, `internal/`, `pkg/`; exact quoted import path |
| `pkg/platform/rollingfile` | 4 | 3 | `cmd/`, `internal/`, `pkg/`; exact quoted import path |
| `pkg/platform/runtimeartifact` | 26 | 14 | `cmd/`, `internal/`, `pkg/`; exact quoted import path |
| service files importing platform logging | 135 | 58 | `pkg/services/` only; exact quoted import path |

The 135-file service count is the current-tree measurement behind the work
item’s “approximately 137” logging consumers. It is a file count, not a claim
that every file emits a log on every operation.

## Historical operational failure evidence

The supplied incident context is preserved as historical evidence, not as a
current-tree measurement or a deletion condition:

- Four Worker Sessions remained `RUNNING` materially beyond a two-hour
  timeout.
- One leaked Worker Session identifier was absent from daemon stderr.
- Diagnosis required manually joining Work, Worker Session, daemon-log, and
  GitHub-check views.

The incident establishes the required diagnostic outcome: an operator must be
able to identify the aged Worker Session, relate it to its Work and execution
attempt, and locate the relevant daemon evidence without four independent
manual searches. It does not establish which current package is at fault, and
the incident age must not be used as a retention or deletion rule.

## Current observability inventory

### Logging

`pkg/platform/logging` is a present, injected structured logging boundary:

- `pkg/platform/logging/logger.go:10-17` defines the `Logger` interface with
  `Debug`, `Info`, `Warn`, `Error`, and `Verbose` operations.
- `logger.go:19-35` supplies `NoopLogger` and nil normalization;
  `logger.go:38-82` constructs the zap logger at the approved composition
  boundary and selects quiet, normal, verbose, or debug terminal behavior.
- `pkg/platform/logging/runtime_logger.go:59-67` defines the runtime file sink;
  `:228-305` reserves a path, wraps `rollingfile.Writer`, and tees JSON zap
  records into the base logger and the runtime file core. The runtime file
  core is `Info` level and the base logger controls terminal output.
- `runtime_logger.go:329-331` places the default runtime log root at
  `$HOME/.you-agent-factory/logs`; an explicit root is accepted by
  `RuntimeLogOpeningRequest`.
- `runtime_logger.go:168-176` closes the owned writer. The opener creates one
  sink per runtime scope; the scope, not the process-wide owner, owns the
  file lifecycle.

The guarantee is structured log delivery through an injected logger and a
runtime-scoped rolling file. The side effect is zap encoding and filesystem
append/rotation. The current correlation guarantee is weak: the runtime sink
adds `runtime_instance_id` at `runtime_logger.go:294-296`, while other fields
are selected independently by callers. There is no single required set of
Work, Factory Session, Worker Session, stage/transition, and attempt fields.

The consumer families are measured below. Representative production symbols
show the current injection shape: `chat_sessions/wire/wire.go:51`,
`events/wire/wire.go:22`, `factory_definitions/wire/catalog_ports.go:103`,
`factory_runtime/composition_contracts.go:81`,
`factory_sessions/internal/runtimehosting/service.go:57`,
`operator_settings/wire/wire.go:33`, `providers/execute_contract.go:194`,
`recordings/wire/providers.go:57`, `webhooks/wire/wire.go`,
`worker_sessions/publish.go:42`, and `workers/runner_process.go:38`.

| `pkg/services` family | All / production files importing platform logging | Current role |
| --- | ---: | --- |
| `chat_sessions` | 9 / 4 | ACP conversation and response-bridge construction |
| `events` | 2 / 2 | source-native event stream construction and storage |
| `factory_definitions` | 7 / 5 | catalog, package, and validation paths |
| `factory_runtime` | 30 / 10 | runtime orchestration, engine, scheduler, and composition contracts |
| `factory_sessions` | 3 / 3 | hosted runtime/session boundary |
| `models` | 2 / 0 | test-only transport bootstrap consumers |
| `operator_settings` | 14 / 4 | settings and construction boundaries |
| `providers` | 20 / 4 | provider execution contract and adapters |
| `recordings` | 11 / 7 | recording, replay, and worker-capture construction |
| `webhooks` | 3 / 2 | webhook service and wire construction |
| `worker_sessions` | 13 / 3 | Worker Session publication and supervision |
| `workers` | 21 / 14 | request execution, command runners, and workstations |

The family rows are reproduced by the service-only command in the previous
section, grouping each result by the first directory below `pkg/services`.
The rows intentionally include tests so the migration denominator is not
silently narrowed; the production column is the runtime caller denominator.

### Metrics

Metrics are partial: there is a logical emitter and a durable file sink, but no
aggregation or external exporter.

- `pkg/platform/metrics/runtime_metrics.go:37-48` defines
  `RuntimeMetricsSink` as a mutex-protected JSON encoder over an `io.Closer`.
- `runtime_metrics.go:62-127` requires runtime identity, root, start time, and
  collision inputs, reserves a metrics path, and applies a rolling-file
  configuration. The default root is `$HOME/.you-agent-factory/metrics` at
  `:241-244`.
- `runtime_metrics.go:163-192` owns close state and encodes each
  `WriteMetric` call as one JSONL record. The `record any` parameter and direct
  encoder write mean the platform sink does not validate a metric schema,
  aggregate points, or batch/export them.
- The Factory Runtime contract narrows the logical vocabulary to `Counter`,
  `Gauge`, and `Sample` in `pkg/services/factory_runtime/metrics.go:18-27`.
  `pkg/services/factory_runtime/runtime_metrics_sink.go:16-35` projects a
  `RuntimeMetricRecord` with timestamp, name, type, value, unit, runtime scope,
  dispatch, Work, domain `TraceID`, workstation, worker type, provider,
  outcome, and reason. The runtime host emits queue, dispatch, provider,
  script, lifecycle, state, and in-flight measurements in
  `internal/host/metrics.go:51-164` and forwards failures as warnings at
  `:276-303`.

The guarantee is one serialized record per logical emission when the sink is
open; the side effect is owner-only JSONL file output through the rolling
writer. `Open` and `Close` bound the runtime scope. There is no current
aggregation, cardinality policy, metric backend, OTLP exporter, or metric
exemplar contract. The three production direct-import consumers are
`pkg/wire/profiles.go:593-630`, `pkg/wire/runtime_inputs.go`, and
`pkg/transports/cli/runconfig/config.go`; the remaining ten matches are test
consumers or boundary fixtures.

### Rolling-file and runtime-artifact mechanics

`pkg/platform/rollingfile` is a policy-free filesystem writer shared by the
runtime log and metrics sinks:

- `rollingfile.go:30-45` defines `Writer` with size, age, backup, local-time,
  compression, and mutex state.
- `:217-245` rotates before an oversized write; `:250-267` exposes `Close` and
  explicit `Rotate`.
- `:301-324` creates the file and performs retention synchronously;
  retention errors are diagnostic-only and do not turn a successful write into
  a runtime failure.
- `:60-143` and `:337-362` provide checkpoint/rollback and reservation-aware
  rotation for callers that couple a file record to another effect.

Its guarantee is serialized append, bounded size rotation, retention, and
optional compression. Its side effect is local filesystem mutation. Its
lifecycle is owned by the caller: `Close` closes the active file, while the
writer’s documented later-write reopening behavior means wrapper sinks must
enforce a closed-state policy. It has no knowledge of Work, sessions, stages,
attempts, metrics, or exporters.

`pkg/platform/runtimeartifact` owns collision-safe path reservation and
immutable metadata, not observability policy:

- `reserve.go:18-27` defines the injected `FileSystem` effect and `Reserver`
  port; `:38-67` creates dated, collision-safe paths and reserves them with
  owner-only `0600` permissions.
- `diagnostics.go:5-18` exposes immutable log/metrics path and retention
  metadata without writers or lifecycle handles.
- The private path vocabulary used by log and metric openers is under
  `pkg/platform/internal/runtimeartifact` and is reached by those platform
  owners; it does not become a service-facing policy surface.

The reservation side effect is directory creation and exclusive file creation;
the reservation file is closed immediately and the selected sink owns later
writes. No runtime-artifact package exports an exporter or trace facility.

### Enforcement surface

`cmd/loggingboundarycheck` is a production logging-construction guard, not a
signal emitter:

- `main.go:19-25` identifies the platform logging path and the three approved
  logger-construction files.
- `:27-56` lists prohibited process-global zap, slog, log, and
  `BuildLogger` acquisitions.
- `:70-128` scans production `.go` files under `cmd`, `internal`, and `pkg`,
  skipping tests, vendor, and testdata; `:131-169` reports AST call findings.
- `Makefile:780-781` runs it as the `logging-boundary-check` lint target.

The checker’s side effect is diagnostic stdout/stderr and a non-zero process
result on findings. Its lifecycle is one lint invocation. It catches logger
acquisition regressions but does not check metric schema, correlation fields,
trace/span creation, sensitive-data redaction, or cross-signal joinability.

## Three-pillar classification

| Pillar | Current classification | Evidence and boundary |
| --- | --- | --- |
| Logging | Present, with partial correlation | Injected `Logger`, zap terminal construction, and runtime rolling-file tee in `pkg/platform/logging/logger.go:10-82` and `runtime_logger.go:247-305`; required cross-service context is not enforced. |
| Metrics | Partial | Factory Runtime has logical counter/gauge/sample contracts and a typed projected record, while `pkg/platform/metrics` writes one unaggregated `any` value to JSONL in `runtime_metrics.go:177-192`; no exporter or backend exists. |
| OpenTelemetry-style tracing | Absent | The boundary-aware non-test scan above returned zero matches, and `go.mod`/`go.sum` contain zero `opentelemetry`, `open-telemetry`, or `otel` matches. |

The absence classification is about an instrumentation facility: there is no
tracer, span API, context propagation implementation, SDK, exporter, or wire
protocol. It is not contradicted by domain fields named `TraceID`.

## Trace-name disambiguation

The repository uses “trace” for customer Work lineage and invocation
correlation, not for spans:

- `pkg/services/work/contracts.go:129` and `:170-172` define Work and chaining
  `TraceID` values that are public/domain lineage fields.
- `pkg/services/factory_runtime/runtime_metrics_sink.go:27-35` stores that
  domain value as an optional metric record field; it does not start a span.
- `pkg/services/recordings/internal/replay/event_reducer.go:356-411` restores
  Work and chaining trace IDs during canonical event replay.
- `pkg/services/factory_sessions/internal/invocation/session_telemetry.go:171-235`
  copies request/result `trace_id` into structured invocation logs; it does not
  expose a tracer or span lifecycle.
- The token helpers in
  `pkg/services/factory_runtime/internal/services/orchestration/token/lineage.go:5-26`
  compute predecessor/current Work lineage. No `SpanID`, `StartSpan`, or
  tracer-provider symbol is present in the audited non-test source.

These identifiers are useful future join keys, but treating them as trace
instrumentation would falsely report the traces pillar as implemented.

## Current behavioral guards and known gaps

The current tree has direct behavior coverage for local mechanics:

- Logger contract, no-op behavior, verbosity, runtime path layout, rotation,
  close behavior, legacy-path handling, and concurrent collision avoidance are
  exercised by `pkg/platform/logging/logger_test.go:38-162`,
  `runtime_logger_test.go:85-443`, `runtime_log_opener_test.go:13-25`, and
  `runtime_artifact_layout_test.go:21-160`.
- Metrics defaults, path/open validation, JSONL envelope stability, close
  behavior, concurrent path/correlation isolation, and rotation are covered by
  `pkg/platform/metrics/runtime_metrics_test.go:18-472` and the Factory Runtime
  projection tests in `pkg/services/factory_runtime/runtime_metrics_sink_test.go:24-104`.
- Rolling rotation, compression, retention, checkpoint/rollback, and invalid
  input behavior are covered by `pkg/platform/rollingfile/rollingfile_test.go:17-476`.
- Owner-only artifact permissions, dated paths, and collision avoidance are
  covered by `pkg/platform/runtimeartifact/reserve_test.go:24-124`.
- Logger-construction enforcement is covered by
  `cmd/loggingboundarycheck/main_test.go:10-95`.
- Public runtime log creation and path policy are exercised by
  `tests/functional/runtime_api/api_runtime_log_policy_test.go:25-84`.
- Public status, Factory Session, Work, and Factory Event projections are
  aligned through runtime transitions by
  `tests/functional/runtime_api/api_service_mode_observability_smoke_test.go:44-65`.
- The current-main throttled-lane guard,
  `tests/functional/runtime_api/api_provider_throttle_pause_observability_test.go:19-87`,
  proves a failed provider lane does not strand an unaffected lane and that
  public in-flight state returns to zero. It is an event/state behavior guard,
  not a log/metric/trace join guard.

The gaps relevant to the incident are equally explicit:

- No current guard emits and joins one Work ID, Factory Session ID, Worker
  Session ID, stage/transition, and attempt across a log record, metric
  record, and span; spans do not exist.
- The public Worker Session snapshot is identity/state/result/provider
  association/lineage in `pkg/services/worker_sessions/session.go:8-24`. The
  existing tests cover terminal classification, continuation, cursor, and
  process-exit behavior, but do not directly prove a two-hour Worker Session
  age diagnosis with matching daemon-log evidence.
- The incident’s leaked identifier path has no direct behavioral assertion
  that the same Worker Session ID is present in daemon stderr, runtime logs,
  metrics, and a trace. Existing log policy and event projection tests prove
  their individual surfaces only.

This gap inventory is a baseline for the later packet work. It is not a source
scan test and must not be turned into a meta-test; future proof must exercise
observable runtime outputs and emitted signals.
