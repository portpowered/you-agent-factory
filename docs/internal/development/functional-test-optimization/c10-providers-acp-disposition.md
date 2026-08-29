# C10 ACP provider disposition

## Gate and scope

- Story: `functional-test-optimization-c10-providers-acp-disposition-001`
- Behavior: BEH-01 — recovered ACP work receives an evidence-based disposition
  against the current c06 topology.
- Gate: `RECOVER-001`
- Recorded: `2026-08-28` against local `HEAD` `0c3bb857910bf4e356b01942954e7272572510f9`.
- Scope: read-only recovery and characterization. No ACP code, shared support,
  production, generated, CI, or sibling-package change is made by this story.
- Dependency fidelity: local git history, GitHub PR metadata, current
  `origin/main`, and checked-in local-real c06 evidence. No remote or paid
  provider call was made.

The PRD-referenced `progress.txt` and `docs/temp/functional-test-optimization.md`
were absent at the start of this worktree. `progress.txt` was recreated as
ignored worktree scaffolding. The c06 evidence artifact records the same missing
inputs and is used as the available current-topology authority; the absence is
not silently treated as prior-plan evidence.

## Recovery decision

Decision: **supersede #2305**.

PR #2305 is based on the pre-c06 43-top-level/root-per-test shape. Its useful
invocation-scoped fixture idea is absent from current main, but its parallel
markers and caller edits conflict with c06's shared-process/explicit-session
spine. The safe recovery is to reimplement the fixture seam on top of c06 and
re-audit parallelism cell by cell. No commit was cherry-picked or rebased.

The three options were characterized as follows:

| Option | Disposition | Evidence |
| --- | --- | --- |
| Merge or rebase #2305 | discard | The branch is dirty against current main and assumes the obsolete pre-c06 topology. |
| Discard #2305 entirely | discard | `fixture_test.go` and the argument-scoped controls remove process-global non-secret state and remain useful. |
| Supersede with a current-topology implementation | retain as lane decision | Preserves c06's 15-root spine and limits future parallelism to audited disjoint cells. |

## Exact ancestry and PR metadata

### PR #2305

Source: [PR #2305](https://github.com/portpowered/you-agent-factory/pull/2305)

| Field | Observed value |
| --- | --- |
| State | `OPEN` |
| Title | `test(acp): invocation-scoped ACP peer fixtures and selective parallelism for faster focused confidence` |
| Head | `fpkg-providers-acp-package-latency` / `ed9538fa44b2ca1eb2dd27363ce6b97f75e7ac7e` |
| Base | `main` / `22e1e096d14c97038de4092b41fba3e7348c69fb` |
| Mergeability | `CONFLICTING`; `mergeStateStatus=DIRTY` |
| Created | `2026-08-26T08:17:35Z` |
| Updated | `2026-08-26T08:28:47Z` |
| Three-dot merge base with current `origin/main` | `deb077d8dbe121e6fba5316ef00636a42238de6f` |
| Ahead/behind from that base | current main `484`, PR head `2` (`git rev-list --left-right --count`) |
| Changed paths / additions / deletions | `21` / `368` / `143` |

The branch ancestry is:

```text
584f10528e6ef3c8c4719dbc556b412584f37c7c
  -> deb077d8dbe121e6fba5316ef00636a42238de6f
  -> 8c8cdf7080a65fd93cfeeeae74bbaa28d151d255
  -> ed9538fa44b2ca1eb2dd27363ce6b97f75e7ac7e
```

The two recovered commits are:

| Commit | Parent | Subject | Diff |
| --- | --- | --- | --- |
| `8c8cdf7080a65fd93cfeeeae74bbaa28d151d255` | `deb077d8dbe121e6fba5316ef00636a42238de6f` | `test(acp): replace process-global fixture env with invocation-scoped peer configuration` | 21 files, `+345/-143` |
| `ed9538fa44b2ca1eb2dd27363ce6b97f75e7ac7e` | `8c8cdf7080a65fd93cfeeeae74bbaa28d151d255` | `test(acp): selectively parallelize isolated cells under invocation-scoped fixtures` | 12 files, `+28/-5` |

### Merged c06 authority

Source: [PR #2391](https://github.com/portpowered/you-agent-factory/pull/2391)

| Field | Observed value |
| --- | --- |
| State | `MERGED` |
| Head | `functional-test-optimization-c06-providers-acp` / `fdd4109f8533560cbcea75f241cb717139878d6e` |
| Base | `main` / `936fdae38f91799dd3b1903e9a77d3e9f3e4de39` |
| Merge commit | `0c46bf66a41a78472da16e23794cebf7531d3e27` |
| Merged | `2026-08-28T14:02:45Z` |
| Current-main relation | `0c46bf66a4` is an ancestor of current `origin/main` |

The c06 ACP ancestry inspected for this reconciliation is:

```text
936fdae38f91799dd3b1903e9a77d3e9f3e4de39  test(acp): record c06 witness baseline
699c7fda252af09c6662a71cbd62065fffdbebec  test(acp): establish shared functional process spine
645cfdd757ba424dff8cf19588b4f30779f1f1cd  test(acp): migrate eligible witnesses to shared process
72c4598516c069312c468210f31f42679366c13e  docs(acp): record eligible migration evidence
ec2c3c964a748c28db4374db64139184faf5baf9  test(acp): harden isolated lifecycle cleanup
598148d32cdc4b8584531c7383f7ca4c525cc7b6  test(acp): observe shared peer process teardown
dec6956185f1c4d3ea46085489ea03e5663116f6  test(acp): synchronize shared peer teardown
2be7c0306fc96f17f5c6852bc1ca35604a778849  test(acp): account for started helper processes
7476b54bf7183b3f62bda17e87a2bace0c92dc25  test(acp): address provider review feedback
149f18ea545276ac506d467b9b9140590142a3d8  docs(acp): record clean-room validation
fdd4109f8533560cbcea75f241cb717139878d6e  docs(acp): refresh validation head
0c46bf66a41a78472da16e23794cebf7531d3e27  merge commit for PR #2391
```

## Three-dot diff and conflict inventory

The exact procedures used were:

```text
git merge-base remotes/origin/main remotes/origin/pr-2305
git rev-list --left-right --count remotes/origin/main...remotes/origin/pr-2305
git diff --stat remotes/origin/main...remotes/origin/pr-2305
git diff --name-status remotes/origin/main...remotes/origin/pr-2305
git merge-tree $(git merge-base remotes/origin/main remotes/origin/pr-2305) remotes/origin/main remotes/origin/pr-2305
```

The three-dot diff contains only these package-local paths:

```text
M tests/functional/providers/acp/acp_error_test.go
M tests/functional/providers/acp/acp_provider_events_test.go
M tests/functional/providers/acp/basic_factory_run_test.go
M tests/functional/providers/acp/btrc_p0_characterization_test.go
M tests/functional/providers/acp/daemon_concurrency_test.go
M tests/functional/providers/acp/daemon_crash_recovery_test.go
M tests/functional/providers/acp/daemon_process_test.go
M tests/functional/providers/acp/daemon_reuse_test.go
M tests/functional/providers/acp/daemon_shutdown_test.go
A tests/functional/providers/acp/fixture_test.go
M tests/functional/providers/acp/functional_rpc_peer_test.go
M tests/functional/providers/acp/golden_rpc_peer_test.go
M tests/functional/providers/acp/javascript_factory_run_test.go
M tests/functional/providers/acp/mixed_provider_factory_test.go
M tests/functional/providers/acp/packaged_conformance_test.go
M tests/functional/providers/acp/packaged_spawn_test.go
M tests/functional/providers/acp/packaged_tournament_test.go
M tests/functional/providers/acp/run_failure_diagnostics_test.go
M tests/functional/providers/acp/run_parameters_content_test.go
M tests/functional/providers/acp/run_permissions_test.go
M tests/functional/providers/acp/run_unsupported_capabilities_test.go
```

`git merge-tree` reports 20 paths changed in both sides. It reports ten paths
with raw conflict markers:

```text
tests/functional/providers/acp/acp_error_test.go
tests/functional/providers/acp/acp_provider_events_test.go
tests/functional/providers/acp/basic_factory_run_test.go
tests/functional/providers/acp/btrc_p0_characterization_test.go
tests/functional/providers/acp/daemon_process_test.go
tests/functional/providers/acp/daemon_shutdown_test.go
tests/functional/providers/acp/functional_rpc_peer_test.go
tests/functional/providers/acp/javascript_factory_run_test.go
tests/functional/providers/acp/mixed_provider_factory_test.go
tests/functional/providers/acp/run_parameters_content_test.go
```

The source plan and PRD call this **nine content conflicts**. The nine
behavior-bearing conflict files are the same list with
`btrc_p0_characterization_test.go` removed. That file's merge marker is an
annotation/insertion-order overlap between c06's isolation comment and the old
branch's `t.Parallel` insertion; its fixture call is otherwise part of the
same recovered fixture change. Both the raw Git count (10) and the plan's nine
content-conflict count are recorded here so the extra marker is not hidden.
No merge result was used as implementation input.

## PR body, checks, reviews, and comments

The complete PR body was fetched with `gh pr view 2305 --json body` before
reuse. Its UTF-8 SHA-256 is
`0ca7098ed1d96a3409f308d63ae2a14acea11d44e641110f294a3c25e6c10315` and its
string length is 30,911 characters. The body declares project
`fpkg-providers-acp-package-latency`, package-only scope, a pre-c06 43-test
matrix, invocation-scoped fixture controls, selective `t.Parallel`, and a
three-sample timing target. It claims baseline samples
`87.4s(cold)/38.09s/38.03s` (median `38.088s`) and head samples
`25.22s/25.28s/24.89s` (median `25.220s`). Those are historical author claims
on the obsolete topology, not c10 evidence.

`gh pr checks 2305` reported these failing checks:

| Check | Result | Evidence |
| --- | --- | --- |
| Backend Test Stability | fail | [job 98109960315](https://github.com/portpowered/you-agent-factory/actions/runs/32947007900/job/98109960315) |
| UI Backend Integration | fail | [job 98109960212](https://github.com/portpowered/you-agent-factory/actions/runs/32947007900/job/98109960212) |
| Verification Policy | fail | [job 98112730766](https://github.com/portpowered/you-agent-factory/actions/runs/32947007900/job/98112730766) |

The same check query reported passing Backend Functional Coverage
([job 98109960399](https://github.com/portpowered/you-agent-factory/actions/runs/32947007900/job/98109960399)), Backend Unit Coverage, Backend Lint, Classify Verification, Workflow Lint, frontend checks, README/docs checks, and development-package build checks. Release/publication jobs were skipped by workflow conditions. The old PR had no pull-request reviews (`gh api .../pulls/2305/reviews` returned an empty list).

The issue-comment audit found four comments:

| Comment | Author | Disposition |
| --- | --- | --- |
| `5422547219` | `AndreasAbdi` | Historical timing/behavior claim on `ed9538fa4`; superseded by c06 topology and c10 evidence. |
| `5422576575` | `github-actions[bot]` | Backend lint report; inspected as old-head CI output, not reused as current evidence. |
| `5422624104` | `github-actions[bot]` | Backend unit coverage report with existing floor/hold findings; no c10 production scope. |
| `5422661209` | `github-actions[bot]` | Backend functional coverage report; old-head CI output, not reused as current package verdict. |

## Current c06 source and topology

The current source inventory was obtained with:

```text
go test -list '^Test' ./tests/functional/providers/acp
```

It exited `0`, printed exactly 25 top-level identities, and reported the
package in `0.070s`:

```text
TestACPCommandStartFailureMapsToDependencyFailure
TestACPFailureRedactsConfiguredSecretsFromStderr
TestACPProtocolFailuresMapToStableWorkerFailureClasses
TestUnavailableACPExecutableFailsBeforeStartWithMissingExecutableClass
TestFactoryRunRetriesACPProviderByResumingExactSession
TestRootConstructionDoesNotStartACPProcess
TestUnknownExecutorProviderFailsBeforeACPProcessStart
TestACPAgentHelperProcess
TestProvidersACPSerializesConcurrentPromptsOnOneStdioConnection
TestProvidersACPRestartsAfterCrashWithoutReplayingUncertainPrompt
TestProvidersACPRetiresDisconnectedConnectionBeforeReuse
TestProvidersACPRetainsOneOSProcessAndConnectionAcrossExecutions
TestProvidersACPRejectsIncompatibleProtocolVersionAtStdioBoundary
TestProvidersShutdownCancelsActivePromptAndJoinsACPProcess
TestPinnedACPSDKGoldenManifestIsCompleteAndParseable
TestACPGoldenRPCPeerProcess
TestPackagedACPProfilesUseSharedConformanceBehavior
TestPackagedACPUnexpectedCommand
TestPackagedSpawnRunsPlannerChildrenAndMergerThroughPersistentACPStdio
TestPackagedTournamentRunsCompetitorsAndJudgeThroughPersistentACPStdio
TestYouRunMapsGoldenSessionAndConfigRPCFailuresToTerminalWork
TestYouRunUsesPinnedACPWireGoldensAndProjectsTerminalOutput
TestYouRunMapsSkipPermissionsToSDKGoldenPermissionSelection
TestYouRunReturnsUnsupportedFilesystemAndTerminalRPCsAtTheACPBoundary
TestACPSharedProcess
```

The checked-in c06 evidence records the current topology as:

| Measure | Current c06 value | Meaning |
| --- | ---: | --- |
| Top-level identities | 25 | exact `go test -list` inventory above |
| Executed records | 75 | 25 parent records plus shared matrix child records |
| Root constructions | 15 | one shared root plus retained isolated root expansions |
| Shared peer starts | 17 | migrated shared public Work/session/event witnesses |
| Retained isolated peer starts | 21 | process, connection, negotiation, crash, shutdown, golden, and bidirectional witnesses |
| Accounted peer starts | 38 | `17 + 21`; not an optimization target by itself |

The c06 shared-process/explicit-session path remains canonical. It uses local
real `root.BuildProcess`/`Process.Execute`, public Factory Sessions, production
Providers composition, loopback HTTP where owned, OS subprocesses and stdio,
and controlled ACP peers only through `serviceedges.Edges`. The c06 evidence
records no change to public Work, Factory Event, response-event, Worker Session,
or Provider Session witnesses.

## Recovered edit disposition ledger

Every path in the #2305 three-dot diff is classified below. “Reimplemented”
means the behavior is useful but must be authored against current c06 code in a
later c10 story; it does not mean the old diff was copied.

| Path | Recovered edit | Disposition |
| --- | --- | --- |
| `acp_error_test.go` | Pass fixture configuration as an argument; add parallel markers to start/auth/cancellation/failure cases. | Fixture seam reimplemented in TASK-002; old parallel markers discarded pending TASK-003 audit; public failure assertions retained. |
| `acp_provider_events_test.go` | Replace helper environment with fixture values and add parallel markers to event, auth, model, and resource cells. | Fixture seam reimplemented; old parallel markers discarded pending audit; event/session assertions retained. |
| `basic_factory_run_test.go` | Remove package-global fixture constants, pass fixture to command factories/peer, and parallelize several root-built cases. | Fixture seam reimplemented; root-per-test parallelism discarded pending audit; c06 shared spine retained. |
| `btrc_p0_characterization_test.go` | Pass mode through fixture and add `t.Parallel` to BTRC cases. | Fixture seam reimplemented; old parallel marker discarded pending audit; BTRC event/order parity retained. |
| `daemon_concurrency_test.go` | Pass prompt signal/release paths through fixture and add `t.Parallel`. | Fixture seam reimplemented; connection-serialization parallel marker discarded; one-connection witness retained. |
| `daemon_crash_recovery_test.go` | Pass crash/disconnect marker paths through fixture and add `t.Parallel`. | Fixture seam reimplemented; both replacement-lifecycle parallel markers discarded after stability exposed their connection-retirement boundary; lifecycle witnesses retained. |
| `daemon_process_test.go` | Add fixture parameter to daemon server/process command factory. | Reimplemented in TASK-002; daemon construction boundary retained. |
| `daemon_reuse_test.go` | Pass persistent/version modes through fixture and add `t.Parallel`. | Fixture seam reimplemented; process/connection identity parallel marker discarded; reuse and negotiation witnesses retained. |
| `daemon_shutdown_test.go` | Pass blocking prompt signal through fixture and add `t.Parallel`. | Fixture seam reimplemented; shutdown parallel marker discarded; cancellation/join witness retained. |
| `fixture_test.go` | Add base64url JSON grammar, strict validation, malformed-child checks, and valid functional/golden checks. | Reimplemented in TASK-002 after reconciling current c06 helper entrypoints. |
| `functional_rpc_peer_test.go` | Store mode/session/marker/content controls on the peer instead of reading process-global environment. | Reimplemented in TASK-002; raw stdio JSON-RPC behavior retained. |
| `golden_rpc_peer_test.go` | Pass golden mode in fixture and validate functional/golden kind at the child boundary. | Reimplemented in TASK-002; pinned golden wire remains isolated. |
| `javascript_factory_run_test.go` | Replace helper mode environment with fixture argument. | Reimplemented in TASK-002; JavaScript public route assertions retained. |
| `mixed_provider_factory_test.go` | Replace helper mode environment and add `t.Parallel`. | Fixture seam reimplemented; parallel marker discarded pending route/path audit; mixed routing retained. |
| `packaged_conformance_test.go` | Pass package-conformance release path through fixture and add `t.Parallel`. | Fixture seam reimplemented; parallel marker discarded pending packaged cleanup audit; profile count/allowlist witness retained. |
| `packaged_spawn_test.go` | Pass spawn mode through fixture and add `t.Parallel`. | Fixture seam reimplemented; persistent-connection parallel marker discarded; packaged workflow witness retained. |
| `packaged_tournament_test.go` | Pass tournament mode through fixture and add `t.Parallel`. | Fixture seam reimplemented; persistent-connection parallel marker discarded; packaged workflow witness retained. |
| `run_failure_diagnostics_test.go` | Select golden failure mode through command-factory argument. | Reimplemented in TASK-002; sanitized golden failure assertions retained. |
| `run_parameters_content_test.go` | Pass content sentinel and golden mode through fixture; add parallel marker to content cell. | Fixture seam reimplemented; content parallel marker discarded pending audit; pinned transcript/content assertions retained. |
| `run_permissions_test.go` | Select golden permission mode through command-factory argument. | Reimplemented in TASK-002; permission-wire assertions retained. |
| `run_unsupported_capabilities_test.go` | Pass unsupported mode through fixture and add `t.Parallel`. | Fixture seam reimplemented; parallel marker discarded pending bidirectional-RPC audit; unsupported-response witness retained. |

The recovered implementation therefore contributes one retained direction
(invocation-scoped non-secret child controls), one reimplementation seam
(fixture grammar and callers), and one discarded-until-proven direction
(parallel markers). No old root count, timeout, golden, assertion, or public
contract is accepted as c10 evidence.

## ACP-001 through ACP-056 ledger

The c06 case matrix was reconciled row by row. Each row has an owning current
test or an explicit inherited/conditional disposition; none is unclassified.

| ID | Named outcome and current witness | Disposition / owner |
| --- | --- | --- |
| ACP-001 | `cursor-acp` completes Work, legacy route is unused, and one Provider Session is recorded through a controlled raw peer. | Reimplement shared fixture path in TASK-002. |
| ACP-002 | Retry fails first, loads the exact opaque session on a replacement peer, then succeeds with two starts. | Retain isolated restart/session witness. |
| ACP-003 | Legacy ACP executor spelling completes through the ACP route. | Reimplement shared fixture path. |
| ACP-004 | Configured `custom-acp` selection and Provider Session are projected. | Reimplement shared fixture path. |
| ACP-005 | Root construction starts zero ACP peers. | Retain startup-boundary witness. |
| ACP-006 | Unknown provider fails before ACP or fallback starts. | Retain pre-start witness. |
| ACP-007 | `SCRIPT_WRAP` completes with one legacy call and zero ACP starts. | Reimplement shared route witness. |
| ACP-008 | JavaScript Factory ACP execution completes with output and Provider Session evidence. | Reimplement shared fixture path. |
| ACP-009 | JavaScript MockWorkers succeeds with zero live ACP/provider calls. | Retain in owning Workers mock cell; no weakening. |
| ACP-010 | Mixed ACP and SCRIPT_WRAP routes both complete exactly once. | Reimplement shared route witness. |
| ACP-011 | Response events and Worker Session records preserve order/provenance and stay separate from replay. | Reimplement shared event witness. |
| ACP-012 | Partial output followed by RPC failure yields failed Work and one terminal error. | Reimplement shared failure witness. |
| ACP-013 | Authentication-required peer yields typed auth failure with guidance. | Reimplement shared failure witness. |
| ACP-014 | Advertised model option is applied through session configuration. | Reimplement shared configuration witness. |
| ACP-015 | Canonical image resource link reaches the ACP prompt. | Reimplement shared resource witness. |
| ACP-016 | BTRC success preserves exact Factory Event/order, Provider Session, response terminal, and Work/session projection. | Reimplement as parity witness. |
| ACP-017 | BTRC failure preserves exact failure order, typed failure, response error, and failed projection. | Reimplement as parity witness. |
| ACP-018 | Missing ACP add name is rejected and settings remain absent. | Reimplement root CLI command witness. |
| ACP-019 | Non-canonical ACP name is rejected and settings remain absent. | Reimplement root CLI command witness. |
| ACP-020 | TCP transport request is rejected because ACP is stdio-only. | Reimplement root CLI command witness. |
| ACP-021 | Empty command is rejected and settings remain absent. | Reimplement root CLI command witness. |
| ACP-022 | Delete without name is rejected and settings remain absent. | Reimplement root CLI command witness. |
| ACP-023 | Add/list/delete over one root persists one entry then zero entries without policy fields. | Reimplement shared catalog scenario. |
| ACP-024 | Init/add/init is idempotent; defaults occur once and custom entry remains. | Reimplement shared catalog scenario. |
| ACP-025 | Two prompts on one held stdio connection do not complete early, then serialize and succeed with one peer. | Retain isolated connection-serialization witness; parallelism is not admitted. |
| ACP-026 | Crash during an uncertain prompt fails first work, avoids replay, and replacement succeeds with two starts. | Retain isolated crash/replacement witness. |
| ACP-027 | Live disconnect retires stale connection and replacement succeeds with two starts. | Retain isolated connection-retirement witness. |
| ACP-028 | Two sequential executions reuse one OS process and one connection. | Retain isolated process/connection identity witness. |
| ACP-029 | Incompatible negotiated version fails at the stdio boundary. | Retain isolated negotiation witness. |
| ACP-030 | Blocking prompt cancellation joins the ACP process within the bound. | Retain isolated shutdown/join witness. |
| ACP-031 | Pinned SDK manifest and JSON fixtures have valid checksums, identity, uniqueness, and counts. | Retain root-free asset witness. |
| ACP-032 | OS command-start refusal becomes dependency failure without panic/hang. | Retain isolated OS-start witness. |
| ACP-033 | Secret written to peer stderr is absent from public diagnostics. | Retain isolated subprocess-stderr/security witness. |
| ACP-034 | Peer `StopReasonCancelled` maps to canceled Work failure and diagnostic. | Reimplement shared controlled cancellation witness. |
| ACP-035 | Incompatible version maps to `MISCONFIGURED`. | Retain isolated negotiation witness; split from ACP-036. |
| ACP-036 | Generic protocol failure maps to terminal `UNKNOWN`. | Reimplement controlled generic-failure witness. |
| ACP-037 | Missing executable maps to `MISSING_EXECUTABLE` with zero starts. | Retain isolated lookup witness. |
| ACP-038 | Generated catalog has exactly 20 ACP v1 initialize-conformance profiles. | Retain root-free catalog group. |
| ACP-039 | All 20 named profile fixtures decode to matching provider/protocol/version. | Retain all 20 root-free asset subtests. |
| ACP-040 | First packaged profile command projects correctly and starts exactly one allowlisted command. | Retain isolated command/executable projection witness. |
| ACP-041 | Unexpected packaged command fails without launching an ambient executable. | Retain isolated helper/process guard. |
| ACP-042 | Packaged spawn uses one persistent peer for four agents and returns merged text. | Retain isolated persistent-connection witness. |
| ACP-043 | Packaged tournament uses one persistent peer for three agents and returns champion/rationale. | Retain isolated persistent-connection witness. |
| ACP-044 | Golden `session/new` failure becomes sanitized terminal Work failure. | Retain isolated pinned-wire witness. |
| ACP-045 | Golden `session/set_config_option` failure becomes sanitized terminal Work failure. | Retain isolated pinned-wire witness. |
| ACP-046 | Input Work sentinel arrives as ACP text and Work completes. | Reimplement shared content witness. |
| ACP-047 | Pinned initialize/session/prompt/update fixtures preserve completion, Provider Session, and response NDJSON order. | Retain isolated pinned-wire transcript. |
| ACP-048 | Default permission policy sends reject selection over pinned wire. | Retain isolated permission-wire witness. |
| ACP-049 | `skipPermissions=true` sends allow selection and completes. | Retain isolated permission-wire witness. |
| ACP-050 | Filesystem/terminal RPC requests receive unsupported responses and prompt completes. | Retain isolated bidirectional-RPC witness. |
| ACP-051 | Functional helper without a parent fixture stays inert; parent owns protocol assertions. | Retain helper boundary; fixture validation is TASK-002. |
| ACP-052 | Golden helper without a parent fixture stays inert; parent owns wire assertions. | Retain helper boundary; fixture validation is TASK-002. |
| ACP-053 | Response and Factory Event sequences increase, preserve order, and publish one terminal. | Reimplement with ACP-011/016/017 owners. |
| ACP-054 | Worker Session source keys and manifest names remain unique. | Retain/migrate with ACP-011 and ACP-031 owners. |
| ACP-055 | No public ACP provider-attempt deadline exists; `wait.timeoutMillis` is only a synchronous wait budget. | Explicit no-new-contract disposition; later `GATE-SCOPE-001` only if separately authorized. |
| ACP-056 | Failure before terminal assertions still cleans sessions, listeners, peers, processes, routes, and paths. | Inherited from each owner; direct cleanup proof is TASK-003. |

## FIX-001 through FIX-012 recovery matrix

These rows are characterized as future c10 gates, not falsely marked as
runtime-passing in this recovery story.

| ID | Named property | Current disposition / owning gate |
| --- | --- | --- |
| FIX-001 | Valid functional fixture selects a real child and stdio JSON-RPC peer. | Reimplement #2305 fixture idea in TASK-002 / `FIXTURE-002`. |
| FIX-002 | Valid golden fixture selects the pinned golden child and stdio JSON-RPC peer. | Reimplement in TASK-002 / `FIXTURE-002`. |
| FIX-003 | Invalid base64url is rejected before JSON-RPC traffic. | Reimplement strict decoder in TASK-002 / `FIXTURE-002`. |
| FIX-004 | Invalid JSON is rejected before JSON-RPC traffic. | Reimplement strict decoder in TASK-002 / `FIXTURE-002`. |
| FIX-005 | Missing/unknown fixture kind is rejected before traffic. | Reimplement strict kind validation in TASK-002 / `FIXTURE-002`. |
| FIX-006 | Missing or invalid mode is rejected, including kind/mode mismatch. | Reimplement allowlist validation in TASK-002 / `FIXTURE-002`. |
| FIX-007 | Relative path controls are rejected before filesystem side effects. | Reimplement absolute-path validation in TASK-002 / `FIXTURE-002`. |
| FIX-008 | Production secret remains environment-only and absent from arguments/output/artifacts. | Reimplement while retaining `ACP_TEST_API_TOKEN` and golden sentinel witnesses in TASK-002 / `FIXTURE-002`. |
| FIX-009 | Different fixture modes can overlap without cross-talk. | TASK-003 / `CONTEND-003`. |
| FIX-010 | Raised package/test parallelism preserves capacity and output isolation. | TASK-003 / `CONTEND-003`. |
| FIX-011 | Cancellation/failure paths clean all owned resources deterministically. | TASK-003 / `CONTEND-003` and `LIFE-003`. |
| FIX-012 | Recovery repeat has no stale fixture, process, connection, or temporary-file state. | TASK-003 / `CONTEND-003`; full package in TASK-004. |

## Jobs=16 diagnostic evidence

The current-main jobs=16 artifact was inspected with:

```text
gh run view 33218340928 --job 99006961978 --log
```

Observed result:

- Run: [33218340928](https://github.com/portpowered/you-agent-factory/actions/runs/33218340928)
- Job: [99006961978 Backend Functional Coverage](https://github.com/portpowered/you-agent-factory/actions/runs/33218340928/job/99006961978)
- Host lane: Linux jobs=16 functional coverage run.
- `tests/functional/providers/acp`: `105.059s`, outcome `fail`.
- The report recorded six failed tests and did not evaluate package floors
  because the coverage test run failed; the failure occurred at the package
  deadline/poll boundary.

This is diagnostic evidence of the c06/current-main contention tail. It is not
converted into a local test threshold or a fabricated provider-attempt timeout.
The c06 timeout decision remains authoritative: public
`FactorySessionExecutionRequest.wait.timeoutMillis` bounds synchronous waiting,
not the ACP provider attempt, and Providers exposes no ACP deadline input.

## RECOVER-001 evidence and limits

Exact procedures and observed results:

```text
git merge-base remotes/origin/main remotes/origin/pr-2305
=> deb077d8dbe121e6fba5316ef00636a42238de6f

git rev-list --left-right --count remotes/origin/main...remotes/origin/pr-2305
=> 484  2

git diff --stat remotes/origin/main...remotes/origin/pr-2305
=> 21 files changed, 368 insertions(+), 143 deletions(-)

git merge-tree <merge-base> remotes/origin/main remotes/origin/pr-2305
=> 20 changed-in-both paths; 10 raw conflict-marker paths; 9 content-bearing
   conflicts plus one annotation/insertion-only overlap as classified above

go test -list '^Test' ./tests/functional/providers/acp
=> exit 0; 25 top-level identities; package reported ok in 0.070s
```

Property proved: the stranded branch, merged c06 ancestry, current source
inventory, current topology counts, PR metadata, historical checks/comments,
all recovered paths, and all ACP/FIX rows have an explicit disposition before
code reuse.

Not proved by this gate: fixture runtime correctness, post-change public parity,
parallel contention, repeated cleanup, final topology, package timing, clean
room behavior, terminal CI, or #2305 closure. Those remain owned by
`FIXTURE-002`, `CONTEND-003`, `PACKAGE-004`, `CLEAN-004`, PR-CI-004, and the
review-stage disposition handoff.

## Story status

`RECOVER-001` is complete for its declared read-only characterization scope.
The raw ten-marker versus nine-content-conflict discrepancy is explicitly
recorded and does not change the supersede decision or authorize reuse of a
merge result. The next story may implement the fixture seam against current
c06 source; it must preserve this ledger and re-audit every parallel marker.
