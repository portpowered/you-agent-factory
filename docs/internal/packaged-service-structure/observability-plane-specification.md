# Observability Plane Packet Specification

Status: Story 002 target contracts authored; current-tree inventory and target
contract baseline are complete. Later stories add the post-BTRC-P6 packets to
this same specification.

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
