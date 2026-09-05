# factory-feasibility-disposition-prerequisite-004 — Deliver the reviewed design through CI, review, merge, and clean-room handoff

**Parent behavior:** BEH-DESIGN-DELIVERY — deliver one reviewed design
prerequisite with explicit CI/review/merge ownership while keeping runtime and
rollout as future gates.

**Problem:** T003 has the complete feasibility, incident, replay, orphan, and
rollout evidence, but the one-document delivery path, exact implementation
finish line, clean-room handoff, and review-owned merge boundary still need one
self-contained task packet.

**Outcome:** The final task packet gives an independent reviewer one immutable
design head to inspect, preserves the exact Current/Proposed contract and
M-01–M-18 matrix, routes VAL-001 through its required read-only template, and
stops the implementation workstation after the final head is pushed, the PR is
open, required CI has started, and blocking feedback is addressed. Review owns
terminal CI, conflicts, and merge; the Supervisor owns any later canary.

**Plan reference:** `factory/docs/operating-policy.md` — Validation missions;
Learning and controlled change; No-action and hold policy; Executable recovery
boundaries; requirement: reviewed merge precedes controlled rollout.

**Actor and trigger:** The planning/review owner receives dependency-ready
T001–T003 evidence and prepares the final handoff. Validation receives only the
immutable final head, acceptance rubric, public entry points, sanitized fixture,
and budget. Review owns the PR's terminal checks, conflict resolution, and
merge; the Portfolio Supervisor owns post-merge canary admission and promotion.

**Dependencies:**

- `factory-feasibility-disposition-prerequisite-001` — feasibility vocabulary,
  LocalAI contradiction, stable prerequisite identity, and M-01–M-03/M-18.
- `factory-feasibility-disposition-prerequisite-002` — incident event,
  fingerprint/action identities, handled protocol, replay, lost-response, and
  safe-orphan semantics.
- `factory-feasibility-disposition-prerequisite-003` — complete M-01–M-18
  matrix, tabletop, clean-room procedure, path leases, and rollout safeguards.

**Parallel and shared-surface ownership:** T004 owns the final task packet and
its delivery handoff. No task may concurrently author the future
`docs/internal/development/plans/factory-feasibility-disposition-correction.md`.
The future implementation owner owns production/API/generated/test changes;
Recordings owns event append/replay; Automations owns classification and
eligibility; Work/Factory Runtime owns public Work admission/move controls;
build/release owns the immutable prebuilt `YOU_TEST_BINARY`; validation owns
VAL-001; review owns terminal CI, conflicts, and merge; the Portfolio
Supervisor owns post-merge canary admission and promotion.

**Scope:**

- In: the complete M-01–M-18 given/when/then matrix; the additive
  reconciliation diagnostic result; a controlled tabletop; the exact future
  unit, contract, functional, integration, and clean-room proof placement;
  one future design-document lease; PR/review/merge ownership; exact owners,
  immutable artifact leases, commands, unproven edges, one-session canary,
  observation, hold, rollback, and promotion conditions; security, privacy,
  performance, accessibility, localization, and cost boundaries.
- Out: production implementation, generated files, executable test changes,
  live event emission, live Factory settings or active Work mutation, LocalAI
  or provider/model calls, canary execution, VAL-001 execution, terminal CI
  waiting, merge, and silent loopback repair.

**Implementation constraints:**

- Factory Work and Factory Events remain the lifecycle authority. Incident
  disposition is event metadata plus a replay-built projection, never a Work
  state, filesystem queue, or second store.
- Preserve T001's ten-minute no-build/no-broad-test preflight and T002's
  reader-before-writer, same-ID payload equality, stable identity, and
  fail-closed partial-write rules.
- Preserve the customer-facing vocabulary `Factory`, `Factory Session`,
  `Work`, `Worker Session`, and `Factory Event`; do not expose Petri-net
  implementation terminology in the future public design.
- Routine design and implementation remain autonomous. Do not invent an
  operator-approval Work. Escalate only a real authority, immutable-contract,
  safety, budget, or data-integrity decision.
- The implementation-stage delivery criterion is exact: stop after the final
  head is pushed, the PR is open, CI has started, and blocking review feedback
  is addressed; do not poll or re-check CI after that finish line. Review owns
  terminal-and-passing CI, conflicts, and merge.
- This iteration writes only the two leased task artifacts plus the untracked
  planner scaffolding `prd.json` and `progress.txt`; it does not create the
  future design document, commit CI/audit evidence, or edit any excluded path.

## Integrated contract boundary

The only new shape retained in this T004 handoff is T003's future additive
diagnostic result from the existing project reconciler. T004 adds no runtime or
public contract change. The diagnostic is not a second lifecycle store and
does not make incident metadata a Work state.

Authored source: `factory/scripts/reconcile-projects.py` — `reconcile()` return
value and `main()` JSON stdout.

Current:

```json
{"sessionId":"session-id","server":"http://127.0.0.1:7437","status":"completed","moved":[],"skipped":[]}
```

The JSON above is one successful `completed` example only. The complete
current native result contract is:

| Variant | Required top-level shape and behavior |
| --- | --- |
| Running, normal | `sessionId` (trimmed non-empty string), `server` (trimmed non-empty string), `status:"completed"`, `moved: MovedRecord[]`, and `skipped: SkippedRecord[]`. |
| Running, `--dry-run` | The same fields, with `status:"dry-run"`; `moved` reports the same candidate details but no public move is issued. |
| Not running | `sessionId`, `server`, `status:"skipped"`, `reason:"factory-not-running"`, `moved:[]`, and `skipped:[]`; no Work list is inspected. |

The current record shapes are exact: `MovedRecord` requires
`name:string`, `workId:string`, `reason:"missing-cycle"`, and
`observationRevision:string` containing 20 lowercase hexadecimal characters;
it may include `unfinishedChildren:string[]` only when a child is
nonterminal. `SkippedRecord` requires `reason` from
`project-without-name`, `ambiguous-project-name`, `blocked-inspect-only`,
`project-without-work-id`, `cycle-in-progress`,
`cycle-transition-pending`, or `project-lead-active`; it may include
`name:string`, which is absent for `project-without-name`.

For a `ReconcileError` from input validation, a public read/move failure,
invalid JSON, or an invalid response, stdout is empty, stderr is exactly
`project reconciliation failed: <message>` plus a newline, and the process
exits `1`. Argument-parser rejection remains stderr-only with exit `2`.

Proposed:

```json
{"sessionId":"session-id","server":"http://127.0.0.1:7437","status":"completed","moved":[],"skipped":[],"incidents":[{"schemaVersion":"factory.incident-disposition.v1","fingerprint":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","workId":"work-id","workGeneration":"trace-id","failureCategory":"contract_conflict","causalEvidence":"omni-text-usage-to-work-artifact-gap","classification":"scope_or_plan_failure","disposition":"correction-in-progress","owner":"localai-project-lead","correctiveWorkId":"corrective-work-id","nextCheckAt":"2026-09-05T11:02:28Z","deadlineAt":"2026-09-05T11:02:28Z","correctionRevision":"localai-project-v1","verificationWitness":null,"action":"none","actionRequestId":null,"holdCondition":null,"evidence":[{"kind":"factory-event","reference":"dispatch-completed/dispatch-id","summary":"bounded failure feedback identifies the contract conflict"}]}],"escalations":[]}
```

The proposed response is the same complete union plus two additive arrays on
every successful JSON result:

| Variant | Additive behavior |
| --- | --- |
| Running, normal or `--dry-run` | Preserve every current top-level field and record shape, then add `incidents: IncidentDiagnostic[]` and `escalations: EscalationDiagnostic[]`. A dry run reports bounded classifications or would-be stable actions but performs no Work Request, Factory Event append, or move. |
| Not running | Preserve the exact current skipped response and add `incidents:[]` and `escalations:[]`; no incident source is inspected. |
| Read/validation/process error | Preserve the current stderr, stdout, and exit behavior. No guessed diagnostic, Work Request, retry, Factory Event, or move is emitted. |

`IncidentDiagnostic` has the following complete bounded shape:

| Field | Type/validation |
| --- | --- |
| `schemaVersion` | Required literal `factory.incident-disposition.v1`. |
| `fingerprint` | Required 64-character lowercase hexadecimal SHA-256 over `sessionId\|workId\|workGeneration\|failureCategory\|causalEvidence`; sweep timestamps, ticks, and Worker Session attempts are excluded. |
| `workId`, `workGeneration` | Required non-empty bounded Work identity and public generation token. |
| `failureCategory` | Required normalized token, maximum 128 characters. |
| `causalEvidence` | Required bounded normalized reference, maximum 1024 characters; raw provider output is excluded. |
| `classification` | Required enum: `recoverable`, `permanent`, `scope_or_plan_failure`, `external_blocker`, `unknown`. |
| `disposition` | Required enum: `untriaged`, `correction-in-progress`, `externally-blocked`, `verified-resolved`. |
| `owner`, `correctiveWorkId` | Required nullable strings, maximum 256 characters each. |
| `nextCheckAt`, `deadlineAt` | Required nullable RFC 3339 date-times. |
| `correctionRevision` | Required nullable normalized string, maximum 256 characters. |
| `verificationWitness` | Required nullable bounded string, maximum 4096 characters. |
| `action` | Required enum: `none`, `discovery_prerequisite`, `escalation`, `orphan_repair`. |
| `actionRequestId` | Required nullable stable action identity, maximum 180 characters. |
| `holdCondition` | Required nullable bounded string, maximum 1024 characters. |
| `evidence` | Required array of 1–16 objects; each object has non-empty `kind` (max 64), `reference` (max 512), and `summary` (max 1024). |

`EscalationDiagnostic` requires `fingerprint`, the matching stable
`actionRequestId`, `dispositionEventId` in the form
`incident-disposition/<sha256>`, `reason` from `unowned`, `overdue`, or
`repeated-after-correction`, and `action:"escalation"`; it may include a
nullable normalized `correctionRevision`.

The arrays are views of canonical Factory Event facts and accepted public
actions, not a second queue. A malformed or contradictory incident fails
closed before action selection. A lost action/event response is reread by
stable identity: accepted readback is reported once, while uncertain readback
is a structured blocker and creates no further action. Automations owns
classification, Recordings owns append/replay/latest projection, Work and
Factory Runtime own public actions, and the script only renders bounded
diagnostics. Readers validate the complete union before a writer emits it;
same-ID payload conflicts remain failures.

Compatibility, migration, and rollback are explicit: all existing fields,
record reasons, flags, public reads, move behavior, stderr, and exit behavior
remain unchanged; old output remains valid; no output, Work, recording, or
Factory Event is backfilled or rewritten; readers must be ready before a new
writer; and disabling the new diagnostic writer returns to the current
read/move output while preserving failed Work, Events, and replay inputs.
No generated output is refreshed in this design packet. A future
implementation must author the event schema and its readers before writers,
then regenerate the declared clients and run the contract gates.

Future consumers are `factory/workstations/project-reconcile`, Portfolio
Supervisor diagnostics, the focused `tests/factory/scripts/test_reconcile_projects.py`
policy lane, and the small public integration witness. A malformed or
contradictory incident is rejected or reported as a structured blocker; it
never deletes Work, hides failed history, or triggers a retry.

## Integrated state, identity, and ownership rules

The following rules are inherited from T001/T002 and are part of the review
contract, not an implementation claim:

| Concern | Canonical rule | Owner and later gate |
| --- | --- | --- |
| Feasibility | Map criterion to capability/file-symbol, permitted change, semantic dependency, owner, and witness within ten minutes; unknown or contradictory input returns `DISCOVERY_REQUIRED`, `CONTRADICTION`, or `BLOCKED`. | Future admission owner; `GATE-IMPL-PREFLIGHT` |
| Incident identity | Normalize trimmed values, lowercase ASCII identifiers, reject empty/control-character/pipe values, and hash `sessionId\|workId\|workGeneration\|failureCategory\|causalEvidence`; exclude sweep timestamps, ticks, and Worker Session attempts. | Automations with Recordings projection; `GATE-IMPL-INCIDENT` |
| Action identity | `factory.incident-action.v1\|fingerprint\|correctionRevision-or-unrevised\|action` → `incident-action/<sha256>`; the same accepted request is reused. | Work/Factory Runtime; `GATE-IMPL-INCIDENT` |
| Event identity | `factory.incident-event.v1\|fingerprint\|correctionRevision-or-unrevised\|disposition\|action\|actionRequestId-or-none` → `incident-disposition/<sha256>`; same ID requires equivalent payload. | Recordings; `CONTRACT-EVENT` |
| Handled evidence | Owned correction-in-progress with nonterminal corrective Work and a future check; named external hold before its deadline; verified-resolved with no current failure; or an already recorded identical action/event. | Automations; `GATE-IMPL-INCIDENT` |
| Actionable evidence | Complete unowned/untriaged failure; overdue owner/hold; same causal failure after a new correction revision; or all valid orphan predicates. | Automations and Supervisor; `GATE-IMPL-INCIDENT` / `GATE-IMPL-ORPHAN` |
| Fail-closed evidence | Stale generation, active execution, invalid readiness, malformed/contradictory metadata, same-ID payload conflict, or uncertain public readback produces a structured no-action/blocker. | Work/Factory Runtime and Recordings; `CONTRACT-EVENT` / `GATE-IMPL-ORPHAN` |
| Canonical state | Retain every Factory Event and failed Work; derive only the latest-by-fingerprint projection for eligibility. No filesystem marker or second queue is correctness state. | Recordings; `GATE-IMPL-ISOLATED` |

The future reconciler checks the latest handled projection before Astra or a
recovery move. It may emit one stable action/event for actionable evidence,
then rereads public Work/Event facts. If a response is lost, accepted public
state reuses the same identity; uncertain acceptance yields
`PARTIAL_WRITE_UNCERTAIN` and submits nothing more.

## M-01–M-18 complete case matrix

Every row has a deterministic result, public or controlled observer, explicit
request/event/move expectation, preserved-state rule, and owning later gate.
The rows intentionally combine the T001 feasibility cases, the T002 incident
cases, and the routine-autonomy boundary; they do not claim that runtime code
already exists.

| Case | Given | When | Then: result and expected counts | Observer and preserved state | Later gate |
| --- | --- | --- | --- | --- | --- |
| M-01 | A required capability has no bounded public operation or directly cited symbol. | The ten-minute preflight runs. | `DISCOVERY_REQUIRED`; 0 Work Requests, 0 retries, 0 events, 0 moves. Return missing capability, owner, dependency, witness, smallest exit, and stable prerequisite ID. | Controlled result fields and public request count; all existing Work/history remain unchanged. | `GATE-IMPL-PREFLIGHT` |
| M-02 | LA-06 requires `InferenceOutput.Artifact`, while `InvocationProtocolResponse` exposes only `Text`/`Usage`; `localai-project-v1` is immutable for this slice. | The preflight evaluates the criterion. | `CONTRADICTION`; 0 duplicate Work Requests, 0 retries, 0 events, 0 moves. Name the four cited boundaries and allowed discovery/design exit. | Exact LocalAI event/Worker Session/Work lineage; failed and corrective/recovery Work remain visible. | `GATE-IMPL-PREFLIGHT` + `LA-05/LA-06` |
| M-03 | A routine criterion is covered by existing public Work, Factory Event, and Worker Session reads and has a permitted design change. | The preflight evaluates the bounded slice. | `ADMIT_BOUNDED_SLICE`; 0 approval requests, 0 retries, 0 incident events, 0 moves. Emit capability/witness map. | Reviewer sees capability, permitted change, dependency, owner, and witness; no invented `HUMAN_APPROVAL`. | `GATE-IMPL-PREFLIGHT` |
| M-04 | A fingerprint is correction-in-progress with an owned nonterminal corrective Work and future check. | The first unchanged eligibility pass runs. | `HANDLED`; 0 Astra requests, 0 retries, 0 new events, 0 moves. | Latest projection and failed Work read remain visible. | `GATE-IMPL-INCIDENT` |
| M-05 | M-04 evidence is unchanged on a second tick. | Eligibility runs again. | Exact M-04 no-op counts; no timestamp-derived identity or new event. | Fingerprint, action/event counts, and historical failure are unchanged. | `GATE-IMPL-INCIDENT` |
| M-06 | M-04's projection is reconstructed after restart/replay. | The next pass checks handled state. | Same latest projection; 0 new Astra requests, retries, events, or moves. | Public event replay and Work history equal the pre-replay facts. | `GATE-IMPL-ISOLATED` |
| M-07 | Complete unowned/untriaged failure evidence has a valid causal witness. | Eligibility runs. | One stable revision-scoped Astra/escalation request and 1 matching disposition event; unchanged repeat is 0/0/0/0. | Public Work request and Factory Event IDs; failed Work and prior evidence remain. | `GATE-IMPL-INCIDENT` |
| M-08 | A named external hold is overdue. | Eligibility runs. | One stable escalation request and 1 event; no failed-Work retry or move; repeat is a no-op. | Escalation/hold deadline, event, and Work history remain visible. | `GATE-IMPL-INCIDENT` |
| M-09 | The same causal failure remains after a recorded correction revision. | Eligibility runs. | One new revision-scoped escalation request and 1 event; prior correction/history are retained. | Distinct revision-scoped IDs and complete event/Work history. | `GATE-IMPL-INCIDENT` |
| M-10 | A named external hold has a future deadline. | Eligibility runs before its deadline. | `HANDLED`; 0 requests, 0 retries, 0 events, 0 moves. | Latest disposition and hold remain visible. | `GATE-IMPL-INCIDENT` |
| M-11 | Captured Work generation differs from current public generation. | Orphan/action selection runs. | `BLOCKED(stale-generation)`; 0 moves, 0 events, 0 requests, 0 retries. | Current Work generation and failed history are unchanged. | `GATE-IMPL-ORPHAN` |
| M-12 | An active Worker Session exists, including an active observation without Work identity. | Orphan/action selection runs. | `BLOCKED(active-execution)`; 0 moves, 0 events, 0 requests, 0 retries. | Worker Session read and Work state show no mutation. | `GATE-IMPL-ORPHAN` |
| M-13 | Factory Session is not `RUNNING` or a required public read is invalid. | Orphan/action selection runs. | `BLOCKED(invalid-readiness)`; 0 moves, 0 events, 0 requests, 0 retries. | Session/read error is retained; no guessed mutation or history loss. | `GATE-IMPL-ORPHAN` |
| M-14 | The same action request or event ID is already visible with an equivalent payload. | Eligibility/action selection runs. | `ALREADY_RECORDED`; 0 new requests, 0 new events, 0 retries, 0 moves. | Public IDs and exact payload equality are checked; retained event/history stay intact. | `GATE-IMPL-INCIDENT` + `CONTRACT-EVENT` |
| M-15 | An action or event response is lost. | The next pass rereads public facts. | Accepted readback reuses the same identity with 0 duplicate requests/events; otherwise `PARTIAL_WRITE_UNCERTAIN` with 0 further actions. | Public Work/Event readback and structured blocker; no deletion or retry. | `GATE-IMPL-INCIDENT` + `GATE-IMPL-ISOLATED` |
| M-16 | Metadata is missing, malformed, contradictory, or fingerprint-invalid. | Append or action selection runs. | Validation rejects/fails closed; 0 requests, 0 events, 0 retries, 0 moves. | Structured validation reason; Work, failed history, and unrelated active Work remain. | `CONTRACT-EVENT` |
| M-17 | Waiting parent has matching generation, no same-name cycle, no active Worker Session, valid readiness, and no duplicate. | Public move runs. | Exactly 1 stable move, 1 `WORK_STATE_CHANGE`, and 1 successful next-dispatch readback; no failed-Work retry. | Public Work target/dispatch readback; failed Work and unrelated Work remain visible. | `GATE-IMPL-ORPHAN` |
| M-18 | Routine design has no authority, budget, safety, or immutable-contract decision. | Supervisor/lead submits through ordinary delivery/review. | Continue autonomously; 0 invented approval requests; escalate only on a real authority/contract/safety/budget decision. | Review path and request count show normal autonomy; no live Factory mutation. | `REVIEW-CI-MERGE` + `VAL-001` |

### Tabletop procedure and expected evidence

The reviewer uses one sanitized fixture identity and copies only scalar facts
into controlled case inputs. The LocalAI fixture is
Factory Session `5cd877a9-2674-451d-bcc8-c3bda4d3f9c0`, event
`factory-event/dispatch-completed/96b67964-3721-4d05-afd0-611f4a4f6f8b`
(sequence 70), and the preserved failed/corrective/recovery Work lineage from
T001. The fixture is read-only evidence; the tabletop submits no Work Request,
move, event append, provider call, or model call.

The controlled run is:

1. Record the fixture identity, source revision, and baseline counts for
   requests, moves, events, retries, failed Work, handled visibility, and
   unrelated active Work.
2. Walk M-01–M-03 and M-18 with a monotonic ten-minute preflight timer. Stop
   at ten minutes; timeout is `BLOCKED`, never permission to continue.
3. Walk M-04–M-10 with a deterministic clock: first pass, unchanged second
   tick, restart/replay, overdue/future hold, and correction revision. Record
   the same fingerprint/action/event IDs and exact no-op counts.
4. Walk M-11–M-17 with copied public Work, Session, Worker Session, and event
   facts. Run stale, active, invalid, duplicate, lost-response, malformed,
   and valid-orphan branches. Use controlled fake append/action/readback
   outcomes only at the external-effect boundary.
5. Compare every result with the matrix. Any mismatch is a finding, not a
   repair. Record the smallest owner/gate delta and leave the source fixture
   and active Factory untouched.

The evidence record must include the matrix row, input fixture identity, exact
request/event/move/no-action counts, fingerprints and stable IDs where
applicable, projection before/after replay, public readback outcome, preserved
failed/history facts, unrelated Work result, and later gate. It proves design
completeness and deterministic policy; it does not prove production event
append, compiled reconciler behavior, real LocalAI inference, terminal CI,
merge, or rollout.

## Clean-room validation and loopback

VAL-001 is a fresh, read-only engineering validation against the final design
head. It uses `factory/docs/standards/validation-loopback-template.md` exactly
and writes its report only to the Project validation path owned by the
validation Work. It receives the final artifact/commit identity, acceptance
rubric, public entry points, sanitized fixture identity, and budget; it does
not receive a claimed-fix narrative or another validation report.

The validator must:

1. Verify the final packet has the exact Current/Proposed diagnostic contract,
   all M-01–M-18 rows, source-plan references, owners, path leases, commands,
   unproven edges, and no-live-change boundary.
2. Reproduce the controlled tabletop from the matrix without writing to the
   Factory, making provider/model calls, building a binary, or changing
   settings/Work.
3. Fill the template's Environment and artifact, Project criteria, Customer
   journey, Cross-task integration and usability, Findings, and Verdict
   sections. Every criterion is classified independently; no criterion is
   silently omitted because a later runtime gate is unavailable.
4. Emit `PASS` only for the design property actually observed. Emit
   `BLOCKED` for delivery/runtime edges that this design packet cannot prove,
   with the owner, exact uncertainty, smallest delta, and retest scope in the
   required Delta-plan request. A `FAIL` or `BLOCKED` result never edits the
   packet or live Factory.

Expected classification at this task's fidelity is:

| Criterion | Expected clean-room classification | Evidence or required delta |
| --- | --- | --- |
| AC-01 | `PASS` for the design prerequisite; runtime admission remains unproven. | M-02, LocalAI symbol map, stable identity, no-retry/no-duplicate fields; later `GATE-IMPL-PREFLIGHT` and `LA-05/LA-06`. |
| AC-02 | `PASS` for the feasible tabletop; execution remains unproven. | M-03/M-18 capability/witness map and no-approval rule; later `GATE-IMPL-PREFLIGHT`. |
| AC-03 | `PASS` for matrix semantics; public replay/escalation behavior remains unproven. | M-04–M-17 counts, identities, handled projection, and fail-closed rules; later `GATE-IMPL-INCIDENT`, `GATE-IMPL-ISOLATED`, and `GATE-IMPL-ORPHAN`. |
| AC-04 | `PASS` when every contract, matrix row, owner, exit, and blocker format is present. | This packet's contract, matrix, structured escalation, and source-plan trace; later `VAL-001` confirms independently. |
| AC-05 | `PASS` for the future delivery/rollout design only. | Path lease, build artifact owner, one-session window, holds, rollback, and Supervisor promotion boundary; later `GATE-CANARY`. |
| AC-06 | `BLOCKED` until review owns terminal CI/conflicts/merge; this packet must not claim it. | Review owner must open/update the PR, start required CI, resolve blocking feedback, and later merge; future runtime/canary gates remain explicit. |

The clean-room report's final Verdict is therefore not allowed to promote this
design to runtime acceptance. If a design defect is found, the report uses the
template's Delta-plan request with: affected criterion, exact reproduction,
expected/actual result, root-cause evidence, smallest correction, owner,
dependencies, and retest scope.

## Future verification layers and exact commands

No tests change in T004. The following placement is the future implementation
contract and avoids meta-tests that only inspect source topology:

| Layer | Future proof and observer | Boundary and ownership | Exact command/gate |
| --- | --- | --- | --- |
| Unit/policy | M-01–M-03, M-08, M-10, M-14, M-16, M-18 normalization, eligibility, and fail-closed outcomes; observe returned decision/counts/reasons. | `tests/factory/scripts/test_reconcile_projects.py`; no root process, sleep, binary, provider, or source-scanning test; controlled clock and fakes at external boundaries. | `python -m unittest tests/factory/scripts/test_reconcile_projects.py`; `POLICY-UNIT` |
| Contract | T002 event enum, payload, discriminator, field limits, same-ID equality, and additive diagnostic stdout; observe authored schema validation and generated representations. | Authored `api/components/` first; Recordings/event consumers own implementation; generated Go/TypeScript clients are build outputs, never hand-edited. | `make api-smoke`; `CONTRACT-EVENT` and scoped interface generation in the implementation lane. |
| Functional | Public Work Request, handled suppression, escalation, replay projection, and orphan readback; observe Factory Session Work/Event/Worker Session reads. | Shared `root.BuildProcess` per package; each parallel scenario owns a Factory Session, identity, temp workspace, and controlled `edges.Edges`; no CLI binary build/invocation. | `make test` or the focused public Factory Session lane; `GATE-IMPL-INCIDENT` / `GATE-IMPL-ORPHAN`. |
| Integration | M-04–M-07 and M-09–M-17 through the compiled server, including public replay/idempotence and unrelated Work preservation. | `tests/factory/scripts/test_factory_recovery_integration.py`; build/release supplies immutable `YOU_TEST_BINARY`; each case owns profile, port, session, Work IDs, and cleanup; the test never builds. | `YOU_TEST_BINARY=<immutable-path> python -m unittest tests/factory/scripts/test_factory_recovery_integration.py`; `GATE-IMPL-ISOLATED`. |
| Clean-room/end-to-end design | Matrix, source-plan trace, criterion table, usability, and future-gate handoff; observe the required validation report. | Fresh read-only validator and `validation-loopback-template.md`; no repair. | `VAL-001` using the exact template. |
| Load/stress | Not applicable to this bounded design packet; no throughput or latency claim is made. | A future dedicated load lane owns capacity evidence if the implementation changes event volume. | `GATE-LOAD` only if a later plan adds a measured requirement. |

The functional tests run in parallel by Factory Session. The integration cell is
small and consumes one already compiled artifact; no test may compile it.
Contract and runtime checks must be added only after the authored event source
exists. These future commands are not current test evidence.

## Security, privacy, accessibility, localization, performance, and cost

- Incident evidence stores bounded normalized keys, references, and summaries;
  it never copies raw provider output, prompts, credentials, tokens, or
  transcript bodies. Existing Factory Session/Work/Event authorization remains
  the access boundary.
- Stable IDs and same-payload conflict checks protect against duplicate
  external effects and ambiguous writes. A malformed or unauthorized read is
  fail-closed, not a guessed mutation.
- This packet has no UI, browser, focus, responsive, or localized product-copy
  change. The future diagnostic JSON remains machine-readable and existing
  customer terminology; any future UI must use the repository website and
  accessibility standards rather than adding a hidden incident surface.
- The future projection is bounded by retained canonical events and must not
  turn the fifteen-minute reconciler into a broad event scan without a measured
  need. The implementation must report event/read/action counts and any
  readback latency at the canary boundary; no performance threshold is invented
  by this design packet.
- Paid validation is prohibited: 0 provider/model calls, USD 0, 0 download
  bytes. Current planning uses at most the two read-only inspection processes,
  256 MiB temporary disk, and no live mutation. Future integration uses the
  prebuilt artifact and controlled external effects only.

## Artifact ownership, path lease, and rollout handoff

Current writes are limited to:

- `tasks/todo/factory-feasibility-disposition-prerequisite.md`;
- `tasks/todo/factory-feasibility-disposition-prerequisite.json`; and
- untracked planner scaffolding `prd.json` and `progress.txt`.

The future implementation/design lease is exactly one file:
`docs/internal/development/plans/factory-feasibility-disposition-correction.md`.
Its single author owns the integrated Current/Proposed blocks and must not
edit `factory/factory.json`, `factory/scripts/**`, `factory/workstations/**`,
`pkg/**`, `api/**`, `ui/**`, `tests/**`, or
`docs/internal/development/plans/localai-extensions-factory-artifact-delta.md`
under this packet. The operating policy and LocalAI failure/event fixture are
upstream immutable inputs; the LocalAI Project lead owns any amendment to that
contract. Build/release owns the digest and provenance of `YOU_TEST_BINARY`.

After authored implementation and focused/public tests have passed, review and
merge have completed, and the Supervisor has authorized promotion, the rollout
is:

1. Select exactly one private or otherwise explicitly bounded Factory Session
   with no unrelated active Work. Record the session, implementation commit,
   generated-contract revision, fixture identity, and configuration hash.
2. Observe one complete fifteen-minute reconciler interval in that session plus
   the immediate public replay/readback check. Do not change live Factory
   settings or broaden the session during the window.
3. Require zero duplicate action/event IDs, zero retries for M-04–M-06/M-10,
   exactly one stable escalation for each actionable M-07–M-09 fixture, no
   unrelated Work mutation, preserved failed/history visibility, and no
   contract-conflict or partial-write uncertainty.
4. Hold promotion immediately on a duplicate or retry, same-ID payload
   conflict, uncertain readback, stale/active orphan mutation, lost history,
   invalid generated contract, unexpected permission/data exposure, canary
   session failure, or any budget/capacity signal outside the declared bound.
5. Roll back by disabling the new incident writer at its canonical implementation
   seam and returning to the existing reconciler read/move behavior. Preserve
   all prior Factory Events, failed Work, and replay projection inputs; do not
   delete, rewrite, or retry the affected Work. Record the hold reason and
   owner, then require a reviewed correction before re-enable.
6. Only the Portfolio Supervisor promotes beyond the one-session window. This
   task/workstation does not execute or approve that rollout.

## T004 delivery, review, and clean-room handoff

The future authored design is exactly one file:
`docs/internal/development/plans/factory-feasibility-disposition-correction.md`.
It is not created by this task. The future implementation/design owner must
carry T001–T003's Current/Proposed blocks, event/readers-before-writers order,
M-01–M-18 matrix, path lease, and rollout boundaries into that file before any
production change is admitted. No second design copy, filesystem incident
queue, or live Factory setting is introduced.

### Implementation-stage delivery criterion

The following criterion is preserved verbatim from `prd.json`:

> Implementation-stage delivery criterion: The implementation stage marks this criterion satisfied and stops after its final head is pushed, the PR is open, CI has started, and all blocking review feedback is addressed. It does not poll or re-check CI after this finish line. The review stage owns driving CI to terminal-and-passing, resolving merge conflicts, and merging the PR; merge remains the lane-wide delivery boundary. CI-run evidence goes in a PR comment and never in a commit.

This criterion distinguishes this workstation's ready-for-review finish line
from the lane-wide merge boundary. A PR being open or CI being started is not
runtime acceptance, a clean-room PASS, or canary authorization.

### Bounded PR body projection

The full `prd.json` is the immutable handoff input, but it is larger than
GitHub's 65,536-byte PR-body limit. Delivery therefore computes a deterministic
bounded projection immediately before the GitHub command. It reads the exact
UTF-8 source bytes, records the source SHA-256 and byte count in the PR body,
copies the ordered keys `project`, `branchName`, `description`, `sourcePlan`,
`problem`, `solution`, `acceptanceCriteria`, `behaviorLanes`, and `userStories`,
serializes with sorted keys and compact separators, asserts the encoded body is
less than 65,536 bytes, writes only the untracked temporary `.prd-body.json`,
and deletes that file after the GitHub command. The full source is never
committed or passed directly as the body file.

The exact projection command is:

```powershell
python -c "import hashlib,json; p='prd.json'; raw=open(p,'rb').read(); doc=json.loads(raw); keys=('project','branchName','description','sourcePlan','problem','solution','acceptanceCriteria','behaviorLanes','userStories'); body={'source':{'file':p,'sha256':hashlib.sha256(raw).hexdigest(),'bytes':len(raw),'note':'Full prd.json remains the immutable handoff input; this is its deterministic bounded projection.'}}; body.update({key:doc[key] for key in keys}); encoded=json.dumps(body,ensure_ascii=False,sort_keys=True,separators=(',',':')); assert len(encoded.encode('utf-8')) < 65536; open('.prd-body.json','w',encoding='utf-8',newline='').write(encoded)"
gh pr create --base main --head factory-feasibility-disposition-prerequisite --title factory-feasibility-disposition-prerequisite --body-file .prd-body.json
# If the named PR already exists, use: gh pr edit 2540 --body-file .prd-body.json
Remove-Item -LiteralPath .prd-body.json
```

The PR description produced by this procedure must expose the computed
`source.file`, `source.sha256`, and `source.bytes` values. The source hash and
byte count are recomputed after any planner-scaffolding update; no stale
measurement is reused.

### Exact delivery sequence and ownership

1. Run the packet-only checks: `python -m json.tool` on the task JSON and
   `prd.json`; `git diff --check`; and a field-by-field Markdown/JSON parity
   review covering story ID, contract excerpts, matrix IDs, evidence, owners,
   budgets, leases, and gates.
2. Review the complete diff and ensure only the two leased task artifacts are
   tracked. Keep `prd.json` and `progress.txt` untracked; never add them to the
   PR. Commit the task artifact change with a focused message.
3. Push the final head to the named branch and open/update the PR with title
   `factory-feasibility-disposition-prerequisite` and the guarded `.prd-body.json`
   projection as the PR description. The PR must target `main`, expose the
   final commit identity, and retain the full `prd.json` only as untracked
   handoff input.
4. Confirm required CI has started on that exact head and address any known
   blocking review feedback or reported conflict. If feedback reveals a
   contract/plan contradiction, stop and return a structured blocker with the
   failed criterion, reproduction, impact, safe work, owner, observable exit,
   dependencies, and smallest delta.
5. Once the head is pushed, the PR is open, CI has started, and blocking
   feedback is addressed, stop this implementation stage. Do not wait for
   terminal CI, merge, VAL-001, runtime tests, LocalAI, or canary behavior.

Review then owns terminal required CI, conflict resolution, and merge. CI-run
evidence is recorded in a PR comment, never in a commit. Validation owns the
fresh VAL-001 report and must run it from the final design head through
`factory/docs/standards/validation-loopback-template.md`; a FAIL or BLOCKED
result requests a delta plan and never edits the design or live Factory.

### Handoff inputs and finish records

The handoff passes these immutable inputs to review and validation:

- final task-artifact commit and PR head;
- `prd.json` acceptance rubric and source-plan references;
- the exact diagnostic Current/Proposed excerpts;
- M-01–M-18 and the sanitized LocalAI fixture identity;
- future implementation path lease and excluded paths;
- future command matrix, artifact owner, budgets, unproven edges, and
  one-session canary hold/rollback conditions.

The implementation stage records only the delivery facts needed for review in
the untracked planner log: final head, PR URL/number, CI-start observation,
and any addressed feedback. It does not commit CI results, audit notes, or a
validation report. Review records terminal CI and merge in the PR conversation;
the Supervisor records any later canary decision through its owned workflow.

## Acceptance criteria

- [x] Given the integrated design, an independent reviewer can find the exact
  diagnostic Current/Proposed contract, M-01–M-18 matrix, path lease, owners,
  commands, and unproven edges, with no live Factory change claimed.
- [x] Given a blocking review finding or conflict, the handoff requires a
  reviewed design/PR correction or a structured blocker with an owner and
  observable exit; it never hides the finding or repairs live state.
- [x] Given the final merged design, the Supervisor handoff keeps preflight,
  incident/replay, LocalAI artifact, isolated public behavior, and canary as
  explicit future gates; the proposal is not runtime accepted.
- [x] Given the final design head, VAL-001 has the exact read-only template,
  all criterion classifications, and a required delta plan for every
  `FAIL`/`BLOCKED` result; it never silently repairs.
- [x] The exact implementation-stage delivery criterion is preserved verbatim:
  final head pushed, PR open, CI started, blocking feedback addressed, then
  implementation stops while review owns terminal CI, conflicts, and merge.

## Verification

- **Behavioral witness:** Independent review and fresh VAL-001 receive the
  final design head, reproduce the sanitized M-01–M-18 tabletop and LocalAI
  contradiction, verify all six project-criterion mappings, and distinguish
  design PASS from future runtime/delivery gates.
- **Executable-spine effect:** `promote`.
- **Required evidence 1:** Scope `end-to-end`; dependency fidelity `controlled`.
  Procedure: fresh read-only VAL-001 follows
  `factory/docs/standards/validation-loopback-template.md`, the clean-room
  steps above, and the integrated matrix. Artifact: final design head and
  validation report. Proves independent reviewability, criterion
  classification, source-plan trace, and safe handoff. Does not prove runtime
  event append/replay, real LocalAI, terminal CI, merge, or canary.
- **Required evidence 2:** Scope `functional`; dependency fidelity `none`.
  Procedure: `python -m json.tool` on the task JSON and `prd.json`,
  `git diff --check`, field-by-field Markdown/JSON parity review, and a
  tracked-path check before commit. Proves both packet representations are
  parseable, clean, aligned, and scoped. Does not prove semantic runtime
  behavior.
- **Required evidence 3:** Scope `end-to-end`; dependency fidelity `none`.
  Procedure: push the final task-artifact head, generate the guarded bounded
  projection of `prd.json` with source SHA-256 and byte count, open/update the
  named PR with `.prd-body.json`, delete the temporary body, and confirm
  required CI has started on that exact head; record only the PR/head/CI-start
  facts in untracked progress.
  Proves implementation-stage delivery readiness. Does not prove terminal CI,
  merge, VAL-001 outcome, runtime behavior, LocalAI, or rollout.
- **Highest feasible level:** Independent clean-room design handoff plus the
  implementation-stage PR/CI-start finish line. Runtime E2E is not applicable
  because the path lease forbids production representation/code, executable
  tests, and live Factory mutation.
- **Remaining unproven edges:** preflight runtime → `GATE-IMPL-PREFLIGHT`;
  incident/event runtime → `GATE-IMPL-INCIDENT`; authored/generated contract
  alignment → `CONTRACT-EVENT`; public replay/isolation →
  `GATE-IMPL-ISOLATED`; orphan runtime → `GATE-IMPL-ORPHAN`; LocalAI artifact
  acceptance → `LA-05/LA-06`; clean-room execution → `VAL-001`; terminal
  CI/conflicts/merge → `REVIEW-CI-MERGE`; canary → `GATE-CANARY`.
- **Test-layer design when tests change:** No tests change in T004. Future pure
  policy cases use the isolated script unit lane; public behavior uses parallel
  Factory Sessions and one reusable `root.BuildProcess`; compiled integration
  uses the build/release-owned prebuilt `YOU_TEST_BINARY`; load/stress remains
  dedicated. No source-scanning meta-tests or in-test builds are allowed.

**Paid validation, when applicable:**

- Trigger: None; the packet authorizes no provider/model validation.
- Maximum calls: 0.
- Maximum cost: USD 0.
- Maximum duration: 0 paid execution minutes.
- Fixture and output validator: sanitized public facts, controlled tabletop,
  and the validation-loopback template.
- Evidence-reuse key: `FACTORY-FEASIBILITY-01/matrix-v1`.

**Operational and rollout notes:** This packet changes no production/API/
generated/test/live Factory surface. It preserves T001–T003 failure history and
hands future readers, writers, tests, clean-room validation, and canary
promotion to named owners. The implementation stage stops at pushed-head,
open-PR, started-CI, addressed-feedback readiness; review owns terminal CI,
conflicts, and merge; a failed or blocked validation is a structured delta,
not an implementation or runtime acceptance claim.

**Escalation:** Stop with a structured blocker if any M-01–M-18 row lacks a
deterministic observer/outcome, if the diagnostic/event contract cannot remain
additive, if a future artifact owner or immutable input is missing, if the
prebuilt artifact cannot be identified, or if the one-session canary cannot
have a measurable hold/rollback condition. The blocker must name the failed
criterion, exact evidence, system impact, safe work completed, owner,
observable exit, and smallest plan/contract delta. Do not broaden scope or
repair the live Factory.

**Handoff artifacts:** Integrated Current/Proposed reconciliation result;
M-01–M-18 matrix; controlled tabletop procedure and fixture identity; VAL-001
clean-room procedure and criterion rubric; unit/contract/functional/
integration layer placement; exact one-document path lease and artifact
ownership; PR title/body/CI-start delivery procedure; one-session canary
observation, hold, rollback, and Supervisor-promotion gate; and inputs for
`VAL-001`, `REVIEW-CI-MERGE`, `GATE-IMPL-ISOLATED`, and `GATE-CANARY`.
