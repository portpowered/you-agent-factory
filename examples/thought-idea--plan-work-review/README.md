# Example: Thought to Reviewed Work Workflow

This example models a generic flow that turns thoughts into ideas, plans, implementation tasks, and review.

## Workflow

```
thoughts:init → [ideafy] → idea:complete
idea:init → [plan] → idea:complete + plan:init
plan:init → [setup-workspace] → plan:complete + task:init
task:init → [process] → task:in-review → [review] → task:complete
                 ↑                         |
                 └────── onRejection ──────┘
```

1. Thoughts become ideas.
2. Ideas become plans.
3. Plans create implementation tasks.
4. Tasks move through processing and review until complete or failed.
5. Guarded `LOGICAL_MOVE` loop breakers route over-limit task retries or review loops to `task:failed`; the review loop breaker consumes `task:init` after `review` rejects work back there.

## Directory Structure

```
examples/thought-idea--plan-work-review/
├── factory.json                    # Workflow: thought → idea → plan → task → review with guarded loop breakers
├── workers/
│   ├── README.md
│   ├── processor/AGENTS.md         # MODEL_WORKER: handles planning and task work
│   └── workspace-setup/AGENTS.md   # MODEL_WORKER: prepares workspaces
├── workstations/
│   ├── README.md
│   ├── ideafy/AGENTS.md            # Prompt template for idea generation
│   ├── plan/AGENTS.md              # Prompt template for plan generation
│   ├── setup-workspace/AGENTS.md   # Workstation definition for workspace preparation
│   ├── process/AGENTS.md           # Prompt template for implementation
│   └── review/AGENTS.md            # Prompt template for review
├── inputs/
│   └── task/default/               # Sample task payloads
└── README.md                       # This file
```

## Running

```bash
you run -dir examples/thought-idea--plan-work-review
```

Or submit work programmatically:

```bash
cp my-task.md examples/thought-idea--plan-work-review/inputs/task/default/
you run -dir examples/thought-idea--plan-work-review
```

## Retained Sample Input

Some files under `inputs/task/default/` are retained as historical sample task payloads. Those payloads may describe product-specific scope, but they are input data only and are not required factory defaults.
