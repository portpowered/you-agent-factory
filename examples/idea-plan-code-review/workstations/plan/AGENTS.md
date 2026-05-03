---
type: MODEL_WORKSTATION
limits:
  maxExecutionTime: 30m
---


You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.

The customer is asking you to convert the following ask into a prd using the /prd and /ralph skills.

Before planning, read and follow:

- `docs/standards/code/planning-standards.md`
- `docs/standards/code/code-review-standards.md`
- `docs/standards/code/general-backend-standards.md` when the ask touches backend, contracts, runtime, CLI, or tests
- `docs/standards/code/general-website-standards.md` when the ask touches UI, browser behavior, frontend state, styling, accessibility, or frontend tests

Your planning output **MUST** follow these rules:

- Each user story should correspond to roughly one observable behavior or one tightly bounded enabling step.
- Prefer vertically sliced stories that deliver a coherent behavior across layers instead of splitting one behavior into backend-only, frontend-only, and tests-only stories.
- Acceptance criteria must be reviewer-verifiable outcomes, not internal implementation chores such as moving files or adding helpers.
- Include the right quality gates and verification expectations for the touched surfaces, and keep them in addition to behavior criteria rather than instead of behavior criteria.
- Keep scope narrow; do not bundle unrelated cleanup, broad refactors, or inventory work unless the ask explicitly requires it.
- Make the resulting `prd.json` implementation-ready without depending on hidden chat context.

Please convert the file into the corresponding tasks/todo/{{ (index .Inputs 0).Name }}.json.

Note that you are working in autonomous mode, do not ask any questions to the customer.

When you are done, respond with exactly: "<COMPLETE>".

The customer ask is as follows: 

{{ (index .Inputs 0).Payload }}
