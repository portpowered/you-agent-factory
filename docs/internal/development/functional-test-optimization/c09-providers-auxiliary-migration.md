# C09 Auxiliary Provider Characterization Ledger

Status: stories `functional-test-optimization-c09-providers-auxiliary-migration-001`,
`-002`, `-003`, and `-004` complete. The Claude golden migration,
discovery/permission functional evidence, and the corrected current-head
validation loopback are recorded below. The operator has resolved the
source-plan authority and classified the earlier unrelated unit-lane sample;
terminal CI, conflict resolution, and merge remain review-owned.

## Authority and scope

- Repository: `you-agent-factory`
- Branch: `functional-test-optimization-c09-providers-auxiliary-migration`
- Review base / pre-migration artifact: `177ebdd07a176863221f11410ab84fd075f1eb80`
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
- Source-plan reference: `docs/temp/functional-test-optimization.md` is an
  operator-held, gitignored planning document and is intentionally absent from
  this checkout, `origin/main`, and local refs. The operator disposition on
  [PR #2416](https://github.com/portpowered/you-agent-factory/pull/2416#issuecomment-5461024854)
  grants this tracked ledger replacement authority under planning-standards
  §11, confirms Scope 10 at `functional-test-optimization-v2`, and requires no
  reconstruction of the source plan.
- Paid/remote validation: zero calls, `$0`, no credentials or customer data.

The process counts below are source-derived application-process starts, not a
host process census. They come from the number of calls to the existing
root-built completion helper and the explicit `BuildProcess` call. Runtime
after-counts and package PR timing are recorded in the final loopback below.

## Characterization procedure and result

The review-base artifact was used to establish the pre-migration inventory with
the controlled command and permission edges already present in the tests.

```text
go test ./tests/functional/providers/claude ./tests/functional/providers/discovery ./tests/functional/providers/permission -list '^Test'
```

Exit status: `0` at review base. The command reported exactly these five
pre-migration top-level tests:

- `TestClaudeHaikuStreamJSONGoldens`
- `TestClaudeStreamJSONCommandThroughRootBuildProcess`
- `TestProvidersListThroughRootBuildProcess`
- `TestPackagedACPProjectionRejectsInvalidRuntimeBindings`
- `TestProviderPermissionBypassFunctionalContract`

The current-head inventory was then checked independently at
`62098cfeb659c3dbd69fbf912f7422f31b179e6c` with the same command. It exited
with status `0` and reported eight top-level tests:

- `TestClaudeHaikuGoldenAdverseValidationFailsBeforeRouting`
- `TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks`
- `TestClaudeHaikuGoldenAdverseProcessPathsReclaimResources`
- `TestClaudeHaikuStreamJSONGoldens`
- `TestClaudeStreamJSONCommandThroughRootBuildProcess`
- `TestProvidersListThroughRootBuildProcess`
- `TestPackagedACPProjectionRejectsInvalidRuntimeBindings`
- `TestProviderPermissionBypassFunctionalContract`

The three additional current-head tests are the Story 002 adverse validation,
route, and process-cleanup tests. They are assigned to Story 002 below; they
were not part of the five-test pre-migration behavior denominator.

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

To attach direct behavioral evidence to this characterization story rather
than relying on the later migration runs, the same package command was then
run from the review base commit `177ebdd07a176863221f11410ab84fd075f1eb80`
in a clean detached worktree:

```text
go test -count=1 -timeout=10m ./tests/functional/providers/claude ./tests/functional/providers/discovery ./tests/functional/providers/permission
```

Exit status: `0`; the base artifact reported:

```text
ok  github.com/portpowered/infinite-you/tests/functional/providers/claude     6.630s
ok  github.com/portpowered/infinite-you/tests/functional/providers/discovery  0.350s
ok  github.com/portpowered/infinite-you/tests/functional/providers/permission 3.161s
```

This is direct functional characterization through the existing real
`root.BuildProcess`/`Process.Execute` paths and controlled command or
permission edges. The five review-base top-level tests retain the public
witnesses listed below: Claude stream/command, Work, and Factory Event
assertions; discovery catalog, ordering, invalid-flag, and zero-provider-call
assertions; and capable/incapable permission Work, dispatch, bypass-flag,
bounded-diagnostic, and command-call assertions. It proves the pre-migration
behavior baseline; it does not prove the migrated topology or parity after
structural change. The three current-head adverse tests are separately assigned
to Story 002 and are not retroactively included in this baseline.

The command completed in 13.351 seconds on the shared Windows host. This is a
contaminated local timing observation, not a threshold or a PR performance
verdict.

## Exact top-level inventory and source-derived topology

### Pre-migration inventory

The five rows below are the complete review-base default-tag denominator from
the pre-migration test-list command. Named subcases are included in the row
that owns them. This is the comparison set for the `4 -> 2` Claude process
disposition; it must not be confused with the current eight-test source
inventory above.

| Package | Top-level test and source | Executable cases and public boundary | Before starts | After story 002 / disposition |
| --- | --- | --- | ---: | --- |
| `providers/claude` | `TestClaudeHaikuStreamJSONGoldens` — `haiku_golden_test.go:51` | Manifest order: `alias`, `family`, `pinned`; each case validates its embedded stream and checksum before start, prepares a copied fixture with a Factory-directory working route, opens one unique explicit Factory Session, submits Work through the session API, and observes session-scoped Work plus Factory Events. | 3 | 1 shared golden process with 3 explicit sessions; immutable routes are prepared before start. |
| `providers/claude` | `TestClaudeStreamJSONCommandThroughRootBuildProcess` — `process_harness_test.go:18` | One standalone Claude `claude-sonnet-5` command case; observes done/failed Work, one command call, command identity, and stream-json flags. | 1 | 1 retained independent command-boundary process. |
| `providers/discovery` | `TestProvidersListThroughRootBuildProcess` — `discovery_test.go:25` | One `BuildProcess`; three sequential `Process.Execute` calls for human list, JSON list, and unsupported-flag failure. Observes catalog output, diagnostics, and zero provider command calls. | 1 | 1 retained process; no per-command rebuild exists. |
| `providers/discovery` | `TestPackagedACPProjectionRejectsInvalidRuntimeBindings` — `discovery_test.go:65` | Four pure subcases: unknown profile, unsupported transport, argument drift, and canonical alias duplication. Calls catalog projection directly and creates no process/session. | 0 | Pure/no-fixture retained; a process would be synthetic overhead. |
| `providers/permission` | `TestProviderPermissionBypassFunctionalContract` — `permission_bypass_test.go:19` | `capable Codex route uses the command edge` and `registered incapable Codex route fails before the command edge`; each subtest copies a fixture and calls one completion helper. | 2 | 2 isolated processes retained because capability wiring is immutable and differs before process construction. |

### Current-head inventory

At current source head `62098cfeb659c3dbd69fbf912f7422f31b179e6c`, the same
default-tag list reports eight top-level tests: the five pre-migration tests
above plus the three Story 002 adverse tests below. This current inventory is
the accepted source denominator for the corrected characterization ledger.

| Package | Current top-level test and source | Story ownership and executable boundary |
| --- | --- | --- |
| `providers/claude` | `TestClaudeHaikuGoldenAdverseValidationFailsBeforeRouting` — `haiku_adverse_test.go:22` | Story 002; validates empty, malformed, partial, unsanitized, and checksum-invalid fixtures before any route call. It owns a direct bounded router fixture, not an application process or Factory Session. |
| `providers/claude` | `TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks` — `haiku_adverse_test.go:72` | Story 002; rejects duplicate, unknown, mismatched, unexpected-provider, and closed routes through the controlled command edge. It owns a direct bounded router fixture and no application process. |
| `providers/claude` | `TestClaudeHaikuGoldenAdverseProcessPathsReclaimResources` — `haiku_adverse_test.go:144` | Story 002; runs partial-result/assertion-failure and cancellation cleanup probes through public Factory Session boundaries. Its two subcases create bounded test-local process probes; those probes are adverse cleanup evidence and do not change the migrated golden `4 -> 2` comparison. |
| `providers/claude` | `TestClaudeHaikuStreamJSONGoldens` — `haiku_golden_test.go:50` | Story 002; the three migrated manifest-order golden cases share one process and three explicit sessions. |
| `providers/claude` | `TestClaudeStreamJSONCommandThroughRootBuildProcess` — `process_harness_test.go:18` | Pre-migration behavior retained by Story 002; one standalone Claude command process remains independent. |
| `providers/discovery` | `TestProvidersListThroughRootBuildProcess` — `discovery_test.go:25` | Story 003; one process serves the human, JSON, and unsupported-flag invocations. |
| `providers/discovery` | `TestPackagedACPProjectionRejectsInvalidRuntimeBindings` — `discovery_test.go:65` | Story 003; four pure projection subcases create no process or session. |
| `providers/permission` | `TestProviderPermissionBypassFunctionalContract` — `permission_bypass_test.go:19` | Story 003; two separate processes retain immutable capable/incapable capability configurations. |

The current-head adverse process probes are reported separately because they
were added after the pre-migration artifact to exercise failure cleanup. The
planned topology comparison remains the customer-path migration count: Claude
`3 golden helper processes + 1 standalone = 4` before, and `1 shared golden
process + 1 standalone = 2` after.

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
adverse Claude loopback also exercises malformed/partial inputs, rejected
routes, assertion failure, cancellation, session deletion, listener shutdown,
and route release. The final loopback below adds the integrated package run,
clean tracked checkout, host process-tree check, and exclusion/ancestry
evidence.

## Story 001 evidence boundary

| Criterion / gate | Result in this story | Evidence and remaining edge |
| --- | --- | --- |
| `AUX-CHAR-001` | PASS | The review-base `-list` command records the complete five-test pre-migration denominator; the current-head `-list` command records all eight tests and assigns the three added adverse tests to Story 002. The 47 one-row matrix entries, exact public witnesses, cleanup obligations, and package dispositions are recorded above. No migrated parity is claimed by this characterization row. |
| Direct behavioral characterization | PASS | Clean review-base artifact `177ebdd07a176863221f11410ab84fd075f1eb80` passed the declared three-package command with exit status `0` (Claude `6.630s`, discovery `0.350s`, permission `3.161s`). The existing tests exercised real root/public boundaries and retained the Claude stream/command/Work/Event, discovery catalog/diagnostic/zero-call, and permission success/failure/dispatch/bypass/zero-call witnesses listed above. | This is the pre-migration baseline; migrated parity remains owned by stories 002 and 003. |
| Current topology basis | PASS | Source inspection derives Claude `4`, discovery `1`, and permission `2` application-process starts; the planned `2/1/2` target is explicitly marked unproven. |
| Dependency fidelity | PASS for characterization | Real root composition and current controlled command/permission edges were inspected and the declared package suite passed. No remote provider call was made. |
| Behavior preservation | NOT CLAIMED | Current pre-change behavior passed; parity after structural migration is story 002/003 evidence. |
| Cleanup and integrated loopback | NOT CLAIMED | Shared-process failure/cancellation and clean-room cleanup are story 004 evidence. |
| PR package timing | NOT CLAIMED | The local 13.351-second observation is contaminated and is not PR Backend Functional Coverage. |
| Exclusions and ancestry | NOT CLAIMED | Final `origin/main...HEAD` and PR #2316 checks belong to story 004. |

No topology mismatch was found between the PRD and current source. The
source-plan file is absent from the review base and all inspected refs because
it is operator-held by design; the operator disposition linked above grants
this tracked ledger replacement authority under planning-standards §11 and
confirms Scope 10 at `functional-test-optimization-v2`. This ledger does not
silently reconstruct the absent source plan.

## Story 002 evidence boundary

| Criterion / gate | Result in this story | Evidence and remaining edge |
| --- | --- | --- |
| `AUX-CLAUDE-002` | PASS | The migrated selector run uses one root-built server/process for three manifest-order golden cases, three unique non-default explicit sessions, pre-start directory/selector routes, and one command call per route. Discovery/permission counts remain unchanged. |
| Golden behavior parity | PASS | Each selector retains checksum and native stream-shape validation, exact Claude streaming flags, one successful `task:done`, zero `task:failed`, and a session-scoped successful Model Response Factory Event with the expected Provider Session ID. |
| Route and session isolation | PASS | Routes reject duplicate directory/selector registration before start and unknown/closed/mismatched requests without including request payload or environment data in diagnostics; each case maps to its own Factory directory and explicit session. |
| Normal cleanup | PASS | Each explicit session is terminated and deleted after its scoped assertions; the shared server is stopped, the root process is closed by the support owner, routes are closed/released, and copied fixtures/operator home remain `t.TempDir` owned. |
| Adverse cleanup loopback | PASS (bounded) | `TestClaudeHaikuGoldenAdverseValidationFailsBeforeRouting` covers empty, malformed, partial, unsanitized, and checksum-invalid fixtures without route calls; `TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks` covers duplicate, unknown, mismatched, unexpected-provider, and closed routes with bounded diagnostics and verifies every rejected request leaves `len(router.Requests())` unchanged; `TestClaudeHaikuGoldenAdverseProcessPathsReclaimResources` invokes the same `assertSuccessfulHaikuGoldenWork` witness sequence as an expected partial-result failure, then covers cancellation through real Factory Session HTTP boundaries. The process paths delete the session, stop and join the server/process, verify listener shutdown, and release all routes. | This story does not claim an OS-wide resource census or the cross-package clean-room loopback owned by story 004. |
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

## Story 004 validation loopback

The read-only loopback below was rerun against the corrected current source
inventory. The five-test review-base denominator remains the historical
pre-migration comparison, while the current source contains those five tests
plus the three Story 002 adverse tests. The loopback therefore evaluates all
eight current top-level tests without changing the `4 -> 2` customer-path
process comparison.

# Validation report: BEH-01 — Preserve auxiliary provider behavior while removing eligible repeated Claude construction

## Environment and artifact

- Commit/build identifier: final loopback head
  `e57ed12a698e744593ea88986dcd2b6b57b94676`; its executable and
  functional-test sources are the source-equivalent artifact at
  `62098cfeb659c3dbd69fbf912f7422f31b179e6c`, because the final handoff
  changes only this ledger.
- Environment and configuration: Windows PowerShell on the shared local host; tracked worktree clean (`git status --porcelain --untracked-files=no` empty); ignored `prd.json` and `progress.txt` remain local scaffolding.
- Customer entry point: real `root.BuildProcess`/`Process.Execute` functional paths and public Factory Session HTTP boundaries exercised by the owned tests.
- Real and substituted dependencies: real repository root/provider composition; controlled local `ProviderCommandRunner` and permission edges; checked-in sanitized, checksum-validated Claude streams; no remote provider or paid dependency.
- Cost/call budget used: zero remote calls, zero paid calls, `$0`.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Integrated owned-package behavior | PASS | The current-head inventory command `go test -count=1 -timeout=10m ./tests/functional/providers/claude ./tests/functional/providers/discovery ./tests/functional/providers/permission -list '^Test'` exited `0` and reported the five characterized tests plus the three Story 002 adverse tests: eight top-level tests total. The declared package run exited `0`: Claude `8.238s`, discovery `0.278s`, and permission `3.921s`. All current owned-package assertions therefore passed through real root composition with controlled edges. | Remote providers and the lane-wide full functional suite remain outside this local loopback. |
| Touched Claude determinism | PASS | `go test -count=3 -timeout=10m ./tests/functional/providers/claude -run '^TestClaudeHaikuStreamJSONGoldens$'` emitted package `ok` in `6.048s`; three manifest-order replays retained their stream, command, Work, Factory Event, and explicit-session assertions. | This does not prove remote Claude behavior; sanitized checked-in streams are intentionally used. |
| Cleanup and topology | PASS | `TestClaudeHaikuGoldenAdverseProcessPathsReclaimResources` exited `0` in `2.276s`. Its partial-result assertion-failure and cancellation cases observe failed/terminal Work, session deletion, listener shutdown, route count `0`, and the unchanged rejected-route request count. A before/after relevant-process snapshot reported no new Go/you/Claude/functional process after cleanup. Source-derived application starts remain Claude `4 -> 2`, discovery `1 -> 1`, permission `2 -> 2`. | This loopback does not force an OS-wide listener census; the focused test proves its owned listener boundary and terminal CI owns its own cleanup evidence. |
| Process disposition | PASS | Source-derived application starts are Claude `4 -> 2`, discovery `1 -> 1`, permission `2 -> 2`. The shared Claude fixture has one root-built process, three explicit sessions, and three pre-start immutable routes; discovery and permission retain their justified topologies. | Package PR timing is supplied by Backend Functional Coverage rather than contaminated local wall-clock observations. |
| Exclusions and ancestry | PASS | `git diff --name-only origin/main...HEAD` reports only this ledger and the five owned Claude/discovery/permission test files; `git diff --check origin/main...HEAD` is clean. PR #2316 commits `66430639c09a7b48c1451e5cd7636afbbd9e7a80` and `d7c545090d4c2da3bf72c010ee033c1425429ad2` are not ancestors of the validated head. No AGY, ACP, Codex, root Providers, shared support, inventory, baseline, or workflow file is changed. | Review may still report a merge conflict after new main commits. |
| Security/privacy and compatibility | PASS | All provider outputs are sanitized checked-in streams validated by fixture shape and SHA-256; route errors are bounded and do not include request/environment values. No API, CLI, Factory Event, persisted schema, production, generated, or configuration file changed. | Real credentials, customer data, and remote-provider behavior are intentionally untested. |
| Required unit/full gate | PASS | Final-head PR run [33244701357](https://github.com/portpowered/you-agent-factory/actions/runs/33244701357) on `e57ed12a698e744593ea88986dcd2b6b57b94676` reports the required Backend Lint, Backend Unit Coverage, Backend Unit Latency, Backend Functional Coverage, UI Backend Integration, and Verification Policy checks successful. | Review owns terminal CI for the final handoff head, conflict resolution, and merge. |
| `VAL-006` validation-loopback | PASS | This read-only report records the corrected eight-test current inventory, package results, adverse cleanup/process observation, topology disposition, exclusions, and exact current PR package timing. The operator disposition linked above resolves `PLAN-AUTH-001` and authorizes this ledger as the replacement authority. | Real or paid providers remain intentionally untested; review owns terminal CI for the final handoff head, conflict resolution, and merge. |
| `PR-CI-005` handoff | PASS | The final-head Backend Functional Coverage comment [5459739530](https://github.com/portpowered/you-agent-factory/pull/2416#issuecomment-5459739530), produced by run [33244701357](https://github.com/portpowered/you-agent-factory/actions/runs/33244701357) on `e57ed12a698e744593ea88986dcd2b6b57b94676`, reports successful owned-package results and timings: Claude `8.675s`, discovery `1.112s`, permission `3.509s`. It is the primary PR timing/full-gate evidence; local timings are recorded only as bounded directional evidence. | Review owns terminal CI on the final implementation head, conflict resolution, and merge. |

## Customer journey

1. A clean tracked checkout executes the three owned provider packages through
   real root/provider composition and controlled command/permission edges. The
   current eight-test inventory consists of the five characterized top-level
   tests plus the three Story 002 adverse tests; the corrected loopback reported
   all eight package tests and their assertions.
2. Claude Haiku alias, family, and pinned goldens execute in manifest order in
   one process through three unique explicit Factory Sessions. Each route
   retains one Claude request, successful Work, expected Model Response Factory
   Event, and deterministic session/route cleanup.
3. Discovery retains one process for human, JSON, and invalid-flag calls plus a
   pure ACP projection test. Permission retains separate capable/incapable
   processes because capability overrides are immutable construction-time
   wiring; the incapable route fails before the command edge.

## Cross-task integration and usability

- Documentation discoverability: the C09 ledger contains the full matrix,
  dispositions, cleanup ownership, and this final loopback report.
- Permission and error behavior: existing bounded capability and invalid-input
  diagnostics remain asserted; no command detail is exposed for the incapable
  route.
- Persistence/reload behavior: no persistence or restart claim is made; the
  tests use transient `--no-record` fixtures.
- Accessibility/keyboard/responsive behavior: not applicable to provider
  functional packages.
- Operational signals: command-call counts, explicit session IDs, Factory
  Events, route counts, package outcomes, and process-tree cleanup are observed.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| ENV-WIN-GO-WRAPPER | Informational | Run either declared Go command on this shared Windows host. | The wrapper exits after printing package results. | Package binaries print `ok`, but the wrapper remains alive for more than 60 seconds and is canceled with Ctrl+C; no C09 test/server/provider process remains. | Integrated and focused command output plus post-cancel process-tree queries. |
| REVIEW-UNIT-001 | Resolved by operator | Run the hosted Backend Unit Latency samples and inspect the failed package output. | Every fresh sample completes all 444 packages and the required unit/full gate remains green. | The earlier sample stopped at 320/444 in untouched `pkg/services/providers/internal/services/acp/internal/service`; the final-head run [33232246174](https://github.com/portpowered/you-agent-factory/actions/runs/33232246174) is green, and the operator classifies the earlier result as an unowned intermittent. | [Failed hosted job](https://github.com/portpowered/you-agent-factory/actions/runs/33231285001/job/99044466103), [base hosted run](https://github.com/portpowered/you-agent-factory/actions/runs/33227539317), and final-head required checks. |
| PLAN-AUTH-001 | Resolved by operator | Resolve the PRD source-plan reference `docs/temp/functional-test-optimization.md` from a clean checkout. | Scope 10 and each `sourcePlanRef` are independently comparable to the governing plan. | The file is intentionally absent because it is operator-held and gitignored; the operator grants this tracked ledger replacement authority under planning-standards §11 and confirms Scope 10 at `functional-test-optimization-v2`. | [Operator disposition on PR #2416](https://github.com/portpowered/you-agent-factory/pull/2416#issuecomment-5461024854). |

## Verdict

PASS — the current eight-test inventory and Story 002 adverse evidence are
reconciled, and the read-only loopback passes the owned-package behavior,
cleanup, topology, exclusion, and PR timing gates. The operator disposition
linked above closes `PLAN-AUTH-001`, grants the tracked ledger replacement
authority, confirms Scope 10 retention, and classifies the unrelated earlier
unit-latency sample as unowned. Review owns terminal CI, conflict resolution,
and merge.

## Delta-plan request

No delta plan requested. All implementation-owned loopback criteria pass;
review owns terminal CI, conflict resolution, and merge.
