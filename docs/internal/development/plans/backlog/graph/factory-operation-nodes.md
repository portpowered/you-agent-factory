# Factory Operation Nodes Plan

## Status

Proposed.

## Problem

Factory graphs can run workers, classify results, move Work, repeat, schedule
cron Work, and express several specialized guard patterns. They do not yet
provide a coherent customer-facing way to model common orchestration
operations:

- stateful stream selection such as skip, take, or pick-first;
- exact-count Work generation and collection of several Work items of the same
  type;
- typed conditions over structured Work output;
- deterministic random choice and seed generation;
- timers and windows such as delay, debounce, throttle, and timeout; and
- utilities such as tee, artifact creation, and bounded file writes; plus
- a deliberately small external surface for file/email Work ingress, email
  delivery, and outbound HTTP requests.

Some of these behaviors exist internally as Petri arc cardinalities,
parent/child guards, generated `FACTORY_REQUEST_BATCH` output, cron time tokens,
or JavaScript `parallel()` / `pipeline()`. They are hard to compose in an
authored graph because each uses a different special-case contract. The result
is that customers must understand internal topology or write a script for
behaviors that should be ordinary graph nodes.

## Customer Ask

Customers should be able to author bounded, replayable operation nodes for
stream selection, structured routing, fan-out, fan-in, aggregation, randomness,
time, and utility effects. External ingress/capability nodes should be a closed
allowlist rather than a general integration catalog. Together, those nodes
should be sufficient to build, among other workflows:

1. an evolutionary Factory that creates a population, evaluates it, selects and
   mixes candidates, and repeats for a configured number of rounds; and
2. a leaderboard Factory that runs up to a configured number of autonomous
   agents, gives each agent isolated branch/notes state, evaluates submissions,
   updates standings, and stops after a global execution ceiling.

## Current Baseline

The implementation plan must extend, rather than duplicate, these current
capabilities:

- `FactoryWorkstationConfig` already distinguishes worker runs,
  `LOGICAL_MOVE`, classifiers, cron, and pollers.
- The internal Petri scheduler already supports `ONE`, `N`, `ALL`,
  `ALL_TERMINAL`, and `ZERO_OR_MORE` cardinalities.
- Generated worker output can already admit bounded Work batches with lineage
  and `DEPENDS_ON` relationships.
- `ALL_CHILDREN_COMPLETE`, `ANY_CHILD_FAILED`, `MATCHES_FIELDS`, `SAME_NAME`,
  `SAME_TRACE_ID`, and `VISIT_COUNT` guards provide several narrow correlations.
- Work has canonical typed content, including JSON content parts, plus payload,
  tags, trace identity, parent identity, and relationships.
- Factory Runtime already receives clock and randomness implementations through
  explicit boundaries; the browser emulator already uses deterministic virtual
  time and seeded identities.
- JavaScript Factories already expose `agent.run()`, `parallel()`, `pipeline()`,
  phases, checkpoints, and bounded child policy. They remain the escape hatch
  for algorithms that do not fit the built-in catalog.

## n8n Core-Node Review

This plan also reviews the current [n8n core-node catalog](https://docs.n8n.io/integrations/builtin/core-nodes/)
as a mature workflow-authoring reference. The goal is semantic coverage, not
name-for-name compatibility. n8n processes generic item lists and integrates
many external systems directly; You Agent Factory owns typed Work, worker and
provider execution, Factory Sessions, Automations, canonical Factory Events,
and replay. A node that is “core” in n8n may therefore belong in an existing
service here rather than in the operation runtime.

Detailed comparison used the documented semantics of n8n's
[Aggregate](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.aggregate/),
[Merge](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.merge/),
[Limit](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.limit/),
[Remove Duplicates](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.removeduplicates/),
[Sort](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.sort/),
[Split Out](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.splitout/),
[Loop Over Items](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.splitinbatches/),
[Summarize](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.summarize/),
[Compare Datasets](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.comparedatasets/),
[Wait](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.wait/),
[Edit Fields](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.set/),
[Rename Keys](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.renamekeys/),
[Date & Time](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.datetime/),
[Stop And Error](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.stopanderror/),
[Local File Trigger](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.localfiletrigger/),
[Email Trigger (IMAP)](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.emailreadimap/),
[Send Email](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.sendemail/),
[HTTP Request](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.httprequest/),
[Data Table](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.datatable/),
[Convert to File](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.converttofile/),
and [Extract From File](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.extractfromfile/).

### Adopt as operation semantics

| n8n concept | Factory operation decision | Adaptation for this repository |
| --- | --- | --- |
| No Operation | `PASS` | Preserve typed Work and lineage; emit a canonical operation fact; create no worker dispatch. |
| Filter, If, Switch | `FILTER` and ordered `SWITCH` | Use typed predicates and named false/default routes. `If` is the two-route `SWITCH` presentation, not a distinct runtime kind. |
| Limit | `TAKE` plus closed-collection `LIMIT` | Streaming take-first closes after N; take-last requires a closed/bounded collection and cannot pretend future items are known. |
| Remove Duplicates | `DISTINCT` | Support selected keys and bounded within-collection/session history. Cross-session history is deferred until a durable owner exists. |
| Sort | `SORT` and seeded `SHUFFLE` | Require typed keys, explicit null ordering, stable tie-breakers, and recorded randomness rather than JavaScript string coercion. |
| Split Out | `SPLIT` | Convert a selected JSON array/content collection into atomic child Work with deterministic index and lineage. |
| Loop Over Items | `BATCH` | Release bounded groups to a loop route and emit a done route after the registered collection is exhausted; require termination and iteration ceilings. |
| Aggregate | `COLLECT` plus `MERGE_CONTENT` | Separate waiting/collection semantics from output shaping; support selected fields, list flattening, and missing/null policy. |
| Summarize | `REDUCE` | Add average, count-unique, concatenate, and group-by to count/sum/min/max/latest/top-K. |
| Merge | `JOIN` plus append/collection modes | Support append, keyed inner/left/right/full joins, and positional zip with explicit duplicate/clash policy. Defer SQL and unbounded Cartesian products. |
| Compare Datasets | `DIFF` | Emit only-left, equal, changed, and only-right routes with typed match keys and explicit field comparison policy. |
| Edit Fields, Rename Keys | `PROJECT` | Set, copy, remove, and rename selected typed fields. No arbitrary expression or regex engine is embedded in the contract. |
| Stop And Error | `FAIL` | End the scoped Work or Factory path with a stable code, sanitized message/details, and canonical failure event. |
| Date & Time | `DATE_TIME` | Parse, format, add/subtract, extract, round, and calculate differences using explicit timezone and locale policy; current-time reads use the injected clock. |
| Wait after interval/at time | `DELAY` / timed `WAIT` | Record deadlines and use the injected clock. External webhook/form resume is a separate durable-session feature. |
| Read/Write Files | `WRITE_ARTIFACT` / bounded `WRITE_FILE` | Use authorized roots, atomic/idempotent effects, quotas, and replayable metadata. |

### Retain only an explicit ingress and capability allowlist

The n8n catalog does not establish a general integrations backlog for this
product. Near-term external ingress and effect nodes are limited to the
following allowlist:

| Approved surface | Public shape | Repository owner/answer |
| --- | --- | --- |
| Local file watch | `FILE_WATCH` Automation | `pkg/services/automations` observes bounded create/modify/delete changes under authorized roots and admits mapped Work through `pkg/services/work`. It is not a mid-graph polling operation. |
| Email receive | `EMAIL_RECEIVE` Automation | Automations observes one configured mailbox/folder/filter, checkpoints provider message identity, and admits bounded message/attachment content as Work. |
| Email send | `SEND_EMAIL` effect operation | An explicit effect node sends one bounded, fully rendered message through a credential reference and records provider delivery metadata. It is not a generic connector or arbitrary command. |
| Outbound HTTP request | `HTTP_REQUEST` effect operation | An explicit effect node performs one policy-constrained request and emits a bounded response/result. It is outbound only; it does not create an inbound webhook trigger. |

`FILE_WATCH` and `EMAIL_RECEIVE` are Work ingress, so their durable cursor,
deduplication, schedule/push observation, backpressure, and admission policy
belong to Automations and Work. `SEND_EMAIL` and `HTTP_REQUEST` are explicit,
recorded effects and therefore remain outside the pure predicate/collection
kernel. None of these contracts creates a generic connector registry, dynamic
plugin node, or arbitrary network/process execution surface.

Existing product entry points and internal capabilities remain supported, but
are not promoted into new user-authored nodes by this plan. In particular,
native CLI/API/MCP/ACP invocation is not a "trigger node"; existing cron and
poller behavior is not expanded; Worker worktrees are not a generic Git node;
and `SCRIPT_RUN` compatibility is not an Execute Command node.

AI transforms, chat, evaluation, and semantic guardrails continue to use the
existing `INFERENCE_RUN`, `AGENT_RUN`, classifier, output-schema, provider, and
model contracts. JavaScript Factory orchestration remains the escape hatch for
bespoke code. A deterministic operation must never hide an inference call.

### Defer or exclude from the core operation program

| n8n concept | Decision |
| --- | --- |
| Data Table | Defer. A cross-Factory mutable table needs a durable domain owner, authorization, schema/versioning, query, retention, and event/replay relationship; operation-local state is not that database. |
| Wait on webhook/form | Defer until durable Factory Session suspension/resume and one-time authenticated correlation are product contracts. The current process-local queue cannot safely emulate n8n's database-offloaded wait. |
| Activation, Manual, Schedule, RSS, SSE, Webhook, form, workflow, evaluation, error, chat, MCP-server, and vendor-specific triggers | Exclude as new node kinds. Preserve existing native invocation and automation behavior where already supported, but do not broaden the ingress catalog beyond `FILE_WATCH` and `EMAIL_RECEIVE` in this plan. |
| Respond to Webhook, chat response, and form response | Exclude. They imply durable external correlation and a product interaction surface that is not part of the approved ingress/effect allowlist. |
| Execute Command, Git, SSH, FTP, GraphQL, LDAP, RSS Read, MCP Client, and vendor/service integrations | Exclude as capability nodes. Existing internal uses keep their current owners, but this plan adds no generic command, connector, protocol, or credential-driven integration surface. |
| Execute Sub-workflow and its trigger | Exclude from this program. Child-Factory invocation would require an explicit Factory Sessions/Runtime contract for recursion, budget, cancellation, results, and lineage. |
| Execution Data | Do not add a node. Factory Events, recordings, projections, logs, metrics, and tags remain the observability surfaces. |
| Cross-execution deduplication | Defer with Data Table/durable state ownership. Initial `DISTINCT` is bounded to a Factory Session or explicit input collection. |
| SQL Merge and custom-code Sort | Exclude from built-ins. Typed joins/sorts cover the stable cases; JavaScript remains the escape hatch. |
| All-possible-combination Merge | Defer unless a bounded use case justifies it; Cartesian growth must have prospective output and byte ceilings before execution. |
| Compression, HTML, Markdown, XML, CSV/spreadsheet/PDF/file extraction and conversion | Add later as `ENCODE_CONTENT` / `DECODE_CONTENT` adapters owned with Work content/materialization, one format family at a time. Do not make a large codec bundle a prerequisite for orchestration operations. |
| Crypto, JWT, and TOTP | Defer to security/credential-owned capabilities. Seeded orchestration randomness is not cryptographic randomness, signing, token issuance, or secret custody. |
| Edit Image | Use model/media services or a dedicated capability; it is not a general orchestration primitive. |
| Debug Helper | Keep test/debug fixtures outside the public production-node catalog. |
| n8n administration node | Not applicable; Factory Definition/Session and operator-setting APIs already own this product's administration. |

The resulting principle is: adopt nodes that perform deterministic routing,
collection, transformation, bounded state, or time over Work; permit only the
four named ingress/effect surfaces above; and exclude other trigger and
integration nodes instead of implying a future connector catalog.

## Goals

- Make common orchestration behavior readable and editable as first-class
  Factory graph nodes.
- Give stateful nodes explicit scope, ordering, buffering, closure, and overflow
  semantics.
- Make structured predicates typed, fail-closed, and reusable by guards and
  conditional routing.
- Generate and collect exact or bounded sets of same-type Work without repeated
  hand-authored inputs or hidden prompt protocols.
- Keep time, randomness, filesystem IO, and mutable operator state deterministic
  or explicitly recorded so live execution and replay agree.
- Admit Work from authorized file changes and received email, and support only
  the explicitly approved email-send and outbound-HTTP external effects.
- Preserve Work identity, content, lineage, relationships, and terminal failure
  behavior across every operation.
- Provide safe bounds, backpressure, diagnostics, and UI validation before a
  Factory runs.
- Prove the catalog with evolutionary and leaderboard packaged examples.

## Non-Goals

- Reimplement all of RxJS or expose an arbitrary stream-programming language in
  Factory JSON/YAML.
- Expose Petri places, tokens, arcs, or markings as customer-facing operation
  configuration.
- Add arbitrary Go templates, JavaScript, shell commands, or regular expressions
  to guards as a shortcut around a typed predicate contract.
- Replace JavaScript Factories. Complex bespoke algorithms can still use the
  JavaScript orchestrator and its existing bounded child primitives.
- Make operation nodes a second Work store, event ledger, artifact store, or
  scheduler.
- Add RSS, SSE, webhook, form, chat, MCP, vendor, database, shell, Git, SSH,
  FTP, GraphQL, LDAP, or other generalized trigger/integration nodes. Existing
  native transports, automation behavior, worktrees, and script runners are not
  removed, but this plan does not expand them into graph-node families.
- Build a generic connector marketplace, capability registry, credential
  broker, or arbitrary request/command node. `SEND_EMAIL` and `HTTP_REQUEST`
  are closed, independently governed contracts.
- Promise recovery of unrelated in-flight provider execution. Operation state
  will be replayable, but broader Factory Session restart guarantees remain the
  responsibility of Factory Sessions and Factory Runtime recovery.

## Product Model

### One workerless operation workstation

Add one public workerless workstation type, `OPERATION`, rather than a separate
runtime workstation type for every utility. Its configuration is carried by a
new `builtin` object:

```yaml
workstations:
  - id: first-valid-result
    name: first-valid-result
    type: OPERATION
    builtin:
      kind: TAKE
      count:
        literal: 1
      scope:
        kind: TRACE
    inputs:
      - id: candidate
        workType: candidate
        state: evaluated
    outputs:
      - id: selected
        workType: candidate
        state: selected
      - id: overflow
        workType: candidate
        state: not-selected
```

`operation` remains the existing provider-neutral model operation field used by
`MODEL_INVOKE`; it is not overloaded for built-in graph behavior. `builtin.kind`
is a generated public enum and `builtin.config` is a discriminated contract for
the selected kind.

An operation is compiled by Factory Definitions into the existing
transport-neutral Factory Runtime model. The internal Petri compiler may use
arcs, cardinalities, guards, and system tokens to execute it, but those remain
implementation details.

### Stable ports and collection inputs

Add an optional stable `id` to workstation inputs and outputs. Guard references,
predicate input references, UI handles, and operation routes use the port ID.
Existing definitions without IDs retain their current generated binding names.

An operation input can declare collection behavior without repeating the same
Work type:

```yaml
inputs:
  - id: candidates
    workType: candidate
    state: evaluated
    collection:
      mode: EXACT
      count:
        invocationArgument: populationSize
      groupBy:
        source: PARENT_ID
```

Supported collection modes are `ONE`, `EXACT`, `AT_LEAST`, `ALL_REGISTERED`,
and `UNTIL_CLOSED`. `ALL_REGISTERED` uses an explicit generated-child
registration as its denominator; it never guesses completion from the Work
currently visible in one state.

### Shared value sources

Counts, durations, keys, seeds, and other scalar settings use one bounded value
source contract:

- `literal`: an authored JSON scalar;
- `invocationArgument`: one normalized Factory invocation argument;
- `selector`: a value selected from one named input; or
- `default`: an optional fallback used only when the primary source is absent.

Validation defines the required output type, minimum, maximum, and whether a
fallback is allowed for each use. Missing, malformed, non-finite, negative, or
out-of-policy values fail before partial Work or effects are produced.

### Structured selectors and predicates

Selectors address canonical Work fields, tags, invocation arguments, or a
canonical content part. JSON content uses RFC 6901 JSON Pointer after selecting
the part by stable slot or label. A selector never relies on map insertion order
or an ambiguous “first JSON object” when several parts match.

Predicates form a small typed tree:

- boolean composition: `ALL`, `ANY`, `NOT`;
- presence/type: `EXISTS`, `IS_NULL`, `TYPE_IS`;
- comparison: `EQ`, `NE`, `LT`, `LTE`, `GT`, `GTE`;
- collection/text: `CONTAINS`, `STARTS_WITH`, `ENDS_WITH`, `IN`;
- schema: `JSON_SCHEMA_VALID` using an authored or bundled schema reference; and
- operands: literal, invocation argument, or selector from a named input.

Comparisons do not coerce strings into numbers or booleans. An absent path,
wrong type, invalid schema, or ambiguous content part fails closed and produces
a structured diagnostic. Sensitive invocation arguments cannot be projected
into events, output content, paths, or UI previews.

Pure predicates can be attached as readiness guards, but conditional business
routing should use `SWITCH` or `FILTER`. A false readiness guard leaves Work
waiting; a conditional operation explicitly routes false/unmatched Work and
therefore does not strand it accidentally.

### Stateful operation scope and ordering

Every stateful operation declares a scope:

- `TRACE` (default): workstation plus canonical chaining trace;
- `PARENT`: workstation plus parent Work identity;
- `SESSION`: workstation plus Factory Session; or
- `VALUE`: workstation plus a typed selected partition key.

Items are observed in canonical Factory Event sequence order, with Work ID as a
stable tie-breaker when one accepted event contains several items. Provider
completion timing is not re-sorted later. Each state change records its scope
key, selected Work IDs, prior version, next version, and outcome.

Stateful operations must declare or inherit bounds for buffered items, bytes,
open groups, timers, generated Work, and total firings. Reaching a bound routes
to an explicit overflow/failure output or fails the operation with a stable
code; it never silently drops Work.

### Deterministic time and randomness

Runtime time comes from the injected Factory Runtime clock. Timed operations
record scheduled, canceled, and fired timer facts with logical deadline and
operation scope. Tests and emulator execution advance an injected/virtual clock
and do not sleep.

A Factory Session has one recorded root random seed. A caller may supply it;
otherwise a platform random source creates it once and the accepted seed is
recorded before use. Each random operation derives a substream from the root
seed, workstation ID, scope key, and firing ordinal. The chosen value or order is
recorded in the operation event, so replay projects the choice instead of
sampling again.

Randomness is an operation, not a guard side effect. Re-evaluating a readiness
guard must never consume random state or change its answer merely because the
scheduler inspected it again.

### Canonical operation facts

Recordings remains the canonical Factory Event ledger. Add the minimum public
facts needed to explain and replay operation behavior:

- `OPERATION_APPLIED`: kind, workstation, scope, consumed/observed Work,
  selected route, derived values, state version, and related Work Request IDs;
- `OPERATION_TIMER_SCHEDULED`, `OPERATION_TIMER_CANCELED`, and
  `OPERATION_TIMER_FIRED` for durable time decisions; and
- `OPERATION_EFFECT_COMPLETED` / `OPERATION_EFFECT_FAILED` for external effects
  with idempotency identity and artifact/file metadata.

Canonical `WORK_REQUEST`, `WORK_STATE_CHANGE`, relationship, and artifact facts
remain authoritative for Work and artifact state. Operation facts reference
those records rather than duplicating their complete payloads. Replay does not
rerun predicates, randomness, timers, or filesystem writes.

## Operation Catalog

The catalog lands in capability groups. Every operation has an explicit success
route, explicit non-selected/failure routes where applicable, deterministic
ordering, bounded configuration, and a UI editor.

| Group | Kinds | Observable behavior |
| --- | --- | --- |
| Routing | `PASS`, `FILTER`, `SWITCH`, `TEE`, `FAIL` | Move, conditionally route, copy lineage, or terminate with an explicit failure without a worker dispatch. |
| Selection | `SKIP`, `TAKE`, `LIMIT`, `DISTINCT`, `SORT` | Select/order Work according to scoped accepted-item history or one explicitly closed collection. `TAKE` with count 1 is pick-first. |
| Fan-out | `GENERATE`, `SPLIT`, `BATCH` | Create an exact bounded child population, split a selected array into children, or release registered members in bounded batches. |
| Fan-in | `COLLECT` | Wait for exact, quorum, all-registered, or closed groups and emit a stable ordered collection. |
| Combine | `JOIN`, `DIFF` | Append/zip/key-join bounded collections or route their keyed equal/changed/one-sided members. |
| Transform | `PROJECT`, `MERGE_CONTENT`, `DATE_TIME` | Build canonical typed Work content, merge a collection, or perform explicit timezone-aware date/time transformations. |
| Aggregate | `REDUCE` | Maintain bounded built-in aggregates: count, count-unique, sum, average, min, max, concatenate, latest-by-key, group-by, and top-K. |
| Random | `RANDOM_VALUE`, `SHUFFLE`, `SAMPLE` | Produce seeded values or deterministic permutations/samples. |
| Time | `DELAY`, `WAIT`, `DEBOUNCE`, `THROTTLE`, `TIMEOUT` | Route Work according to recorded deadlines and scoped activity. Initial `WAIT` supports interval or absolute time, not webhook/form resume. |
| Effects | `WRITE_ARTIFACT`, `WRITE_FILE`, `SEND_EMAIL`, `HTTP_REQUEST` | Perform one explicitly approved, bounded side effect and emit a recorded result with idempotency, retry, and failure policy. |

`REDUCE` is intentionally a catalog of built-in reducers, not a customer-coded
function. A reducer outside that set belongs in a JavaScript Factory until it
has stable semantics worth promoting.

### Important per-kind semantics

- `SKIP(n)` routes the first `n` items in a scope to `skipped`, then later items
  to `selected`.
- `TAKE(n)` routes the first `n` items to `selected`, closes the scoped gate, and
  routes later items to `overflow`. `TAKE(1)` is the canonical pick-first node.
- `DISTINCT` requires a typed key selector and a bounded retention policy:
  session lifetime, count window, time window, or one closed input collection.
- `LIMIT` operates on a closed collection and can retain its first or last N
  members. Streaming pick-first remains `TAKE`; streaming “last N” is invalid
  because the future collection is not yet known.
- `SORT` requires typed keys, direction, null placement, and a stable Work-ID
  tie-breaker. Seeded random ordering is `SHUFFLE`, not an implicit sort mode.
- `TEE` emits one derived Work item per declared output, preserving a common
  parent/trace and assigning deterministic child identities. It is not an
  aliasing reference to one mutable token.
- `GENERATE` resolves its count before producing anything, validates the full
  prospective batch, then atomically admits exactly that many children with
  deterministic `index`, `generation`, `seed`, parent, and spawned-by metadata.
- `COLLECT` groups by trace, parent, or selected key; selects members in stable
  event order; emits once per group unless configured for sliding updates; and
  declares whether source Work is consumed or observed.
- `SPLIT` selects one bounded JSON array and atomically creates one child per
  member, with optional projection of the other parent fields. `BATCH` releases
  registered members in fixed-size groups and emits `done` only after every
  member is accounted for or explicitly failed.
- `JOIN` supports append, positional zip, and typed key joins with explicit
  inner/left/right/full output, multiple-match, unpaired-item, deep/shallow
  field-clash, and output-size policies. It does not accept SQL.
- `DIFF` shares the key and comparison engine with `JOIN` but emits named
  only-left, equal, changed, and only-right routes instead of one merged set.
- `SWITCH` evaluates ordered cases and requires a default route unless the
  author explicitly selects fail-on-no-match.
- `REDUCE(top-K)` requires a numeric score selector, stable descending order,
  deterministic tie-breaker, and maximum retained count. `latest-by-key`
  requires both key and ordering selectors. Group-by and count-unique have
  explicit missing/null policies and maximum group/cardinality bounds.
- `DATE_TIME` requires an explicit input format/timezone when parsing is not
  unambiguous, uses the Factory/session timezone only when selected, and reads
  “now” through the injected clock.
- `DEBOUNCE` retains the latest item and routes replaced items to `superseded`;
  `THROTTLE` routes suppressed items to `suppressed`; `TIMEOUT` has separate
  completed and timed-out routes.
- `FAIL` records a stable non-sensitive error code plus bounded message/details,
  then follows the Work/Factory failure policy; it cannot fabricate a success
  output.
- `WRITE_ARTIFACT` writes through the Recordings artifact operation.
  `WRITE_FILE` writes atomically under an explicitly authorized Factory/session
  root, rejects traversal and unsafe symlinks, and declares `FAIL`, `REPLACE`, or
  ordered `APPEND` collision policy. It emits artifact/file metadata instead of
  hiding the side effect.
- `SEND_EMAIL` renders bounded recipients, subject, body, and attachments from
  typed selectors; uses an opaque credential reference; rejects secret
  material in recorded output; and records an idempotency key plus sanitized
  provider delivery identity. Automatic retry is allowed only when the adapter
  can preserve at-most-once intent or reconcile an ambiguous result.
- `HTTP_REQUEST` uses an allowed method, validated URL, bounded headers/body,
  opaque credential reference, redirect policy, timeout, response-size limit,
  and explicit status/body output mapping. Network policy must address private
  address ranges, DNS rebinding, unsafe redirects, TLS, and sensitive header
  redaction. Automatic retries default to idempotent methods only unless the
  author supplies an adapter-supported idempotency contract.

### Approved Work ingress

Ingress remains outside the `OPERATION` workstation catalog because it creates
Work from external observations rather than transforming already-admitted
Work:

- `FILE_WATCH` watches explicit authorized roots and event kinds with glob,
  debounce/coalescing, stable cursor, size/content policy, and a Work template.
  Restart and duplicate filesystem notifications must not admit duplicate Work
  for the same checkpointed observation.
- `EMAIL_RECEIVE` observes an explicit account, mailbox/folder, and supported
  filter with a durable provider cursor and message-identity deduplication. It
  maps headers, text/HTML bodies, and quota-checked attachments into typed Work,
  redacts credentials, and exposes malformed/oversized/dead-letter outcomes.

Both ingress kinds use the existing Automation lifecycle and Work admission
contracts. They do not execute inside Factory Runtime, and replay of a Factory
Session never polls the filesystem or mailbox again.

## Architecture And Ownership

### Factory Definitions

`pkg/services/factory_definitions` owns the authored operation schemas,
normalization, validation, compatibility behavior, and compilation request. Its
private contract and validation code should gain operation values rather than a
new top-level product service.

Validation must cover:

- operation kind/config discrimination;
- required and allowed ports per kind;
- stable unique port IDs and valid references;
- selector and predicate typing;
- count/duration/buffer limits;
- required false, overflow, timeout, superseded, and failure routes;
- bounded state and effect policy;
- illegal worker, prompt, model, cron, classifier, or operation combinations;
- cycles that can fire synchronously without a configured bound; and
- portability of schema and file references.

### Factory Runtime

`pkg/services/factory_runtime` owns operation execution policy and exposes only
transport-neutral Factory behavior. Add an internal operation runtime under the
Factory Runtime owner with focused pure packages for selectors, predicates,
value resolution, and reducers, plus stateful executors for selection, groups,
timers, randomness, and effects.

The orchestration compiler maps `OPERATION` nodes into runtime transitions. The
scheduler continues to own atomic enablement and claims. The operation runtime
returns proposed Work mutations, Work Request admissions, timer changes, and
effects; the normal event-first loop accepts and records them. Operations do not
mutate canonical world state directly.

### Work

`pkg/services/work` remains the owner of Work Request admission, typed content,
lineage, relationships, and materialization. `GENERATE`, `TEE`, `COLLECT`,
`PROJECT`, and `MERGE_CONTENT` call narrow injected Work service operations or
pure Work value contracts; they do not create a parallel operation-owned Work
model.

Generated and derived Work must preserve trace lineage, declare
`PARENT_CHILD` / `SPAWNED_BY` relationships where applicable, and use atomic
batch admission. Existing worker-emitted generated Work remains supported and
uses the same validation/limit policy.

### Recordings And Platform

`pkg/services/recordings` owns operation event persistence, replay projections,
and session artifacts. `pkg/platform/clock`, platform randomness, and focused
portable file, mail, and HTTP implementations provide policy-free effects.
Factory Runtime chooses operation policy; Platform must not choose Work,
Factory, scheduling, recipient/endpoint authorization, retry, path, or collision
policy.

All service and effect dependencies are injected directly once through the
canonical `pkg/wire` graph. No operation registry may become a service locator,
secondary injector, or runtime constructor graph.

### Automations, Workers, And JavaScript

- Automations continues to own existing external cron, watcher, poller, and
  hosted-source Work admission. This plan adds or hardens only `FILE_WATCH` and
  `EMAIL_RECEIVE`; it does not add new trigger families. Intra-session
  delay/window timers belong to Factory Runtime.
- Workers continues to own agent worktrees, prompts, provider dispatch, and
  request-scoped execution. Operation nodes are workerless.
- JavaScript Factories continue to use `parallel()` / `pipeline()` for bespoke
  dynamic algorithms. Built-ins and JavaScript must converge on the same Work,
  dispatch, event, budget, artifact, and replay contracts rather than maintain
  separate copies of selection or file policy.

### Expected implementation surfaces

The exact file split should stay within package file-count limits, but the
implementation is expected to extend these current owners rather than create a
new top-level package family:

| Surface | Current files/directories to extend | Expected new focused code |
| --- | --- | --- |
| Authored contracts | `pkg/services/factory_definitions/internal/contracts/factory_config.go`, `contracts_root.go` | Focused operation contract files at the Factory Definitions root/private contract boundary. |
| Definition validation | `pkg/services/factory_definitions/internal/services/validation/impl/`, `internal/services/validation/internal/topology/` | Operation discrimination, selector/predicate, port, bounds, and cycle rules beside existing topology validation. |
| Runtime compilation | `pkg/services/factory_runtime/internal/services/orchestration/definitionmapping/mapper.go` | Operation mapping helpers/tests that lower public config into runtime transitions without exposing Petri types publicly. |
| Scheduling/runtime | `pkg/services/factory_runtime/internal/services/orchestration/scheduler/`, `runtime/`, `subsystems/` | An `operations/` internal area split by pure evaluation, scoped state, timers, and effects; avoid growing existing large files. |
| Work/lineage | `pkg/services/work/admission_contract.go`, `contracts.go`, `lineage_contract.go` | Narrow admission/materialization value contracts only where the current Work root lacks the required operation call. |
| Events/replay | `pkg/services/factory_definitions/internal/contracts/factory_events.go`, Factory Runtime recording/projection contracts, `pkg/services/recordings/` projections | Operation event payloads and state reducers; no duplicate ledger. |
| Approved ingress | `pkg/services/automations/`, `pkg/services/work/` admission contracts | `FILE_WATCH` hardening plus focused `EMAIL_RECEIVE` observation, durable cursor/deduplication, mapping, quota, and dead-letter behavior; no generic trigger registry. |
| Approved external effects | Factory Runtime effects, `pkg/services/edges/`, focused `pkg/platform/` adapters | Narrow mail-send and outbound-HTTP ports with injected implementations, authorization/redaction, idempotency, retry, and bounded response/delivery results; no connector service locator. |
| OpenAPI | `api/components/schemas/data-models/Workstation.yaml`, `WorkstationIO.yaml`, `WorkstationType.yaml`, guard schemas, `api/components/schemas/events/FactoryEvent*.yaml` | New operation schemas under `api/components/schemas/data-models/` and operation projection schemas under the owning event/world families. |
| Packaged schema | `packages/packaged-factories/schemas/factory.schema.yaml` and generated package artifacts | Generated operation definitions and authored example Factories; generated JSON/YAML remains generated. |
| Editor operations | `ui/src/features/current-factory-definition/lib/`, `ui/src/features/current-selection/workstation-selection/editing/` | Pure operation-node mutations, validation, and React Flow projection adapters under feature-owned `lib/` / `editing/` directories. |
| Editor components | `ui/src/features/current-selection/workstation-selection/components/editable/`, feature message catalogs | Kind-specific fields composed from shared form/action primitives; no parallel canonical schema in component state. |
| Emulator | `ui/packages/factory-emulator/src/`, schema and frozen runtime references | Hosted-parity implementations for the declared deterministic subset only. |
| Functional proof | `tests/functional/orchestration/`, `tests/stress/` | Root-built operation scenarios and bounded concurrency/time/effect stress cells. |

## API, Generated Contracts, And Compatibility

Author changes in `api/openapi-main.yaml` and `api/components/`:

- add `OPERATION` to `WorkstationType`;
- add `Workstation.builtin` and the operation kind/config schemas;
- add stable `id` and optional `collection` to `WorkstationIO`;
- add selector, value-source, predicate, scope, limit, route, aggregation, timer,
  random, artifact, file-effect, email-send, and HTTP-request schemas;
- add `FILE_WATCH` and `EMAIL_RECEIVE` Automation schemas without adding a
  general trigger or connector union;
- extend guard schemas with a pure structured predicate attachment;
- add operation event payloads and replay projection state; and
- preserve existing `operation` / model-invoke, legacy Workstation, and guard
  decoding.

Run `make generate-api` for the direct OpenAPI and generated Go/dashboard
clients. Run `make interfaces-all` when publishable contracts, packaged Factory
schemas, or UI package clients change. Never hand-edit generated outputs.

Compatibility rules:

- Existing Factories behave identically when `builtin` and port IDs are absent.
- `LOGICAL_MOVE` remains supported; no automatic rewrite to `PASS` occurs.
- Existing specialized fan-in guards remain supported. New authoring and docs
  prefer `COLLECT` after parity is proven.
- Existing worker-emitted Work batches remain supported. `GENERATE` provides a
  prompt-independent option, not a replacement requirement.
- Unknown operation kinds and unsupported emulator kinds fail validation with
  precise paths rather than degrading to `LOGICAL_MOVE`.

## Dashboard And Graph Editor

The canonical editable source remains the generated Factory definition held by
the current-Factory feature. React Flow nodes, handles, badges, and summaries
are disposable projections from that definition plus explicit UI state.

Add feature-owned operations for:

- add an operation node from the node palette;
- change operation kind while preserving only compatible configuration;
- add/remove/reorder switch routes and predicate groups;
- set value sources, selectors, scope, cardinality, bounds, and effect policy;
- connect/disconnect stable input and output ports;
- validate/save through the existing current-Factory mutation path; and
- show server validation without reconstructing canonical state from React
  Flow.

The UI should group operations as Routing, Selection, Fan-out/in, Transform,
Aggregate, Random, Time, and Effects. Each editor shows only the fields and
ports valid for its kind, plus a concise semantic preview such as “take first 1
per trace; overflow → not-selected.” Side-effect nodes prominently show the
relevant authorized root, credential reference, destination policy, retry, and
idempotency behavior. Automation authoring exposes only the approved file-watch
and email-receive ingress additions from this plan.

Reuse existing shared controls, dialogs, selects, fields, action rows, and
status treatments. All new copy belongs to feature-owned message catalogs.
Operation forms and graph handles must be keyboard-operable, mobile-friendly,
and explicit about loading, validation, save failure, and stale definition
state.

## Work Stories

Each story is a vertically sliced, independently reviewable behavior. A story
includes its authored schema, backend behavior, generated contracts, editor
support, documentation, and focused tests when those surfaces are part of the
behavior.

### OPS-001: Add and run a pass-through operation node

As a Factory author, I can add an `OPERATION` workstation with `PASS`, save it,
run Work through it without a worker dispatch, and inspect the resulting
operation and Work events.

Acceptance criteria:

- Factory JSON/YAML, OpenAPI, generated Go/TypeScript clients, packaged schema,
  import/export, and graph projection preserve `type: OPERATION`, `builtin`, and
  stable port IDs.
- Definition validation rejects workers, prompts, models, cron config, or
  missing/duplicate ports on a `PASS` operation with precise field paths.
- `PASS` applies through the event-first runtime, preserves Work content and
  lineage, and emits no Worker or Provider dispatch.
- The editor can add, connect, inspect, save, reload, and delete the node while
  canonical Factory state remains the source of truth.
- Old definitions and `LOGICAL_MOVE` behavior remain unchanged.
- Contract, definition, runtime/replay, UI operation/projection/component, and
  one root-built functional test pass.

### OPS-002: Route structured output with typed predicates

As a Factory author, I can select a JSON field from Work content and use typed
conditions to route Work through `FILTER` or ordered `SWITCH` cases.

Acceptance criteria:

- Selectors can address stable content slot/label plus JSON Pointer, canonical
  Work fields, tags, and non-sensitive invocation arguments.
- Predicate validation rejects missing inputs, ambiguous content parts, invalid
  pointers, incompatible operand types, unsupported operators, and sensitive
  value exposure before execution.
- `SWITCH` chooses the first matching case in authored order and uses its
  required default/fail policy when no case matches.
- `FILTER` visibly routes pass and fail; a false condition does not leave Work
  silently waiting.
- A case/default route may target `FAIL`, which records the configured stable
  error code and sanitized bounded details and produces no success Work.
- Pure predicate guards reuse the same evaluator, fail closed, and create no
  time, random, or IO side effects.
- Replay yields the recorded route without reevaluating the predicate.
- Editor predicate operations, projections, accessible controls, contract
  tests, evaluator unit tests, runtime integration tests, and functional JSON
  routing tests pass.

### OPS-003: Tee Work to several explicit branches

As a Factory author, I can use `TEE` to emit one derived Work item on each named
output with deterministic identity and common lineage.

Acceptance criteria:

- A tee requires at least two unique output ports and validates the complete
  derived batch before emitting anything.
- Output order is authored port order; each child has a unique deterministic ID,
  common parent/trace identity, and declared output type/state.
- Per-output propagation can preserve input content or use an authored
  projection without sharing mutable state between branches.
- Partial output admission is impossible; an invalid destination or capacity
  limit produces no children.
- UI handle projection and a browser interaction prove one connection per
  stable output port.
- Work admission, lineage, replay, editor, and functional branch tests pass.

### OPS-004: Skip, take, pick-first, and deduplicate scoped Work

As a Factory author, I can select Work according to prior items observed by one
node and scope.

Acceptance criteria:

- `SKIP`, `TAKE`, and `DISTINCT` implement the ordering and route semantics in
  this plan; `TAKE(1)` is documented and labeled as pick-first in the editor.
- Default scope is trace; parent, session, and selected-value scopes isolate
  state exactly as documented.
- Multiple scheduler eligibility checks do not increment state; only one
  accepted operation application advances the state version.
- Every non-selected item follows the configured skipped/overflow/duplicate
  route, and bounds cannot cause silent loss.
- Recorded operation state reconstructs counts, closed gates, and retained
  distinct keys on replay.
- Unit, repeated-run, concurrent-arrival, replay, UI, and root-built functional
  tests cover counts 0/1/N, independent scopes, duplicates, and overflow.

### OPS-005: Generate an exact bounded population

As a Factory author, I can use `GENERATE` to create exactly the configured
number of child Work items without asking a worker to emit a special JSON
batch.

Acceptance criteria:

- Count resolves from a literal, invocation argument, or structured selector
  and is validated against positive configured and system ceilings before any
  child is admitted.
- The template can set name, Work type/state, typed content, tags, and
  relationships from parent data, deterministic index, generation, and derived
  seed values.
- The entire generated batch is atomically validated/admitted through Work and
  carries parent, spawned-by, trace, request, and generation provenance.
- Replaying the same accepted facts reconstructs the same identities and order;
  retrying an applied idempotency key does not duplicate the population.
- Zero-count behavior is explicit (`ALLOW_EMPTY` or `REJECT`) and cannot leave a
  later all-children collector waiting on an unknown denominator.
- Editor template/count previews and package, runtime, Work admission,
  failure-path, and functional tests pass for 0, 1, typical N, and over-limit N.

### OPS-006: Collect and merge same-type Work

As a Factory author, I can wait for an exact count, quorum, all registered
children, or explicit close signal and consume/observe the stable collection in
one node.

Acceptance criteria:

- One collection input can bind several Work items of the same type/state; the
  author does not repeat identical inputs or refer to an internal arc.
- Group keys, denominator source, collection mode, consume/observe mode,
  timeout/overflow behavior, and late-arrival behavior are explicit.
- `ALL_REGISTERED` uses completed child registration from `GENERATE` or existing
  generated Work metadata, including zero-child completion.
- `EXACT(N)` never fires with fewer than N and deterministically selects the
  earliest N when more are ready; remaining Work follows declared late/overflow
  policy.
- `MERGE_CONTENT` emits canonical content as ordered JSON array, object-by-key,
  or text concatenation with explicit separator and maximum output bytes.
- Child failure can route an observed parent/group failure without waiting
  forever for successful members.
- Same-type cardinality, out-of-order completion, duplicate keys, zero children,
  failure, replay, editor, and functional tests pass.

### OPS-007: Maintain bounded aggregates and leaderboards

As a Factory author, I can use `REDUCE` to summarize a collection or maintain a
bounded count, numeric aggregate, grouped projection, latest value, or
deterministic top-K projection.

Acceptance criteria:

- Reducer kinds have discriminated typed configs and bounded retained state;
  arbitrary customer code is not accepted.
- Count, count-unique, sum, average, min, max, concatenate, and append modes
  define strict input types plus missing/null behavior; optional group-by emits
  stable groups under an explicit maximum group count.
- `top-K` requires key and numeric score selectors, stable score ordering, a
  documented tie-breaker, and an update policy for a repeated key.
- The node can emit on every accepted update or only on explicit close/window
  completion without mutating already emitted Work.
- Invalid scores, duplicate-policy violations, capacity exhaustion, and output
  size overflow take explicit failure routes.
- Operation/replay projections expose aggregate size, version, last update, and
  sanitized summary without leaking complete sensitive payloads.
- Unit property tests, concurrent update tests, replay tests, UI configuration,
  and a functional top-K scenario pass.

### OPS-008: Generate reproducible random values and samples

As a Factory author, I can create a seed/value, shuffle a collection, or sample
N members and reproduce the result from the same recorded seed.

Acceptance criteria:

- A supplied root seed is normalized and recorded; an omitted seed is generated
  once through an injected source and recorded before use.
- Substreams are isolated by workstation, scope, and firing ordinal, so adding a
  random node elsewhere does not perturb an existing node's choices.
- `RANDOM_VALUE` supports bounded integer, decimal, boolean, stable identifier,
  and choice-from-list outputs; every range rejects invalid or empty bounds.
- `SHUFFLE` and `SAMPLE` operate on a stable input collection, sample without
  replacement by default, and reject N larger than the population unless an
  explicit replacement policy allows it.
- Guard reevaluation consumes no random state; replay reads recorded choices and
  does not call the random source.
- Seed parity, substream isolation, distribution smoke (not brittle exact
  frequency), replay, editor, emulator, and functional tests pass.

### OPS-009: Apply wait, delay, debounce, throttle, and timeout with virtual time

As a Factory author, I can make routing depend on time without scripts, sleeps,
or cron-only topology.

Acceptance criteria:

- Durations and absolute deadlines resolve through the shared value-source
  contract and enforce valid minimum/maximum policy before scheduling.
- `WAIT` supports an interval or specified instant; `DELAY`, `DEBOUNCE`,
  `THROTTLE`, and `TIMEOUT` implement the selected, superseded/suppressed,
  completed, and timed-out routes documented above.
- Webhook/form/external-signal resume is rejected by this story rather than
  providing a process-local imitation of a durable suspended session.
- Timer identities and deadlines are stable per operation scope; cancel, pause,
  session close, and retry behavior are explicit and idempotent.
- Factory Runtime uses its injected clock/scheduler; package and functional
  tests advance fake time deterministically and contain no synchronization
  sleeps.
- Recorded timer facts and operation state rebuild pending/terminal timer
  projections; restart recovery does not fire the same accepted deadline twice.
- Timer count, buffered bytes, and open-scope limits apply backpressure with
  observable diagnostics.
- Editor duration/scope previews, race/repeated-run tests, replay tests, and
  root-built functional tests pass.

### OPS-010: Write artifacts and authorized files

As a Factory author, I can persist selected Work content as a session artifact
or an explicitly authorized file and route success/failure metadata.

Acceptance criteria:

- `WRITE_ARTIFACT` uses the Recordings artifact contract and returns a stable
  artifact reference that is visible in session history and invocation results.
- `WRITE_FILE` is limited to a declared Factory/session root, normalizes and
  validates resolved paths, rejects traversal and unsafe symlinks, and never
  accepts a sensitive value in a path.
- `FAIL`, atomic `REPLACE`, and serialized deterministic `APPEND` collision
  policies have distinct validated behavior; a failed write never reports
  success or a partial completed effect.
- An idempotency identity prevents duplicate writes after retry/recovery, and
  completed replay does not touch the filesystem.
- Size, file count, extension/content-type, and total artifact quotas are
  enforced before or during bounded streaming with explicit failure records.
- Filesystem effects are provided through an injected narrow edge and platform
  implementation; focused effect, security, failure-injection, replay, editor,
  and functional tests pass.

### OPS-011: Complete operation authoring, diagnostics, and observability

As a Factory author or operator, I can understand why an operation is waiting,
selected a route, reached a bound, scheduled a timer, or failed an effect.

Acceptance criteria:

- Definition preview reports resolved ports, required routes, value-source type,
  state scope, maximum fan-out/buffer/timers/effects, and side-effect capability.
- Runtime projections show kind, state version, open group/timer counts, last
  applied event, and stable failure code without exposing sensitive values.
- Service operations log accepted intent and terminal outcome with session,
  workstation, kind, scope digest, duration, and failure classification.
- Metrics cover applications, route counts, wait duration, buffers, generated
  Work, timer drift, bound failures, effect latency/failure, and replay
  divergence.
- Dashboard loading, empty, stale, validation-error, runtime-error, and success
  states are explicit and accessible on mobile and desktop.
- Focused component/browser tests prove a third-party graph interaction
  dispatches a feature operation and visibly updates the canonical projection.

### OPS-012: Bring the deterministic emulator to declared parity

As a package consumer, I can preflight and emulate the documented deterministic
operation subset without mistaking unsupported effects for hosted support.

Acceptance criteria:

- Compatibility inspection lists each supported/unsupported operation kind and
  exact config path before any events are written.
- The first parity tier supports pure routing, selection, tee, generate,
  collect/merge, aggregate, seeded random, and virtual-time operations.
- File effects remain unsupported or use a caller-supplied bounded sink; the
  emulator never writes host files implicitly.
- Frozen hosted-runtime references compare selected Work, operation routes,
  state versions, derived values, timer ticks, generated identities, lineage,
  and terminal projections at every logical tick.
- Same seed and command sequence are byte-stable; changed seed changes only
  random-derived decisions and their downstream effects.
- Package verify and installed-consumer checks pass.

### OPS-013: Split a structured collection into child Work

As a Factory author, I can select a bounded JSON array and split it into one
lineage-linked Work item per member.

Acceptance criteria:

- `SPLIT` requires one unambiguous array selector and validates the entire array,
  prospective Work count, projected content, output bytes, and destination
  before admitting any child.
- Each child receives deterministic index, parent, trace, spawned-by, and request
  identity; optional selected/all parent fields are copied through typed
  projections rather than mutable object aliasing.
- Empty arrays follow explicit allow-empty/done or reject behavior and register a
  known zero-child denominator for downstream collection.
- Non-array, sparse/invalid member, over-count, and over-byte inputs emit no
  partial batch and take a stable failure route.
- Editor selector/projection previews, Work admission/lineage tests, replay, and
  root-built functional tests cover empty, one, N, malformed, and over-limit
  arrays.

### OPS-014: Process a registered collection in bounded batches

As a Factory author, I can release a known collection in fixed-size batches,
loop each batch through downstream work, and take a done route only when all
members are accounted for.

Acceptance criteria:

- `BATCH` resolves a positive bounded batch size and operates over an explicit
  registered or closed collection; it never treats currently visible Work as an
  unknowable complete population.
- Every member is released exactly once per batch execution in stable order,
  unless an explicit reset starts a new version of the registered collection.
- The node exposes current batch index, member indices, remaining count, and
  no-items-left state as typed operation metadata without hidden mutable context.
- The `done` route fires only after every member succeeds or follows the
  configured member-failure policy; loop iteration and reset counts have hard
  ceilings that prevent infinite synchronous loops.
- Concurrency/resource limits can throttle downstream processing without
  changing batch membership or duplicating Work.
- Fake-worker functional tests cover uneven final batches, size 1/N, reset,
  member failure, cancellation, backpressure, replay, and loop exhaustion.

### OPS-015: Sort and limit a closed collection

As a Factory author, I can sort a bounded collection by typed keys and keep its
first or last N members deterministically.

Acceptance criteria:

- `SORT` supports ordered typed key clauses, ascending/descending direction,
  explicit null placement, strict type comparison, and stable Work-ID
  tie-breaking.
- `LIMIT` accepts first/last selection only after its input collection is
  explicitly closed; streaming first-N remains `TAKE`, and streaming last-N is
  rejected during validation.
- Non-selected members follow an explicit overflow route or are retained only
  when the author chooses observe mode; no item disappears implicitly.
- Random ordering is represented by seeded `SHUFFLE` and cannot be selected as
  an unrecorded sort mode.
- Unit/property tests cover numeric/string/date ordering, ties, nulls, N=0/1/
  greater-than-size, ascending/descending, replay, and UI configuration.

### OPS-016: Join bounded Work collections

As a Factory author, I can append, positionally zip, or key-join several bounded
collections with explicit clash and unmatched-item behavior.

Acceptance criteria:

- `JOIN` waits for every declared input collection to close and supports append,
  positional zip, and typed inner/left/right/full key joins.
- Multiple matches, unpaired members, shallow/deep field clashes, input
  precedence, missing/null keys, and maximum output count/bytes are explicit and
  validated.
- Append preserves authored input then member order; zip uses stable member
  position; key joins use strict typed equality and stable input/Work-ID order.
- SQL, fuzzy type coercion, and unbounded all-pairs combinations are not accepted
  by the built-in contract.
- Any prospective Cartesian/duplicate expansion is bounded before output; a
  violation emits no partial joined Work.
- Join-mode unit tests, same-key duplicate tests, out-of-order arrival/replay,
  editor projection, and root-built functional tests pass.

### OPS-017: Compare two datasets and route their differences

As a Factory author, I can compare two closed Work collections by typed keys and
route only-left, equal, changed, and only-right members separately.

Acceptance criteria:

- `DIFF` reuses the strict key and duplicate-match semantics of `JOIN` while
  separately configuring compared/ignored fields and changed-value output.
- Four stable named routes receive deterministic ordered results: `onlyLeft`,
  `equal`, `changed`, and `onlyRight`.
- Changed items can retain left, right, selected-field mix, or both versions
  through typed projections; there is no fuzzy coercion default.
- Duplicate keys, missing keys, ignored fields, nested objects, large datasets,
  output bounds, and failure routes have direct unit/integration evidence.
- Replay produces the recorded diff without recomputing it, and editor/browser
  coverage proves each stable port can be connected and saved.

### OPS-018: Transform dates and times explicitly

As a Factory author, I can parse, format, add/subtract, extract, round, compare,
or obtain the current time under an explicit timezone policy.

Acceptance criteria:

- `DATE_TIME` supports parse/format, add, subtract, extract part, round,
  difference, and current-time modes with typed input/output contracts.
- Ambiguous input formats and timezone-less values follow explicit reject or
  configured-timezone behavior; locale/timezone settings are recorded in the
  operation fact.
- Calendar arithmetic and duration arithmetic are distinct where daylight
  saving or month length can change the result.
- Current-time mode uses the injected clock and records the accepted instant;
  replay never reads the wall clock.
- Unit tests cover UTC, a daylight-saving transition, leap day, invalid dates,
  rounding boundaries, negative differences, fake-clock replay, and editor
  configuration.

### OPS-019: Prove the catalog with an evolutionary Factory

Publish an example or packaged Factory that optimizes a population for a bounded
number of rounds using graph operations rather than hidden runtime fixtures.

Acceptance criteria:

- Invocation accepts request, population size, rounds, elite count, and optional
  seed; all derived call and Work counts are rejected before execution if they
  exceed effective policy.
- `GENERATE` or `SPLIT` creates the initial population, workers produce
  schema-validated candidates/scores, `COLLECT` waits for the registered
  generation, `SORT`/`LIMIT` or `top-K` selects elites, and seeded `SAMPLE`
  forms deterministic parent pairs.
- Agent/model work performs domain-specific mutation/crossover; built-in
  operations own count, grouping, selection, time, and randomness rather than
  prompts pretending to be the scheduler.
- Each generation has explicit parent/generation lineage and cannot mix results
  from another trace or round.
- The loop ends after exactly the requested rounds and returns the best candidate
  plus score/provenance; failure, cancellation, invalid score, and budget
  exhaustion cannot return a partial success.
- Tests cover population 1, typical N, multiple rounds, fixed-seed repeatability,
  out-of-order evaluation completion, evaluator failure, and final replay.

### OPS-020: Prove the catalog with an autonomous leaderboard Factory

Publish an example or packaged Factory that runs a bounded autonomous agent
pool, evaluates branch submissions, maintains standings, and stops at a global
execution ceiling.

Acceptance criteria:

- Invocation accepts agent count, maximum total executions, per-agent iteration
  ceiling, evaluator settings, and optional seed; policy validates all derived
  concurrency and call bounds before dispatch.
- `GENERATE` creates stable competitor Work, existing worker/worktree templates
  give each competitor isolated branch identity, and each iteration receives
  only that competitor's notes/artifact history.
- `WRITE_ARTIFACT` or authorized `WRITE_FILE` maintains a bounded per-competitor
  notesheet; `TEE` sends a submission both to evaluation and the competitor's
  next-step path.
- Schema-validated evaluator output updates a deterministic `top-K` leaderboard
  by competitor key. Repeated submissions update the configured competitor
  record rather than creating ambiguous duplicates.
- Session-scoped `TAKE(maxTotalExecutions)` and per-competitor scoped `TAKE`
  gates enforce both ceilings under concurrent completions. Overflow routes end
  cleanly; no extra agent dispatch starts after a gate closes.
- Each iteration uses a new deterministic branch/worktree name when configured,
  preserves the prior branch reference in Work metadata, and never lets one
  competitor write another competitor's notes.
- The result includes standings, winning branch/artifact, execution counts, and
  evaluation provenance. Tests cover concurrency, ties, evaluator failure,
  notes isolation, global/per-agent ceiling races, cancellation, and replay.

### OPS-021: Harden file-watch Work ingress

As a Factory author, I can admit Work from authorized filesystem changes without
creating duplicate or unbounded Work when watchers restart or coalesce events.

Acceptance criteria:

- The existing filesystem-watcher Automation is represented as the explicit
  `FILE_WATCH` ingress kind; no second watcher implementation or mid-graph
  polling operation is introduced.
- Configuration declares authorized roots, include/exclude globs, supported
  create/modify/delete event kinds, debounce/coalescing behavior, maximum file
  size, content/materialization policy, and the target Work template.
- Paths are normalized under authorized roots and reject traversal, unsafe
  symlinks, special files, and sensitive values in emitted metadata.
- A durable cursor/checkpoint plus stable observation identity prevents
  duplicate Work admission across duplicate notifications and restart; rename,
  rapid-write, deleted-before-read, and watcher-overflow behavior is explicit.
- Backpressure, admission failure, malformed/oversized content, and dead-letter
  behavior produce observable Automation and Work outcomes without silently
  dropping events.
- Focused watcher, restart, deduplication, path-security, load, Work-admission,
  API/editor, and root-built functional tests pass.

### OPS-022: Add email-receive Work ingress

As a Factory author, I can turn received email into typed Work through one
bounded, checkpointed Automation contract.

Acceptance criteria:

- `EMAIL_RECEIVE` declares an account credential reference, mailbox/folder,
  supported provider filter, observation mode/interval, initial cursor policy,
  attachment policy, content limits, and target Work template.
- Credential values never enter Factory definitions, Work content, Factory
  Events, logs, diagnostics, or replay snapshots; only opaque references and
  sanitized account identity may be recorded.
- Provider message identity and a durable mailbox cursor prevent duplicate Work
  admission after retries/restarts while allowing an operator-visible replay or
  reprocess action to create an intentional new admission identity.
- The mapper exposes a documented typed envelope for selected headers,
  addresses, dates, text/HTML bodies, and quota-checked attachment references;
  missing multipart variants and malformed messages have explicit policy.
- Poll/push failures, authentication expiry, rate limiting, oversized messages,
  unsafe attachments, Work admission failure, and dead-letter outcomes are
  bounded and observable.
- Adapter-contract, cursor/deduplication, MIME/mapping, secret-redaction,
  restart, API/editor, and root-built functional tests pass without requiring a
  generic email/connector framework.

### OPS-023: Send email through an explicit effect node

As a Factory author, I can send a bounded email from Work and route a recorded
delivery result without invoking an arbitrary command or connector.

Acceptance criteria:

- `SEND_EMAIL` has a closed schema for credential reference, from/reply-to,
  recipients, subject, text/HTML body, approved attachment references, headers,
  timeout, and success/failure routes; typed selectors may supply values but
  cannot read prohibited sensitive fields.
- Definition validation enforces recipient, message, attachment, and header
  limits and rejects header injection, unsafe attachment paths, raw secret
  values, and unsupported provider options before execution.
- The effect uses a narrow injected mail-send port. It does not expose shell,
  arbitrary provider SDK calls, dynamic node installation, or a generic
  connector registry.
- The accepted intent is recorded before sending and terminal output contains a
  sanitized provider delivery identity/status. An explicit idempotency policy
  prevents blind resend after ambiguous completion; replay never sends mail.
- Retry is bounded and classification-aware; authentication, policy rejection,
  rate limiting, timeout, ambiguous result, and permanent delivery failure have
  stable failure codes and routes.
- Contract, validation, adapter, idempotency/failure-injection, replay,
  redaction, editor, and root-built functional tests pass.

### OPS-024: Perform an outbound HTTP request through an explicit effect node

As a Factory author, I can call an approved HTTP endpoint and map its bounded
response into Work without gaining a general network or command surface.

Acceptance criteria:

- `HTTP_REQUEST` has a closed schema for method, URL/value source, query,
  headers, bounded body, opaque credential reference, timeout, redirect policy,
  response-size limit, accepted status policy, response mapping, and
  success/failure routes.
- URL and network policy is evaluated for the initial request and every
  redirect/DNS resolution; it rejects disallowed schemes, user-info secrets,
  unauthorized hosts/ports, private/link-local/metadata destinations where not
  explicitly permitted, DNS rebinding, invalid TLS, and credential forwarding
  across origins.
- Sensitive request fields and response headers are redacted from events, logs,
  diagnostics, and Work unless an explicit safe mapping allows a non-secret
  value. Bodies and responses honor byte/content-type/materialization quotas.
- The accepted intent and idempotency identity are recorded before the request;
  replay never repeats network IO. Retries default to idempotent methods and
  require an adapter-supported idempotency contract for non-idempotent methods.
- DNS, connection, TLS, timeout, redirect, rate-limit, status, decode,
  oversize, cancellation, and ambiguous-result failures have stable
  classification and bounded behavior.
- Contract, validation, SSRF/redirect/DNS-rebinding, redaction,
  idempotency/failure-injection, replay, editor, and root-built functional tests
  pass. No inbound webhook or generic connector node is added.

### OPS-025: Publish migration guidance and complete delivery

Document operation authoring and migrate only selected maintained examples where
the new node is materially clearer than the specialized guard topology.

Acceptance criteria:

- `you docs` covers operation concepts, selector/predicate syntax, scope,
  ordering, bounds, time, randomness, effects, failure routes, and complete
  YAML examples.
- Documentation distinguishes `OPERATION` graph nodes, provider/model
  `operation`, JavaScript Factories, the two approved external Automation
  ingress kinds, the two approved network effects, and internal Petri
  machinery.
- The capability guide states the closed allowlist (`FILE_WATCH`,
  `EMAIL_RECEIVE`, `SEND_EMAIL`, and `HTTP_REQUEST`) and explicitly states that
  the reviewed n8n trigger/integration catalog is not a planned connector
  backlog.
- A migration table maps common old shapes to new nodes: worker-emitted static
  batch → `GENERATE`, all-children guard → `COLLECT`, logical branch copies →
  `TEE`, JSON-array prompt expansion → `SPLIT`, manual rate-limit loop →
  `BATCH`, repeated merge inputs → `JOIN`, and structured classifier prompt →
  `SWITCH` only when no inference is required.
- Existing examples are not mechanically rewritten; behavior/replay parity is
  proven before each intentional migration.
- API smoke, docs smoke, README checks when touched, packaged Factory generation
  and package verification, frontend verification, backend lint/tests, and the
  relevant functional/stress tiers pass.

## Dependency-Aware Delivery Sequence

1. OPS-001 establishes the public operation host, stable ports, event envelope,
   editor projection, and compatibility boundary.
2. OPS-002 establishes shared selectors/predicates and explicit conditional
   routing.
3. OPS-003 delivers stateless multi-route derivation.
4. OPS-004 establishes scoped state and replay, which later window and ceiling
   behavior reuse.
5. OPS-005 establishes prompt-independent atomic fan-out and child registration.
6. OPS-006 establishes same-type collection and fan-in.
7. OPS-007 establishes reusable bounded reducers.
8. OPS-008 adds deterministic random substreams over stable collections.
9. OPS-009 adds durable timer state after scoped state/replay is proven.
10. OPS-010 adds controlled effects after event/idempotency semantics are proven.
11. OPS-011 completes cross-catalog diagnostics and UI hardening.
12. OPS-012 adds emulator parity from frozen hosted-runtime behavior.
13. OPS-013 adds array-to-Work splitting on the fan-out/admission foundation.
14. OPS-014 adds bounded batch-loop behavior over registered collections.
15. OPS-015 adds deterministic closed-collection sort and limit behavior.
16. OPS-016 adds typed joins after collection closure and ordering are stable.
17. OPS-017 reuses join keys/comparison for four-route dataset differences.
18. OPS-018 adds explicit date/time transformations over the injected clock and
    shared typed projection contract.
19. OPS-019 and OPS-020 prove composition in the evolutionary and leaderboard
    Factories after their required collection operations are complete.
20. OPS-021 hardens the existing file watcher at the Automations/Work admission
    boundary and can proceed after shared Work admission limits are fixed.
21. OPS-022 adds the only new external ingress family, email receive, reusing
    the Automation cursor, deduplication, and Work admission behavior.
22. OPS-023 adds the allowlisted email-send effect after the OPS-010 effect
    envelope, idempotency, and replay rules are stable.
23. OPS-024 adds the allowlisted outbound HTTP effect on the same foundation,
    with its additional network-security policy proven independently.
24. OPS-025 publishes migration and allowlist guidance and completes the release
    surface.

OPS-003 can proceed alongside early pure-predicate work after OPS-001. OPS-013
can proceed after Work batch admission in OPS-005, while OPS-015 can proceed
after collection ordering is fixed in OPS-006. Timers, date/time helpers, and
filesystem effects should not block shipping the pure routing/fan-out/fan-in
catalog. OPS-021 and OPS-022 are an Automations/Work ingress lane; OPS-023 and
OPS-024 are an effect lane. Neither lane authorizes work on any other n8n
trigger, integration, or connector.

## Verification Strategy

### Unit and contract

- Selector/predicate parsing, type checking, missing/ambiguous values, and
  sensitive-value restrictions.
- Value source resolution, numeric/duration bounds, and default rules.
- Definition discrimination, port compatibility, illegal field combinations,
  synchronous-cycle limits, generated contract parity, and import/export.
- Pure operation functions for route, projection, merge, reducer, seed
  derivation, random sampling, and path policy.
- Closed-schema validation for file/email ingress, email send, and outbound HTTP
  requests, including credential-reference and destination policy.

### Package integration

- Compiler mapping from authored operations to runtime transitions.
- Atomic Work claims/admission, same-type cardinality, generated-child
  registration, lineage, and relationships.
- Scoped state under concurrent eligibility/application, cancellation, and
  replay.
- Timer scheduling/cancellation/firing with fake clocks.
- Artifact/file effect idempotency, partial failure, quotas, traversal, and
  symlink handling through explicit fakes.
- File-watch and email-receive cursor/deduplication, restart, bounded mapping,
  admission failure, backpressure, and secret-redaction behavior.
- Mail-send and outbound-HTTP adapter contracts, authorization, redaction,
  idempotency, ambiguous completion, retry classification, cancellation, and
  replay-without-IO behavior.
- Recording live/replay projection equivalence for every stateful/effect kind.

### Functional

Functional application tests construct through `root.BuildProcess`, execute
ordinary customer paths through `Process.Execute` and the CLI, and replace only
exact external effects through `edges.Edges`. They use injected clocks and
file/mail/HTTP edges instead of sleeps or custom in-process application graphs.

Required scenarios:

- structured switch with pass/default/failure;
- tee with atomic derived Work and lineage;
- skip/take under concurrent arrivals and independent scopes;
- exact generate/split/collect with zero, one, N, malformed structured input,
  out-of-order completion, and child failure;
- bounded batches with an uneven final batch, downstream backpressure, member
  failure, and done routing;
- closed-collection typed sort/first-last limit with ties and nulls;
- append/zip/inner/outer joins and four-route dataset diff with duplicate keys;
- top-K with ties and repeated competitor keys;
- seeded sample repeatability;
- date/time parsing/arithmetic across a daylight-saving boundary and replayed
  current time;
- delay/wait/debounce/throttle/timeout with deterministic clock advancement;
- artifact/file success, permission failure, retry, and replay without IO;
- file-watch duplicate/restart and email-receive cursor/restart admission;
- email-send success, rejection, ambiguous completion, and replay without send;
- outbound HTTP success, status/timeout/oversize failure, SSRF/redirect
  rejection, idempotent retry, and replay without request; and
- the evolutionary and leaderboard Factories.

### Stress and race

- thousands of scoped counters/groups with bounded memory;
- large allowed fan-out/fan-in and output-byte limits;
- concurrent group completion and global/per-key `TAKE` closure;
- timer cancellation/firing races and session shutdown;
- serialized append and idempotent effect retry;
- watcher/mailbox event bursts under Work-admission backpressure;
- concurrent mail/HTTP effect ceilings, cancellation, and ambiguous results;
- replay of long operation histories; and
- cancellation/backpressure while generated Work or timers are queued.

### Frontend and browser

- Pure feature-operation tests for add/remove/connect/disconnect/change-kind,
  route ordering, predicate editing, and save preparation.
- Projection tests that map canonical Factory operations and stable ports into
  React Flow nodes/handles.
- Mutation tests for validation failure, stale definition, retry, and generated
  contract handling.
- Focused component tests for palette, operation form, predicate builder, bound
  warnings, effect policy, and only the four approved external surfaces.
- Browser tests for add/connect/save/reload and a running operation's projected
  route/state, at mobile and desktop breakpoints with keyboard/a11y checks.

## Quality Gates

Use the narrowest focused package and UI commands per story, then broaden for
shared/public surfaces:

```sh
make generate-api
make interfaces-all             # when publishable schemas/clients change
make api-smoke
make packaged-factory-catalog-generate
make packaged-factory-catalog-check
make packaged-factory-package-verify
make docs-reference-smoke
make ui-test
make ui-lint
make verify-fast
make lint
make verify-pr                  # high-risk shared runtime/replay releases
```

Run focused Go package tests before broad targets. Run race/stress and repeated
execution for scheduler, timer, grouping, randomness, and effect stories. When
the editor changes, visually inspect the graph in a browser in addition to
automated tests.

## Project Acceptance Criteria

- A customer can author and run every operation in the catalog without using
  internal Petri vocabulary or a prompt-defined scheduling protocol.
- Skip/pick-first, exact generation, same-type collection, structured
  conditionals, seeded randomness, time windows, tee, and bounded file/artifact
  writes have explicit happy, false/overflow, failure, cancellation, and replay
  semantics.
- Split, batch, sort/limit, typed join/diff, aggregation, explicit failure, and
  date/time behavior cover the high-value deterministic n8n-style data-flow
  semantics without importing SQL, arbitrary expressions, a generic datastore,
  or the broader n8n integration catalog into Factory Runtime.
- External ingress/effects are limited to `FILE_WATCH`, `EMAIL_RECEIVE`,
  `SEND_EMAIL`, and outbound `HTTP_REQUEST`; their authorization, credentials,
  bounds, cursor/idempotency, redaction, failure, restart, and replay behavior is
  explicit and tested.
- No new RSS, SSE, webhook, form, chat, MCP, vendor, database, shell, Git, SSH,
  FTP, GraphQL, LDAP, sub-workflow, or generic connector/capability node is
  delivered or implied by this plan.
- Operation scope, ordering, bounds, timer behavior, random seed derivation, and
  effect policy are visible in authored config and runtime diagnostics.
- Work, Factory Events, Recordings, artifacts, and generated API contracts remain
  canonical; no operation-owned shadow state escapes as a second product model.
- Existing Factory definitions and JavaScript workflows remain compatible.
- The evolutionary and leaderboard Factories run within configured population,
  round, concurrency, execution, time, and artifact limits and reproduce their
  recorded decisions.
- Required contract, unit, integration, functional, stress/race, frontend,
  browser, package, docs, lint, and generated-artifact checks pass.
- Delivery for every implementation PR continues until required CI is terminal
  and passing, all blocking review feedback is explicitly addressed, conflicts
  and shared-file drift are resolved, and the PR is actually merged. An opened,
  approved, or green-but-unmerged PR is not complete.

## Delivery Loop

Each story should normally be one PR. For every PR, continue implementation and
review until required CI is terminal and green, blocking review conversations
are addressed, conflicts are resolved, generated artifacts are current, and the
PR is merged. Record follow-up work only when it is independently valuable and
does not weaken the current story's acceptance criteria.
