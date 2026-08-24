# Service Shape Follow-Ups

Status: open. Written 2026-08-18 against `b21919b44`.

This is the successor planning document to
`decoupling-program-remaining-work.md`. That document recorded what the `dcp`
program finished and what it left. This one records the *shape* defects found
while answering "where should the shared vocabulary actually live", plus the
follow-ups identified while reading the result.

Everything numeric here was measured on `origin/main` at `b21919b44` by
materializing the tree and counting imports and qualified references. Where a
number is a proxy rather than a direct count, it says so.

## 0. One-line summary

Four services import a fifth to name a subprocess port. That subprocess port
carries workstation identity and Petri token bindings. The linters that should
have caught both read only production files, so a test-only dependency is
invisible and a production one gets attributed to the wrong place. None of these
is a large change; all three are the same mistake.

## 1. The shape we do not want

**Status: complete (2026-08-24).** Verified against merged PR #2188: the three
leaked host-effect ports (`workers.CommandRunner`, `workers.PTYAllocator`, and
`factory_runtime.InputFileSystem`) were privatized, and the final non-test peer
scan reports zero references.

**Rule.** An exported interface on a service's public root whose method set is a
*host effect* — process execution, filesystem, clock, network, PTY — may be
referenced by that service, by `pkg/services/edges`, and by `wire`. It must
never be referenced by a peer product service. A peer that needs the effect
takes it from `pkg/platform/*`.

### 1.1 Why most of what looks like a violation is not

There are 20 exported port-shaped interfaces on service public roots, 33
filesystem-shaped interfaces under `pkg/`, and 9 process-shaped ones. That count
reads alarming and mostly is not. Consumer-defined narrow interfaces are
idiomatic Go: `LoadingFileSystem`, `PersistenceFileSystem`, `ScaffoldFileSystem`,
`WorktreeFileSystem`, `AgentToolFileSystem`, `WorktreeGitCommander` and
`ContentStagingFileSystem` are each referenced only by their own service plus
`pkg/services/edges`, which is the declared external-effect aggregation point.
That is the design working, not debt.

**The discriminator is whether a peer product service names the type.** Under
that test there are exactly three violations:

| Symbol | Peer services | Refs |
| --- | --- | ---: |
| `workers.CommandRunner` | `automations`, `factory_runtime`, `factory_sessions`, `recordings` | 74 |
| `workers.PTYAllocator` | `factory_sessions`, `providers` | 2 |
| `factory_runtime.InputFileSystem` | `automations` | 2 |

Because the violation set is three rather than thirty, this rule can land at
zero tolerance in `cmd/pkgboundarycheck` *after* the three are fixed. It must not
become another baseline file — see §8 for why the repository does not need a
twenty-first ratchet.

### 1.2 `workers.CommandRunner`: the case study

`pkg/platform/process/command.go:17` already declares:

```go
type CommandRunner interface {
    Run(ctx context.Context, req CommandRequest) (CommandResult, error)
}
```

`pkg/services/workers/command.go:12` declares a second one with the same method
signature. The duplication is not the interesting part. This is:

```go
type CommandRequest struct {
    Command, Args, Stdin, Env, WorkDir      // subprocess inputs
    DispatchID, TransitionID                // engine identity
    WorkerType, WorkstationName, ProjectID  // domain identity
    CurrentChainingTraceID                  // trace identity
    PreviousChainingTraceIDs
    Execution     work.ExecutionMetadata
    InputTokens   []any
    InputBindings map[string][]string
}
```

Five fields describe a subprocess. Nine describe a work dispatch. **The type is a
work dispatch wearing a subprocess interface**, and that is precisely why it
climbed onto a public surface: a peer cannot say "run this" without also naming
workstations, transitions, and chaining trace IDs.

The four consumers, and what each actually needs:

| Consumer | What it does today | Destination |
| --- | --- | --- |
| `factory_runtime` | `providerCommandRunner`, `scriptCommandRunner`, `mockCommandRunnerFactory` — constructor arguments only | Disappears; wiring belongs in `edges`/`wire` |
| `factory_sessions` | `commandRunnerOverride` threaded into the child worker executor | Disappears; same |
| `automations` | `internal/script_poller.go` genuinely runs a shell command | Takes `platform/process.CommandRunner` |
| `recordings` | `internal/replay/side_effects.go` *implements* the port to replay recorded commands | Implements the platform port |

Four peers, 74 references, and **none of them need Workers.** Two are pure
dependency-injection plumbing; two want the platform port that already exists.
The target state: `CommandRunner` becomes internal to Workers, and external
callers hand Workers a work request instead of a command.

## 2. The request shape is still engine-shaped

This is a second, independent defect living in the same type. Fixing §1 without
it would relocate the problem rather than remove it.

`CommandRequest` carries `TransitionID`, `InputTokens` and `InputBindings` —
Petri-net internals. `workers.Token` (`execution_tokens.go:69`) carries
`PlaceID`. `pkg/services/work/contracts.go` carries `PlaceID`, `FromPlaceID` and
`ToPlaceID`. Counted across the Workers and Work public surfaces:

| Symbol | Occurrences |
| --- | ---: |
| `InputTokens` | 25 |
| `PlaceID` | 15 |
| `TransitionID` | 10 |
| `InputBindings` | 8 |

`CLAUDE.md` and `docs/architecture/data-model.md` both state the rule this
breaks: internal packages may use tokens, places, transitions, markings and
guards; **public surfaces should prefer customer-facing vocabulary.** Fifty-eight
occurrences of engine vocabulary across two service public surfaces is the
largest standing violation of that rule.

The intended shape is a work request independent of the orchestration engine: it
names the work, its inputs, and its destination in product terms, while the
engine's own identifiers stay inside
`factory_runtime/internal/orchestrators/petri`. A Workers service that accepts
that request is the "workers have to be cleaner" outcome — Workers stops being
the place where engine identity, subprocess policy and execution vocabulary all
meet.

**Sequencing.** Do §2 before or together with §1. If `CommandRunner` goes
internal while its request type is still Petri-shaped, the leak becomes a private
problem instead of a fixed one, and the work-request boundary that peers are
supposed to call through inherits the engine coupling.

## 3. The dependency linters cannot see test-side edges

This one caused a measurable wrong decision, so it is worth stating precisely.

**Measured:** every boundary checker skips test files.
`cmd/pkgboundarycheck` filters `strings.HasSuffix(entry.Name(), "_test.go")` in
at least six places (`main.go:1082`, `1658`, `1899`, `1958`,
`constructed_service_edges.go:58`, `initializer_behavior.go:54`);
`cmd/ownershipboundarycheck` at `main.go:209` and `peer_import.go:126`;
`cmd/packagetargetmanifestcheck` at `inventory.go:44`.

The checkers are therefore not *conflating* test and production edges — they are
**blind** to test edges entirely. Nothing in the repository reports that a
test-only edge exists, and nothing labels an observed edge as one class or the
other.

**The consequence, concretely.** A hand grep for `providers -> workers` returns
ten files. All ten are `_test.go`. The production edge count is zero, and the
real production direction is `workers -> providers` across 20 files — which is
the direction the architecture wants. Because no instrument distinguishes the two
classes, that grep was recorded as a structural blocker in `dcp-7`'s `prd.json`,
and a story was deferred on the strength of it. The blocker does not exist in
production code. Note that `go build` does not compile test files, so a build
check would not have caught the error either, and `go vet` would have reported
the test edges without saying they were test edges.

**The fix.** Boundary checkers should classify every edge as `production` or
`test-only`, report both, and gate on them separately: production edges blocking,
test-only edges reported and ratcheted on their own counter. Two things fall out
for free — a test-only edge stops being invisible, and it stops being mistakable
for a production one when someone greps by hand.

This is the highest-leverage item in this document. It is a small change to
existing checkers, and it removes an entire class of wrong architectural
conclusions rather than one instance of one.

## 4. Where the shared vocabulary should go

### 4.1 Execution-outcome vocabulary to `pkg/services/work`

`pkg/services/work` measures strictly upstream of both problem services with no
back edge: `workers -> work: 49 / work -> workers: 0`;
`providers -> work: 4 / work -> providers: 0`; `factory_definitions -> work: 36`.

What moves, all currently on the Workers public root:

| Group | Symbols | Refs | Peers |
| --- | --- | ---: | ---: |
| Result and token | `WorkResult`, `Token`, `WorkInput`, `WorkLineage`, `WorkMetrics` | ~140 | 5 |
| Outcome | `WorkOutcome`, `OutcomeAccepted/Failed/Rejected/Continue` | ~120 | 5 |
| Failure taxonomy | `WorkFailureType` + 11 constants, `WorkFailureFamily` + 4, `WorkFailureMetadata`, `CloneWorkFailureMetadata`, `FailureDecisionFromMetadata`, `FailureDetail`, `Failure` | ~110 | 6 |
| Diagnostics | `WorkDiagnostics`, `SafeWorkDiagnostics`, `CloneSafeWorkDiagnostics`, `SafeWorkDiagnosticsFromWorkDiagnostics`, `SafeWorkDiagnosticsEventPayload` | ~35 | 4 |
| Attempt | `AttemptContext`, `AttemptFacts`, `AttemptReason*` | ~12 | 3 |
| Proposal | `ProposedOutput`, `WorkProposedItemsFromProposedWork`, `ExpectedArtifactVerification` | ~13 | 3 |

Physically: `failure.go` (311), `safe_diagnostics.go` plus
`safe_diagnostics_forward.go` (462), `execution_tokens.go` (172),
`proposed_output_contracts.go` plus `proposal_mappers.go` (147), and the
result/outcome half of `execution_contracts.go` (761) — roughly 1,400 lines.

The membership test: if a *definition* could sensibly reference it, it is not
Workers' vocabulary. `factory_definitions` references all of the above because
its invocation-policy decision envelope grades outcomes; it is a consumer of
results, not of Workers.

**Open question, deliberately unresolved.** Whether `work` is the right home for
the work-request shape itself is still ambiguous. It is defensible relative to
what exists today, and measurably better than the two alternatives considered
(`events`, which is one consumer of three; or leaving it in `workers`, which is
what created the problem). It should be revisited once §2 lands, because an
engine-independent request shape may argue for a different owner. Do not treat
"it went to `work`" as settled architecture.

### 4.2 Session transcript vocabulary to `recordings`

The `Draft` / `Kind*` / `Phase*` / `ContentBlock*` / `*Payload` / `Delivery*` /
`Representation*` / `Fidelity*` / `Provenance` / `SessionLineage` cluster is
consumed by exactly `chat_sessions`, `factory_sessions`, `recordings` and
`worker_sessions` — the session and recording family, nobody else. `Draft` alone
has 80 references.

Recordings already owns the canonical event ledger, replay, and read-model
projections. A transcript *is* a recording, and moving it there gives factory
session events and session transcripts a single owner instead of two.

Two checks before committing: confirm `recordings` measures upstream of the other
three consumers, and note that `Draft` is currently declared in
`workers/internal/draftvalidation/contracts.go` and re-exported through the
472-line `workers/response_drafts.go`, so the move is internal-to-recordings plus
deleting a re-export.

### 4.3 Provider response metadata

`ProviderResponseMetadata*` (9 symbols), `ProviderDiagnostic`, `ProviderError`
and `ProviderIdentity` want to live in `providers`. With §3 resolved this is no
longer blocked by a phantom cycle, but it should still be sequenced after the
`workers -> providers` direction is confirmed clean by the reclassified linter.

Separately worth investigating: `providers.ExecuteRequest` and
`providers.ExecuteResult` are referenced by `operator_settings`, which imports
`providers` across 11 files. A settings service naming an execution request is
the next most suspicious edge after `CommandRunner`. Not yet diagnosed.

## 5. How to do any of these moves cleanly

Go type aliases make a vocabulary rehome a true strangler, which is the
repository's standing directive and the thing `dcp-7` did not attempt when it
tried the move as one packet:

```go
// pkg/services/workers/failure.go — transitional, deleted in the last lane
type  WorkFailureMetadata         = work.WorkFailureMetadata
const WorkFailureTypeTimeout      = work.WorkFailureTypeTimeout
var   FailureDecisionFromMetadata = work.FailureDecisionFromMetadata
```

- **Lane 1** creates the real declarations in the destination and reduces the old
  home to aliases. **Zero consumer edits.** A type alias is the identical type,
  so nothing can regress.
- **Lanes 2..N**, one per consuming service, rewrite imports. Small, independent,
  parallelizable.
- **Lane N+1** deletes the alias file and adds the edge to the boundary check so
  it cannot come back.

Use `var F = pkg.F` for functions rather than a
`func F(...) { return pkg.F(...) }` wrapper — the wrapper form reliably trips the
deadcode ratchet with an exported-wrapper +1.

**Budget for six registries on every move.** Both coverage manifests are sorted
*and* completeness-checked, so a move that desorts one aborts the gate before any
floor is evaluated. Also `package-structure-baseline.json`,
`backend-package-file-count.json` (deletion-only, never raise),
`unfinished-package-moves.json` and `ownership-inventory.json`. Never
bulk-regenerate; remove and re-add specific entries. Moving *well-covered* code
lowers the source package's ratio while the destination starts with no manifest
entry at all.

**`workers/worker_vocabulary_contract.go` and
`factory_definitions/owned_contract.go` assert the current ownership.** Amending
them is what makes §4.1 an ownership decision rather than a refactor.

## 6. The 44 unfinished package moves are a different shape

`docs/internal/baselines/unfinished-package-moves.json` holds 44 rows, and it is
worth being explicit that they are **not** cross-service relocations. Every row's
`successor` sits under the same owner — for example
`pkg/services/workers/internal/diagnostics` to `pkg/services/workers/internal`.
They are intra-service folds of transitional packages, closed by named cutover
proofs such as `CLN-WRK-FOLD-TOPLEVEL`. The semantics are trivial; the risk is
entirely in the registries listed in §5.

| Destination | Rows |
| --- | ---: |
| `factory_definitions/internal` | 7 |
| `operator_settings/internal` | 6 |
| `work/internal` | 6 |
| `factory_runtime/internal` | 4 |
| `recordings/internal/services/projection_query` | 4 |
| `recordings/internal/services/replay` | 4 |
| `workers/internal` | 4 |
| `factory_sessions/internal/services/runtime_opening` | 3 |
| `recordings/internal/services/canonical_ledger` | 3 |
| `factory_runtime/internal/services/orchestration` | 1 |
| `providers/internal/services/execution` | 1 |
| `recordings/internal/services/artifacts_export` | 1 |

So there are four distinct move shapes in flight, and conflating them has already
cost planning time:

1. **Fold** — the 44 rows above. Mechanical.
2. **Move-down** — a shared `pkg/transports/*` package serving exactly one owner
   moves under it. Completed as `dcp-t1..t5`. Requires measuring *both* import
   directions and routing peer in-edges through a shared facade.
3. **Rehome shared vocabulary** — §4. The only shape with real design content.
4. **Demote to platform** — §1. Small, and gated only on §2.

## 7. Transports still holds business logic

Non-generated, non-test code under `pkg/transports`:

| Subtree | Lines | Files |
| --- | ---: | ---: |
| `cli` | 25,693 | 110 |
| `mapping` | 12,318 | 38 |
| `acp` | 6,872 | 36 |
| `http` | 1,481 | 7 |
| `mcp` | 191 | 2 |
| **total** | **46,555** | **193** |

The `http` and `mcp` numbers are what a transport should look like: thin, and
small relative to the services behind them. The other three are not.

Largest single files, as candidate starting points:

```
1673  mapping/factoryconfig/factory_config_mapping.go
1577  mapping/factoryconfig/factory_config_mapping_internal.go
1420  cli/root.go
1057  acp/internal/stdio/session_prompt.go
 995  cli/run/invocation_error.go
 989  cli/run/factory_invocation_input.go
 930  acp/internal/stdio/prompt_stream.go
 919  cli/run/run.go
```

Line count is a proxy, not a diagnosis — a 1,673-line mapping file may be
legitimately large. The specific claim worth testing first is that `cli/run/*`
(invocation error precedence, invocation input construction, run orchestration —
roughly 2,900 lines across three files) contains invocation *policy* that belongs
in `factory_definitions` invocation policy or in `factory_sessions`, not in the
CLI. `acp/internal/stdio/*` is the second candidate: prompt streaming and
session-prompt handling are session behavior wearing a protocol adapter.

Scope this by reading, not by line count. It is listed here so it is not
forgotten, not because it is ready to plan.

## 8. Other measured debt carried forward

| Registry | Entries | Direction |
| --- | ---: | --- |
| `package-structure-baseline.json` | 558 | shrink-only |
| `backend-exemption-budget.json` | 474 | shrink-only |
| `go-unit-coverage-package-minimums.json` | 470 | floors, not debt |
| `go-functional-coverage-package-minimums.json` | 350 | floors, not debt |
| `functional-undocumented-tests.json` | 221 | shrink-only |
| `petri-public-surface-baseline.json` | 101 | shrink-only |
| `backend-package-file-count.json` | 62 | deletion-only |
| `frontend-deadcode-baseline.json` | 30 | shrink-only |
| `ownership-inventory.json` | 8 | shrink-only |

`petri-public-surface-baseline.json` at 101 entries is the ratchet that already
exists for §2. Whatever fixes the request shape should draw that number down
rather than adding a parallel instrument.

## 9. The API surface is noisy

Raised but not yet measured. The claim is that the public API has accumulated
more surface than the product needs, and that a pass to remove or consolidate
endpoints and schemas is worth doing.

Before planning it, produce the equivalent of the tables above: endpoint count by
resource, schema count, which endpoints have no dashboard or CLI consumer, and
which exist only to serve a single caller. `api/components/` and
`ui/src/api/generated/openapi.ts` are the inputs. Deleting public surface is a
compatibility decision, so this needs a maintainer in the loop in a way the
internal moves do not.

## 10. Suggested order

1. **§3, the linter test/production split.** Smallest change, and until it lands
   every other conclusion here is drawn with an instrument that cannot see half
   the graph.
2. **§2, de-Petri the request shape**, drawing down
   `petri-public-surface-baseline.json`.
3. **§1, `CommandRunner` internal to Workers**, plus `PTYAllocator` and
   `InputFileSystem`. Then land the host-effect rule at zero tolerance.
4. **§4.1 and §4.2 vocabulary rehomes** via the §5 alias strangler, one consuming
   service per lane. §4.1 needs the ownership decision first.
5. **§6 folds**, owner-scoped, one lane per owner-plus-destination.
6. **§7 transports**, scoped by reading `cli/run/*` and `acp/internal/stdio/*`.
7. **§9 API surface**, after it is measured.

## 11. Open defects, unchanged

Carried from `decoupling-program-remaining-work.md` §5, none fixed:

- Worker Sessions listing returns 200 with an empty array while workers run.
- `work move` only addresses idea-typed work; task and review tokens are
  unrecoverable when wedged.
- Eleven `@xyflow/react` test doubles omit `useStore`.
- `pkg/platform/process/command_process_test_unix.go` does not end in `_test.go`,
  so it compiles as production and its `Test` functions have never run.
- `findAvailablePort()` in `ui/integration/browser-test-harness.mjs` is a
  check-then-act race.
- `.github/workflows/ci.yml` uses `if: always()` where it means
  `if: ${{ !cancelled() }}`.
