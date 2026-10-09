# Factory Operating Policy

This policy is the control law for the long-running you-agent-factory. It
describes how the Factory chooses useful work, assigns authority, responds to
failure, and learns from evidence. The executable Factory definition remains
the runtime authority for Work states, Workstations, relations, resources, and
model selection. This document governs decisions made by those Workstations.

The canonical local Factory server for this deployment is
http://127.0.0.1:7437. Workers must pass that server explicitly to every
API-backed you command about this Factory. Port 7437 is the Factory endpoint; do not infer a
server from the CLI default. A product test that builds its own binary and
runs it on a private port, HOME and fixtures is separate: it uses that
isolated server and never 7437.

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
| Portfolio Supervisor | GPT-6.1 Sol, medium reasoning with high autonomy | Whole-repository health, Project admission, cross-Project priority, exception handling, and Factory-level improvement |
| Project Lead | GPT-6.1 Sol, high reasoning | One Project's immutable contract, immediate behavior slices, local dependency map, and Project completion decision |
| Planning workers | GPT-6.1 Sol, high reasoning | One bounded plan and its admission evidence |
| Implementation workers | GPT-6.1 Sol, medium reasoning | One local implementation item and its declared delivery evidence |
| Review workers | GPT-6.1 Sol, medium reasoning | One independent review of a current implementation head |
| Validation workers | GPT-6.1 Sol, medium reasoning | One read-only validation mission against one immutable build and fixture identity |

The Portfolio Supervisor is normally triggered every eight hours. The runtime may
also trigger it for a significant exception. Project cycle completion and
ordinary child Work completion are handled by the Project Lead and the inner
graph; they do not themselves require an immediate portfolio-wide pass.

The supervisor and Project Leads submit new Work using the explicit-session
`you submit batch` CLI. Each batch is dry-run, submitted with a stable request
ID, and checked against the returned Work IDs and live Work list. Their final
workstation response is a decision envelope reporting that result, not a Work
request. A failed or uncertain CLI submission cannot be reported as accepted
Work; inspect the request ID before retrying to avoid duplicates.

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

A Project Lead batch contains only its immediate `idea` and `validation` Work,
each tagged `project: <project tag>`. It contains no loopback item: no
dependent `project-cycle` and no `thoughts` join. Instead, the runtime wakes
the owning lead (`project-lead-wake`) once for each tagged child that reaches
`complete` or `failed`, and names that child. A slow child therefore never
withholds the lead's reaction to a finished sibling or an unrelated red PR.
A marked mailbox park on a tagged lane also admits one version-keyed
`mailbox-question` project-report in its explicit Session. The existing wake
matches only its Project; this variant uses payload laneWorkId and requestVersion
rather than a terminal report's ParentID. Untagged lanes keep the operator route.
The lead answers within the immutable plan, acceptance and rules.md authority,
including narrowing changes, at the original shared responses/<lane>.md path,
and records authority, evidence and version in its progress.md. It forwards only
widening public exposure, adding an owner or a second path, growing a lint or
boundary baseline, contradicting immutable plan or acceptance, or factory/tooling
defects. A separate operator request references the original request/version;
the lead bridges a binding operator answer on any wake/check-in after rechecking
live membership and version. It never guesses an answer or duplicates forwarding.
AM-T0, the compatibility marker, no-slot wait and 60-minute/second-park routes
remain; admission outage keeps the bounded wait and check-in recovery.
A failed item needs a cause-corrected successor, decided on its wake. The
same-name `project-cycle` remains only as the relation-free terminal
`complete` or `blocked` decision.

An existing project-tagged thoughts loopback that finds gaps saves and dry-runs
a raw proposal at `docs/temp/projects/<project>/proposals/<loopback-name>.json`
in the main checkout. It submits no Project children and completes ACCEPTED
naming that path; admission ownership alone is not a failure. Actual write or
dry-run failures retain truthful evidence and any saved reference. Completed
and failed thoughts each emit one origin-preserving report through the existing
project-report/wake path; busy leads leave reports pending. The owning lead
reviews/edits against immutable authority and live ownership, admits ready fixes
with a verified receipt or records a reason/release event in progress.md, and
deduplicates by request/origin. Recovery attempts are positive integers with no
ceiling; lineage, workspace adoption and current-Session dependency rules remain. Untagged loopbacks
retain self-submission/receipt or accepted hold. This creates no new joins and
does not authorize lead Work controls.

Each lead pass or check-in inventories all current-Session Work pages, active
sessions, prior-Session carryover, and retained PRs at exact heads. It records
an owner or named release event for each relevant failed or open item. Old
failed Work is evidence, not a current owner. A check-in can use supported
Work controls for a proven stranded state and the explicit-session CLI to
admit a narrow current-Session successor. It cannot silently complete
delivery Work or use an old Session's Work ID as a current dependency.

The supervisor observes and classifies active Projects, their pending
`project-report` wakes, and any pending cycle. It must not freely mutate an
active same-name cycle or acknowledge another lead's reports. A repair is
allowed only through an explicit runtime-supported route with a stable request
identity and recorded evidence; otherwise the supervisor reports the Factory
defect and lets the Project Lead or operator handle the next decision.

## Worker browser tooling

The host's codex config loads browser and computer-use tooling into every
session: the Playwright MCP, Chrome DevTools MCP, the `node_repl` runtime, and
plugin servers such as the computer-use `cua_repl`. Each session that loads them
starts about 6 extra `node.exe` processes and about 0.8 GB of private memory,
even when it never opens a browser. At 16 workers that load adds up and causes
contention failures in timing-sensitive tests.

The `project-lead`, `planner`, `ideafier`, `processor`, and `reviewer` workers
in `factory/factory.json` start codex without that tooling. Each of those
workers sets these `args`, which the codex adapter adds as extra `codex exec`
options:

- `--config mcp_servers.<name>={command="none",enabled=false}` for
  `playwright`, `chrome-devtools`, and `node_repl`. Use the inline-table form.
  A bare `mcp_servers.<name>.enabled=false` makes codex fail at startup with
  `invalid transport` on a host whose config does not define that server.
- `--disable plugins`. This removes the plugin-supplied `cua_repl` server,
  which `-c plugins."<id>".enabled=false` does not remove.

This covers these workstations: `project-lead`, `project-lead-wake`,
`project-lead-checkin`, `plan`, `ideafy`, `process`, and `review`. The
`validator` worker, used by `validate`, keeps the full tooling.

When a lane needs browser verification, it opts in for that step only.
UI verification is still required by the repository standards. The worker runs
a nested `codex exec --dangerously-bypass-approvals-and-sandbox` from its shell
and gives it the verification instructions. The nested session reads the
unmodified user config, so it has the Playwright and Chrome DevTools MCP
servers. The browser processes then exist only while that step runs. A PRD or
payload that needs live browser evidence names this step. Repository browser
scripts such as `ui/` `storybook:*-check` and Playwright-backed `vitest` runs
need no opt-in. To give a whole worker the tooling back, delete its `args`.

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
as a child wake plus Factory Events. A plan, workspace, executor, CI, review,
or validation failure is not an implicit success for its parent. If a tagged
child reaches a terminal state without producing a `project-report`, or a
pending report never wakes its waiting Project, classify that as P0 Factory
instability and repair the topology before advancing the Project.

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
quality properties. Both missions use a fresh validator context and are
read-only.

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

The Sol supervisor aggregates retrospective reports on scheduled passes. It
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
external condition, next scheduled eight-hour review, and exception signals that
should wake the supervisor sooner. It does not create placeholder Work,
duplicate validation, weaken acceptance, or restart a healthy Project.

## Executable recovery boundaries

A Project Lead check-in runs every 15 minutes. Each tick binds one waiting
Project to a lead visit, so with N waiting Projects each one is visited roughly
every N quarter-hours. The check-in is a safety net beside child wakes: it
catches children stuck in a nonterminal state, untagged Work, and red PRs with
no Work. The lead inspects children, retained PRs, and evidence. It may make
a cause-corrected repair through supported Work controls or admit independent
ready, tagged idea/validation Work through the CLI. Healthy Projects with no
other ready Work remain waiting. Blocked Projects require changed evidence
before a deliberate retry; supervisor diagnosis is needed only when the lead
lacks a supported repair route. Timer passage is not retry evidence.

Each tagged child that reaches `complete` or `failed` passes through a
reporting state whose deterministic move emits one `project-report:pending`
Work named after the child and carrying its `project` tag. When the Project
is `waiting`, `project-lead-wake` consumes that report and wakes the lead,
which classifies and repairs; the report alone does not resubmit the child.
Reports that arrive while the lead is busy wait in `pending` and wake the
lead one at a time; the lead may acknowledge reports it already reconciled by
moving them to `delivered`. A Project without a `project` tag gets no wakes
and relies on check-ins. A blocked cycle, failed
lead, or exhausted lead visit budget passes through `project:needs-supervision`
once, preserving
`project:blocked` and creating a supervisor thought. This route cannot repeatedly
consume the unchanged blocked state. A failed lead check-in notifies the
supervisor. A stopped host cannot run its own timers; host restart supervision
is a separate deployment concern.

CI waiting, workspace preparation, cycle classification and recovery use
deterministic Python scripts. They do not consume a language-model worker.
