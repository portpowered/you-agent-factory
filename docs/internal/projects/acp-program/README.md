# ACP Program — Lane Map and Cross-Lane Contracts

Status: proposed
Date: 2026-08-02
Audience: planners and implementers across ACP, Chat, Events, Worker Sessions,
Runtime, Workers, Recordings, and packaged-service-structure

This document is the routing index for four independently executable lanes. It
owns lane boundaries, cross-lane contracts, and the decisions that let the lanes
run in parallel. It does not contain implementation plans; each lane owns its
own.

## 1. Lanes

| Lane | Project | Ships | Depends on |
| --- | --- | --- | --- |
| **L1 — ACP Core** | `docs/internal/projects/acp-client/` | Chat Sessions, ACP stdio transport, Events stream service, Factory Builder. Factory targets, streaming, cancel/close/load. | nothing |
| **L2 — Root Consolidation** | `docs/internal/projects/root-consolidation/` | The root contracts worker events require, plus opportunistic cleanup and dead-API removal. | nothing |
| **L3 — PSS remainder** | `docs/internal/projects/packaged-service-structure/` | Everything in packaged-service-structure not required by worker events. | L2 |
| **L4 — ACP Worker Events** | `docs/internal/projects/acp-worker-events/` | Worker Sessions control plane, Runtime dispatch cutover, child Worker tool calls, control fan-out. | L1 contracts, L2 |

L3 runs in the background and **gates nothing**. It is slow because it is
complex; that is acceptable because no other lane waits on it.

L2 is extracted *out of* L3 and inverted: packaged-service-structure consumes
L2's sealed roots rather than owning them. L2 is deliberately small — only the
highest-value root cleanup. Everything else defers to L3.

## 2. Decisions

These are settled. Lanes cite them rather than re-deriving them.

### D1 — No storage engine, ever

You introduces no durable event journal, no embedded database for its own state,
and no write-ahead log. There is no SQLite, bolt, or segment-file backend for
Chat, Worker, or Factory event history.

- Session state is **session-scoped**. It lives for the duration of the process
  and is discarded on exit. Terminating a session is a normal, acceptable
  outcome.
- Historical reconstruction comes from **the recorder's recorded JSONL
  artifacts** only.
- The existing read-only SQLite in `provider_sessions` stays read-only and stays
  scoped to inspecting third-party agent databases. It is not a precedent.

This supersedes the follow-on in `DEC-RUN-REC-DURABILITY` that anticipated
"Recordings durable log/cursor/retention." That work is cancelled, not deferred.
The process-local `CheckpointStore` adapter shipped under
`factory_runtime/internal/services/checkpoint_recovery` is therefore the
permanent answer, not an interim one.

### D2 — Persistence and event stream are separate concerns

packaged-service-structure currently bundles durable log, cursor, and retention
into one Recordings sequence step. That bundling is wrong and L3 revises it.

| Concern | Owner | Nature |
| --- | --- | --- |
| Canonical Factory history and replay | Recordings | JSONL artifacts on disk |
| Event stream: ordering, cursors, subscriptions, retention, gaps, backpressure | Events service | in-memory, session-scoped |

The Events service is a **stream**, not a store. It has no persistence
responsibility and no `internal/store/` package. Recordings remains the
canonical ledger and cedes nothing.

### D3 — `pkg/wire` is not an exclusive lease

Adding a service to the composition graph is a few lines in
`pkg/wire/wire.go`, `wire_gen.go`, and `profiles.go`. Treating that as an
exclusive path lease serialises every lane behind one packet for no correctness
benefit.

`pkg/wire/`, `pkg/root/`, and `pkg/initializer/` are **shared append-only
composition surfaces**. Conflicts there are resolved by normal rebase. No lane
holds a lease on them, no lane waits for a fan-in integration packet, and
composition edits are not phase gates.

Exclusive leases remain correct for paths where two packets would make
*semantically incompatible* edits to the same contract. They are not correct for
additive composition.

### D4 — Thin shims over current contracts, registered for deletion

L1 and L4 consume existing services through the narrowest consumer-owned port
that satisfies them, adapted from whatever the root looks like today — including
the 45-method `factory_sessions.Service`. Shims:

- live in the consumer package, never the provider;
- import only the provider's public root, never its `internal/` tree;
- change no behavior;
- are registered as deletion candidates in L2's catalog at the moment they are
  created, so they cannot rot silently.

Additive root operations are exempt: adding one deliberate operation to a root
is preferred over a shim that reaches around it.

### D5 — Opportunistic cleanup belongs to L2

Architecture cleanup is wanted, but it does not travel with feature work. Dead
API removal, duplicate-method collapse, alias-surface reduction, and
secondary-injection retirement are L2 tasks. L1 and L4 carry none of it.

### D6 — Target references are unversioned

`factory:@you/review` carries no version or digest. Resolving a target that has
changed or been uninstalled is a runtime error, not a historical-fidelity
problem. Accepted as-is.

### D7 — `pkg-file-count` / `pkg-boundary` are evaluated diff-scoped, not repo-wide-zero

`make pkg-file-count` and `make pkg-boundary` already exit non-zero on `main`
itself, independent of any lane's change. Confirmed on `origin/main` at
`b9c081e34` (2026-08-02): `pkg-file-count` reports 42 baseline violations
(`pkg/services/factory_definitions`, `pkg/services/factory_runtime`,
`pkg/services/factory_sessions`, `pkg/wire`, and others growing past their
`docs/internal/baselines/backend-package-file-count.json` entries, plus
several never-baselined oversized transport/internal packages), and
`pkg-boundary` reports 100 prohibited-import findings (mostly
`pkg/services/factory_definitions` consuming `pkg/transports/mapping/...`
directly). Neither list contains a lane's own package the first time that
lane's contract-only slice lands.

Both gates are pre-existing, deletion-only debt trackers scoped to packages
other lanes own (see `docs/internal/baselines/README.md`); they are not part
of the required merge-blocking CI umbrella (`.github/workflows/ci.yml`'s
`Verification Policy` job dependencies do not include `make lint` or either
check). A lane satisfies its own slice of these two gates by contributing
**zero new findings against packages it owns**, verified by running the
checker and grepping its output for the lane's own package path(s). Bringing
either gate to a literal repo-wide zero exit code is out of scope for any
single lane's feature work per D5; it requires a dedicated cleanup lane (or
scoping the Makefile/CI invocation to changed packages) and is not something
one lane's PR can deliver or should attempt, since the flagged packages
belong to other lanes/services and touching their debt out-of-band risks
colliding with concurrent work on those same packages.

Lanes citing this decision in review: link to this section instead of
re-deriving the evidence; re-run both checkers and re-confirm zero findings
against your own package(s) if a reviewer disputes it, since violation counts
drift as other lanes land unrelated changes.

### D8 — `backend-size` (`make lint`) is evaluated diff-scoped, not repo-wide-zero

`make lint` already exits non-zero on `main` itself because `backend-size`
(`go run ./cmd/backendsizecheck`) reports pre-existing function/file-length
violations outside any lane's change. Confirmed on `origin/main` at
`cbf49eb50` (2026-08-02): `backend-size` reports 69 violations, spanning
`cmd/`, `internal/`, `pkg/services/*`, and `tests/functional/*` packages
unrelated to Operator Settings; zero of the 69 name a file under
`pkg/services/operator_settings`.

`backend-size` is the same kind of pre-existing, deletion-only debt tracker as
D7's `pkg-file-count`/`pkg-boundary`, and it is not part of the required
merge-blocking CI umbrella either — `.github/workflows/ci.yml`'s only lint-like
required step is `make typecheck ui-lint`; no job in the `Verification Policy`
dependency list runs `make lint` or `backendsizecheck`. A lane satisfies its
own slice of AC-6's "lint passes" clause by contributing **zero new
`backend-size` findings against packages it owns**, verified the same way as
D7: run `go run ./cmd/backendsizecheck -root .` before and after the lane's
diff (in a disposable detached worktree of the pre-diff commit) and confirm no
new entry for the lane's own package path(s). Bringing `backend-size` itself to
a literal repo-wide zero exit code is out of scope for any single lane's
feature work per D5, for the same reason as D7: the flagged functions/files
belong to other lanes/services, and trimming them out-of-band risks colliding
with concurrent work on those same files.

Lanes citing this decision in review: link to this section instead of
re-deriving the evidence; re-run `backendsizecheck` and re-confirm zero new
findings against your own package(s) if a reviewer disputes it, since
violation counts drift as other lanes land unrelated changes.

## 3. Cross-lane contracts

Published on day 0 so all four lanes code against them with fakes immediately.
Contract definition is not a lane; it is the first task inside each owning lane,
and no lane waits for another lane's implementation.

| Contract | Owner lane | Consumed by |
| --- | --- | --- |
| `events.Service` — append, attach source, read, subscribe | L1 | L1, L4 |
| `events` record envelope and topic identity | L1 | L1, L4 |
| `chat_sessions.Service` — session, target, turn, control, attachment | L1 | L1, L4 |
| `ChatTargetRef` and target-catalog values | L1 | L1, L4 |
| `worker_sessions.Service` — start, turn, control, association | L4 | L4 |
| Sealed `workers` execution root | L2 | L4 |
| Sealed Runtime dispatch operations | L2 | L4 |
| Providers attempt-control capability results | L2 | L4 |
| `operator_settings.ACPAgentProfile` (additive) | L1 | L1, L4 |

### Vocabulary is not re-declared

`pkg/services/workers/response_drafts.go` is the canonical normalized event
vocabulary: `Kind`, `Phase`, `Draft`, `ContentBlock`, and the typed payloads
including `ToolPayload{ToolCallID, ToolName, Status, ArgumentsSummary,
ResultSummary}`.

No lane introduces a second taxonomy. The Events service carries source-native
payloads behind a delivery envelope; the *kind* taxonomy is the one above.
`factory_sessions/internal/responseevents` and
`factory_sessions/response_event_contract.go` are existing re-alias layers over
it and are migration inputs to L1, not new vocabulary.

## 4. Shared surfaces and how conflicts resolve

| Surface | Rule |
| --- | --- |
| `pkg/wire/**`, `pkg/root/**`, `pkg/initializer/**` | Shared, append-only, normal rebase. No lease. (D3) |
| `pkg/services/workers/response_drafts.go` | L2 may harden; no lane may extend the taxonomy. |
| `factory_sessions/internal/{responseeventstore,responsestream,cursors}` | L1 migrates these into `pkg/services/events`. L2/L3 do not touch them while L1 is active. |
| `pkg/services/factory_sessions` root | L1 and L4 read through shims only. L2 owns changes. |
| Recordings JSONL artifact format | Recordings-owned. Any lane needing a field files it as an L2 or L3 request. |
| `docs/architecture/{architecture,data-model,event-streams}.md` | L1 owns the reconciliation pass (§6). |

## 5. Non-goals across all lanes

- No durable event store, in any lane, at any phase. (D1)
- No second canonical Factory ledger. Recordings stays canonical.
- No universal normalized event vocabulary beyond the existing one.
- No repository freeze, baseline-recording task, or path lease on composition.
- No ACP filesystem callbacks, terminals, fork, agent plans, client-supplied MCP
  servers, or authentication in either ACP lane's first release.
- No Factory represented as a Model.

## 6. Documentation reconciliation owed

L1 carries these; they are currently inconsistent with every lane.

- `docs/architecture/architecture.md` names `pkg/services/chat`,
  `pkg/services/event_stream`, and `pkg/services/docs`. Actual packages are
  `chat_sessions` and `events`; there is no Docs service. Reconcile.
- `docs/architecture/data-model.md` documents `FactoryResponseEvent` as
  ephemeral and outside replay history. That remains true under D1 and D2 — but
  the public vocabulary gains `Chat Session`, `Worker Session`, `Target
  Episode`, and `Turn`. Add them.
- `docs/architecture/event-streams.md` sketches a third hierarchical event
  vocabulary. Either reconcile it with `workers.Kind` or mark it superseded.
- `packaged-service-structure/README.md` and its local plan need the D1/D2/D3
  revisions recorded.

## 7. Lane plans

| Lane | Plan document |
| --- | --- |
| L1 | `docs/internal/projects/acp-client/final-proposal.md` |
| L2 | `docs/internal/projects/root-consolidation/proposal.md` |
| L3 | `docs/internal/projects/packaged-service-structure/README.md` |
| L4 | `docs/internal/projects/acp-worker-events/proposal.md` |
