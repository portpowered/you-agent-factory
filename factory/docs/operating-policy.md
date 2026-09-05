# Factory Operating Policy

This policy is the control law for the long-running you-agent-factory. It
describes how the Factory chooses useful work, assigns authority, responds to
failure, and learns from evidence. The executable Factory definition remains
the runtime authority for Work states, Workstations, relations, resources, and
model selection. This document governs decisions made by those Workstations.

The canonical local Factory server for this deployment is
http://127.0.0.1:7437. Workers must pass that server explicitly to every
API-backed you command. Port 7437 is the Factory endpoint; do not infer a
server from the CLI default.

## Mission and control structure

The Factory is a production system for repository outcomes. Its control loop
is:

```text
observe Factory Session and Work
  -> classify health, evidence, and priority
  -> choose the smallest safe behavior slice or proof
  -> dispatch through the existing Work graph
  -> observe result and failure evidence
  -> reconcile and learn
```

The roles are deliberately separated:

| Role | Worker profile | Authority |
| --- | --- | --- |
| Portfolio Supervisor | Astra, medium reasoning with high autonomy | Whole-repository health, Project admission, cross-Project priority, exception handling, and Factory-level improvement |
| Project Lead | Sol, high reasoning | One Project's immutable contract, immediate behavior slices, local dependency map, and Project completion decision |
| Planning, delivery, and review workers | Luna, maximum reasoning | One local Work item and its declared planning, implementation, or review evidence |
| Validation workers | Luna, maximum reasoning | One read-only validation mission against one immutable build and fixture identity |

The Portfolio Supervisor is normally triggered every four hours. The runtime may
also trigger it for a significant exception. Project cycle completion and
ordinary child Work completion are handled by the Project Lead and the inner
graph; they do not themselves require an immediate portfolio-wide pass.

A significant exception is one of:

- a Factory Session, dispatch loop, provider, model, or required resource is
  dead or unavailable;
- a Workstation failure has no reachable Project feedback route;
- the same deterministic failure recurs after a documented correction;
- a Project is stale, stranded, contract-inconsistent, or consuming capacity
  without producing evidence;
- a required LocalAI or other real dependency changes readiness or fails its
  declared contract;
- a validation result is FAIL or BLOCKED on a current acceptance criterion; or
- a safety, authority, budget, persistence, or data-integrity decision cannot
  be made by the current role.

Do not turn every queue event into an exception. An exception must change the
supervisor's decision or require a Factory-level action.

## Priority order

The supervisor selects work by priority class before considering utilization
or convenience:

| Priority | Class | Examples | First useful action |
| --- | --- | --- | --- |
| P0 | Internal quality and stability | unreachable state transitions, missing failure feedback, corrupted or drifting contracts, dead sessions, dispatch loss, deterministic runtime defects, unsafe persistence | Restore a truthful, observable control loop or hold with the exact blocker |
| P1 | Functional quality | failed customer journeys, unmet Project criteria, regressions, LocalAI readiness/inference fidelity, model/provider behavior required by the contract | Repair the smallest behavior slice and validate the affected real edge |
| P2 | Documentation and distribution | public terminology, packaged Factory artifacts, generated contract alignment, examples, installability, docs usability | Update the canonical authored source and verify the delivered package |
| P3 | Auxiliary improvement | non-blocking ergonomics, cleanup, measurements, optimization, exploratory work | Schedule only when P0–P2 have no ready action and the outcome is bounded |

Within a class, score an item by:

1. the amount of customer or system risk it removes;
2. the number of downstream decisions it unblocks;
3. the quality and freshness of evidence it will produce;
4. whether a named owner and semantic prerequisite exist;
5. reversibility and rollout safety;
6. age of the valid request; and
7. cost, capacity, collision, and real-dependency exposure.

Do not raise a low-risk item because a Worker is idle. Do not lower a high-risk
item because it is inconvenient to validate. A Project with complete local Work
but unproven acceptance remains P1 until its missing evidence is obtained or a
named external condition is recorded.

## Cadence, liveness, and state

At each scheduled or exception-triggered supervisor pass:

1. read the current customer request and immutable Project contracts;
2. inspect the Factory Session, Work, relations, active Worker Sessions,
   provider/model readiness, resource capacity, and recent Factory Events;
3. compare active Projects with their latest cycle, child Work, failures,
   validation reports, and acceptance criteria;
4. reconcile unhealthy state before admitting new Work;
5. select one or a small number of dependency-ready actions in priority order;
6. record the decision and evidence in the supervisor state; and
7. stop when no safe, useful action remains.

The supervisor's durable working memory is local and untracked:

```text
docs/temp/progress.md
docs/temp/checklist.md
docs/temp/meta.md
```

Project working memory belongs under:

```text
docs/temp/projects/<project-name>/
```

Runtime Work and Factory Events are authoritative for lifecycle. These files
explain decisions and preserve evidence; they must never become a shadow queue.
Keep them concise and compact old entries when they no longer influence a
decision.

A liveness observation must distinguish:

- the Factory Session exists and accepts Work;
- Work is attached to a reachable Workstation;
- a Worker Session has started and is making progress;
- provider/model/resource dependencies are ready; and
- a terminal result or explicit hold is recorded.

A quiet queue is not proof of health. A running Worker is not proof of
progress. The next decision must name the observed signal.

## Project contract and autonomy boundaries

A substantial outcome is admitted as one `project` Work item. Its name is
unique for the outcome and its payload carries the authorized request,
acceptance criteria, contract revision, governing source plan, and canonical
root `docs/temp/projects/<project-name>/`.

The operator or admission path supplies these immutable Project files:

```text
docs/temp/projects/<project-name>/
  source-plan.md
  request.md
  acceptance.md
```

The Project Lead may maintain:

```text
docs/temp/projects/<project-name>/
  state.md
  progress.md
  validation/
```

The Portfolio Supervisor may inspect these files but may not rewrite an
immutable Project contract, weaken a criterion, or mark Project acceptance.
Only the operator can approve a contract amendment. A mismatch is a blocked
Project and an escalation containing the exact conflicting evidence.

The Project Lead is autonomous within that boundary. It decides the immediate
behavior slice, package/shared-surface ownership, semantic dependencies,
validation missions, and whether current evidence supports `continue`,
`complete`, or `blocked`. It does not implement delivery Work directly.

The supervisor admits a small unowned `idea` only when it is genuinely bounded,
has no active Project owner, and has an observable outcome. It must not use a
legacy loop to bypass a Project Lead.

## Work shaping and throughput

Shape Work around an observable behavior or a justified bounded enabler. A
behavior slice includes its customer/system outcome, relevant scope, owner,
failure behavior, evidence witness, dependency fidelity, and budget.

Use package or package-family ownership as the default collision boundary.
Package-first is an ownership rule: it assigns a shared surface to one Work
item. It is not permission to emit an inventory of every package or a complete
speculative roadmap. Emit only the next behavior slice that current evidence
makes ready. Put real semantic prerequisites in relations; do not hide them in
an unpublished future plan.

Resource capacity controls concurrency. Parallel Work is valid only when its
semantic prerequisites are satisfied and its branch/worktree/shared-surface
ownership is clear. Holding ready Work in a private prompt to make the queue
look small is prohibited. Emitting speculative Work to maximize utilization is
also prohibited.

A Project cycle is a synchronization point. The Project Lead emits exactly one
same-name `project-cycle` Work item alongside the current `idea` and
`validation` Work. The cycle depends on every current item reaching its
terminal success state. Local Work may complete while the Project acceptance
criteria remain open; the cycle then returns the lead for the next immediate
slice or proof.

The supervisor observes and classifies active cycles. It must not freely mutate
an active same-name cycle or bypass its dependency barrier. A cycle repair is
allowed only through an explicit runtime-supported route with a stable request
identity and recorded evidence; otherwise the supervisor reports the Factory
defect and lets the Project Lead or operator handle the next decision.

## Failure classification and escalation

Every non-terminal, failed, blocked, or apparently stranded item receives one
classification:

| Classification | Meaning | Allowed response |
| --- | --- | --- |
| `recoverable` | New evidence shows that a transient capacity, timeout, interruption, or dependency condition has cleared | One bounded retry or repair with a stable request identity, then re-inspect |
| `stranded` | The Work is valid but a failed transition left it outside its next Workstation | Move only to the valid input state and verify reachability |
| `deterministic_blocker` | Current evidence predicts the same failure again | Do not retry; issue a narrow prerequisite or hold for the named external condition |
| `scope_or_plan_failure` | The requested behavior, dependency, source plan, or criterion is wrong or incomplete | Return the evidence to the Project Lead/operator for a delta or amendment |
| `terminal_healthy` | The declared outcome and evidence are complete | No action |

A retry is never justified by age, an empty queue, a Worker becoming free, or a
hope that the model will behave differently. The same unchanged failure may be
retried at most once in a supervisor pass and only after the reason is
recorded. Repeated failure becomes a correction or hold.

Child Work failures must preserve their evidence and reach the Project Lead
through Project-cycle state and Factory Events. A plan, workspace, executor,
CI, review, or validation failure is not an implicit success for its parent.
If the route is missing or leaves a cycle permanently unable to wake, classify
that as P0 Factory instability and repair the topology before advancing the
Project.

Escalate when the role lacks authority, the Project contract must change, a
real dependency is unavailable, a budget or safety boundary must change, or
persistence/data integrity is uncertain. An escalation states the criterion or
health signal, reproduction/evidence, customer impact, safe action already
taken, and the smallest decision required.

## Validation missions

Validation is first-class `validation` Work processed through the ordinary
Project graph. It is not an informal subagent call, a checklist, or a vote.

A validation payload contains:

```text
role: customer | engineering | retrospective
project: Project name
mission: one observable question
criteria: criterion IDs with pass/fail rubrics
reportPath: absolute unique path under the Project validation directory
budget: time, download, disk, process, and paid limits
build.identity: immutable commit, artifact, or digest for customer/engineering
fixture.identity: immutable fixture or input identity when used
```

Customer missions receive only the acceptance contract, public entry points,
mission, rubric, and immutable build/fixture identity. They must not receive the
source plan, implementation plan, claimed fixes, or another validation report.
Engineering missions independently inspect failure behavior, regression,
persistence/recovery, performance, LocalAI/model fidelity, or other declared
quality properties. Both missions use a fresh Luna context at maximum
reasoning or xhigh and are read-only.

A Project Lead schedules two complementary evidence paths when completion is
near. Both must pass for Project completion. A FAIL or BLOCKED result remains
a failure even when another path passes. Validators may not edit, repair,
advance Work, weaken a rubric, or reinterpret an immutable criterion. Their
reports are saved at the declared path and include the exact artifact,
procedure, observed result, limits, and remaining unproven edges.

A retrospective uses role `retrospective`. It reports an observed pattern and
whether it is common or special cause, then proposes an owner, required
evidence, verification procedure, and rollback/stop condition. A retrospective
can inform Factory improvement; it cannot mark product acceptance complete.

## Learning and controlled change

The Astra supervisor aggregates retrospective reports on scheduled passes. It
promotes a learned rule only when:

1. the pattern is supported by more than one relevant observation or a strong
   single causal witness;
2. the proposed change has one accountable owner and a measurable behavioral
   witness;
3. the change is implemented through the canonical Factory definition,
   prompt, documentation, or runtime owner;
4. focused and integrated validation passes at the declared dependency
   fidelity; and
5. rollout has a canary or bounded scope, an observation window, and a
   rollback/hold condition.

Do not encode a global policy from a one-off model response. Do not turn a
retrospective into speculative cleanup. Preserve the causal distinction between
a common Factory defect that deserves a mechanism change and a special incident
that deserves a local correction.

## No-action and hold policy

After reconciliation, the supervisor records a hold and stops when:

- all active Projects have a reachable next transition or a named external
  blocker;
- no P0 or P1 criterion has an unowned or dependency-ready correction;
- P2 and P3 work is either progressing under an owner or has no justified
  immediate outcome; and
- no new evidence requires admission, validation, escalation, or route repair.

A hold records the reason, evidence, affected Work or Projects, owner of the
external condition, next scheduled four-hour review, and exception signals that
should wake the supervisor sooner. It does not create placeholder Work,
duplicate validation, weaken acceptance, or restart a healthy Project.

## Executable recovery boundaries

Every fifteen minutes the script reconciler wakes a waiting Project whose
same-name cycle is missing, after checking no lead is active. It preserves
existing children. A visible cycle remains owned by the normal graph; the
sweep never replaces it. Blocked Projects require supervisor diagnosis and
changed evidence before a deliberate retry. Timer passage is not retry evidence.

A child failure wakes its Project Lead to classify the failure; it does not
automatically resubmit the child. A blocked cycle, failed lead, or exhausted
lead visit budget passes through `project:needs-supervision` once, preserving
`project:blocked` and creating a supervisor thought. This route cannot repeatedly
consume the unchanged blocked state. Script sweep failures also notify the
supervisor. A stopped host cannot run its own timers; host restart supervision
is a separate deployment concern.

CI waiting, workspace preparation, cycle classification and recovery use
deterministic Python scripts. They do not consume a language-model worker.
