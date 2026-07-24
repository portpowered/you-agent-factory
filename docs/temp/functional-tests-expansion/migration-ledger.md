# Existing Functional Test Migration Ledger

**Status:** planning-only (Wave 0 / FND-007)  
**Authority:** this file is the Wave 0 migration authority for source→destination
mapping of current customer functional scenarios.

## Planning-only notice

This ledger is a **planning artifact**. It does **not** move, rename, split, or
delete functional test files. Later move batches consume the row mappings and
named deletion-only batch ids recorded here. Makefile specialty-target
retargets also belong to those later batches; this ledger only records current
bindings and intended post-move package/path targets.

Destination topology remains owned by
[`test-file-checklist.md`](test-file-checklist.md). Ownership rules remain
owned by [`plan.md`](plan.md).

## Row schema

Every customer scenario row uses the same required fields. Later mapping
stories append rows; they do not invent alternate column sets.

| Field | Required | Meaning |
| --- | --- | --- |
| `source_path` | yes | Current `_test.go` path relative to repo root |
| `package` | yes | Current Go package import path under `tests/functional/...` |
| `scenario` | yes | Top-level `Test*` function name |
| `lane` | yes | `short` or `functionallong` (from `//go:build` / package convention) |
| `destination` | yes* | Exact checklist cell path from `test-file-checklist.md`, **or** an approved wrong-layer rationale |
| `catch_all` | yes | Named catch-all owner if any: `runtime_api`, `smoke`, `workflow`, `guards_batch`, `bootstrap_portability`, `replay_contracts`, or `none` |
| `specialty_targets` | yes | Make target names that currently select this scenario or its package; use `none` when unbound |
| `deletion_only_batch` | yes* | Named batch id for later independent move work when applicable; use `n/a` when not part of a deletion-only batch |

\* During schema publication, inventory-only rows may leave `destination` and
`deletion_only_batch` as `TBD` until the matching mapping story fills them.
After FND-007 completes, no customer row may remain `TBD`.

### Destination values

Use exactly one of:

1. **Checklist cell path** — a path that already appears as a checkbox cell in
   `test-file-checklist.md` (do not invent destinations).
2. **Wrong-layer rationale** — `wrong-layer: <layer> — <why>` where `<layer>`
   names the correct proof layer (for example `unit`, `package-integration`,
   `stress/race`, `ui-browser`, `contract-smoke-outside-functional`) and `<why>`
   states why the scenario must not remain a customer functional owner. When
   replacement evidence already exists elsewhere, name that owner.

### Catch-all owners

| Owner | Later handling |
| --- | --- |
| `runtime_api` | Deletion-only batches until package ownership reaches zero |
| `smoke` | Split by durable domain owner; featureless package must reach zero |
| `workflow` | Split by durable domain owner; featureless package must reach zero |
| `guards_batch` | Split (example durable owners: `guards`, `resources`, `resilience`) |
| `bootstrap_portability` | Split (example durable owner: `factory/portability`) |
| `replay_contracts` | Split (example durable owner: `events/replay`) |
| `none` | Already under a durable or remaining non-catch-all package; still map |

### Lane membership

| Value | Rule |
| --- | --- |
| `functionallong` | Source file has `//go:build functionallong` (or equivalent long-only constraint) |
| `short` | Default / `!functionallong` / no long-only constraint |

### Markdown row template

Use this table shape in every inventory section:

```markdown
| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/<pkg>/<file>_test.go | you-agent-factory/tests/functional/<pkg> | TestExample | short | tests/functional/<domain>/.../<file>_test.go | none | none | n/a |
```

### Machine-readable companion (optional)

A JSON/YAML companion in this directory may mirror the same fields for later
batch tooling. If present, the Markdown ledger remains the human review
authority; the companion must not diverge from these required fields.

## Document sections (filled by later stories)

| Section | Owning story | Purpose |
| --- | --- | --- |
| [Inventory](#inventory) | FND-007-002 | Complete customer `Test*` inventory + exclusion list (**done**: 503 scenarios) |
| [runtime_api deletion-only batches](#runtime_api-deletion-only-batches) | FND-007-003 | Map + batch every `runtime_api` scenario |
| [smoke and workflow split plans](#smoke-and-workflow-split-plans) | FND-007-004 | Explicit move/split plans |
| [guards_batch, bootstrap_portability, and replay_contracts split plans](#guards_batch-bootstrap_portability-and-replay_contracts-split-plans) | FND-007-005 | Explicit move/split plans |
| [Remaining packages and wrong-layer approvals](#remaining-packages-and-wrong-layer-approvals) | FND-007-006 | Map non-catch-all packages |
| [Specialty Make target bindings](#specialty-make-target-bindings) | FND-007-003…007 | Current vs intended post-move bindings |
| [Deletion-only batch index](#deletion-only-batch-index) | FND-007-003…007 | Ordered batch ids for later move work |
| [Completeness audit](#completeness-audit) | FND-007-007 | Zero-unmapped proof against live tree + checklist |

---

## Inventory

_Status: complete — FND-007-002. Fresh scan at `2026-07-24T02:17:19Z` (UTC)._

Customer inventory covers every top-level `Test*` under `tests/functional/`
except `TestMain`, helper-only files with no customer `Test*`, and
`tests/functional/internal/**` (listed under exclusions).

Machine-readable companion (same required fields):
[`migration-ledger-inventory.json`](migration-ledger-inventory.json).
The Markdown tables below remain the human review authority for this
inventory story; later mapping stories update `destination` /
`deletion_only_batch` / `specialty_targets` in both artifacts together.

### Inventory summary

| Measure | Count |
| --- | ---: |
| Customer top-level `Test*` scenarios | 503 |
| Files with ≥1 customer `Test*` | 185 |
| Functional `_test.go` files walked | 224 |
| Lane `short` | 381 |
| Lane `functionallong` | 122 |
| Helper-only / non-customer harness files excluded | 34 |
| `tests/functional/internal/**` harness `_test.go` files excluded | 5 |
| Internal harness top-level `Test*` (not customer inventory) | 15 |

### Counts by top-level package

| Top-level package | Customer scenarios | Catch-all owner |
| --- | ---: | --- |
| `acceptance` | 16 | `none` |
| `bootstrap_portability` | 34 | `bootstrap_portability` |
| `cli` | 56 | `none` |
| `config_init` | 8 | `none` |
| `guards_batch` | 28 | `guards_batch` |
| `models` | 1 | `none` |
| `operator_settings` | 1 | `none` |
| `providers` | 46 | `none` |
| `replay_contracts` | 24 | `replay_contracts` |
| `runtime_api` | 108 | `runtime_api` |
| `sessionparity` | 13 | `none` |
| `smoke` | 94 | `smoke` |
| `work` | 1 | `none` |
| `workflow` | 73 | `workflow` |
| **Total** | **503** | |

### Counts by catch-all owner

| catch_all | Customer scenarios |
| --- | ---: |
| `bootstrap_portability` | 34 |
| `guards_batch` | 28 |
| `none` | 142 |
| `replay_contracts` | 24 |
| `runtime_api` | 108 |
| `smoke` | 94 |
| `workflow` | 73 |

### Customer scenario rows

`destination` and `deletion_only_batch` are `TBD` until mapping stories
(FND-007-003…006) fill them. `specialty_targets` is `none` until specialty
binding stories record Make selectors (may be updated per package later).

#### `acceptance` (16 scenarios, catch_all=`none`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/acceptance/harness_smoke_test.go | you-agent-factory/tests/functional/acceptance | TestBuiltCLIHarness_IsolatesHomeAndLogDirectoriesAcrossSessions | short | TBD | none | none | TBD |
| tests/functional/acceptance/harness_smoke_test.go | you-agent-factory/tests/functional/acceptance | TestBuiltCLIHarness_NonZeroExitIncludesDiagnostics | short | TBD | none | none | TBD |
| tests/functional/acceptance/harness_smoke_test.go | you-agent-factory/tests/functional/acceptance | TestBuiltCLIHarness_WithNoExternalServerReservesUnusedPort | short | TBD | none | none | TBD |
| tests/functional/acceptance/install_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestFreshInstall_EmptyHomeProducesDocumentedCustomerOutcome | short | TBD | none | none | TBD |
| tests/functional/acceptance/install_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestMigratedInstall_ExistingConfigIsPreservedWithoutRewrite | short | TBD | none | none | TBD |
| tests/functional/acceptance/install_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestMigratedInstall_JSONReportsSkippedAndCreatedOutcomes | short | TBD | none | none | TBD |
| tests/functional/acceptance/install_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestMigratedInstall_MaterializesMissingPackagedDefaultsWithoutCorruption | short | TBD | none | none | TBD |
| tests/functional/acceptance/invalid_quiet_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestInvalidGoal_InvalidTopology_RejectsWithDocumentedGraphReferenceError | short | TBD | none | none | TBD |
| tests/functional/acceptance/invalid_quiet_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestInvalidGoal_OutputModesExitNonZero | short | TBD | none | none | TBD |
| tests/functional/acceptance/invoke_repeat_subagent_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestGoalRepeat_RepeatedNamedRunsAssignDistinctInvocationIdentityAndReuseInstalledCopy | short | TBD | none | none | TBD |
| tests/functional/acceptance/invoke_repeat_subagent_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestLocalModelInvoke_MissingReadiness_FailsWithDocumentedBootstrapGuidance | short | TBD | none | none | TBD |
| tests/functional/acceptance/invoke_repeat_subagent_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestSubagentInvocation_SuccessfulNamedRun_ReturnsAuthoritativePrimaryResultJSON | short | TBD | none | none | TBD |
| tests/functional/acceptance/output_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestInvocationOutput_TerminalFailureExitsNonZero | short | TBD | none | none | TBD |
| tests/functional/acceptance/provider_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestProviderPosture_Absent_UnresolvedDefaultRejectsWithDocumentedGuidance | short | TBD | none | none | TBD |
| tests/functional/acceptance/provider_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestProviderPosture_Configured_ExplicitHomeConfigEnablesNamedGoalSuccessPath | short | TBD | none | none | TBD |
| tests/functional/acceptance/provider_outcomes_test.go | you-agent-factory/tests/functional/acceptance | TestProviderPosture_Discovered_EnvDefaultResolvesWithoutFileProvider | short | TBD | none | none | TBD |

#### `bootstrap_portability` (34 scenarios, catch_all=`bootstrap_portability`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/bootstrap_portability/agent_factory_export_import_fixture_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestAgentFactoryExportImportFixture_AuthoredLayoutInterpolatesProjectSpecificPromptContent | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/agent_factory_export_import_fixture_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestAgentFactoryExportImportFixture_FlattenedPayloadKeepsCanonicalArrayRoutes | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/api_export_import_e2e_smoke_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestExportImportSmoke_ExportedFactoryCanBeReimportedThroughCustomerPath | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/api_export_import_e2e_smoke_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestExportImportSmoke_ImportedFactoryPersistsThinSplitRuntimeLayout | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/api_export_import_e2e_smoke_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestExportImportSmoke_PreservesBatchInboxGitkeepAfterImport | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/api_export_import_e2e_smoke_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestExportImportSmoke_PreservesPortableLayoutThroughExportImportAndActivation | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/api_export_import_e2e_smoke_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestExportImportSmoke_PublicShareImportSurfaceCarriesDetachedStarterWork | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/automat_portability_fixture_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestAutomatPortabilityFixture_ExpandRestoresPortableRuntimeLayout | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/automat_portability_fixture_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestAutomatPortabilityFixture_ExpandedLayoutIsDispatchReadyForBoundedSmoke | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/automat_portability_fixture_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestAutomatPortabilityFixture_FlattenPreservesPortableBundleContract | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/automat_portability_fixture_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestAutomatPortabilityFixture_ModelsBoundedPortableRuntimeLayout | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/automat_portability_integration_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestAutomatPortabilityFixture_IntegrationSmoke_CoversFlattenExpandAndBoundedReadiness | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/cli_init_agent_slot_regression_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestInitFactory_AgentSlotResourceAlignmentRunsWithMockWorkers | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/cli_init_factory_schema_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestInitFactory_DefaultScaffoldFactoryJSONValidatesAgainstOpenAPISchema | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/cli_init_factory_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestInitFactory_ClaudeEndToEndUsesClaudeStarterWorker | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/cli_init_factory_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestInitFactory_EndToEnd | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/cli_init_factory_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestInitFactory_FailureRouting | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/cli_init_factory_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestInitFactory_Idempotent | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/cli_init_factory_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestInitFactory_StructureIsValid | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/cli_relative_working_directory_long_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestRelativeWorkingDirectory_UsesFactoryRuntimeRoot | functionallong | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/export_import_fixture_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestExportImportFixture_BuildsCanonicalExportAndImportContractsFromAuthoredFixture | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/export_import_fixture_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestExportImportFixture_PersistedFactoryExposesReusableCurrentFactorySignals | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/export_import_nested_docs_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestExportImportSmoke_PreservesNestedFactoryDocsThroughExportImport | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/factory_config_portability_long_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestFactoryConfigPortability_ExpandThenFlattenPreservesSemanticConfig | functionallong | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/factory_config_portability_long_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestFactoryConfigPortability_FlattenInlineScriptBackedFactoryExecutesStandalone | functionallong | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/factory_config_portability_long_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestFactoryConfigPortability_FlattenSplitLayoutExecutesStandalone | functionallong | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/factory_config_portability_long_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestFatFactory_LoadOnlyStandaloneFileUsesSharedMappingPath | functionallong | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/factory_config_portability_long_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestFatFactory_StandaloneCanonicalFileExecutesWithInlineDefinitions | functionallong | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/factory_validation_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestFactoryValidation_RejectsWorkstationWithNonexistentWorker | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/metadata_contract/factory_metadata_contract_test.go | you-agent-factory/tests/functional/bootstrap_portability/metadata_contract | TestFactoryMetadataContractLoadsLegacyExamplesAndRendersCanonicalHelp | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/metadata_contract/factory_metadata_contract_test.go | you-agent-factory/tests/functional/bootstrap_portability/metadata_contract | TestFactoryMetadataSnapshotRejectsInvalidProgrammaticExampleArguments | short | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/watcher_current_factory_activation_long_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestCurrentFactoryActivationFixture_ActivatesSecondPersistedFactoryAndResolvesCurrentFactory | functionallong | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/watcher_current_factory_activation_long_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestCurrentFactoryActivationFixture_LiveAPIReadsFollowActivatedFactory | functionallong | TBD | bootstrap_portability | none | TBD |
| tests/functional/bootstrap_portability/watcher_current_factory_activation_long_test.go | you-agent-factory/tests/functional/bootstrap_portability | TestCurrentFactoryActivationFixture_WatchedFileExecutionFollowsActivatedFactory | functionallong | TBD | bootstrap_portability | none | TBD |

#### `cli` (56 scenarios, catch_all=`none`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/cli/dynamic_workflows/dynamic_workflow_run_test.go | you-agent-factory/tests/functional/cli/dynamic_workflows | TestRunJavaScriptFactoryResponseStreamPublishesCanonicalLifecycle | short | TBD | none | none | TBD |
| tests/functional/cli/dynamic_workflows/dynamic_workflow_run_test.go | you-agent-factory/tests/functional/cli/dynamic_workflows | TestRunJavaScriptFactoryWithMockWorkersUsesFakeChildExecutor | short | TBD | none | none | TBD |
| tests/functional/cli/factory_run/events/events_test.go | you-agent-factory/tests/functional/cli/factory_run/events | TestSuccessfulInvocationEmitsCanonicalNDJSON | short | TBD | none | none | TBD |
| tests/functional/cli/factory_run/events/javascript_test.go | you-agent-factory/tests/functional/cli/factory_run/events | TestJavaScriptInvocationEmitsCanonicalPhaseAndCheckpointEvents | short | TBD | none | none | TBD |
| tests/functional/cli/factory_run/output/failure_test.go | you-agent-factory/tests/functional/cli/factory_run/output | TestInvocationFailureOutputContracts | short | TBD | none | none | TBD |
| tests/functional/cli/factory_run/output/output_test.go | you-agent-factory/tests/functional/cli/factory_run/output | TestSuccessfulInvocationOutputModes | short | TBD | none | none | TBD |
| tests/functional/cli/factory_run/yaml_io_parity/yaml_io_parity_test.go | you-agent-factory/tests/functional/cli/factory_run/yaml_io_parity | TestPackagedFactoryJSONAndYAMLValidateFlattenAndRunParity | short | TBD | none | none | TBD |
| tests/functional/cli/factory_run/yaml_io_parity/yaml_io_parity_test.go | you-agent-factory/tests/functional/cli/factory_run/yaml_io_parity | TestPublicBlockingValidationRetainsJSONAndYAMLSourceContext | short | TBD | none | none | TBD |
| tests/functional/cli/factory_run/yaml_io_parity/yaml_io_parity_test.go | you-agent-factory/tests/functional/cli/factory_run/yaml_io_parity | TestPublicMappingFailuresRetainJSONAndYAMLSourceContext | short | TBD | none | none | TBD |
| tests/functional/cli/factory_run/yaml_io_parity/yaml_io_parity_test.go | you-agent-factory/tests/functional/cli/factory_run/yaml_io_parity | TestRejectedAuthoredSourcesFailBeforeRuntimeExecution | short | TBD | none | none | TBD |
| tests/functional/cli/factory_run/yaml_io_parity/yaml_io_parity_test.go | you-agent-factory/tests/functional/cli/factory_run/yaml_io_parity | TestRuntimeMappingFailureRetainsSelectedSourceContext | short | TBD | none | none | TBD |
| tests/functional/cli/factory_run/yaml_io_parity/yaml_io_parity_test.go | you-agent-factory/tests/functional/cli/factory_run/yaml_io_parity | TestYAMLCreateAndUpdateRemainRunnableAfterCanonicalPersistence | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/canonical_input_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestCanonicalFactoryGroupRejectsUnknownSubcommand | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/canonical_input_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestCanonicalRunInputCompositionAndResolution | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/canonical_input_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestGenericRepresentativeProjectionIsObservableThroughApplicationRoot | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/canonical_input_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestGenericSessionProjectionCoversProductionCommandShapes | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/canonical_input_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestGenericSessionProjectionEnforcesProductionInputContracts | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/canonical_input_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestGenericSessionProjectionParsesVariadicInputsThroughApplicationRoot | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeAcceptsRepresentativeGeneratedFlagRecords | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeAppliesOptionalDefaultsAndFixedCardinality | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeAppliesTypedDefaultsAndRejectsInvalidInvocations | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeAssignsTypedPositionalArgumentsByStableInputID | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeBuildsSyntheticHierarchyDeterministically | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeEnforcesEveryRelationshipKindBeforeHandler | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeParsesSchemaNeutralTypedFlagsByStableInputID | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeRejectsInvalidArgumentAndRelationshipRecords | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeRejectsInvalidFlagRecordsBeforeReturningTree | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_contract_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeRejectsInvalidManifestBeforeReturningTree | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_coverage_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeDispatchesByStableHandlerIDWithNormalizedInputs | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_coverage_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeDispatchesScalarBooleanAndInt64Arguments | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_coverage_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeKeepsNonRunnableCommandsOnCobraHelpPath | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_coverage_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeProjectsMatchingInheritedPresentation | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_coverage_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeProjectsSchemaHelpLifecycleAndCompletion | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_coverage_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeRejectsInvalidHandlerBindingsBeforeExecution | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_coverage_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeRejectsInvalidLifecycleAndCompletionBeforeProjection | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_coverage_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeRejectsRepeatedScalarArgumentValueTypes | short | TBD | none | none | TBD |
| tests/functional/cli/input_contract/generic_constructor_helpers_test.go | you-agent-factory/tests/functional/cli/input_contract | TestNewCommandTreeRejectsInvalidCanonicalFlagValuesBeforeDispatch | short | TBD | none | none | TBD |
| tests/functional/cli/mcp_resume/resume_non_regression_test.go | you-agent-factory/tests/functional/cli/mcp_resume | TestMCPResumeSmokeLane_ResumeControlStaysOnCanonicalFactorySessionTools | short | TBD | none | none | TBD |
| tests/functional/cli/mcp_resume/resume_non_regression_test.go | you-agent-factory/tests/functional/cli/mcp_resume | TestMCPResumeSmokeLane_ResumeReadModelsUseSharedFactorySessionVocabulary | short | TBD | none | none | TBD |
| tests/functional/cli/mcp_resume/resume_non_regression_test.go | you-agent-factory/tests/functional/cli/mcp_resume | TestMCPResumeSmokeLane_RuntimeBackedAsyncServeRegression | short | TBD | none | none | TBD |
| tests/functional/cli/mcp_resume/serve_runtime_resume_smoke_test.go | you-agent-factory/tests/functional/cli/mcp_resume | TestRunServe_RuntimeResumeSmoke_DispatchContinuityPreservesCompletedChildDispatchesWithoutReplay | short | TBD | none | none | TBD |
| tests/functional/cli/mcp_resume/serve_runtime_resume_smoke_test.go | you-agent-factory/tests/functional/cli/mcp_resume | TestRunServe_RuntimeResumeSmoke_InterruptedSessionResumesThroughMCPControl | short | TBD | none | none | TBD |
| tests/functional/cli/mcp_resume/serve_runtime_resume_smoke_test.go | you-agent-factory/tests/functional/cli/mcp_resume | TestRunServe_RuntimeResumeSmoke_RunningSessionResumeReturnsTypedNoOpAndPreservesSessionRead | short | TBD | none | none | TBD |
| tests/functional/cli/mcp_resume/serve_runtime_resume_smoke_test.go | you-agent-factory/tests/functional/cli/mcp_resume | TestRunServe_RuntimeResumeSmoke_TerminalSessionResumeReturnsTypedRejectionAndPreservesSessionRead | short | TBD | none | none | TBD |
| tests/functional/cli/mcp_resume/serve_runtime_smoke_test.go | you-agent-factory/tests/functional/cli/mcp_resume | TestRunServe_RuntimeSmoke_DiscoveryAsyncPollAndResult | short | TBD | none | none | TBD |
| tests/functional/cli/named_invocation/named_invocation_test.go | you-agent-factory/tests/functional/cli/named_invocation | TestRun_NamedGoalHermeticInvocationSucceedsWithoutListeningServer | short | TBD | none | none | TBD |
| tests/functional/cli/named_invocation/named_invocation_test.go | you-agent-factory/tests/functional/cli/named_invocation | TestRun_NamedSubagentHermeticInvocationSucceedsWithoutListeningServer | short | TBD | none | none | TBD |
| tests/functional/cli/root_discovery/root_discovery_test.go | you-agent-factory/tests/functional/cli/root_discovery | TestBareRootPrintsConciseHelpWithoutProductEffects | short | TBD | none | none | TBD |
| tests/functional/cli/session/session_enumeration_test.go | you-agent-factory/tests/functional/cli/session | TestSessionEnumeration | short | TBD | none | none | TBD |
| tests/functional/cli/session/session_enumeration_test.go | you-agent-factory/tests/functional/cli/session | TestSessionEnumerationJSON | short | TBD | none | none | TBD |
| tests/functional/cli/session_resume/resume_non_regression_test.go | you-agent-factory/tests/functional/cli/session_resume | TestCLIResumeSmokeLane_NonResumeTerminalSessionShowPreservesShippedCLIReadSemantics | short | TBD | none | none | TBD |
| tests/functional/cli/session_resume/resume_non_regression_test.go | you-agent-factory/tests/functional/cli/session_resume | TestCLIResumeSmokeLane_ResumeInspectionStaysOnSharedSessionHTTPSurface | short | TBD | none | none | TBD |
| tests/functional/cli/session_resume/resume_smoke_test.go | you-agent-factory/tests/functional/cli/session_resume | TestCLIResumeSmoke_DurableResumeContinuityPreservesCompletedChildDispatchesWithoutReplay | short | TBD | none | none | TBD |
| tests/functional/cli/session_resume/resume_smoke_test.go | you-agent-factory/tests/functional/cli/session_resume | TestCLIResumeSmoke_InterruptedJavaScriptFactorySessionResumesThroughSharedSessionCommands | short | TBD | none | none | TBD |
| tests/functional/cli/session_resume/resume_smoke_test.go | you-agent-factory/tests/functional/cli/session_resume | TestCLIResumeSmoke_RunningSessionResumeReturnsTypedNoOpAndPreservesSessionRead | short | TBD | none | none | TBD |
| tests/functional/cli/session_resume/resume_smoke_test.go | you-agent-factory/tests/functional/cli/session_resume | TestCLIResumeSmoke_TerminalSessionResumeReturnsTypedRejectionAndPreservesSessionRead | short | TBD | none | none | TBD |

#### `config_init` (8 scenarios, catch_all=`none`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/config_init/config_init_test.go | you-agent-factory/tests/functional/config_init | TestInit_ConfigCreationFailureSurfacesActionableCLIError | short | TBD | none | none | TBD |
| tests/functional/config_init/config_init_test.go | you-agent-factory/tests/functional/config_init | TestInit_DoubleRunJSONReportsSkippedOutcomes | short | TBD | none | none | TBD |
| tests/functional/config_init/config_init_test.go | you-agent-factory/tests/functional/config_init | TestInit_DoubleRunReportsSkippedOutcomes | short | TBD | none | none | TBD |
| tests/functional/config_init/config_init_test.go | you-agent-factory/tests/functional/config_init | TestInit_ExistingConfigReportsSkippedWithoutRewrite | short | TBD | none | none | TBD |
| tests/functional/config_init/config_init_test.go | you-agent-factory/tests/functional/config_init | TestInit_FactoryMaterializationFailureSurfacesActionableCLIError | short | TBD | none | none | TBD |
| tests/functional/config_init/config_init_test.go | you-agent-factory/tests/functional/config_init | TestInit_FreshHomeCreatesSystemConfigAndReportsOutcome | short | TBD | none | none | TBD |
| tests/functional/config_init/config_init_test.go | you-agent-factory/tests/functional/config_init | TestInit_JSONEmitsStructuredSummary | short | TBD | none | none | TBD |
| tests/functional/config_init/config_init_test.go | you-agent-factory/tests/functional/config_init | TestInit_UsesProvidedHomeDirWithoutReadingProcessHome | short | TBD | none | none | TBD |

#### `guards_batch` (28 scenarios, catch_all=`guards_batch`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/guards_batch/cascading_failure_test.go | you-agent-factory/tests/functional/guards_batch | TestCascadingFailure_CompletedNotCascaded | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/cascading_failure_test.go | you-agent-factory/tests/functional/guards_batch | TestCascadingFailure_DirectChild | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/cascading_failure_test.go | you-agent-factory/tests/functional/guards_batch | TestCascadingFailure_Transitive | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/concurrency_limit_long_test.go | you-agent-factory/tests/functional/guards_batch | TestConcurrencyLimit_BlocksExcessDispatches | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/concurrency_limit_long_test.go | you-agent-factory/tests/functional/guards_batch | TestConcurrencyLimit_ReducedCapacityStillCompletes | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/concurrency_limit_long_test.go | you-agent-factory/tests/functional/guards_batch | TestConcurrencyLimit_ResourceReleasedOnFailure | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/concurrency_limit_long_test.go | you-agent-factory/tests/functional/guards_batch | TestConcurrencyLimit_ResourceTokensConsumedDuringProcessing | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/dependency_terminal_test.go | you-agent-factory/tests/functional/guards_batch | TestDependencyTerminal_BlockedDuringProcessing | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/dependency_terminal_test.go | you-agent-factory/tests/functional/guards_batch | TestDependencyTerminal_BlockedUntilArchived | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/dependency_terminal_test.go | you-agent-factory/tests/functional/guards_batch | TestDependencyTerminal_BothComplete | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/dependency_tracking_test.go | you-agent-factory/tests/functional/guards_batch | TestDependencyTracking_BlocksUntilSatisfied | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/dependency_tracking_test.go | you-agent-factory/tests/functional/guards_batch | TestDependencyTracking_NoDepsPassThrough | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/executor_failure_test.go | you-agent-factory/tests/functional/guards_batch | TestExecutorFailure_NoFailureArcs | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/executor_failure_test.go | you-agent-factory/tests/functional/guards_batch | TestExecutorFailure_OutcomeFailed_NoFailureArcs | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/executor_failure_test.go | you-agent-factory/tests/functional/guards_batch | TestExecutorFailure_WithFailureArcs | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/executor_failure_test.go | you-agent-factory/tests/functional/guards_batch | TestExecutorSuccess_TokenAtOutputPlace | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/failed_immutability_long_test.go | you-agent-factory/tests/functional/guards_batch | TestFailedImmutability_CannotBeReDispatched | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/failed_immutability_long_test.go | you-agent-factory/tests/functional/guards_batch | TestFailedImmutability_NoDuplicateTokens | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/failed_immutability_long_test.go | you-agent-factory/tests/functional/guards_batch | TestFailedImmutability_ReviewerFailure | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/partial_batch_long_test.go | you-agent-factory/tests/functional/guards_batch | TestPartialBatch_ProviderExitFailureRoutesTokenToFailedWithContext | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/partial_batch_long_test.go | you-agent-factory/tests/functional/guards_batch | TestPartialBatch_RetryableProviderFailuresRetryThroughScriptWrapPath | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/partial_batch_long_test.go | you-agent-factory/tests/functional/guards_batch | TestPartialBatch_SomeTokensFail | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/partial_batch_long_test.go | you-agent-factory/tests/functional/guards_batch | TestPartialBatch_SomeTokensRejected_RoutedViaRejectionArcs | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/partial_batch_long_test.go | you-agent-factory/tests/functional/guards_batch | TestPartialBatch_TemplateResolvesFromTags | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/partial_batch_long_test.go | you-agent-factory/tests/functional/guards_batch | TestPartialBatch_ThrottledProviderFailureWithoutAuthoredGuardEventuallyFails | functionallong | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/resource_contention_test.go | you-agent-factory/tests/functional/guards_batch | TestConfigDriven_ResourceContention | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/resource_token_name_test.go | you-agent-factory/tests/functional/guards_batch | TestResourceGated_DispatchTokenName | short | TBD | guards_batch | none | TBD |
| tests/functional/guards_batch/watcher_parent_child_batch_long_test.go | you-agent-factory/tests/functional/guards_batch | TestFileWatcherParentChildBatch_SubmittedFanInSmoke | functionallong | TBD | guards_batch | none | TBD |

#### `models` (1 scenarios, catch_all=`none`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/models/model_invoke/cli_test.go | you-agent-factory/tests/functional/models/model_invoke | TestProcessModelsInvokeUsesCanonicalGraphAndExactExternalEdges | short | TBD | none | none | TBD |

#### `operator_settings` (1 scenarios, catch_all=`none`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/operator_settings/configcore/operator_config_core_test.go | you-agent-factory/tests/functional/operator_settings/configcore | TestOperatorConfigCore_PromptedAndPresuppliedUpdatesShareAtomicBehavior | short | TBD | none | none | TBD |

#### `providers` (46 scenarios, catch_all=`none`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_ArgTemplating | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_ArgTemplatingWithTags | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_AsyncWorkerPoolTemplateFallbackScenarios | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_Failure | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_FailureRoutesToFailedPlace | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_PreservesTokenColor | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_RuntimeConfigMergePreservesCanonicalTopologyAndPromptTemplates | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_RuntimeWorkstationConfigResolvesWorkingDirectoryAndEnv | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_RuntimeWorkstationTimeoutRequeuesAndRetriesOnLaterTick | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_Success | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_SuccessWithColorMetadata | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_WorkTypeIDFromTargetPlace | short | TBD | none | none | TBD |
| tests/functional/providers/cli_script_executor_timeout_long_test.go | you-agent-factory/tests/functional/providers | TestScriptExecutor_RuntimeWorkerTimeoutFromLoadedConfigRequeuesAndRetriesOnLaterTick | functionallong | TBD | none | none | TBD |
| tests/functional/providers/cli_template_resolution_long_test.go | you-agent-factory/tests/functional/providers | TestTemplateTests_ScriptExecutorDropsResourceTokensFromArgTemplates | functionallong | TBD | none | none | TBD |
| tests/functional/providers/cli_template_resolution_long_test.go | you-agent-factory/tests/functional/providers | TestTemplateTests_ScriptExecutorOrdersMultipleInputsByWorkstationConfigWithResources | functionallong | TBD | none | none | TBD |
| tests/functional/providers/cli_template_resolution_long_test.go | you-agent-factory/tests/functional/providers | TestTemplateTests_ScriptWrapClaudeResolvesWorkstationExecutionTemplates | functionallong | TBD | none | none | TBD |
| tests/functional/providers/cli_template_resolution_long_test.go | you-agent-factory/tests/functional/providers | TestTemplateTests_ScriptWrapCodexResolvesWorkstationExecutionTemplates | functionallong | TBD | none | none | TBD |
| tests/functional/providers/cli_template_resolution_long_test.go | you-agent-factory/tests/functional/providers | TestTemplateTests_ScriptWrapCursorResolvesWorkstationExecutionTemplates | functionallong | TBD | none | none | TBD |
| tests/functional/providers/cli_template_resolution_long_test.go | you-agent-factory/tests/functional/providers | TestTemplateTests_ScriptWrapDropsResourceTokensFromWorkstationTemplates | functionallong | TBD | none | none | TBD |
| tests/functional/providers/cli_template_resolution_long_test.go | you-agent-factory/tests/functional/providers | TestTemplateTests_ScriptWrapOrdersMultipleInputsByWorkstationConfigWithResources | functionallong | TBD | none | none | TBD |
| tests/functional/providers/cli_timeout_cleanup_smoke_test.go | you-agent-factory/tests/functional/providers | TestIntegrationSmoke_ProcessTreeHelper | short | TBD | none | none | TBD |
| tests/functional/providers/cli_timeout_cleanup_smoke_test.go | you-agent-factory/tests/functional/providers | TestIntegrationSmoke_TimeoutCancelsProcessTreeAndClearsActiveExecution | short | TBD | none | none | TBD |
| tests/functional/providers/cli_timeout_cleanup_smoke_test.go | you-agent-factory/tests/functional/providers | TestIntegrationSmoke_TimeoutRequeuesWorkAndSucceedsOnLaterAttempt | short | TBD | none | none | TBD |
| tests/functional/providers/cli_timeout_companion_smoke_long_test.go | you-agent-factory/tests/functional/providers | TestIntegrationSmoke_ScriptTimeoutCompanionRequeuesBeforeLaterCompletion | functionallong | TBD | none | none | TBD |
| tests/functional/providers/cli_worktree_passthrough_test.go | you-agent-factory/tests/functional/providers | TestWorktreePassthrough | short | TBD | none | none | TBD |
| tests/functional/providers/codex/process_harness_test.go | you-agent-factory/tests/functional/providers/codex | TestRootBuiltProcessExecutesThroughSharedSupport | short | TBD | none | none | TBD |
| tests/functional/providers/codex_content_test.go | you-agent-factory/tests/functional/providers | TestCodexContentDispatch_MixedContentEmitsOrderedImageArgs | short | TBD | none | none | TBD |
| tests/functional/providers/codex_content_test.go | you-agent-factory/tests/functional/providers | TestCodexContentDispatch_TextOnlyContentDoesNotEmitImageArgs | short | TBD | none | none | TBD |
| tests/functional/providers/codex_worktree_workstation_test.go | you-agent-factory/tests/functional/providers | TestCodexWorktreeWorkstationDispatch_MaterializesCheckoutAndOmitsCLIWorktreeFlag | short | TBD | none | none | TBD |
| tests/functional/providers/cursor_provider_command_test.go | you-agent-factory/tests/functional/providers | TestCursorProviderCommand_DispatchesAgentWithRenderedPrompt | short | TBD | none | none | TBD |
| tests/functional/providers/cursor_provider_command_test.go | you-agent-factory/tests/functional/providers | TestCursorProviderCommand_PublicModelProviderEnumRoutesToAgent | short | TBD | none | none | TBD |
| tests/functional/providers/cursor_provider_command_test.go | you-agent-factory/tests/functional/providers | TestCursorProviderCommand_SkipPermissionsPassesForceFlag | short | TBD | none | none | TBD |
| tests/functional/providers/mock_workers_agent_test.go | you-agent-factory/tests/functional/providers | TestMockWorkers_AgentDefaultAcceptMovesWorkToOutputPlace | short | TBD | none | none | TBD |
| tests/functional/providers/mock_workers_agent_test.go | you-agent-factory/tests/functional/providers | TestMockWorkers_AgentRejectConfigRoutesFailureWithoutLoggingCommandOutput | short | TBD | none | none | TBD |
| tests/functional/providers/mock_workers_agent_test.go | you-agent-factory/tests/functional/providers | TestMockWorkers_AgentRejectConfigWithZeroExitCodeIsRejectedAtCustomerBoundary | short | TBD | none | none | TBD |
| tests/functional/providers/mock_workers_cli_http_stability_smoke_long_test.go | you-agent-factory/tests/functional/providers | TestMockWorkers_CLIServiceModeStartupWorkFileSupportsRepeatedLiveHTTPPollingBeforeCompletion | functionallong | TBD | none | none | TBD |
| tests/functional/providers/mock_workers_end_to_end_smoke_test.go | you-agent-factory/tests/functional/providers | TestMockWorkers_EndToEndSmokeRunsMixedOutcomesWithoutLiveProviderCredentials | short | TBD | none | none | TBD |
| tests/functional/providers/mock_workers_script_test.go | you-agent-factory/tests/functional/providers | TestMockWorkers_ScriptConfigExecutesCommandRunnerSideEffect | short | TBD | none | none | TBD |
| tests/functional/providers/mock_workers_script_test.go | you-agent-factory/tests/functional/providers | TestMockWorkers_ScriptDefaultAcceptProducesSuccessfulScriptResult | short | TBD | none | none | TBD |
| tests/functional/providers/mock_workers_script_test.go | you-agent-factory/tests/functional/providers | TestMockWorkers_ScriptHelper | short | TBD | none | none | TBD |
| tests/functional/providers/mock_workers_script_test.go | you-agent-factory/tests/functional/providers | TestMockWorkers_ScriptRejectConfigRoutesFailureAndLogsCommandOutput | short | TBD | none | none | TBD |
| tests/functional/providers/mock_workers_script_test.go | you-agent-factory/tests/functional/providers | TestMockWorkers_ScriptRejectConfigWithZeroExitCodeStillRoutesFailure | short | TBD | none | none | TBD |
| tests/functional/providers/mock_workers_service_runner_test.go | you-agent-factory/tests/functional/providers | TestMockWorkers_ServiceCommandRunnerCompletesModelAndScriptWorkers | short | TBD | none | none | TBD |
| tests/functional/providers/packaged_script_runtime_test.go | you-agent-factory/tests/functional/providers | TestPackagedScriptRuntime_FreshInstallExecutesFactoryRelativeScript | short | TBD | none | none | TBD |
| tests/functional/providers/packaged_script_runtime_test.go | you-agent-factory/tests/functional/providers | TestPackagedScriptRuntime_NonZeroExitUsesStandardFailureOutcome | short | TBD | none | none | TBD |
| tests/functional/providers/runtime_logging_smoke_test.go | you-agent-factory/tests/functional/providers | TestRuntimeLoggingSmoke_SuccessAndFailureRespectOutputEnvAndRollingPolicies | short | TBD | none | none | TBD |

#### `replay_contracts` (24 scenarios, catch_all=`replay_contracts`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/replay_contracts/canonical_topology_snapshot_projection_test.go | you-agent-factory/tests/functional/replay_contracts | TestCanonicalTopologySnapshotsPreservePublicIdentityAndResourceEvidence | short | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_event_stream_artifact_smoke_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayEventStreamArtifactSmoke_ReplaysCheckedInSampleArtifact | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_event_stream_artifact_smoke_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayEventStreamArtifactSmoke_ReplaysCheckedInSampleArtifactWithCopiedRootFactoryDefinition | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_factory_only_serialization_smoke_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayFactoryOnlySerializationSmoke_RecordReplayUsesRunStartedFactoryPayload | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_legacy_unary_retirement_smoke_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestLegacyUnaryRetirementSmoke_ReplaySubmitsCanonicalBatchWorkRequests | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_record_end_to_end_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestRecordReplayEndToEnd_CLIRecordReplayAndRegressionHarnessSucceed | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_record_end_to_end_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestRecordReplayEndToEnd_DefaultLiveRecordingPathReplaysThroughExistingFlow | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_record_end_to_end_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestRecordReplayEndToEnd_FactoryRequestBatchAndWorkerGeneratedBatchReplayDeterministically | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_record_end_to_end_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestRecordReplayEndToEnd_ProviderCommandDiagnosticsPersistRedactedEnv | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_regression_harness_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayRegressionHarness_AssertsExpectedDivergence | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_regression_harness_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayRegressionHarness_LoadsArtifactAndAssertsSuccessfulReplay | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_runtime_config_smoke_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayRuntimeConfigSmoke_CanonicalWorkstationsDriveDispatchAndReplay | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_script_boundary_events_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayScriptBoundaryEvents_CanonicalHistoryAndArtifactIncludeScriptResponseBoundary | short | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_script_boundary_events_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayScriptBoundaryEvents_CanonicalHistoryIncludesScriptRequestBoundary | short | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_script_boundary_events_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayScriptBoundaryEvents_ProcessFailureBoundaryPersistsInCanonicalHistoryAndArtifact | short | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_thin_event_dual_dispatch_smoke_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayThinEventDualDispatchSmoke_ReplayAndReadersReuseSharedArtifact | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_thin_event_dual_dispatch_smoke_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayThinEventDualDispatchSmoke_SharedArtifactCapturesModelAndScriptDispatches | short | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_thin_event_dual_dispatch_smoke_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayThinEventDualDispatchSmoke_SharedArtifactGuardsThinRawContract | short | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_work_dispatch_contract_smoke_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayWorkDispatchContractSmoke_CanonicalWorkRequestPreservesPayload | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_work_dispatch_contract_smoke_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayWorkDispatchContractSmoke_LegacySubmitRequestAdapterPreservesPayload | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/replay_work_dispatch_contract_smoke_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayWorkDispatchContractSmoke_RecordReplayKeepsSplitContractCorrelation | functionallong | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/short_helpers_contract_test.go | you-agent-factory/tests/functional/replay_contracts | TestFactoryRelationsValueReturnsUnderlyingSlice | short | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/short_helpers_contract_test.go | you-agent-factory/tests/functional/replay_contracts | TestReplayEventCountCountsMatchingEventTypes | short | TBD | replay_contracts | none | TBD |
| tests/functional/replay_contracts/worker_public_contract_smoke_long_test.go | you-agent-factory/tests/functional/replay_contracts | TestWorkerPublicContractSmoke_CanonicalWorkerExecutesAndKeepsRuntimeOnlyFieldsPrivate | functionallong | TBD | replay_contracts | none | TBD |

#### `runtime_api` (108 scenarios, catch_all=`runtime_api`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/runtime_api/api_batch_submission_boundary_smoke_long_test.go | you-agent-factory/tests/functional/runtime_api | TestFactoryRequestBatch_PublicBatchShapeStaysAlignedAcrossWatchedFileAndHTTP | functionallong | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_cleanup_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestCleanupSmoke_BackendDashboardAndCanonicalEventsExposeOnlyCleanedFactorySurfaces | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_config_driven_submit_query_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestConfigDriven_RESTAPISubmitAndQuery | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_cron_workstations_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestCronWorkstations_ServiceModeExpiryConsumesStaleTriggerWithTerminalOutputAndDefaultWindow | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_cron_workstations_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestCronWorkstations_ServiceModeImplicitFailureRoutingMovesFailedCronWorkIntoFailedState | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_cron_workstations_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestCronWorkstations_ServiceModeSmoke_SubmitsInternalTimeWorkExpiresRetriesDispatchesAndFiltersViews | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_event_replay_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestAPIEventReplaySmoke_PublicEventsAndSessionProjectionExposeActiveAndCompletedTimeline | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_factory_event_mapping_test.go | you-agent-factory/tests/functional/runtime_api | TestFactoryEventTransportMappingRejectsMalformedCanonicalPayload | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_functional_server_override_regression_test.go | you-agent-factory/tests/functional/runtime_api | TestFunctionalServerOverrideCompatibilityRegression_MockWorkersAndProviderOverride | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_functionallong_compile_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestRuntimeAPI_CompilesWithFunctionalLongTag | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_rest_client_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedRESTClientSmoke_BoundsCancellationAndDeadline | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_rest_client_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedRESTClientSmoke_ConfiguresCallerOwnedDependencies | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_rest_client_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedRESTClientSmoke_RoundTripsTypedSuccessAndAPIFailure | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_schema_deserialization_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedSchemaDeserializationSmoke_FileAndRecordedTransportRejectRetiredFieldsAtSameBoundaryStage | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_schema_deserialization_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedSchemaDeserializationSmoke_FileHTTPAndReplayTransportsStayAligned | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedAPIIntegrationSmoke_BatchUpsertAcceptsWorksContent | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedAPIIntegrationSmoke_CLIWorkTypeNameReachesLiveAPIHandler | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedAPIIntegrationSmoke_OpenAPIGeneratedServerAndLiveRuntimeStayAligned | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedAPIIntegrationSmoke_SubmitWorkContentAcceptsCanonicalParts | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsAcceptHeaderOnlyStructuredSubmission | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsAcceptMixedTextAndImageSubmissionOnSupportedRunner | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsAcceptOrderedTextSubmission | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsRejectEmptyStructuredSubmission | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsRejectForgedStructuredFileReference | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_generated_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsRejectMixedTextAndImageSubmissionOnUnsupportedRunner | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_inference_events_test.go | you-agent-factory/tests/functional/runtime_api | TestInferenceEvents_HTTPStreamAndPublicWorkCorrelateRetryAttempts | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_inference_events_test.go | you-agent-factory/tests/functional/runtime_api | TestInferenceEvents_ModelProviderAttemptsRecordInCanonicalHistoryAndArtifact | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_inference_events_test.go | you-agent-factory/tests/functional/runtime_api | TestInferenceEvents_ScriptWorkersDoNotEmitInferenceEvents | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_inference_events_test.go | you-agent-factory/tests/functional/runtime_api | TestInferenceEvents_ThinEventSmoke_CapturesThinnedDispatchInferenceSequenceAndReconstructsViews | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_legacy_unary_retirement_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_manual_work_recovery_test.go | you-agent-factory/tests/functional/runtime_api | TestManualWorkRecovery_CascadeFailureThenAPIMovesResumeProgress | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_model_local_inference_long_test.go | you-agent-factory/tests/functional/runtime_api | TestRealLocalInference_OMNIVOICEModelInvokeAndDirectAPIProduceAudio | functionallong | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_model_transport_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestModelTransportSmoke_ServiceModeStartupAndDirectModelRoutesStayAligned | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_ootb_experience_smoke_long_test.go | you-agent-factory/tests/functional/runtime_api | TestOOTBExperience_APIPreseededSimplePipelineCompletes | functionallong | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_ootb_experience_smoke_long_test.go | you-agent-factory/tests/functional/runtime_api | TestOOTBExperience_APIPreseededTwoStagePipelineCompletes | functionallong | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_ootb_experience_smoke_long_test.go | you-agent-factory/tests/functional/runtime_api | TestOOTBExperience_APIStatusStaysQueryableAcrossCompletion | functionallong | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_packaged_deep_research_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_PackagedDeepResearchUsesMaterializedFactorySource | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_packaged_goal_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestPackagedGoalBuiltInTopology_SubmitWhilePausedResumesThroughSessionControl | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_packaged_goal_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_PackagedGoalContinueRepeatsBeforeCompletion | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_packaged_goal_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_PackagedGoalRejectRepeatsBeforeCompletion | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_packaged_goal_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_PackagedGoalReturnsExplicitSummaryPrimaryResult | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_packaged_goal_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_PackagedGoalWorkerFailureReturnsFailedStatusDetails | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_packaged_quorum_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_PackagedQuorumAppliesRoleArguments | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_packaged_quorum_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_PackagedQuorumGatesMergeUntilBothBranchesComplete | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_packaged_review_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_PackagedReviewRejectsThenApprovesRevision | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_packaged_review_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_PackagedReviewReturnsApprovedCandidate | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_packaged_review_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_PackagedReviewWorkerFailureReturnsFailedStatus | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_provider_throttle_pause_observability_test.go | you-agent-factory/tests/functional/runtime_api | TestProviderErrorSmoke_ThrottleFailureIsolatesOtherLaneThroughPublicSession | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_request_batch_boundary_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestGeneratedAPIIntegrationSmoke_BatchWorkTypeNameNormalizesRuntimeWork | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_runtime_config_alignment_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_runtime_log_policy_test.go | you-agent-factory/tests/functional/runtime_api | TestFunctionalAPIServer_RuntimeLogDirectoryIsAProcessInput | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_runtime_log_policy_test.go | you-agent-factory/tests/functional/runtime_api | TestFunctionalAPIServer_UsesProductionRuntimeFileLoggingDefault | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_service_config_override_alignment_test.go | you-agent-factory/tests/functional/runtime_api | TestServiceConfigOverrideAlignment_FunctionalHTTPServerProviderCommandRunner | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_service_config_override_alignment_test.go | you-agent-factory/tests/functional/runtime_api | TestServiceConfigOverrideAlignment_FunctionalHTTPServerScriptCommandRunner | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_service_mode_observability_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestObservabilitySmoke_PublicStatusSessionWorkAndEventsAlignAcrossRuntimeTransitions | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_service_mode_observability_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestServiceModeSmoke_EmptyStartupIdleSubmissionAndPostCompletionIdleStayReachableUntilCanceled | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_session_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_CanceledRequestContextStopsRequest | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_session_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_PausedSessionReturnsPausedStatus | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_session_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_RejectsArgsWithoutActiveSignature | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_session_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_RejectsInvalidStructuredArgValueShape | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_session_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_RejectsWhitespaceOnlyText | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_session_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_ReturnsPrimaryResult | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_session_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_TimeoutReturnsTimedOutStatus | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_session_invocation_test.go | you-agent-factory/tests/functional/runtime_api | TestSessionInvocationAPI_UnresolvedPrimaryResultReturnsFailedStatus | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_submit_runtime_work_trace_shaping_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestSubmitRuntimeWork_EmitsCanonicalTraceAwareBatchEvent | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_throttle_failure_regression_test.go | you-agent-factory/tests/functional/runtime_api | TestRetryableThrottleFailureWithoutGuardUsesDefaultRetryLimitAndFailsWork | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/api_unified_event_log_smoke_test.go | you-agent-factory/tests/functional/runtime_api | TestAPIUnifiedEventLogSmoke_LiveRecordReplayProjectionAndDivergenceUseSameTimeline | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/dashboard_engine_state_test.go | you-agent-factory/tests/functional/runtime_api | TestDashboard_EngineStateSnapshot_EndToEnd | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_docs_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryEvents_InitialStructureIncludesBundledFileContent | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_docs_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_DocsCreateEditRenameDeleteRoundTrip | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_docs_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_DocsSaveEmitsFactoryChangeWithBundledFilesAndVersion | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_docs_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_RejectsDuplicateDocTargetPaths | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_docs_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_RejectsInvalidDocTargets | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_docs_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_ShellEscapedBundledInlineReplayReturnsPayloadInvalid | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryEvents_ExposePortableLayoutOnInitialStructureAndFactoryChange | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_AcceptsLayoutForKnownBundledDocNode | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_AcceptsLayoutNodeMissingSize | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_PreservesPortableLayoutThroughSaveReloadAndRuntimeExecution | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_PrunesStaleLayoutWithoutReturningEphemeralLayoutMetadata | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_RejectsLayoutForUnknownBundledDocNode | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_validation_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_PrePersistLayoutFailureRetainsStructuredPath | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_variants_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_AcceptsPortableLayoutEdgeWithMultipleWaypoints | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_variants_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_AcceptsPortableLayoutEdgeWithOneWaypoint | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_variants_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_AcceptsPortableLayoutMultipleNodesWithSize | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_layout_variants_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_AcceptsPortableLayoutMultipleNodesWithoutSize | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_session_import_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_NonDefaultSessionImportIsolatesDefaultFactoryAndMaterializesBundledFiles | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_DefaultFactoryAcceptsFullCurrentFactoryReadbackDocument | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_DefaultFactoryMaterializesBundledFilesAndReturns | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_FactoryChangeVersionsAdvanceOnEverySave | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_RejectsTypeCountCollisionBeforePersistingDefaultFactory | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_RequiresAdvancedSaveVersion | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_ReturnsCanonicalTopologyValidationTargets | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_ReturnsMultipleTopologyValidationTargets | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_SaveDefaultFactoryDefinitionPersistsAndRunsReplacement | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_SaveEditableCurrentFactoryDefinitionEmitsCanonicalFactoryChangeEvent | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestCurrentFactoryPUT_SessionScopedNamedFactoryTransformationReadbackIsIsolated | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_import_replace_current_split_layout_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestFactoryTransformation_ReplaceCurrentImportMatchesCreateNamedSplitLayout | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_named_factory_layout_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestFactoryTransformation_CreateNamedFactoryPreservesPortableLayoutThroughActivationAndReadback | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_named_factory_layout_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestFactoryTransformation_UpsertNamedFactoryReplacePreservesPortableLayout | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_named_factory_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestFactoryTransformation_CreateNamedFactoryEmitsCanonicalFactoryChangeEvent | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_named_factory_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestFactoryTransformation_CreateNamedFactoryReadbackAndWorkSurface | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_named_factory_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestFactoryTransformation_CreateNamedFactory_ReturnsBobOnFailureTarget | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_named_factory_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestFactoryTransformation_CreateNamedFactory_ReturnsMultipleTopologyValidationTargets | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_named_factory_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestFactoryTransformation_NamedFactoryPortableFilesReadBackThroughCanonicalContract | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_session_factory_save_version_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestSessionFactoryPUT_UpsertCreateAllowsOmittedVersion | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_session_factory_save_version_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestSessionFactoryPUT_UpsertReplaceDoesNotReturnAlreadyExists | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/factory_transformation/api_session_factory_save_version_test.go | you-agent-factory/tests/functional/runtime_api/factory_transformation | TestSessionFactoryPUT_UpsertReplaceRejectsStaleVersion | short | TBD | runtime_api | none | TBD |
| tests/functional/runtime_api/topology_projection_smoke_long_test.go | you-agent-factory/tests/functional/runtime_api | TestEndToEndTopologyProjectionSmoke_LiveEventsAndReplayConfigMatch | functionallong | TBD | runtime_api | none | TBD |

#### `sessionparity` (13 scenarios, catch_all=`none`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/sessionparity/compare_test.go | you-agent-factory/tests/functional/sessionparity | TestCompare_ReportsAdjacentLargeEventSequences | short | TBD | none | none | TBD |
| tests/functional/sessionparity/compare_test.go | you-agent-factory/tests/functional/sessionparity | TestCompare_ReportsEveryRetainedFactMismatch | short | TBD | none | none | TBD |
| tests/functional/sessionparity/compare_test.go | you-agent-factory/tests/functional/sessionparity | TestCompare_ReportsMissingUnexpectedDuplicateAndReorderedFacts | short | TBD | none | none | TBD |
| tests/functional/sessionparity/compare_test.go | you-agent-factory/tests/functional/sessionparity | TestNormalizers_ExcludeTransportDetailsAndRetainSessionFacts | short | TBD | none | none | TBD |
| tests/functional/sessionparity/contract_test.go | you-agent-factory/tests/functional/sessionparity | TestProjectionContract_PreservesOptionalStableFactAbsence | short | TBD | none | none | TBD |
| tests/functional/sessionparity/contract_test.go | you-agent-factory/tests/functional/sessionparity | TestProjectionContract_RetainsEveryStableFactorySessionFact | short | TBD | none | none | TBD |
| tests/functional/sessionparity/fixtures_test.go | you-agent-factory/tests/functional/sessionparity | TestTerminalFixtureObservations_AreDeterministic | short | TBD | none | none | TBD |
| tests/functional/sessionparity/fixtures_test.go | you-agent-factory/tests/functional/sessionparity | TestTerminalFixtureObservations_NormalizeAcrossCustomerInterfaces | short | TBD | none | none | TBD |
| tests/functional/sessionparity/normalize_test.go | you-agent-factory/tests/functional/sessionparity | TestNormalizeCLIJSON_PreservesCustomerVisibleCollectionOrder | short | TBD | none | none | TBD |
| tests/functional/sessionparity/normalize_test.go | you-agent-factory/tests/functional/sessionparity | TestNormalizeMCP_RejectsReorderedCanonicalEventCursors | short | TBD | none | none | TBD |
| tests/functional/sessionparity/normalize_test.go | you-agent-factory/tests/functional/sessionparity | TestNormalizers_MapRepresentativeRealCustomerShapes | short | TBD | none | none | TBD |
| tests/functional/sessionparity/normalize_test.go | you-agent-factory/tests/functional/sessionparity | TestNormalizers_PreserveDistinctLargeIntegerResultValues | short | TBD | none | none | TBD |
| tests/functional/sessionparity/normalize_test.go | you-agent-factory/tests/functional/sessionparity | TestNormalizers_RejectEveryMissingRequiredScalarFact | short | TBD | none | none | TBD |

#### `smoke` (94 scenarios, catch_all=`smoke`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/smoke/archive_terminal_test.go | you-agent-factory/tests/functional/smoke | TestArchiveTerminal_MultipleTokensAllTerminate | short | TBD | smoke | none | TBD |
| tests/functional/smoke/archive_terminal_test.go | you-agent-factory/tests/functional/smoke | TestArchiveTerminal_NoFurtherFiring | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_docs_smoke_test.go | you-agent-factory/tests/functional/smoke | TestDocsCommandSmoke_AuthoringFactoriesDescribesMinimalGoalRepeater | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_docs_smoke_test.go | you-agent-factory/tests/functional/smoke | TestDocsCommandSmoke_PackagedTopicsRemainAvailableOutsideRepositoryDocsTree | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_factory_prompt_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestFactoryPromptRun_RealCLIAmbiguousPromptAndStdinFailsBeforeRuntimeStartup | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_factory_prompt_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestFactoryPromptRun_RealCLICleanInvocationStdoutRemainsPipeableAcrossRuns | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_factory_prompt_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestFactoryPromptRun_RealCLIFailureWritesNoSuccessPayloadToStdout | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_factory_prompt_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestFactoryPromptRun_RealCLIRejectsConflictingPositionalAndStdinInput | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_factory_prompt_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestFactoryPromptRun_RealCLIRejectsFactoryWithoutDefaultHandling | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_factory_prompt_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestFactoryPromptRun_RealCLIStdinOnlyCleanInvocationStdoutRemainsPipeable | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_factory_prompt_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestFactoryPromptRun_RealCLIWritesPrimaryResultFromPositionalText | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_factory_prompt_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestFactoryPromptRun_RealCLIWritesPrimaryResultFromStdin | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_factory_prompt_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedFactoryRun_RealCLIResolvesGlobalFactoryFromUnrelatedWorkingDirectory | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_factory_prompt_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestPackagedGoalRun_RealCLIWritesSummaryPrimaryResult | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_javascript_factory_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestJavaScriptFactoryRun_RealCLIProvesOrderedTwoStagePipeline | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_javascript_factory_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestJavaScriptFactoryRun_RealCLIUsesMockWorkersAndReturnsPrimaryResult | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_deep_research_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedDeepResearchCLI_DefaultInvocationReturnsLeadSynthesis | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_deep_research_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedDeepResearchCLI_InvokesConfiguredBoundedResearchWithApprovedFlags | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_invocation_parity_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalInvocationParity_EmptyInputRejectedWithStableCode | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_invocation_parity_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalInvocationParity_NamedFactoryCLIAndAPIShareSuccessOutcome | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_invocation_parity_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalInvocationParity_PositionalCLIAndAPIShareSuccessOutcome | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_invocation_parity_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalInvocationParity_SourceConflictRejectedBeforeInvocation | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_invocation_parity_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalInvocationParity_StdinCLIAndAPITextShareSuccessOutcome | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_invocation_parity_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalInvocationParity_UnresolvedPrimaryResultReportsStableFailure | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_operator_controls_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalOperatorControls_ClIPauseResumeDrainsBufferedGoalsInOrder | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_operator_controls_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalOperatorControls_InterruptedGoalInspectSurfacesDispatchAndStopSummary | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_operator_controls_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalOperatorControls_PauseBuffersSubmitUntilResume | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_operator_controls_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalOperatorControls_PauseResumeRecordsLifecycleControlEventsForReplay | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_routing_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalRouting_AcceptedCompletesWithPrimaryResult | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_routing_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalRouting_ClassifierNonSuccessOutcomesSurfaceDistinctStates | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_routing_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalRouting_InterruptedSuppressesSuccessPrimaryResult | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_routing_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalRouting_ReworkLoopsBackThenCompletesWithGoalContext | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_routing_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalRouting_StructuredUnknownDecisionRoutesToFailed | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_routing_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalRouting_UnknownClassifierLabelRoutesToFailed | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalRun_RealCLICompletesBatchInvocationWithPrimaryResultOnStdout | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_goal_run_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedGoalRun_RealCLIExitsAfterBatchCompletionWithoutContinuousMode | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_quorum_invocation_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedQuorumRun_RealCLIAcceptsRoleFlagsAndReturnsOneMergeResult | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_named_review_invocation_smoke_test.go | you-agent-factory/tests/functional/smoke | TestNamedReviewInvocationVariants_RealCLIRequireApprovalAfterRejection | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_run_mode_compat_smoke_test.go | you-agent-factory/tests/functional/smoke | TestRunModeCompat_RealCLIFactoryTextInvocationSuppressesOperatorChatter | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_run_mode_compat_smoke_test.go | you-agent-factory/tests/functional/smoke | TestRunModeCompat_RealCLINamedGoalBatchStdoutDoesNotIncludeOperatorChatter | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_run_mode_compat_smoke_test.go | you-agent-factory/tests/functional/smoke | TestRunModeCompat_RealCLIOperatorContinuousRunReportsStartupOutputWithoutQuiet | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_submit_batch_smoke_test.go | you-agent-factory/tests/functional/smoke | TestSubmitBatch_RealCLIUpsertsToRunningFactory | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cli_work_move_smoke_test.go | you-agent-factory/tests/functional/smoke | TestWorkMove_RealCLIMovesSubmittedWork | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cold_start_long_test.go | you-agent-factory/tests/functional/smoke | TestColdStart_SingleTokenReachesTerminal | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/cold_start_test.go | you-agent-factory/tests/functional/smoke | TestColdStart_MixedPreSeededAndLateSubmit | short | TBD | smoke | none | TBD |
| tests/functional/smoke/cold_start_test.go | you-agent-factory/tests/functional/smoke | TestColdStart_PreSeededTokensProcessed | short | TBD | smoke | none | TBD |
| tests/functional/smoke/config_driven_execution_test.go | you-agent-factory/tests/functional/smoke | TestConfigDrivenExecution_AddWorkType | short | TBD | smoke | none | TBD |
| tests/functional/smoke/config_driven_execution_test.go | you-agent-factory/tests/functional/smoke | TestConfigDrivenExecution_GlobalConfigDrivesDefaultsAndWorkerPreset | short | TBD | smoke | none | TBD |
| tests/functional/smoke/config_driven_execution_test.go | you-agent-factory/tests/functional/smoke | TestConfigDrivenExecution_HappyPath | short | TBD | smoke | none | TBD |
| tests/functional/smoke/config_driven_execution_test.go | you-agent-factory/tests/functional/smoke | TestConfigDrivenExecution_HappyPathFailureRouting | short | TBD | smoke | none | TBD |
| tests/functional/smoke/end_to_end_dispatch_test.go | you-agent-factory/tests/functional/smoke | TestEndToEndDispatch_CompletesThroughCustomerProcess | short | TBD | smoke | none | TBD |
| tests/functional/smoke/end_to_end_dispatch_test.go | you-agent-factory/tests/functional/smoke | TestEndToEndDispatch_MultipleWorkItemsCompleteIndependently | short | TBD | smoke | none | TBD |
| tests/functional/smoke/guarded_loop_breaker_long_test.go | you-agent-factory/tests/functional/smoke | TestIntegrationSmoke_GuardedLoopBreakerRoutesOverLimitExampleWorkToFailed | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/guarded_loop_breaker_test.go | you-agent-factory/tests/functional/smoke | TestIntegrationSmoke_GuardedLoopBreakerExampleRejectsRetiredExhaustionRulesAtBoundary | short | TBD | smoke | none | TBD |
| tests/functional/smoke/service_config_override_alignment_long_test.go | you-agent-factory/tests/functional/smoke | TestServiceConfigOverrideAlignment_CustomerProcessScriptCommandRunner | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/service_config_override_alignment_test.go | you-agent-factory/tests/functional/smoke | TestServiceConfigOverrideAlignment_CustomerProcessSharesScriptAndProviderCommandRunner | short | TBD | smoke | none | TBD |
| tests/functional/smoke/service_harness_smoke_long_test.go | you-agent-factory/tests/functional/smoke | TestCustomerProcess_HappyPath | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/service_harness_smoke_long_test.go | you-agent-factory/tests/functional/smoke | TestCustomerProcess_MultipleWorkItems | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/service_harness_smoke_long_test.go | you-agent-factory/tests/functional/smoke | TestCustomerProcess_NoopFallback | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/service_lifecycle_smoke_long_test.go | you-agent-factory/tests/functional/smoke | TestServiceLifecycle_CopyFixtureDirParallelCopiesStayIsolated | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/service_lifecycle_smoke_long_test.go | you-agent-factory/tests/functional/smoke | TestServiceLifecycle_EmptyPreseedDirectoryRemainsIdle | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/service_lifecycle_smoke_long_test.go | you-agent-factory/tests/functional/smoke | TestServiceLifecycle_InitInputCompletesThroughFactoryService | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/service_lifecycle_smoke_long_test.go | you-agent-factory/tests/functional/smoke | TestServiceLifecycle_PreseededWorkCompletesOnStartup | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/service_lifecycle_smoke_long_test.go | you-agent-factory/tests/functional/smoke | TestServiceLifecycle_TerminalStatusSignalsForSeededWatcherInput | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/service_lifecycle_smoke_long_test.go | you-agent-factory/tests/functional/smoke | TestServiceLifecycle_WorkFileSubmissionCompletesTwoStagePipeline | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/service_pipeline_config_behavior_test.go | you-agent-factory/tests/functional/smoke | TestServicePipelineConfigBehavior_SimplePipelineCompletesOneTask | short | TBD | smoke | none | TBD |
| tests/functional/smoke/service_pipeline_config_behavior_test.go | you-agent-factory/tests/functional/smoke | TestServicePipelineConfigBehavior_TwoStagePipelineCompletesAcrossBothWorkers | short | TBD | smoke | none | TBD |
| tests/functional/smoke/stateless_collector_long_test.go | you-agent-factory/tests/functional/smoke | TestStatelessCollector_MultipleWorkItems | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/stateless_collector_long_test.go | you-agent-factory/tests/functional/smoke | TestStatelessCollector_TwoStagePipeline | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/stateless_execution_long_test.go | you-agent-factory/tests/functional/smoke | TestStatelessExecution_DifferentWorkstationsResolveDifferentWorkers | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/stateless_execution_long_test.go | you-agent-factory/tests/functional/smoke | TestStatelessExecution_SharedExecutorResolvesDifferentWorkstations | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/stateless_loaded_config_smoke_long_test.go | you-agent-factory/tests/functional/smoke | TestStatelessExecutionSmoke_LoadedConfigDrivesExecution | functionallong | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestBackendCoverageAliasesSmoke_RedirectToIndependentLanes | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestBackendVerificationLaneScriptSmoke_PreservesFailureExitAndLog | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestBackendVerificationLaneScriptSmoke_UsesCanonicalOwnedCommandAndCapturesLog | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestConcurrentUIVerificationLanesScriptSmoke_DoesNotWaitForDetachedLogHandle | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestConcurrentUIVerificationLanesScriptSmoke_FailureReportsExactLaneRerun | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestConcurrentUIVerificationLanesScriptSmoke_RunsBothOwnedLanesConcurrently | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestLongTestsCommandSmoke_FailureReportsExactSpecialtyLaneRerun | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestShardedUICoverageScriptSmoke_CleansStaleVitestReportBlobsBeforeShards | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestShardedUICoverageScriptSmoke_FailureReportsExactShardRerun | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestShardedUICoverageScriptSmoke_RunsAllShardsThenMerge | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestUICoverageCommandSmoke_RunsPackageCoverageThenReplayCheck | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestUIPackageCoverageCommandSmoke_InvokesPackageOwnedCoverageScript | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestVerifyCompatibilityAliasSmoke_RedirectsToCanonicalPRTier | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestVerifyExtendedCommandSmoke_UsesOnlyExplicitLongSuitesAfterPRTier | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestVerifyFastCommandSmoke_ContractFailureStopsLaterSuites | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestVerifyFastCommandSmoke_FailureReportsOwnedSuiteAndRerunCommand | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestVerifyFastCommandSmoke_UsesOnlyShortOwnedSuites | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestVerifyPRCommandSmoke_FailureReportsExactLaneRerun | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestVerifyPRCommandSmoke_UsesRequiredLanesOnce | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestVerifyPRInferenceCommandSmoke_FailureReportsOwnedRerunCommand | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestVerifyPRInferenceCommandSmoke_RunsSingleNamedRegressionOnly | short | TBD | smoke | none | TBD |
| tests/functional/smoke/verify_fast_command_smoke_test.go | you-agent-factory/tests/functional/smoke | TestVerifyPRInferenceCommandSmoke_StaysOutsideRequiredPRAndExtendedTiers | short | TBD | smoke | none | TBD |

#### `work` (1 scenarios, catch_all=`none`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/work/visualization/dependency_graph_test.go | you-agent-factory/tests/functional/work/visualization | TestDependencyGraphVisualization_RendersCompleteEscapedFlowchart | short | TBD | none | none | TBD |

#### `workflow` (73 scenarios, catch_all=`workflow`)

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tests/functional/workflow/batch_ideation_pipeline_test.go | you-agent-factory/tests/functional/workflow | TestBatchIdeationPipeline_ConcurrencyLimit2 | short | TBD | workflow | none | TBD |
| tests/functional/workflow/batch_ideation_pipeline_test.go | you-agent-factory/tests/functional/workflow | TestSerialIdeationPipeline_ConcurrencyLimit1 | short | TBD | workflow | none | TBD |
| tests/functional/workflow/cli_ralph_init_smoke_test.go | you-agent-factory/tests/functional/workflow | TestIntegrationSmoke_RalphInitScaffoldCompletesFromGeneratedLoop | short | TBD | workflow | none | TBD |
| tests/functional/workflow/cli_ralph_init_smoke_test.go | you-agent-factory/tests/functional/workflow | TestIntegrationSmoke_RalphInitScaffoldExhaustsNonConvergingLoop | short | TBD | workflow | none | TBD |
| tests/functional/workflow/code_review_loop_long_test.go | you-agent-factory/tests/functional/workflow | TestCodeReviewLoop | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/config_driven_retry_loop_breaker_test.go | you-agent-factory/tests/functional/workflow | TestConfigDrivenRetryLoopBreaker_SucceedsBeforeLimit | short | TBD | workflow | none | TBD |
| tests/functional/workflow/config_driven_retry_loop_breaker_test.go | you-agent-factory/tests/functional/workflow | TestConfigDrivenRetryLoopBreaker_TerminatesAfterMaxRetries | short | TBD | workflow | none | TBD |
| tests/functional/workflow/conflict_resolution_test.go | you-agent-factory/tests/functional/workflow | TestConflictResolution_ResolverFails | short | TBD | workflow | none | TBD |
| tests/functional/workflow/conflict_resolution_test.go | you-agent-factory/tests/functional/workflow | TestConflictResolution_ReviewApproveFirstTry | short | TBD | workflow | none | TBD |
| tests/functional/workflow/conflict_resolution_test.go | you-agent-factory/tests/functional/workflow | TestConflictResolution_ReviewFailResolveReReview | short | TBD | workflow | none | TBD |
| tests/functional/workflow/dispatcher_lifecycle_long_test.go | you-agent-factory/tests/functional/workflow | TestDispatcherLifecycle_ExecutorFailure | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/dispatcher_lifecycle_test.go | you-agent-factory/tests/functional/workflow | TestDispatcherLifecycle_IdeaToArchive | short | TBD | workflow | none | TBD |
| tests/functional/workflow/dispatcher_lifecycle_test.go | you-agent-factory/tests/functional/workflow | TestDispatcherLifecycle_PlannerFailure | short | TBD | workflow | none | TBD |
| tests/functional/workflow/dispatcher_workflow_long_test.go | you-agent-factory/tests/functional/workflow | TestDispatcherWorkflow_ExecutionPoolIsolation | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/dispatcher_workflow_long_test.go | you-agent-factory/tests/functional/workflow | TestDispatcherWorkflow_MultipleSeedFiles | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/dispatcher_workflow_long_test.go | you-agent-factory/tests/functional/workflow | TestDispatcherWorkflow_ReviewFailurePerItem | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/dispatcher_workflow_long_test.go | you-agent-factory/tests/functional/workflow | TestDispatcherWorkflow_SingleSeedFile | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/dispatcher_workflow_long_test.go | you-agent-factory/tests/functional/workflow | TestDispatcherWorkflow_TwoSeedFiles | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/executor_context_long_test.go | you-agent-factory/tests/functional/workflow | TestExecutorContext_InputTokenColors | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/executor_context_long_test.go | you-agent-factory/tests/functional/workflow | TestExecutorContext_ParentLineage | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/executor_context_long_test.go | you-agent-factory/tests/functional/workflow | TestExecutorContext_RejectionFeedback | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/full_ideation_pipeline_long_test.go | you-agent-factory/tests/functional/workflow | TestFullIdeationPipeline_CrossWorkTypeLineage | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/full_ideation_pipeline_long_test.go | you-agent-factory/tests/functional/workflow | TestFullIdeationPipeline_HappyPath | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/full_ideation_pipeline_long_test.go | you-agent-factory/tests/functional/workflow | TestFullIdeationPipeline_RejectionLoop | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/idea_plan_review_execute_with_limits_long_test.go | you-agent-factory/tests/functional/workflow | TestIdeaPlanExecuteReviewWithLimitsFailsOnExecutorDueToRepeatingTooMuch | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/idea_plan_review_execute_with_limits_long_test.go | you-agent-factory/tests/functional/workflow | TestIdeaPlanExecuteReviewWithLimitsFailsOnExecutorFullPass | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/idea_plan_review_execute_with_limits_long_test.go | you-agent-factory/tests/functional/workflow | TestIdeaPlanExecuteReviewWithLimitsFailsOnIdeation | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/idea_plan_review_execute_with_limits_long_test.go | you-agent-factory/tests/functional/workflow | TestIdeaPlanExecuteReviewWithLimitsFailsOnScriptExecution | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/idea_plan_review_execute_with_limits_long_test.go | you-agent-factory/tests/functional/workflow | TestIdeaPlanExecuteReviewWithLimits_TraceLineageAndOutcomes | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/idea_to_prd_long_test.go | you-agent-factory/tests/functional/workflow | TestIdeaToPRD_CrossWorkTypeOutput | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/idea_to_prd_long_test.go | you-agent-factory/tests/functional/workflow | TestIdeaToPRD_MultipleIdeas | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/idea_to_prd_long_test.go | you-agent-factory/tests/functional/workflow | TestIdeaToPRD_PlannerFailure | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/logical_move_long_test.go | you-agent-factory/tests/functional/workflow | TestLogicalMove_PreservesTokenColor | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/logical_move_long_test.go | you-agent-factory/tests/functional/workflow | TestLogicalMove_Success | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/multi_output_color_propagation_long_test.go | you-agent-factory/tests/functional/workflow | TestMultiOutputColorPropagation | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/multi_output_color_propagation_long_test.go | you-agent-factory/tests/functional/workflow | TestMultiOutputColorPropagation_NameAvailableDownstream | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/multi_output_color_propagation_test.go | you-agent-factory/tests/functional/workflow | TestDocReviewerExamplePNGFanoutPreservesSharedNameDownstream | short | TBD | workflow | none | TBD |
| tests/functional/workflow/multi_output_color_propagation_test.go | you-agent-factory/tests/functional/workflow | TestMultiOutputReviewerFanoutPreservesSharedNameDownstream | short | TBD | workflow | none | TBD |
| tests/functional/workflow/multi_output_color_propagation_test.go | you-agent-factory/tests/functional/workflow | TestNtoN_TypeMatching | short | TBD | workflow | none | TBD |
| tests/functional/workflow/multi_output_long_test.go | you-agent-factory/tests/functional/workflow | TestMultiOutput_NoStopWordsConfigured | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/multi_output_long_test.go | you-agent-factory/tests/functional/workflow | TestMultiOutput_OutputTokensInheritInputLineage | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/multi_output_long_test.go | you-agent-factory/tests/functional/workflow | TestMultiOutput_SecondStopWord | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/multi_output_long_test.go | you-agent-factory/tests/functional/workflow | TestMultiOutput_WithStopWord | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/multi_output_long_test.go | you-agent-factory/tests/functional/workflow | TestMultiOutput_WithoutStopWord | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/name_propagation_long_test.go | you-agent-factory/tests/functional/workflow | TestNamePropagation_InPromptTemplate | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/name_propagation_long_test.go | you-agent-factory/tests/functional/workflow | TestNamePropagation_MarkdownFile | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/ralph_loop_long_test.go | you-agent-factory/tests/functional/workflow | TestRalphLoop_TemplateFieldsResolvePerIteration | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/ralph_loop_test.go | you-agent-factory/tests/functional/workflow | TestRalphLoop_ConvergesOnReviewerAccept | short | TBD | workflow | none | TBD |
| tests/functional/workflow/rejection_path_test.go | you-agent-factory/tests/functional/workflow | TestRejectionPath_NoRejectionArcsFailsToken | short | TBD | workflow | none | TBD |
| tests/functional/workflow/rejection_path_test.go | you-agent-factory/tests/functional/workflow | TestRejectionPath_NoRejectionArcsFailureRecordSet | short | TBD | workflow | none | TBD |
| tests/functional/workflow/rejection_path_test.go | you-agent-factory/tests/functional/workflow | TestRejectionPath_NoRejectionArcsReleasesResources | short | TBD | workflow | none | TBD |
| tests/functional/workflow/rejection_path_test.go | you-agent-factory/tests/functional/workflow | TestRejectionPath_WithRejectionArcsRoutesViaArcs | short | TBD | workflow | none | TBD |
| tests/functional/workflow/repeater_parameterized_long_test.go | you-agent-factory/tests/functional/workflow | TestRepeater_GuardedLoopBreakerTerminatesRejectedRepeater | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/repeater_parameterized_long_test.go | you-agent-factory/tests/functional/workflow | TestRepeater_RefiresOnRejectedStopsOnAccepted | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/repeater_parameterized_long_test.go | you-agent-factory/tests/functional/workflow | TestRepeater_ResourceReleaseBetweenIterations_ServiceHarness | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/repeater_parameterized_test.go | you-agent-factory/tests/functional/workflow | TestParameterizedFields_UnresolvedTemplateRoutesToFailure | short | TBD | workflow | none | TBD |
| tests/functional/workflow/repeater_parameterized_test.go | you-agent-factory/tests/functional/workflow | TestParameterizedFields_WorkingDirectoryResolvesFromTags | short | TBD | workflow | none | TBD |
| tests/functional/workflow/repeater_parameterized_test.go | you-agent-factory/tests/functional/workflow | TestRepeater_ResourceReleaseBetweenIterations | short | TBD | workflow | none | TBD |
| tests/functional/workflow/repeater_parameterized_test.go | you-agent-factory/tests/functional/workflow | TestRepeater_YieldsBetweenIterations | short | TBD | workflow | none | TBD |
| tests/functional/workflow/review_retry_exhaustion_long_test.go | you-agent-factory/tests/functional/workflow | TestReviewRetryLoopBreaker_FeedbackPropagated | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/review_retry_exhaustion_long_test.go | you-agent-factory/tests/functional/workflow | TestReviewRetryLoopBreaker_SucceedsBeforeLimit | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/review_retry_exhaustion_long_test.go | you-agent-factory/tests/functional/workflow | TestReviewRetryLoopBreaker_TerminatesAfterMaxRetries | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/watcher_multichannel_submission_long_test.go | you-agent-factory/tests/functional/workflow | TestMultiChannelFileWatcher_DefaultSubmission | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/watcher_multichannel_submission_long_test.go | you-agent-factory/tests/functional/workflow | TestMultiChannelFileWatcher_DynamicExecDir | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/watcher_multichannel_submission_long_test.go | you-agent-factory/tests/functional/workflow | TestMultiChannelFileWatcher_ExecutionIDSubmission | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/watcher_seed_submission_long_test.go | you-agent-factory/tests/functional/workflow | TestFileWatcherFlowConcurrent | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/watcher_seed_submission_long_test.go | you-agent-factory/tests/functional/workflow | TestFileWatcherFlowNoTokenLeaks | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/watcher_seed_submission_long_test.go | you-agent-factory/tests/functional/workflow | TestFileWatcherFlowSequential | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/watcher_seed_submission_long_test.go | you-agent-factory/tests/functional/workflow | TestFileWatcherFlowSingle | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/workflow_modification_long_test.go | you-agent-factory/tests/functional/workflow | TestWorkflowModificationAndReload | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/workflow_modification_long_test.go | you-agent-factory/tests/functional/workflow | TestWorkflowModificationPreservesIndependentWorkflows | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/workflow_modification_long_test.go | you-agent-factory/tests/functional/workflow | TestWorkflowModificationRejectionLoop | functionallong | TBD | workflow | none | TBD |
| tests/functional/workflow/workstation_stopwords_long_test.go | you-agent-factory/tests/functional/workflow | TestWorkstationStopWords_ThroughCustomerProcess | functionallong | TBD | workflow | none | TBD |

### Non-customer harness exclusions

These paths are excluded from the customer scenario count. They are listed
explicitly so completeness accounting does not silently omit them.

#### Helper-only files (no top-level customer `Test*`)

| path | rationale |
| --- | --- |
| tests/functional/bootstrap_portability/cli_relative_working_directory_functionallong_helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/bootstrap_portability/export_import_harness_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/bootstrap_portability/helpers_functionallong_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/bootstrap_portability/helpers_http_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/bootstrap_portability/helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/bootstrap_portability/portability_functionallong_helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/bootstrap_portability/work_assertions_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/cli/mcp_resume/stdio_client_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/guards_batch/helpers_long_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/guards_batch/helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/providers/cli_timeout_cleanup_process_unix_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/providers/cli_timeout_cleanup_process_windows_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/providers/helpers_long_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/providers/helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/replay_contracts/helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/replay_contracts/replay_live_helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/replay_contracts/replay_process_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/replay_contracts/short_helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/api_batch_submission_boundary_smoke_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/api_inference_events_thin_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/api_runtime_config_alignment_smoke_support_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/api_runtime_config_alignment_support_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/events_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/external_support_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/factory_transformation/api_current_factory_put_helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/functional_server_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/runtime_support_long_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/runtime_support_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/runtime_api/short_helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/smoke/helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/smoke/packaged_goal_public_contract_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/smoke/service_pipeline_config_helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |
| tests/functional/workflow/helpers_test.go | Helper-only _test.go with no top-level customer Test* scenarios (shared helpers, fixtures, or TestMain-only). |

#### `tests/functional/internal/**` harness files

| path | top-level `Test*` present (excluded) | rationale |
| --- | --- | --- |
| tests/functional/internal/restclient/adapter_test.go | `TestAdapterUsesCallerBaseURLAndHTTPClient`, `TestNewRejectsInvalidConfiguration` | tests/functional/internal/** is shared harness / support verification, not a customer scenario owner. |
| tests/functional/internal/support/deadcode_contract_test.go | `TestAcceptedCommandResults_ReturnsRequestedCompleteResponses`, `TestCountFactoryEvents_CountsMatchingEventTypes`, `TestFactoryRelationsValue_ReturnsNilAndPopulatedRelations`, `TestNewStaticSuccessCommandRunner_ReturnsFixedStdoutWithoutFailureFields`, `TestProviderCommandRunnerRecordsPolicyFreeProcessRequests`, `TestUpdateFactoryConfig_RewritesScaffoldedFactoryConfig` | tests/functional/internal/** is shared harness / support verification, not a customer scenario owner. |
| tests/functional/internal/support/dispatch_observation_test.go | `TestObserveDispatchEvents_ResponseOnlyRetainsPublicTransitionAndWorkIdentity` | tests/functional/internal/** is shared harness / support verification, not a customer scenario owner. |
| tests/functional/internal/support/root_run_host_test.go | `TestRootRunFunctionalHostContextCancellationCompletesAndReleasesListener`, `TestRootRunFunctionalHostReportsOccupiedAddressAndAllowsReuse`, `TestRootRunFunctionalHostReportsReadinessDeadlineAtCustomerBoundary`, `TestRootRunFunctionalHostShutdownIsBoundedAndIdempotent`, `TestRootRunFunctionalHostStartsThroughCustomerRESTAndSSE` | tests/functional/internal/** is shared harness / support verification, not a customer scenario owner. |
| tests/functional/internal/support/work_session_paths_test.go | `TestDefaultSessionEventsURL_UsesCanonicalSessionScopedRoute` | tests/functional/internal/** is shared harness / support verification, not a customer scenario owner. |
---

## runtime_api deletion-only batches

_Status: empty — filled by FND-007-003._

### Move/split plan

`runtime_api` is deletion-only debt. Every current customer scenario maps to
exactly one checklist destination or approved wrong-layer rationale, then is
grouped into named deletion-only batch ids that later work can execute
independently until package ownership reaches zero.

### Scenario rows

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| _TBD_ | _TBD_ | _TBD_ | _TBD_ | TBD | runtime_api | _TBD_ | TBD |

---

## smoke and workflow split plans

_Status: empty — filled by FND-007-004._

### smoke plan

_TBD: which scenarios move together, which split across domains, which become
deletion-only once coverage exists elsewhere._

### workflow plan

_TBD: which scenarios move together, which split across domains, which become
deletion-only once coverage exists elsewhere._

### Scenario rows

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| _TBD_ | _TBD_ | _TBD_ | _TBD_ | TBD | smoke\|workflow | _TBD_ | TBD |

---

## guards_batch, bootstrap_portability, and replay_contracts split plans

_Status: empty — filled by FND-007-005._

### guards_batch plan

_TBD: durable domain owners (for example `guards`, `resources`, `resilience`)._

### bootstrap_portability plan

_TBD: durable domain owners (for example `factory/portability`)._

### replay_contracts plan

_TBD: durable domain owners (for example `events/replay`)._

### Scenario rows

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| _TBD_ | _TBD_ | _TBD_ | _TBD_ | TBD | guards_batch\|bootstrap_portability\|replay_contracts | _TBD_ | TBD |

---

## Remaining packages and wrong-layer approvals

_Status: empty — filled by FND-007-006._

Covers every remaining customer scenario outside the six named catch-alls
(including `cli/**`, `providers/**`, `acceptance`, `sessionparity`, `work/**`,
`models/**`, `operator_settings/**`, `config_init`, and any other live
packages).

### Scenario rows

| source_path | package | scenario | lane | destination | catch_all | specialty_targets | deletion_only_batch |
| --- | --- | --- | --- | --- | --- | --- | --- |
| _TBD_ | _TBD_ | _TBD_ | _TBD_ | TBD | none | _TBD_ | n/a |

### Approved wrong-layer cases

| scenario | wrong-layer rationale | replacement evidence owner |
| --- | --- | --- |
| _TBD_ | _TBD_ | _TBD_ |

---

## Specialty Make target bindings

_Status: skeleton — filled as catch-all and completeness stories record bindings._

Each specialty Make target that currently selects a functional package or
`-run` pattern records its current binding and intended post-move
package/path binding. Do not change Makefile behavior in FND-007 unless a tiny
documentation comment is required to point at this ledger.

| Make target | Current binding | Intended post-move binding | Notes |
| --- | --- | --- | --- |
| `api-smoke` | TBD | TBD | Includes runtime_api generated-API smoke selector today |
| `docs-reference-smoke` | TBD | TBD | Includes `tests/functional/smoke` docs command selector today |
| `cron-time-work-smoke` | TBD | TBD | Includes runtime_api cron selector today |
| `current-factory-watcher-switch-smoke` | TBD | TBD | Includes bootstrap_portability selector today |
| `release-surface-smoke` / artifact closeout functional selectors | TBD | TBD | Includes runtime_api and replay_contracts selectors via closeout |
| `long-tests-managed-runtime` / related long selectors | TBD | TBD | Record actual Makefile bindings; preserve coverage intent |
| `pr-inference-approval` | TBD | TBD | runtime_api long-tag selector today |

---

## Deletion-only batch index

_Status: empty — filled by FND-007-003…007._

Ordered list of named deletion-only batches that later move work can consume
without inventing destinations. Prefer independent, reviewable batch sizes.

| batch_id | source catch-all | scenario count | destination domains | status |
| --- | --- | ---: | --- | --- |
| _TBD_ | _TBD_ | TBD | _TBD_ | planned |

---

## Completeness audit

_Status: empty — filled by FND-007-007._

| Check | Result |
| --- | --- |
| Unmapped customer scenarios vs fresh `tests/functional` inventory | TBD |
| Destination paths exist in `test-file-checklist.md` or approved wrong-layer | TBD |
| Short/long membership preserved on every row | TBD |
| Specialty Make targets fully accounted for | TBD |
| Deletion-only batch index covers runtime_api + featureless catch-alls | TBD |
| `make pkg-structure` | TBD |
