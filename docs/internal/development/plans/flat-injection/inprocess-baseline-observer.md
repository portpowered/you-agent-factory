# Authorized in-process baseline observation method

Authority: OWNER option b, 2026-10-03T22:45Z. External-reader acquisition and the no-test-hook restriction alone are replaced. Original pin, resource meanings, 10 inert + 10 lifecycle valid samples, full ranges, <=10% final median/p95 and 100-cycle retention remain unchanged. Historical rejection dossiers and immutable Project request/source-plan/acceptance are not edited.

Executable runbook: [in-process owner observation support](../../../../../tests/stress/observer/README.md). No production API, generated contract, owner policy or Wire changes. Independent OBS-VAL/OBS-REVIEW owns clean-checkout reproduction. Full P01/FI-A6/I01/VAL01 remains unproven.

## Amendment table

| Amendment | Authored source | Authority |
| --- | --- | --- |
| AM10-plan | `docs/internal/development/plans/flat-injection/plan.md:230 (method requirement)` | OWNER22:45Z |
| AM10-prerequisite | `docs/internal/development/plans/flat-injection/plan.md:245 (method requirement)` | OWNER22:45Z |
| AM10-T27 | `docs/internal/development/plans/flat-injection/tasks.md:2892 (method requirement)` | OWNER22:45Z |
| AM10-T22 | `docs/internal/development/plans/flat-injection/tasks.md:2375 (method requirement)` | OWNER22:45Z |
| AM10-contracts | `docs/internal/development/plans/flat-injection/contracts.md:2128 (method requirement)` | OWNER22:45Z |
| AM10-inventory | `docs/internal/development/plans/flat-injection/inventory.md:260 (method requirement)` | OWNER22:45Z |
| AM10-review | `docs/internal/development/plans/flat-injection/validation-review.md:133 (method requirement)` | OWNER22:45Z |
| RQ-2 carried-forward method amendment | `docs/internal/development/plans/flat-injection/inprocess-baseline-observer.md RQ-2 amendment table (historical excerpt: tasks/todo/fi-t27-no-write-reader-qualification-20261003.json acceptanceCriteria[id=RQ-2]; old packet unchanged)` | OWNER22:45Z |

## AM10-plan

Current:

```text
| AM10 / T27 | Pinned baseline cannot count private handles/leases through public session counts. `docs/internal/development/plans/flat-injection/tasks.md:2788,2807,2822; pkg/services/factory_runtime/internal/services/instance_host/internal/service/service.go:22; pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service/service.go:21-22 (pin 95e213cfb35b50236fd7a34ad66c797d2ee7b5b6)` | Require a read-only owner-specific baseline observation prerequisite before P01. Count instance-host handles, Models lease records and capacity-holder values, plus goroutines, at quiescent lifecycle checkpoints. The prerequisite must deliver an external read-only observer runbook for the exact pinned artifact and prove private-map access, synchronization, nonmutation and matching timing methodology before T27 can report. Observation tool/seam feasibility is explicitly unresolved at that owning gate; this docs amendment does not claim it is proven. No production instrumentation, public-count substitute, alternate pinned commit or fabricated P01. Unsupported observation remains BLOCKED at that gate. | FI-A6/P01, FI-A4 | FI engineering baseline-observation prerequisite owner; T27 consumes, T22 compares | Amendment merge; independently reviewed read-only observation runbook/artifact proof on pinned source, then T27 |
```

Proposed:

```text
| AM10 / T27 | Pinned baseline cannot count private handles/leases through public session counts. `docs/internal/development/plans/flat-injection/tasks.md:2788,2807,2822; pkg/services/factory_runtime/internal/services/instance_host/internal/service/service.go:22; pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service/service.go:21-22 (pin 95e213cfb35b50236fd7a34ad66c797d2ee7b5b6)` | Require synchronized program-owned in-process counters from an admitted runtime smoke, through a test-only hook or existing metrics/diagnostics and no new public API, at the exact original source pin. Attest all test-overlay bytes and artifact identities separately. Prove owner-map access, nonmutation, checkpoints and matching final observation/overhead before T27 reports. Preserve all other FI-A6/P01 requirements and the historical rejection dossiers. Unsupported observation remains a finding. | FI-A6/P01, FI-A4 | FI engineering baseline-observation prerequisite owner; T27 consumes, T22 compares | Amendment merge; independently reviewed read-only observation runbook/artifact proof on pinned source, then T27 |

Authority: OWNER option b, 2026-10-03T22:45Z. All acceptance outside the observation technology remains unchanged.
```

## AM10-prerequisite

Current:

```text
| FI-PREREQ-BASELINE-OBS | FI engineering baseline-observation owner; pinned artifact and private counted owners in inventory.md | Independently reviewed read-only observer runbook proves map access, synchronization, nonmutation and comparable timing before T27; feasibility unresolved, otherwise BLOCKED. No public-count substitution or baseline production edits. |
```

Proposed:

```text
| FI-PREREQ-BASELINE-OBS | FI engineering in-process baseline-observation owner; original pin plus attested test-overlay artifact | Independently reviewed synchronized owner-counter smoke/runbook proves complete private owner observations, nonmutation and matching final observation/overhead before T27. No new public API or public-count substitution. Failure remains a finding; feasibility alone does not pass P01. |

Authority: OWNER option b, 2026-10-03T22:45Z. All acceptance outside the observation technology remains unchanged.
```

## AM10-T27

Current:

```text
AM10: baseline stays pinned to `95e213cfb35b50236fd7a34ad66c797d2ee7b5b6`. Lifecycle driving uses public BuildProcess/Execute/session boundaries; private handle/lease/capacity observations require the independent read-only baseline observer runbook in inventory.md. Prove access, synchronization, nonmutation and matching timing before reporting P01. Tool/seam feasibility is unresolved at FI-PREREQ-BASELINE-OBS; unsupported access remains BLOCKED. No baseline production instrumentation or public session-count substitute is permitted.
```

Proposed:

```text
AM10 / OWNER22:45Z: baseline source stays pinned to `95e213cfb35b50236fd7a34ad66c797d2ee7b5b6`. Lifecycle driving uses public BuildProcess/Execute and explicit-session open/replace/close boundaries. FI-PREREQ-BASELINE-OBS supplies synchronized program-owned instance-host handles, Models lease records with status, each capacityHolders integer and total, and live goroutines through an authorized test-only hook or existing metrics/diagnostics. No new public API. Prove complete owner access, nonmutation, Q0/Q1/Q2 and matching final observation/overhead; identify every overlay hash, build command and artifact separately from original source. Unsupported observation remains a finding. No public-session count substitute or fabricated P01 result. All timing/sample/range/100-cycle retention requirements remain unchanged.

Authority: OWNER option b, 2026-10-03T22:45Z. All acceptance outside the observation technology remains unchanged.
```

## AM10-T22

Current:

```text
**Amendment v1.1:** AM10: final P01 comparison consumes T27 exact pinned baseline and the independently reviewed FI-PREREQ-BASELINE-OBS runbook. Count the same private owners/checkpoints with matching methodology; no public session-count substitution, changed pin or invented measurement.
```

Proposed:

```text
**Amendment v1.1 / OWNER22:45Z:** AM10: final P01 comparison consumes T27 exact pinned-source baseline and the independently reviewed FI-PREREQ-BASELINE-OBS in-process observer runbook. Read the same private owners and Q0/Q1/Q2 checkpoints with matching test-overlay instrumentation and overhead accounting. Attest final source/tool/overlay/artifact identities separately; no public session-count substitution, changed pin, dropped sample/range, altered 10-percent median/p95 threshold, changed retention, or invented measurement.

Authority: OWNER option b, 2026-10-03T22:45Z. All acceptance outside the observation technology remains unchanged.
```

## AM10-contracts

Current:

```text
AM10: no public observation API or baseline instrumentation is introduced. FI-PREREQ-BASELINE-OBS must independently prove read-only private handles/leases/capacityHolders observation at original pin; T27/T22 P01 stays blocked if infeasible.
```

Proposed:

```text
AM10 / OWNER22:45Z: no public observation API is introduced. Authorized test-only in-process observations or existing metrics/diagnostics replace external-reader acquisition and the no-test-hook prohibition only. FI-PREREQ-BASELINE-OBS independently proves synchronized complete owner handles, Models lease records with status, per-slot capacityHolders integers and total, and live goroutines at Q0/Q1/Q2 on original pinned source with separately attested overlay/tool/artifact identities. Reads never expire, release or delete state. Unsupported observation remains a finding; T27/T22 timing/retention and full FI-A6/P01 remain later gates.

Authority: OWNER option b, 2026-10-03T22:45Z. All acceptance outside the observation technology remains unchanged.
```

## AM10-inventory

Current:

```text
Pin: `95e213cfb35b50236fd7a34ad66c797d2ee7b5b6`. FI engineering baseline-observation owner must deliver an external read-only observer runbook for this exact artifact; feasibility remains unresolved, not certified here. Synchronization and map access must be proved without production instrumentation, exports, mutation or alternative public counts. Unsupported observation is BLOCKED.
```

Proposed:

```text
Pin: `95e213cfb35b50236fd7a34ad66c797d2ee7b5b6`. FI engineering baseline-observation owner delivers a synchronized in-process observer runbook using an authorized test-only hook or existing metrics/diagnostics, with no new public API. OWNER22:45Z retires external-reader acquisition and permits test-only observation, changing no resource meanings, sample count, timing/range or retention obligation. Actual private-map records and integer values are copied under owner locks without mutation. Original source, overlay bytes, tool and instrumented artifact have separate identities. Unsupported observation remains a finding, never a zero or P01 PASS.

Authority: OWNER option b, 2026-10-03T22:45Z. All acceptance outside the observation technology remains unchanged.
```

## AM10-review

Current:

```text
| AM10 | Exact pin, private handles/lease records/holder values and goroutine counts require read-only access proof | FI-PREREQ-BASELINE-OBS feasibility held; T27/T22 P01 |
```

Proposed:

```text
| AM10 | Exact original source pin, synchronized program-owned handles/lease records with status/per-slot capacity-holder integers and total/live goroutines at Q0/Q1/Q2 through authorized test-only observation; attest overlay/artifact and matching final overhead | FI-PREREQ-BASELINE-OBS independent observer feasibility/checkpoint proof; T27 full baseline, T22 final P01/I01 and VAL01 remain later gates. OWNER22:45Z replaces external-reader acquisition only. |

Authority: OWNER option b, 2026-10-03T22:45Z. All acceptance outside the observation technology remains unchanged.
```

## RQ-2 carried-forward method amendment

Current:

```json
{
  "acceptanceCriteria": [
    {
      "id": "RQ-2",
      "requirement": "An admitted runtime smoke must read the exact composed instance-host handles, retained Models lease records including status, every capacityHolders integer and total, and target live goroutines at coherent owned Q0/Q1/Q2 checkpoints; include a nonzero capacity-holder witness and quiescent lifecycle join correspondence. Never label missing values zero or key count total holders. Record nonmutation evidence and timing comparability; no P01 latency or 100-cycle workload here.",
      "sliceOwns": [
        "RQ-2"
      ],
      "evidenceOwner": "Story 002 if admissible; RQ-SMOKE; otherwise explicitly BLOCKED with RQ-3 evidence",
      "projectRefs": "FI-A6/P01"
    }
  ]
}
```

Proposed:

```json
{
  "acceptanceCriteria": [
    {
      "id": "RQ-2",
      "requirement": "An admitted in-process runtime smoke using an OWNER22:45Z-authorized test-only hook or existing metrics/diagnostics, with no new public API, must read the exact composed instance-host handles, retained Models lease records including status, every capacityHolders integer and total, and target live goroutines at coherent owned Q0/Q1/Q2 checkpoints; include a nonzero capacity-holder witness and quiescent lifecycle join correspondence. Never label missing values zero or key count total holders. Record synchronized nonmutation evidence and matching pin/final observation overhead and timing comparability with separately attested source/overlay/tool/artifact identities; no P01 latency or 100-cycle workload here.",
      "sliceOwns": [
        "RQ-2"
      ],
      "evidenceOwner": "This slice proves calibrated owner reads and composed checkpoints; independent OBS-VAL/OBS-REVIEW qualifies only proven properties. Any absent composed nonzero capacity-holder witness remains BLOCKED at RQ-2-COMPOSED-CAPACITY, owned by Lead/T27 before full prerequisite release. Unit nonzero is not substituted for composed nonzero.",
      "projectRefs": "FI-A6/P01"
    }
  ]
}
```

## Retained verification boundaries

Actual empty composed Models reads prove checkpoint access only. Owner unit nonzero/retained-status calibration cannot substitute for composed nonzero. RQ-2-COMPOSED-CAPACITY remains BLOCKED with Lead/T27 before full prerequisite release. Unsupported observation and goroutine residuals remain findings; no state is deleted to pass.

T27-P01 owns original samples and 100-cycle retention; T22-P01 owns final samples and comparison. I01 owns delivered CLI OS shutdown/flush; VAL01 owns aggregate acceptance. OBS-VAL owns independent observer proof; author evidence is not independent validation.
