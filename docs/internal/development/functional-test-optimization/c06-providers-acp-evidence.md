# C06 ACP provider functional evidence

## Gate and scope

- Story: `functional-test-optimization-c06-providers-acp-001`
- Gates: `DISC-001/CHAR-001`
- Recorded: `2026-08-28` on commit `67710223e327d02c0de93a6ad826c754fe5c1702`
- Scope: characterization and classification only; no migration, production,
  contract, shared-support, c01, remote, or paid-provider changes.
- Dependency fidelity: local-real root composition with controlled ACP child
  processes at the `PlatformProcessCommandFactory` edge.
- Status: complete for this story. Migration parity, lifecycle hardening,
  repeat/race evidence, topology-after evidence, PR timing, and clean-room
  validation remain owned by later stories.

The requested `progress.txt` and the PRD-referenced
`docs/temp/functional-test-optimization.md` parent plan are absent from this
worktree. The checked-in `prd.json`, the c01 inventory, and the current test
bodies were used as the available authority; the missing files are recorded so
this lane does not silently claim parent-plan or prior-progress evidence.

## Discovery and baseline run

| Property | Observation |
| --- | --- |
| Package | `./tests/functional/providers/acp` |
| Top-level identities | 43 from `go test -list '^Test'` |
| Go JSON parent records | 43 |
| Go JSON child records | 31: protocol `2`, invalid catalog `5`, packaged profiles `20`, golden failures `2`, permissions `2` |
| Executed Go test records | 74 (`43 + 31`); all passed |
| Root constructions | 42, from the source-call audit below |
| ACP peer starts | 36 expected/observed through package-local atomic start counters: 31 functional peers and 5 golden-wire peers |
| Package elapsed | 57 seconds from the terminal package JSON event |
| Go / OS | `go1.25.0 windows/amd64` |
| GOMOD | `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\functional-test-optimization-c06-providers-acp\go.mod` |
| GOCACHE | `C:\Users\andre\AppData\Local\go-build` |
| GOFLAGS / GOAMD64 | empty / `v1` |

Exact procedures and observed results:

```text
go test -list '^Test' ./tests/functional/providers/acp
exit 0; 43 top-level test identities printed

go test -json -count=1 -timeout=15m ./tests/functional/providers/acp
exit 0; terminal package event elapsed=57; 43 parent records and 31 child records; no fail or skip records
```

The JSON run is the uncached test-run baseline (`-count=1`) and was executed
against the recorded commit. It proves the current package inventory and
current behavior pass at the local-real functional boundary. It does not prove
post-migration parity, race freedom, remote ACP behavior, or a clean host-wide
process census.

The post-run host snapshot contained unrelated `go`, `gopls`, `unitlane`, and
`you.exe` processes. Therefore this artifact makes no global claim that the
host had zero processes after the run. Package-owned cleanup remains a
source-level and test-owned observation for `LIFE-004`: the factory helpers
register process/server cleanup, daemon tests stop their server, packaged
process tests stop before temporary-directory cleanup, and the baseline exited
without a hang or failure.

## Topology audit

The baseline package has 39 top-level identities that enter a root-built
application boundary. Three helper targets do not build a root:
`TestACPAgentHelperProcess`, `TestACPGoldenRPCPeerProcess`, and
`TestPackagedACPUnexpectedCommand`; `TestPinnedACPSDKGoldenManifestIsCompleteAndParseable`
is root-free asset validation. The protocol, golden-failure, and permission
tables add one extra root per child case, yielding `39 + 3 = 42` root
constructions.

The 36 ACP peer starts are the exact package-owned start-counter expectations:

- 31 starts use the raw JSON-RPC `functionalRPCPeer` through
  `TestACPAgentHelperProcess`. This includes the ordinary, failure,
  packaged-conformance, spawn, and tournament modes.
- 5 starts use the pinned-wire `goldenRPCPeer` through
  `TestACPGoldenRPCPeerProcess`: two RPC-failure children, one success stream,
  and two permission children.
- Zero-start witnesses remain explicit for command-start failure, unavailable
  executable, root construction, unknown provider, SCRIPT_WRAP, JavaScript
  mock workers, catalog mutations, manifest/asset checks, and helper targets.
- Two-start witnesses remain explicit for retry/session continuation, crash
  replacement, and disconnected-connection replacement. The retry and daemon
  tests retain one root and deliberately own the two peer identities.

This is a reproducible source-and-counter topology baseline, not a new runtime
counter. `TOPO-007` owns direct before/after instrumentation after migration.

## Case-level classification rules

- `shareable`: no process, connection, environment, command-selection, or
  session identity is the property under test; the root-free asset or settings
  behavior can share the package spine.
- `shareable-with-mock`: the public root, Factory Session, Work, Factory Event,
  response stream, and/or Provider Session behavior is the witness, while the
  ACP external effect is a controlled command-runner subprocess at
  `edges.Edges`. Sharing does not erase a public assertion.
- `isolated-with-reason`: a fresh process, stdio connection, command,
  environment, protocol negotiation, crash/replacement, shutdown, stderr, or
  helper-process identity is the behavior under test. The reason below names
  the exact property that sharing would destroy.

The c01 P028 inventory recorded most ACP rows as a generic
`isolated-with-reason: providers-acp-protocol-boundaries`. C06 replaces that
coarse label with the actual witness below. No row is left without one of the
three classifications or an explicit conditional/inherited disposition.

## Go test identity and expansion map

The following table accounts for every top-level identity. The child list is
also the exact 31-record expansion observed in JSON output; a top-level row
with no child list has one parent execution record.

| # | Top-level identity (source) | Child / matrix accounting | Actual dependency witness | Classification and disposition |
| ---: | --- | --- | --- | --- |
| 1 | `TestACPCommandStartFailureMapsToDependencyFailure` (`acp_error_test.go`) | ACP-032 | Root-built Work run; OS refuses the configured executable; zero peer process. | isolated-with-reason: OS start; retain |
| 2 | `TestACPFailureRedactsConfiguredSecretsFromStderr` (`acp_error_test.go`) | ACP-033 | Root-built Work run; one raw peer writes stderr; failure is redacted. | isolated-with-reason: subprocess stderr; retain |
| 3 | `TestACPAgentSelfReportedCancellationMapsToCanceledFailure` (`acp_error_test.go`) | ACP-034 | Root-built Work run; one controlled peer returns `StopReasonCancelled` after partial output. | shareable-with-mock; migrate |
| 4 | `TestACPProtocolFailuresMapToStableWorkerFailureClasses` (`acp_error_test.go`) | `/version` -> ACP-035; `/fail` -> ACP-036 | Two child roots and two raw peers distinguish negotiation from generic protocol failure. | aggregate: isolated-with-reason for ACP-035; shareable-with-mock for ACP-036; split/retain |
| 5 | `TestUnavailableACPExecutableFailsBeforeStartWithMissingExecutableClass` (`acp_error_test.go`) | ACP-037 | Root-built Work run; executable locator rejects before command start; zero peer. | isolated-with-reason: executable lookup; retain |
| 6 | `TestACPUpdatesPublishExistingFactorySessionResponseEventsInOrder` (`acp_provider_events_test.go`) | ACP-011, ACP-053, ACP-054 | One raw peer; public response stream, Worker Session source records, Provider Session, and Factory replay are inspected. | shareable-with-mock; migrate |
| 7 | `TestACPFailurePublishesTerminalErrorEvent` (`acp_provider_events_test.go`) | ACP-012 | One raw peer emits partial output then an RPC failure; failed Work and terminal ERROR are inspected. | shareable-with-mock; migrate |
| 8 | `TestACPAuthenticationRequiredMapsToCanonicalWorkerFailure` (`acp_provider_events_test.go`) | ACP-013 | One raw peer advertises auth methods and rejects `session/new`; typed public failure is inspected. | shareable-with-mock; migrate |
| 9 | `TestACPModelIsAppliedOnlyThroughAdvertisedSessionConfig` (`acp_provider_events_test.go`) | ACP-014 | One raw peer advertises a model option and verifies the configured session option. | shareable-with-mock; migrate |
| 10 | `TestACPReceivesCanonicalWorkResourceAsSDKResourceLink` (`acp_provider_events_test.go`) | ACP-015 | One raw peer verifies the canonical image resource-link prompt block. | shareable-with-mock; migrate |
| 11 | `TestFactoryRunRoutesExecutorProviderThroughACPAdapter` (`basic_factory_run_test.go`) | ACP-001 | One raw peer; Work completion, no legacy provider call, and Provider Session are inspected. | shareable-with-mock; migrate |
| 12 | `TestFactoryRunRetriesACPProviderByResumingExactSession` (`basic_factory_run_test.go`) | ACP-002 | One root, two raw peers; first failure, exact opaque session load, and two starts are required. | isolated-with-reason: restart and session continuation; retain |
| 13 | `TestFactoryRunRetainsLegacyNamedExecutorProviderCompatibility` (`basic_factory_run_test.go`) | ACP-003 | One raw peer through the legacy executor spelling. | shareable-with-mock; migrate |
| 14 | `TestFactoryRunProjectsOperatorConfiguredACPIntegrationIntoInvocationCatalog` (`basic_factory_run_test.go`) | ACP-004 | One configured raw peer; operator catalog selection and Provider Session are inspected. | shareable-with-mock; migrate |
| 15 | `TestRootConstructionDoesNotStartACPProcess` (`basic_factory_run_test.go`) | ACP-005 | Root construction only; command factory start counter must remain zero. | isolated-with-reason: startup boundary; retain |
| 16 | `TestUnknownExecutorProviderFailsBeforeACPProcessStart` (`basic_factory_run_test.go`) | ACP-006 | Root-built Work run rejects provider identity before ACP or fallback invocation; zero peer. | isolated-with-reason: pre-start boundary; retain |
| 17 | `TestScriptWrapExecutorProviderRetainsLegacyProviderRoute` (`basic_factory_run_test.go`) | ACP-007 | Root-built Work run uses the injected legacy route; zero ACP starts. | shareable-with-mock; migrate |
| 18 | `TestACPAgentHelperProcess` (`basic_factory_run_test.go`) | ACP-051 | Child-process target is inert without its parent environment; parent tests own all protocol assertions. | isolated-with-reason: helper-process boundary; retain as helper |
| 19 | `TestBTRCP0ACPTargetSuccessCharacterization` (`btrc_p0_characterization_test.go`) | ACP-016 | One raw peer; exact Factory Event order, Work/session projection, Provider Session, and response terminal. | shareable-with-mock; migrate as parity witness |
| 20 | `TestBTRCP0ACPTargetProtocolFailureCharacterization` (`btrc_p0_characterization_test.go`) | ACP-017 | One raw peer; exact failure event order, typed failure, failed Work/session projection, and response error. | shareable-with-mock; migrate as parity witness |
| 21 | `TestRootBuiltACPCommandsRejectInvalidMutationsWithoutPersistingSettings` (`catalog_cli_negative_test.go`) | `missing_name`, `non_canonical_name`, `unsupported_transport`, `empty_command`, `missing_delete_name` -> ACP-018..022 | One root, five command subcases, no provider command; invalid mutations leave settings absent. | shareable; migrate |
| 22 | `TestRootBuiltACPCommandsAddDeleteAndUnifiedListOneSettingsBackedCatalogEntry` (`catalog_cli_test.go`) | ACP-023 | One root and repeated `Process.Execute` calls over one settings home; no provider process. | shareable; migrate |
| 23 | `TestYouInitMaterializesPackagedACPDefaultsAndPreservesCustomEntries` (`catalog_cli_test.go`) | ACP-024 | One root and repeated init/add commands; packaged defaults and custom entry persist without duplication. | shareable; migrate |
| 24 | `TestProvidersACPSerializesConcurrentPromptsOnOneStdioConnection` (`daemon_concurrency_test.go`) | ACP-025 | One daemon root, one held raw peer, two prompts; connection serialization is the witness. | isolated-with-reason: connection serialization; retain |
| 25 | `TestProvidersACPRestartsAfterCrashWithoutReplayingUncertainPrompt` (`daemon_crash_recovery_test.go`) | ACP-026 | One daemon root, two raw peers; intentional crash, failed first execution, recovered second execution, two starts. | isolated-with-reason: crash and replacement; retain |
| 26 | `TestProvidersACPRetiresDisconnectedConnectionBeforeReuse` (`daemon_crash_recovery_test.go`) | ACP-027 | One daemon root, two raw peers; response-ready, stdout disconnect, retirement, and replacement are asserted. | isolated-with-reason: connection retirement; retain |
| 27 | `TestProvidersACPRetainsOneOSProcessAndConnectionAcrossExecutions` (`daemon_reuse_test.go`) | ACP-028 | One daemon root and one persistent raw peer serve two sequential executions; one start is required. | isolated-with-reason: process and connection identity; retain |
| 28 | `TestProvidersACPRejectsIncompatibleProtocolVersionAtStdioBoundary` (`daemon_reuse_test.go`) | ACP-029 | Fresh daemon root and one raw peer negotiate version `999`; durable execution fails at stdio. | isolated-with-reason: initialization negotiation; retain |
| 29 | `TestProvidersShutdownCancelsActivePromptAndJoinsACPProcess` (`daemon_shutdown_test.go`) | ACP-030 | One daemon root and one blocking raw peer; shutdown cancels active prompt and joins. | isolated-with-reason: shutdown join; retain |
| 30 | `TestPinnedACPSDKGoldenManifestIsCompleteAndParseable` (`golden_fixture_test.go`) | ACP-031, ACP-054 | Root-free tracked assets; checksums, JSON, uniqueness, and manifest counts. | shareable; retain |
| 31 | `TestACPGoldenRPCPeerProcess` (`golden_rpc_peer_test.go`) | ACP-052 | Child-process target is inert without its parent golden mode; parent tests own wire assertions. | isolated-with-reason: helper-process boundary; retain as helper |
| 32 | `TestJavaScriptFactoryAgentRunRoutesExecutorProviderThroughACP` (`javascript_factory_run_test.go`) | ACP-008 | One root-built `Process.Execute`, one raw peer, JavaScript output, route, and Provider Session. | shareable-with-mock; migrate |
| 33 | `TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected` (`javascript_factory_run_test.go`) | ACP-009 | One root-built `Process.Execute`; MockWorkers path intentionally makes zero live provider calls. | shareable-with-mock; migrate without weakening the assertion |
| 34 | `TestFactoryMixesACPAndScriptWrapWorkersWithoutCrossRouting` (`mixed_provider_factory_test.go`) | ACP-010 | One root-built Work run; one raw ACP peer and one injected legacy provider route complete independently. | shareable-with-mock; migrate |
| 35 | `TestPackagedACPProfilesUseSharedConformanceBehavior` (`packaged_conformance_test.go`) | 20 profile children -> ACP-039; parent asset count -> ACP-038; one runtime execution -> ACP-040 | Twenty root-free fixture checks plus one root-built run with one raw peer and an allowlisted packaged command. | aggregate: shareable for ACP-038/039; isolated-with-reason for ACP-040 command projection; retain/migrate only the eligible asset portion |
| 36 | `TestPackagedACPUnexpectedCommand` (`packaged_conformance_test.go`) | ACP-041 | Empty child target used when the command allowlist is violated; it must not launch an ambient executable. | isolated-with-reason: helper/process command boundary; retain as helper |
| 37 | `TestPackagedSpawnRunsPlannerChildrenAndMergerThroughPersistentACPStdio` (`packaged_spawn_test.go`) | ACP-042 | One packaged root, one persistent raw peer, four ACP agent sessions, merged output, explicit server stop. | isolated-with-reason: persistent connection count; retain |
| 38 | `TestPackagedTournamentRunsCompetitorsAndJudgeThroughPersistentACPStdio` (`packaged_tournament_test.go`) | ACP-043 | One packaged root, one persistent raw peer, three agent sessions, champion/rationale, explicit server stop. | isolated-with-reason: persistent connection count; retain |
| 39 | `TestYouRunMapsGoldenSessionAndConfigRPCFailuresToTerminalWork` (`run_failure_diagnostics_test.go`) | `new-fail` -> ACP-044; `config-fail` -> ACP-045 | Two child roots and two golden-wire peers; sanitized RPC diagnostics and failed Work are asserted. | isolated-with-reason: pinned wire peer; retain |
| 40 | `TestYouRunSendsInputWorkContentAsACPText` (`run_parameters_content_test.go`) | ACP-046 | One raw peer verifies the input Work sentinel arrives as ACP text. | shareable-with-mock; migrate |
| 41 | `TestYouRunUsesPinnedACPWireGoldensAndProjectsTerminalOutput` (`run_parameters_content_test.go`) | ACP-047, ACP-053 | One golden-wire peer; pinned request fixtures, Provider Session, exact response NDJSON order, and terminal output. | isolated-with-reason: pinned wire transcript; retain |
| 42 | `TestYouRunMapsSkipPermissionsToSDKGoldenPermissionSelection` (`run_permissions_test.go`) | `default_rejects` -> ACP-048; `skipPermissions_allows` -> ACP-049 | Two child roots and two golden-wire peers; exact permission selection is the wire witness. | isolated-with-reason: pinned permission wire; retain |
| 43 | `TestYouRunReturnsUnsupportedFilesystemAndTerminalRPCsAtTheACPBoundary` (`run_unsupported_capabilities_test.go`) | ACP-050 | One raw peer initiates filesystem/terminal RPCs; unsupported responses and completed prompt are asserted. | isolated-with-reason: bidirectional protocol RPC; retain |

Child-record expansion: `TestACPProtocolFailuresMapToStableWorkerFailureClasses/{version,fail}`;
`TestRootBuiltACPCommandsRejectInvalidMutationsWithoutPersistingSettings/{missing_name,non_canonical_name,unsupported_transport,empty_command,missing_delete_name}`;
`TestPackagedACPProfilesUseSharedConformanceBehavior/{copilot-acp,cursor-acp,droid-acp,fast-agent-acp,gemini-acp,grok-build-acp,iflow-acp,kilocode-acp,kimi-acp,kiro-acp,mux-acp,openclaw-acp,opencode-acp,pi-acp,pool-acp,qoder-acp,qwen-acp,reasonix-acp,trae-acp,zeroclaw-acp}`;
`TestYouRunMapsGoldenSessionAndConfigRPCFailuresToTerminalWork/{new-fail,config-fail}`;
and `TestYouRunMapsSkipPermissionsToSDKGoldenPermissionSelection/{default_rejects,skipPermissions_allows}`.

## ACP-001 through ACP-056 behavior ledger

The following ledger is the c06 case matrix. It preserves the executable
given/when/then outcome while assigning the actual witness classification. ACP-
053 and ACP-054 are intentional cross-witness observations attached to the
owning tests above; ACP-055 and ACP-056 are explicit conditional/inherited
cleanup decisions rather than silently invented new tests.

| ID | Given / when / then | Actual witness and classification | Disposition |
| --- | --- | --- | --- |
| ACP-001 | Given a Worker selects `cursor-acp`; when one Work runs; then it completes, the legacy provider is unused, and one Provider Session is recorded. | Root, public Work/Factory Event, controlled raw peer; shareable-with-mock. | migrate |
| ACP-002 | Given a retryable ACP error retains an opaque session ID; when retry runs; then the first peer fails, a replacement loads that exact session, Work succeeds, and starts equal two. | Fresh peer/process replacement and session continuation; isolated-with-reason: restart and session continuation. | retain |
| ACP-003 | Given a Worker uses the legacy ACP executor spelling; when Work runs; then ACP completes once. | Root plus controlled raw peer; shareable-with-mock. | migrate |
| ACP-004 | Given settings define `custom-acp`; when Work runs; then custom selection and its Provider Session are recorded. | Root, operator settings, controlled raw peer; shareable-with-mock. | migrate |
| ACP-005 | Given a root is constructed; when no command executes; then no ACP peer starts. | Root construction/start counter; isolated-with-reason: startup boundary. | retain |
| ACP-006 | Given an unknown ACP provider is named; when Work runs; then it fails before ACP or fallback start. | Provider selection before process start; isolated-with-reason: pre-start boundary. | retain |
| ACP-007 | Given `SCRIPT_WRAP` is selected while ACP exists; when Work runs; then legacy runs once and ACP starts zero times. | Root plus injected legacy edge and zero ACP start counter; shareable-with-mock. | migrate |
| ACP-008 | Given a JavaScript Factory selects ACP; when `Process.Execute` runs it; then output contains completion and Provider Session evidence. | Root, JavaScript public process, controlled raw peer; shareable-with-mock. | migrate |
| ACP-009 | Given a JavaScript Factory uses MockWorkers with ACP selected; when it runs; then it succeeds with zero live provider calls. | Root and existing MockWorkers behavior, zero live process/runner calls; shareable-with-mock. | migrate without weakening assertion |
| ACP-010 | Given one Factory has ACP and SCRIPT_WRAP Work; when both dispatch; then both complete and each route is called once. | Root, one controlled ACP peer, one injected legacy edge; shareable-with-mock. | migrate |
| ACP-011 | Given a controlled peer emits supported updates; when Work runs; then response events and Worker Session records preserve order, provenance, Provider Session identity, and replay separation. | Raw peer response stream plus public Worker Session/Factory Event observations; shareable-with-mock. | migrate |
| ACP-012 | Given a peer emits partial output then fails; when Work runs; then Work fails with one non-empty terminal ERROR and no false success. | Raw peer partial update/RPC failure plus public failed Work; shareable-with-mock. | migrate |
| ACP-013 | Given a peer requires authentication; when Work runs; then `MODEL_RESPONSE` reports auth failure with login guidance. | Raw peer auth round trip and typed public error; shareable-with-mock. | migrate |
| ACP-014 | Given a peer advertises model configuration; when Work selects a model; then the advertised session config is applied and Work completes. | Raw peer checks `session/set_config_option`; shareable-with-mock. | migrate |
| ACP-015 | Given Work contains an image resource link; when ACP prompt runs; then the SDK receives the canonical resource link and Work completes. | Raw peer checks resource-link prompt payload; shareable-with-mock. | migrate |
| ACP-016 | Given a normal ACP target runs; when public observations are captured; then exact Factory Event order/IDs, Provider Session, response terminal, and Work/session projections match. | Existing BTRC parity assertions through one controlled raw peer; shareable-with-mock. | migrate as parity witness |
| ACP-017 | Given an ACP prompt fails; when public observations are captured; then exact failure order, typed failure, response error, and failed Work/session projections match. | Existing BTRC failure parity assertions through one controlled raw peer; shareable-with-mock. | migrate as parity witness |
| ACP-018 | Given ACP add omits name; when the CLI command runs; then required-flag error appears and settings remain absent. | Root `Process.Execute`, no peer; shareable. | migrate |
| ACP-019 | Given ACP add uses a non-canonical name; when the command runs; then lowercase validation error appears and settings remain absent. | Root command validation, no peer; shareable. | migrate |
| ACP-020 | Given ACP add requests TCP; when the command runs; then stdio-only validation error appears and settings remain absent. | Root command validation, no peer; shareable. | migrate |
| ACP-021 | Given ACP add has an empty command; when the command runs; then command-required error appears and settings remain absent. | Root command validation, no peer; shareable. | migrate |
| ACP-022 | Given ACP delete omits name; when the command runs; then required-flag error appears and settings remain absent. | Root command validation, no peer; shareable. | migrate |
| ACP-023 | Given a valid integration is added, listed, then deleted; when commands use one root; then output/persistence show exactly one then zero entry without policy fields. | Repeated public commands over one root/home, no peer; shareable. | migrate |
| ACP-024 | Given init runs, a custom entry is added, and init runs again; when settings are reread; then defaults occur once and the custom entry remains. | Repeated init/settings reads over one root/home, no peer; shareable. | migrate |
| ACP-025 | Given two prompts target one held stdio connection; when the first releases; then neither completes early, both succeed serially, and one peer starts. | Real concurrent requests over one OS stdio connection; isolated-with-reason: connection serialization. | retain |
| ACP-026 | Given a peer crashes during an uncertain first prompt; when a second invocation follows; then first fails, uncertain work is not replayed, second succeeds, and starts equal two. | Real child crash and replacement; isolated-with-reason: crash and replacement. | retain |
| ACP-027 | Given a peer responds then disconnects alive; when a second invocation follows; then first stays successful, stale connection retires, second uses replacement, and starts equal two. | Real stdout disconnect/retirement and replacement; isolated-with-reason: connection retirement. | retain |
| ACP-028 | Given two sequential executions target one daemon; when both complete; then one OS process and connection serve both. | Real process/connection reuse identity; isolated-with-reason: process and connection identity. | retain |
| ACP-029 | Given a peer negotiates an incompatible version; when prompt runs; then durable execution fails at stdio. | Real initialization negotiation; isolated-with-reason: initialization negotiation. | retain |
| ACP-030 | Given a peer blocks an active prompt; when root/provider stops; then Execute joins within the bound and one peer is accounted for. | Real blocked prompt, cancellation, server stop, and process join; isolated-with-reason: shutdown join. | retain |
| ACP-031 | Given pinned SDK manifest/fixtures load; when checksums and JSON parse; then source identity, uniqueness, validity, and counts match. | Tracked assets only, no root/peer; shareable. | retain root-free |
| ACP-032 | Given lookup succeeds but OS refuses command start; when Work runs; then dependency failure and FAILED response occur without panic/hang. | Real start boundary with refused command; isolated-with-reason: OS start. | retain |
| ACP-033 | Given a peer writes a configured secret to stderr; when Work fails; then public error is redacted and omits the secret. | Real subprocess stderr path; isolated-with-reason: subprocess stderr. | retain |
| ACP-034 | Given a peer returns `StopReasonCancelled`; when Work runs; then Work fails with canceled classification and diagnostic. | Controlled peer self-cancellation, no process identity assertion; shareable-with-mock. | migrate |
| ACP-035 | Given a peer returns an incompatible version; when Work runs; then failure is `MISCONFIGURED`. | Negotiation failure at peer/process boundary; isolated-with-reason: negotiation; split from ACP-036. | retain |
| ACP-036 | Given a peer returns a generic protocol failure; when Work runs; then failure is `UNKNOWN` and terminal. | Controlled generic RPC failure and public typed outcome; shareable-with-mock. | migrate; split from ACP-035 |
| ACP-037 | Given executable locator reports missing; when Work runs; then Work fails `MISSING_EXECUTABLE` and starts zero peers. | Executable lookup and zero-start boundary; isolated-with-reason: executable lookup. | retain |
| ACP-038 | Given generated ACP catalog is loaded; when fixtures are inspected; then count is exactly 20 and profiles map to ACP v1 initialize-conformance. | Root-free catalog/fixture count; shareable asset subcases. | retain in one root-free group |
| ACP-039 | Given each of `copilot-acp`, `cursor-acp`, `droid-acp`, `fast-agent-acp`, `gemini-acp`, `grok-build-acp`, `iflow-acp`, `kilocode-acp`, `kimi-acp`, `kiro-acp`, `mux-acp`, `openclaw-acp`, `opencode-acp`, `pi-acp`, `pool-acp`, `qoder-acp`, `qwen-acp`, `reasonix-acp`, `trae-acp`, and `zeroclaw-acp` is inspected; when its initialize fixture decodes; then provider/protocol/version/fixture match. | Twenty root-free asset subtests; shareable. | retain all 20 |
| ACP-040 | Given the first packaged profile command projection runs; when Work executes; then normalized output/Provider Session match and exactly one expected command starts. | Package command allowlist and actual executable projection; isolated-with-reason: command/executable projection. | retain |
| ACP-041 | Given an unexpected packaged command reaches helper; when helper runs; then parent observes failure without ambient executable. | Dedicated child-process command guard; isolated-with-reason: helper/process command boundary. | retain |
| ACP-042 | Given spawn runs planner, two children, and merger; when workflow executes; then one persistent peer handles four agents and merged text returns. | Real persistent ACP connection count; isolated-with-reason: persistent connection count. | retain |
| ACP-043 | Given tournament runs two competitors and judge; when workflow executes; then one persistent peer handles three agents and champion/rationale returns. | Real persistent ACP connection count; isolated-with-reason: persistent connection count. | retain |
| ACP-044 | Given `session/new` fails; when golden peer runs; then Work fails with sanitized session/new diagnostic. | Pinned golden-wire subprocess; isolated-with-reason: pinned wire peer. | retain |
| ACP-045 | Given `session/set_config_option` fails; when golden peer runs; then Work fails with sanitized config diagnostic. | Pinned golden-wire subprocess; isolated-with-reason: pinned wire peer. | retain |
| ACP-046 | Given input Work has a sentinel title; when ACP prompt runs; then peer receives it as ACP text and Work completes. | Controlled peer verifies prompt text; shareable-with-mock. | migrate |
| ACP-047 | Given pinned initialize/session/prompt/permission/update fixtures drive a run; when Work executes; then completion, Provider Session, and response NDJSON order match. | Pinned golden-wire transcript; isolated-with-reason: pinned wire transcript. | retain |
| ACP-048 | Given `skipPermissions` is false; when peer requests permission; then reject selection is sent and documented Work outcome remains. | Pinned permission wire and selected option; isolated-with-reason: pinned permission wire. | retain |
| ACP-049 | Given `skipPermissions` is true; when peer requests permission; then allow selection is sent and Work completes. | Pinned permission wire and selected option; isolated-with-reason: pinned permission wire. | retain |
| ACP-050 | Given peer requests filesystem/terminal RPCs; when Work runs; then unsupported responses return and prompt completes. | Bidirectional real protocol RPC exchange; isolated-with-reason: bidirectional protocol RPC. | retain |
| ACP-051 | Given helper target runs without parent helper environment; when Go discovers it; then it exits without product assertions and parents own evidence. | Child-process helper target; isolated-with-reason: helper boundary. | retain as helper |
| ACP-052 | Given golden helper runs without parent mode; when Go discovers it; then it exits without product assertions and parents own wire evidence. | Child-process golden helper target; isolated-with-reason: helper boundary. | retain as helper |
| ACP-053 | Given ACP response and Factory Events are captured; when compared; then sequences increase, expected order remains, and terminal publication occurs once. | Cross-check attached to ACP-011/016/017; shareable-with-mock where the owning test is migratable, otherwise inherited. | migrate with owning case |
| ACP-054 | Given Worker Session source records and manifest names are inspected; when duplicate identities are encountered/checked; then duplicate source keys fail and names remain unique. | Worker Session observation plus root-free manifest check; shareable observation. | retain/migrate with owning case |
| ACP-055 | Given a controlled peer stalls; when a supported public deadline is sought; then a typed timeout and cleanup are required only if an existing public input supports it. | No baseline ACP dependency-deadline input exists. Public `wait.timeoutMillis` bounds synchronous waiting, not the provider attempt; classify the missing edge as isolated provider/process behavior. | GATE-SCOPE-001; no invented contract or test |
| ACP-056 | Given a shared or isolated scenario fails before terminal assertions; when cleanup runs; then sessions, listeners, peers, processes, routes, and paths reach terminal state. | Inherited from each owning scenario: shareable-with-mock for migratable cases and isolated-with-reason for lifecycle cases. | LIFE-004 owns direct teardown proof |

## Timeout decision

The public `FactorySessionExecutionRequest.wait.timeoutMillis` contract was
inspected in `api/components/schemas/api/FactorySessionExecutionWaitOptions.yaml`.
It is explicitly a synchronous wait budget; `cancelOnTimeout` optionally
cancels the durable Factory Session. It is not an ACP provider-attempt
deadline. The Providers `ExecuteRequest` contract contains no ACP timeout or
deadline input, and the ACP package has only internal cancellation-send bounds.
Consequently, adding an FT-TIMEOUT input or claiming provider timeout behavior
would expand the public/runtime contract beyond this story. `GATE-SCOPE-001`
remains explicit for the later lifecycle story.

## Remaining edges

- `PARITY-003`: migrate eligible cases while preserving every listed witness.
- `LIFE-004`: directly prove normal, failure, cancellation, crash, recovery,
  and early-assertion teardown on retained process/connection cases.
- `REPEAT-005` / `RACE-006`: repeat and supported race gates.
- `TOPO-007`: direct before/after topology instrumentation and material work
  reduction after migration.
- `PR-CI-008`: package timing from PR Backend Functional Coverage.
- `CLEAN-009`: read-only clean-room validation.
- `GATE-SCOPE-001`: customer-visible ACP dependency timeout, if separately
  authorized; no public contract was invented here.
