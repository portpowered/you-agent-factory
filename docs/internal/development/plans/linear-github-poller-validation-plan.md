# Linear and GitHub Poller Validation Plan

## Status

Proposed.

## Problem statement

The Factory runtime contains a partially exercised hosted Linear poller and a
generic script-poller path, but customers and operators cannot yet prove from
the product that a configured poller can authenticate, collect the intended
external changes, admit exactly-once Work across polling and restart, and
explain its current health. GitHub has no first-class hosted poller contract.

## Customer ask

Make hosted-source polling trustworthy by:

- validating the existing Linear integration against a controlled real Linear
  workspace as well as deterministic test doubles;
- exposing poller configuration and runtime health in the dashboard;
- making configuration safely editable without exposing credentials;
- adding a first-class GitHub poller that invokes the `gh` CLI through the
  repository's injected command-runner boundary; and
- retaining repeatable test evidence for success, filtering, deduplication,
  restart, cancellation, authentication failure, rate limiting, and malformed
  provider responses.

For this plan, “GitHub poller using the `gh` CLI as a function” means an
Automations-owned hosted-source adapter that builds and invokes a controlled
`gh` command through an injected command runner. It does not mean asking users
to author shell glue or interpolating untrusted Factory fields into a shell
command. If the intended product shape is instead a reusable authored Factory
function, that should be resolved before Story 4 without changing the Linear
validation work.

## Security prerequisite

The Linear personal API key supplied with the request must be revoked and
replaced before any test execution. It was transmitted in conversation text
and must be treated as exposed. The replacement must never be committed,
included in a fixture, passed on a command line, printed in logs, captured in a
golden, or rendered in the dashboard.

Live tests will receive credentials only through the existing `auth.secretRef`
resolution path. Developer machines may resolve the reference from an
`INFINITE_YOU_SECRET_<NORMALIZED_REF>` environment variable or a mode-`0600`
runtime secret file. CI will use a protected secret and an isolated test
environment. Errors, command requests, HTTP recordings, and status projections
must prove that resolved secret values are redacted.

## Current repository findings

The repository already has more implementation than the original request
assumed:

- Factory Definitions and OpenAPI define `HOSTED_WORKER`, provider `LINEAR`,
  `auth.secretRef`, poll interval, team/state filters, Work mapping, and an
  optional assignee claim field.
- The worker editor renders and edits every current hosted Linear field, maps
  server validation errors to those fields, and persists them through the
  canonical Factory Definition mutation.
- Automations owns hosted-source polling, GraphQL request construction,
  provider-side team/state filtering, cursor pagination, issue-to-Work mapping,
  atomic checkpoint files, secret resolution, supervision, and cancellation.
- Unit tests cover filtered submissions, pagination, checkpoint recovery,
  duplicate suppression, submission failure, secret precedence, redaction,
  retry, and cancellation.
- A functional test starts a real runtime against an `httptest` Linear server
  and observes admitted Work.
- The Automations root and a typed HTTP adapter expose detached lifecycle and
  status operations internally, but those operations are not authored as
  public OpenAPI paths and are not available in the dashboard.
- Existing functional poller coverage uses an HTTP server helper,
  `UseMockWorkers`, channel timeouts, and a test-owned secret file. It is useful
  characterization evidence but does not satisfy the repository's current
  preferred functional-test construction for new coverage.
- There is no real-provider test lane, provider contract fixture harvested
  from Linear, operator-visible last-poll outcome, or GitHub hosted-provider
  implementation.
- Script pollers can already run `gh` indirectly if a user supplies a script,
  but this provides no typed GitHub configuration, normalized payload,
  provider-specific checkpoint semantics, readiness probe, or dashboard
  health.

Baseline observed while writing this plan on 2026-08-10:

```text
go test ./pkg/services/automations/... ./tests/functional/automations ./tests/functional/workstations/poller
  PASS

bunx --no-install vitest run \
  src/features/current-selection/worker-selection/components/editable/fields/worker-editable-configuration-hosted-fields.test.tsx \
  src/features/current-selection/worker-selection/lib/worker-editable-validation.test.ts \
  src/api/factory-definition/api.test.ts
  3 files, 73 tests PASS
```

## External contracts checked

- Linear documents its GraphQL endpoint, personal-key Authorization header,
  GraphQL error-array handling, provider-side filtering guidance, and preference
  for updated-first polling in its
  [GraphQL getting-started guide](https://linear.app/developers/graphql).
- Linear documents Relay-style `first`/`after` pagination, `hasNextPage`,
  `endCursor`, and `updatedAt` ordering in its
  [pagination guide](https://linear.app/developers/pagination).
- GitHub CLI documents JSON issue fields and repository/filter arguments in
  [`gh issue list`](https://cli.github.com/manual/gh_issue_list).
- GitHub CLI documents GraphQL invocation and cursor pagination requirements in
  [`gh api`](https://cli.github.com/manual/gh_api).
- GitHub documents `GH_TOKEN` as the non-interactive authentication environment
  for CLI automation in
  [Using GitHub CLI in workflows](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-github-cli).

## Intended outcome

A customer can configure a Linear or GitHub poller in a Factory Definition,
validate its non-secret settings and credentials, run or start it, see its
sanitized live health, and observe external issues or pull requests arrive as
canonical Work exactly once per external version. Restart resumes from a
durable checkpoint. Provider outages and malformed responses degrade the
individual source with actionable status without leaking credentials or
silently losing acknowledged Work.

The same provider-neutral lifecycle and observation model serves Linear and
GitHub. Provider-specific adapters retain responsibility for API/CLI invocation
and payload mapping; Automations remains the owner of polling, checkpoint,
reconciliation, and invocation-schedule policy.

## Scope

### In scope

- A repeatable, opt-in live Linear conformance lane against a dedicated test
  workspace and team.
- Corrections found by the live Linear exercise, including provider-aware retry
  and explicit bootstrap/checkpoint semantics.
- A public Automation Source API for configured-source inventory, sanitized
  configuration, readiness/probe, lifecycle control, and runtime observation.
- A dashboard Automation Sources view plus integration with the existing worker
  editor.
- A GitHub hosted-source provider implemented with `gh` through an injected
  command runner.
- GitHub issues and pull requests in one configured repository for v1, with
  explicit resource-type, state, label, and Work mapping filters.
- Deterministic unit, integration, contract, functional, UI, failure, restart,
  and live-provider evidence.
- Customer documentation and a non-secret example Factory for each provider.

### Out of scope

- Writing back to Linear or GitHub from admitted Work.
- General bidirectional synchronization or conflict resolution.
- Webhook ingestion; polling is the behavior under validation. Linear's own
  guidance prefers webhooks for broad production update flows, so a future
  webhook lane may replace frequent polling without changing Work mapping.
- Arbitrary GitHub search syntax or arbitrary `gh` flags in Factory
  Definitions. V1 uses typed fields to avoid command injection and unstable
  output contracts.
- Polling multiple GitHub repositories from one source. Customers configure
  one source per repository so identity, status, and checkpoints stay clear.
- Persisting credential values in Factory Definitions or returning them from
  any API.
- Making live third-party tests part of the ordinary offline PR gate.

## Canonical state and ownership

| Concern | Canonical owner | Projection or effect boundary |
| --- | --- | --- |
| Authored provider, filters, mapping, interval, bootstrap policy, and secret reference | Factory Definitions | OpenAPI Factory document and existing worker editor |
| Source lifecycle, poll policy, observation, reconciliation, and checkpoint decision | `pkg/services/automations` | Public Automations root |
| Linear GraphQL request/response translation | Automations hosted-sources Linear adapter | Injected HTTP doer |
| GitHub query and output translation | Automations hosted-sources GitHub adapter | Dedicated `HostedGitHubCommandRunner` edge invoking `gh` directly |
| Canonical Work admission and duplicate/upsert behavior | Work service | Automations uses the public Work submission contract |
| Runtime source health | Automations source observation state | HTTP/CLI projections and dashboard query cache |
| Dashboard edit state | Feature-owned worker draft operations | A draft only; the saved Factory Definition remains canonical |
| Dashboard source list/status | Automations API response | A read projection; it does not mutate Factory Definition state |
| Checkpoint persistence | Automations-owned checkpoint contract | Atomic filesystem implementation supplied through `edges.Edges` |
| Secrets | Operator environment or runtime secret file | Resolver returns a value only to the provider effect call |

The dashboard must not merge runtime status into the saved Factory document.
Editing uses the existing Factory Definition save operation. Probe/start/stop
use Automations operations. Query invalidation joins those views without
creating a second UI-owned source of truth.

## Proposed public behavior

### Source identity and configuration

Every poller source has a stable identity derived from the Factory Session,
poller workstation, and worker. Mutable display names must not be the only
durable identity when public IDs are available.

The Linear configuration keeps the current fields and adds an explicit
bootstrap policy:

- `LATEST_ONLY`: establish a checkpoint without importing pre-existing issues;
- `BACKFILL`: admit matching existing issues up to a required bounded limit.

There must be no implicit unbounded first-run import. Existing definitions that
omit the field retain a documented compatibility default chosen and locked by
contract tests before implementation proceeds.

The GitHub configuration should contain:

- repository identity in canonical `owner/name` form;
- resource types: `ISSUE`, `PULL_REQUEST`, or both;
- optional states and labels expressed as typed arrays;
- poll interval and explicit bootstrap policy/backfill limit;
- deterministic Work type/state mapping; and
- `auth.secretRef`, resolved to `GH_TOKEN` only for the child command.

The adapter invokes `gh api graphql` or an equivalently stable `gh` JSON
surface with an explicit repository and selected fields. `gh api graphql`
supports cursor pagination when the query accepts an end cursor and requests
page information. The implementation must parse raw JSON itself and must not
depend on locale-sensitive terminal text, user aliases, the current Git remote,
or ambient interactive authentication.

### Source observation

Each source status response should include only safe facts:

- identity, provider, configured desired state, and observed lifecycle state;
- current opaque checkpoint/version;
- last attempt, last success, and next scheduled attempt timestamps;
- consecutive failure count and retry-after timestamp;
- last result counts: fetched, matched, submitted, duplicate, and rejected;
- typed last error category and sanitized actionable message; and
- readiness prerequisites such as missing `gh`, unresolved secret, invalid
  repository, or inaccessible Linear team.

It must never include a resolved credential, Authorization header, child
process environment, complete command line containing secrets, or raw provider
body that might contain private data.

### Probe and run-once

Add an explicit provider probe that validates dependency availability,
credential resolution, remote access, configured team/repository and filters,
and response decoding without admitting Work or advancing a checkpoint.

Add a run-once operation for deterministic operator verification. It executes
one normal poll cycle, uses the same checkpoint/admission path as supervision,
returns sanitized counts, and cannot overlap another cycle for the same source.
This avoids treating “wait for the background loop” as the only test method.

Probe and run-once are external side effects and must be labeled as such in the
API and dashboard. Saving a Factory Definition remains inert.

## Work stories

### Story 1: Lock provider-neutral poll semantics

As a customer, I can predict what the first poll, repeated poll, concurrent
poll, and restarted poll will admit.

Acceptance criteria:

- Bootstrap behavior is explicit and bounded; compatibility behavior for
  existing Linear definitions is documented and tested.
- A source version is identified by provider item ID plus update version/time,
  with a deterministic tie-breaker for equal timestamps.
- A checkpoint advances only after all matched Work in the cycle receives a
  terminal accepted or duplicate admission outcome.
- Rejected or unavailable admission does not skip the provider version.
- Repeating a cycle or restarting from the committed checkpoint does not admit
  another Work version.
- Two run-once/background attempts for one source cannot race checkpoint
  advancement or duplicate admission.
- Cancellation stops network/process work and all supervised goroutines join.

Evidence:

- Table-driven pure ordering/checkpoint tests including equal timestamps,
  update-to-an-existing-ID, empty pages, multi-page checkpoint search, and
  backfill bounds.
- Integration tests with an injected clock, checkpoint store, and Work
  submitter; no sleeps or timeout-padded polling loops.
- Race/stress coverage for run-once versus supervision and checkpoint commit.

### Story 2: Prove and harden the Linear adapter

As a customer, I can trust that a configured Linear source uses the documented
API correctly and degrades safely when Linear is unavailable.

Acceptance criteria:

- Requests use `https://api.linear.app/graphql`, the correct personal-key
  Authorization form, Relay cursor pagination, `updatedAt` ordering, and
  provider-side team/state filtering.
- A GraphQL `errors` array fails the cycle even when HTTP status is 200 or
  partial data is present.
- 401/403, 429/rate-limited, 5xx, timeout, malformed JSON, missing/null required
  fields, and pagination anomalies map to typed safe outcomes.
- Rate limiting honors provider delay metadata when available; other retry uses
  bounded exponential backoff with jitter and a production-safe minimum. The
  current 25-250 ms restart loop is not retained for live provider failures.
- The poller emits deterministic Work ID, trace/version identity, payload, tags,
  mapping, and optional assignee claim data.
- Logs include source identity, latency/outcome, counts, and retry decision but
  not API keys or private issue bodies.

Evidence:

- A stateful fake Linear GraphQL server that verifies the exact request and
  exercises every page and failure class.
- Sanitized contract fixtures derived from real Linear responses, with all
  tenant/user/content values replaced.
- Existing Linear unit and supervisor tests retained or rewritten around the
  provider-neutral semantics instead of topology assertions.

### Story 3: Exercise Linear live without making CI flaky

As a maintainer, I can run one command against the dedicated Linear test
workspace and receive a self-cleaning conformance report.

Acceptance criteria:

- The test refuses to run unless an explicit live-test flag, test workspace
  identity, and secret reference are supplied.
- The harness discovers and prints only non-secret team/state identifiers,
  creates a uniquely marked test issue, and records cleanup ownership.
- A matching issue is admitted with the expected Work identity, state, payload,
  and tags.
- An unchanged second poll produces no new Work; an issue update produces
  exactly one new external version; restart from the checkpoint produces no
  duplicate.
- A non-matching team/state issue is not admitted.
- Cancellation shuts the source down cleanly and a subsequent start resumes.
- Cleanup closes/archives or deletes all items created by the run even after a
  failed assertion. Cleanup failure is reported with the marker needed for
  manual repair.
- Test output and retained artifacts are scanned for the resolved secret.

Evidence:

- An opt-in live test target such as `make test-live-linear` outside ordinary
  PR CI.
- A scheduled/manual protected CI job using the dedicated test workspace,
  concurrency group, least-privilege replacement credential, and retained
  sanitized report.
- A short checked-in runbook with setup, expected output, cleanup, and token
  rotation steps.

### Story 4: Add a GitHub `gh` hosted-source adapter

As a customer, I can poll one GitHub repository for changed issues and pull
requests without authoring a custom script.

Acceptance criteria:

- `GITHUB` is a supported hosted-worker provider with a provider-specific,
  generated Factory Definition/OpenAPI contract.
- Configuration supports repository, resource types, typed filters, interval,
  bootstrap/backfill policy, and Work mapping.
- The adapter calls `gh` through a directly injected, dedicated
  `HostedGitHubCommandRunner` edge,
  passes the token only in `GH_TOKEN`, selects explicit JSON fields, and never
  invokes a shell.
- Missing executable, unsupported `gh` version, unauthenticated/unauthorized
  access, missing repository, rate limit, timeout/cancellation, non-zero exit,
  stderr content, malformed JSON, and pagination failure are typed and safely
  observable.
- Issues and pull requests have distinct stable Work identities and normalized
  payloads; updates generate one new trace/version and unchanged items do not.
- The adapter does not rely on current directory Git remotes, user aliases,
  terminal locale, or existing `gh auth login` state.
- GitHub Enterprise hostname support is either included explicitly and tested
  or rejected with a clear validation message in v1.

Evidence:

- Command-runner fixture tests assert exact executable, argument array,
  environment, work directory, cancellation, stdout parsing, and stderr
  redaction.
- Golden normalized issue/PR payloads from sanitized `gh` JSON fixtures.
- Multi-page, equal-timestamp, issue-versus-PR, filter, checkpoint, restart, and
  concurrent-cycle tests shared with Linear where semantics are identical.
- One functional `root.BuildProcess` path with the dedicated command-runner effect
  replaced only through `edges.Edges`.

### Story 5: Expose source inventory, health, probe, and lifecycle publicly

As an operator, I can inspect and control configured sources without reading
logs or checkpoint files.

Acceptance criteria:

- Authored OpenAPI paths and schemas expose list/get status, probe, run-once,
  start, stop, and wait operations through the Automations service root.
- Inventory is derived from the selected/current Factory Definition while live
  observation remains Automations-owned session state.
- Responses use stable provider-neutral values and typed failures; no HTTP
  handler reaches into private hosted-source implementations.
- CLI commands expose equivalent customer behavior and deterministic JSON.
- Start/stop/run-once operations are idempotent or return an explicit conflict
  for an unsafe overlap.
- Generated Go and TypeScript artifacts, HTTP handlers/mappings, CLI adapters,
  and contract examples remain aligned.

Evidence:

- Service operation tests, domain-to-HTTP/CLI mapping tests, OpenAPI contract
  tests, error/status mapping tests, and CLI/API parity fixtures.
- Functional customer flows built with `root.BuildProcess` and
  `Process.Execute`, preferring CLI over HTTP except for API-owned contract
  cells.
- Safe structured-operation logging tests for every mutating or live-effect
  operation.

### Story 6: Make pollers operable in the dashboard

As an operator, I can configure a source, validate it, and understand whether
it is working from the dashboard.

Acceptance criteria:

- An Automation Sources view lists configured sources with provider,
  workstation/worker, desired and observed state, last success, next attempt,
  counts, and an actionable sanitized failure summary.
- Selecting a source links to the existing worker editor; the current Linear
  fields are retained rather than reimplemented.
- The editor adds GitHub fields using generated types and shared form
  primitives, with typed selectors for resource types and filters.
- Secret input edits only the reference. The UI never fetches, reveals, or
  tests the credential value directly.
- Probe, run-once, start, and stop actions use Automations mutations with clear
  pending, success, failure, stale-session, and conflict states.
- Status refreshes from the API/query cache and does not become component-local
  canonical state.
- Empty, loading, disconnected, stopped, degraded, and running states are
  accessible, keyboard operable, localized, and usable at narrow viewports.

Evidence:

- Pure draft and projection tests; hook/mutation and query-invalidation tests;
  focused component tests for forms, statuses, and actions; and one browser
  interaction from edit/save through probe/run-once result.
- Accessibility checks for labels, descriptions, live status announcements,
  focus restoration, and disabled/busy controls.
- Manual browser inspection at desktop and narrow viewport sizes against the
  deterministic fake-provider backend.

### Story 7: Publish examples, operations guidance, and ongoing conformance

As a customer and maintainer, I can set up and troubleshoot either provider
without guessing about secrets, filters, first-run behavior, or test scope.

Acceptance criteria:

- Public reference docs explain poller workstation/worker relationships,
  secret references, bootstrap/backfill, mappings, checkpoints, lifecycle,
  probe/run-once, and safe troubleshooting.
- Examples contain placeholder secret references only and include Linear plus
  GitHub issue/PR configurations.
- Maintainer docs distinguish offline deterministic gates from opt-in live
  conformance and document test-account ownership and cleanup.
- A compatibility note explains the first-run behavior of existing Linear
  definitions.
- Live Linear and GitHub jobs have an explicit owner, rotation schedule,
  concurrency control, failure notification, and quarantine policy; a broken
  external service does not silently disable the lane.

Evidence:

- `make docs-reference-smoke`, example validation, and generated example/schema
  checks.
- A recorded successful run of each live-provider lane before release.

## Test matrix

| Behavior | Pure/unit | Provider integration | `root.BuildProcess` functional | UI/browser | Live provider |
| --- | --- | --- | --- | --- | --- |
| Config validation and generated contract | Yes | No | CLI validate/save | Form validation/save | Probe config |
| First-run latest-only/backfill | Yes | Stateful fake | Run-once | Result summary | Yes |
| Provider filtering | Yes | Exact query/command | Admitted Work | Config/status | Yes |
| Pagination and equal-time ordering | Yes | Multi-page fake | Representative case | No | Bounded sample |
| No-change duplicate suppression | Yes | Two cycles | Restart case | Counts | Yes |
| Existing item updated | Yes | Versioned fixture | Work upsert/lineage | Counts/detail link | Yes |
| Checkpoint commit after admission | Yes | Submit failure | Restart recovery | Failure status | Yes |
| Authentication/authorization failure | Error mapping | 401/403 or command exit | Safe CLI error | Safe action error | Dedicated negative probe |
| Rate limit/backoff | Policy | 429/provider fixture | Deterministic clock | Retry status | Observe only; do not induce load |
| Malformed/partial response | Decoder | Fixture | Safe failure | Degraded status | No |
| Cancellation and shutdown | Supervisor | Blocking effect | Process lifecycle | Stop action | Yes |
| Secret non-disclosure | Redactor | Headers/env/log capture | Output scan | DOM/network fixture scan | Artifact scan |
| Missing `gh`/bad version | GitHub adapter | Command edge | CLI readiness | Prerequisite status | Environment probe |

## Live Linear protocol

1. Revoke the credential shared with the request and provision a replacement
   for the dedicated `youagentfactory` test workspace.
2. Create a dedicated test team/project and two workflow states: one matching
   and one excluded. Do not use customer or personal production issues.
3. Configure the Factory with a unique source ID, a conservative interval, and
   `LATEST_ONLY`; run probe and record the discovered non-secret IDs.
4. Create a uniquely marked issue after the checkpoint is established.
5. Run one cycle and assert the complete admitted Work contract and source
   status counts.
6. Run again unchanged and assert zero new admission.
7. Update title/body/state/assignee in controlled subcases and assert exactly
   one new external version plus correct claim behavior.
8. Restart the Factory Session from the durable checkpoint and assert no
   duplicate.
9. Create an excluded issue and prove provider/local filtering does not admit
   it.
10. Exercise a deliberately invalid secret reference through probe, verify the
    typed/redacted error, then restore the valid reference.
11. Stop the source and verify no new poll; start it and verify resume.
12. Clean up every marked issue and scan logs/reports for the resolved key.

Live testing must not intentionally trigger Linear rate limits. Rate-limit and
outage behavior belongs to deterministic fakes; the live lane only verifies
normal provider compatibility and a bounded authentication failure.

## Live GitHub protocol

1. Use a dedicated private repository and a fine-grained token limited to
   metadata and read/write test issues/pull requests as required by the harness.
2. Resolve the token from `auth.secretRef` into `GH_TOKEN`; verify probe reports
   the expected `gh` version and repository access without exposing auth state.
3. Establish a `LATEST_ONLY` checkpoint, create a uniquely marked issue and a
   small test pull request, and run one cycle.
4. Assert distinct normalized Work identities, mappings, payloads, tags, and
   source status counts.
5. Repeat unchanged, update each resource once, and restart from checkpoint to
   prove no-change suppression and update versioning.
6. Verify label/state/resource-type filters with excluded fixtures.
7. Run negative probes for an inaccessible repository and invalid token in an
   isolated invocation.
8. Close the pull request and issues, remove its test branch, and scan all
   retained artifacts for the token.

## API and generated-artifact changes

Expected authored changes:

- Extend `HostedWorkerProvider` with `GITHUB`.
- Add provider-specific GitHub config, resource type, mapping, filter, and
  bootstrap-policy component fragments.
- Add provider-neutral Automation Source identity/config summary,
  observation/result, probe, run-once, and lifecycle request/response schemas.
- Add Automations paths and operation IDs to `api/openapi-main.yaml`.
- Extend the authored Worker schema with `github` configuration.

Expected generated and handwritten consumers:

- `api/openapi.yaml`;
- generated Go HTTP server/client contracts;
- `ui/src/api/generated/openapi.ts` and publishable UI client artifacts;
- Automations HTTP mappings/handlers and CLI manifest/handlers;
- Factory Definition decoders, validators, clone/compile/persistence paths;
- existing worker-editor draft operations and the new Automation Sources
  projection/hooks/components.

Use `make generate-api` for direct contract consumers and
`make interfaces-all` when publishable package schemas or clients change.
Generated files must not be hand-edited.

## Verification gates

During implementation, run the narrowest package and feature tests for each
story. Before merge, the expected shared gates are:

```text
go test ./pkg/services/automations/...
go test ./pkg/services/factory_definitions/...
go test ./pkg/transports/cli/...
go test ./pkg/transports/http/...
go test ./tests/functional/automations/...
go test ./tests/functional/workstations/poller/...

make generate-api
make interfaces-all
make api-smoke
make docs-reference-smoke
make ui-test
make ui-lint
make verify-fast
make verify-pr
make build-all

make test-live-linear   # opt-in/protected environment only
make test-live-github   # opt-in/protected environment only
```

New functional tests must construct with `root.BuildProcess` and execute with
`Process.Execute` by default, prefer CLI for ordinary customer flows, and
replace HTTP, command, filesystem, clock, and submission effects only through
the exact `edges.Edges` ports. They must not add `MockWorkers` outside its
owning test cells or use sleeps/timeouts where deterministic observation and
injected clocks can prove the behavior.

## Delivery sequencing

1. Story 1 fixes semantics before either provider accumulates more behavior.
2. Stories 2 and 3 prove Linear offline and live; correctness findings feed
   back into the shared semantics.
3. Story 4 adds GitHub against the now-proven provider-neutral contract.
4. Story 5 publishes lifecycle/health operations once both adapters can project
   the same observation model.
5. Story 6 builds the dashboard over the public API and extends the existing
   editor for GitHub.
6. Story 7 finalizes customer examples, operator runbooks, and recurring live
   conformance.

Stories 1-3 are the minimum Linear confidence milestone. Story 4 can be a
separate PR series. Stories 5-6 should be vertically sliced by observable
operation where practical rather than delivered as one backend PR followed by
one UI PR.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Exposed credential enters history or logs | Revoke before use; secret references only; automated output/artifact scans; redaction tests |
| First poll imports an unexpected backlog | Explicit latest-only/backfill policy and mandatory bound |
| Checkpoint advances before Work is durable | Commit only after terminal accepted/duplicate admissions; failure/restart tests |
| Same timestamp or provider reorder loses work | Stable ID tie-breaker and multi-page/equal-time fixtures |
| Fast restart loop hammers Linear/GitHub | Provider-aware delay, bounded exponential backoff, jitter, and visible retry time |
| Live tests become flaky or destructive | Dedicated accounts, unique markers, serialized jobs, conservative intervals, cleanup ledger, offline PR gate |
| `gh` behavior varies by locale/config/version | Explicit JSON fields, minimum supported version, controlled args/env, no shell/current-remote dependence |
| Command injection through repository/filter fields | Typed validation and argument-array execution; no arbitrary query/flags or shell interpolation |
| UI becomes a second source of truth | Factory Definition owns config; Automations owns observation; UI keeps only drafts/query projections |
| Provider-private data leaks through status | Typed summaries/counts; sanitized errors; never return raw bodies/stdout/stderr |
| OpenAPI, Go, and TypeScript drift | Authored components plus generation and parity gates |

## Project-level acceptance criteria

- The replacement Linear key is used only through a secret reference and the
  originally shared key has been revoked.
- Linear deterministic and live lanes prove matching admission, no-change
  suppression, update versioning, filtering, checkpoint restart, cancellation,
  and secret redaction.
- GitHub issues and pull requests produce the same provider-neutral lifecycle
  and checkpoint guarantees through an injected, shell-free `gh` invocation.
- Customers can edit provider configuration, probe it, run one cycle, control
  lifecycle, and inspect safe health from supported CLI/API/dashboard surfaces.
- Factory Definitions remain canonical for authored configuration;
  Automations remains canonical for runtime polling state; Work remains
  canonical for admission.
- The ordinary PR gate is deterministic and network-independent; protected
  live jobs provide real compatibility evidence without becoming an excuse to
  omit provider fakes.
- All required focused tests, generated-artifact checks, documentation checks,
  `verify-pr`, and relevant build gates are terminal and passing.
- Delivery continues through blocking review feedback, conflict resolution,
  terminal green CI, and actual PR merge. A pushed branch, open PR, approval,
  or green CI without merge is not completion.

## Work-story task packets

Every implementation task submitted to You Agent Factory workers must retain
the headings below and point back to this plan.

### Task packet: provider-neutral poll semantics

# problem statement

Current hosted polling does not publish an explicit bounded first-run policy or
a provider-neutral version/checkpoint contract.

## customer ask

Make first poll, repeated poll, concurrent poll, and restart behavior
predictable before adding another provider.

## solution

Define shared bootstrap, external-version ordering, checkpoint commit, cycle
exclusion, retry observation, and cancellation behavior in Automations.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\linear-github-poller-validation-plan.md`

# changes

## package changes

- Extend Automations contracts and hosted-source internals without moving Work
  admission or Factory Definition ownership.

## contracts

- Add bootstrap/backfill and safe cycle-observation values.

## services

- Implement provider-neutral checkpoint/version and cycle-exclusion policy.

## API changes

- Extend authored Factory worker config for bootstrap behavior and regenerate
  consumers.

## tests

- Add pure ordering, checkpoint, admission failure, concurrency, restart, and
  cancellation coverage.

### Task packet: Linear conformance and hardening

# problem statement

The Linear adapter passes mocked tests but has no live conformance evidence and
uses an unsafe production retry floor.

## customer ask

Prove Linear compatibility and safe failure behavior against documented and
real responses.

## solution

Build a stateful GraphQL contract server, harden decoding/retry/observability,
and add a protected self-cleaning live test lane.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\linear-github-poller-validation-plan.md`

# changes

## package changes

- Harden the Automations-owned Linear adapter and live-test support.

## contracts

- Preserve existing Linear public fields while locking bootstrap and typed
  failure behavior.

## services

- Use provider-aware backoff and safe operation observations.

## API changes

- No provider-specific lifecycle API; Linear projects the shared observation.

## tests

- Add exact GraphQL, response fixture, retry, log-redaction, root-composition,
  and opt-in live Linear tests.

### Task packet: GitHub `gh` hosted source

# problem statement

GitHub polling requires user-authored scripts and lacks typed config,
normalization, checkpoint behavior, and readiness diagnostics.

## customer ask

Poll GitHub issues and pull requests as a first-class hosted source using the
`gh` CLI.

## solution

Add a GITHUB hosted provider and Automations adapter that invokes controlled
JSON `gh` commands through the injected command runner.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\linear-github-poller-validation-plan.md`

# changes

## package changes

- Add GitHub config/validation/clone/compile paths and an Automations-owned
  GitHub adapter wired through a dedicated `HostedGitHubCommandRunner` process
  edge.

## contracts

- Add repository, resource type, filters, mapping, interval, bootstrap, and
  provider enum values.

## services

- Add `gh` probe, collection, normalization, and supervision behind the shared
  hosted-source service.

## API changes

- Extend Worker OpenAPI and regenerate Go/TypeScript/package consumers.

## tests

- Add exact command request, JSON fixtures, errors, pagination, filters,
  checkpoint/restart, functional, and opt-in live GitHub coverage.

### Task packet: public source operations

# problem statement

Operators cannot inspect or control configured pollers through public product
surfaces.

## customer ask

List, probe, run once, start, stop, wait for, and inspect Automation Sources.

## solution

Publish provider-neutral Automations operations through authored OpenAPI and
equivalent CLI commands.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\linear-github-poller-validation-plan.md`

# changes

## package changes

- Extend the Automations root, HTTP adapter, CLI adapter, and safe operation
  logging.

## contracts

- Add inventory, observation, probe, run-once, and lifecycle values.

## services

- Derive inventory from current Factory config and observation from
  Automations runtime state.

## API changes

- Author Automations paths/components and regenerate all consumers.

## tests

- Add service, mapping, error, CLI/API parity, OpenAPI, and standards-compliant
  functional coverage.

### Task packet: dashboard poller operations

# problem statement

The dashboard can edit current Linear fields but cannot expose source inventory
or live operational health and has no GitHub form.

## customer ask

Configure, validate, run, control, and troubleshoot pollers from the website.

## solution

Add an Automation Sources view over the public API, retain the existing Linear
editor, and add GitHub fields and source actions using shared UI primitives.

# original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\linear-github-poller-validation-plan.md`

# changes

## package changes

- Add feature-owned API adapters, projections, hooks, operations, localized
  messages, and components.

## contracts

- Consume generated Factory Definition and Automations schemas without UI-only
  domain types.

## services

- Keep config mutation in the Factory Definition save operation and live
  actions in Automations mutations.

## API changes

- No dashboard-only endpoints; use the generated public API.

## tests

- Add draft/projection, hook/mutation, component, accessibility, responsive,
  and browser-flow evidence.
