# Example: Simple Story Review Workflow

This example models a simple story execution and review loop as an agent factory configuration.

## Workflow

```
story:init → [execute-story] → story:in-review → [review-story] → story:complete
                  ↑                                     |
                  └──────── onRejection ────────────────┘
                                                        |
                                            (max 3 visits) → story:failed
```

1. A story enters `story:init`.
2. The `execute-story` workstation implements it (runs quality checks, commits).
3. The `review-story` workstation reviews against acceptance criteria.
4. If rejected, the story returns to `init` for rework (with rejection feedback).
5. After 3 review visits, the guarded `LOGICAL_MOVE` loop breaker routes the story to `failed`.

## Directory Structure

```
examples/simple-tasks/
├── factory.json                    # Workflow: execute → review loop with guarded loop breaker
├── workers/
│   ├── executor/AGENTS.md          # MODEL_WORKER: implements stories
│   └── reviewer/AGENTS.md         # MODEL_WORKER: reviews against criteria
├── workstations/
│   ├── execute-story/AGENTS.md    # Prompt template with rejection feedback
│   └── review-story/AGENTS.md    # Prompt template for code review
├── inputs/
│   └── story/default/             # Drop story files here
│       └── example-story.md       # Example input
├── GAPS.md                         # Known gaps vs current ralph.sh workflow
└── README.md                       # This file
```

## Running

```bash
you run -dir examples/simple-tasks
```

Or submit work programmatically:

```bash
cp my-story.md examples/simple-tasks/inputs/story/default/
you run -dir examples/simple-tasks
```
