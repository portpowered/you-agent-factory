# Functional latent-failure inventory

This inventory records the authoritative failure set for the repaired real
functional gate before parent-versus-tip classification. It is intentionally
not a quarantine decision: comparison evidence and ownership are recorded in
later updates to this file.

## Source and extraction

- Workflow run: [31831258532](https://github.com/portpowered/you-agent-factory/actions/runs/31831258532)
- Job: [94867390893 — Backend Functional Coverage](https://github.com/portpowered/you-agent-factory/actions/runs/31831258532/job/94867390893)
- Main SHA: `ed502b78182b9b88c282f84505704e09c814b653`
- Source command: `gh api repos/portpowered/you-agent-factory/actions/jobs/94867390893/logs`
- Extraction command (PowerShell; package-summary events are excluded by the
  non-empty `Test` predicate):

  ```powershell
  $events = gh api repos/portpowered/you-agent-factory/actions/jobs/94867390893/logs | ForEach-Object { $line = $_; $start = $line.IndexOf('{'); if ($start -ge 0) { try { $line.Substring($start) | ConvertFrom-Json } catch { } } }; @($events | Where-Object { $_.Action -eq 'fail' -and $_.Test } | Select-Object Package,Test | Sort-Object Package,Test -Unique | ForEach-Object { '{0} | {1}' -f $_.Package,$_.Test })
  ```

The raw structured reconciliation is 44 `fail` events: 36 named test events
and 8 package-summary events. The named events initially produced 36 distinct
package/test pairs across 8 packages. The alignment selector and the
service-command-runner selector have since passed; their parent and nested
rows were removed from the active inventory, leaving 28 distinct package/test
pairs. Nested subtests remain separate rows; their slash-delimited identities
are preserved exactly as emitted. The quarantine schema currently accepts only
top-level test selectors, so later quarantine entries must use the
corresponding top-level name while retaining these nested identities as
evidence.

## Distinct authoritative package/test pairs

| Package | Structured test identity |
| --- | --- |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestACPServeCommandStreamsUsageUpdateThroughRootBuildProcess` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestACPSessionAnswersEachTurnWithThatTurnsOwnResult` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestACPWorkerChildStreamSurvivesRetainedReplay` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestFactoryBuilderGreetsOnAVagueFirstACPTurn` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestJavaScriptFactoryChildrenAreVisibleAsWorkers` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestPackagedFactoriesCompleteOneACPPromptTurn` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestPackagedFactoriesCompleteOneACPPromptTurn/classify` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestPackagedFactoriesCompleteOneACPPromptTurn/goal` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestPackagedFactoriesCompleteOneACPPromptTurn/loop` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestPackagedJavaScriptFactoryCompletesOneACPPromptTurn` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestPackagedJavaScriptFactoryWithStructuredResultStreamsItsResult` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestPackagedPlanParallelCompletesOneACPPromptTurn` |
| `github.com/portpowered/infinite-you/tests/functional/chat_sessions/root_composition` | `TestTwoACPWorkersKeepChildStreamsAttributed` |
| `github.com/portpowered/infinite-you/tests/functional/cli/named_invocation` | `TestRun_EmptyEffectiveSignatureInputUsesSchemaBeforeExecution` |
| `github.com/portpowered/infinite-you/tests/functional/cli/named_invocation` | `TestRun_EmptyEffectiveSignatureInputUsesSchemaBeforeExecution/file_default-only` |
| `github.com/portpowered/infinite-you/tests/functional/cli/named_invocation` | `TestRun_EmptyEffectiveSignatureInputUsesSchemaBeforeExecution/named_default-only` |
| `github.com/portpowered/infinite-you/tests/functional/cli/named_invocation` | `TestRun_NamedAndExplicitFactorySelectionsExecuteEquivalentEffectiveSignatureInput` |
| `github.com/portpowered/infinite-you/tests/functional/cli/named_invocation` | `TestRun_NamedAndExplicitNoSignatureFactoriesPreserveCompatibilityInputs` |
| `github.com/portpowered/infinite-you/tests/functional/cli/named_invocation` | `TestRun_NamedAndExplicitNoSignatureFactoriesPreserveCompatibilityInputs/signature-only_syntax_remains_literal_text` |
| `github.com/portpowered/infinite-you/tests/functional/factory/packaged/cross` | `TestPackagedFactoryInvokedByCLICanBeInspectedByAPI` |
| `github.com/portpowered/infinite-you/tests/functional/observability/verification` | `TestFunctionalTestVizLaneScriptSmoke_TimesOutAndRetainsDiagnostics` |
| `github.com/portpowered/infinite-you/tests/functional/product/packaged_factory_portability` | `TestPackagedFactoryInitMaterialization_InvokesOutsideRepositoryWithBootstrapParity` |
| `github.com/portpowered/infinite-you/tests/functional/providers` | `TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials` |
| `github.com/portpowered/infinite-you/tests/functional/providers/acp` | `TestPackagedTournamentRunsCompetitorsAndJudgeThroughPersistentACPStdio` |
| `github.com/portpowered/infinite-you/tests/functional/runtime_api` | `TestAPIUnifiedEventLogSmoke_LiveRecordReplayProjectionAndDivergenceUseSameTimeline` |
| `github.com/portpowered/infinite-you/tests/functional/runtime_api` | `TestDashboard_EngineStateSnapshot_EndToEnd` |
| `github.com/portpowered/infinite-you/tests/functional/runtime_api` | `TestInferenceEvents_ModelProviderAttemptsRecordInCanonicalHistoryAndArtifact` |

**Active inventory count: 28 distinct package/test pairs.**

## Parent-versus-tip comparison

The authoritative rows were replayed independently, one exact selector per
command, in separate disposable detached worktrees. The comparison machine
was Windows/amd64 with Go `go1.25.0`; both commits used the same machine,
environment, and command shape. The full push-tier behavior was selected with
`-short=false`, and fresh execution was forced with `-count=1`.

- Parent worktree: `C:\Users\andre\AppData\Local\Temp\infinite-you-latent-triage-20260814\parent-ed502b781` at `ed502b78182b9b88c282f84505704e09c814b653`
- Tip worktree: `C:\Users\andre\AppData\Local\Temp\infinite-you-latent-triage-20260814\tip-db7379d47` at `db7379d47039ae1e165ba401aa8f9712272596f5`
- Worktree setup commands: `git worktree add --detach <parent-worktree> ed502b78182b9b88c282f84505704e09c814b653` and `git worktree add --detach <tip-worktree> db7379d47039ae1e165ba401aa8f9712272596f5`
- Per-row command pattern: `go test -json -count=1 -short=false -timeout=15m -run ^<exact-structured-test-identity>$ <package>`
- Every row below records the wall-clock duration of that exact command. Nested identities were run independently, not inferred from their parent result.

The comparison table's package column is module-relative; its full import path
is `github.com/portpowered/infinite-you/` followed by the displayed value.

The two historical passes had identical classifications: 21 failures at both
commits, 14 passes at both commits, and one setup-incomparable skip at both
commits. There was no test that failed at `ed502b781` and passed at
`db7379d47`; no genuine tip-only regression was found. This lane repaired five
alignment rows that had failed in that historical comparison, leaving 16
same-machine failures as confirmed pre-existing candidates for the next
quarantine story. The 14 CI failures that passed when isolated on both commits
remain ambiguous/non-reproduced and must not be quarantined. The observability
row is setup-incomparable because its Linux-only test files are not runnable on
this Windows machine and must not be quarantined from this comparison.
same-machine failures as confirmed pre-existing candidates for the next
quarantine story. The service-command-runner parent and two nested rows are
also repaired in this lane, leaving 13 same-machine failures as confirmed
pre-existing candidates for the next quarantine story. The 14 CI failures that
passed when isolated on both commits remain ambiguous/non-reproduced and must
not be quarantined. The observability row is setup-incomparable because its
Linux-only test files are not runnable on this Windows machine and must not be
quarantined from this comparison.

| # | Package | Test identity | `ed502b781` | `db7379d47` | Verdict | Failure or comparison evidence |
| ---: | --- | --- | --- | --- | --- | --- |
| 1 | `tests/functional/chat_sessions/root_composition` | `TestACPServeCommandStreamsThroughRootBuildProcessWithoutDuplicateFinalText` | PASS (36.69s) | PASS (5.27s) | AMBIGUOUS | Authoritative CI failure was not reproduced by either isolated replay. |
| 2 | `tests/functional/chat_sessions/root_composition` | `TestACPServeCommandStreamsUsageUpdateThroughRootBuildProcess` | PASS (4.96s) | PASS (4.78s) | AMBIGUOUS | Authoritative CI recorded a canonical response progress-publication failure for an `UPDATED` phase; isolated replays passed at both commits. |
| 3 | `tests/functional/chat_sessions/root_composition` | `TestACPSessionAnswersEachTurnWithThatTurnsOwnResult` | PASS (3.75s) | PASS (5.28s) | AMBIGUOUS | Authoritative CI reported shutdown `context canceled`; isolated replays passed at both commits. |
| 4 | `tests/functional/chat_sessions/root_composition` | `TestACPWorkerChildStreamSurvivesRetainedReplay` | FAIL (4.92s) | FAIL (6.43s) | CONFIRMED PRE-EXISTING | Both replays logged `CANONICAL_EVENT_PUBLISH_FAILED` because `UPDATED` is invalid for `RUN`/`MESSAGE` progress phases. |
| 5 | `tests/functional/chat_sessions/root_composition` | `TestFactoryBuilderGreetsOnAVagueFirstACPTurn` | PASS (3.10s) | PASS (4.44s) | AMBIGUOUS | Authoritative CI observed streamed text `help` instead of Factory Builder guidance and a shutdown cancellation; isolated replays passed at both commits. |
| 6 | `tests/functional/chat_sessions/root_composition` | `TestJavaScriptFactoryChildrenAreVisibleAsWorkers` | PASS (6.54s) | PASS (4.43s) | AMBIGUOUS | Authoritative CI observed child-task JSON instead of the merged workflow result; isolated replays passed at both commits. |
| 7 | `tests/functional/chat_sessions/root_composition` | `TestPackagedFactoriesCompleteOneACPPromptTurn` | PASS (11.86s) | PASS (7.11s) | AMBIGUOUS | Authoritative CI parent failure was not reproduced by either isolated replay. |
| 8 | `tests/functional/chat_sessions/root_composition` | `TestPackagedFactoriesCompleteOneACPPromptTurn/classify` | PASS (3.92s) | PASS (4.42s) | AMBIGUOUS | Authoritative CI streamed `small`, expected text containing `classified answer over ACP`; isolated replays passed at both commits. |
| 9 | `tests/functional/chat_sessions/root_composition` | `TestPackagedFactoriesCompleteOneACPPromptTurn/goal` | PASS (3.98s) | PASS (4.54s) | AMBIGUOUS | Authoritative CI reported shutdown `context canceled`; isolated replays passed at both commits. |
| 10 | `tests/functional/chat_sessions/root_composition` | `TestPackagedFactoriesCompleteOneACPPromptTurn/loop` | PASS (3.49s) | PASS (5.39s) | AMBIGUOUS | Authoritative CI reported shutdown `context canceled`; isolated replays passed at both commits. |
| 11 | `tests/functional/chat_sessions/root_composition` | `TestPackagedJavaScriptFactoryCompletesOneACPPromptTurn` | PASS (2.96s) | PASS (5.84s) | AMBIGUOUS | Authoritative CI reported shutdown `context canceled`; isolated replays passed at both commits. |
| 12 | `tests/functional/chat_sessions/root_composition` | `TestPackagedJavaScriptFactoryWithStructuredResultStreamsItsResult` | PASS (2.84s) | PASS (4.82s) | AMBIGUOUS | Authoritative CI reported shutdown `context canceled`; isolated replays passed at both commits. |
| 13 | `tests/functional/chat_sessions/root_composition` | `TestPackagedPlanParallelCompletesOneACPPromptTurn` | PASS (3.32s) | PASS (5.80s) | AMBIGUOUS | Authoritative CI reported shutdown `context canceled`; isolated replays passed at both commits. |
| 14 | `tests/functional/chat_sessions/root_composition` | `TestTwoACPWorkersKeepChildStreamsAttributed` | FAIL (9.83s) | FAIL (6.53s) | CONFIRMED PRE-EXISTING | Both replays logged `CANONICAL_EVENT_PUBLISH_FAILED` because `UPDATED` is invalid for `RUN`/`MESSAGE` progress phases. |
| 15 | `tests/functional/cli/named_invocation` | `TestRun_EmptyEffectiveSignatureInputUsesSchemaBeforeExecution` | FAIL (19.37s) | FAIL (10.66s) | CONFIRMED PRE-EXISTING | Parent failure reproduced at both commits; its named/file variants report unknown `${executorProvider}` selection. |
| 16 | `tests/functional/cli/named_invocation` | `TestRun_EmptyEffectiveSignatureInputUsesSchemaBeforeExecution/file_default-only` | FAIL (4.83s) | FAIL (4.54s) | CONFIRMED PRE-EXISTING | `RUN_INVOCATION_FAILED`: provider `${executorProvider}` is unknown. |
| 17 | `tests/functional/cli/named_invocation` | `TestRun_EmptyEffectiveSignatureInputUsesSchemaBeforeExecution/named_default-only` | FAIL (7.41s) | FAIL (4.58s) | CONFIRMED PRE-EXISTING | `RUN_INVOCATION_FAILED`: provider `${executorProvider}` is unknown. |
| 18 | `tests/functional/cli/named_invocation` | `TestRun_NamedAndExplicitFactorySelectionsExecuteEquivalentEffectiveSignatureInput` | FAIL (5.27s) | FAIL (3.93s) | CONFIRMED PRE-EXISTING | `RUN_INVOCATION_FAILED`: provider `${executorProvider}` is unknown. |
| 19 | `tests/functional/cli/named_invocation` | `TestRun_NamedAndExplicitNoSignatureFactoriesPreserveCompatibilityInputs` | FAIL (8.99s) | FAIL (5.67s) | CONFIRMED PRE-EXISTING | Parent compatibility-input failure reproduced at both commits; its nested selector reports the unknown `mode` argument. |
| 20 | `tests/functional/cli/named_invocation` | `TestRun_NamedAndExplicitNoSignatureFactoriesPreserveCompatibilityInputs/signature-only_syntax_remains_literal_text` | FAIL (5.87s) | FAIL (3.98s) | CONFIRMED PRE-EXISTING | `INVOCATION_ARGUMENT_UNKNOWN_ARGUMENT`: unknown named argument `mode` for `@you/goal`. |
| 21 | `tests/functional/factory/packaged/cross` | `TestPackagedFactoryInvokedByCLICanBeInspectedByAPI` | PASS (4.93s) | PASS (4.32s) | AMBIGUOUS | Authoritative CI never observed a live session/status during CLI invocation; isolated replays passed at both commits. |
| 22 | `tests/functional/observability/verification` | `TestFunctionalTestVizLaneScriptSmoke_TimesOutAndRetainsDiagnostics` | SETUP-SKIP (0.57s) | SETUP-SKIP (0.49s) | SETUP-INCOMPARABLE | The Linux test files are absent under Windows build constraints (`[no test files]`); the authoritative Linux assertion was not comparable. |
| 23 | `tests/functional/product/packaged_factory_portability` | `TestPackagedFactoryInitMaterialization_InvokesOutsideRepositoryWithBootstrapParity` | FAIL (20.40s) | FAIL (18.68s) | CONFIRMED PRE-EXISTING | Both replays timed out waiting for the process API server and reported missing invocation input during shutdown. |
| 24 | `tests/functional/providers` | `TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials` | FAIL (6.97s) | FAIL (4.31s) | CONFIRMED PRE-EXISTING | Same-machine rejection/process failure reproduced at both commits; authoritative CI also failed the worker-session start path. |
| 25 | `tests/functional/providers/acp` | `TestPackagedTournamentRunsCompetitorsAndJudgeThroughPersistentACPStdio` | PASS (10.71s) | PASS (5.47s) | AMBIGUOUS | Authoritative CI emitted a failure with only progress-publication warnings; isolated replays passed at both commits. |
| 26 | `tests/functional/runtime_api` | `TestAPIUnifiedEventLogSmoke_LiveRecordReplayProjectionAndDivergenceUseSameTimeline` | FAIL (22.50s) | FAIL (4.33s) | CONFIRMED PRE-EXISTING | `/events` replay failed with `unexpected EOF` at both commits. |
| 27 | `tests/functional/runtime_api` | `TestDashboard_EngineStateSnapshot_EndToEnd` | FAIL (7.01s) | FAIL (3.99s) | CONFIRMED PRE-EXISTING | Provider sessions were absent from events; `sess-world-view-success` was missing at both commits. |
| 28 | `tests/functional/runtime_api` | `TestInferenceEvents_ModelProviderAttemptsRecordInCanonicalHistoryAndArtifact` | FAIL (5.34s) | FAIL (4.61s) | CONFIRMED PRE-EXISTING | Event order stopped before the expected dispatch-created/inference-request/inference-response/dispatch-completed sequence at both commits. |

The exact command for each row is the per-row command pattern above with the
row's full test identity and package substituted verbatim. The authoritative
CI source remains run `31831258532`, job `94867390893`; local replays classify
parent-versus-tip behavior and do not replace that source failure set.

## Quarantine mapping

The remaining 13 confirmed pre-existing failure rows map to 10 top-level selectors
because the quarantine schema intentionally matches top-level Go tests. A
parent selector covers its independently recorded nested subtests; no broader
package selector is used. Every entry below is `GENUINELY FAILING`, names the
observed failure, and records reproduction at both `ed502b781` and
`db7379d47` in `tests/functional/functional-quarantine.json`.

| Package | Quarantined top-level selector | Evidence rows | Observed failure | Owning area |
| --- | --- | --- | --- | --- |
| `tests/functional/chat_sessions/root_composition` | `TestACPWorkerChildStreamSurvivesRetainedReplay` | 4 | `CANONICAL_EVENT_PUBLISH_FAILED`: `UPDATED` is invalid for `RUN`/`MESSAGE` progress phases | Chat Sessions / ACP stream publication |
| `tests/functional/chat_sessions/root_composition` | `TestTwoACPWorkersKeepChildStreamsAttributed` | 14 | `CANONICAL_EVENT_PUBLISH_FAILED`: `UPDATED` is invalid for `RUN`/`MESSAGE` progress phases | Chat Sessions / ACP stream publication |
| `tests/functional/cli/named_invocation` | `TestRun_EmptyEffectiveSignatureInputUsesSchemaBeforeExecution` | 15–17 | `RUN_INVOCATION_FAILED`: provider `${executorProvider}` is unknown | CLI named invocation / provider selection |
| `tests/functional/cli/named_invocation` | `TestRun_NamedAndExplicitFactorySelectionsExecuteEquivalentEffectiveSignatureInput` | 18 | `RUN_INVOCATION_FAILED`: provider `${executorProvider}` is unknown | CLI named invocation / provider selection |
| `tests/functional/cli/named_invocation` | `TestRun_NamedAndExplicitNoSignatureFactoriesPreserveCompatibilityInputs` | 19–20 | `INVOCATION_ARGUMENT_UNKNOWN_ARGUMENT`: `mode` is unknown for `@you/goal` | CLI named invocation / compatibility inputs |
| `tests/functional/product/packaged_factory_portability` | `TestPackagedFactoryInitMaterialization_InvokesOutsideRepositoryWithBootstrapParity` | 23 | Process API server readiness timeout and missing invocation input during shutdown | Packaged factory portability / bootstrap |
| `tests/functional/providers` | `TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials` | 24 | Same-machine rejection/process failure and worker-session start failure | Providers / mock-worker startup |
| `tests/functional/runtime_api` | `TestAPIUnifiedEventLogSmoke_LiveRecordReplayProjectionAndDivergenceUseSameTimeline` | 26 | `/events` replay failed with `unexpected EOF` | Runtime API / event replay framing |
| `tests/functional/runtime_api` | `TestDashboard_EngineStateSnapshot_EndToEnd` | 27 | Provider sessions were absent from events; `sess-world-view-success` was missing | Runtime API / provider-session projection |
| `tests/functional/runtime_api` | `TestInferenceEvents_ModelProviderAttemptsRecordInCanonicalHistoryAndArtifact` | 28 | Canonical event order stopped before the expected dispatch/inference sequence | Runtime API / inference event emission |

The 14 ambiguous/non-reproduced rows (1–3, 5–13, 21, and 25) and the one
setup-incomparable Windows row (22) remain unquarantined. No failure that
passed at `ed502b781` and failed at `db7379d47`, or otherwise indicated a
tip-only regression, was found.

The runtime-config alignment selector changed from observed-fail to
observed-pass. Its quarantine entry and the five corresponding inventory rows
were removed after the exact selector passed. The service-command-runner
selector also changed from observed-fail to observed-pass; its quarantine entry
and three corresponding inventory rows were removed after the exact selector
passed.
