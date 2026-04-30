# Checked-In Root Workflow Starter

This repository ships a checked-in multi-stage starter under `./factory/`.
It is not the default `agent-factory` or `agent-factory init` scaffold.

## Workflow

This checked-in starter models a neutral thought-to-plan-to-task loop:

- `thoughts:init` -> `ideafy` -> `thoughts:complete`
- `idea:init` -> `plan` -> `idea:complete` + `plan:init`
- `plan:init` -> `setup-workspace` -> `plan:complete` + `task:init`
- `task:init` -> `process` -> `task:in-review`
- `task:in-review` -> `review` -> `task:complete`

Loop-breaker workstations protect the checked-in review path with visit-count guards.

## Structure

- `factory/factory.json` defines the checked-in root workflow.
- `factory/workers/processor/AGENTS.md` drives the model-backed idea, plan, process, and review stages.
- `factory/workers/workspace-setup/AGENTS.md` prepares a workspace for plan outputs before task execution begins.
- `factory/workstations/` contains the prompt templates for `ideafy`, `plan`, `process`, `review`, and `cleaner`.

## Usage

The checked-in starter is a repository-owned example surface. It is richer than the default `agent-factory init` scaffold and is intended for end-to-end repository workflow exercises.
