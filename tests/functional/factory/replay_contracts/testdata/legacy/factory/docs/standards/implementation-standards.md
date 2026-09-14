# Factory implementation standards

---
author: andreas abdi
last modified: 2026, august, 31
doc-id: FSTD-002
---

This standard governs implementation agents executing a factory task. The task
packet and its parent plan define scope; repository engineering standards
define the quality floor. Any test work **MUST** follow
[testing-standards.md](./testing-standards.md).

## Quick rules

- Read the parent plan, task packet, current repository instructions, and
  relevant repository standards before changing code.
- Execute one behavior task per iteration and preserve unrelated user work.
- Prove the task at the highest feasible verification level and preserve the
  parent lane's executable spine.
- Keep tests with the behavior they prove. Do not defer direct behavioral proof
  to a later test-only task or to loopback.
- Use production wiring for integration evidence and place controlled
  substitutes at external boundaries.
- Keep unit tests inside one component, run functional customer scenarios as
  parallel Factory Sessions without building a binary, and keep compiled
  integration and load evidence in their dedicated lanes.
- Do not overclaim evidence. Record what remains unproven and its owning gate.
- Do not silently broaden scope. Return a structured blocker or delta request.
- Never commit transient planning state, CI transcripts, audit notes, secrets,
  or paid-provider payloads.
- Stop at the implementation finish line; review owns terminal CI and merge.

## 1. Establish context and safety

Before editing, the implementer **MUST**:

1. identify the task's parent behavior, dependencies, behavioral witness,
   scope, non-goals, and shared-surface owner;
2. inspect the affected implementation and its existing behavioral coverage;
3. confirm prerequisites are present and the task can leave the branch
   releasable on its own;
4. identify repository-generated files and their canonical sources;
5. preserve unrelated changes already present in the worktree.

If reality contradicts the plan, stop before making an expansive architectural
choice. Report the contradiction, evidence, impact, safe work completed, and
the smallest recommended plan delta.

## 2. Implement the behavior slice

Implementation **MUST** stay within the task's single primary outcome. Changes
may cross technical layers when required to deliver that behavior. Incidental
cleanup is allowed only when necessary for correctness and should remain
proportional to the task.

The implementer **MUST** follow existing service and package boundaries,
preserve public vocabulary, update authored contracts before generated output,
and use existing abstractions and shared UI primitives unless the plan justifies
a new reusable boundary.

Structural work **MUST** preserve characterized behavior. Replacement work
**MUST** retain the declared canonical path and must not remove compatibility
paths before their planned removal task.

## 3. Produce task-owned evidence

The implementer **MUST** run the verification declared by the task and record:

- exact command or reproducible procedure;
- artifact or commit tested;
- relevant environment and dependency fidelity;
- observed result and exit status;
- the property proved;
- remaining unproven edges.

Focused unit evidence localizes rules. Functional evidence proves internal use
cases visible through a public customer boundary. Integration evidence
**MUST** consume an already compiled delivered artifact, use production wiring,
and cross a real production boundary. The test itself **MUST NOT** compile that
artifact. Runtime proof **MUST** exercise the actual delivered artifact when
the task changes runtime-observable behavior.

Paid validation must stay within the task's call, cost, and duration budgets.
It **MUST NOT** be used for failure matrices that controlled fault injection can
prove. Redact credentials and sensitive payloads from all evidence.

An unavailable optional tool is recorded once with its exact error and the
approved fallback is used. A missing mandatory dependency, authority, or
required real edge is a blocker, not a synthetic pass.

## 4. Handle failures and scope growth

The implementer fixes defects caused by the task and may iterate until its
declared evidence passes. It **MUST NOT** weaken assertions, replace real-edge
criteria with substitutes, or change acceptance criteria to fit the result.

Return a structured blocker when:

- a semantic prerequisite is missing;
- required authorization, credentials, infrastructure, or authority is absent;
- the smallest correct change materially exceeds the task's behavior;
- the task cannot remain independently releasable or revertible; or
- evidence reveals a plan or architecture contradiction.

The report **MUST** include the failed criterion, reproduction evidence,
customer/system impact, safe work completed, and the narrowest recommended
follow-up. "Too much work" without this evidence is insufficient.

## 5. PR and review handoff

The implementation stage owns responding to concrete blocking review feedback
with code, tests, or a documented correction. It does not own waiting for
terminal CI, resolving a conflict that has not been reported, or merging.

Implementation is ready to hand off only when:

- every task criterion is satisfied or has an explicitly permitted waiver;
- the behavioral witness has been exercised at the declared level;
- affected focused tests and quality gates have passed;
- generated artifacts are synchronized;
- the final implementation head is pushed;
- the PR is open and CI has started on that head; and
- all currently known blocking review feedback has been addressed.

After that point, implementation **MUST** stop and must not poll or re-check CI.
CI-run evidence belongs in a PR conversation comment and never in a commit.

## Implementation checklist

- Scope matches one behavior task or justified bounded enabler.
- Existing work and generated-source boundaries are preserved.
- Contracts, implementation, tests, and documentation agree.
- Test classification, boundaries, sessions, process reuse, parallelism,
  artifact ownership, and suite placement follow the factory testing standard.
- Failure and operational behavior required by the task are implemented.
- Evidence proves the named property at the declared fidelity.
- Remaining unproven edges have named gates.
- No assertions or criteria were weakened to obtain a pass.
- No transient factory artifacts or evidence-only commits are in the PR.
- The final head, PR, CI start, and feedback state satisfy the handoff boundary.
