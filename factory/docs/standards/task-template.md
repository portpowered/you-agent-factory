# Task template

Every implementation task submitted by the factory **MUST** use this shape.
Replace every placeholder; do not depend on the original chat for context.

````markdown
### <TASK-ID> — <Observable behavior or bounded enabling outcome>

**Parent behavior:** <BEH-ID and one-sentence behavior>

**Problem:** <One sentence describing the gap this task closes.>

**Outcome:** <One independently reviewable and revertible result.>

**Plan reference:** <Absolute plan path and relevant heading/behavior ID.>

**Actor and trigger:** <Who or what initiates the behavior, with a concrete
input or precondition.>

**Dependencies:** <Semantic prerequisite task IDs, or `None`.>

**Parallel and shared-surface ownership:** <Tasks that may run concurrently;
owner for shared contracts, generated files, migrations, or UI surfaces.>

**Scope:**

- In: <Required contract, service, UI, documentation, migration, and test work.>
- Out: <Explicit non-goals and deferred behaviors.>

**Implementation constraints:**

- <Applicable package, service, dependency, compatibility, security, and
  repository-standard constraints.>

**Contract and configuration excerpts [Required when changed]:**

Authored source: `<path and component/operation/key>`

Current:

```yaml
<Exact current native contract/configuration shape, or `# Not present`.>
```

Proposed:

```yaml
<Exact proposed native contract/configuration shape, or `# Removed`.>
```

Generated outputs and consumers: <Explicit files/packages to regenerate and
verify.>

**Acceptance criteria:**

Every criterion has a stable ID, explicit `owner=process` or `owner=review`,
proof status and evidence/later gate. Use the planning standard's ownership
categories: terminal/hosted CI, merge, independent/post-merge validation and
reviewer judgment belong to review; direct implementation and its delivery
criterion belong to process. Legacy absent-owner objects and strings default
to process; invalid explicit owners never bypass obligations. Preserve immutable
IDs, requirements and gates. Story `passes` means retained process completion,
not review proof. Process leaves unproved review criteria false and lists their
IDs/gates as "owned by review"; all delivery prerequisites still apply.

Structured criterion example (also use these owners in the project mapping):

```json
{"acceptanceCriteria":[
  {"id":"C-1","requirement":"Focused behavior proof","owner":"process","passes":false,"gateIds":["U-1"]},
  {"id":"C-2","requirement":"Independent current-head review and terminal CI","owner":"review","passes":false,"gateIds":["R-1"]}
]}
```

- [ ] <Criterion ID | owner=process | gate ID> Given <state>, when <action>, then <observable result>.
- [ ] <Criterion ID | owner=process | gate ID> Given <relevant failure>, when <action>, then <defined error and state
  outcome>.
- [ ] <Criterion ID | owner=process or review | gate ID> <Named quality gate reports the property it measures.>

**Verification:**

- Behavioral witness: <Exact behavior demonstrating completion.>
- Executable-spine effect: <establish | preserve | extend |
  increase_fidelity | promote>.
- Required evidence:
  - Scope: <unit | functional | integration | end-to-end>
  - Dependency fidelity: <none | controlled | schema_mock | emulator |
    local_real | remote_real | remote_paid>
  - Command or procedure: <Exact reproducible command/procedure.>
  - Proves: <Property established by this evidence.>
  - Does not prove: <Boundary outside this evidence.>
- Highest feasible level: <Scope and fidelity, with reason.>
- Remaining unproven edges: <Edge -> later gate ID, or `None`.>
- Test-layer design when tests change: <Behavior and observer; selected layer;
  public/testable boundary; controlled dependencies; session and shared-process
  strategy; parallel isolation; prebuilt artifact owner for integration;
  dedicated load/stress or lint placement.>

**Paid validation, when applicable:**

- Trigger:
- Maximum calls:
- Maximum cost:
- Maximum duration:
- Fixture and output validator:
- Evidence-reuse key:

**Operational and rollout notes:** <Telemetry, migration, compatibility,
feature flag, stop condition, rollback, and cleanup relevant to this task.>

**Escalation:** Stop and return a structured blocker when the required change
exceeds this task's outcome, a prerequisite or authority is absent, or the
observed architecture contradicts the plan. Include evidence, impact, safe work
completed, and the smallest recommended delta. Do not broaden scope silently.

**Handoff artifacts:** <Code/contracts/docs generated, PR evidence, migration
notes, and any follow-up gate inputs.>
````

A bounded enabling task replaces actor/trigger behavior with the independently
verifiable capability it establishes and **MUST** justify why it cannot be
delivered inside the parent behavior slice.
