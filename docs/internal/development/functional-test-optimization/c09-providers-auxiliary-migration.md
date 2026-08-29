# C09 Auxiliary Provider Characterization Ledger

Status: stories `functional-test-optimization-c09-providers-auxiliary-migration-001`,
`-002`, and `-003` complete. The Claude golden migration and the discovery/
permission functional evidence are recorded below; integrated cleanup and PR-CI
evidence remain later story `-004`.

## Authority and scope

- Repository: `you-agent-factory`
- Branch: `functional-test-optimization-c09-providers-auxiliary-migration`
- Discovery head and `origin/main`: `254bb71db2`
- PRD stories: `functional-test-optimization-c09-providers-auxiliary-migration-001`,
  `-002`, `-003`, and `-004`
- Parent behavior: `BEH-01` — preserve auxiliary provider behavior while
  removing eligible repeated Claude construction.
- Owned surfaces inspected:
  - `tests/functional/providers/claude/**`
  - `tests/functional/providers/discovery/**`
  - `tests/functional/providers/permission/**`
  - this ledger
- Excluded surfaces were not edited: AGY, ACP, Codex, root Providers files,
  shared functional support, the C01 inventory, baselines, workflows, and the
  PR #2316 branch/history.
- Source-plan reference: `docs/temp/functional-test-optimization.md` was not
  present in this checkout. The PRD and repository standards are the authority
  for this characterization; the missing source plan is recorded rather than
  reconstructed.
- Paid/remote validation: zero calls, `$0`, no credentials or customer data.

The process counts below are source-derived application-process starts, not a
host process census. They come from the number of calls to the existing
root-built completion helper and the explicit `BuildProcess` call. Runtime
after-counts and package PR timing remain later-story evidence.

## Characterization procedure and result

The current local Go toolchain was used with the controlled command and
permission edges already present in the tests.

```text
go test ./tests/functional/providers/claude ./tests/functional/providers/discovery ./tests/functional/providers/permission -list '^Test'
```

Exit status: `0`. The command reported exactly these five top-level tests:

- `TestClaudeHaikuStreamJSONGoldens`
- `TestClaudeStreamJSONCommandThroughRootBuildProcess`
- `TestProvidersListThroughRootBuildProcess`
- `TestPackagedACPProjectionRejectsInvalidRuntimeBindings`
- `TestProviderPermissionBypassFunctionalContract`

The one pre-change package diagnostic was then run once at the declared story
scope:

```text
go test -count=1 -timeout=10m ./tests/functional/providers/claude ./tests/functional/providers/discovery ./tests/functional/providers/permission
```

Exit status: `0`; observed package output was:

```text
ok  github.com/portpowered/infinite-you/tests/functional/providers/claude     8.162s
ok  github.com/portpowered/infinite-you/tests/functional/providers/discovery  0.411s
ok  github.com/portpowered/infinite-you/tests/functional/providers/permission 3.722s
```

The command completed in 13.351 seconds on the shared Windows host. This is a
contaminated local timing observation, not a threshold or a PR performance
verdict.

## Exact top-level inventory and source-derived topology

The five rows below are the complete default-tag denominator from the test-list
command. Named subcases are included in the row that owns them.

| Package | Top-level test and source | Executable cases and public boundary | Before starts | After story 002 / disposition |
| --- | --- | --- | ---: | --- |
| `providers/claude` | `TestClaudeHaikuStreamJSONGoldens` — `haiku_golden_test.go:51` | Manifest order: `alias`, `family`, `pinned`; each case validates its embedded stream and checksum before start, prepares a copied fixture with a Factory-directory working route, opens one unique explicit Factory Session, submits Work through the session API, and observes session-scoped Work plus Factory Events. | 3 | 1 shared golden process with 3 explicit sessions; immutable routes are prepared before start. |
| `providers/claude` | `TestClaudeStreamJSONCommandThroughRootBuildProcess` — `process_harness_test.go:18` | One standalone Claude `claude-sonnet-5` command case; observes done/failed Work, one command call, command identity, and stream-json flags. | 1 | 1 retained independent command-boundary process. |
| `providers/discovery` | `TestProvidersListThroughRootBuildProcess` — `discovery_test.go:25` | One `BuildProcess`; three sequential `Process.Execute` calls for human list, JSON list, and unsupported-flag failure. Observes catalog output, diagnostics, and zero provider command calls. | 1 | 1 retained process; no per-command rebuild exists. |
| `providers/discovery` | `TestPackagedACPProjectionRejectsInvalidRuntimeBindings` — `discovery_test.go:65` | Four pure subcases: unknown profile, unsupported transport, argument drift, and canonical alias duplication. Calls catalog projection directly and creates no process/session. | 0 | Pure/no-fixture retained; a process would be synthetic overhead. |
| `providers/permission` | `TestProviderPermissionBypassFunctionalContract` — `permission_bypass_test.go:19` | `capable Codex route uses the command edge` and `registered incapable Codex route fails before the command edge`; each subtest copies a fixture and calls one completion helper. | 2 | 2 isolated processes retained because capability wiring is immutable and differs before process construction. |

### Start-count derivation

`RunFactoryToCompletionWithEdgesAndWork` and
`RunFactoryToCompletionWithEdgesAndObservations` both reach
`runFactoryToCompletionWithHome`, which creates one `ProcessAPIServer`, calls
the functional `BuildProcess` wrapper, starts one `Process.Execute`, stops the
daemon, and closes the application process. Therefore the source-derived
current total is:

| Package | Before starts | After story 002 | Calculation |
| --- | ---: | ---: | --- |
| Claude | 4 | 2 | Before: 3 golden helper calls + 1 standalone command. After: one shared golden server/process + 1 standalone command. |
| Discovery | 1 | 1 | One process serves three public invocations; the projection test is pure. |
| Permission | 2 | 2 | One process per incompatible capability configuration. |

The Claude after-count is established by the shared test structure: one
`StartFunctionalAPIServer` call builds one root process for all three golden
subtests, while the standalone test retains its one helper-owned process. The
focused run also records three command calls, one per route, and three unique
non-default explicit Factory Session IDs. Discovery and permission are not
changed by story 002.

## Public witness ledger

### Claude

Sources: `tests/functional/providers/claude/haiku_golden_test.go:51-225`,
`haiku_shared_process_test.go:1-205`,
`process_harness_test.go:18-57`, and
`testdata/haiku_stream_json/manifest.json:1-35`.

- The manifest requires schema version `1`, exactly three cases, the capture
  command flags `-p`, `--verbose`, `--output-format stream-json`, and
  `--include-partial-messages`, plus non-empty provider-version and prompt
  metadata.
- Each case requires a name, selector, reported model, expected Provider
  Session ID, stream file, and SHA-256. The embedded file must exist, pass
  `ValidateProviderSessionFixtureContent`, and match the manifest checksum.
- Native stream shape is checked line by line: every line is JSON, a line
  reports the expected model, a text delta contains `HAIKU GOLDEN COMPLETE`,
  and a terminal result has the same exact result.
- Before the process starts, each replay copies the `executor_success` Factory
  fixture, writes the provider/model worker and a workstation working-directory
  route, validates the stream, and installs one immutable directory/selector
  route. No seed file or live provider is used.
- One `StartFunctionalAPIServer` call owns the loopback server and root-built
  process. Cases run in manifest order; each opens a unique non-default
  explicit Factory Session, submits one Work through its session endpoint, and
  reads only that session's Work and Factory Events. Each public result is
  exactly one `task:done`, zero `task:failed`, and one Claude command call.
- The route rejects an unknown directory, unexpected provider command, closed
  fixture, or selector mismatch with bounded diagnostics. Duplicate directory
  and selector registrations fail before process start. Request witnesses are
  cloned and are never included in route error text, so provider input and
  environment values are not emitted by routing failures.
- The request retains the selector and the exact ordered flags
  `--verbose`, `--output-format stream-json`, and
  `--include-partial-messages`. The retained Factory Event history must contain
  a successful Model Response with the exact result and the case's expected
  Provider Session ID.
- The standalone test retains the same done/failed Work witness, one command
  call, command identity `claude`, and the exact stream-json argument sequence
  for `claude-sonnet-5`.

### Discovery

Source: `tests/functional/providers/discovery/discovery_test.go:23-126`.

- Human and JSON `you providers list` both return successfully with empty
  stderr. Human output emits every expected provider ID exactly once and
  retains the asserted provider names, readiness, model, modality, tool,
  effort, limit, and capability facts.
- JSON output decodes as the public list shape; provider count and IDs are
  exact, IDs are sorted, duplicate IDs fail, and first-party providers retain
  explicit non-null models, prerequisites, tools, and known-limits arrays.
  The existing provider-specific assertions retain aliases, capabilities,
  modalities, limits, and readiness/configuration details.
- `you providers list --unsupported` fails with the standard `unknown flag`
  diagnostic, emits no partial stdout, and leaves the counting command runner
  at zero calls.
- The pure ACP projection subcases retain their bounded diagnostics: `unknown
  runtime profile`, `unsupported transport`, `command arguments drift`, and
  `duplicates its canonical identity`. No process, session, listener, or
  external edge is created by this test.

### Permission

Source: `tests/functional/providers/permission/permission_bypass_test.go:19-103`.

- The capable route completes exactly one Work item, fails none, makes one
  Codex command call, and includes
  `--dangerously-bypass-approvals-and-sandbox` in the request.
- The incapable route uses the real published Codex route with a pre-start
  `CatalogCapabilityOverride` that contains only prompt submission. It fails
  exactly one Work item, emits one terminal dispatch response, retains the
  bounded diagnostic `provider "codex" does not support capability
  "permission_bypass"`, reports `permanent_bad_request`, includes no command
  detail, and makes zero command calls.
- The two routes intentionally use separate process construction. The
  capability override is immutable configuration supplied through
  `serviceedges.Edges`; sharing one process would require mutable capability
  state or post-start routing and would change the characterized dependency.

## Applicable behavior matrix

This is one row for every `functionalTestCaseMatrix` entry in `prd.json`.
`001` means characterization/no synthetic claim, `002` means Claude migration,
`003` means discovery/permission proof, and `004` means cleanup/integrated
validation. A row marked as having no current behavior is an explicit
inapplicable disposition, not a pass for a nonexistent case.

### Claude rows

| ID | Given | When | Then / public witness | Owner |
| --- | --- | --- | --- | --- |
| `CL-H1` | A Claude worker selects `claude-sonnet-5`. | Work runs through the command edge. | One Work is done, none fails, and one Claude request contains the existing model and streaming arguments. | 002 |
| `CL-H2` | The `claude-haiku-4-5-20251001` sanitized stream and selector are supplied. | Work runs in its explicit Factory Session. | Work succeeds and the Model Response reports `HAIKU GOLDEN COMPLETE` with its expected Provider Session ID. | 002 |
| `CL-H3` | The `claude-haiku-4-5` sanitized stream and selector are supplied. | Work runs in its explicit Factory Session. | The current native stream, Work, command, and Factory Event witnesses remain. | 002 |
| `CL-H4` | The `haiku` alias sanitized stream and selector are supplied. | Work runs in its explicit Factory Session. | The current native stream, Work, command, and Factory Event witnesses remain. | 002 |
| `CL-U1` | A golden entry is empty, malformed, unsanitized, or checksum-mismatched. | The fixture loads it. | The test fails before route execution with a case/file diagnostic and records no success. | 002 |
| `CL-D1` | A controlled Claude result lacks a required model, delta, or terminal result. | The golden is validated or replayed. | Existing shape or Factory Event assertions fail and the explicit session is reclaimed. | 002 |
| `CL-T1` | No current auxiliary Claude test defines provider timeout behavior. | The migration is reviewed. | No synthetic timeout case or timeout claim is added; provider execution failure suites retain ownership. | 001 |
| `CL-A1` | No Claude authorization behavior exists in this package. | The migration is reviewed. | No authorization claim is made. | 001 |
| `CL-C1` | A shared golden case fails or is canceled after resources open. | Cleanup runs. | All sessions, server command, process, listener, routes, and roots are reclaimed. | 004 |
| `CL-P1` | Work becomes terminal without the expected successful Model Response. | Assertions run. | The case fails and does not report successful parity. | 002 |
| `CL-N1` | Three golden routes exist in one process. | Cases execute. | They run sequentially with unique session IDs and no cross-route request contamination. | 002 |
| `CL-E1` | A required manifest field or output is empty. | Validation runs. | The fixture fails at the exact missing witness. | 002 |
| `CL-DUP1` | Two pre-start routes use the same Factory directory or selector. | Registration runs. | The duplicate is rejected, route count is unchanged, and no provider command runs. | 002 |
| `CL-O1` | Golden cases run in manifest order. | Requests and events are inspected. | Each request maps to its Factory directory and expected Provider Session regardless of prior cases. | 002 |
| `CL-CAP1` | Exactly three golden routes and sessions are configured. | Topology is inspected. | The fixture reports the bounded three-route and three-session maximum without unbounded retained state. | 002 |
| `CL-PS1` | Golden runs use `--no-record` and no persisted contract changes. | The migration is reviewed. | No persistence or restart claim is made and all transient session state is deleted. | 001 |
| `CL-I1` | Cleanup is invoked by both scenario and final fixture paths. | Finalization runs. | Session, process, and route cleanup is idempotent and reports no retained resource. | 004 |
| `CL-R1` | An adverse route or shape assertion occurs. | Finalization runs. | Cleanup is idempotent and no later case is silently claimed as passed. | 004 |

### Discovery rows

| ID | Given | When | Then / public witness | Owner |
| --- | --- | --- | --- | --- |
| `DS-H1` | The provider catalog is available. | `you providers list` runs. | Human output contains every expected provider and capability fact exactly once. | 003 |
| `DS-H2` | The same provider catalog is available. | `you providers list --json` runs. | JSON contains the exact providers, arrays, models, capabilities, aliases, limits, and readiness facts. | 003 |
| `DS-U1` | The unsupported flag is supplied. | The list command runs. | It returns `unknown flag`, emits no partial inventory, and invokes no provider command. | 003 |
| `DS-U2` | ACP runtime input has an unknown profile, unsupported transport, argument drift, or canonical alias duplication. | Projection validation runs. | Each named subcase returns its current bounded diagnostic. | 003 |
| `DS-O1` | JSON providers are projected. | IDs are read. | IDs are deterministic and sorted. | 003 |
| `DS-DUP1` | Provider output or an ACP alias duplicates an identity. | Assertions or validation run. | Duplicate provider emission or alias identity is rejected. | 003 |
| `DS-E1` | Externally supplied ACP capability collections are empty. | JSON output is decoded. | Models, tools, and limits remain explicit non-null empty arrays. | 003 |
| `DS-A1` | Discovery performs no provider execution. | Commands run. | The controlled command call count remains zero and no authorization or remote claim is made. | 003 |
| `DS-T1` | Discovery and pure projection use no remote provider dependency. | The disposition is reviewed. | No dependency-timeout claim or synthetic case is added. | 001 |
| `DS-N1` | The list test may run beside the pure projection test. | Both execute. | They share no mutable fixture state and preserve deterministic output. | 003 |
| `DS-CAP1` | The shipped provider inventory is the current bounded catalog. | Output is asserted. | Exact count and identities are preserved and no load-scale claim is added. | 003 |
| `DS-PS1` | Discovery performs no state mutation. | Commands and projection validation run. | No persisted state is created or migrated. | 003 |
| `DS-R1` | No current discovery case retries after an invalid command. | The disposition is reviewed. | No post-error recovery claim or synthetic case is added and process cleanup remains the owned recovery behavior. | 001 |
| `DS-C1` | A discovery assertion fails. | Test cleanup runs. | The single process closes and the pure projection owns no process, session, port, or root. | 004 |

### Permission rows

| ID | Given | When | Then / public witness | Owner |
| --- | --- | --- | --- | --- |
| `PM-H1` | Codex publishes permission-bypass capability. | Work runs. | One Codex request includes the bypass flag and Work succeeds. | 003 |
| `PM-U1` | The immutable Codex catalog omits permission-bypass capability. | Work runs. | One Work fails with permanent bad request, bounded capability detail, and zero command calls. | 003 |
| `PM-D1` | The incapable route blocks before the command edge. | Dispatch is observed. | One terminal response carries the current failure reason and no command detail. | 003 |
| `PM-T1` | No permission auxiliary test defines a provider timeout after authorization. | The disposition is reviewed. | No timeout claim or synthetic case is added. | 001 |
| `PM-C1` | Either permission subcase fails or is canceled. | Cleanup runs. | Its isolated process, session, server, port, and temporary root are reclaimed. | 004 |
| `PM-E1` | The incapable capability override contains only prompt submission. | Permission selection runs. | Missing permission-bypass is treated as incapable rather than implicit authorization. | 003 |
| `PM-DUP1` | No duplicate permission request contract exists in this package. | The disposition is reviewed. | No idempotency claim is added and Work duplicate behavior remains owned elsewhere. | 001 |
| `PM-O1` | Capable and incapable cases execute in declared order. | Results are inspected. | Each process uses only its immutable pre-start capability configuration. | 003 |
| `PM-P1` | The incapable route returns a terminal dispatch without executing the command. | Dispatch and Work are inspected. | Failure remains terminal and no successful Work or command call is reported. | 003 |
| `PM-N1` | Permission subcases require different immutable process wiring. | The disposition is reviewed. | They remain isolated and sequential and no shared mutable capability state is introduced. | 001 |
| `PM-CAP1` | Each permission subcase submits one Work item. | Results are inspected. | Exactly one terminal Work and dispatch outcome belongs to that isolated process. | 003 |
| `PM-PS1` | Permission tests use transient fixtures and `--no-record`. | Cleanup runs. | No persisted state or migration remains. | 004 |
| `PM-R1` | The incapable case fails as designed. | Its isolated process finalizes. | The capable configuration and later cleanup are not contaminated. | 004 |
| `PM-I1` | No duplicate Work contract is exercised here. | The disposition is reviewed. | No Work-idempotency claim is added and cleanup itself remains idempotent. | 001 |

### Cross-package cleanup row

| ID | Given | When | Then / public witness | Owner |
| --- | --- | --- | --- | --- |
| `ALL-CL1` | All applicable cases complete or fail. | Finalizer and clean-room loopback run. | Every process, session, stream, listener/port, route, and temporary root is reclaimed. | 004 |

## Cleanup obligations and security boundary

The current tests establish the ownership that later structural work must
preserve:

- `testutil.CopyFixtureDir` uses a `t.TempDir` destination, and seed/config
  writes are test-local. No fixture path is shared across Claude golden
  subtests or permission subtests.
- The root-built completion helper owns the temporary API server, application
  process, asynchronous `Process.Execute`, default Factory Session, command
  runner edge, and temporary operator home. Its cleanup sequence stops the
  daemon, closes the response capture when used, closes the application
  process, and relies on `t.Cleanup` for assertion/cancellation paths.
- The discovery test explicitly registers `CleanupProcess`; its pure test has
  no process or temporary runtime resource.
- Controlled runner instances hold only sanitized fixture output and copied
  request data. The Claude manifest states that host paths, credentials,
  thinking text/signatures, usage/cost, timestamps, durations, and random
  identifiers were removed. No validation procedure invokes a remote or paid
  provider.
- The migrated Claude fixture has no package-global mutable routing state. Its
  route map is built before `StartFunctionalAPIServer`, explicit sessions are
  checked for uniqueness and non-default identity, and route memory is released
  by the fixture cleanup after all cases close. Permission retains separate
  immutable configurations.

The focused Claude run proves normal shared-process/session/route cleanup. The
full adverse cleanup loopback, host resource counts, and clean-checkout
integration remain story 004 evidence.

## Story 001 evidence boundary

| Criterion / gate | Result in this story | Evidence and remaining edge |
| --- | --- | --- |
| `AUX-CHAR-001` | PASS | Five top-level tests from the `-list` command, source-derived subcases, 47 one-row matrix entries, exact public witnesses, cleanup obligations, and package dispositions are recorded above. No migrated parity is claimed. |
| Current topology basis | PASS | Source inspection derives Claude `4`, discovery `1`, and permission `2` application-process starts; the planned `2/1/2` target is explicitly marked unproven. |
| Dependency fidelity | PASS for characterization | Real root composition and current controlled command/permission edges were inspected and the declared package suite passed. No remote provider call was made. |
| Behavior preservation | NOT CLAIMED | Current pre-change behavior passed; parity after structural migration is story 002/003 evidence. |
| Cleanup and integrated loopback | NOT CLAIMED | Shared-process failure/cancellation and clean-room cleanup are story 004 evidence. |
| PR package timing | NOT CLAIMED | The local 13.351-second observation is contaminated and is not PR Backend Functional Coverage. |
| Exclusions and ancestry | NOT CLAIMED | Final `origin/main...HEAD` and PR #2316 checks belong to story 004. |

No topology mismatch was found between the PRD and current source, so no plan
delta is requested. The absent source-plan file remains an explicit authority
note above.

## Story 002 evidence boundary

| Criterion / gate | Result in this story | Evidence and remaining edge |
| --- | --- | --- |
| `AUX-CLAUDE-002` | PASS | The migrated selector run uses one root-built server/process for three manifest-order golden cases, three unique non-default explicit sessions, pre-start directory/selector routes, and one command call per route. Discovery/permission counts remain unchanged. |
| Golden behavior parity | PASS | Each selector retains checksum and native stream-shape validation, exact Claude streaming flags, one successful `task:done`, zero `task:failed`, and a session-scoped successful Model Response Factory Event with the expected Provider Session ID. |
| Route and session isolation | PASS | Routes reject duplicate directory/selector registration before start and unknown/closed/mismatched requests without including request payload or environment data in diagnostics; each case maps to its own Factory directory and explicit session. |
| Normal cleanup | PASS | Each explicit session is terminated and deleted after its scoped assertions; the shared server is stopped, the root process is closed by the support owner, routes are closed/released, and copied fixtures/operator home remain `t.TempDir` owned. |
| Adverse cleanup loopback | NOT CLAIMED | Full assertion-failure, cancellation, host-resource, and clean-room cleanup evidence remains story 004. The focused positive path and fail-closed construction are covered here. |
| `TestClaudeHaikuStreamJSONGoldens` repeatability | PASS | The exact touched selector was run once and with `-count=3`; local output and timing are recorded in progress, not committed as a CI artifact. |
| Dependency fidelity | PASS for story 002 | Production root composition and public Factory Session/Work/Event HTTP boundaries were used with a controlled `ProviderCommandRunner` and sanitized embedded streams; no live Claude call was made. |
| PR package timing | NOT CLAIMED | Package-level Backend Functional Coverage timing remains story 004/PR-CI evidence. |

## Story 003 evidence boundary

Story 003 retains the existing discovery and permission behavior paths. The
only source changes are comments that make the already-characterized topology
explicit: discovery uses one immutable root-built process for its three public
invocations, its ACP projection validation is pure, and the two permission
subcases remain on separate processes because capability overrides are
immutable construction-time wiring.

The declared verification was run with the local controlled edges:

```text
go test -count=1 -timeout=10m ./tests/functional/providers/discovery ./tests/functional/providers/permission
```

Both package test binaries reported success:

```text
ok  github.com/portpowered/infinite-you/tests/functional/providers/discovery  0.325s
ok  github.com/portpowered/infinite-you/tests/functional/providers/permission 3.972s
```

On this shared Windows host, the Go wrapper remained alive after emitting those
package results for more than 60 seconds; the invocation was then canceled
(shell exit status `1` from that cancellation). A process-tree check found no
remaining C09 test, server, or provider process. This is an environmental
wrapper/host observation, not a test assertion failure; the prior declared
package run in the characterization section exited `0`, and the current run
reached `ok` for both unchanged behavior packages.

### Disposition and witnesses

| Area | Result | Evidence and property proved |
| --- | --- | --- |
| Discovery human list (`DS-H1`) | PASS | `TestProvidersListThroughRootBuildProcess` executes `you providers list` through one root-built process and retains exact provider identity, readiness, model, modality, tool, effort, limit, capability, and empty-stderr assertions. |
| Discovery JSON list (`DS-H2`, `DS-O1`, `DS-DUP1`, `DS-E1`, `DS-CAP1`) | PASS | The same process executes `you providers list --json`; decoded public output retains exact count/IDs, sorted ordering, duplicate rejection, explicit non-null arrays, aliases, capabilities, models, limits, and readiness facts. |
| Discovery invalid command (`DS-U1`, `DS-A1`) | PASS | The same process executes the unsupported flag, retains the bounded `unknown flag` error and empty stdout, and the counting command runner remains at zero. |
| Pure ACP projection (`DS-U2`, `DS-T1`, `DS-PS1`, `DS-R1`) | PASS | Four direct projection subcases retain `unknown runtime profile`, `unsupported transport`, `command arguments drift`, and canonical-identity duplication diagnostics without constructing a process, session, port, or external edge. No timeout, persistence, or recovery claim is added. |
| Permission capable (`PM-H1`, `PM-E1`, `PM-O1`, `PM-CAP1`) | PASS | The capable isolated root process completes one Work, emits one Codex command request, and retains `--dangerously-bypass-approvals-and-sandbox`; capability selection is explicit rather than inferred. |
| Permission incapable (`PM-U1`, `PM-D1`, `PM-P1`) | PASS | The separate process with an immutable prompt-only Codex capability view emits one failed Work and one terminal dispatch with the bounded permanent-bad-request diagnostic, no command detail, and zero provider command calls. |
| Permission disposition (`PM-C1`, `PM-N1`, `PM-PS1`, `PM-R1`) | PASS | The two subcases remain isolated and sequential; each helper owns a copied `t.TempDir` fixture and its own root/process cleanup. No mutable capability state, shared route, persistence, timeout, duplicate-request, or idempotency claim is introduced. |

This evidence proves all story-003 discovery and permission witnesses at the
functional/pure-validation level with real root composition and controlled
command/permission edges. It does not prove remote provider behavior, Claude
adverse cleanup, host-level resource absence beyond the process-tree check, or
PR Backend Functional Coverage timing; those remain story 004 gates.

The next bounded step is story 004: integrated cleanup, exclusions, clean-room
loopback, and delivery handoff.
