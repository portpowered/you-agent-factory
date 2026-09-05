# factory-feasibility-disposition-prerequisite-001 — Establish a ten-minute feasibility preflight with a concrete contradiction witness

**Parent behavior:** BEH-ADMISSION — admit only capability-supported slices and
turn unknown or contradictory prerequisites into one bounded discovery/design
prerequisite.

**Problem:** Supervisor and Project Lead slicing has no short, explicit map from
a criterion to the capability, permitted change, dependency, owner, and witness
that would prove it. The known LocalAI artifact mismatch can therefore consume a
full dispatch before it is reported.

**Outcome:** A reviewer has one deterministic, no-build preflight procedure that
classifies feasible, unknown, and contradictory criteria, derives a stable
discovery-prerequisite identity, detects an equivalent active Work before any
submission, and records the exact LocalAI contradiction without retrying or
duplicating corrective Work.

**Plan reference:** `factory/docs/operating-policy.md` — Project contract and
autonomy boundaries; Work shaping and throughput; Failure classification and
escalation. Source requirement: unknown capability yields one short
discovery/design prerequisite with an explicit exit condition before
implementation planning.

**Actor and trigger:** The Portfolio Supervisor or Project Lead runs this check
before admitting an implementation slice or allowing a planner to begin a
broad investigation.

**Dependencies:** None.

**Parallel and shared-surface ownership:** T001 establishes the preflight
vocabulary and identity rule. T002 consumes it and must not define a conflicting
capability or prerequisite identity. Luna owns this design surface. The LocalAI
lead owns the immutable LocalAI evidence and LA-05/LA-06 only. No production,
API, generated, test, Factory-definition, or active-Work file is edited by this
task.

**Scope:**

- In: the ten-minute procedure; criterion/capability/allowed-change/
  dependency/owner/witness mapping; exact Models and Workers symbols; feasible,
  unknown, contradictory, and no-approval outcomes; active Work and stable
  request-identity checks; the current LocalAI witness.
- Out: production admission code, LocalAI changes, active Work mutation,
  incident-event implementation, executable test changes, generated artifacts,
  live settings, and operator approval for routine design.

**Implementation constraints:**

- Use customer-facing `Factory`, `Factory Session`, `Work`, and `Worker Session`
  vocabulary. Keep Petri-net and other implementation mechanics private.
- Read only the selected public Factory Session, Work, Worker Session, and
  Factory Event surfaces plus the directly cited source symbols. Do not build,
  download, call a provider/model, run broad tests, or perform an architecture
  survey during the preflight.
- Factory Work and Factory Events remain authoritative. `docs/temp` or another
  filesystem is not an identity store or queue.
- The current LocalAI Project contract, failed lineage, corrective lineage, and
  recovery Work are immutable read-only inputs. A known contradiction is not a
  retry signal.
- Routine design proceeds through ordinary delivery/review. Do not invent a
  `HUMAN_APPROVAL` or operator-approval Work unless an actual authority,
  immutable-contract, safety, or budget decision is required.

**Contract and configuration excerpts [Required when changed]:**

Authored source: `# None — T001 changes no public contract or configuration.`

Current:

```text
# Not present: the Factory has no authored bounded-feasibility result shape.
```

Proposed:

```text
# Design-only handoff shape below; no API, generated file, or runtime writer is
# authorized by T001. Authoring of any future contract belongs to the later
# implementation/design lane.
```

Generated outputs and consumers: None for this task. The future implementation
must use the authored API component source before regenerating any public
client, and T002 owns the separate incident-event contract decision.

## Preflight design contract

The preflight is a bounded decision procedure, not a second admission queue.
It reads one criterion and only the evidence directly needed to classify that
criterion. Its input map is:

| Input | Required content | Evidence boundary |
| --- | --- | --- |
| `project`, `criterionId`, `contractRevision` | Immutable Project and criterion identity | Project request/acceptance and supplied task input |
| `requiredCapability` | The observable capability the criterion needs | One named public operation or file/symbol |
| `permittedChange` | The smallest change authorized for this slice | Immutable scope and source-plan boundary |
| `dependencies` | Real semantic prerequisites, not preferred ordering | Current public Work/Factory Event/Worker Session facts or source-plan requirement |
| `owner` | One accountable owner for the next decision | Project/Factory role ownership |
| `witness` | A public read or controlled observation that can prove the capability | Named command, route, symbol, or later gate |
| `candidatePrerequisiteId` | Stable identity derived before any submit | Canonical identity algorithm below |

The design-only result vocabulary is:

| Decision | When it is returned | Required action and state |
| --- | --- | --- |
| `ADMIT_BOUNDED_SLICE` | Every required capability has direct evidence, the change is permitted, dependencies have owners, and a witness is named | Continue ordinary bounded design/delivery; no operator-approval gate |
| `DISCOVERY_REQUIRED` | A required capability is unknown or cannot be mapped within the timebox | Return one prerequisite with owner, dependency, witness, smallest exit condition, and stable identity; no implementation admission or retry |
| `CONTRADICTION` | A capability is visible in a shape that cannot satisfy the criterion under the permitted change/immutable contract | Name both shapes and the allowed-change boundary; preserve existing Work/history and submit nothing |
| `ALREADY_ADMITTED` | A public Work/Event read finds the same stable prerequisite or corrective identity already accepted and nonterminal | Do not submit another Work Request; do not retry the failed Work |
| `BLOCKED` | A required public read or identity cannot be trusted, or the timebox expires | Return a structured blocker with owner and observable exit; perform no guessed mutation |

The future implementation may serialize this shape internally, but T001 does
not make it a public API:

```json
{
  "schema": "factory.feasibility.preflight.v1",
  "project": "localai",
  "criterionId": "LA-06",
  "contractRevision": "localai-project-v1",
  "decision": "CONTRADICTION",
  "capability": {
    "name": "artifact-bearing generic inference output",
    "symbol": "models.InferenceOutput.Artifact",
    "evidence": "pkg/services/workers/internal/services/runners/internal/inference/generic_invocation.go:269-273"
  },
  "permittedChange": "one discovery/design prerequisite; no LocalAI Project or active Work mutation",
  "dependency": {
    "owner": "LocalAI Project lead",
    "exit": "reviewed artifact-bearing OMNI propagation contract and LA-05/LA-06 witness"
  },
  "owner": "Luna",
  "witness": "Factory Event dispatch-completed feedback plus the cited Models/Workers symbols",
  "candidatePrerequisiteId": "discovery-prerequisite/localai/la-06/localai-project-v1/96452494c59c43816af0dd5190602e9876157a2be06e8c09f01e2efd4a23fb1e",
  "existingWorkIds": [
    "batch-localai-project-cycle-003-authority-retry-e10e3884-localai-omni-artifact-contract-delta-authority-retry",
    "batch-localai-project-cycle-001-e10e3884-localai-recover-real-inference"
  ],
  "workRequestSubmitted": false,
  "retryRequested": false,
  "operatorApprovalRequired": false
}
```

## Ten-minute procedure

Measure from the first criterion read with a monotonic stopwatch. The hard stop
is ten minutes; a timeout is a `BLOCKED` result, not permission to continue
investigating.

| Elapsed time | Bounded action | Stop condition |
| --- | --- | --- |
| 0:00–1:00 | Read the criterion, Project, contract revision, and permitted scope | Missing immutable identity returns `BLOCKED` |
| 1:00–3:00 | Inspect the one directly named public capability or file/symbol | Missing capability is recorded as `DISCOVERY_REQUIRED` |
| 3:00–5:00 | Map the allowed change, real dependency, accountable owner, and witness | An immutable-contract or authority conflict returns `CONTRADICTION` |
| 5:00–6:30 | Normalize the identity tuple and compute the candidate prerequisite request ID | Invalid identity input returns `BLOCKED` |
| 6:30–8:00 | Read public Work/Event state for an equivalent accepted/nonterminal prerequisite or corrective Work | Match returns `ALREADY_ADMITTED`; failed history remains evidence only |
| 8:00–9:00 | Select `ADMIT_BOUNDED_SLICE`, `DISCOVERY_REQUIRED`, `CONTRADICTION`, `ALREADY_ADMITTED`, or `BLOCKED` | Never turn an elapsed timer into a retry |
| 9:00–10:00 | Emit the bounded result with owner, witness, exit condition, and no-action/request fields, then stop | No build, broad test, provider call, download, or architecture survey |

Known contradictions stop as soon as both incompatible shapes and the allowed
change are established. The remaining identity/read steps are only to prove
that an existing prerequisite or corrective Work suppresses a duplicate.

## Stable identity and duplicate check

Normalize each component by trimming whitespace, lowercasing ASCII identifiers,
and rejecting empty `project`, `criterionId`, `contractRevision`, or
`capabilityKey` values. Build this exact UTF-8 tuple, with no timestamps, tick
numbers, sweep counters, or model output:

```text
project|criterionId|contractRevision|capabilityKey
```

For the LocalAI witness the canonical tuple is:

```text
localai|la-06|localai-project-v1|models.inferenceoutput.artifact
```

Its SHA-256 is
`96452494c59c43816af0dd5190602e9876157a2be06e8c09f01e2efd4a23fb1e`.
The bounded request identity is therefore:

```text
discovery-prerequisite/localai/la-06/localai-project-v1/96452494c59c43816af0dd5190602e9876157a2be06e8c09f01e2efd4a23fb1e
```

This is below the existing 180-character request limit and is reused on a
lost response or repeated pass. A Work-specific prerequisite may add its
stable public Work identity to `capabilityKey`; it must never use a sweep time
or an ephemeral Worker Session attempt as its only identity.

Before a future submit, read the selected session's public Work list with
superseded history as applicable and its canonical Factory Event stream. Match
the request ID first, then the normalized tuple in the Work/Event correlation.
An `INITIAL` or `PROCESSING` equivalent is `ALREADY_ADMITTED`. A failed Work
or failed historical event is preserved causal evidence, not an active
duplicate. A new contract revision or causal capability produces a new identity
only after the owner has recorded that it is genuinely new evidence.

## Concrete LocalAI contradiction witness

The measured public fixture is Factory Session
`5cd877a9-2674-451d-bcc8-c3bda4d3f9c0`, selected explicitly from
`GET /factory-sessions`; its runtime lifecycle status was `RUNNING`.

| Owner/path and symbol | Observed current capability | Consequence |
| --- | --- | --- |
| `pkg/services/models/managed_runtime_contract.go:163-166`, `models.InvocationProtocolResponse` | The detached protocol response contains only `Text` and `Usage`; it has no artifact reference, size, or digest | The protocol boundary cannot itself provide the required artifact identity |
| `pkg/services/models/internal/backends/localai/omni_codec.go:118-166`, `(*localai.OmniCodec).Invoke` | OMNI calls `Predict`, requires non-empty `response.Text`, returns text `InferenceContent`, and optionally returns usage when declared; it does not construct an artifact-bearing output | The pinned LocalAI OMNI path exposes text/usage only |
| `pkg/services/models/wire/wire.go:454-462`, `operationInvocationRuntime.Invoke` | OMNI requests route to `runtime.omni.Invoke`; only non-OMNI fallthrough reaches the generic runtime | The artifact-capable generic fallback does not repair this OMNI response shape |
| `pkg/services/workers/internal/services/runners/internal/inference/generic_invocation.go:257-273`, `baseWorkContentPartFromModelOutput` | `WorkContentPart.ArtifactID` and artifact metadata are assigned only when `InferenceOutput.Artifact` is non-nil and non-zero | Text/usage alone cannot prove the immutable Work artifact identity/size-or-digest lineage |

Factory Event `factory-event/dispatch-completed/96b67964-3721-4d05-afd0-611f4a4f6f8b`
was read at context sequence `70` and event time
`2026-09-05T06:31:23.5071553Z`. Its feedback retained
`category=contract_conflict` and named the same `Text`/`Usage` versus
`InferenceOutput.Artifact` gap. The associated Worker Session read reports the
same attempt ID, `FAILED` state, and `WORKERS_EXECUTION_FAILURE` with
`family=terminal type=unknown`; this is failure evidence, not retry evidence.

The current public Work read also preserved the causal lineage:

- `batch-localai-project-cycle-001-e10e3884-localai-factory-llm-lineage` is
  `failed` and remains historical evidence.
- `batch-localai-project-cycle-002-contract-delta-e10e3884-localai-omni-artifact-contract-delta`
  is `failed` and remains preserved; it is not silently replaced.
- `batch-localai-project-cycle-001-e10e3884-localai-recover-real-inference` is
  `to-complete`, so its recovery dependency remains visible.
- `batch-localai-project-cycle-003-authority-retry-e10e3884-localai-omni-artifact-contract-delta-authority-retry`
  is `init`, an existing nonterminal authority-retry lineage. The preflight
  must not submit another corrective Work for the same causal tuple.

Because the criterion requires an artifact-bearing capability while the
permitted slice cannot amend immutable `localai-project-v1` or LocalAI runtime
contracts, the result is `CONTRADICTION` with the allowed exit
`discovery-prerequisite/localai/la-06/localai-project-v1/<sha256>`. The result
sets `workRequestSubmitted=false` and `retryRequested=false`, preserves every
listed Work and the historical event, and does not add an operator gate.

## Controlled tabletop

The following rows are the T001 witness. Each row uses fresh fixture values and
records the expected public result; it is not a claim that future runtime code
already exists.

| Case | Given | When | Then / observer |
| --- | --- | --- | --- |
| M-01 | A required capability has no bounded public operation or directly cited symbol | The ten-minute preflight runs | Return `DISCOVERY_REQUIRED` with the missing capability, owner, dependency, witness, smallest exit condition, and stable prerequisite ID; submit no implementation Work and retry nothing |
| M-02 | The LocalAI criterion requires `InferenceOutput.Artifact`, while `InvocationProtocolResponse` exposes only `Text`/`Usage`, and `localai-project-v1` cannot be amended by this slice | The preflight evaluates the criterion | Return `CONTRADICTION` naming all four symbols/boundaries, recognize the existing corrective/recovery lineage, submit no duplicate Work Request, and retry no failed Work |
| M-03 | A representative routine criterion is covered by the existing public Work, Factory Event, and Worker Session read controls and has a permitted design change | The preflight evaluates the bounded slice | Return `ADMIT_BOUNDED_SLICE` with capability, permitted change, dependency, owner, and witness; do not create `HUMAN_APPROVAL` or another operator-approval gate |
| M-18 | The routine design has no authority, budget, safety, or immutable-contract decision | The supervisor/lead submits the design path through ordinary delivery/review | Continue autonomously; escalate only if a real authority or contract decision appears |

For M-03, the capability/witness map is explicit: the capability is the
existing public Work list (`listWorkBySessionId`), canonical Factory Event
stream (`getEventsBySessionId`), and Worker Session list
(`listWorkerSessionsBySessionId`) in `api/openapi-main.yaml`; the permitted
change is the leased design artifact only; the dependency is `None`; the owner
is Luna; and the witness is a read-only reviewer check of those public results.
This is a controlled feasibility example, not an authorization to change
those APIs.

## Verification

- **Behavioral witness:** An independent reviewer walks M-01, M-02, M-03, and
  M-18 with the exact fixture and records the decision, capability map,
  prerequisite ID, owner, witness, and no-action/request outcome.
- **Executable-spine effect:** `establish`.
- **Required evidence 1:** Scope `functional`; dependency fidelity
  `local_real`. Procedure: read-only `GET /factory-sessions`, selected-session
  Work and Worker Session reads, a bounded Factory Event SSE read beginning
  after sequence 65, and direct inspection of the four cited Models/Workers
  symbols. It proves the current failure identity, preserved corrective/recovery
  lineage, and concrete capability boundary. It does not prove future preflight
  runtime code or real LocalAI inference.
- **Required evidence 2:** Scope `functional`; dependency fidelity
  `controlled`. Procedure: one reviewer tabletop of M-01, M-02, M-03, and M-18
  using the ten-minute stopwatch and no writes. It proves deterministic
  classification, stable prerequisite identity, and the no-approval/no-retry
  boundary. It does not prove canonical incident append, replay, or public
  runtime idempotence.
- **Highest feasible level:** Functional design review combining local public
  evidence and the controlled tabletop. Production runtime is explicitly out
  of scope.
- **Remaining unproven edges:** future preflight implementation and public
  admission behavior → `GATE-IMPL-PREFLIGHT`; LocalAI artifact contract and
  acceptance → `LA-05/LA-06`; independent clean-room completeness → `VAL-001`.
- **Test-layer design when tests change:** No tests change in T001. Future pure
  classification branches belong in `POLICY-UNIT` with explicit fake public
  responses, fresh values per case, no root process, no sleeps, and no
  source-scanning meta-tests. Public admission behavior belongs in the prebuilt
  `PUBLIC-REPLAY` lane. No current command claims either future lane.

**Paid validation, when applicable:**

- Trigger: None; the customer budget prohibits new provider/model validation.
- Maximum calls: 0.
- Maximum cost: USD 0.
- Maximum duration: 0 paid execution minutes.
- Fixture and output validator: Sanitized local public reads and controlled
  tabletop only.
- Evidence-reuse key: `FACTORY-FEASIBILITY-01/preflight-v1`.

**Operational and rollout notes:** This is a design-only admission boundary.
Do not change the live schedule, Factory settings, `localai-project-v1`, or any
active Work. A future implementation stops on a known contradiction, checks
identity before submitting, and uses the named owner/exit condition. A
duplicate, stale, malformed, or unreadable identity is a fail-closed result;
timer passage never retries blocked Work.

**Escalation:** Stop with a structured blocker if a criterion cannot be mapped
to a concrete capability, if public Work/Event identity cannot be read, if the
ten-minute boundary would be exceeded, or if the permitted change conflicts
with an immutable contract. The blocker must name the failed criterion, exact
evidence, customer/system impact, safe read-only work completed, accountable
owner, observable exit, and smallest discovery/contract delta. Do not guess or
broaden the task.

**Handoff artifacts:** This task packet's preflight decision table, LocalAI
contradiction witness, stable prerequisite identity rule, M-01/M-02/M-03/M-18
tabletop, and `GATE-PREFLIGHT` inputs. T002 consumes the vocabulary and identity
rule for the canonical incident disposition; T003/T004 own the complete matrix,
clean-room report, delivery path, and rollout gates.
