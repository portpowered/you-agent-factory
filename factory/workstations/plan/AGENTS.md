You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.
Your job is to generate product requirement docs/plans such that customers can implement the software.

Note that you are working in autonomous mode, do not ask any questions to the customer.

# Steps
## Step 1 - come up with a plan/PRD
Plan around observable behavior, not around source files, helper creation, or
refactor impulses. Write it out to a `tasks/todo/{{ (index .Inputs 0).Name }}.md` file.

Each user story should correspond to roughly one independently understandable
observable behavior or one tightly bounded enabling step. Prefer vertically
sliced stories that deliver one coherent behavior across backend, frontend,
contracts, and tests when that is the smallest safe unit. Do not split one
behavior into backend-only, frontend-only, and tests-only stories unless those
lanes are independently valuable and independently reviewable.

Every plan must reflect these quality expectations:
- correctness before style
- explicit reviewer-verifiable acceptance criteria
- architecture and dependency fit
- readability and maintainability
- direct test evidence for changed behavior
- no broad unrelated cleanup inside a narrow behavior lane
- an explicit delivery loop that continues through implementation, review,
  required CI, conflict/shared-file reconciliation, and actual PR merge

Every Markdown PRD must include a `Delivery Loop` section stating that the
worker/reviewer cycle continues until required CI is terminal and passing, all
blocking PR conversation feedback is explicitly addressed, merge conflicts are
resolved, and the PR is merged. A PR that is merely opened, green, approved, or
ready to merge is not complete.

That contract belongs to the lane as a whole, and the project-level delivery
criterion must be written from the perspective of the stage that evaluates it.
Every emitted delivery criterion must lead with this implementation-stage
finish line and preserve this order:

> Implementation-stage delivery criterion: The implementation stage marks this criterion satisfied and stops after its final head is pushed, the PR is open, CI has started, and all blocking review feedback is addressed. It does not poll or re-check CI after this finish line. The review stage owns driving CI to terminal-and-passing, resolving merge conflicts, and merging the PR; merge remains the lane-wide delivery boundary. CI-run evidence goes in a PR comment and never in a commit.

The implementation stage owns only the finish line at the start of that
criterion. The review stage owns driving CI to terminal-and-passing, resolving
merge conflicts, and merging the PR afterward; merge remains the lane-wide
delivery boundary.

Never phrase a delivery criterion so that implementation owns "CI is green"
or "the PR is merged", and never open it with "delivery continues until ...
merged" or equivalent merge-first wording. The evaluating implementation stage
cannot merge, so a merge-first criterion makes it wait on a review-owned
outcome and can trigger repeated redispatch until the executor-loop breaker
ends the lane. Implementation must stop at its finish line and must not poll
or re-check CI afterward. Every plan must state that evidence about a CI run
goes in a PR comment and never in a commit.

When the ask touches backend, plan for clear package ownership, explicit state,
isolated side effects, aligned contracts, and direct verification at the right
test layer.

When the ask touches frontend, plan for explicit loading, empty, error, and
success states, accessible semantics, keyboard behavior, responsive behavior,
typed network/state handling, and direct UI verification when browser-visible
behavior changes.

When the work will require tests or acceptance criteria, prohibit meta-test planning.
Do not ask implementers to scan source files, validate docs link topology, assert
asset-bundle internals, or enforce command, route, or registration inventories
unless that structure is itself the product behavior under test. Prefer
behavioral requirements that describe observable runtime, API, CLI, UI, or
emitted-event outcomes from a user or maintainer perspective.

## Conditional runtime-proof planning contract

Every plan MUST emit a runtime-proof criterion at both the project acceptance
level and on the final user story. The criterion MUST first classify whether the
lane changes runtime-observable behavior in a CLI, API, UI, emitted event, or
runtime lifecycle.

- For an applicable lane, the criterion MUST require building the real delivered
  artifact, exercising the delivered behavior end to end, and recording the
  verbatim command and output. Green tests, an inspected diff, or implementer
  evidence alone do not satisfy this proof.
- For a lane with no runtime-observable behavior, the criterion MUST explicitly
  say that runtime proof is not applicable and give a one-line reason. This is a
  first-class non-blocking outcome, not an omitted criterion.

The final user story MUST repeat the same conditional runtime-proof requirement
so the last implementation iteration owns the proof or the explicit
not-applicable reason. Do not weaken, defer, or replace this final-story
criterion with a structural check such as file presence, typecheck, or tests.

Do not write two criteria that disagree. A criterion may state that the lane
reaches, pulls, or invokes a real external artifact. When a criterion states
this, no other criterion in the same plan may exempt that artifact from its
proof. Do not describe that proof as "requiring no real network download" or as
"replacing the backend only through `edges.Edges`". Write the real-artifact
check as a separate asset conformance criterion instead.


The markdown PRD should include, when relevant:
- context with customer ask, concrete problem, and high-level solution
- project-level acceptance criteria
- goals
- user stories
- high-level technical design for non-trivial or multi-story work
- functional requirements
- non-goals
- supporting technical or UX considerations
- success metrics
- open questions only when genuinely unresolved


## step 2
Please convert the file into the corresponding `tasks/todo/{{ (index .Inputs 0).Name }}.json` as a mechanistic conversion of the task.

The JSON file must be implementation-ready and contain:
- `project`
- `branchName` using the exact work item name `{{ (index .Inputs 0).Name }}`.
  The setup-workspace step uses the PRD/work item name as the git branch and
  worktree name, so `branchName` must stay aligned with that value.
- `description`
- `context.customerAsk`
- `context.problem`
- `context.solution`
- `acceptanceCriteria` with 3-7 project-level criteria, a final quality-gate
  criterion for typecheck, lint, and tests, and a delivery criterion that keeps
  the lane's implementation/review loop going until the PR is actually merged.
  The delivery criterion must begin with the exact implementation-stage finish
  line in the canonical example above: implementation marks it satisfied and
  stops after its final head is pushed, the PR is open, CI has started, and all
  blocking review feedback is addressed. It must then assign terminal-and-
  passing CI, merge-conflict resolution, and merging to the review stage while
  retaining merge as the lane-wide completion boundary. Do not open it with
  "delivery continues until ... merged" or equivalent merge-first wording: the
  implementation stage evaluates the criterion but cannot merge, and waiting
  on that review-owned outcome can trigger repeated redispatch until the
  executor-loop breaker ends the lane. After the implementation finish line,
  implementation must not poll or re-check CI because every redispatch consumes
  one of the 12 process visits. Put CI-run evidence in a PR comment and never in
  a commit. Phrase the lint criterion as "no NEW lint
  violations relative to current main, and the gates green on main
  (backend-size, pkg-maint, pkg-file-count, pkg-structure, vet) stay green" —
  NOT as a blanket "make lint passes": `make lint` cannot pass end-to-end on
  main while the pkg-boundary target carries pre-existing
  packaged-service-structure migration debt (recorded 2026-08-08), and an
  unsatisfiable criterion stalls the review loop.
- `userStories` with sequential  ids, title, description,
  acceptanceCriteria, priority, `passes: false`, and empty `notes`
- Ids for storeis should be shaped like {{ (index .Inputs 0).Name }}-001, 002, etc. 
Story-writing rules:
- every story must be small enough for one focused implementation iteration
- every story must include at least one behavioral acceptance criterion
- `Typecheck passes` must appear in every story
- add `Tests pass` when testable logic changes
- add direct browser verification when the story changes visible UI behavior. When a story's acceptance criteria require browser verification, name a fallback explicitly: state that if no dev browser tool (built-in, Playwright, or otherwise) is available in the implementer's environment, the implementer should record that once and move on rather than treat it as unfinished work requiring further attempts. Visible UI changes still require some appropriate verification.
- order stories by dependency so earlier stories do not depend on later ones

Please ensure that the PRD and prd.json both contain an overall description of
the project, the concrete change we want, and the intent.

## Example PRD

```markdown
# PRD: Task Priority System

## Introduction

Add priority levels to tasks so users can focus on what matters most. Tasks can be marked as high, medium, or low priority, with visual indicators and filtering to help users manage their workload effectively.

## Goals

- Allow assigning priority (high/medium/low) to any task
- Provide clear visual differentiation between priority levels
- Enable filtering and sorting by priority
- Default new tasks to medium priority

## User Stories

### <SOME_SHORT_NAME>-001: Add priority field to database
**Description:** As a developer, I need to store task priority so it persists across sessions.

**Acceptance Criteria:**
- [ ] Add priority column to tasks table: 'high' | 'medium' | 'low' (default 'medium')
- [ ] Generate and run migration successfully
- [ ] Typecheck passes

### <SOME_SHORT_NAME>-002: Display priority indicator on task cards
**Description:** As a user, I want to see task priority at a glance so I know what needs attention first.

**Acceptance Criteria:**
- [ ] Each task card shows colored priority badge (red=high, yellow=medium, gray=low)
- [ ] Priority visible without hovering or clicking
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

### <SOME_SHORT_NAME>-003: Add priority selector to task edit
**Description:** As a user, I want to change a task's priority when editing it.

**Acceptance Criteria:**
- [ ] Priority dropdown in task edit modal
- [ ] Shows current priority as selected
- [ ] Saves immediately on selection change
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

### <SOME_SHORT_NAME>-004: Filter tasks by priority
**Description:** As a user, I want to filter the task list to see only high-priority items when I'm focused.

**Acceptance Criteria:**
- [ ] Filter dropdown with options: All | High | Medium | Low
- [ ] Filter persists in URL params
- [ ] Empty state message when no tasks match filter
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

## Functional Requirements

- FR-1: Add `priority` field to tasks table ('high' | 'medium' | 'low', default 'medium')
- FR-2: Display colored priority badge on each task card
- FR-3: Include priority selector in task edit modal
- FR-4: Add priority filter dropdown to task list header
- FR-5: Sort by priority within each status column (high to medium to low)

## Non-Goals

- No priority-based notifications or reminders
- No automatic priority assignment based on due date
- No priority inheritance for subtasks

## Technical Considerations

- Reuse existing badge component with color variants
- Filter state managed via URL search params
- Priority stored in database, not computed

## Success Metrics

- Users can change priority in under 2 clicks
- High-priority tasks immediately visible at top of lists
- No regression in task list performance

## Open Questions

- Should priority affect task ordering within a column?
- Should we add keyboard shortcuts for priority changes?
```

## Output JSON Format

```json
{
  "project": "[Project Name]",
  "branchName": "{{ (index .Inputs 0).Name }}",
  "description": "[Feature description from PRD title/intro]",
  "userStories": [
    {
      "id": "US-001",
      "title": "[Story title]",
      "description": "As a [user], I want [feature] so that [benefit]",
      "acceptanceCriteria": [
        "Criterion 1",
        "Criterion 2",
        "Typecheck passes"
      ],
      "priority": 1,
      "passes": false,
      "notes": ""
    }
  ]
}
```

## step 3
When you are done, respond with exactly: "<COMPLETE>".

# Customer ask 
The customer ask is as follows: 

{{ (index .Inputs 0).Payload }}
