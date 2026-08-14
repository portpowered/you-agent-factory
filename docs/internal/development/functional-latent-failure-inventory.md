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
and 8 package-summary events. The named events produce 36 distinct
package/test pairs across 8 packages. Nested subtests remain separate rows;
their slash-delimited identities are preserved exactly as emitted. The
quarantine schema currently accepts only top-level test selectors, so later
quarantine entries must use the corresponding top-level name while retaining
these nested identities as evidence.

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
| `github.com/portpowered/infinite-you/tests/functional/providers` | `TestMockWorkers_ServiceCommandRunnerCompletesModelAndScriptWorkers` |
| `github.com/portpowered/infinite-you/tests/functional/providers` | `TestMockWorkers_ServiceCommandRunnerCompletesModelAndScriptWorkers/model_worker` |
| `github.com/portpowered/infinite-you/tests/functional/providers` | `TestMockWorkers_ServiceCommandRunnerCompletesModelAndScriptWorkers/script_worker` |
| `github.com/portpowered/infinite-you/tests/functional/providers/acp` | `TestPackagedTournamentRunsCompetitorsAndJudgeThroughPersistentACPStdio` |
| `github.com/portpowered/infinite-you/tests/functional/runtime_api` | `TestAPIUnifiedEventLogSmoke_LiveRecordReplayProjectionAndDivergenceUseSameTimeline` |
| `github.com/portpowered/infinite-you/tests/functional/runtime_api` | `TestDashboard_EngineStateSnapshot_EndToEnd` |
| `github.com/portpowered/infinite-you/tests/functional/runtime_api` | `TestInferenceEvents_ModelProviderAttemptsRecordInCanonicalHistoryAndArtifact` |
| `github.com/portpowered/infinite-you/tests/functional/runtime_api` | `TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases` |
| `github.com/portpowered/infinite-you/tests/functional/runtime_api` | `TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases/canonical_split_factory_stays_aligned_across_flatten_replay_and_execution` |
| `github.com/portpowered/infinite-you/tests/functional/runtime_api` | `TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases/split_worker_frontmatter_rejects_retired_model_provider_alias` |
| `github.com/portpowered/infinite-you/tests/functional/runtime_api` | `TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases/split_workstation_frontmatter_rejects_retired_cron_trigger_at_start_alias` |
| `github.com/portpowered/infinite-you/tests/functional/runtime_api` | `TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases/split_workstation_frontmatter_rejects_retired_runtime_type_alias` |

**Authoritative count: 36 distinct package/test pairs.**
