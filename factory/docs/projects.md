# Projects and Project Leads

`project` Work is the outer control loop for one substantial outcome. It sits on
top of the existing delivery graph; it does not replace `idea`, `plan`, `task`,
CI, or `review` Work.

## Ownership

The meta-planner owns factory health, admission, capacity, stale-Project
detection, and repair of factory-level failures. A Project Lead owns one Project
request end to end: it reads current state, chooses the next idea batch, waits
for the inner graph, validates the integrated result, and repeats until the
Project acceptance criteria are proven.

The runtime Project Work and Factory Event stream are authoritative for
lifecycle. Project admission does not pre-create local state. On its first
dispatch, the Project Lead bootstraps this ignored working-memory directory from
the admitted payload and source plan:

```txt
docs/temp/<project-name>/
  request.md
  acceptance.md
  state.md
  progress.md
  validation/
```

The Project Lead creates `request.md` and `acceptance.md` once as durable
projections of the operator-owned Project payload. It may not edit them after
bootstrap. `state.md`, `progress.md`, and `validation/` are Project-Lead-owned
evidence. A required contract change is a blocked Project plus a proposed
delta, never a silent goal rewrite.

## Admission

Admit each outcome as one uniquely named `project:init` Work item. Its payload
must identify `projectRoot`, `sourcePlan`, `contractRevision`, the authorized
request, and the complete immutable acceptance criteria needed to bootstrap the
Project root. The meta-planner must not create or populate `projectRoot`.
Separate Projects have separate roots and no Work relation unless their outcomes
have a real semantic dependency.

Example:

```json
{
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "example-project",
      "workTypeName": "project",
      "state": "init",
      "payload": {
        "projectRoot": "docs/temp/example-project",
        "sourcePlan": "docs/internal/development/plans/backlog/example.md",
        "contractRevision": "example-project-v1",
        "request": "Implement and validate every acceptance criterion.",
        "acceptance": ["The behavior is proven from a clean build."]
      }
    }
  ],
  "relations": []
}
```

## Cycle semantics

Each Project Lead response is a `FACTORY_REQUEST_BATCH` containing every
currently known, well-scoped idea plus exactly one same-name `project-cycle`
item. There is no Project-Lead batch-size quota. The cycle item depends on every
emitted idea reaching `complete`, so it wakes the lead only after the existing
planner/executor/CI/reviewer chain has converged.

The lead must maximize safe throughput rather than use one umbrella idea as a
default. It builds a dependency/collision map and emits the complete known task
graph using package- or ownership-scoped ideas. Factory resource capacity,
worker availability, host CPU and memory, worktree creation, and provider
limits control active concurrency; excess emitted Work waits in the runtime.
Known semantic or shared-surface ordering is encoded with `DEPENDS_ON` rather
than retained as an unpublished later wave. Global baseline and final
integration gates remain separate from package implementation lanes. A
recovered umbrella worktree preserves prior evidence but does not force
unrelated remaining packages through the same sequential task.

For test-optimization Projects, shared-host compute saturation is normal. Local
timings prioritize and diagnose work; they do not gate package implementation
on an idle runner, low variance, repeated pristine baselines, or an imported
absolute threshold. A Project Lead should continue when a change materially
uses a proven optimization shape and preserves observable behavior, submit the
PR, and use its package-level latency result as the hill-climbing signal. An
improving package advances; a non-improving package receives the next bounded
optimization pass.

Cycle payloads are explicit:

- `continue`: re-enter the Project Lead from current repository/evidence state;
- `complete`: mark the Project complete after independent probes pass;
- `blocked`: hold the Project for a concrete external or factory-level issue.

If a dependency fails, cascading failure moves the cycle to `failed`; the graph
re-enters the Project Lead so it can diagnose and issue a smaller corrective
cycle. Twelve lead visits without convergence trip the Project loop breaker and
surface `project:blocked` to the meta-planner.

## Completion

Task review proves a task. Project completion proves the integrated acceptance
contract. Before `complete`, the Project Lead runs at least two blind read-only
probes from clean context, records their separate reports under `validation/`,
and requires both to pass. A failing probe produces a delta cycle; it never
silently repairs the implementation.
