# C10 Work relationships shared-process evidence

## Story 001 — inventory and shared executable spine

Status: PASS for `functional-test-optimization-c10-work-relationships-001`.

This document records the plan-time denominator and the first bounded migration
before the remaining eligible rows are moved. It is diagnostic evidence, not a
local performance threshold.

### Plan and scope trace

The task packet names these governing references:

- `docs/temp/functional-test-optimization.md`: Scope 4 — Preserve and extend
  the process-isolation inventory; Scope 5 — Cleanup and teardown determinism
  audit; Scope 10 — Full-inventory migration sweep; Functional test-case
  discipline; Acceptance Criteria 2 and 3.
- `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.md`
  and `.json`: C01 package and row classification.
- `factory/docs/standards/implementation-standards.md` and
  `factory/docs/standards/task-template.md`: one behavior slice, production
  wiring, bounded evidence, and no overclaiming.

The governing `docs/temp/functional-test-optimization.md` path is not present
in this checkout. The exact source-plan references above are retained from the
task packet (`prd.md`/`prd.json`); no excluded plan or inventory surface was
modified.

### GATE-INVENTORY — pre-edit denominator

The pre-edit discovery command was run at commit
`0c3bb85791` before changing the relationship package:

```text
go test ./tests/functional/work/relationships -list '^Test'
```

Exit status: `0`. It reported 14 top-level tests and 12 named shared-server
subtests. The exact top-level identities were:

```text
TestCrossBatchDependsOnRejectsCrossSessionTargetAtomically
TestCrossBatchDependsOnCompletedTargetReleasesAtAdmission
TestCrossBatchDependsOnFailedTargetCascadesAtAdmission
TestCrossBatchDependsOnMixedTerminalFanInCascades
TestDependentWorkDoesNotDispatchAfterPrerequisiteFailure
TestParentChildLineageSurvivesDispatchAndReplay
TestChildFailureProjectsToDocumentedParentView
TestDispatchPreservesSubmittedWorkPayloadTagsAndType
TestRejectionFeedbackSurfacesOnExecutorRetry
TestParentAndDependsOnLineageSurviveOnChildDispatch
TestDependentWorkFailsWhenDirectPrerequisiteFails
TestTransitiveDependencyFailureCascadesToFailedTerminals
TestCompletedPrerequisiteIsNotCascadedWhenDependentFails
TestSharedServerRelationships
```

The named denominator was the 12 rows under
`TestSharedServerRelationships`: `MultiOutputFanoutPreservesSourceNameOnDownstreamWork`,
`MultiOutputNameAvailableOnDownstreamTask`, `ReviewerFanoutPreservesSharedNameDownstream`,
`DocReviewerPNGFanoutPreservesSharedNameDownstream`,
`NtoNTypeMatchingCompletesEveryAuthoredBranch`,
`DependentWorkWaitsForPrerequisiteTargetState`,
`WorkWithoutDependsOnRelationsDispatchesNormally`,
`DependentWorkBlockedUntilPrerequisiteArchived`,
`DependentWorkBlockedDuringPrerequisiteProcessing`,
`DependentWorkAndPrerequisiteBothReachArchived`,
`FanInReleasesOnlyAfterEveryPrerequisite`, and
`CrossBatchDependsOnActivePrerequisiteReleasesAfterCompletion`.

The C01 classification and behavior witness for every relationship row is
fixed below. The current selectors and source locations are the pre-edit
selectors; rows marked `migrated in story 001` have a new shared selector
recorded in the implementation table.

| ID | Pre-edit selector and source | C01 classification | Behavior witness |
| --- | --- | --- | --- |
| REL-001 | `^TestCrossBatchDependsOnRejectsCrossSessionTargetAtomically$` — `cross_batch_isolation_test.go:24` | shareable-with-mock | Cross-session `DEPENDS_ON` admission returns the existing 400 markers and admits no dependent or sibling Work. |
| REL-002 | `^TestCrossBatchDependsOnCompletedTargetReleasesAtAdmission$` — `cross_batch_terminal_test.go:25` | shareable-with-mock | A completed earlier target releases a later batch, keeps the canonical target ID, and preserves dispatch ordering. |
| REL-003 | `^TestCrossBatchDependsOnFailedTargetCascadesAtAdmission$` — `cross_batch_terminal_test.go:53` | shareable-with-mock | A later batch against a failed target is admitted, cascades to failed, and receives no dependent dispatch. **Migrated in story 001.** |
| REL-004 | `^TestCrossBatchDependsOnMixedTerminalFanInCascades$` — `cross_batch_terminal_test.go:74` | shareable-with-mock | A complete plus failed fan-in keeps both relations, fails the dependent, and suppresses its dispatch. |
| REL-005 | `^TestDependentWorkDoesNotDispatchAfterPrerequisiteFailure$` — `dependencies_test.go:97` | shareable-with-mock | A failed prerequisite projects its dependent to failed, with prerequisite-only dispatches and no dependent dispatch. **Migrated in story 001.** |
| REL-006 | `^TestParentChildLineageSurvivesDispatchAndReplay$` — `parent_child_test.go:45` | isolated-with-reason | Built CLI submission preserves PARENT_CHILD lineage, request identity, Work, dispatch, retained events, replay, and Work show. |
| REL-007 | `^TestChildFailureProjectsToDocumentedParentView$` — `parent_child_test.go:147` | isolated-with-reason | Built CLI child failure preserves lineage and the documented parent failure projection and event order. |
| REL-008 | `^TestDispatchPreservesSubmittedWorkPayloadTagsAndType$` — `parent_child_test.go:237` | shareable-with-mock | Dispatch preserves submitted payload, tags, Work type, and non-empty Work ID. |
| REL-009 | `^TestRejectionFeedbackSurfacesOnExecutorRetry$` — `parent_child_test.go:283` | shareable-with-mock | The first executor attempt has no rejection tag; the second contains feedback and call count is exactly two. |
| REL-010 | `^TestParentAndDependsOnLineageSurviveOnChildDispatch$` — `parent_child_test.go:315` | shareable-with-mock | Child dispatch retains exact PARENT_CHILD and DEPENDS_ON targets after prerequisite completion. |
| REL-011 | `^TestDependentWorkFailsWhenDirectPrerequisiteFails$` — `parent_child_test.go:393` | shareable-with-mock | Direct prerequisite failure fails both affected Work items, leaves the relation public, and suppresses dependent dispatch. |
| REL-012 | `^TestTransitiveDependencyFailureCascadesToFailedTerminals$` — `parent_child_test.go:443` | shareable-with-mock | An upstream failure reaches all affected terminal Work items, with only upstream attempts and well-formed command requests. |
| REL-013 | `^TestCompletedPrerequisiteIsNotCascadedWhenDependentFails$` — `parent_child_test.go:507` | shareable-with-mock | A completed prerequisite remains complete when a later dependent fails; the existing four-attempt assertion remains owned by that test. |
| REL-014 | `^TestSharedServerRelationships$` — `shared_server_test.go:19` | shareable-with-mock | One root-built application host runs isolated relationship sessions with deterministic gate cleanup. |
| REL-015 | `^TestSharedServerRelationships$/MultiOutputFanoutPreservesSourceNameOnDownstreamWork$` — `shared_server_test.go:54` | shareable-with-mock | Multi-output downstream Work preserves source name, IDs, trace identity, and tags. |
| REL-016 | `^TestSharedServerRelationships$/MultiOutputNameAvailableOnDownstreamTask$` — `shared_server_test.go:55` | shareable-with-mock | A downstream Work item observes the submitted source name and exact Work count. |
| REL-017 | `^TestSharedServerRelationships$/ReviewerFanoutPreservesSharedNameDownstream$` — `shared_server_test.go:56` | shareable-with-mock | Authored reviewer fan-out branches complete and preserve the source document name. |
| REL-018 | `^TestSharedServerRelationships$/DocReviewerPNGFanoutPreservesSharedNameDownstream$` — `shared_server_test.go:57` | shareable-with-mock | Packaged PNG reviewer branches complete and preserve the submitted name. |
| REL-019 | `^TestSharedServerRelationships$/NtoNTypeMatchingCompletesEveryAuthoredBranch$` — `shared_server_test.go:58` | shareable-with-mock | Authored N-to-N branches complete with preserved names, distinct IDs and traces, and tag policy. |
| REL-020 | `^TestSharedServerRelationships$/DependentWorkWaitsForPrerequisiteTargetState$` — `shared_server_test.go:59` | shareable-with-mock | Dependent Work stays undispatched until the prerequisite reaches the declared required state, then both complete. |
| REL-021 | `^TestSharedServerRelationships$/WorkWithoutDependsOnRelationsDispatchesNormally$` — `shared_server_test.go:60` | shareable-with-mock | Work without `DEPENDS_ON` dispatches normally and completes once. |
| REL-022 | `^TestSharedServerRelationships$/DependentWorkBlockedUntilPrerequisiteArchived$` — `shared_server_test.go:61` | shareable-with-mock | An archived-state dependent stays blocked until the prerequisite archives, then both archive in order. |
| REL-023 | `^TestSharedServerRelationships$/DependentWorkBlockedDuringPrerequisiteProcessing$` — `shared_server_test.go:62` | shareable-with-mock | The dependent remains blocked during prerequisite processing and unlocks only at archived. |
| REL-024 | `^TestSharedServerRelationships$/DependentWorkAndPrerequisiteBothReachArchived$` — `shared_server_test.go:63` | shareable-with-mock | Both Work reach archived with zero initial, review, or failed Work. |
| REL-025 | `^TestSharedServerRelationships$/FanInReleasesOnlyAfterEveryPrerequisite$` — `shared_server_test.go:64` | shareable-with-mock | Fan-in remains blocked after a partial prerequisite set and dispatches only after every prerequisite completes. |
| REL-026 | `^TestSharedServerRelationships$/CrossBatchDependsOnActivePrerequisiteReleasesAfterCompletion$` — `shared_server_test.go:67` | shareable-with-mock | A gated active cross-batch prerequisite keeps its dependent at init, then releases it once after completion. |

The lifecycle witness inventory is also frozen for the lane:

| ID | Lifecycle witness | Story-001 status |
| --- | --- | --- |
| LIFE-001 | Overlapping gated, failure, fan-out, and ordinary sessions keep IDs, Work, events, edge calls, and cleanup disjoint. | Representative shared host exercised; complete proof is story 002. |
| LIFE-002 | Cancellation ends a controlled wait, stops/deletes the session, releases streams/routes, and leaves another session usable. | Story 002. |
| LIFE-003 | Bounded dependency timeout names the stage, avoids a false terminal assertion, and cleans up. | Story 002. |
| LIFE-004 | Early assertion exit runs cleanup, releases session/temp roots, preserves the primary failure, and permits host reuse. | Reuse probe pattern established; complete early-exit proof is story 002. |
| LIFE-005 | Successful cleanup releases session-owned routes, streams, temporary roots, and artifacts while the host stays usable. | Representative success and reuse probes pass. |
| LIFE-006 | Failure cleanup releases the same resources without masking the primary diagnostic. | Representative failure and reuse probes pass. |
| LIFE-007 | Retry, dispatch, and Work counts and canonical identities remain unchanged. | Existing assertions remain; complete migration audit is story 002. |
| LIFE-008 | Isolated persistence/replay history matches and teardown removes test artifacts. | Retained isolated rows; story 003 integrated proof. |
| LIFE-009 | The loopback server has no authentication boundary. | Not applicable; no auth fake added. |
| LIFE-010 | Fan-in and overlapping explicit sessions exercise representative capacity without claiming a new maximum. | Representative shared host exercised; complete overlap proof is story 002. |

### Topology ledger

| Point | Shared root-built host | Remaining eligible scenario hosts | Isolated built-CLI hosts | Estimated root starts | Built-CLI commands |
| --- | ---: | ---: | ---: | ---: | ---: |
| Plan-time pre-edit denominator | 1 | 11 | 2 | **14** | **5** |
| Story-001 head | 1 | 9 | 2 | **12** | **5** |
| Story-002 head | 1 | 0 | 2 | **3** | **5** |
| Planned final C10 topology | 1 | 0 | 2 | **3** | **5** |

The story-001 shared host has 14 named scenario selectors: the original 12
plus migrated REL-003 and REL-005. Each migrated scenario opens a unique
explicit Factory Session with `OpenFactorySessionAt`; no migrated scenario
uses the default session. The existing two built-CLI rows are unchanged and
remain scenario-owned for their complete executable-to-server/provider
boundary proof.

The C01 diagnostic timing samples were 14.177s, 11.461s, and 12.512s on its
recorded environment, with a 12.512s median and 21.71% range-over-median
variance. They are retained as directional context only; no local wall-time
threshold is imposed.

### GATE-SPINE and representative migration evidence

The following commands were run after the story-001 edits at the local head.
All use production `root.BuildProcess` through
`support.StartFunctionalAPIServer`, public CLI submission, explicit Factory
Session/Work/Event observations, and controlled local mock-worker edges.

| Evidence | Command | Result | Property proved |
| --- | --- | --- | --- |
| Representative migration | `go test -count=1 -timeout=10m -run '^TestSharedServerRelationships$/(CrossBatchDependsOnFailedTargetCascadesAtAdmission\|DependentWorkDoesNotDispatchAfterPrerequisiteFailure)$' ./tests/functional/work/relationships` | exit 0; package 2.487s; shell wall 8.775s | REL-003 and REL-005 pass through the one host, with explicit sessions, unchanged public failure/relation/no-dispatch assertions, and reuse probes after cleanup. |
| Shared executable spine | `go test -count=1 -timeout=10m -run '^TestSharedServerRelationships$' ./tests/functional/work/relationships` | exit 0; package 2.907s; shell wall 9.311s | All 14 shared named scenarios, including REL-014 through REL-026 and the two migrated representatives, coexist on one host. |
| Post-edit discovery | `go test ./tests/functional/work/relationships -list '^Test'` | exit 0; 12 top-level tests listed | The two migrated rows moved under the shared umbrella; the remaining top-level rows, including both isolated cases, remain discoverable. |

`openSharedRelationshipSession` registers idempotent cleanup through
`support.CloseFactorySessionAt`. The representative helpers close their
session before running `runSharedHostReuseProbe`, which opens a new explicit
session and completes a Work item on the same host. This proves the owned
success/failure cleanup boundary and later host reuse; it does not claim a
host-wide process census or universal cleanup coverage.

### Assertion migration mapping

| Row | Before | Story-001 shared path | Preserved witness |
| --- | --- | --- | --- |
| REL-003 | Own process, default session, `crossBatchPrerequisite*` fixture IDs | `TestSharedServerRelationships/CrossBatchDependsOnFailedTargetCascadesAtAdmission`, unique explicit session and unique failure fixture IDs | Failed prerequisite/dependent states, canonical `DEPENDS_ON` target ID, and no dependent dispatch. |
| REL-005 | Own process with a sequence `ProviderOverride` | `TestSharedServerRelationships/DependentWorkDoesNotDispatchAfterPrerequisiteFailure`, explicit session and the existing shared mock-worker edge keyed to a unique prerequisite ID | Two failed Work items, no successful dependent state, exactly one prerequisite start and finish dispatch, no dependent dispatch, and terminal counts `0/2`. |

The controlled edge implementation remains package-local to the relationship
test package. No production, OpenAPI, generated, shared-support, inventory,
baseline, workflow, or stability-cleanup file changed.

### Story-001 handoff gates (resolved or carried by Story 002)

- GATE-MIGRATION, GATE-REPEAT, and GATE-RACE are closed by the Story-002
  evidence below; REL-006 and REL-007 remain intentionally process-boundary
  tests.
- GATE-PACKAGE: the exact final-head full package command is story 003 and is
  not run in Stories 001 or 002.
- GATE-SCOPE and GATE-LOOPBACK: final owned-surface audit and clean-room
  validation are story 003.
- GATE-PR-CI: review-owned CI and PR timing direction are not claimed here.
- Project-level AC3: three uncached full functional-suite runs remain owned by
  the project gate after relevant slices merge.

## Story 002 — complete remaining migration and lifecycle isolation

Status: PASS for `functional-test-optimization-c10-work-relationships-002`.

The remaining eligible rows now use the same root-built host as the Story-001
representatives. Every scenario opens a unique explicit Factory Session, and
the provider override routes controlled responses by normalized Factory
directory so parallel sessions cannot consume one another's response state.
The shared host also injects the package-local static script edge required by
the existing SCRIPT_WORKER fixtures. REL-006 and REL-007 remain unchanged as
the two real built-CLI process-boundary proofs, with inline isolation reasons.

### GATE-MIGRATION — before and after evidence

Before editing the two remaining migration groups, their current top-level
selectors were run once at the Story-001 head. No pre-change full package run
was used:

| Group | Before command | Before result and material assertion |
| --- | --- | --- |
| Cross-batch admission | `go test -count=1 -timeout=10m -run '^(TestCrossBatchDependsOnRejectsCrossSessionTargetAtomically\|TestCrossBatchDependsOnCompletedTargetReleasesAtAdmission\|TestCrossBatchDependsOnMixedTerminalFanInCascades)$' ./tests/functional/work/relationships` | exit 0; package 3.980s; the three public cross-session, completed-target, and mixed terminal/fan-in witnesses passed. |
| Parent/dependency boundaries | `go test -count=1 -timeout=10m -run '^(TestDispatchPreservesSubmittedWorkPayloadTagsAndType\|TestRejectionFeedbackSurfacesOnExecutorRetry\|TestParentAndDependsOnLineageSurviveOnChildDispatch\|TestDependentWorkFailsWhenDirectPrerequisiteFails\|TestTransitiveDependencyFailureCascadesToFailedTerminals\|TestCompletedPrerequisiteIsNotCascadedWhenDependentFails)$' ./tests/functional/work/relationships` | exit 0; package 3.992s; payload/tags/type, retry feedback, lineage, direct/transitive failure, and completed-prerequisite witnesses passed. |

The post-migration mapping is complete: REL-014 is the shared umbrella, its
23 child selectors cover REL-001 through REL-005, REL-008 through REL-013,
and REL-015 through REL-026, and REL-006/REL-007 are the only top-level
scenario selectors. All migrated
public Work, Factory Session, dispatch, Factory Event, ordering, relation,
failure, retry, payload, and count assertions remain in their owning test
functions. The native provider migrations for REL-011 through REL-013 retain
the controlled-edge proof by asserting non-empty transition, Work correlation,
and Factory-directory request identity in addition to the unchanged public
projections and call counts.

| After evidence | Command | Result and property proved |
| --- | --- | --- |
| Complete shared behavior | `go test -count=1 -run '^TestSharedServerRelationships$' -timeout=10m ./tests/functional/work/relationships` | exit 0; package 4.368s; all 23 eligible child scenarios plus the lifecycle probe group pass on one host, with unique explicit sessions and session-scoped observations. |
| Retained process proofs | `go test -count=1 -run '^(TestParentChildLineageSurvivesDispatchAndReplay\|TestChildFailureProjectsToDocumentedParentView)$' -timeout=10m ./tests/functional/work/relationships` | exit 0; package 2.436s; both built-CLI cases preserve their public CLI, event/replay, lineage, and parent-failure projections. |
| Final discovery | `go test ./tests/functional/work/relationships -list '^Test'` | exit 0; exactly three top-level tests: the two retained built-CLI tests and `TestSharedServerRelationships`. |

### LIFE-001 through LIFE-010 — lifecycle and isolation evidence

The shared umbrella runs its 23 behavior children in parallel. Each fixture
directory is unique, each session is opened with `OpenFactorySessionAt`, each
session is closed through the public terminate/wait/delete sequence, and the
provider router is checked for zero remaining routes after child cleanup. The
following non-row lifecycle probes run on that same host:

- `CancellationReleasesGatedAttempt` terminates a session while a controlled
  finish attempt is blocked, observes the provider context cancellation and
  stopped session, deletes it, and completes a reuse probe.
- `BoundedObservationTimeoutPreservesDiagnostic` uses a 100ms bounded public
  Work observation, requires the timeout diagnostic and still-processing gated
  state, then closes the session and proves host reuse.
- `EarlyReturnRetainsPrimaryDiagnostic` returns a synthetic assertion error
  through a helper with deferred session cleanup, verifies the exact primary
  error survives, and proves host reuse afterward.

These probes exercise session cancellation, timeout diagnostics, early-return
cleanup, successful and failed scenario cleanup, route ownership, overlapping
session isolation, retry/dispatch counts, and representative fan-in capacity.
They do not claim a host-wide process census, universal race freedom, or a new
capacity maximum. The two retained built-CLI rows continue to own their child
process, isolated provider state, and temporary-artifact proof.

### GATE-REPEAT and GATE-RACE

| Gate | Command | Result and property proved |
| --- | --- | --- |
| GATE-REPEAT | `go test -count=3 -run '^TestSharedServerRelationships$/(CrossBatchDependsOnRejectsCrossSessionTargetAtomically\|CrossBatchDependsOnFailedTargetCascadesAtAdmission\|DependentWorkDoesNotDispatchAfterPrerequisiteFailure\|FanInReleasesOnlyAfterEveryPrerequisite\|CrossBatchDependsOnActivePrerequisiteReleasesAfterCompletion)$' -timeout=10m ./tests/functional/work/relationships` | exit 0; package 8.257s; all five gated/failure/cross-session selectors pass three times with their existing Work, dispatch, and cleanup assertions. |
| GATE-RACE | `go test -race -count=1 -run '^TestSharedServerRelationships$' -timeout=10m ./tests/functional/work/relationships` | exit 0; package 10.098s; no exercised race in shared routing, parallel sessions, gates, cancellation, or cleanup probes. |
| Lifecycle focused selector | `go test -count=1 -run '^TestSharedServerRelationships$/LifecycleCleanupProbes$' -timeout=10m ./tests/functional/work/relationships` | exit 0; package 2.777s; cancellation, bounded timeout, early-return diagnostic preservation, and host reuse pass. |

The package-level result is intentionally not claimed here. GATE-PACKAGE,
GATE-SCOPE, GATE-LOOPBACK, GATE-PR-CI, and the project-level AC3 suite remain
owned by Story 003 or the review/project gates.
