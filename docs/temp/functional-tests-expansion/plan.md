# PRD: Functional Test Coverage Expansion

## Overview

The repository has hundreds of functional test functions, but they are
distributed across historical catch-alls and often lack customer-readable
descriptions. This program reorganizes them into domain-owned black-box tests
that mirror how the product and code are shaped, adds missing high-value
system coverage, introduces provider execution goldens, and generates a single
readable inventory and package-coverage report.

The plan is designed for high parallelism. Shared harness and artifact cells
land first. Afterwards, each named test file in
[`test-file-checklist.md`](test-file-checklist.md) is independently assignable.

## Current baseline

Snapshot on 2026-07-23:

| Measure | Observation |
| --- | ---: |
| Functional `_test.go` files | 222 |
| Files with top-level `Test*` functions | 188 |
| Top-level `Test*` functions | 515 |
| Conventional `// Test...` descriptions | 39 |
| Functional Go packages | 36 |
| Required overall functional statement-coverage floor | 33.1% |

The deprecated `tests/functional/runtime_api` package and broad packages such
as `smoke`, `workflow`, and `guards_batch` make ownership unclear. Statement
coverage exists, but there is no customer-readable report connecting tests to
product domains.

## Goals

- Make the functional suite browsable the way a customer or maintainer thinks
  about the product: workers, orchestration, workstations, transport, work,
  sessions, and related domains.
- Shape directories like the owning code, with explicit exceptions only for
  true cross-domain behavior.
- Validate Provider Session and response-event behavior from sanitized, real
  provider execution transcripts stored as golden fixtures.
- Define the complete intended functional-test file inventory before
  implementation.
- Make every test file a small, independently assignable work cell.
- Generate one Markdown artifact containing every functional test, its
  description, its source, domain owner, and production-package statement
  coverage.
- Eliminate featureless catch-all ownership and the deprecated `runtime_api`
  package.

## Functional-test boundary

Customer functional tests:

- build the application through `root.BuildProcess`;
- invoke `Process.Execute`, public HTTP, or public MCP;
- replace only exact external effects through `edges.Edges`;
- assert public CLI, API, MCP, Factory, Factory Session, Work, Provider
  Session, Factory Event, and FactoryResponseEvent behavior;
- do not import service implementations, Wire, Initializer, runtime scopes, or
  internal Petri-net primitives.

Unit tests remain the primary proof for pure parsing and exhaustive mapping.
Package integration tests prove bounded component collaboration. Stress/race
tests prove scale, fairness, and backpressure. Functional tests prove the
high-value customer flow across subsystem boundaries.

## Intended layout

Functional tests live under domain nouns that match the product and code.
There is no `features/` wrapper and no transport-first ownership for domain
behavior.

```text
tests/functional/
  transport/
    cli/                 # process contract, params, streams, thin command wiring
    http/                # server, routing, content negotiation, generated client
    mcp/                 # stdio, protocol, tool discovery
  workers/
    script/              # script workers
    inference/           # inference workers, provider selection, goldens
      <provider>/
    mock/                # mock worker behavior visible at the public boundary
  orchestration/
    javascript/          # JS/TS workflow runtime
    petri/               # graph/Petri orchestration behavior
  workstations/
    execution/           # ordinary execution workstations
    cron/
    repeater/
    poller/
    watcher/
  work/
    submission/
    relationships/
    routing/
    recovery/
    visualization/
  sessions/
    lifecycle/           # open/list/get/close
    controls/            # pause/resume/cancel/terminate
    execution/           # results, dispatches, run visibility
    restart/             # logical identity remap and resume
  factory/
    definitions/         # init, validate, import/export, defaults
    packaged/            # catalog and one matrix per shipped package
    current/             # current-factory read/save and templates
  provider_sessions/
    details/
    association/
  events/
    factory_events/
    response_events/
    replay/
  models/
  guards/
  resources/
  observability/
    logging/
    metrics/
  product/
    docs/
    dashboard/
  resilience/
    process/
    batch/
    platform/
  internal/
    support/             # only shared harness exception
```

### Ownership rules

1. **Mirror the code.** Prefer the domain a maintainer would open in `pkg/` or
   the customer vocabulary in `docs/architecture/data-model.md`.
2. **Transport is mechanics only.** `transport/cli|http|mcp` owns stdin/stdout,
   flags, routing, content types, protocol errors, and thin command/operation
   wiring. It does not own Work, Session, Worker, or Factory semantics just
   because a test entered through that surface.
3. **Domain owns domain proofs.** A customer looking for worker variants goes
   to `workers/...`, not `transport/cli/...`. The test may still invoke CLI,
   HTTP, or MCP; the destination folder names what is being proven.
4. **Cross-purpose is the exception.** When a scenario truly spans domains and
   has no primary owner, place it under the smallest primary domain and name
   the file for the secondary concern, or use a local `cross/` subsection only
   when necessary. Do not create a generic parity bucket.
5. **One file, one cell.** Each checklist path is independently assignable.
   Shared harness changes land only in foundation cells.

Examples:

| Customer question | Destination |
| --- | --- |
| How do inference worker variants behave? | `workers/inference/<provider>/...` |
| How does script worker execution fail? | `workers/script/...` |
| How does JS composition run? | `orchestration/javascript/...` |
| How does Petri/graph dispatch behave? | `orchestration/petri/...` |
| How does cron/repeater fire work? | `workstations/cron|repeater/...` |
| How do I submit work? | `work/submission/...` |
| How do session pause/resume work? | `sessions/controls/...` |
| How does CLI flag parsing work? | `transport/cli/parameters/...` |
| How does HTTP routing/status work? | `transport/http/...` |

## Provider execution goldens

Provider goldens are a first-class functional test input. Their detailed
contract is in [`provider-session-goldens.md`](provider-session-goldens.md).

The source-of-truth fixture root is:

```text
docs/temp/functional/provider-sessions/
```

Because `docs/temp/**` is ignored today, implementation must add a narrow
`.gitignore` exception that makes only
`docs/temp/functional/provider-sessions/**` tracked. Tests must fail with a
clear message if required goldens are absent, so CI can never silently skip
them.

Each golden case contains:

- sanitized provider process stdout/stderr and exit metadata;
- the request metadata supplied to the provider execution;
- normalized expected Provider Session identity and capabilities;
- normalized expected response-event sequence;
- normalized expected terminal invocation result;
- a manifest describing provider, fidelity class, source version, sanitizer,
  and fields intentionally normalized.

Functional tests replay the raw execution output through the same provider
adapter and application boundaries used by production. They do not construct
expected metadata by calling the code under test. IDs, timestamps, temporary
paths, and other nondeterministic values are normalized by an explicit
test-owned normalizer before comparison.

Golden-backed scenarios live under `workers/inference/<provider>/`. Public
Provider Session lookup/association scenarios live under `provider_sessions/`.

## Generated visualization

`make functional-test-viz` produces:

```text
.artifacts/functional-test-viz/
  functional-tests.md
  coverage.out
  coverage-summary.json
```

The command must:

1. run `functional-boundary-check`;
2. execute the required short functional coverage lane exactly once;
3. inventory top-level tests with `go/parser` and `go/ast`;
4. read the first sentence of each test's Go doc comment as its description;
5. infer domain ownership from its conforming path;
6. label short, long-only, golden-backed, deprecated, and undocumented tests;
7. show scenario counts by domain and test package;
8. show total and per-production-package functional statement coverage;
9. link each catalog row to its source file and line;
10. return non-zero on boundary, suite, metadata, coverage, or rendering
    failure while preserving diagnostic artifacts when possible.

The Markdown report starts with prioritized domain summaries in this browse
order:

1. transport
2. workers
3. orchestration
4. workstations
5. work
6. sessions
7. factory
8. provider_sessions
9. events
10. models
11. guards / resources
12. observability / product / resilience

## Test metadata

Every top-level customer functional test has a Go doc comment:

```go
// TestRunStdinPrompt verifies that `you run -` consumes one prompt from stdin,
// writes the primary result to stdout, and leaves stderr empty on success.
func TestRunStdinPrompt(t *testing.T) {
    // ...
}
```

The report inventories top-level tests. Literal subtests may be shown as
detail, but dynamic table cases are not separate catalog records. Helper-only
files and `tests/functional/internal/**` are reported as harness verification,
not customer scenarios.

Missing legacy descriptions are exact deletion-only debt. New undocumented
tests fail immediately. The debt must reach zero.

Golden-backed tests include a source comment or test-owned declaration naming
the fixture manifest. The visualizer validates that the referenced fixture
exists and reports its provider and fidelity class.

## Foundation work

### FND-001: Domain-layout enforcement

- [ ] Update the normative backend standard and `packaged-structure.md` to
  require `tests/functional/<domain>/<subsection>/...`.
- [ ] Teach `make pkg-structure` the domain tree above.
- [ ] Reject new shallow, catch-all, or unclassified scenario packages.
- [ ] Preserve `tests/functional/internal/support` as the only shared support
  exception.

### FND-002: Test metadata parser

- [ ] Parse test declarations and doc comments with the Go AST.
- [ ] Detect build constraints and golden declarations.
- [ ] Exclude helpers and internal support from customer counts.
- [ ] Add an exact deletion-only undocumented-test baseline.
- [ ] Test malformed source, Windows paths, build tags, table tests, and
  Markdown-sensitive descriptions.

### FND-003: Coverage JSON

- [ ] Add optional JSON output to `cmd/gocoveragecheck`.
- [ ] Include covered/measurable statements, percentage, package floor, and
  measurement exception.
- [ ] Preserve existing behavior when the option is absent.
- [ ] Write results after a completed run even when a floor fails.

### FND-004: Markdown generator

- [ ] Render prioritized domain summaries and detailed test rows.
- [ ] Render golden fixture provenance and undocumented/deprecated debt.
- [ ] Render per-production-package statement coverage.
- [ ] Provide stable ordering and golden tests for the report itself.

### FND-005: Make and CI integration

- [ ] Add `make functional-test-viz`.
- [ ] Ensure the suite executes once.
- [ ] Make `functional-boundary-check` unavoidable in required coverage CI.
- [ ] Upload Markdown, JSON, profile, and command log on success or failure.
- [ ] Add a Makefile contract test that does not run the full suite.

### FND-006: Provider golden harness

- [ ] Add the narrow `.gitignore` exception for the golden root.
- [ ] Define and validate `manifest.json`.
- [ ] Load raw request, stdout, stderr, and exit metadata.
- [ ] Normalize only manifest-declared nondeterministic fields.
- [ ] Compare Provider Session, response-event, and invocation-result goldens.
- [ ] Reject secrets, absolute host paths, credentials, and unsanitized prompts.
- [ ] Require an explicit update environment variable to rewrite expected
  output.

### FND-007: Existing-test migration ledger

Wave 0 migration authority:
[`migration-ledger.md`](migration-ledger.md) (planning-only; does not move
tests). Later move batches consume its row mappings and named deletion-only
batch ids.

- [ ] Map every current scenario to one target test file or a justified
  non-functional layer.
- [ ] Move `runtime_api` scenarios in deletion-only batches.
- [ ] Split `smoke`, `workflow`, `guards_batch`, `bootstrap_portability`, and
  `replay_contracts` by durable domain owner.
- [ ] Preserve short/long membership and specialty Make targets.

## Execution waves

### Wave 0: Shared foundations

FND-001 through FND-007. Domain cells may begin when their required harness
cell is merged; they do not need to wait for unrelated foundations.

### Wave 1: Entry and execution core

`transport`, `workers`, `orchestration`, and `workstations`. These answer the
first customer questions: how do I talk to the system, who runs work, how is
work orchestrated, and which workstation kinds exist.

### Wave 2: Runtime product surface

`work`, `sessions`, `factory`, `provider_sessions`, and `events`. These prove
submission, live session behavior, packaged/authored factories, provider
inspection, and durable/ephemeral event contracts.

### Wave 3: Supporting domains and resilience

`models`, `guards`, `resources`, `observability`, `product`, and `resilience`.

## Independent-agent protocol

Each test-file cell must state:

- exact destination file;
- prerequisite foundation cell, if any;
- public entrypoint;
- fixture or golden input;
- happy-path assertion;
- failure-path assertion;
- deterministic synchronization method;
- focused command to run;
- product defect discovered, if any.

Agents must not coordinate by editing one shared giant test file. Shared
harness changes land in FND cells. Domain files use the merged harness through
stable helpers. Two cells that require the same new helper declare that
dependency rather than racing to add competing helpers.

## Quality gates

```sh
make pkg-structure
make functional-boundary-check
make test-functional
make test-functional-coverage
make functional-test-viz
make lint
```

OpenAPI changes also require generation and API smoke checks. Concurrency cells
require focused repeat/race/stress evidence. Live provider calls remain opt-in;
the required lane replays sanitized goldens or uses exact external-effect
fakes.

## Success criteria

- Every intended test file in the granular checklist is implemented or marked
  with an approved wrong-layer rationale.
- Every customer test has an observable-behavior description.
- Every supported provider has sanitized successful and failing execution
  goldens, plus the fidelity cases it claims to support.
- Transport domains cover input, output, error, lifecycle, and streaming
  mechanics without owning domain semantics.
- Every packaged Factory has required-input, optional-input, success, and
  failure coverage under `factory/packaged/`.
- JavaScript and Petri orchestration have black-box coverage under
  `orchestration/`.
- `runtime_api` and broad featureless catch-alls reach zero.
- `make functional-test-viz` produces the complete inventory and package
  coverage from one suite execution.

## Non-goals

- Test count alone is not a success metric.
- The normal lane does not require paid provider credentials.
- Browser interaction remains in UI browser tests.
- Functional tests do not exhaustively cover pure parsing branches.
- Golden updates are never automatic during a normal test run.
- The suite is not organized primarily by CLI vs API feature families.
