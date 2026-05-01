# Example: Dispatcher Parity

This example models the `scripts/agents/dispatcher.ps1` workflow as an agent factory configuration, demonstrating structured inputs, worktree execution, parameterized fields, and repeater workstations working together.

## Workflow

```
                          FACTORY_REQUEST_BATCH input (PRD)
                                    |
                         [batch token expansion]
                                    |
                     one token per story in story:init
                                    |
                                    v
story:init → [execute-story (repeater)] → story:in-review → [review-story] → story:complete
     ^               |                                            |
     |     (re-fires until                                        |
     |      ACCEPTED)                                             |
     |                                                            |
     └──────────────────── onRejection ───────────────────────────┘
                                    |
                     (max 3 review visits at story:init)
                                    v
                              [review-loop-breaker]
                                    |
                                    v
                               story:failed
```

### Dispatcher Phase Mapping

| dispatcher.ps1 Phase | Factory Equivalent |
|---|---|
| Phase 1: Executor dispatch | `execute-story` workstation (repeater kind) |
| Phase 2: Reviewer dispatch | `review-story` workstation (standard kind) |
| Phase 3: Job tracking & metrics | Engine session metrics + `RecordRepeat` |
| Max iterations per executor | Guarded `LOGICAL_MOVE` loop breaker watching `execute-story` (`maxVisits: 50`) |
| Max review rejections | Guarded `LOGICAL_MOVE` loop breaker watching `review-story` (`maxVisits: 3`) |
| `--MaxExecutors` concurrency | `agent-slot` resource (`capacity: 5`) |
| PRD → stories splitting | `FACTORY_REQUEST_BATCH` work request with batch submission |
| Worktree isolation | `worktree` field on `execute-story` with parameterized path |
| Branch from PRD | Tags flow from input → token → template resolution |

### What's Different

The dispatcher.ps1 also handles **ideation** (generating new ideas) and **planning** (converting ideas to PRDs). These are not modeled here because they are upstream of execution — a separate factory could handle the ideation-to-PRD pipeline and feed its output into this factory's `FACTORY_REQUEST_BATCH` request.

## Directory Structure

```
dispatcher-parity/
├── factory.json                        # Workflow with structured input, repeater, worktrees
├── workers/
│   ├── executor/AGENTS.md              # MODEL_WORKER: implements stories iteratively
│   └── reviewer/AGENTS.md              # MODEL_WORKER: reviews against acceptance criteria
├── workstations/
│   ├── execute-story/AGENTS.md         # Repeater prompt with worktree context
│   └── review-story/AGENTS.md          # Standard review prompt
├── inputs/
│   └── sample-prd.json                 # Example FACTORY_REQUEST_BATCH input
└── README.md                           # This file
```

## Key Features Demonstrated

### 1. Structured Inputs (`FACTORY_REQUEST_BATCH`)

When a `FACTORY_REQUEST_BATCH` PRD request is submitted (see `inputs/sample-prd.json`), the engine:

1. Validates the JSON against the `FACTORY_REQUEST_BATCH` schema
2. Creates one token per entry in the `work[]` array
3. Preserves `DEPENDS_ON` relations so stories dispatch in order
4. Attaches `tags` to every token for downstream parameterization

### 2. Repeater Workstation (`execute-story`)

The executor uses `"behavior": "REPEATER"` so it re-fires on every non-terminal (REJECTED) result. The agent keeps iterating until quality checks pass and it outputs `<result>ACCEPTED</result>`. The `executor-loop-breaker` guarded `LOGICAL_MOVE` workstation caps iterations at 50.

### 3. Guarded Loop Breakers

The example uses explicit guarded `LOGICAL_MOVE` workstations for both loop
breakers. Each loop breaker declares the source state, target state, watched
workstation, and inclusive visit threshold in normal workstation topology. The
review loop breaker watches `review-story` but consumes `story:init`, because
rejected review work returns there before the over-limit route fires.

### 4. Parameterized Worktree Execution

The `execute-story` workstation configures:
- `"worktree": ".worktrees/{{ index (index .Inputs 0).Tags "branch" }}/{{ (index .Inputs 0).WorkID }}"` — each story gets an isolated git worktree
- `"workingDirectory": "{{ index (index .Inputs 0).Tags "worktree" }}"` — the worker runs in the worktree
- `"env"` — environment variables resolved from tags at dispatch time

### 5. Dependency Ordering

The sample input defines `US-002 DEPENDS_ON US-001` and `US-003 DEPENDS_ON US-002`. The engine's `DependencyGuard` holds back dependent tokens until their prerequisites reach a terminal state.

## Running

```bash
# Submit a FACTORY_REQUEST_BATCH input
agent-factory run -dir examples/dispatcher-parity -input examples/dispatcher-parity/inputs/sample-prd.json -input-type prd
```

Or programmatically via the Go API:

```go
engine.SubmitWork(factory.SubmitRequest{
    WorkType:  "story",
    InputType: "prd",
    Payload:   string(prdJSON),
})
```

## Concurrency

The `agent-slot` resource has `capacity: 5`, allowing up to 5 stories to execute in parallel (matching dispatcher.ps1's `--MaxExecutors` default). Stories without dependency constraints dispatch concurrently; dependent stories wait for their predecessors.
