# C11 packaged plan and consensus characterization ledger

## Scope and status

This is the story `functional-test-optimization-c11-packaged-plan-consensus-001`
characterization artifact. It freezes the current witness before any fixture
topology change. No test source, shared support, production code, contract,
generated file, or timing threshold is changed by this story.

The C11 package scope is the six explicitly named directories below. Their C01-
C07 rows sum to 38. The numeric range in the PRD is retained as source
terminology; the separate C01 F017 `packaged/review` package is outside the
explicit C11 package list and remains read-only. `packaged/goal` is also
excluded. If the source-plan owner later assigns either package to this lane,
the denominator must be reopened rather than silently expanded.

The source plan named by the PRD (`docs/temp/functional-test-optimization.md`)
and the referenced task packet are absent from this checkout and from the
tracked file list. The available authorities are the PRD, repository
instructions and standards, and the tracked C01-C07 inventory and C07 factory
package ledger. The PRD records the operator-confirmed source-plan revision as
`functional-test-optimization-v2`; the absent source plan is not recreated or
committed.

**Story status:** PASS for the characterization criteria. This does not claim
post-change reuse, race safety, or PR timing.

## Environment and procedures

The characterization base was `d4ce490f7cc1ac6257f0e98038bc52a09d3601e0`,
which was equal to `origin/main` before this ledger was added. The environment
was Windows `10.0.26200.0`, `go1.25.0 windows/amd64`, `GOAMD64=v1`, 24 logical
processors, and the repository `go.mod` with no special `GOFLAGS`. No remote or
paid provider was called; call count and cost were both zero.

### GATE-INVENTORY discovery

Procedure:

```text
go test ./tests/functional/factory/packaged/named_invocation ./tests/functional/factory/packaged/plan_execute ./tests/functional/factory/packaged/plan_parallel ./tests/functional/factory/packaged/quorum ./tests/functional/factory/packaged/ralph ./tests/functional/factory/packaged/subagent -list '^Test' -count=0
```

Observed exit code `0`:

| Package | Top-level selectors | Discovery result |
| --- | ---: | --- |
| `packaged/named_invocation` | 4 | `TestNamedInvocationSharedPreparationFailures`, `TestRun_EffectiveSchemaPreparationFailuresStopBeforeExecutionSideEffects`, `TestNamedInvocationSharedSuccess`, `TestNamedInvocationProviderRouterRejectsAmbiguousAndUnknownRoutes` |
| `packaged/plan_execute` | 1 | `TestPackagedPlanExecute` |
| `packaged/plan_parallel` | 1 | `TestPackagedPlanParallel` |
| `packaged/quorum` | 1 | `TestPackagedQuorum` |
| `packaged/ralph` | 1 | `TestPackagedRalph` |
| `packaged/subagent` | 1 | `TestPackagedSubagent` |

The tracked C01-C07 inventory was filtered to package IDs F012, F013, F014,
F015, F016, and F018. The structural query returned `F012=12`, `F013=2`,
`F014=7`, `F015=5`, `F016=6`, `F018=6`, `total=38`,
`isolated=0`, `shareable_with_mock=38`, and `unclassified=0`. The inventory
contains 812 rows in total and records source commit
`ec194b5ab5d24803307b0cd8bb8895cb6d5ab9ee`; its status is
`story-004-clean-room-pass`. Those source-confirmed rows are the denominator
and are not reclassified from source hints.

### Diagnostic six-package runtime

Procedure, run once on the same base:

```text
go test ./tests/functional/factory/packaged/named_invocation ./tests/functional/factory/packaged/plan_execute ./tests/functional/factory/packaged/plan_parallel ./tests/functional/factory/packaged/quorum ./tests/functional/factory/packaged/ralph ./tests/functional/factory/packaged/subagent -count=1
```

Observed exit code `0` for every package:

| Package | Observed package result |
| --- | --- |
| `packaged/named_invocation` | `ok ... 23.678s` |
| `packaged/plan_execute` | `ok ... 2.499s` |
| `packaged/plan_parallel` | `ok ... 5.325s` |
| `packaged/quorum` | `ok ... 3.781s` |
| `packaged/ralph` | `ok ... 5.171s` |
| `packaged/subagent` | `ok ... 11.055s` |

These are shared-host diagnostics only. They impose no local wall-clock,
percentage, sample-count, variance, or regression threshold. The PR package
result remains the later performance authority.

## Current root and lifecycle topology

The functional executable spine is production `root.BuildProcess` through
`support.BuildProcessWithContext`, then `Process.Execute` with public CLI,
Factory Session, Work, Factory Event, recording, and replay observations. Only
external provider, filesystem, listener, and recorder effects are controlled
through `serviceedges.Edges`.

| Package | Current construction site(s) | Current process count | Session owner and expected count | Route/effect owner | Cleanup and residue observation |
| --- | --- | ---: | --- | --- | --- |
| `named_invocation` | Success fixture `shared_success_test.go:484` -> `:515`; preparation fixture `shared_failure_test.go:151` -> `:176`; cancellation `shared_failure_test.go:544` | **3** | One-shot CLI cases do not open a hosted explicit session; preparation observations assert session/work/dispatch/recording deltas are zero; cancellation is one explicit-file one-shot invocation | Success uses `namedInvocationProviderRouter` keyed by normalized working directory; preparation uses direct provider plus side-effect observation edges; cancellation uses `cancelingRootLookupFileSystem` | Each site registers `support.CleanupProcess`; success checks one build, zero listener starts, and zero routes; preparation and cancellation check one build, zero listener starts, and zero side-effect deltas |
| `plan_execute` | `shared_fixture_test.go:265` | **1** | `planExecuteExpectedSessions=1`; scenario opens a unique non-default Factory Session and closes it with public GET-404 verification | `planExecuteProviderCommandRouter` selects a scenario runner by Factory/work paths | Lifecycle ledger checks one API host start, session closed/absent, scenario root removed; fixture probes listener shutdown and shared-root removal |
| `plan_parallel` | `shared_fixture_test.go:266` | **1** | `planParallelExpectedSessions=6`; every scenario owns a unique non-default Factory Session | `planParallelProviderCommandRouter` uses normalized Factory/work path keys and scenario unregister cleanup | Lifecycle ledger checks one API host start, six unique/closed/absent sessions and roots; fixture probes listener shutdown and shared-root removal |
| `quorum` | `shared_fixture_test.go:265` | **1** | `packagedQuorumExpectedSessions=4`; every scenario owns a unique non-default Factory Session | `packagedQuorumProviderCommandRouter` routes branch/merge runners by Factory/work paths | Lifecycle ledger checks one API host start, four unique/closed/absent sessions and roots; fixture probes listener shutdown and shared-root removal |
| `ralph` | `shared_fixture_test.go:259` | **1** | `ralphExpectedSessions=6`; every scenario owns a unique non-default Factory Session | `ralphProviderCommandRouter` routes planner/iterator runners by Factory/work paths | Lifecycle ledger checks one API host start, six unique/closed/absent sessions and roots; fixture probes listener shutdown and shared-root removal |
| `subagent` | `shared_fixture_test.go:288` | **1** | `subagentExpectedSessions=7`; CLI scenarios are later accounted for by explicit non-default sessions; session IDs are unique and non-default | `subagentProviderCommandRouter` routes child runners by Factory/work paths; `subagentAPIServerStarter` counts listener requests for the hermetic no-server assertion | Lifecycle ledger checks one API host start, seven unique/closed/absent sessions and roots; listener counter distinguishes the continuous host from no-server invocation; fixture probes listener shutdown and shared-root removal |

The five already-shared fixtures register their post-process assertion before
`support.CleanupProcess`, so LIFO cleanup closes hosted commands and the
reusable process before listener/root assertions. Scenario cleanup closes the
public Factory Session, verifies it is absent, removes the scenario root, and
then unregisters its provider paths. These are current ownership facts, not a
claim that every failure path is already perfect after consolidation.

## Exact 38-row public-witness map

Every row below is `shareable-with-mock` in the tracked inventory. The selector
is the current top-level or named `t.Run` identity; nested dynamic assertions
are expanded in the next section. The source hash is the current SHA-256 of the
file containing the witness.

| ID | Package | Current selector / witness | Public assertion retained | Source / SHA-256 |
| --- | --- | --- | --- | --- |
| PKG-001 | F012 | `^TestNamedInvocationSharedPreparationFailures$` | Missing-required, reserved-name, and sensitive normalization preparation errors return stable codes with no Work, event, provider, listener, recording, or leaked value | `shared_failure_test.go:31` / `c5eb7563fd65ec78a3dfb12e168784c32093e0999facafd2714364b98b31c0c1` |
| PKG-002 | F012 | `^TestRun_EffectiveSchemaPreparationFailuresStopBeforeExecutionSideEffects$` | Effective-schema preparation stops before execution-side-effect deltas | `shared_failure_test.go:505` / `c5eb7563fd65ec78a3dfb12e168784c32093e0999facafd2714364b98b31c0c1` |
| PKG-003 | F012 | `^TestRun_EffectiveSchemaPreparationFailuresStopBeforeExecutionSideEffects/cancellation_during_explicit_file_root_lookup$` | Explicit-file lookup preserves `context.Canceled`, redaction, and zero provider/session/recording/listener effects | `shared_failure_test.go:510` / `c5eb7563fd65ec78a3dfb12e168784c32093e0999facafd2714364b98b31c0c1` |
| PKG-004 | F012 | `^TestNamedInvocationSharedSuccess$` | Shared builder, goal, subagent, compatibility, default, and recording journeys retain outputs, calls, arguments, and cleanup | `shared_success_test.go:27` / `bfd88233a2c305d15600a1b5d6dbd30d73675e1438496110b6315f41aad0a3d6` |
| PKG-005 | F012 | `^TestNamedInvocationSharedSuccess/factory builder list and help$` | Factory list/help discovers the packaged builder without provider execution | `shared_success_test.go:33` / `bfd88233a2c305d15600a1b5d6dbd30d73675e1438496110b6315f41aad0a3d6` |
| PKG-006 | F012 | `^TestNamedInvocationSharedSuccess/named goal$` | Named goal returns the provider result with one exact call and no listener | `shared_success_test.go:36` / `bfd88233a2c305d15600a1b5d6dbd30d73675e1438496110b6315f41aad0a3d6` |
| PKG-007 | F012 | `^TestNamedInvocationSharedSuccess/named subagent$` | Named subagent returns the child result and does not echo request text | `shared_success_test.go:39` / `bfd88233a2c305d15600a1b5d6dbd30d73675e1438496110b6315f41aad0a3d6` |
| PKG-008 | F012 | `^TestNamedInvocationSharedSuccess/no-signature compatibility$` | Positional, stdin, and signature-looking literal inputs retain named/file parity and six exact provider calls | `shared_success_test.go:42` / `bfd88233a2c305d15600a1b5d6dbd30d73675e1438496110b6315f41aad0a3d6` |
| PKG-009 | F012 | `^TestNamedInvocationSharedSuccess/effective signature parity$` | Named/file canonical arguments and resolved prompts match for positional, named, numeric, file, and stdin inputs | `shared_success_test.go:45` / `bfd88233a2c305d15600a1b5d6dbd30d73675e1438496110b6315f41aad0a3d6` |
| PKG-010 | F012 | `^TestNamedInvocationSharedSuccess/default-only input$` | Omitted defaulted input materializes identically for named/file invocation | `shared_success_test.go:48` / `bfd88233a2c305d15600a1b5d6dbd30d73675e1438496110b6315f41aad0a3d6` |
| PKG-011 | F012 | `^TestNamedInvocationSharedSuccess/recorded named invocation$` | Recording has one v2 header, ordered dispatch/result events, terminal Work, primary result, and finalized record | `shared_success_test.go:51` / `bfd88233a2c305d15600a1b5d6dbd30d73675e1438496110b6315f41aad0a3d6` |
| PKG-012 | F012 | `^TestNamedInvocationProviderRouterRejectsAmbiguousAndUnknownRoutes$` | Duplicate and unknown routes fail deterministically without consuming a runner | `shared_success_test.go:659` / `bfd88233a2c305d15600a1b5d6dbd30d73675e1438496110b6315f41aad0a3d6` |
| PKG-013 | F013 | `^TestPackagedPlanExecute$` | Shared plan-execute host preserves the one explicit-session package lifecycle | `invocation_test.go:29` / `a1a8d4d7b9afe0814f99d3e41941c661e66284d909ec0bafe2fc6282cf8d65cd` |
| PKG-014 | F013 | `^TestPackagedPlanExecute/TestPackagedPlanExecutePlansThenExecutesWithOperatorDefaults$` | Planner then executor calls use operator defaults, pass the PRD, complete, and create the artifact | `invocation_test.go:31` / `a1a8d4d7b9afe0814f99d3e41941c661e66284d909ec0bafe2fc6282cf8d65cd` |
| PKG-015 | F014 | `^TestPackagedPlanParallel$` | Shared parallel host retains result, error, count, ordering, replay, and cleanup journeys | `invocation_test.go:26` / `369c809dae52d80d8c435ca075f0c1541811d790b6e0cfaed50403954ef5ba6f` |
| PKG-016 | F014 | `^TestPackagedPlanParallel/TestPackagedPlanParallelMergerReceivesEveryUniqueCompletedChildResult$` | Ten generated children reach one merger once in deterministic order using results, not raw inputs | `invocation_test.go:28` / `369c809dae52d80d8c435ca075f0c1541811d790b6e0cfaed50403954ef5ba6f` |
| PKG-017 | F014 | `^TestPackagedPlanParallel/TestPackagedPlanParallelExecutesReadyDAGConcurrentlyAndMerges$` | Two ready executors overlap, five dispatches/replay events retain order, and the dependent DAG merges | `invocation_test.go:31` / `369c809dae52d80d8c435ca075f0c1541811d790b6e0cfaed50403954ef5ba6f` |
| PKG-018 | F014 | `^TestPackagedPlanParallel/TestPackagedPlanParallelExecutorEffortCanBeOverridden$` | Exact low-effort executor configuration reaches each task | `invocation_test.go:34` / `369c809dae52d80d8c435ca075f0c1541811d790b6e0cfaed50403954ef5ba6f` |
| PKG-019 | F014 | `^TestPackagedPlanParallel/TestPackagedPlanParallelRejectsUnsupportedEffortBeforeExecutorProviderExecution$` | Unsupported effort fails after planner validation with no executor call | `invocation_test.go:37` / `369c809dae52d80d8c435ca075f0c1541811d790b6e0cfaed50403954ef5ba6f` |
| PKG-020 | F014 | `^TestPackagedPlanParallel/TestPackagedPlanParallelRejectsPlannerBatchAboveCeilingAtomically$` | Above-ceiling plan rejects atomically with zero executor or merger work | `invocation_test.go:40` / `369c809dae52d80d8c435ca075f0c1541811d790b6e0cfaed50403954ef5ba6f` |
| PKG-021 | F014 | `^TestPackagedPlanParallel/TestPackagedPlanParallelChildFailureFansInWithoutMerge$` | One child failure produces a failed invocation, one terminal child attempt, and no merger | `invocation_test.go:43` / `369c809dae52d80d8c435ca075f0c1541811d790b6e0cfaed50403954ef5ba6f` |
| PKG-022 | F015 | `^TestPackagedQuorum$` | Shared quorum host retains required/default/parallel/failure behavior and cleanup | `invocation_test.go:11` / `805e004f9e892483ac2d638d18921680b84d188e1d428de3e4bdd6f618e79f07` |
| PKG-023 | F015 | `^TestPackagedQuorum/TestPackagedQuorumRequiredInputCompletes$` | Required request dispatches both members and merger once with a completed result | `invocation_test.go:13` / `805e004f9e892483ac2d638d18921680b84d188e1d428de3e4bdd6f618e79f07` |
| PKG-024 | F015 | `^TestPackagedQuorum/TestPackagedQuorumOptionalMemberSettingsReachWorkers$` | Member and merger provider/model overrides reach intended workers exactly | `invocation_test.go:16` / `805e004f9e892483ac2d638d18921680b84d188e1d428de3e4bdd6f618e79f07` |
| PKG-025 | F015 | `^TestPackagedQuorum/TestPackagedQuorumGatesMergeUntilBothBranchesComplete$` | Merger remains at zero until both gated branches complete, then runs once | `invocation_test.go:19` / `805e004f9e892483ac2d638d18921680b84d188e1d428de3e4bdd6f618e79f07` |
| PKG-026 | F015 | `^TestPackagedQuorum/TestPackagedQuorumInsufficientSuccessfulMembersFails$` | Below-capacity success fails without a false completed primary result or merger | `invocation_test.go:22` / `805e004f9e892483ac2d638d18921680b84d188e1d428de3e4bdd6f618e79f07` |
| PKG-027 | F016 | `^TestPackagedRalph$` | Shared Ralph host retains iteration, model, failure, bound, and cleanup behavior | `invocation_test.go:36` / `53473d62863a8334b0ea301baf769c73b25a83277dd9aee0b1441dbd2aacbddf` |
| PKG-028 | F016 | `^TestPackagedRalph/TestPackagedRalphPlansThenIteratesToCompletionThroughNamedCLI$` | Planner and two iterator visits preserve plan artifact, role order, terminal result, and completion token | `invocation_test.go:38` / `53473d62863a8334b0ea301baf769c73b25a83277dd9aee0b1441dbd2aacbddf` |
| PKG-029 | F016 | `^TestPackagedRalph/TestPackagedRalphUsesOperatorDefaultsWhenOptionalRoleParametersAreOmitted$` | Operator provider/model defaults reach planner and iterator roles | `invocation_test.go:41` / `53473d62863a8334b0ea301baf769c73b25a83277dd9aee0b1441dbd2aacbddf` |
| PKG-030 | F016 | `^TestPackagedRalph/TestPackagedRalphUsesConfiguredAndRoleOverrideModels$` | Configured role models apply and explicit planner/iterator flags take precedence | `invocation_test.go:44` / `53473d62863a8334b0ea301baf769c73b25a83277dd9aee0b1441dbd2aacbddf` |
| PKG-031 | F016 | `^TestPackagedRalph/TestPackagedRalphFailsOnIteratorWorkerFailure$` | Iterator failure is `FAILED`, primary result is nil, and no false completion occurs | `invocation_test.go:47` / `53473d62863a8334b0ea301baf769c73b25a83277dd9aee0b1441dbd2aacbddf` |
| PKG-032 | F016 | `^TestPackagedRalph/TestPackagedRalphFailsAfterBoundedIncompleteIterations$` | Incomplete iteration stops at the authored bound with no extra attempt | `invocation_test.go:50` / `53473d62863a8334b0ea301baf769c73b25a83277dd9aee0b1441dbd2aacbddf` |
| PKG-033 | F018 | `^TestPackagedSubagent$` | Shared subagent host retains CLI/API/result/event/effort/failure behavior | `invocation_test.go:27` / `84ea807d28c39eb18d69c7b3f90d1d57598e5a3a7b69b83da31334f61adc96a3` |
| PKG-034 | F018 | `^TestPackagedSubagent/TestPackagedSubagentReturnsChildResult$` | Child primary result is authoritative, request text is not echoed, and hermetic invocation starts no listener | `invocation_test.go:29` / `84ea807d28c39eb18d69c7b3f90d1d57598e5a3a7b69b83da31334f61adc96a3` |
| PKG-035 | F018 | `^TestPackagedSubagent/TestPackagedSubagentChildFailureReturnsStableFailure$` | CLI/API child failure is stable `FAILED`, has no primary result, and does not echo input | `invocation_test.go:32` / `84ea807d28c39eb18d69c7b3f90d1d57598e5a3a7b69b83da31334f61adc96a3` |
| PKG-036 | F018 | `^TestPackagedSubagent/TestPackagedSubagentStreamsChildResponseEvents$` | Response events carry the child result and terminal API result completes | `invocation_test.go:35` / `84ea807d28c39eb18d69c7b3f90d1d57598e5a3a7b69b83da31334f61adc96a3` |
| PKG-037 | F018 | `^TestPackagedSubagent/TestPackagedSubagentPropagatesLunaXHighReasoningEffort$` | Exact Luna model and `xhigh` reasoning configuration reach the child command | `invocation_test.go:38` / `84ea807d28c39eb18d69c7b3f90d1d57598e5a3a7b69b83da31334f61adc96a3` |
| PKG-038 | F018 | `^TestPackagedSubagent/TestPackagedSubagentOmittedReasoningEffortPreservesProviderDefault$` | Omitted effort preserves provider default and injects no effort config | `invocation_test.go:41` / `84ea807d28c39eb18d69c7b3f90d1d57598e5a3a7b69b83da31334f61adc96a3` |

## Dynamic nested assertions

These cases are nested assertions within the 38-row witnesses, not additional
denominator rows. They remain tied to the exact parent selector and source
body so a later consolidation cannot remove them while preserving only the
parent name.

| ID | Parent selector and dynamic case | Source body | Observable assertion |
| --- | --- | --- | --- |
| DYN-001 | `^TestNamedInvocationSharedPreparationFailures$`; `test.name=named/missing_required` | `named_invocation/shared_failure_test.go:117` | Missing-required-input code and zero execution-side-effect delta |
| DYN-002 | same parent; `test.name=file/missing_required` | `shared_failure_test.go:117` | File selection has the same stable missing-required code and zero delta |
| DYN-003 | same parent; `test.name=named/reserved_collision` | `shared_failure_test.go:117` | Named reserved long-name collision is reported before execution |
| DYN-004 | same parent; `test.name=file/reserved_collision` | `shared_failure_test.go:117` | Explicit-file reserved long-name collision is reported before execution |
| DYN-005 | same parent; `test.name=explicit/static_collision` | `shared_failure_test.go:117` | Static collision is reported without the sensitive positional value or side effects |
| DYN-006 | same parent; `test.name=explicit/sensitive_normalization_failure` | `shared_failure_test.go:117` | String-validation mismatch is reported without sensitive-value leakage |
| DYN-007 | `^TestNamedInvocationSharedSuccess/no-signature compatibility$`; `test.name=positional compatibility` | `shared_success_test.go:149-153` | Legacy positional input matches named/file output |
| DYN-008 | same parent; `test.name=stdin compatibility` | `shared_success_test.go:149-153` | Dash plus stdin input matches named/file output |
| DYN-009 | same parent; `test.name=signature-only syntax remains literal text` | `shared_success_test.go:149-153` | `--mode fast` remains literal compatibility input and matches named/file output |
| DYN-010 | `^TestPackagedRalph/TestPackagedRalphUsesConfiguredAndRoleOverrideModels$`; `test.name=installed worker configuration` | `ralph/invocation_test.go:155` | Authored planner/iterator model configuration is used for all three stages |
| DYN-011 | same parent; `test.name=explicit role flags` | `ralph/invocation_test.go:155` | Explicit planner/iterator provider and model flags override configuration/defaults |
| DYN-012 | `^TestPackagedSubagent/TestPackagedSubagentReturnsChildResult$`; `CLI JSON returns authoritative child primary result` | `subagent/invocation_test.go:47` | JSON completed response contains only the child primary result, not request text |
| DYN-013 | same parent; `hermetic named invocation succeeds without listening server` | `subagent/invocation_test.go:66` | Stdout is the child result, stderr is empty, and listener starts remain zero |
| DYN-014 | `^TestPackagedSubagent/TestPackagedSubagentChildFailureReturnsStableFailure$`; `CLI JSON returns stable failed terminal outcome` | `subagent/invocation_test.go:160` | Execute errors, stable failed JSON has no primary result, and one matching error response is emitted |
| DYN-015 | same parent; `API returns stable failed terminal outcome` | `subagent/invocation_test.go:183` | Explicit-session API failure is stable `FAILED` with no primary result |

## Source hash register

These are the pre-change hashes used to tie the row and topology claims to the
observed source. Hashes are not generated artifacts and no source file was
modified by this story.

| File | SHA-256 |
| --- | --- |
| `tests/functional/factory/packaged/named_invocation/named_invocation_test.go` | `327150c17797627c63cb40d9a6fcfa757c3a8b2588b4c0fa6c34388364c567f5` |
| `tests/functional/factory/packaged/named_invocation/shared_failure_test.go` | `c5eb7563fd65ec78a3dfb12e168784c32093e0999facafd2714364b98b31c0c1` |
| `tests/functional/factory/packaged/named_invocation/shared_success_test.go` | `bfd88233a2c305d15600a1b5d6dbd30d73675e1438496110b6315f41aad0a3d6` |
| `tests/functional/factory/packaged/plan_execute/invocation_test.go` | `a1a8d4d7b9afe0814f99d3e41941c661e66284d909ec0bafe2fc6282cf8d65cd` |
| `tests/functional/factory/packaged/plan_execute/shared_fixture_test.go` | `e933ea1d4061721b9d19c99d9fa8a91c38743f1aa7535f9b6b72718775cf2025` |
| `tests/functional/factory/packaged/plan_parallel/invocation_test.go` | `369c809dae52d80d8c435ca075f0c1541811d790b6e0cfaed50403954ef5ba6f` |
| `tests/functional/factory/packaged/plan_parallel/shared_fixture_test.go` | `c7a39ae3929e0f05135e3e41961d4e25906ad57b7e58788b259a8e67d018b6c2` |
| `tests/functional/factory/packaged/quorum/helpers_test.go` | `38128130e83c33add85321754ff03709b77b68d39982c96176d6f195041a8a8a` |
| `tests/functional/factory/packaged/quorum/invocation_test.go` | `805e004f9e892483ac2d638d18921680b84d188e1d428de3e4bdd6f618e79f07` |
| `tests/functional/factory/packaged/quorum/shared_fixture_test.go` | `c913414a3c81f156a5bd4007c4e49fd861a5de80af3bd8d064b036cbb38495ad` |
| `tests/functional/factory/packaged/ralph/invocation_test.go` | `53473d62863a8334b0ea301baf769c73b25a83277dd9aee0b1441dbd2aacbddf` |
| `tests/functional/factory/packaged/ralph/shared_fixture_test.go` | `db8aeb66b967484bea867c845a73aef01bbfc9f871f69f83c9bee215740e4d35` |
| `tests/functional/factory/packaged/subagent/invocation_test.go` | `84ea807d28c39eb18d69c7b3f90d1d57598e5a3a7b69b83da31334f61adc96a3` |
| `tests/functional/factory/packaged/subagent/shared_fixture_test.go` | `346b3ef11ebccd9e853aca5696870fdac68bad709972ba4cc97ddd727c88ec2d` |

## Evidence boundary and next gates

This story establishes the current 38-row denominator, public selector and
dynamic-case mapping, eight current construction sites, session/route/cleanup
ownership, and a passing diagnostic six-package runtime. It does not prove the
target six-root topology, post-consolidation cancellation/reuse, race safety,
cross-package delivered behavior, terminal PR CI, or PR performance direction.

The next bounded gate is `GATE-NAMED` / story
`functional-test-optimization-c11-packaged-plan-consensus-002`, which owns the
named-invocation consolidation and must retain the cancellation, redaction,
recording, route, and no-listener witnesses above. Five-package lifecycle
reconciliation is `GATE-CONSENSUS`; final integrated proof is
`GATE-PACKAGES`/`GATE-LOOPBACK`; PR timing and terminal CI are
`GATE-PR-CI` and review-owned.
