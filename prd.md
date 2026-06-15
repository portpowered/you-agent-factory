# PRD: Dynamic Workflows Recovery Executor Review State Reconciliation

## Context

### Project Overview

`infinite-you` uses a queue-driven Factory Session runtime to move recovery work through `task:init -> process -> task:in-review -> review -> task:to-complete` and then into the final consume path. The planner depends on the durable queue and replayable event history to decide whether a recovery lane is still active, needs manual repair, or is already effectively complete.

### Customer Ask

Recover executor and review state reconciliation for dynamic workflow recovery lanes so completed executor or review work no longer leaves stale `task:init`, `task:failed`, or duplicate `review:init` residue for the same recovery trace. The result must explain the remaining three residual lanes concretely enough for a later planner pass to classify each lane as complete, safe manual-repair, or superseded queue noise without guesswork.

### Problem

As of 2026-06-15, live queue evidence and worktree evidence disagree for three dynamic workflow recovery lanes:

- `dynamic-workflows-recovery-session-backend-runtime` still appears at `idea:to-complete` with `work-task-24` failed even though its worktree progress shows implementation, verification, and review-ready status.
- `dynamic-workflows-recovery-mcp-install-plan-scope` still appears at `idea:to-complete` with `work-task-58` failed even though its worktree progress shows completed install-path implementation and smoke coverage.
- `dynamic-workflows-recovery-setup-workspace-git-pull-hygiene` still shows `work-task-59` at `task:init` plus duplicate `work-review-64` and `work-review-65` at `review:init` even though its worktree progress shows completed stories and verification.

This mismatch blocks further planning because the owning queue-reconciliation layer is not explained well enough to distinguish a real runtime bug from historical residue that can be repaired safely.

### High-Level Solution

Investigate the exact runtime, projection, and same-trace ownership path that handles executor completion, review creation, review completion, and residual token cleanup. Then implement the smallest safe repair that makes durable queue state converge on one coherent outcome per recovery trace. If investigation proves the runtime is already correct and only bounded historical residue remains, document exact preconditions for a manual `you work move` repair so later planning does not generalize it into a shortcut.

## Project-Level Acceptance Criteria

- Focused backend tests, replay coverage, or equivalent behavioral proof show that completed executor or review work no longer leaves duplicate `review:init` tokens or stale `task:init` or `task:failed` residue for the same recovery trace.
- The implementation or investigation output explains the current residual state of `dynamic-workflows-recovery-session-backend-runtime`, `dynamic-workflows-recovery-mcp-install-plan-scope`, and `dynamic-workflows-recovery-setup-workspace-git-pull-hygiene` concretely enough that a later planner pass can classify each lane as complete, safe manual-repair, or superseded queue noise without guessing.
- If durable runtime behavior is already correct and a manual move remains necessary for historical residue, the final repair path records exact safe preconditions, the exact lane shapes it applies to, and the exact queue states it must not be used for.
- Queue-state ownership stays in the existing Factory runtime, session, service, and projection vocabulary and does not introduce a standalone workflow-run model, new customer-facing dynamic-workflow nouns, or unrelated product-surface work.
- The recovered behavior keeps executor completion, review creation, review completion, and consume-path state transitions aligned across runtime state and replayed projections for the same trace.
- Quality gate: typecheck, lint, and focused backend tests for factory runtime, session execution, service, and replay surfaces pass.

## Goals

- Restore planner trust in durable queue state for dynamic workflow recovery lanes.
- Ensure one same-trace recovery lane converges to one coherent task or review outcome.
- Distinguish active runtime bugs from historical queue residue with reviewer-verifiable evidence.
- Keep any manual repair path narrow, explicit, and safe.
- Limit the work to queue reconciliation rather than widening into new dynamic workflow features.

## User Stories

### dynamic-workflows-recovery-executor-review-state-reconciliation-001: Explain the three residual recovery lanes from durable queue evidence
**Description:** As a follow-up planner, I want the three residual recovery lanes explained from queue and replay evidence so I can tell whether each lane is complete, safe to repair manually, or just superseded residue.

**Acceptance Criteria:**
- [x] The implementation or attached investigation output identifies the exact work IDs, review IDs, and same-trace lineage for the three named recovery lanes.
- [x] For each of the three lanes, the output states whether the observed mismatch comes from active runtime behavior, projection drift, duplicate review creation, failed post-processing, or historical residual queue state.
- [x] The explanation uses existing Factory Session, work, review, and event terminology and does not rely on hidden source-file knowledge to understand the classification.
- [x] Reviewers can map each named lane to one of these states from the produced evidence: complete, safe manual-repair, or superseded queue noise.
- [x] Typecheck passes.
- [x] Tests pass.

### dynamic-workflows-recovery-executor-review-state-reconciliation-002: Reconcile same-trace executor and review completion into one durable queue outcome
**Description:** As a maintainer of recovery lanes, I want executor and review completion to reconcile to one durable queue outcome so the same recovery trace does not leave duplicate review starts or stale task residue behind.

**Acceptance Criteria:**
- [x] When executor completion creates or advances review work for a recovery trace, the durable queue contains at most one active `review:init` outcome for that same trace.
- [x] When review work completes for that same trace, stale `task:init`, `task:failed`, or equivalent no-longer-authoritative residue is not left behind as if the lane were still blocked.
- [x] Replay or projection rebuilding from the same event history reaches the same reconciled queue outcome rather than reintroducing duplicate review or stale task-state residue.
- [x] The change stays inside existing queue-state ownership paths and does not introduce a new workflow-run model or unrelated dynamic workflow surface changes.
- [x] Typecheck passes.
- [x] Tests pass.

### dynamic-workflows-recovery-executor-review-state-reconciliation-003: Define a bounded manual repair path only for proven historical residue
**Description:** As a planner recovering old queue residue, I want an exact manual-repair rule only when the runtime is already correct so I can fix safe leftovers without turning a one-off move into a general shortcut.

**Acceptance Criteria:**
- [ ] If the investigation proves current runtime behavior is correct and the remaining mismatch is historical residue, the plan documents the exact preconditions required before any manual `you work move` repair is allowed.
- [ ] The documented manual path names the exact queue states, trace shape, and evidence required before moving work, including what would make the move unsafe.
- [ ] The documented manual path is explicitly limited to the proven residual lane shape and is not framed as a replacement for executor, review, or projection reconciliation logic.
- [ ] If code changes fully remove the need for manual repair, the story records that no manual move is required for the final path.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-recovery-executor-review-state-reconciliation-004: Prove reconciled queue behavior with focused replay and lifecycle verification
**Description:** As a reviewer, I want focused replay and lifecycle evidence for executor and review reconciliation so I can trust that completed recovery work no longer looks active in the durable queue.

**Acceptance Criteria:**
- [ ] Focused tests or equivalent behavioral proof cover at least one completed-executor path, one completed-review path, and the duplicate-review historical regression shape described in the customer ask.
- [ ] Verification proves the reconciled behavior across the owning runtime or projection layer rather than only asserting helper-level implementation details.
- [ ] Verification shows the three named residual recovery lanes have enough evidence for later planner classification without requiring a source-code audit.
- [ ] Verification remains focused on queue reconciliation and does not expand into API parity, MCP install expansion, or unrelated factory cleanup.
- [ ] Typecheck passes.
- [ ] Tests pass.

## High-Level Technical Design

- Keep ownership in the existing queue-state runtime and projection layers that decide task completion, review creation, review completion, and consume-path advancement for a recovery trace.
- Use the event stream as the canonical source of truth. The repair must explain whether the mismatch is caused by bad event emission, incorrect transition ownership, replay drift, duplicate same-trace review creation, or residue that only persists in durable queue state from older runs.
- Prefer one authoritative reconciliation rule per same-trace lane: once later work for the trace is authoritative, earlier stale task or review residue must no longer remain active in the durable queue or replayed projection.
- If bounded manual repair is necessary, record it as an operational rule with hard preconditions rather than as an implicit runtime behavior change.
- Verification should stay at the behavioral layer that owns trust for planners: focused runtime, replay, session, or service tests that demonstrate the final queue state and trace classification directly.

## Functional Requirements

1. FR-1: The system must explain the durable queue and trace lineage for the three named residual recovery lanes using existing work, review, and event vocabulary.
2. FR-2: The system must prevent the same recovery trace from retaining multiple active `review:init` outcomes after executor or review completion.
3. FR-3: The system must prevent stale `task:init`, `task:failed`, or equivalent no-longer-authoritative task residue from remaining active once a later same-trace outcome is authoritative.
4. FR-4: Replay or projection rebuilding from canonical event history must produce the same reconciled queue state as the live runtime for the covered regression shapes.
5. FR-5: If manual repair remains part of the final path, the implementation artifacts must document exact safe preconditions and unsafe counterexamples for that move.
6. FR-6: Focused automated or reviewer-verifiable behavioral proof must cover executor completion, review completion, and the duplicate-review regression shape.
7. FR-7: The work must stay within queue-state reconciliation boundaries and must not introduce standalone workflow-run resources, new customer-facing dynamic-workflow nouns, or unrelated surface changes.

## Non-Goals

- No new standalone workflow-run model or new customer-facing dynamic-workflow resource vocabulary.
- No widening into MCP install expansion, API parity work, or unrelated dynamic workflow feature work.
- No broad cleanup of runtime or factory packages beyond what is required to repair queue-state reconciliation.
- No generic manual queue-move shortcut for future recovery work without the exact proven preconditions from this investigation.

## Supporting Technical Considerations

- The most important boundary is ownership: the same layer that decides completion and review creation must also own cleanup or suppression of obsolete same-trace residue.
- Evidence must be trace-shaped, not only state-shaped. The planner needs to know whether duplicate or stale queue entries belong to the same recovery lineage.
- Replay matters because planner trust depends on durable history, not only in-memory live state.
- If the final answer includes a manual move, the artifact should name the exact command intent and preconditions without normalizing that move into routine product behavior.

## Success Metrics

- A completed executor or review recovery lane no longer appears to have duplicate active review work or stale failed or initial task residue for the same trace.
- Reviewers can classify each of the three named residual lanes from the produced evidence without additional source-code archaeology.
- Any remaining manual repair path is narrow enough that later planners can apply it safely without overgeneralizing it.

## Open Questions

- None.
