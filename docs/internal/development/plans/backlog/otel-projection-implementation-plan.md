---
author: Agent Factory Team
last-modified: 2026-08-10
doc-id: agent-factory/plans/otel-projection
---

# OpenTelemetry Projection Implementation Plan

## Outcome

Customers can enable an OpenTelemetry Protocol (OTLP) destination and receive
Factory operational metrics and execution traces that align with the
`you metrics` vocabulary. The projection uses Analytics as its only product
data dependency. Analytics obtains monetary valuation from Costs and combines
it with existing Factory Session, Worker Session, Work, and recording facts.

OpenTelemetry is an export projection, not a new source of truth. Export loss,
cardinality limits, sampling, or an unavailable collector cannot change
Factory execution, canonical history, cost accounting, or `you metrics`
results.

## Customer problem

Customers can inspect You-owned runtime metrics artifacts and will eventually
be able to query `you metrics`, but they cannot send the same Factory evidence
to an existing OpenTelemetry backend. Without a supported projection they must
maintain custom adapters, cannot correlate slow metric samples to Work and
dispatch traces, and may define provider or token dimensions inconsistently.

## Customer ask

Provide an OTel projection shaped above the analytical service graph:

```text
OTel Projection
  -> Analytics
       -> Costs
            -> existing usage and session facts
       -> Factory Sessions
       -> Worker Sessions
       -> Work
       -> Recordings
```

These services are read-oriented utility layers above the existing domain
owners. They do not move lifecycle, execution, Work lineage, provider usage, or
Factory event ownership out of the current services.

## Entry prerequisites

This plan is specifically for the OTel projection. Its implementation starts
only after these separately owned prerequisites are available:

1. `pkg/services/costs` exposes exact, currency-aware valuation and coverage
   results without treating missing prices or usage as zero.
2. `pkg/services/analytics` is constructed with a narrow Costs capability and
   narrow read capabilities from the existing domain services.
3. Analytics can produce deterministic operational observations from canonical
   session and Work history.

If those prerequisites have not landed, they must be delivered through their
own plans and reviewed service contracts first. This plan may add the narrow
Analytics export capability needed by OTel, but it must not absorb the general
Costs or Analytics implementation.

## Architecture decision

### Dependency direction

```text
pkg/services/otel_projection
  -> pkg/services/analytics (TelemetryReader capability only)
  -> OTLPSender external-effect port
  -> ProjectionCheckpointStore external-effect port
  -> injected clock and logger

pkg/services/analytics
  -> pkg/services/costs
  -> narrow existing read capabilities

pkg/services/costs
  -> narrow existing usage, model, provider, and session facts
```

The following dependencies are prohibited:

- OTel Projection importing Costs directly.
- OTel Projection importing Factory Sessions, Worker Sessions, Work,
  Recordings, Providers, or their internal implementations.
- Analytics or Costs importing OTel types.
- Existing domain services importing Analytics, Costs, or OTel Projection.
- Construction outside the owning `wire/` provider or canonical `pkg/wire`
  graph.
- A global OTel provider, service locator, secondary injector, or lazy service
  construction path.

A package-boundary regression check should enforce the most important parts of
this direction rather than relying only on review memory.

### Ownership

| Owner | Responsibility |
|---|---|
| Factory Sessions | Factory Session lifecycle, dispatch association, live and durable reads. |
| Worker Sessions | Provider execution observations, normalized usage, transcripts, and execution identity. |
| Work | Work identity, admission, state, trace/chaining lineage, and relations. |
| Recordings | Canonical Factory Event ledger, replay, and historical projections. |
| Costs | Rate resolution, exact monetary values, currencies, coverage, and valuation provenance. |
| Analytics | Cohorts, durations, distributions, critical-path facts, rework/revisit classification, and cost-enriched analytical observations. |
| OTel Projection | OTel naming, signal mapping, cardinality policy, cursors, export lifecycle, and projection diagnostics. |
| Platform OTLP adapter | Policy-free OTLP serialization, connection, and one-attempt network transport. |

OTel Projection owns only its destination checkpoint and export status. It
does not own the analytical observations it reads.

## Canonical state and projection boundaries

The projection reads an Analytics-owned, protocol-neutral batch contract. The
contract must preserve observation timestamps and relationships without
exposing OTel SDK or protobuf types:

```go
type TelemetryReader interface {
    ReadTelemetry(context.Context, TelemetryReadRequest) (TelemetryBatch, error)
}
```

The exact public names can change during contract review, but the capability
must provide:

- an opaque input cursor;
- a bounded batch size;
- a stable observation watermark;
- metric observations with kind, unit, time window, value or distribution,
  low-cardinality dimensions, and optional exemplar references;
- trace observations with operation identity, timestamps, parent or link
  relationships, outcome, and safe attributes;
- cost observations already enriched by Analytics through Costs;
- explicit coverage and unavailable classifications; and
- a next cursor that is advanced only after successful export.

Analytics must not pre-render OTLP. OTel Projection owns the mapping from these
neutral facts into OTel resources, scopes, metrics, spans, links, and
exemplars.

## Initial signal contract

### Metrics

The first release exports observed base measurements. Percentages, rates, and
percentiles are derived by the receiving backend or Analytics rather than
emitted as independently aggregatable measurements.

| Metric | OTel kind | Unit | Required dimensions |
|---|---|---|---|
| `you.factory.session.duration` | Histogram | `s` | factory, revision, outcome, orchestrator kind |
| `you.work.cycle.duration` | Histogram | `s` | factory, revision, Work type, outcome |
| `you.dispatch.queue.duration` | Histogram | `s` | factory, workstation, worker, outcome |
| `you.dispatch.execution.duration` | Histogram | `s` | factory, workstation, worker, outcome |
| `you.dispatch.attempts` | Counter | `{attempt}` | factory, workstation, worker, outcome |
| `you.dispatch.retries` | Counter | `{retry}` | factory, workstation, worker, reason |
| `you.work.revisits` | Counter | `{revisit}` | factory, Work type, stage |
| `you.work.completed` | Counter | `{work}` | factory, revision, Work type, outcome |
| `you.dispatch.in_flight` | UpDownCounter | `{dispatch}` | factory, workstation |
| `you.analytics.coverage` | Gauge | `1` | fact kind and availability status |
| `you.cost.amount` | Histogram | `1` | currency, valuation kind, billing provider |

`you.cost.amount` is a telemetry approximation represented as an OTel numeric
value. Costs remains authoritative for exact decimals, applied pricebooks, and
historical currency facts. Unknown or unpriced usage is excluded from the
amount and represented through coverage/status measurements; it is never
exported as zero cost.

Provider operations use the pinned OTel GenAI semantic conventions where they
fit, including:

- `gen_ai.client.operation.duration`;
- `gen_ai.client.token.usage`;
- `gen_ai.provider.name`;
- request and response model attributes supported by the selected convention
  version; and
- input/output token types defined by that version.

Cache-read, cache-write, and reasoning-token details use the pinned GenAI
conventions when available. Any necessary `you.*` extension must be documented
and versioned instead of silently redefining a standard attribute.

### Metric cardinality policy

Allowed metric dimensions are controlled catalog values such as:

- Factory name and immutable effective revision;
- orchestrator kind;
- Work type and terminal outcome;
- workstation and authored Worker name;
- provider and resolved model;
- dispatch outcome or bounded reason class;
- currency and valuation kind; and
- coverage fact/status.

The following values must not be metric attributes:

- Factory Session ID;
- Worker Session ID;
- Work ID;
- Dispatch ID;
- trace ID or chaining trace ID;
- prompt, response, tool arguments, artifact paths, or payload content; and
- raw error messages.

Those identifiers belong on spans, links, exemplars, or safe diagnostic logs.
The OTel adapter must configure an explicit cardinality limit and expose
overflow/drop diagnostics.

### Traces

The initial trace projection provides:

- one bounded Work-lineage trace for customer Work that crosses dispatches or
  Factory Sessions;
- dispatch spans for queued-to-terminal execution attempts;
- Worker Session spans when a distinct supervised execution context is known;
- GenAI client spans for provider operations;
- span links for fan-in, predecessor Work, and cross-session continuation;
- safe outcome, Factory revision, provider, model, Work type, workstation, and
  Worker attributes; and
- exemplars that connect latency or token histograms to representative spans.

Long-running Factory Sessions are not represented by indefinitely open root
spans. A completed bounded session may receive a terminal summary span, while
continuously running sessions are correlated through safe session attributes,
Work traces, and lifecycle observations.

OTel logs and prompt/response content export are out of scope for the initial
release. Canonical Factory Events remain available through Recordings and the
session event APIs.

## Configuration contract

OTLP export is disabled by default. Add a distinct `runtime.otel` operator
configuration rather than overloading the existing rolling `runtime.metrics`
artifact settings:

```json
{
  "runtime": {
    "otel": {
      "enabled": true,
      "endpoint": "https://collector.example.com",
      "protocol": "grpc",
      "signals": ["metrics", "traces"],
      "exportInterval": "30s",
      "timeout": "10s",
      "startFrom": "latest",
      "backfillWindow": "0s"
    }
  }
}
```

Required behavior:

- accept reviewed OTLP/gRPC and OTLP/HTTP protobuf values;
- support the applicable standard `OTEL_EXPORTER_OTLP_*` environment settings
  with documented precedence against operator configuration;
- obtain authorization headers from the approved secret-safe environment path;
- never print header values or credentials in status, logs, errors, or JSON;
- validate endpoint, protocol, durations, signals, and backfill policy before
  starting the exporter;
- begin from the current Analytics watermark by default;
- require explicit configuration for retained-history backfill; and
- preserve the existing file-backed runtime metrics artifact independently.

The authored OpenAPI fragments, operator settings contracts, generated Go and
TypeScript clients, and global configuration schema must remain synchronized.
Generated files must not be hand-edited.

## Export lifecycle and failure behavior

`pkg/wire` constructs Costs, then Analytics, then OTel Projection, passing the
same service instances directly. `pkg/initializer` starts and stops the already
constructed projection lifecycle.

The projection loop must:

1. request a bounded Analytics batch after the committed cursor;
2. map the batch deterministically to OTel signals;
3. export with an explicit deadline;
4. commit the next cursor only after the destination accepts the batch;
5. retry transient failures with bounded exponential backoff and jitter;
6. classify permanent rejection without retrying indefinitely;
7. expose lag, rejection, retry, overflow, and last-success diagnostics; and
8. flush within a bounded shutdown deadline.

Exporter failure must never block or fail Factory Session, Worker Session,
Work, recording, Costs, or Analytics operations. Because the projection pulls
from retained analytical facts, it should prefer lag over an unbounded
in-memory queue. If the Analytics retention window overtakes the committed
cursor, the projection records an explicit gap and resumes according to the
configured gap policy rather than silently claiming complete export.

Checkpoint identity includes a sanitized destination fingerprint, signal set,
projection schema version, and Analytics stream identity. Changing those
values creates a new projection lineage instead of applying an incompatible
cursor.

## Status experience

Add a read-only diagnostic surface under the unified metrics namespace:

```bash
you metrics otel status
you --json metrics otel status
```

The status reports:

- enabled/disabled and lifecycle state;
- sanitized endpoint authority and protocol;
- enabled signals;
- projection and pinned semantic-convention versions;
- current Analytics watermark and committed cursor age;
- last attempt and last successful export times;
- exported metric-point and span counts;
- retry, rejection, cardinality-overflow, and gap counts;
- last sanitized failure classification; and
- whether shutdown has a pending flush.

If CLI commands are server-backed, add the corresponding read-only OpenAPI
operation and generated clients. Status reads must not trigger an export,
retry, backfill, or configuration mutation.

## Non-goals

- Replacing canonical Factory Events or recordings with OTel.
- Querying an arbitrary OTel backend to implement `you metrics`.
- Making Costs depend on Analytics or OTel Projection.
- Exporting cost forecasts as observed metrics.
- Exporting prompts, responses, tool arguments, secrets, or artifacts.
- Supporting OTel logs in the first release.
- Automatically classifying every graph loop as rework.
- Building dashboards for a specific OTel vendor.
- Removing the existing rolling runtime metrics JSONL artifact.
- Claiming exactly-once delivery across collector or network failures.

## Work stories

### Story 1: Publish a cursorable Analytics telemetry feed

#### Problem statement

OTel Projection cannot consume Analytics without a bounded, deterministic, and
protocol-neutral observation contract.

#### Customer ask

Allow OTel Projection to obtain cost-enriched metric and trace observations
without reaching through Analytics into lower-level services.

#### Solution

Add a narrow Analytics `TelemetryReader` capability with cursors, timestamps,
watermarks, coverage, distributions, relationships, and safe dimensions.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\otel-projection-implementation-plan.md`

#### Changes

##### Package changes

- Add the root Analytics telemetry read contract and private projection logic
  under `pkg/services/analytics`.
- Keep Costs enrichment inside Analytics.
- Add a package-boundary guard preventing OTel Projection from importing lower
  service roots.

##### Contracts

- Define cursor, batch, watermark, observation, distribution, relationship,
  coverage, and unavailable-status values.
- Define clone/detach behavior for every returned collection.

##### Services

- Analytics exposes deterministic reads from its already-injected dependencies.
- OTel Projection receives only the narrow reader capability.

##### API changes

- None; this is an internal service-root capability.

##### Tests

- Unit tests for deterministic ordering, cursor continuation, timestamp
  preservation, cost enrichment, and unavailable facts.
- Integration tests proving Analytics calls Costs while OTel Projection does
  not.
- Boundary tests proving prohibited imports fail the repository check.

#### Acceptance criteria

- Re-reading the same cursor against unchanged facts returns an equivalent
  detached batch.
- A successful next-cursor read neither mutates nor consumes canonical events.
- Unknown cost remains explicitly unpriced in the batch.
- Analytics and its tests contain no OTel SDK or OTLP protobuf types.

### Story 2: Export the initial OTel metric catalog

#### Problem statement

Customers cannot send Factory duration, queue, reliability, token, cost, and
coverage measurements to an OTel backend with stable semantics.

#### Customer ask

Export aggregatable Factory and GenAI measurements with controlled dimensions.

#### Solution

Implement deterministic metric mapping in `pkg/services/otel_projection`, use
the pinned GenAI conventions where applicable, and send mapped batches through
an injected exporter effect.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\otel-projection-implementation-plan.md`

#### Changes

##### Package changes

- Add OTel Projection root contracts, internal metric mapper, metric catalog,
  and focused construction provider.
- Add a policy-free OTLP sender implementation under `pkg/platform` only if
  the chosen official library does not already provide the exact injectable
  adapter needed by the service.

##### Contracts

- Define projection request/result, exporter batch, signal counts, semantic
  version, and sanitized failure classification.

##### Services

- OTel Projection maps Analytics observations without adding pricing or
  analytical policy.

##### API changes

- None in this story.

##### Tests

- Golden mapping tests for every initial metric, unit, kind, and allowed
  dimension.
- Regression tests rejecting high-cardinality entity IDs from metric points.
- Cost tests covering multiple currencies, estimated/actual status, and
  unpriced usage.
- GenAI convention compatibility tests pinned to the selected schema version.

#### Acceptance criteria

- A representative Analytics batch produces collector-valid metric data.
- Percentages and percentiles are not emitted as mergeable base metrics.
- Every metric has a documented unit, kind, description, and bounded dimension
  set.
- Cost output is labeled by currency and valuation kind and never invents a
  value for unknown usage.

### Story 3: Export Work, dispatch, Worker Session, and provider traces

#### Problem statement

Aggregate metrics cannot explain which Work path or dispatch caused an outlier.

#### Customer ask

Correlate Factory performance measurements with navigable OTel traces.

#### Solution

Project Work lineage into bounded traces, attempts into dispatch spans,
supervised execution into Worker Session spans, and model calls into GenAI
client spans. Use links for fan-in and cross-session continuation and exemplars
for metric-to-trace navigation.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\otel-projection-implementation-plan.md`

#### Changes

##### Package changes

- Add private trace identity, relationship, mapping, and exemplar modules under
  OTel Projection.

##### Contracts

- Extend exporter batches with spans, links, events, status, and exemplar
  references without exposing content fields.

##### Services

- OTel Projection derives trace topology only from Analytics-provided
  relationships.

##### API changes

- None in this story.

##### Tests

- Trace-tree and span-link tests for sequential, parallel, fan-in, retry, and
  cross-session Work fixtures.
- Tests proving continuously running Factory Sessions do not create indefinitely
  open root spans.
- Privacy tests proving prompts, responses, tool arguments, headers, and raw
  errors are absent.
- Exemplar tests linking a metric point to the expected trace/span identity.

#### Acceptance criteria

- A representative Work lineage is navigable from Work through dispatch and
  provider spans.
- Fan-in and predecessor relationships use links rather than false parenting.
- Entity identifiers appear only on permitted span/link/exemplar surfaces.
- Trace timestamps reproduce the recorded operation intervals rather than the
  later export time.

### Story 4: Configure and operate a real OTLP exporter safely

#### Problem statement

Mapped signals are not useful until operators can configure a supported OTLP
destination with predictable lifecycle and failure behavior.

#### Customer ask

Enable OTel export without making Factory execution depend on collector health.

#### Solution

Add `runtime.otel` settings, standard environment resolution, injected OTLP
network transport, bounded retry/backoff, and initializer-managed startup and
shutdown.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\otel-projection-implementation-plan.md`

#### Changes

##### Package changes

- Extend Operator Settings authored/document/effective contracts.
- Add the exact OTLP network effect to `pkg/services/edges` when required for
  functional replacement.
- Compose exporter, Analytics reader, clock, logger, and lifecycle once in
  `pkg/wire`.

##### Contracts

- Add enabled, endpoint, protocol, signal, interval, timeout, start, and
  backfill settings with validation and precedence.

##### Services

- OTel Projection runs one bounded background export loop.
- The external adapter performs one cancellation-aware network attempt per
  call; projection policy owns retries.

##### API changes

- Author `runtime.otel` OpenAPI configuration fragments and regenerate every
  required Go, TypeScript, schema, and publishable client artifact.

##### Tests

- Settings decode/encode/normalization/precedence tests.
- Disabled-mode tests proving no network or background export activity.
- Timeout, transient failure, permanent rejection, cancellation, retry, and
  shutdown-flush tests with deterministic clocks/effects.
- A functional test through `root.BuildProcess` and `Process.Execute`, replacing
  only the exact exporter effect through `edges.Edges`.

#### Acceptance criteria

- Valid OTLP/gRPC and OTLP/HTTP configurations export successfully.
- Invalid configuration fails before exporter lifecycle activation with an
  actionable error.
- Collector failure is visible but does not fail or delay domain operations.
- Credentials never appear in logs, status, errors, test snapshots, or API
  output.
- Shutdown completes within the configured bound whether the collector is
  healthy, slow, or unavailable.

### Story 5: Resume projection with durable checkpoints and explicit gaps

#### Problem statement

Process restart or collector outage can otherwise duplicate retained signals
or silently omit an interval.

#### Customer ask

Resume export predictably and make any unavoidable gap visible.

#### Solution

Persist a destination-scoped Analytics cursor only after successful export,
support explicit retained-history backfill, and report expired-cursor gaps.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\otel-projection-implementation-plan.md`

#### Changes

##### Package changes

- Add an OTel Projection-owned checkpoint contract and a policy-free durable
  store adapter under `pkg/platform`.
- Keep destination fingerprinting and gap policy in OTel Projection.

##### Contracts

- Define checkpoint identity, committed cursor, watermark, schema version,
  first-start policy, gap outcome, and reset reason.

##### Services

- Projection commits after accepted export and resumes from the last committed
  cursor.

##### API changes

- Include checkpoint and gap facts in the later status contract.

##### Tests

- Restart tests for successful resume, failed-batch retry, destination change,
  schema change, retained cursor expiry, and explicit backfill.
- Concurrency tests proving only one loop advances a destination checkpoint.
- Corrupt-checkpoint tests with fail-closed recovery diagnostics.

#### Acceptance criteria

- A failed export never advances the cursor.
- An ordinary restart resumes after the last accepted batch.
- First enablement starts at `latest` unless backfill is explicit.
- Cursor expiry produces a visible gap count and reason.
- The contract promises at-least-once attempts, not impossible end-to-end
  exactly-once delivery.

### Story 6: Expose OTel projection status through `you metrics`

#### Problem statement

Operators cannot tell whether export is disabled, healthy, lagging, rejecting
data, overflowing cardinality, or missing retained history.

#### Customer ask

Inspect OTel projection health from the same metrics command namespace.

#### Solution

Add read-only service status, OpenAPI mapping, and human/JSON CLI output for
`you metrics otel status`.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\otel-projection-implementation-plan.md`

#### Changes

##### Package changes

- Add status operation and detached result types at the OTel Projection root.
- Add service-owned CLI/HTTP adapters and top-level protocol composition.

##### Contracts

- Define lifecycle, destination, signals, versions, lag, counts, timestamps,
  gap, and sanitized failure fields.

##### Services

- Status reads immutable snapshots and causes no exporter side effect.

##### API changes

- Add the read-only status operation and authored schemas under
  `api/components/`, then regenerate all required clients.
- Add `you metrics otel status` to the authored CLI manifest and generated
  command artifacts.

##### Tests

- Service tests for disabled, healthy, retrying, lagging, rejected, overflow,
  gap, and shutdown states.
- CLI golden tests for human and JSON output.
- HTTP contract tests for the same states and secret redaction.
- CLI-first functional proof through the canonical process graph.

#### Acceptance criteria

- Human output identifies the effective state and next operator action.
- JSON contains stable fields and classifications suitable for automation.
- Endpoint credentials and configured headers are absent.
- Reading status does not export, flush, retry, reset, or advance a cursor.

### Story 7: Prove collector interoperability and publish the convention

#### Problem statement

An internally valid mapping can still fail against a real collector or drift
without a published semantic contract.

#### Customer ask

Trust that exported telemetry is accepted by standard OTel tooling and remains
stable across releases.

#### Solution

Publish the custom `you.*` metric/span catalog, pin schema versions, add a
collector interoperability smoke test, and document configuration,
cardinality, privacy, and migration behavior.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\otel-projection-implementation-plan.md`

#### Changes

##### Package changes

- Add versioned semantic-convention/catalog data owned by OTel Projection.
- Add a focused interoperability fixture under
  `tests/functional/observability/otel/`.

##### Contracts

- Record projection schema URL/version and pinned GenAI convention version in
  exported scopes and status.

##### Services

- No new service behavior beyond version publication and compatibility
  validation.

##### API changes

- Document OTel configuration and status in the canonical reference topic.
- Keep generated CLI/API documentation synchronized.

##### Tests

- Validate representative metrics and traces through a supported OTel
  Collector configuration without contacting an external service.
- Add schema/catalog drift checks and sanitized golden payloads.
- Run contract, generation, docs, package-boundary, race/stress, and PR
  verification appropriate to the changed surfaces.

#### Acceptance criteria

- A supported collector accepts the representative OTLP metrics and traces.
- The catalog documents every custom name, unit, kind, dimension, and
  stability level.
- Convention changes require an explicit version/migration update.
- Public documentation explains that OTel is a lossy projection and canonical
  Analytics/Costs results remain authoritative.

## Project-level acceptance criteria

- The production dependency graph is OTel Projection → Analytics → Costs, with
  no reverse or bypass dependency.
- Existing session, Work, Worker Session, and recording services retain their
  current canonical responsibilities.
- OTel export is disabled by default and performs no network calls when
  disabled.
- Collector failure cannot fail, block, or mutate Factory execution or
  canonical analytical results.
- Metrics use controlled cardinality; entity IDs appear only on traces, links,
  exemplars, and safe diagnostics.
- Exact costs and currencies remain Costs-owned; OTel monetary values are
  clearly approximate telemetry projections.
- OpenAPI, CLI manifests, generated Go/TypeScript clients, schemas, and
  published documentation remain synchronized.
- Focused unit, package integration, contract, functional, race/stress, and
  collector interoperability evidence passes.
- Delivery continues through required CI until it is terminal and passing;
  blocking review feedback is addressed, conflicts are resolved, generated
  drift is reconciled, and the pull request is actually merged. Opening or
  approving the PR is not completion.

## Verification plan

Use the narrowest focused commands during each story, then broaden before
merge. Expected final gates include, as applicable:

```text
go test ./pkg/services/analytics/... ./pkg/services/otel_projection/...
go test -race ./pkg/services/otel_projection/...
go test ./pkg/services/operator_settings/... ./pkg/transports/...
go test ./tests/functional/observability/otel/...
make generate-api
make api-smoke
make docs-reference-smoke
make pkg-boundary
make pkg-file-count
make verify-pr
```

The implementation should add a bounded stress case covering sustained
Analytics batches, slow collector responses, retry backoff, checkpoint safety,
and shutdown. Tests must use deterministic clocks and injected effects rather
than sleep-based synchronization.

## Delivery order

1. Land the prerequisite Costs service contract and behavior.
2. Land the prerequisite Analytics service and Costs dependency.
3. Publish the cursorable Analytics telemetry feed.
4. Land metrics mapping before network lifecycle.
5. Add traces, links, and exemplars.
6. Add configuration and the real OTLP adapter.
7. Add durable checkpoints and backfill/gap behavior.
8. Add status CLI/API and customer documentation.
9. Complete collector interoperability, stress verification, blocking review
   resolution, terminal green CI, conflict resolution, and merge.
