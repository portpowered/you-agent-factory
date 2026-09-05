# Projects and Project Leads

A `project` Work item is the outer control loop for one substantial,
independently owned outcome. It uses the ordinary idea, plan, task, CI, review,
consume, and validation Work graph. A Project is a customer-facing Factory
concept; implementation details such as tokens and places remain internal to
the runtime.

## Ownership

The Astra Portfolio Supervisor owns whole-repository health, Project admission,
cross-Project priority, Factory capacity decisions, significant-exception
response, and promotion of evidence-backed Factory improvements. It normally
runs every four hours and may be triggered sooner by a significant exception.

A Sol Project Lead owns one Project from its admitted contract through
independently validated completion. It chooses the next behavior slice, maps
semantic dependencies and shared-surface ownership, emits immediate idea and
validation Work, reconciles child outcomes, and decides whether the Project
continues, completes, or is blocked.

Luna delivery and review workers own one local delivery Work item at
maximum reasoning. Luna validation workers own one read-only validation
mission at maximum reasoning. No lower-level worker inherits Project or
Portfolio authority because an upstream role is delayed.

## Admission contract

Admit one substantial outcome as one uniquely named `project:init` Work item.
The payload must include:

- `projectRoot`, exactly `docs/temp/projects/<project-name>/`;
- `sourcePlan`, the operator-selected governing plan;
- `contractRevision`, a stable revision identity;
- `request`, the authorized outcome and constraints; and
- `acceptance`, the complete immutable criteria and evidence gates.

The operator or admission path must provide an immutable `source-plan.md` for
the Project root. It may be materialized from `sourcePlan` before the first
Project Lead visit, but no worker may synthesize or silently amend it. The
Project root is separate for every Project; unrelated Work and evidence must
not be stored there.

Example admission payload:

```json
{
  "name": "example-project",
  "workTypeName": "project",
  "state": "init",
  "payload": {
    "projectRoot": "docs/temp/projects/example-project",
    "sourcePlan": "docs/internal/development/plans/backlog/example.md",
    "contractRevision": "example-project-v1",
    "request": "Implement and validate the authorized outcome.",
    "acceptance": [
      "The customer-visible behavior is proven from an immutable build."
    ]
  }
}
```

The supervisor must reject or block admission when the outcome, ownership,
source plan, acceptance criteria, contract revision, or required capacity is
ambiguous. Separate Projects have no relation unless a real semantic
dependency requires one.

## Durable Project state

The Project root is durable working memory and evidence, not a second queue:

```text
docs/temp/projects/<project-name>/
  source-plan.md   # operator-provided governing plan, immutable
  request.md       # operator-provided request projection, immutable
  acceptance.md    # operator-provided acceptance projection, immutable
  state.md         # Project Lead's compact mutable state
  progress.md      # append-only Project cycle log
  validation/      # validation reports and retrospective proposals
```

The Project Lead verifies the root, name, source-plan identity, and contract
revision on first dispatch and on every later cycle. It may create mutable
state, progress, and validation directories when absent. It must never rewrite,
relax, reinterpret, or delete the immutable contract files. A missing,
conflicting, or drifted contract produces a blocked Project cycle and an
operator escalation with the exact evidence.

Runtime Project Work and Factory Events are authoritative for lifecycle. The
local files preserve the contract, reasoning, and evidence needed to make the
next decision. They must not be used to mark Work complete or to hide failed
transitions.

## Project cycle

A Project Lead batch contains the immediate ready Work plus exactly one
same-name `project-cycle` Work item. The batch may contain:

- one or more `idea:init` items for behavior slices or justified bounded
  enablers;
- zero or more `validation:init` items for independent proof; and
- exactly one same-name `project-cycle:init` item.

The cycle has one required-success dependency for every idea and validation
item in that same batch. The relation's precise type and endpoint fields are
owned by the executable Factory definition and the canonical batch-input
contract. The invariant is stable: the cycle cannot decide until every current
predecessor reaches `complete`.

A lead must emit only the immediate behavior and proof Work justified by
current evidence. It must not prewrite an entire speculative graph or issue
future cycles whose work is not yet ready. Use package or package-family
ownership to assign shared surfaces, then slice by independently verifiable
behavior. Package-first is an ownership boundary, not a package inventory.

A local idea or validation can complete while the Project contract remains
open. The Project cycle then returns the lead to choose another behavior slice,
increase dependency fidelity, or issue the missing proof. Empty local work is
not evidence of Project completion.

The conceptual outer flow is:

```text
project:init
  -> project-lead
  -> project:waiting
  -> project-cycle decision
  -> project:init | project:complete | project:blocked
```

The lead's ordinary delivery flow remains:

```text
idea:init
  -> plan
  -> plan:init
  -> setup-workspace
  -> task:init
  -> process
  -> task:awaiting-ci
  -> ci-wait
  -> task:in-review
  -> review
  -> task:to-complete
  -> consume
```

Validation is a first-class route:

```text
validation:init
  -> prepare-validation
  -> validation:ready
  -> validate
  -> validation:complete | validation:failed
```

A failed or rejected plan, workspace, executor, CI, review, or validation
outcome must preserve its failure evidence and reach the dependent Project
cycle. The lead then diagnoses and emits a smaller correction, changes a real
dependency, escalates a contract issue, or records an external hold. A failed
child must never be treated as a completed idea.

## Behavior slicing

Every idea must name one observable behavior or one bounded enabling capability.
Its payload should state:

- parent behavior and criterion IDs;
- requested outcome and explicit scope in/out;
- relevant source-plan section;
- package/shared-surface owner and collision exclusions;
- semantic prerequisites and relation requirements;
- failure behavior and recovery expectation;
- behavioral witness and exact verification procedure;
- highest feasible evidence scope and dependency fidelity;
- remaining unproven edges and later gate owners; and
- time, call, cost, safety, and authority limits.

Use a vertical slice when contract, implementation, test, and documentation must
change together for the observable outcome. Use a horizontal enabler only when
it is independently useful and safe, such as characterization evidence,
reusable harness infrastructure, or a migration seam. Do not ask one planner
or task to solve an entire repository in one PRD.

## Validation missions

Validation is ordinary Project Work, not an informal subagent call. Every
validation payload is self-contained and immutable for its run:

```json
{
  "role": "customer",
  "project": "example-project",
  "mission": "Exercise the public behavior from a fresh context.",
  "criteria": [
    {"id": "criterion-1", "rubric": "The expected observable outcome occurs."}
  ],
  "reportPath": "<absolute-workspace>/docs/temp/projects/example-project/validation/example-customer.md",
  "budget": {
    "time": "<maximum duration>",
    "download": "<maximum download>",
    "disk": "<maximum disk use>",
    "process": "<maximum process use>",
    "paid": "<maximum paid spend>"
  },
  "build": {"identity": "<immutable commit or release>", "path": "<absolute-prebuilt-binary-path>", "sha256": "<64-character-sha256>"},
  "fixtures": []
}
```

The required fields are role, Project name, mission, criterion IDs with rubrics,
an absolute unique report path under the Project validation directory, explicit
time/download/disk/process/paid budgets, and an immutable build identity for
customer and engineering missions. Fixture and public-document paths are
optional when part of the witness.

Roles are:

- `customer`: fresh source-blind exercise of the public customer journey;
- `engineering`: fresh independent check of regression, failure behavior,
  persistence/recovery, performance, LocalAI/model fidelity, or another
  declared quality property; and
- `retrospective`: fresh evidence review that proposes a bounded improvement.

Customer missions receive only the immutable acceptance contract, public entry
points, mission, rubrics, and build/fixture identity. They must not receive the
source plan, implementation plan, claimed fixes, or another validator's
report. Engineering missions receive the contract, mission, rubrics, and
immutable artifact identity needed for the named quality property; they must
still be independent of implementation claims.

Validation workers run in fresh Luna contexts at maximum reasoning. They are
read-only and may inspect or exercise only the declared artifact and fixture.
They may not edit files, fix defects, advance Work, or weaken a rubric. The
report records the artifact identity, procedure, observed result, dependency
fidelity, budget use, and remaining unproven edges.

When completion is near, the lead emits two complementary paths: one customer
and one engineering. Both must pass. A FAIL or BLOCKED result cannot be
outvoted by a pass. The lead enqueues the smallest evidence-driven correction
on a later cycle.

A retrospective report is evidence for the Portfolio Supervisor. It never
marks Project acceptance complete. Its action proposal names the observed
common or special cause, one owner, evidence required, verification procedure,
and rollback or stop condition. The Astra supervisor aggregates retrospective
reports on its scheduled pass and promotes a Factory rule only through a
validated change and controlled rollout.

## Completion and blocking

A Project is complete only when:

1. every immutable acceptance criterion has current evidence;
2. all required implementation, review, CI, and documentation gates are
   satisfied;
3. the complementary customer and engineering validation paths both pass;
4. all required real dependencies, including LocalAI when named by the
   contract, are exercised at the declared fidelity or explicitly waived by
   the owning authority; and
5. Project state records the build identity, validation reports, remaining
   unproven edges, and final decision.

Do not complete because local Work is complete, a cycle limit is near, the
queue is quiet, or no obvious task comes to mind. If no Factory-owned action
can resolve a concrete external blocker, emit a blocked cycle with the exact
condition and owner. If another behavior slice or validation is ready, emit a
continue cycle. A blocked contract is escalated to the operator; it is never
weakened by the lead.

## Recovery and no blind retry

On an interrupted or failed cycle, inspect current Work, relations, Worker
Sessions, branches/worktrees, provider evidence, CI, and Project files before
acting. Preserve valid work and reports. Reuse a previous Work name only when
its edits or evidence are indivisible and still satisfy the immutable contract.

A retry requires new evidence and a concrete correction. Temporary provider
capacity, timeout, or interruption may justify one bounded retry after the
condition clears. Deterministic failure, unresolved review feedback, a
contract mismatch, or an absent external dependency requires a smaller
correction or hold. Never resubmit unchanged failing Work in the hope that a
different model turn will fix it.

Probe preparation requires a prebuilt binary at an absolute `build.path` and
its exact SHA-256 in `build.sha256`; it copies verified bytes into the fresh
probe directory. Use a new validation Work name for each attempt: an existing
probe directory is rejected, including a retry after preparation failure.
Only role, project, mission, criteria, reportPath, budget, build, fixtures and
publicDocs are admitted mission keys. Customer rubrics must describe desired
public behavior, never implementation hints or another probe's result.
