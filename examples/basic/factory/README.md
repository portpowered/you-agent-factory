# Example: Basic Task Workflow

This example models a minimal task-processing workflow as an agent factory configuration.

## Workflow

```
task:init → [process] → task:complete
                |
                └── onFailure → task:failed
```

1. A task enters `task:init`.
2. The `process` workstation handles it with the `processor` worker.
3. Successful tasks move to `task:complete`.
4. Failed tasks move to `task:failed`.

## Directory Structure

```
examples/basic/factory/
├── factory.json                    # Workflow: process task to completion
├── workers/
│   ├── README.md
│   └── processor/AGENTS.md         # MODEL_WORKER: processes tasks
├── workstations/
│   ├── README.md
│   └── process/AGENTS.md           # Prompt template for processing work
├── inputs/
│   └── task/default/               # Drop task files here
│       └── factory-bug-init.md     # Example input
└── README.md                       # This file
```

## Running

```bash
you run -dir examples/basic/factory
```

Or submit work programmatically:

```bash
cp my-task.md examples/basic/factory/inputs/task/default/
you run -dir examples/basic/factory
```
