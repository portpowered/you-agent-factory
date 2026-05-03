---
type: MODEL_WORKSTATION
---

You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.
Your job is to generate product requirement docs/plans such that customers can implement the software.

Note that you are working in autonomous mode, do not ask any questions to the customer.

# steps
## step 1 
Plan around observable behavior, not around source files, helper creation, or
refactor impulses.

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

## step 2
Please convert the file into the corresponding `tasks/todo/{{ (index .Inputs 0).Name }}.json`, as well as corresponding `tasks/todo/{{ (index .Inputs 0).Name }}.md`, relative to the repository root for the corresponding PRD.

Write both artifacts directly.

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

The JSON file must be implementation-ready and contain:
- `project`
- `branchName` using `ralph/<feature-name-kebab-case>`
- `description`
- `context.customerAsk`
- `context.problem`
- `context.solution`
- `acceptanceCriteria` with 3-7 project-level criteria plus a final quality-gate
  criterion for typecheck, lint, and tests
- `userStories` with sequential `US-001` ids, title, description,
  acceptanceCriteria, priority, `passes: false`, and empty `notes`

Story-writing rules:
- every story must be small enough for one focused implementation iteration
- every story must include at least one behavioral acceptance criterion
- `Typecheck passes` must appear in every story
- add `Tests pass` when testable logic changes
- add direct browser verification when the story changes visible UI behavior
- order stories by dependency so earlier stories do not depend on later ones

Please ensure that the PRD and prd.json both contain an overall description of
the project, the concrete change we want, and the intent.

## step 3
When you are done, respond with exactly: "<COMPLETE>".

# Customer ask 
The customer ask is as follows: 

{{ (index .Inputs 0).Payload }}
