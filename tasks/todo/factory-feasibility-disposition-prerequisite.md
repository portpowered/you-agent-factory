# factory-feasibility-disposition-prerequisite-002 — Define the canonical incident disposition and idempotent action contract

**Parent behavior:** BEH-INCIDENT — escalate only actionable failed-work incidents and keep unchanged owned or named-hold evidence quiet across ticks, restart, and replay.

**Problem:** The Project reconciler can observe a failed Work but has no canonical handled-failure fact or stable action identity, so an unchanged owned correction can wake Astra or submit another action on every pass.

**Outcome:** A reviewer can determine whether one incident is handled or actionable before Astra or a recovery move, append one immutable disposition fact through the canonical Factory Event path, rebuild the latest disposition from replay, and fail closed on stale, active, malformed, duplicate, or partially observed actions without changing Work state semantics.

**Plan reference:** factory/docs/operating-policy.md — Cadence, liveness, and state; Failure classification and escalation; No-action and hold policy; Executable recovery boundaries. Source requirement: use a stable fingerprint, handled check, selective escalation, and safe orphan repair without a second queue or Work-state conflation.

**Actor and trigger:** Automations runs after the existing public Factory Session, Work, Worker Session, and Factory Event reads during a scheduled or exception-triggered reconciliation pass. Recordings owns canonical append and replay; Work and Factory Runtime own public Work mutation; the Portfolio Supervisor receives only actionable escalation evidence.

**Dependencies:** factory-feasibility-disposition-prerequisite-001.

**Parallel and shared-surface ownership:** T001 owns the preflight vocabulary and stable prerequisite identity consumed here. T002 owns the single Factory Event contract decision and the identity/action semantics. Recordings owns append, event validation, replay, and the latest-by-fingerprint projection; Automations owns classification and eligibility; Work/Factory Runtime owns Work admission and move controls. T003 integrates the complete M-01–M-18 matrix and rollout packet. No task may add a second incident store or edit the same future design path concurrently.

**Scope:**

- In: one additive INCIDENT_DISPOSITION_RECORDED Factory Event vocabulary and payload proposal; context requirements; stable incident/event/action identities; disposition transitions; replay reduction; handled checks; unowned, overdue, repeated-after-correction, and external-hold selection; concurrency and lost-response behavior; safe orphan predicates and public readback.
- Out: production event implementation, generated clients, a new incident endpoint, a WorkRead field, a new Work state, historical backfill, live event emission, active Work/settings changes, LocalAI changes, provider or model calls, and the complete integrated T003 matrix.

**Implementation constraints:**

- Factory Work and Factory Events remain the lifecycle authority. Incident disposition is metadata in an immutable event and replay projection, never a Work state, filesystem queue, or second persistence queue.
- Use the existing Recordings root Service.Append/canonical-ledger path and existing public Work request/move controls. Do not add a public root method, service locator, or endpoint in this design packet.
- Event readers must be deployed before any writer. Old recordings contain no incident events and replay to an empty disposition index. A rollback stops new incident emission and leaves all prior history readable.
- Reuse T001 normalization: trim values, lowercase ASCII identifiers, reject empty identity components, exclude sweep timestamps/ticks/attempt IDs, and reject the tuple delimiter from identity components. Causal evidence is a bounded normalized key, never raw provider output.
- The current LocalAI failure and corrective/recovery Work are read-only evidence. A deterministic failure, timer passage, an idle Worker, or a lost response is never a retry reason.
- Every action has one stable request identity. If public state cannot prove whether an action was accepted, return a structured partial-write blocker and do not submit a second request.

**Contract and configuration excerpts [Required when changed]:**

Authored source: api/components/schemas/events/FactoryEventType.yaml (x-enum-varnames, enum, x-enum-descriptions relevant tail)

Current:

~~~yaml
x-enum-varnames:
  - FactoryEventTypeJavaScriptPhaseChange
  - FactoryEventTypeArtifactCreated
enum:
  - JAVASCRIPT_PHASE_CHANGE
  - ARTIFACT_CREATED
x-enum-descriptions:
  - A JavaScript workflow phase transition was recorded without using Petri marking terminology.
  - A customer-visible factory artifact was created without exposing raw artifact bodies.
~~~

Proposed:

~~~yaml
x-enum-varnames:
  - FactoryEventTypeJavaScriptPhaseChange
  - FactoryEventTypeArtifactCreated
  - FactoryEventTypeIncidentDispositionRecorded
enum:
  - JAVASCRIPT_PHASE_CHANGE
  - ARTIFACT_CREATED
  - INCIDENT_DISPOSITION_RECORDED
x-enum-descriptions:
  - A JavaScript workflow phase transition was recorded without using Petri marking terminology.
  - A customer-visible factory artifact was created without exposing raw artifact bodies.
  - An incident disposition fact was recorded for replay and handled-failure suppression.
~~~

Authored source: api/components/schemas/events/FactoryEvent.yaml (properties.payload.oneOf tail and discriminator.mapping)

Current:

~~~yaml
      - $ref: ./payloads/JavaScriptPhaseChangeEventPayload.yaml
      - $ref: ./payloads/ArtifactCreatedEventPayload.yaml
discriminator:
  propertyName: type
  mapping:
    ARTIFACT_CREATED: ./payloads/ArtifactCreatedEventPayload.yaml
~~~

Proposed:

~~~yaml
      - $ref: ./payloads/JavaScriptPhaseChangeEventPayload.yaml
      - $ref: ./payloads/ArtifactCreatedEventPayload.yaml
      - $ref: ./payloads/IncidentDispositionRecordedEventPayload.yaml
discriminator:
  propertyName: type
  mapping:
    ARTIFACT_CREATED: ./payloads/ArtifactCreatedEventPayload.yaml
    INCIDENT_DISPOSITION_RECORDED: ./payloads/IncidentDispositionRecordedEventPayload.yaml
~~~

Authored source: api/components/schemas/events/payloads/IncidentDispositionRecordedEventPayload.yaml

Current:

~~~yaml
# Not present
~~~

Proposed:

~~~yaml
type: object
additionalProperties: false
description: >-
  One immutable incident-disposition fact on the canonical Factory Event
  stream. The event is replayed into the latest disposition projection;
  it is not a Work state and it is not a second queue.
required:
  - schemaVersion
  - fingerprint
  - workGeneration
  - failureCategory
  - causalEvidence
  - classification
  - disposition
  - owner
  - correctiveWorkId
  - nextCheckAt
  - deadlineAt
  - correctionRevision
  - verificationWitness
  - action
  - actionRequestId
  - holdCondition
  - evidence
properties:
  schemaVersion:
    type: string
    enum:
      - factory.incident-disposition.v1
    description: Version of the incident disposition payload.
  fingerprint:
    type: string
    pattern: '^[0-9a-f]{64}$'
    description: >-
      SHA-256 of the canonical session, Work, generation, failure-category,
      and causal-evidence tuple; sweep timestamps are excluded.
  workGeneration:
    type: string
    minLength: 1
    maxLength: 256
    description: >-
      Public Work generation token captured from currentChainingTraceId or
      another explicitly approved immutable lineage field.
  failureCategory:
    type: string
    minLength: 1
    maxLength: 128
    description: Normalized failure category such as contract_conflict.
  causalEvidence:
    type: string
    minLength: 1
    maxLength: 1024
    description: Bounded normalized causal key, not raw provider output.
  classification:
    type: string
    enum:
      - recoverable
      - permanent
      - scope_or_plan_failure
      - external_blocker
      - unknown
  disposition:
    type: string
    enum:
      - untriaged
      - correction-in-progress
      - externally-blocked
      - verified-resolved
  owner:
    type: string
    nullable: true
    maxLength: 256
  correctiveWorkId:
    type: string
    nullable: true
    maxLength: 256
  nextCheckAt:
    type: string
    format: date-time
    nullable: true
  deadlineAt:
    type: string
    format: date-time
    nullable: true
  correctionRevision:
    type: string
    nullable: true
    maxLength: 256
  verificationWitness:
    type: string
    nullable: true
    maxLength: 4096
  action:
    type: string
    enum:
      - none
      - discovery_prerequisite
      - escalation
      - orphan_repair
  actionRequestId:
    type: string
    nullable: true
    maxLength: 180
  holdCondition:
    type: string
    nullable: true
    maxLength: 1024
  evidence:
    type: array
    minItems: 1
    maxItems: 16
    items:
      type: object
      additionalProperties: false
      required:
        - kind
        - reference
        - summary
      properties:
        kind:
          type: string
          minLength: 1
          maxLength: 64
        reference:
          type: string
          minLength: 1
          maxLength: 512
        summary:
          type: string
          minLength: 1
          maxLength: 1024
~~~

Compatibility and consumers: the envelope, existing event values, context, and all existing payload variants remain unchanged. The proposed value is reader-before-writer additive. Generated outputs are future-only: api/openapi.yaml, pkg/transports/http/generated/server.gen.go, pkg/transports/http/client/client.gen.go, and ui/src/api/generated/openapi.ts. Consumers are Recordings append/replay and projection, Automations incident eligibility, the existing Factory Event SSE read, and future contract/replay/idempotence tests. This packet edits none of those sources or generated outputs.

## Canonical event context and payload semantics

The payload carries the incident fact; the existing Factory Event context supplies scope and correlation. A future writer must reject an event unless:

| Context field | Required value | Why it is authoritative |
| --- | --- | --- |
| sessionId | Non-empty selected Factory Session identity | Keeps fingerprints and replay projections session-scoped. |
| workIds | Exactly one affected Work ID for this event | The payload intentionally does not create a second Work identity field. |
| requestId | actionRequestId when action is not none; absent for a disposition-only fact | Ties external action idempotency to the canonical event. |
| currentChainingTraceId | Observed workGeneration when the public Work read supplies it | Prevents a correction or move from crossing a new lineage. |
| source | automations.project-reconcile or another approved Automations source | Makes ownership explicit without adding a public endpoint. |

The payload semantic validation is stricter than nullable JSON fields:

- schemaVersion is exactly factory.incident-disposition.v1; fingerprint is lowercase 64-character SHA-256; every bounded string is non-empty after normalization.
- correction-in-progress requires an owner, corrective Work or an explicit discovery prerequisite identity, a correction revision, and a future nextCheckAt. externally-blocked requires an owner, holdCondition, and a review/deadline time. verified-resolved requires verificationWitness and no new action. untriaged may be recorded before ownership is assigned.
- actionRequestId is non-null exactly when an action has been selected. An action-bearing fact is accepted only after the public action boundary proves acceptance; a disposition-only fact uses action none and a null request ID. The action fact is a claim about one stable request, not a new queue.
- evidence is bounded to 16 entries and contains references and sanitized summaries. It must not contain secrets, raw prompts, provider transcripts, or unbounded model output.
- A malformed payload or invalid context is rejected before canonical append, with no ledger/projection/Work mutation. A legacy or unknown event that a reader cannot interpret is preserved in history and never becomes an action signal.

## Stable identities

T002 consumes T001's normalization and never adds a sweep timestamp, tick, Worker Session attempt, or wall-clock value to an identity. The affected Work ID and Factory Session ID come from the event context; generation, category, and causal evidence come from the observed public failure.

The exact normalized incident tuple is:

~~~text
sessionId|workId|workGeneration|failureCategory|causalEvidence
~~~

Each component is trimmed, ASCII identifiers are lowercased, empty values are rejected, control characters and | are rejected, and the UTF-8 tuple is hashed with SHA-256:

~~~text
fingerprint = hex(sha256(normalizedIncidentTuple))
~~~

The action identity is revision-scoped. Use unrevised when no correction revision exists:

~~~text
actionSeed = factory.incident-action.v1|fingerprint|correctionRevision-or-unrevised|action
actionRequestId = incident-action/<action>/<hex(sha256(actionSeed))>
~~~

actionRequestId is at most 180 characters and is passed unchanged as the public Work Request requestId or existing Work move --request-id. The canonical event identity is independent of sweep time:

~~~text
eventSeed = factory.incident-event.v1|fingerprint|correctionRevision-or-unrevised|disposition|action|actionRequestId-or-none
eventId = incident-disposition/<hex(sha256(eventSeed))>
~~~

The same event ID with an equivalent payload is an idempotent replay; the same event ID with a different payload is a CONTRACT_CONFLICT and fails closed. Changing the Work generation, causal evidence, or correction revision creates new evidence/action identity while retaining every prior event and failed Work.

## Disposition and handled-check protocol

The latest projection is keyed by (sessionId, workId, fingerprint). It stores the latest valid disposition by canonical event sequence, the last action identity, and retained event IDs. It is an index over the Factory Event ledger, not an independent persistence system. A correction revision may replace the latest view for eligibility, but the prior verified-resolved or failed evidence remains in history.

Automations evaluates current public facts in this order:

1. Validate the selected session, Work generation, failure category, causal evidence, and latest event projection. Any missing or contradictory fact returns BLOCKED with a reason, owner, and observable exit; it does not submit a request.
2. Match the current fingerprint and generation to the latest projection. A correction-in-progress incident with the same evidence, an existing nonterminal corrective Work, and a not-yet-overdue check is HANDLED. Repeated ticks, restart, and replay return zero new disposition events, zero Astra requests, and zero failed-Work retries.
3. An externally-blocked incident with a named owner/hold and a future nextCheckAt or deadlineAt is HANDLED. A timer does not retry the failed Work. Once overdue, the evidence becomes eligible for one stable escalation.
4. Unowned/untriaged evidence, overdue ownership, or evidence that remains after a correction revision is ACTIONABLE. Select exactly one action and derive its revision-scoped actionRequestId; an unchanged repeat reuses the existing identity and is a no-op.
5. verified-resolved suppresses action only while the current public failure is absent. A failure after correction is a new eligibility decision: it records the new correction revision/action identity and never rewrites the resolved event.
6. unknown classification, an absent owner where a hold is required, a stale generation, an active execution, invalid Factory readiness, or an unreadable public fact is fail-closed. It produces a structured blocker or stable escalation only when the required escalation evidence itself is complete; it never guesses a retry.

Allowed disposition transitions are untriaged to correction-in-progress, externally-blocked, or verified-resolved; correction-in-progress to itself, externally-blocked, or verified-resolved; and externally-blocked to itself, correction-in-progress, or verified-resolved. A corrected repeat creates a new latest fact with the new correction revision while retaining the old transition history. No transition changes the Work's authored state.

## Append, replay, concurrency, and lost responses

The future implementation uses the existing pkg/services/recordings root Service.Append(AppendRecordedEventRequest) and the Recordings-owned canonical-ledger validation/idempotent append seam. The currently observed implementation already retains an event by ID; the implementation gate must also compare the retained payload and reject a same-ID/different-payload collision rather than silently treating it as success.

For a disposition-only transition, append the event with its deterministic eventId. For an action-bearing transition, use this order:

1. Reread public Work and Factory Event facts by actionRequestId and eventId. If an exact accepted action or exact disposition already exists, return ALREADY_RECORDED with no new request.
2. Submit the one existing public Work Request or Work move with the stable actionRequestId. The external boundary must treat equal request IDs as idempotent. No filesystem marker or in-memory lock is correctness state.
3. Reread public Work/Event state. If acceptance is visible, append the exact disposition fact through Recordings. The event records the handled/action identity; it does not create a second queue.
4. If an action response is lost but the public Work/Event read proves the same request was accepted, reuse the same identity and append/read the same event. If the read cannot prove accepted or rejected, return PARTIAL_WRITE_UNCERTAIN and submit nothing more.
5. If an event append response is lost, reread by eventId. An exact event is success; absence with a proven action may retry the same event ID through the idempotent append seam; a conflicting event or unreadable ledger is a structured CONTRACT_CONFLICT/partial-write blocker.

Concurrent ticks converge on one Work request ID and one event ID. The event ledger remains the only historical source, and the next pass can rebuild the projection after process restart. A duplicate request is not silently converted into a new correction or a retry.

## Safe orphan repair

orphan_repair is eligible only when every predicate is true:

- the selected Factory Session public lifecycle is RUNNING;
- the parent Work is the observed waiting/valid input state, its current generation equals the captured generation, and its authored transition to init is valid;
- no same-name cycle is visible, no active Worker Session owns the parent, and an active Worker Session with missing Work identity is treated as potentially owning it and therefore blocks the move;
- no matching action request or accepted WORK_STATE_CHANGE already exists;
- the public move control accepts the stable actionRequestId.

After the move, the implementation must read the canonical WORK_STATE_CHANGE event and the public Work read showing the target state plus next-dispatch reachability. Both are required before an orphan action is considered accepted. A failed Work is never moved to make the projection look healthy; existing failed/history facts remain visible. Stale generation, active execution, invalid readiness, duplicate, malformed metadata, and partial readback all return a structured no-action result.

## T002 controlled case matrix

The rows below are the T002 incident/action witness. T003 owns the final M-01–M-18 integrated matrix and may add cases, but it must preserve these identities and outcomes.

| Case | Given | When | Expected outcome and observer | Later gate |
| --- | --- | --- | --- | --- |
| M-04 | Same fingerprint has correction-in-progress, an owned nonterminal corrective Work, and a future check | A first unchanged eligibility pass runs | HANDLED; zero new incident events, Astra requests, or retries; latest projection and failed history remain visible in Factory Event/Work reads | GATE-IMPL-INCIDENT |
| M-05 | M-04 evidence is unchanged on a second tick | Eligibility runs again | Exact same no-op counts; no timestamp-derived identity and no new event | GATE-IMPL-INCIDENT |
| M-06 | M-04 projection is reconstructed after restart/replay | The next pass checks handled state | Replay yields the same latest disposition and zero new action/event; canonical history is unchanged | GATE-IMPL-ISOLATED |
| M-07 | Same failure is unowned/untriaged with complete causal evidence | Eligibility runs | One stable revision-scoped escalation request and one matching disposition event; repeat with same facts is a no-op | GATE-IMPL-INCIDENT |
| M-08 | An owned external hold has passed its deadline or next-check time | Eligibility runs | One stable escalation for that revision; no failed-Work retry; repeat is a no-op | GATE-IMPL-INCIDENT |
| M-09 | The same causal failure remains after the recorded correction revision | Eligibility runs | New revision-scoped escalation identity and one event/action; prior correction and failed history remain | GATE-IMPL-INCIDENT |
| M-10 | An external hold has a named owner and future deadline | Eligibility runs before the deadline | HANDLED; zero request/event/retry and hold remains visible | GATE-IMPL-INCIDENT |
| M-11 | Captured Work generation differs from the current public generation | Orphan/action selection runs | BLOCKED(stale-generation); zero move/event/request and Work is unchanged | GATE-IMPL-ORPHAN |
| M-12 | A Worker Session is active for the parent, including an active observation with no Work ID | Orphan/action selection runs | BLOCKED(active-execution); zero move/event/request | GATE-IMPL-ORPHAN |
| M-13 | Factory Session is not RUNNING or a required public read is invalid | Orphan/action selection runs | BLOCKED(invalid-readiness); no guessed mutation and all history preserved | GATE-IMPL-ORPHAN |
| M-14 | The same action request/event ID is already visible | Eligibility/action selection runs | ALREADY_RECORDED; zero new Work request, move, or disposition event; exact retained payload is checked | GATE-IMPL-INCIDENT |
| M-15 | An action or event response is lost | Next pass rereads public Work/Event facts | Accepted readback reuses the same identity; otherwise returns PARTIAL_WRITE_UNCERTAIN and submits no second request | GATE-IMPL-INCIDENT |
| M-16 | Metadata is missing, malformed, contradictory, or has an invalid fingerprint | Append or action selection runs | Validation rejects/fails closed with a structured reason; zero action and no Work/history deletion | CONTRACT-EVENT |
| M-17 | A waiting parent has no cycle, matching generation, no active Worker Session, valid readiness, and no duplicate | Public move runs | One stable move, one WORK_STATE_CHANGE, and public next-dispatch readback are required; failed Work remains failed | GATE-IMPL-ORPHAN |

## Acceptance criteria

- [x] Given unchanged owned correction-in-progress evidence across two ticks, restart, and replay, when handled checking runs, then no new Astra request, retry, or disposition event occurs and failed/history visibility remains.
- [x] Given unowned, overdue, or repeated-after-correction evidence, when eligibility runs, then one stable revision-scoped escalation is observable and an unchanged repeat is a no-op.
- [x] Given stale generation, active execution, invalid readiness, duplicate request, partial write, or malformed metadata, when action selection runs, then it fails closed with a structured reason and preserves Work/history.
- [x] Given valid orphan predicates, when the public move is accepted, then WORK_STATE_CHANGE and next-dispatch readback are required and failed Work remains failed.
- [x] Given an event append/action response is lost, when the next pass rereads public facts, then it reuses the same action identity or returns a partial-write blocker rather than creating a second request.

## Verification

- **Behavioral witness:** An independent reviewer walks M-04–M-17 with sanitized public Factory Session, Work, Worker Session, and Factory Event facts plus fake append/action outcomes. The record must include event/action counts, fingerprint, action/event IDs, projection after replay, public move readback, and preserved failed history.
- **Executable-spine effect:** extend.
- **Required evidence 1:** Scope functional; dependency fidelity controlled; procedure is a reviewer-reproducible tabletop over the matrix, exact payload validation rules, and explicit fake public reads and action outcomes. It proves disposition transitions, stable identities, handled suppression, selective escalation, concurrency convergence, and fail-closed branches. It does not prove compiled Recordings/Automations or real LocalAI behavior.
- **Required evidence 2:** Scope integration; dependency fidelity local_real; procedure is future YOU_TEST_BINARY followed by python -m unittest tests/factory/scripts/test_factory_recovery_integration.py using one private Factory Session and public Work/Event/replay assertions. It proves no duplicate public request/event, failed-history preservation, valid orphan readback, and unrelated Work isolation through production wiring. The test must not build the binary and cannot prove remote/paid provider behavior or rollout.
- **Highest feasible level:** Controlled functional design review now. The current changed-path lease forbids production representation and live mutation; compiled public integration is a future implementation gate.
- **Remaining unproven edges:** event/runtime implementation -> GATE-IMPL-INCIDENT; generated contract alignment -> CONTRACT-EVENT; public replay/isolation -> GATE-IMPL-ISOLATED; orphan runtime behavior -> GATE-IMPL-ORPHAN; LocalAI artifact acceptance -> LA-05/LA-06; canary safety -> GATE-CANARY.
- **Test-layer design when tests change:** Pure normalization, validation, eligibility, and fake-runner policy cases belong in tests/factory/scripts/test_reconcile_projects.py without a Factory root, sleeps, source-scanning meta-tests, or in-test builds. Public Work/Event, replay, idempotence, and orphan readback belong in test_factory_recovery_integration.py and consume a build/release-owned prebuilt YOU_TEST_BINARY; each integration case owns its profile, port, session, Work IDs, and cleanup.

**Paid validation, when applicable:**

- Trigger: None; no provider/model validation call is authorized.
- Maximum calls: 0.
- Maximum cost: USD 0.
- Maximum duration: 0 paid execution minutes.
- Fixture and output validator: Sanitized public Work/Event fixtures and controlled append/action outcomes only.
- Evidence-reuse key: FACTORY-FEASIBILITY-01/incident-v1.

**Operational and rollout notes:** This packet changes no live Factory settings, active Work, event stream, generated output, or public endpoint. A future implementation ships readers/projection and contract consumers before writers, validates same-ID payload equality, observes one session at a time, and stops on any contract conflict or partial-write uncertainty. Rollback disables new incident writers, preserves historical disposition events and failed Work, and leaves the existing reconciler's public read/move behavior unchanged until a reviewed implementation cutover.

**Escalation:** Stop with a structured blocker if the existing Recordings append/replay seam cannot provide reader-before-writer validation and same-identity conflict detection, if Work admission/move cannot accept the stable request ID, if public readback cannot distinguish accepted from uncertain, or if an incident metadata field requires a new Work state or endpoint. The blocker must name the failed criterion, exact evidence, customer/system impact, safe design work completed, owner, observable exit, and smallest contract or implementation delta. Do not add a second store or silently weaken the no-retry rule.

**Handoff artifacts:** This packet's additive event contract proposal, canonical fingerprint/event/action formulas, disposition/handled protocol, replay and lost-response rules, orphan predicates, M-04–M-17 witness rows, future test-layer placement, and GATE-IMPL-INCIDENT, GATE-IMPL-ORPHAN, CONTRACT-EVENT, and GATE-IMPL-ISOLATED inputs. T003 owns the complete matrix, clean-room procedure, and controlled rollout packet; T004 owns final delivery and review handoff.
