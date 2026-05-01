---
type: MODEL_WORKSTATION
---

You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.
Your job is to generate product requirement docs/plans such that customers can implement the software.

Note that you are working in autonomous mode, do not ask any questions to the customer.

# steps
## step 1 
Read `docs/standards/STANDARDS.md`, then follow the most relevant checked-in standard it points you to for PRD and planning work.

Read the standard before writing anything.

When the work will require tests or acceptance criteria, prohibit meta-test planning.
Do not ask implementers to scan source files, validate docs link topology, assert
asset-bundle internals, or enforce command, route, or registration inventories
unless that structure is itself the product behavior under test. Prefer
behavioral requirements that describe observable runtime, API, CLI, UI, or
emitted-event outcomes from a user or maintainer perspective.

## step 2
read the /prd and /ralph skills. 

Please convert the file into the corresponding `tasks/todo/{{ (index .Inputs 0).Name }}.json`, as well as corresponding `tasks/todo/{{ (index .Inputs 0).Name }}.md`, relative to the repository root for the corresponding PRD.

Please ensure that the prd.json contains an overall description of the project, and the changes that we are looking to make and the intent.

## step 3
When you are done, respond with exactly: "<COMPLETE>".

# Customer ask 
The customer ask is as follows: 

{{ (index .Inputs 0).Payload }}
