# PRD: Dynamic Workflows Recovery Setup Workspace Git Pull Hygiene

## Context

### Project Overview

`infinite-you` uses a shared workspace-preparation path to create or reuse executor worktrees for queued implementation lanes. Dynamic workflow plan lanes depend on `factory/scripts/setup-workspace.py` to prepare a `.claude/worktrees/<work-item>` checkout, copy the PRD artifacts into that worktree, and leave the lane ready for implementation work without damaging the user's existing root checkout.

### Customer Ask

Repair shared `setup-workspace` behavior so a plan lane can create or reuse its executor worktree even when the root repository has staged or otherwise pull-blocking local changes. The fix must preserve user work, emit concrete failure diagnostics, cover the dirty-root or pull-failure cases with focused verification, and leave a clear next step for requeueing the blocked MCP-install plan lanes after the shared blocker is removed.

### Problem

`factory/scripts/setup-workspace.py` currently runs a root-level `git pull` before any worktree reuse or creation. When the root checkout has local staged or otherwise pull-blocking changes, setup fails before it reaches reusable-worktree or new-worktree paths. This blocks unrelated PRD execution lanes even though those lanes only need a safe executor worktree and PRD copies. The current failure surface is also too generic for later planning or queue-repair passes to distinguish root sync failure from worktree creation failure or PRD copy failure.

### High-Level Solution

Adjust shared workspace preparation so root-repo sync is attempted only when it is safe and necessary, and so dirty-root or pull-blocked conditions do not prevent reuse or creation of an executor worktree from the existing local repository state. Keep the change bounded to setup-workspace behavior, reuse current Factory and worktree vocabulary, and make remaining failures concrete enough for later queue inspection and requeue work. Add focused regression coverage for dirty-root fallback, no-upstream behavior, reusable-worktree behavior, and categorized failure output.

## Project-Level Acceptance Criteria

- `setup-workspace` can create a new executor worktree from the local repository state when the root checkout has staged or otherwise pull-blocking local changes, without resetting, overwriting, or unstaging that root state.
- `setup-workspace` can reuse an existing valid executor worktree when the root checkout cannot be pulled, and reusable-worktree preparation still completes when no additional sync is required.
- Root sync behavior still handles the no-upstream case safely and does not convert "no upstream configured" into a fatal error for workspace preparation.
- Failure output from `setup-workspace` identifies whether the blocking problem happened during root sync, worktree preparation, or PRD copy so later planner or queue-repair passes do not need to guess.
- The change stays inside shared workspace-preparation surfaces and preserves current Factory, Factory Session, and worktree terminology without introducing a separate workflow-run model.
- The repaired behavior leaves blocked plan lanes with an obvious requeue path once setup succeeds, such as rerunning workspace setup and moving the blocked MCP-install plan lanes back toward `plan:init`.
- Typecheck, lint, and focused tests for setup-workspace behavior pass.

## Goals

- Preserve user work in the root checkout while allowing executor worktree setup to proceed.
- Make dirty-root and pull-blocked cases observable, intentional, and reviewer-verifiable.
- Keep reusable-worktree and no-upstream paths working as part of the same shared setup flow.
- Produce concrete diagnostics that support later queue inspection and plan-lane recovery.
- Keep the fix narrowly scoped to workspace preparation rather than widening into dynamic workflow runtime or MCP behavior.

## User Stories

### dynamic-workflows-recovery-setup-workspace-git-pull-hygiene-001: Continue workspace setup when root sync is blocked by local changes
**Description:** As a maintainer recovering blocked plan lanes, I want workspace setup to continue from safe local repository state when root pull is blocked by local changes so executor worktrees can still be created without mutating the root checkout.

**Acceptance Criteria:**
- [ ] When the root checkout has staged or otherwise pull-blocking local changes, running `setup-workspace` does not reset, overwrite, or unstage those root changes.
- [ ] In the same dirty-root scenario, `setup-workspace` can still create the expected executor worktree for a PRD lane when the local repository already has enough history to do so safely.
- [ ] If root sync is skipped or downgraded because local changes would block pull, the outcome is explicit in observable script output rather than hidden behavior.
- [ ] The story remains limited to shared workspace-preparation behavior and does not introduce separate workflow-run terminology or unrelated git tooling cleanup.
- [ ] Typecheck passes.
- [ ] Tests pass.

### dynamic-workflows-recovery-setup-workspace-git-pull-hygiene-002: Reuse existing worktrees and preserve no-upstream behavior under the safer sync policy
**Description:** As a maintainer rerunning blocked work items, I want reusable-worktree and no-upstream cases to keep working under the repaired sync policy so existing plan lanes can recover without extra manual branch repair.

**Acceptance Criteria:**
- [x] When the expected executor worktree already exists and is valid, `setup-workspace` reuses it successfully even if the root checkout has pull-blocking local changes.
- [x] When the repository branch has no upstream configured, `setup-workspace` still treats that condition as non-fatal and can continue to reuse or create the executor worktree from local state.
- [x] If a reusable worktree needs its own branch update and that update cannot proceed safely, the script reports that failure as a worktree-preparation problem rather than a generic root-sync error.
- [x] Focused verification covers reusable-worktree and no-upstream paths without depending on unrelated dynamic-workflow runtime, API, or MCP-install behavior.
- [x] Typecheck passes.
- [x] Tests pass.

### dynamic-workflows-recovery-setup-workspace-git-pull-hygiene-003: Emit categorized setup failures that support queue recovery
**Description:** As a later planner or queue-repair pass, I want setup-workspace failures categorized by stage so I can distinguish root sync failure, worktree preparation failure, and PRD copy failure without guessing.

**Acceptance Criteria:**
- [ ] When root sync truly fails in a way that should still block setup, the script emits failure output that identifies the root-sync stage and the concrete blocking reason.
- [ ] When worktree creation or reuse fails, the script emits failure output that identifies the worktree-preparation stage and preserves the actionable git or filesystem reason.
- [ ] When PRD artifact copy fails after worktree preparation, the script emits failure output that identifies the PRD-copy stage separately from git setup failures.
- [ ] Failure output is concrete enough that a reviewer can map a blocked lane to the right follow-up action, including rerunning workspace setup and requeueing blocked MCP-install plan lanes after the shared blocker is fixed.
- [ ] Typecheck passes.
- [ ] Tests pass.

## High-Level Technical Design

- Keep ownership in the shared setup-workspace script and any directly associated verification surfaces. Do not widen into runtime, MCP host install, or API contract changes.
- Treat root sync as a best-effort preparation step with explicit safety checks. If local root state would make `git pull` unsafe or impossible, continue from local state when worktree preparation can still proceed safely.
- Preserve current worktree creation and reuse semantics, but make the decision points explicit: root sync outcome, worktree reuse or add outcome, and PRD copy outcome should each have distinct observable result categories.
- Prefer deterministic, stage-oriented diagnostics over generic shell failure text so later queue inspection can distinguish whether the next step is "retry after cleaning root," "repair or remove invalid worktree," or "fix PRD artifact copy."
- Verification should focus on observable script behavior: dirty-root fallback, no-upstream continuation, reusable-worktree success, and categorized failures.

## Functional Requirements

1. FR-1: The workspace-setup flow must preserve staged and other local root changes and must not reset, overwrite, or unstage them as part of creating or reusing an executor worktree.
2. FR-2: The workspace-setup flow must allow worktree creation to continue from existing local repository state when root-level pull is blocked by local changes and no additional remote sync is required for safe creation.
3. FR-3: The workspace-setup flow must continue treating "no tracking information for the current branch" or equivalent no-upstream conditions as non-fatal when local state is sufficient to continue.
4. FR-4: The workspace-setup flow must reuse an existing valid executor worktree when possible under the same safer sync policy.
5. FR-5: The script must emit failure output that categorizes the failed stage as root sync, worktree preparation, or PRD copy.
6. FR-6: Focused automated verification must cover dirty-root fallback, reusable-worktree behavior, no-upstream behavior, and categorized failure output.
7. FR-7: Documentation or task-facing output must leave a clear recovery path for rerunning setup and requeueing blocked plan lanes after the shared blocker is removed.

## Non-Goals

- No changes to dynamic workflow runtime, Factory Session execution, MCP tools, MCP host installation, API handlers, or dashboard behavior.
- No broad git-tooling cleanup outside the shared workspace-preparation flow.
- No new workflow-run vocabulary or separate product model outside the current Factory and worktree grammar.
- No automatic queue mutation or lane requeue orchestration beyond leaving clear diagnostics and next steps.

## Supporting Technical Considerations

- The script sits at a process and filesystem boundary, so failure handling should preserve actionable subprocess stderr while still classifying the stage clearly.
- Safe behavior should prefer existing local refs and worktree reuse before requiring remote sync.
- Regression tests should stay focused on behavior and avoid asserting internal helper layout or script structure.
- If script output is machine-consumed by later automation, failure categorization should remain stable enough for a planner pass to branch on it.

## Success Metrics

- A blocked plan lane can prepare its executor worktree successfully in a dirty-root repository without altering root user work.
- Reviewers can tell from one failed setup run whether the next action belongs to root cleanup, worktree repair, or PRD artifact repair.
- The two blocked MCP-install-related plan lanes have a clear shared-path recovery once workspace setup is repaired.

## Open Questions

- None.
