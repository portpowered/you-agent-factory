# Factory Plan Reporting

Status: deferred — contract shape unsettled
Date: 2026-08-08
Audience: whoever implements Factory-derived ACP plan updates
Related: `docs/internal/projects/acp-client/final-proposal.md` §6.2,
`docs/internal/projects/acp-program/README.md`

This document records a design discussion that did not reach a decision. It
exists so the next attempt starts from the findings and the argument rather
than re-deriving both. **Nothing here is implemented.**

The intent it serves: a Factory should report plan updates to an ACP client as
its work progresses through the graph, so a client renders the Factory's
progression as a checklist rather than as an opaque wait.

## 1. Current state, verified

### 1.1 Plans are provider pass-through only

There are exactly two PLAN producers in the tree, and both do the same thing:

| Producer | Trigger |
| --- | --- |
| `pkg/services/worker_sessions/publish.go` (`case "plan"`) | a provider agent's own ACP `plan` update |
| `pkg/services/factory_sessions/internal/responsestream/fragmentmap/mapper.go` (`case "plan"`) | the same, on the Factory Session stream |

Both fire only when a **provider agent** reports a plan of its own. Nothing
observes work-state transitions, Petri transition firings, or JavaScript
workflow stages. `factory_runtime`, `work`, and `factory_sessions` were each
checked for a graph-derived producer; there is none.

### 1.2 A real ACP plan update is currently unreachable

`mapping.Project` → `ProjectPlan` (`pkg/transports/acp/internal/mapping/dispatch.go`)
is the only path that emits a genuine `sessionUpdate: "plan"` frame. It is
reached only for **session-level** PLAN records, and those have no producer.

Since Worker output was routed into Worker Sessions, a provider's plan instead
travels through `mapping.ProjectWorkerChild` → `projectChildPlan`, which
**flattens the entries into a text blob** (`"description (status)"` joined by
newlines) inside that Worker's tool call. The structured steps ACP models as
first-class are destroyed at that boundary.

### 1.3 This matches the documented scope cut

`acp-client/final-proposal.md` §6.2 lists `PLAN` under "no output in L1 — a
scope cut, not because plumbing is missing." The current state is that cut,
honoured. This is not a regression to repair; it is a feature never built.

### 1.4 Test coverage

| Level | What exists |
| --- | --- |
| Unit | `TestProjectPlan`, `projectChildPlan` cases, plan Kind/Phase legality, `planDraftPayload` |
| Functional | one assertion — the scripted peer's plan text appears as child content in `acp_worker_child_events_test.go` |

The single functional assertion checks the **flattened text**, not a plan.
There are zero tests asserting a real `sessionUpdate: "plan"` frame reaches a
client, and zero asserting plan content tracks graph progress. Coverage is
consistent with §6.2's cut, so implementing this feature means writing its
tests from nothing — including the end-to-end frame assertion that does not
exist today.

Also noted while reading: `mapping.Project`'s doc comment still lists `PLAN`
among the `NO_OUTPUT` kinds while its switch routes `KindPlan` to
`ProjectPlan`. The code and its own stated contract disagree. Worth fixing
whichever way this lands.

## 2. The settled part

Two questions were settled before the contract shape stalled.

### 2.1 Per-orchestrator derivation, not dispatch-derived

The alternative considered was a single producer deriving plan entries
uniformly from Worker dispatches. It was rejected for two reasons.

**No lookahead.** A dispatch happens when work is already ready to run, so a
dispatch-derived plan only ever holds completed and in-flight entries. Every
entry would appear already `in_progress` and the list would only grow at the
bottom. That is a progress log wearing a plan's clothes, and it renders as
broken in any client that draws unchecked boxes. Entries existing *before* they
start is what makes a plan a plan.

**It cannot express stages.** A dispatch-level producer cannot see stages. In
`@you/spawn`, the planner stage is one dispatch and the fan-out stage is three;
a dispatch-derived plan shows four flat siblings with no way to say "stage 2 of
3". The uniformity that makes it cheap erases the concept being reported.

### 2.2 The two orchestrators have genuinely different units

They are not two renderings of one thing:

- **JavaScript workflow** — stages are statically knowable from `factory.js`.
  Real lookahead, a real plan. Reporting is by **stage completion**.
- **Petri** — the graph is often cyclic (`@you/loop` never terminates), so a
  whole-graph plan is not well defined. But a work item's path to its
  completion state *is* bounded and knowable, because the work type declares
  its states. Reporting is by **work item reaching its completion state**.

Only the orchestrator can see either unit.

### 2.3 One wire-facing owner

Whatever the contract, ACP gives a session **one** plan. Two producers writing
one plan needs an ownership rule for ordering and status conflicts. The
intended structure is per-orchestrator derivation behind a single component
that owns turning it into plan updates, so the wire never has two producers
racing.

Provider-reported plans stay inside their Worker's tool call — they describe
one Worker's internal steps, not the Factory's progression — but should
survive as structured entries rather than the flattened text of §1.2.

## 3. The unsettled part: contract shape

Three shapes were proposed. The axis that separates them is **how much of the
plan the orchestrator owns**: all of it, its structure and status, or only its
structure.

### Shape 1 — Orchestrator emits PLAN records

No new contract. Orchestrators publish a PLAN response event through the
existing publisher, carrying the **complete** entry list each time (ACP plan
updates are full-list replacements, not deltas).

```go
consume(Draft{
    Kind:  workers.KindPlan,
    Phase: workers.PhaseUpdated,
    Payload: workers.PlanPayload{Steps: []workers.PlanStep{
        {ID: "stage-1", Description: "plan the work", Status: "completed"},
        {ID: "stage-2", Description: "run 3 tasks",   Status: "in_progress"},
        {ID: "stage-3", Description: "merge results", Status: "pending"},
    }},
})
```

The plan becomes a first-class fact: it lands in the ledger, replays, reaches
the dashboard and recordings, and ACP is one more projector. Ordering comes
free from the existing sequencer.

Cost: both orchestrators independently build an entry list, so "how do I
describe myself as a plan" is written twice. Every change re-emits the whole
list.

### Shape 2 — Orchestrator exposes a projection, one consumer emits

The orchestrator answers a question instead of announcing anything. A single
Chat Sessions-side owner re-reads on relevant events, diffs, and emits only on
real change.

```go
package factoryruntime

type PlanEntry struct {
    ID          string
    Description string
    Status      PlanEntryStatus // PENDING | ACTIVE | COMPLETED | FAILED
}

type PlanProjector interface {
    ProjectPlan(ctx context.Context, sessionID string) ([]PlanEntry, error)
}
```

Orchestrators stay free of any emission concern; dedup, diffing and bounding
live in exactly one place; a third orchestrator is one method.

Cost: the plan is **not an event**, so it never reaches replay, recordings, or
the dashboard — ACP becomes the only surface that can ever have it. It also
needs a trigger, and the list of events warranting a re-read is easy to get
subtly wrong.

### Shape 3 — Orchestrator declares an outline, a shared deriver computes status

The orchestrator states only what it knows statically, at turn start. A shared
component maps runtime facts that already exist onto those entries.

```go
type PlanOutlineEntry struct {
    ID          string
    Description string
    Completion  PlanCompletionSignal // binds the entry to ledger facts
}

type PlanCompletionSignal struct {
    WorkTypeName  string // Petri: work of this type reaching...
    TerminalState string //        ...this state completes the entry
    StageIndex    int    // JS: this stage completing does
}
```

Best lookahead by construction — every entry exists `pending` before anything
runs. Status logic is written once.

Cost: `PlanCompletionSignal` is a leaky union, a shared type whose fields are
meaningful only per-orchestrator, growing a field per orchestrator added. And
dynamic work — a REPEATER fanning out, `agent.run` in a loop — does not fit a
pre-declared outline at all. The append escape hatch it needs collapses it into
Shape 1 with extra types.

### Comparison

| | Shape 1 | Shape 2 | Shape 3 |
| --- | --- | --- | --- |
| Plan replays / reaches dashboard | yes | **no** | yes |
| Lookahead (`pending` entries) | if the orchestrator emits them | if the projection includes them | by construction |
| Entry-building logic | duplicated | duplicated | shared |
| Handles dynamic work | naturally | naturally | **poorly** |
| New shared types | none | small | leaky union |

### The recommendation on the table when this was deferred

**Shape 1**, with entry-building factored into a helper both orchestrators
call.

The deciding row is replay: this system is event-first by design, and Shape 2
leaves the plan visible to ACP alone. A Factory's progression is a genuine
domain fact — the dashboard and recordings have as much claim to it as an ACP
client does, and Shape 2 quietly makes ACP the only consumer that can see it.

Shape 3 is the most elegant on paper and would win if Factories were static
pipelines. They are not: REPEATER fan-out and `agent.run` loops are normal
here, and the escape hatch handling them collapses the shape.

The shared helper recovers most of Shape 3's benefit honestly — both
orchestrators call the same code to turn "my stages" or "my work items" into
entries, including emitting the `pending` ones upfront, so lookahead becomes a
property of that helper rather than of a leaky shared struct.

This recommendation was **not accepted**; the shape remains open.

## 4. Decided alongside, if this is picked up

`workers.PlanPayload` carries `Summary` **or** `Steps`, and the child projector
flattens steps to text. Under any shape, `Steps` should become the real path
and `Summary` the degenerate fallback for providers reporting no structure —
otherwise structure keeps being lost at the boundary regardless of how good the
producer is.

## 5. Known consequences to design for

- **`@you/loop` grows its plan without bound.** Inherent to a long-lived
  controller. Bound it explicitly rather than pretend otherwise.
- **Dynamically created work appears mid-turn.** A REPEATER fan-out gives less
  lookahead than a JavaScript stage list. Do not fabricate entries for work
  that does not exist yet; partial lookahead reported honestly beats a
  complete-looking plan that is wrong.

## 6. Open questions

1. Which contract shape (§3).
2. Whether a provider's plan, once structured entries survive, should also
   influence the session plan or stay strictly inside its Worker's tool call.
3. For a JavaScript workflow, whether a stage is a named stage in `factory.js`
   or one `agent.run` call — `@you/spawn`'s fan-out is one stage and three
   runs, so the two differ.
