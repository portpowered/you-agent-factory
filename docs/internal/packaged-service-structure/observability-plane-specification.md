# Observability Plane Packet Specification

Status: Story 004 deletion, enforcement, verification, and delivery closure
authored; the current-tree inventory, target-contract baseline, and post-P6
packet register are complete.

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

## Target signal, correlation, and context contracts (Story 002)

This section fixes the target contract that later packets may implement. It is
still documentation-only: no platform package, interface, exporter, or module
dependency is added here. Any packet that adds one of those surfaces is
post-BTRC-P6 work and must use the public roots left by P6. The existing logger
and JSONL metrics sink remain usable while the target is introduced alongside
them.

### OpenTelemetry decision

The decision is **both**: OpenTelemetry is the instrumentation API and OTLP is
the vendor-neutral wire protocol. The qualification is important for the
repository boundary: service packages consume narrow, platform-owned signal
contracts; they do not import an OpenTelemetry SDK, construct a provider, or
know which exporter is configured. Platform adapters use the OpenTelemetry Go
API/SDK to implement those contracts, and isolated exporters use OTLP when an
external or local backend is selected.

The existing injected `pkg/platform/logging.Logger` remains the call-site
facade during migration. Its target structured record has OpenTelemetry-style
severity, attributes, resource, and correlation fields, and a platform adapter
may map that record to OpenTelemetry log data. Direct service imports of an
OpenTelemetry logging API are not required: keeping the repository logger as
the stable facade avoids a 135-file call-site rewrite before a replacement has
been proved. The target decision therefore applies consistently to all three
signals without turning an exporter or SDK into a service dependency.

| Choice | Decision | Reason |
| --- | --- | --- |
| OpenTelemetry instrumentation API plus OTLP wire | Adopt | Gives traces and metrics one context and lifecycle model, preserves vendor-neutral export, and permits in-memory providers and fake exporters in tests. The structured log adapter can join the same trace context without changing log ownership. |
| OpenTelemetry API without a wire decision | Reject | It would leave export, batching, shutdown, and backend interoperability to each future caller and would reproduce the current “records exist but cannot be joined or exported” gap. |
| OTLP as the caller API | Reject | OTLP is an interchange envelope, not a service-facing operation contract. Making services build protocol payloads would leak transport policy, increase coupling, and make local tests depend on an exporter representation. |
| A vendor SDK or a repository-specific tracing protocol | Reject | Either choice creates vendor lock-in or another bespoke identity/lifecycle model. Existing domain `TraceID` values are not promoted into a substitute tracer. |
| Replace the logger and runtime sink in place | Reject | The current logging and metrics consumers must continue to work while bounded caller families migrate. A strangler path is required; deletion belongs to a later packet with a zero-consumer condition. |

This lane does not edit `go.mod` or `go.sum`. A later implementation packet may
add the smallest pinned OpenTelemetry API, SDK, and OTLP modules after BTRC P6,
with SDK and exporter imports isolated to platform implementation and
`pkg/wire`. The API/facade, provider construction, exporter selection, and
transport are separate concerns:

- Service roots choose event names and map already-known domain values into
  platform contracts. They do not select sampling, aggregation, endpoints,
  credentials, retry policy, or exporter type.
- `pkg/platform/observability` owns the immutable correlation/context value
  and the common signal references. `pkg/platform/logging`,
  `pkg/platform/metrics`, and a target `pkg/platform/tracing` surface own
  their signal-specific contracts and compatibility adapters. These are
  policy-free ports, not service locators.
- `pkg/wire` constructs one provider from explicit configuration and injects
  narrow logger, meter, tracer, and diagnostic ports. `pkg/initializer` owns
  process startup and shutdown of the already-constructed provider. No
  service opens an exporter or reaches into another service's internals.
- `pkg/platform/runtimeartifact` and `pkg/platform/rollingfile` remain
  filesystem effects. They may back a local diagnostic or compatibility sink,
  but they do not decide signal names, correlation policy, sampling, or
  export destinations.

The provider is process-scoped while a local log/metric artifact remains
runtime-scoped. Shutdown uses a bounded flush deadline, closes exporters once,
and preserves the first product or lifecycle error. An exporter outage,
serialization rejection, queue overflow, or flush timeout is reported through
an already-available safe diagnostic path and must not change a Work,
Factory Session, Worker Session, or attempt result. Queues are bounded and
their drop/overflow count is itself a low-cardinality diagnostic metric. A
provider may fail closed for its own signal, but it must never block or fail
the product operation merely because an observability destination is down.

### Platform contract and ownership boundary

The target public shape is a set of narrow contracts, not a new service
registry. The following names describe the later package responsibilities;
they are not files added by this authoring story.

| Contract | Policy-free owner | Required behavior | Explicitly out of scope |
| --- | --- | --- | --- |
| Immutable `Correlation` and context attach/extract helpers | `pkg/platform/observability` | Copy values on enrichment, validate safe scalar IDs, preserve absent state, and provide a bounded `SignalRef` for joins. | Loading Work, Factory Session, Worker Session, or provider records; deciding which operation owns an ID. |
| Context-aware structured logger and record sanitizer | `pkg/platform/logging` | Accept `context.Context`, enrich records from the correlation scope, validate an allowlisted record shape, and route to terminal/runtime sinks or an adapter. | Choosing service log levels/events, reading global context, or serializing raw product payloads. |
| Typed meter, descriptor, point, and exemplar/diagnostic writer | `pkg/platform/metrics` | Validate names/units/attributes, aggregate only by bounded dimensions, attach a controlled join reference, and isolate writer/exporter failures. | Defining business meaning from a token or event, using Work/session IDs as dimensions, or opening a second runtime graph. |
| Context-aware tracer/span lifecycle | target `pkg/platform/tracing` | Start child spans from `context.Context`, propagate parent identity, enforce one terminal `End`, and map typed outcome/error status. | Choosing a factory scheduling policy, inferring terminal state, or owning canonical Factory replay. |
| Provider/exporter lifecycle | `pkg/wire` plus `pkg/initializer`; implementation under `pkg/platform` | Construct once, inject explicitly, flush/close with a deadline, and isolate destination failures. | Service-owned exporter construction, mutable global correlation, or provider/session business state. |

Every service call continues to accept the repository's existing
`context.Context` boundary. A service may receive a narrow logger, meter, or
tracer interface directly through its root construction, but it may not import
another service's `internal` package, use a grab-bag observability container,
or recover a peer contract through a type assertion. Domain mapping stays in
the owning service; signal emission stays behind the platform port.

### Correlation scope

The target correlation scope is an immutable value attached to
`context.Context`. It contains a field state as well as a value so “not known”
is never confused with an empty identifier:

| Field | Stable emitted name | Meaning and source | Allowed signal locations |
| --- | --- | --- | --- |
| Work ID | `work_id` | Customer-visible Work identity from the owning Work/request boundary. | Logs, spans, metric exemplar/diagnostic record; never a metric aggregation attribute. |
| Factory Session ID | `factory_session_id` | Live Factory Session identity, not a logical-session key or runtime artifact path. | Logs, spans, metric exemplar/diagnostic record; never a metric aggregation attribute. |
| Worker Session ID | `worker_session_id` | Execution/control context for one Worker Session. | Logs, spans, metric exemplar/diagnostic record; never a metric aggregation attribute. |
| Stage | `stage` | The bounded runtime stage/workstation scope known to the caller. | Logs and spans; metrics use a code-owned stage class only when a bounded dimension is justified. |
| Transition | `transition` | The transition or operation boundary being attempted. | Logs and spans; never an arbitrary user-defined metric label. |
| Attempt | `attempt_id` | Opaque execution-attempt identity; the optional `attempt_number` is a bounded retry ordinal, not a replacement identity. | Logs, spans, metric exemplar/diagnostic record; never a metric aggregation attribute. |
| Runtime instance | `runtime_instance_id` | One active runtime/artifact scope when the operation has one. | Logs, spans, and diagnostic records; not a default metric aggregation attribute. |
| Dispatch | `dispatch_id` | One request-scoped dispatch association when dispatch exists. | Logs, spans, and diagnostic records; not a default metric aggregation attribute. |
| Request | `request_id` | Public or internal request identity when one exists. | Logs, spans, and diagnostic records; not a default metric aggregation attribute. |
| Domain lineage | `work_trace_id` | Existing Work/chaining `TraceID` used for customer lineage. It is deliberately named differently from instrumentation trace identity. | Logs, spans, and diagnostic records; never an instrumentation `trace_id` or metric label. |
| Instrumentation trace/span | `trace_id`, `span_id` | OpenTelemetry trace/span identity. A child span keeps `trace_id` and receives a new `span_id`. | Span records and one controlled metric exemplar; log fields are copied from the active span when present. |

The first six rows are the mandatory correlation set. “Mandatory” means that
the signal contract carries a state for every row and emits the value as soon
as the owning boundary knows it; it does not authorize inventing an ID before
admission. Additional rows are included only when the operation has that
scope. The current `SessionID`, `TraceID`, and runtime fields are mapped to the
explicit names above at the owning boundary so a Factory Session, a domain
lineage trace, and an instrumentation trace cannot be mistaken for one
another.

Field state has exactly these meanings:

- `known` has a non-empty, validated scalar value and is serialized as the
  named field.
- `not_applicable` means the operation cannot have that scope, such as a
  process-start record before a Factory Session exists. The field is omitted
  and its fixed field-state entry says `not_applicable`.
- `not_yet_assigned` means the operation is before the owning admission or
  attempt boundary. It may appear on a start record but must be enriched on
  the next boundary and on every terminal record.
- `unavailable` means a required source failed to provide an already expected
  value. It is omitted, recorded in the fixed missing-field set, and is an
  observability contract violation to be counted and reviewed; empty strings,
  zero UUIDs, `unknown`, and fabricated placeholders are not valid substitutes.

The scope and its field-state map are copied on every enrichment. A child
operation can add `attempt_id`, `stage`, or `transition`, but cannot mutate its
parent scope. The context key is private to the platform package and holds no
service object, mutable map, channel, or cancellation handle.

### Structured log contract

The target log record has a stable envelope and an allowlisted field set:

| Group | Fields | Contract |
| --- | --- | --- |
| Envelope | `timestamp`, `severity`, `service`, `event`, `operation` | Names are code-owned and bounded; `event` is a stable state/operation fact, not a free-form sentence. |
| Outcome | `outcome`, `error_class`, `retryable`, `duration_ms` | Outcome is an enum (`started`, `succeeded`, `failed`, `cancelled`, `timed_out`, or `degraded`); errors use a stable class/code and never require raw error text. |
| Correlation | The mandatory set and any known optional fields above | Enriched from the context scope; per-record IDs are safe in logs but are not metric dimensions. |
| Detail | Bounded scalar fields such as `provider_family`, `worker_type`, `queue`, or `artifact_kind` | Each field has an owner, type, maximum length, and redaction rule. User-defined names are normalized or omitted when they exceed the field budget. |
| Join | `trace_id`, `span_id`, `signal_ref` | `signal_ref` is fixed-length and opaque; it is not a secret and is not a metric label. |

New call sites use a context-aware record operation (with level-specific
helpers allowed as thin sugar). The compatibility adapter may translate the
current variadic key/value logger calls, but it must reject or sanitize
untyped fields rather than silently expanding the schema. Successful
high-frequency operation logs may be sampled or summarized by platform policy;
start, terminal failure, timeout, stale-session, and exporter-failure records
remain observable and retain their correlation.

Redaction is allowlist-first at the platform boundary. Logs and diagnostics
must not contain API keys, access tokens, cookies, authorization headers,
prompts, model output, raw Work payloads, environment contents, or unbounded
command/error text. An error is represented by a stable class, retryability,
and bounded sanitized detail when explicitly allowed. IDs and paths are
included only when their owning contract marks them safe; paths do not replace
correlation IDs.

### Typed metric contract and cardinality

The target meter accepts a typed descriptor before a point can be emitted:

```text
MetricDescriptor {
  name: stable code-owned name
  kind: Counter | Gauge | Histogram
  unit: stable unit (`1`, `s`, `ms`, `{work}`, `{session}`, ...)
  description: operator-facing meaning
  allowed_attributes: finite typed attribute schema
  aggregation: Sum | LastValue | ExplicitBucketHistogram
}
MetricPoint {
  descriptor: MetricDescriptor
  value: typed numeric value
  attributes: validated low-cardinality values
  signal_ref: fixed-length diagnostic join reference
  exemplar: at most one {trace_id, span_id} pair when a span exists
}
```

`Sample` remains a compatibility input name for the existing Factory Runtime
contract, but it is not a target escape hatch: a packet must declare whether a
sample is a `Gauge` or a `Histogram` before migrating it. Names, units,
descriptions, and aggregation cannot be supplied by a Work payload or a
runtime token. A representative initial vocabulary is:

| Metric name | Kind / unit | Allowed dimensions and aggregation | Diagnostic purpose |
| --- | --- | --- | --- |
| `factory.worker_session.age` | Gauge / `s` | `state` from a fixed terminal/non-terminal enum; last value. | Directly exposes an aged Worker Session without putting its identity in the series key. |
| `factory.worker_session.active` | Gauge / `{session}` | `state` from a fixed enum; last value. | Shows active/stale/terminal population by bounded state. |
| `factory.dispatch.in_flight` | Gauge / `{dispatch}` | `lane` from a code-owned bounded enum; last value. | Shows executor-slot pressure without a dispatch or Work label. |
| `factory.worker_attempt.duration` | Histogram / `s` | `outcome` and bounded `worker_family`; explicit buckets. | Compares attempt latency and timeout behavior without attempt identity. |
| `factory.worker_attempt.terminal` | Counter / `{attempt}` | `outcome` and bounded `error_class`; sum. | Proves exactly-once terminal classification by bounded outcome. |
| `factory.observability.export_failures` | Counter / `{failure}` | `signal` and `destination_kind`; sum. | Makes exporter isolation and dropped diagnostics visible. |

The allowed-dimension rule is strict: service/operation, bounded outcome,
error class, fixed lane or family, and other explicitly registered finite
values may be dimensions. Work ID, Factory Session ID, Worker Session ID,
stage name, transition name, attempt ID, runtime instance, dispatch ID,
request ID, domain `work_trace_id`, instrumentation `trace_id`, and `span_id`
are never aggregation dimensions. If a future metric needs stage detail, it
must use a finite code-owned stage class or remain a log/span concern.

The metric writer aggregates by descriptor plus allowed dimensions only. Each
exported point carries one bounded `signal_ref`; the local diagnostic stream
stores the full correlation scope under that reference, and an active span is
also attached as the single exemplar. The diagnostic record is not an
aggregation label and is capped by a fixed record size and runtime retention
policy. A point and its diagnostic reference are committed together: if the
bounded diagnostic record cannot be written, the point is dropped and the
failure is counted rather than exporting an unjoinable point. Thus operators
can join every exported point to a bounded diagnostic record or span without
turning unique Work/session identities into unbounded metric series.

### Trace and span lifecycle

The target tracer surface is context-first:

1. `Start(ctx, SpanSpec)` creates a child span with a low-cardinality,
   code-owned name and returns a derived context plus a span handle.
2. The span copies the current correlation scope, adds only fields learned at
   that boundary, and carries the instrumentation `trace_id`/new `span_id`.
3. Cross-service, worker-attempt, provider, filesystem, and external-process
   boundaries create child spans or explicit links. A retry is a new attempt
   and a new child span; it never overwrites the prior attempt's identity.
4. The owner ends a span exactly once, including cancellation and error paths.
   Success maps to `OK`; typed failure, timeout, or an unexpected cancellation
   maps to an error outcome with a stable `error_class`; caller-requested
   cancellation is represented as `cancelled` without copying raw error or
   payload text.
5. Span attributes contain safe correlation and bounded operation fields.
   Prompt, output, credentials, arbitrary Work content, and raw subprocess
   stderr are events owned by other diagnostic contracts and are not span
   attributes by default.

The instrumentation trace identity is never populated from the Work/domain
`TraceID`. If no parent exists, the provider starts a new root trace and the
domain lineage remains an independent optional field. If tracing is disabled
or a provider is unavailable, logs and metric diagnostic records still carry
the platform correlation scope and a `trace_unavailable` state; service
correctness does not depend on a span being present.

### Context propagation and boundary rules

Correlation is propagated through immutable values on `context.Context`:

- A public operation receives the caller context, attaches the identities it
  owns, and passes the derived context to every service-root call. The caller
  preserves cancellation and deadline values; observability enrichment never
  replaces them.
- A child stage/transition or attempt uses a new context value containing a
  copied scope plus its new fields. It does not mutate a parent scope or use a
  package-level current-operation variable.
- An asynchronous handoff captures the copied scope and the parent context
  before launching the goroutine. Normal work retains parent cancellation and
  deadline. Deliberate cleanup/terminal publication that must outlive a
  request uses an explicitly owned detached context (for example,
  `context.WithoutCancel` followed immediately by a bounded deadline and
  cancel function), and its owner waits for completion. Detachment is never a
  hidden way to keep product work running.
- A process or provider boundary carries the OTel propagation carrier plus an
  allowlisted correlation envelope for the domain IDs. The receiver creates a
  child span and attaches a new immutable scope. Domain IDs are not accepted
  from arbitrary user headers, and no correlation is stored in global
  environment variables or mutable process state.
- A missing value is propagated as its explicit field state. Context
  enrichment cannot turn `not_yet_assigned` into `known`; only the owning
  admission/session/attempt boundary may do that. Terminal signals validate
  that the mandatory set is known or explicitly not applicable and count any
  `unavailable` field as a contract violation.

This makes the join path deterministic: a Work/Factory Session/Worker Session
operation starts with a scope, child spans and logs retain the same scope,
attempt and stage transitions extend it, and metric points reference a bounded
diagnostic record carrying that same scope. There is no hidden global mutable
correlation state and no requirement that a service import a peer's internal
record to emit a signal.

### Target contract verification

Later implementation packets must prove these contracts with observable
behavior, not source inventories:

- platform unit tests copy and enrich scopes without parent mutation, preserve
  cancellation/deadlines, reject unsafe fields, enforce descriptor schemas,
  cap metric dimensions, and keep a point/diagnostic reference atomic;
- platform/provider integration tests exercise in-memory spans, structured
  logs, metric exemplars, exporter failure, bounded flush, close idempotence,
  and the no-op/degraded provider path;
- service/root tests pass one context through a public operation, child
  attempt, asynchronous handoff, and terminal publication, then assert the
  emitted records contain the same known correlation values and distinct child
  span/attempt identities;
- later functional/race tests must observe stale Worker Session age,
  executor-slot pressure, timeout/terminal evidence, and cross-signal joining
  through emitted logs, metrics, traces, and the public Worker Session view.

These tests must assert emitted records, lifecycle outcomes, and failure
isolation. They must not scan source files, count registrations, inspect route
inventories, or treat documentation topology as behavioral proof.

## Story 002 target-contract closure

This story fixes the target observability posture as OpenTelemetry
instrumentation plus OTLP wire, assigns policy-free ownership under
`pkg/platform`, defines typed safe signals and the full correlation set, and
specifies immutable context propagation and bounded metric joins. It leaves
the current logger and metrics sink in place, adds no production source or
dependency, and places all package/interface-adding implementation after
BTRC P6. `make typecheck` passes on this checkout.

## Post-BTRC-P6 implementation packet register (Story 003)

This register is the implementation sequence for the target above. It is not
permission to change this authoring branch's production tree. Every packet
leaves `main` releasable, introduces a replacement before moving callers, and
keeps the current logger, runtime log artifact, and JSONL metrics sink usable
until a later packet proves their deletion condition.

### Ordering gate and packet-wide rules

The BTRC plan's P6 retirement packet is a hard prerequisite. `OBS-01` through
`OBS-07` may be implemented only after BTRC P6, including its applicable
P6-A–P6-D deletion slices, has merged. The implementation owner must rerun the
caller audit from the then-current `main` before starting `OBS-01`; a missing
or already-deleted BTRC surface is recorded as absent, never recreated. This
avoids targeting the active secondary composition graph and makes the public
post-P6 roots the only construction boundary.

All seven packets add, change, or remove a package/interface surface and are
therefore explicitly post-BTRC-P6 work. OpenTelemetry module changes, if
needed, belong only in `OBS-02` or a later packet and must not be backported
into the BTRC lane. No packet designs, implements, or reserves a resumption
plane, resumption state, resumption API, or resumption migration seam.

Every implementation packet consumes the post-P6 public service roots and
`pkg/root.BuildProcess`/`pkg/wire` construction boundary. It must not import
or reconstruct `internal/runtimeopening`, `internal/executionopening`,
grouped service aliases, obsolete Wire providers, or any BTRC compatibility
graph that P6 retired. A packet that discovers one of those paths as a
remaining caller stops its deletion row, characterizes the behavior, and
routes the caller through the named public successor before continuing.

Each estimate below is bounded for one sitting review and is below roughly
800 changed lines. If implementation crosses roughly 800 changed lines, or a
reviewer cannot hold the caller set in one change, split before coding by
signal or caller family. Both resulting slices must build and preserve the
old path; a split may not introduce a second provider graph, an unowned
compatibility package, or a later-packet dependency that restores behavior.

### Current caller register for packet planning

The baseline's 135 service files are only one part of the migration. The full
direct-import measurement below prevents the packet list from silently
leaving transport, Wire, test-support, or platform compatibility callers
behind. Counts are files, not log calls; “production” excludes every
`*_test.go`. The command groups the exact quoted `pkg/platform/logging`
import-path matches already measured in the audit:

```powershell
$import = '"github.com/portpowered/infinite-you/pkg/platform/logging"'
$files = @(rg -l $import --glob '*.go' cmd internal pkg)
$files | ForEach-Object {
  $parts = $_ -split '[\\/]'
  if ($parts.Count -ge 2) { $parts[0] + '/' + $parts[1] } else { $parts[0] }
} | Group-Object | Sort-Object Name | ForEach-Object {
  "$($_.Name): $($_.Count)"
}
# Repeat the same command with --glob '!**/*_test.go' for production files.
```

| Direct-import family | All / production files | Migration owner | Packet |
| --- | ---: | --- | --- |
| `pkg/services` | 135 / 58 | Service roots and their private callers | `OBS-03`–`OBS-06` |
| `pkg/wire` | 22 / 13 | Wire construction and runtime-scope adapters | `OBS-02`–`OBS-04` |
| `pkg/transports` | 11 / 4 | Transport boundary callers | `OBS-05`–`OBS-06` |
| `pkg/platform` | 5 / 4 | Legacy implementation/adapter internals | `OBS-02`, `OBS-07` |
| `internal/testutil` | 3 / 2 | Test-support fixtures | `OBS-06` |
| `cmd/loggingboundarycheck` | 2 / 1 | Static enforcement tool; not a runtime caller | `OBS-06`, `OBS-07` |
| `cmd/pkgboundarycheck` | 2 / 0 | Test-only boundary fixture | `OBS-06` |
| **Total** | **180 / 82** |  |  |

The service-family rows in the baseline expand to 135 all / 58 production
files: `factory_runtime` 30 / 10, `workers` 21 / 14, `providers` 20 / 4,
`operator_settings` 14 / 4, `worker_sessions` 13 / 3, `recordings` 11 / 7,
`chat_sessions` 9 / 4, `factory_definitions` 7 / 5,
`factory_sessions` 3 / 3, `webhooks` 3 / 2, `events` 2 / 2, and `models`
2 / 0. Those values use the service-only command in the audit and are
rechecked before implementation because later source movement makes the
original snapshot stale.

### Packet sequence at a glance

| Packet | Observable endpoint | Depends on | Rough size |
| --- | --- | --- | ---: |
| `OBS-01` | Immutable correlation and typed signals are capturable without changing product behavior. | BTRC P6 | 8–14 files / 250–500 lines |
| `OBS-02` | One Wire/Initializer-owned provider can export or degrade without affecting a product operation. | BTRC P6, `OBS-01` | 18–30 files / 450–700 lines |
| `OBS-03` | A stale Worker Session, executor-slot pressure, timeout, and terminal outcome are directly joinable. | BTRC P6, `OBS-02` | 12–20 files / 350–650 lines |
| `OBS-04` | Direct and child worker/provider attempts carry safe identity through the daemon boundary. | BTRC P6, `OBS-03` | 16–26 files / 450–750 lines |
| `OBS-05` | Factory Session, Work, recording, and asynchronous response signals share the same scope. | BTRC P6, `OBS-03`, `OBS-04` | 14–24 files / 400–700 lines |
| `OBS-06` | Remaining service/transport callers use the replacement and the boundary checker prevents new legacy use. | BTRC P6, `OBS-05` | 18–30 files / 450–800 lines |
| `OBS-07` | Legacy logger/metrics compatibility is deleted only after zero consumers and behavioral proof. | BTRC P6, `OBS-06` | 14–24 files / 300–650 lines |

### OBS-01 — Correlation context and typed signal kernel

#### Outcome and preserved behavior

Add the policy-free kernel for immutable correlation scope, typed log records,
metric descriptors/points, bounded diagnostic references, and context-first
span handles under the public `pkg/platform` boundary. A deterministic
in-memory provider and no-op/degraded provider make the contract observable
without requiring a destination. Existing `pkg/platform/logging.Logger`,
`RuntimeMetricsSink`, rolling files, and runtime artifact paths remain the
working production path. No service caller changes in this packet.

#### Build first and caller migration

1. Re-audit BTRC P6 and freeze the public post-P6 owner roots before adding a
   package or interface.
2. Add the immutable scope, explicit field-state representation, allowlisted
   safe scalar values, typed descriptors, and context attach/extract helpers.
3. Add in-memory capture and no-op implementations before any production
   caller is moved; test providers must expose emitted records, not internal
   maps.
4. Make the replacement canonical for new packet code. Existing callers keep
   using the old facade until their named migration packet.

| Caller family | Current behavior | Migration direction |
| --- | --- | --- |
| `pkg/platform` tests and adapter fixtures | Exercise logger, metric, rolling-file, and artifact mechanics independently. | Add contract-level capture tests beside existing mechanics; do not remove or bypass the existing sinks. |
| `pkg/services`, `pkg/wire`, and `pkg/transports` | Use the current injected logger or runtime metric record path. | No production migration yet; later packets consume the new context-aware ports. |

#### Strangler and deletion row

There is no deletion in `OBS-01`. The successor is the typed platform kernel;
the legacy logger and untyped JSONL metric sink remain available and receive
no new design work beyond compatibility. This is the build-first replacement
that makes later caller packets independently mergeable.

#### Behavioral guards and static gates

| Behavior | Guard | Static gate / tier |
| --- | --- | --- |
| Scope enrichment does not mutate a parent, lose cancellation, or invent an ID. | Platform unit tests attach, derive, cancel, and inspect known, not-applicable, not-yet-assigned, and unavailable fields. | `make pkg-boundary`, `make pkg-structure`, `make pkg-file-count`, and `make typecheck`; local platform unit tier. |
| Safe records and metric schemas reject arbitrary payloads and unique metric dimensions. | In-memory capture tests assert redaction, fixed field budgets, bounded attributes, and explicit metric rejection. | `go vet` and focused `pkg/platform/...` tests; no source-inventory test. |
| Trace/span handles have one terminal lifecycle. | In-memory span tests observe child identity, status/error mapping, cancellation, and idempotent/no-op behavior. | PR backend unit tier. |

#### Size, ownership, dependency, and endpoint

Platform observability/logging/metrics owners own the packet. It depends only
on merged BTRC P6. If it exceeds roughly 800 lines, split correlation/context
from signal descriptors/capture, with both slices retaining the old paths.
The independently mergeable endpoint is a compiling, tested kernel and
capture provider with no change to customer runtime behavior.

### OBS-02 — Isolated provider and exporter lifecycle

#### Outcome and preserved behavior

Construct one observability provider through `pkg/wire`, activate and unwind
it through `pkg/initializer`, and keep exporter, queue, flush, and shutdown
failure isolated from Work, Factory Session, Worker Session, and attempt
outcomes. OTLP configuration and OpenTelemetry module dependencies, if
adopted by implementation, land here after BTRC P6. The local diagnostic
writer may continue to use `rollingfile.Writer` and `runtimeartifact.Reserver`;
those effects remain policy-free. Existing terminal/runtime log output and
JSONL metrics remain observable during the bridge.

#### Build first and caller migration

1. Add explicit provider configuration, bounded queue/drop accounting, a
   no-op default, in-memory exporter, and bounded flush/close ownership.
2. Add the OTLP adapter behind the provider boundary; services do not import
   an SDK or build OTLP payloads.
3. Add Wire construction and Initializer lifecycle hooks, proving process
   construction remains inert and startup failure unwinds acquired roles.
4. Add a compatibility bridge that can dual-write a safe record to the new
   provider and the current local sink. The bridge does not select service
   policy or create a second composition graph.

| Caller family | Current behavior | Migration direction |
| --- | --- | --- |
| `pkg/wire` (22 / 13 logging-import files) | Constructs injected loggers, runtime log owners, and metric owners. | Keep construction in Wire; inject the provider-owned narrow ports and retain legacy adapters until `OBS-07`. |
| `pkg/initializer/runtimeapplication` and runtime artifact owners | Own activation, close, and local artifact diagnostics. | Own only provider lifecycle and bounded flush; preserve artifact ownership and primary-error cleanup semantics. |
| `pkg/transports/cli/runconfig/config.go` | Directly imports the platform metrics configuration. | Carry explicit observability configuration to Wire; remove the direct metrics dependency only after the replacement is active. |
| `pkg/platform/logging` and `pkg/platform/metrics` compatibility code | Opens local sinks and serializes legacy records. | Adapt behind the provider; do not delete local sinks in this packet. |

#### Strangler and deletion row

`BuildLogger`, runtime log opening, `RuntimeMetricsOpener`, and
`RuntimeMetricsSink` are retained as compatibility paths. The named successor
is the Wire/Initializer-owned provider plus its local diagnostic adapter. A
later deletion row may remove only a compatibility constructor whose last
caller has migrated and whose provider lifecycle guard is green.

#### Behavioral guards and static gates

| Behavior | Guard | Static gate / tier |
| --- | --- | --- |
| Provider construction is inert and lifecycle is owned once. | Root/lifecycle tests build without effects, activate once, close once, and join cleanup errors without masking the primary error. | `make logging-boundary-check`, `make pkg-boundary`, `make pkg-structure`, and `make typecheck`; local root/lifecycle tier. |
| Export failure cannot change a product result. | In-memory/failed exporter tests observe a successful, failed, cancelled, and timed-out operation with the same product outcome and a bounded failure diagnostic. | `go vet`; focused platform/Wire/Initializer integration tier. |
| Flush and queue behavior is bounded and visible. | Tests observe drop/overflow and flush timeout counters, close idempotence, and no write after close. | PR backend integration tier. |

#### Size, ownership, dependency, and endpoint

Platform implementation, Wire construction, and Initializer lifecycle owners
share this packet. It depends on BTRC P6 and `OBS-01`; its module/dependency
change is explicitly after BTRC P6. If it exceeds roughly 800 lines, split
provider lifecycle from OTLP/local-diagnostic adapters, keeping the no-op
provider and old sinks usable in each slice. The endpoint is an opt-in,
failure-isolated provider on the canonical process graph.

### OBS-03 — Worker Session age, slot, timeout, and terminal slice

#### Outcome and preserved behavior

This is the first behavior-delivering packet. `factory_runtime` and
`worker_sessions` emit the direct evidence needed for the supplied incident:
Worker Session age, executor-slot/in-flight pressure, timeout classification,
and exactly one terminal outcome. A bounded metric series points to a
diagnostic record, while logs and spans carry the full safe correlation set.
The public Worker Session view, Factory Runtime transitions, cancellation, and
existing local artifacts remain unchanged. The four-instrument manual join is
replaced by one observable signal path; the historical incident date is not a
retention or deletion rule.

#### Build first and caller migration

1. Characterize current Worker Session identity/state/result and Runtime
   dispatch/timeout behavior at public roots using the existing tests.
2. Attach immutable scope at Work/Factory Session/Worker Session admission and
   extend it at stage, transition, dispatch, and attempt boundaries.
3. Emit the new age, active, in-flight, timeout, and terminal signals beside
   the old logger and `RuntimeMetricsSink`; prove both paths before switching
   the migrated calls to the replacement as canonical.
4. Move bounded production caller batches, then their tests and fixtures. Do
   not remove the old adapter while any family still needs it.

| Caller family | Current evidence | Migration direction |
| --- | --- | --- |
| `pkg/services/worker_sessions` (13 / 3) | Publication, supervision, process-exit, and lifecycle log consumers; public identity/state already exists. | Use the session root's context-aware log/span port; terminal and stale records must carry `worker_session_id`, Work, Factory Session, stage/transition, and attempt state. |
| `pkg/services/factory_runtime` (30 / 10) | `internal/host/metrics.go`, dispatch/scheduling/lifecycle logging, and runtime metric projection. | Emit typed age, active, in-flight, timeout, and terminal points; keep domain `RuntimeMetricRecord` as a compatibility mapper until its final caller moves. |
| `pkg/wire` runtime scope adapters | Opens runtime log/metric scopes for the above services. | Pass provider-owned ports and copied scopes; keep artifact paths and close behavior stable. |
| Public runtime/Worker Session functional callers | Existing status, session, and throttle observability cells. | Extend public scenarios to observe emitted signals and the same Worker Session ID, not source topology. |

#### Strangler and deletion row

No old surface is deleted. `pkg/platform/logging.Logger`, the runtime log
sink, `pkg/platform/metrics.RuntimeMetricsSink`, and the Factory Runtime
metric projection remain the compatibility path while the new Worker Session
signals are dual-emitted. The named successor is the typed provider and
`factoryruntime`/`worker_sessions` root emission. Deletion is deferred to
`OBS-07` until the last old caller is gone.

#### Behavioral guards and static gates

| Behavior | Guard | Static gate / tier |
| --- | --- | --- |
| Aged sessions are directly diagnosable. | A fake-clock/public-session scenario ages a non-terminal Worker Session beyond the configured timeout and observes `factory.worker_session.age`, its bounded diagnostic reference, matching log/span IDs, and the public stale state. | `make logging-boundary-check`, `make pkg-boundary`, `make pkg-structure`, and `make pkg-file-count`; local Runtime/Worker Session tier. |
| Executor pressure and timeout do not strand or cross-talk lanes. | Concurrent public sessions observe `factory.dispatch.in_flight`, timeout/terminal records, one terminal classification per attempt, and no unrelated-lane termination. | `go test -race` on Runtime/Worker Session packages; PR backend functional tier. |
| The daemon evidence is joinable and exporter failure is isolated. | Process-boundary evidence includes the Worker Session ID without raw payload/secret data; a failed exporter leaves the public result and cleanup unchanged. | `go vet` plus the existing root-process and runtime API functional lanes. |

#### Size, ownership, dependency, and endpoint

Factory Runtime and Worker Sessions own the behavior; platform and Wire own
only the ports and construction already introduced. The packet depends on
BTRC P6 and `OBS-02`, not on a later packet. If it exceeds roughly 800 lines,
split Worker Session lifecycle from dispatch/timeout metrics, with each slice
retaining the dual path and its direct behavioral guard. The endpoint is a
public stale-session/slot/timeout scenario that an operator can join from one
diagnostic reference.

### OBS-04 — Worker/provider attempt and daemon-boundary slice

#### Outcome and preserved behavior

Direct, child, retry, provider, and external-process attempts carry safe
attempt identity and parent/child span relationships through the Workers and
Providers roots. Daemon stderr and provider diagnostics contain the same
allowlisted Worker Session/attempt correlation without prompts, model output,
credentials, or unbounded raw stderr. Existing retry, cancellation, timeout,
worktree cleanup, provider selection, and terminal result behavior remain
unchanged.

#### Build first and caller migration

1. Characterize direct/child attempt result, retry, cancellation, timeout, and
   cleanup behavior through `workers.Service.Execute` and provider contracts.
2. Add child-scope creation at the Workers root and provider execution
   boundary; add the allowlisted process carrier and receiver mapping.
3. Dual-emit attempt start/terminal, provider outcome, duration, and safe
   process-boundary diagnostics while the old logger and metric sink remain.
4. Migrate worker and provider caller batches, then remove no compatibility
   path until all attempt tests observe the replacement.

| Caller family | Current evidence | Migration direction |
| --- | --- | --- |
| `pkg/services/workers` (21 / 14) | Runner selection, command execution, worktree, direct/child, and workstation logs. | Use context-aware spans/logs around one attempt and preserve request-scoped scheduling/retry policy in Workers. |
| `pkg/services/providers` (20 / 4) | Provider execution/adapter logs and normalized execution attempts. | Emit provider child spans and bounded outcome/duration metrics through the Providers root; keep selection and protocol policy in Providers. |
| Worker/provider test fixtures and command-runner edges | Existing success/failure/timeout/cancellation behavior tests. | Capture emitted records at the public contract or edge; do not add source-scanning proof or a custom second provider graph. |

#### Strangler and deletion row

The existing logger and runtime metrics adapter remain dual-write consumers.
The named successor is the Workers/Providers context boundary backed by the
provider from `OBS-02`; old per-call variadic logging and `any` metric writes
are not deleted until `OBS-07` proves zero consumers and checker closure.

#### Behavioral guards and static gates

| Behavior | Guard | Static gate / tier |
| --- | --- | --- |
| Attempts remain terminal exactly once. | Direct, child, retry, provider-failure, timeout, and cancellation tests observe one terminal outcome, distinct attempt IDs, and preserved result mapping. | `make logging-boundary-check`, `make pkg-boundary`, and `go vet`; local Workers/Providers tier. |
| Process and provider joins are safe. | Command-runner/provider tests observe the same allowlisted scope in parent/child signals and prove redaction of payload, credentials, and raw unbounded stderr. | `go test -race` for attempt/cancellation paths; PR backend integration tier. |
| External-effect cleanup is preserved. | Worktree/process cleanup tests run on success, cancellation, pre-start failure, timeout, and exporter failure. | Root-process and Linux functional coverage tiers. |

#### Size, ownership, dependency, and endpoint

Workers owns request-scoped execution and Providers owns provider execution;
platform owns only signal mechanics. The packet depends on BTRC P6 and
`OBS-03`. If it exceeds roughly 800 lines, split provider execution from
external-process/child propagation, preserving the same attempt contract in
both. The endpoint is direct and child execution with joinable, redacted
terminal evidence.

### OBS-05 — Factory Session, Work, recording, and response-boundary slice

#### Outcome and preserved behavior

Factory Session, Work admission/materialization, Recordings, Chat Sessions,
and source-native Events carry one immutable scope across synchronous calls,
goroutines, response streams, and process/provider handoffs. Live response
events remain distinct from canonical Factory Event replay. Work ID, Factory
Session ID, Worker Session ID, stage/transition, attempt, and instrumentation
trace identity are joined with explicit absent state; unique IDs remain out
of metric aggregation dimensions.

#### Build first and caller migration

1. Characterize public session open/invoke/control/result, Work admission and
   lineage, recording/replay, response event ordering, and ACP delegation.
2. Attach each identity only at its owning public root and pass derived
   contexts through asynchronous handoffs; preserve cancellation and bounded
   detached cleanup contexts.
3. Dual-emit logs/spans and bounded diagnostic metric references at session,
   Work, recording, and response boundaries.
4. Migrate the service batches and their public tests; leave old sinks usable
   for any family not yet moved.

| Caller family | Current evidence | Migration direction |
| --- | --- | --- |
| `pkg/services/factory_sessions` (3 / 3) | Hosted runtime/session construction and lifecycle logs. | Enrich at Factory Session root; keep customer session identity separate from runtime artifact identity. |
| `pkg/services/recordings` (11 / 7) | Recording, replay, worker capture, and artifact construction logs. | Correlate canonical history and artifacts without making Recordings own live runtime state or exporter policy. |
| `pkg/services/chat_sessions` (9 / 4) and `events` (2 / 2) | ACP bridge, response stream, source-native event storage, and subscription logs. | Carry copied scope through turns, controls, subscriptions, and backpressure paths; keep event retention separate from Factory replay. |
| `pkg/services/work` and transport boundaries | Work owns identity/lineage but is not a current direct logging-import family. | Attach Work ID at Work admission/materialization and let service roots consume the narrow port; do not add a speculative Work logging dependency. |

#### Strangler and deletion row

No session, Work, recording, or event surface is deleted here. Existing log
and metric compatibility paths remain until all service families move. The
named successor is the public Sessions/Work/Recordings/Chat Sessions boundary
using `context.Context`, with canonical history still owned by Recordings and
live response state still owned by Sessions/Events.

#### Behavioral guards and static gates

| Behavior | Guard | Static gate / tier |
| --- | --- | --- |
| Concurrent sessions and turns remain isolated. | Public root tests run two sessions/turns and assert distinct scopes, ordered response events, and unchanged terminal envelopes. | `make logging-boundary-check`, `make pkg-boundary`, `make api-smoke`; local Sessions/Events tier. |
| Canonical replay and live response joining remain distinct. | Recording/replay tests observe canonical event order and response-stream cursors while emitted signals retain the same safe session/Work scope. | `go test -race`; PR backend integration and API contract tiers. |
| ACP/child cancellation does not leak work or suppress terminal evidence. | ACP and session functional cells observe cancellation/close terminalization, one attempt terminal, and a bounded diagnostic record even when cleanup is detached. | Linux functional and pinned ACP tiers. |

#### Size, ownership, dependency, and endpoint

Factory Sessions, Work, Recordings, Chat Sessions, and Events own their public
boundaries; no peer internal type crosses the platform port. The packet
depends on BTRC P6, `OBS-03`, and `OBS-04`. If it exceeds roughly 800 lines,
split Factory Session/Work from Recordings/Chat Sessions/Events, keeping each
public stream and replay behavior independently releasable. The endpoint is a
public concurrent-session and replay/response scenario with cross-signal
correlation.

### OBS-06 — Remaining service callers and enforcement ratchet

#### Outcome and preserved behavior

All remaining service and transport callers use the typed, context-aware
replacement in bounded batches. The measured service denominator of 135 all /
58 production files is exhausted without a big-bang rewrite, while the
remaining platform compatibility implementation is intentionally retained
for `OBS-07`. `cmd/loggingboundarycheck` is extended as the existing static
gate: it rejects new legacy logging imports/construction outside a small,
named compatibility allowlist and checks the migrated boundary shape. No new
filesystem-scanning checker is introduced.

#### Build first and caller migration

1. Add checker fixtures for prohibited new legacy imports/calls and missing
   context-aware boundaries; keep the checker itself outside the runtime
   caller denominator.
2. Migrate one service-family batch at a time, starting with families that
   have no production callers, then the larger settings/definitions/transport
   families. Each batch keeps a dual adapter until its behavioral tests pass.
3. Migrate transport and test-support callers after their owning service
   contract is canonical; remove only stale fixture imports, not behavioral
   coverage.
4. Shrink the compatibility allowlist to only the named platform/Wire
   adapters needed by `OBS-07`; no new caller may land on the old path.

| Caller family | All / production files | Migration direction |
| --- | ---: | --- |
| `factory_definitions` | 7 / 5 | Context-aware catalog/package/validation operation records; preserve package safety and invocation policy. |
| `operator_settings` | 14 / 4 | Context-aware settings resolution and construction diagnostics; keep settings policy in its owner. |
| `webhooks` | 3 / 2 | Context-aware delivery/retry outcome records without moving hosted-source policy into the platform. |
| `models` | 2 / 0 | Update test-only transport/bootstrap fixtures; no production model caller is implied by the count. |
| `pkg/transports` | 11 / 4 | Map protocol context at the public boundary and keep representation mapping free of signal policy. |
| `internal/testutil` and `cmd/pkgboundarycheck` | 5 / 2 | Use capture providers or explicit test adapters; do not make tests preserve a deleted runtime path. |

#### Strangler and deletion row

`OBS-06` deletes no platform compatibility surface. It removes migrated
service/transport imports and reduces the checker allowlist. The named
successor for every moved caller is its service root plus the platform ports
from `OBS-01`/`OBS-02`; the old `Logger`, runtime log sink, and metrics sink
remain until `OBS-07` has a fresh zero-consumer audit.

#### Behavioral guards and static gates

| Behavior | Guard | Static gate / tier |
| --- | --- | --- |
| Moved service operations emit safe, correlated outcomes. | Service/root tests capture known and absent scopes, redaction, failure classification, and cancellation without inspecting source topology. | Extended `cmd/loggingboundarycheck`, `make pkg-boundary`, `make pkg-structure`, `make pkg-file-count`, and `make typecheck`; local service tier. |
| Protocol and package boundaries remain stable. | API/CLI/transport contract cells observe unchanged envelopes, errors, streams, and terminal behavior. | `make api-smoke`, `go vet`, and PR `make verify-fast`. |
| Migration is complete by behavior, not by a meta-test. | Representative operations from every family emit through the capture provider; source counts are audit evidence only. | Backend unit/integration and relevant functional tiers; no source, route, registration, docs-link, or asset-inventory product test. |

#### Size, ownership, dependency, and endpoint

Service owners, transport owners, and `cmd/loggingboundarycheck` own their
parts; the platform does not absorb their policy. The packet depends on BTRC
P6 and `OBS-05`. If it approaches roughly 800 lines, split by the listed
caller families (services first, transports/test support second), with each
slice keeping the old adapter and checker allowlist explicit. The endpoint is
zero service/transport imports of the legacy caller API, with only named
platform/Wire compatibility rows remaining for `OBS-07`.

### OBS-07 — Legacy logger/metrics deletion and canonical-plane closure

#### Outcome and preserved behavior

After `OBS-06`, delete the old caller-facing logger and untyped metrics
compatibility surfaces in separately reviewable logging and metrics slices.
The result is one provider lifecycle, one typed signal boundary, and the
existing policy-free rolling-file/runtime-artifact effects only where a
canonical local diagnostic implementation still needs them. No deletion is
claimed for a surface that still has a caller, and retained compatibility is
recorded as retained rather than silently counted as removed.

#### Build first and caller migration

1. Re-run the exact import/symbol audit on the post-`OBS-06` head and list
   every remaining legacy caller, including tests and compatibility adapters.
2. Require the checker to pass with no unowned allowlist entry and require
   each deletion row below to name its landed replacement and observable
   zero-consumer condition.
3. Delete the legacy logging API/constructor and its Wire adapters only after
   logging callers are zero; separately delete the untyped file-metrics API
   only after metric callers are zero. Keep each slice releasable.
4. Re-run stale-session, attempt, session/replay, exporter-failure, lifecycle,
   race, and local-artifact guards before closing the packet.

| Caller/old surface | Previously landed replacement | Migration/deletion direction |
| --- | --- | --- |
| `pkg/platform/logging.Logger` variadic operations, `NoopLogger`, and legacy `BuildLogger` acquisition | `OBS-01` typed context logger and `OBS-02` provider/local adapter, exercised by `OBS-03`–`OBS-06` | Delete only after no production or test caller uses the old operations and the logging checker has no compatibility allowlist entry for them. |
| `pkg/platform/metrics.RuntimeMetricsOpener`, `RuntimeMetricsSink`, and `WriteMetric(ctx, any)` | `OBS-01` typed meter/diagnostic point and `OBS-02` provider lifecycle, exercised by `OBS-03`–`OBS-06` | Delete only after exact direct imports and symbol references are zero and typed point/diagnostic atomicity is green. |
| `pkg/wire` `runtimeLogSinkAdapter` and `runtimeMetricRecordWriterAdapter` | `OBS-02` provider-owned ports and lifecycle | Delete after Wire/root construction and close tests prove no adapter reference remains. |
| Factory Runtime `RuntimeMetricRecord` compatibility projection | `OBS-01` typed metric point plus provider diagnostic reference | Delete or make private only after its last mapper/test caller moves; preserve all bounded runtime metric meanings in the typed descriptors. |
| Metrics-specific legacy JSONL opener/path policy | `OBS-02` local diagnostic adapter using policy-free `rollingfile`/`runtimeartifact` effects | Delete only after local diagnostic output, rotation, close, and failure isolation are observed through the successor. |

`pkg/platform/rollingfile.Writer` and `pkg/platform/runtimeartifact.Reserver`
are explicitly retained policy-free effects when transcript or local
diagnostic owners still use them. They are not reported as deleted by this
packet. The logging-boundary checker itself is retained; only obsolete
legacy allowlist entries and fixtures are removed.

#### Strangler and deletion register

The following ledger is exhaustive for scheduled removals in this
specification. A row may be closed only after its replacement has already
landed on `main`; a planned replacement, a matching source count, or a green
compile without the observable guard is insufficient. The exact import and
symbol audits are deletion evidence, not product behavior tests.

| ID / exact old surface | Previously landed replacement | Owning deletion packet | Zero-consumer and checker condition | Preserved observable behavior |
| --- | --- | --- | --- | --- |
| `OBS-D01`: `pkg/platform/logging.Logger` and its `Debug`, `Info`, `Warn`, `Error`, and `Verbose` variadic methods; `NoopLogger`, `EnsureLogger`, `NewZapLogger`, `NewDefaultLogger`, `BuildLogger`, `BuildTerminalMutedLogger`, `BuildTerminalLogger`, `RuntimeLogConfig`, `RuntimeLogSink`, `RuntimeLogOpener`, `RuntimeLogOpeningRequest`, `RuntimeLogArtifact`, `RuntimeLogsRoot`, `legacyRuntimeLogDir`, and the private `runtimeLogWriter` acquisition/output path. | `OBS-01` typed context logger plus the `OBS-02` provider/local adapter; `OBS-03`–`OBS-06` migrate and exercise callers. The successor may retain only the policy-free `rollingfile.Writer` and `runtimeartifact.Reserver` effects. | `OBS-07L` | `rg` finds no direct legacy import or symbol reference in production or test Go files outside the replacement adapter; `cmd/loggingboundarycheck` passes with no `OBS-D01` allowlist entry; logger-construction, local-artifact, and redaction guards are green. | Severity, safe fields, terminal/runtime destinations, rotation/permissions, cancellation/error classification, and Work/Factory Session/Worker Session correlation remain observable. |
| `OBS-D02`: `pkg/platform/metrics.RuntimeMetricsConfig`, `RuntimeMetricsOpeningRequest`, `RuntimeMetricsOpener`, `RuntimeMetricsSink`, `WriteMetric(context.Context, any)`, and their untyped JSONL record path. | `OBS-01` typed metric descriptors/diagnostic references plus the `OBS-02` provider-owned lifecycle and local adapter; `OBS-03`–`OBS-06` migrate callers. | `OBS-07M` | Exact direct imports and exported/internal symbol references are zero outside the replacement adapter; typed aggregation, bounded diagnostic-reference, atomic write, failure, and close tests pass. | Counter/gauge/histogram meaning, units, bounded retention, local diagnostics, exporter isolation, and close behavior remain observable. |
| `OBS-D03`: `pkg/wire` construction adapters `runtimeLogSinkAdapter`, `runtimeMetricRecordWriterAdapter`, and any legacy runtime-log/metrics owner closures that adapt the old platform surfaces. | `OBS-02` provider-owned ports and lifecycle constructed through the post-P6 public roots. | `OBS-07L` / `OBS-07M` | `rg` finds no adapter type, constructor, or root wiring reference; root activation, failure unwind, close-once, and process behavior tests pass. | Construction, activation/unwind, cleanup, runtime artifact ownership, and primary-error preservation remain observable. |
| `OBS-D04`: Factory Runtime compatibility vocabulary `RuntimeMetricRecord`, `RuntimeMetricRecordWriter`, the legacy `Fields.TraceID` projection, and mapper/test fixtures whose only purpose is to feed `WriteMetric`. | `OBS-01` typed metric point with bounded diagnostic reference and the provider contract from `OBS-02`; the owning Runtime root retains product measurements, not the platform serializer shape. | `OBS-07M` | The last mapper and test fixture caller is migrated; no legacy projection type or writer reference remains; a typed descriptor preserves every retained measurement and its bounded join reference. | Queue, dispatch, provider, lifecycle, state, in-flight, Work, and session observations remain joinable without turning unique IDs into metric aggregation labels. |
| `OBS-D05`: metrics-specific JSONL opener/path policy and private serializer (`RuntimeMetricsRoot`, `runtimeMetricsWriter`, and metrics-owned rolling-file setup) that exists only to support the old untyped sink. | `OBS-02` local diagnostic adapter using the retained policy-free `rollingfile.Writer` and `runtimeartifact.Reserver` effects. | `OBS-07M` | Successor tests observe local output, rotation, permissions, close, post-close behavior, and exporter-failure isolation; the old opener/path symbols have no callers before removal. | Local diagnostic output, bounded rotation/retention, artifact ownership, and failure isolation remain observable. |
| `OBS-D06`: temporary `cmd/loggingboundarycheck` compatibility allowlist entries and fixtures that permit the old logger/metrics imports, constructors, or calls during migration. | The retained checker rules from `OBS-06`–`OBS-07`, with the typed boundary and dependency-direction rules enabled by default. | `OBS-07L` / `OBS-07M` | Every allowlist entry has a named owner and deletion packet while migration is active; after the corresponding rows close, the entry and fixture are removed and the checker reports zero legacy allowances. | New legacy callers remain rejected, migrated boundaries remain context-aware, and the enforcement signal itself remains available for future regressions. |

No other surface is scheduled for deletion by `OBS-07`. In particular,
`pkg/platform/rollingfile.Writer`, `pkg/platform/runtimeartifact.Reserver`,
and the retained `cmd/loggingboundarycheck` command are policy-free effects or
enforcement infrastructure and remain available to their named successors.
They are explicitly retained, not silently counted as deleted. A future plan
must add a new ledger row before scheduling their removal.

#### Behavioral guards and static gates

| Behavior | Guard | Static gate / tier |
| --- | --- | --- |
| Incident diagnosis remains direct after deletion. | Public stale Worker Session, executor-slot, timeout, daemon-log, and terminal scenario observes one joined scope across log, metric diagnostic record, span, and public state. | `make logging-boundary-check`, `make pkg-boundary`, `make pkg-structure`, `make pkg-file-count`, `go vet`; local Runtime/Workers tier. |
| Provider failure and lifecycle closure remain isolated. | Exporter failure, queue overflow, flush timeout, startup failure, cancellation, and close tests preserve product outcomes and cleanup. | `go test -race`, `make verify-fast`, and PR backend integration tier. |
| Canonical local artifacts remain usable. | Runtime log/diagnostic path, rotation, permissions, retention, and post-close behavior are observed through the successor. | Focused platform/artifact tests and relevant functional runtime API tier. |
| No old path survives invisibly. | Zero-consumer audit and checker output are reviewed alongside behavioral tests; no product acceptance relies on a source or route inventory meta-test. | Final PR static gates plus backend/functional evidence. |

#### Size, ownership, dependency, and endpoint

Platform logging/metrics, Wire, and the checker own this packet. It depends on
BTRC P6 and the completed `OBS-01`–`OBS-06` sequence. If it exceeds roughly
800 lines, split `OBS-07L` and `OBS-07M`; each must delete only its own zero-
consumer surface and leave the other compatibility path usable. The
independently mergeable endpoint is one canonical, typed provider plane with
all old caller-facing surfaces either deleted under a named condition or
explicitly retained as policy-free infrastructure.

### Story 003 packet closure

Story 003 is complete when `OBS-01` through `OBS-07` are an ordered,
post-BTRC-P6 implementation sequence with observable endpoints, build-first
replacement steps, bounded caller migrations, named deletion successors,
static gates, behavioral tiers, dependency/ownership/sizing, and independent
merge points. The sequence prioritizes stale Worker Session age,
executor-slot pressure, timeout/terminal evidence, and cross-signal
correlation before broad caller migration. It does not add production code or
dependencies in this authoring lane, and it contains no resumption design or
reserved resumption packet.

## Story 004 closure: enforcement, verification, and delivery

Story 004 closes the specification, not the future implementation. It makes
the deletion preconditions, static ratchet, behavior evidence, and handoff
rules reviewable before any production packet is started.

### Phased enforcement ratchet

`cmd/loggingboundarycheck` remains the preferred observability-specific
enforcement point. Its existing Go AST/import scan is extended in place; no
new filesystem-scanning checker, source-inventory test, route/command
inventory, or second enforcement command is introduced. The checker may
report a violation of the boundary shape, but behavioral tests remain the
proof that signals contain the right values.

| Phase | Owner | Gate and allowed compatibility | Failure condition and exit evidence |
| --- | --- | --- | --- |
| `E0` — characterize | `OBS-01`/`OBS-02` owners with `cmd/loggingboundarycheck` | Run the existing checker tests and record the exact legacy imports, constructors, and allowlist entries that OBS-01–OBS-02 must coexist with. Existing owner files are the only temporary exceptions. | The baseline does not measure, or a new legacy acquisition path is accepted without a named owner. The packet stops and adds a characterization case before migration. |
| `E1` — introduce the replacement | Platform signal owner and checker maintainer | Extend `cmd/loggingboundarycheck` to reject new direct legacy logger/metrics imports, constructors, dot-imports, and global logger acquisition outside the named owner/adapter allowlist. Require every new typed signal method and exported caller boundary to accept and forward `context.Context`; reject an OTel SDK import outside the platform owner once that dependency exists. | A changed caller adds a legacy path, drops `context.Context`, or imports an exporter/SDK outside the platform boundary. The allowlist entry must name its owner, expiry packet, and deletion row; an unowned exception fails the packet. |
| `E2` — migrate in batches | Owning service/root caller teams with checker maintainer | Keep the checker green after each caller-family batch. The checker verifies the static correlation shape: the typed emit call receives the operation context and no package stores correlation in a process-global mutable variable. Runtime tests verify the values and absent/unknown semantics that static analysis cannot prove. | A new caller uses the old API, a migrated boundary cannot be traced to its context, or a service reaches an internal provider/exporter type. The batch is reverted or split; compatibility is not widened to hide the violation. |
| `E3` — close the caller surface | `OBS-06`/`OBS-07` owners, checker maintainer, and package-gate owners | Remove family-specific allowlist entries as their deletion rows close. Run `cmd/loggingboundarycheck`, `make pkg-boundary`, and `make pkg-structure`; the existing package gates enforce general dependency direction while the logging checker owns the signal-boundary rules. | Any service imports a peer internal package, any platform package takes product policy, any legacy import remains outside an explicitly retained effect, or the checker cannot report property-specific output. `OBS-07` remains open. |
| `E4` — prevent regression | `cmd/loggingboundarycheck` maintainer and owning platform team | Retain the checker and its focused fixtures after OBS-07, but with zero legacy compatibility allowances. Add only behavior-shaped fixtures for a new violation (legacy acquisition, missing context, forbidden SDK/import direction); do not turn the checker into a repository inventory test. | A future change reintroduces a retired import/call, hidden global correlation, or forbidden dependency direction. The owning PR fails before review; no deletion row is reopened unless the behavior replacement itself regresses. |

The checker is deliberately narrow: it protects the path and dependency
direction, not metric cardinality, identifier correctness, exporter health, or
the presence of a particular registration. Those properties belong to the
behavioral matrix below. A passing `rg` count or checker result never closes a
deletion row by itself.

### Behavioral verification matrix

Verification is layered around observable outputs and failure behavior. The
future packet author should use injected clocks, capture providers, controlled
exporter failures, and deterministic cancellation; sleeps and timeout padding
are not synchronization. The named guards below are behavior tests, not
source, route, registration, documentation-link, command, or asset-inventory
meta-tests.

| Behavior to prove | Required direct evidence | Execution tier and failure meaning |
| --- | --- | --- |
| Typed logs, metric points, and spans carry safe fields and preserve absent/unknown semantics. | Focused `pkg/platform` tests assert field typing, redaction, severity/status/error mapping, units, bounded attributes, and no unique Work/Factory Session/Worker Session ID in metric aggregation dimensions. | Local platform unit tests; exporter/provider contract tests in the PR backend tier. A dropped or unsafe field is a contract failure, not a reason to weaken the assertion. |
| Exporter and local-diagnostic failure is isolated from product work. | Platform/provider tests inject startup, enqueue, flush, write, close, and timeout failures; the caller still gets the documented product outcome, cleanup runs once, and the failure is observable in the bounded diagnostic path. | Focused platform/provider tests plus `go test -race`. A provider failure may degrade telemetry, never the Work/Factory Session terminal result or lifecycle unwind. |
| Context and correlation survive service/root and asynchronous handoffs. | Service and root tests start one scope, derive a child scope for stage/transition and attempt, hand it through goroutine/queue callbacks, preserve cancellation, and assert Work ID, Factory Session ID, Worker Session ID, stage/transition, and attempt on every joinable log/span. Missing values are explicit absent/unknown values. | Focused service/root tests and race execution. A value silently read from global state or lost at a handoff fails the packet. |
| Stale Worker Session diagnosis is direct. | A controlled-clock functional scenario creates an aged RUNNING Worker Session, associates its Work, executor slot, timeout, daemon boundary, and terminal attempt, then observes the same scope in public state plus log, metric diagnostic reference, and span output. It asserts the leaked identifier is present in daemon evidence. | Runtime/Workers functional tier and `go test -race`; the supplied four-instrument incident is the regression scenario, not a retention or deletion rule. |
| Terminal signals are exactly once under duplicate completion, cancellation, timeout, and exporter failure. | Runtime/Worker Session integration tests race duplicate callbacks and cancellation, then assert one terminal state and one terminal signal per attempt while non-terminal updates remain ordered. | Focused integration and race tier. Duplicate or missing terminal evidence is a product regression even if the exporter is unavailable. |
| Cross-signal joins are bounded and useful. | A public operation captures one Work/Factory Session/Worker Session/stage/attempt scope, reads log and span attributes, and follows only a bounded metric diagnostic reference/exemplar to the corresponding metric record; no high-cardinality ID is an aggregation label. | Functional cross-signal test with a capture exporter and local artifact; PR functional tier. The test asserts emitted values and join results, never implementation registration or source topology. |
| Existing local artifacts and public outcomes survive the strangler. | Runtime log/diagnostic tests observe path ownership, rotation, permissions, close/post-close behavior, CLI/API terminal shape, and replay/session isolation through the successor after each legacy slice is removed. | Focused platform/runtime and API/CLI functional tiers. A retained policy-free effect is tested as an effect; it is not mislabeled as a deleted compatibility surface. |

The deletion audits and checker output are necessary static preconditions for
OBS-D01–OBS-D06, but they are not product proof. In particular, do not add a
test that scans source files, counts routes or commands, enumerates
registrations/packages, checks documentation links, or inspects asset bundles
to claim observability behavior. If a static gate cannot emit the property it
claims to measure, it cannot be cited as passing evidence.

### Final authoring reconciliation

At the final implementation head, the author records the result of
`git diff --name-only origin/main...HEAD` and `git diff --check`. The expected
tracked diff for this lane is exactly:

```text
docs/internal/packaged-service-structure/observability-plane-specification.md
```

`prd.json` and `progress.txt` are local work-item scaffolding and remain
untracked/ignored; they are never staged or included in the PR. No index or
catalog update is required: neighboring packet specifications are standalone,
and no repository convention demonstrably requires adding this document to an
index. The authoring diff must contain no `.go` file, `go.mod`, `go.sum`,
`api/`, `ui/`, `Makefile`, baseline, generated artifact, existing document,
production instrumentation, exporter/dependency wiring, BTRC composition
surface, or resumption-plane design/change. A generated or baseline change is
not an acceptable way to make a documentation gate green.

The final content review checks architecture fit against the public platform
and service roots, confirms every package/interface/dependency packet remains
after BTRC P6, confirms all OBS-D rows name a landed replacement and
observable condition, and confirms retained rolling-file/runtime-artifact
effects are not reported as deleted. It also checks that the incident dates
remain historical context rather than deletion or retention conditions.

### Delivery Loop

The implementation and review stages have separate finish lines:

1. The implementation stage marks the delivery criterion satisfied only after
   the final documentation head is committed and pushed, the PR is open, CI
   has started on that exact head, and all blocking review conversation on the
   current head is addressed.
2. CI-run URLs, failing-test explanations, and baseline-flake notes belong in
   a PR comment. They must never be committed as an audit, verification, or
   CI-results file because that would create a new head and invalidate the run.
3. After that finish line, the implementation stage stops. It does not poll or
   re-check CI to await a terminal result.
4. The review stage owns driving required CI to terminal-and-passing,
   resolving merge conflicts, and merging the PR. Merge is the lane-wide
   completion boundary; an acceptance criterion mentioning “merged” does not
   extend the implementation stage.

This delivery loop does not introduce a resumption packet, resumption state,
resumption API, or resumption migration seam.

### Story 004 acceptance closure

Story 004 is complete when the OBS-D01–OBS-D06 ledger is exhaustive and
distinguishes retained effects, the E0–E4 ratchet has a named owner and
property-specific gate at each phase, the behavioral matrix proves signal
failure isolation/correlation/stale-session diagnosis/exactly-once terminal
behavior at the appropriate layers, and the final reconciliation plus
Delivery Loop makes the documentation-only handoff explicit. No production
code, dependency, generated contract, baseline, composition surface, or
resumption design is part of this authoring story.
