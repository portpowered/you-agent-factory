issues wit hrunning

## failing request
- command with bad request parameters shoudl be marked just as bad request

error, bad request provided, unknown argument rather than INVOCATION_ARGUMENT_UNKNOWN_ARGUMENT/INTERNAL_SERVER_ERROR.

Bad request not ISE.

## executing work
- we should print out some type of spinner for every active worker that matches up to the active worker stream printed to stderr when in non json mode.

⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏

⠋ ⠙ ⠹ - means three workers, matching to their corresponding colors

## work items
when the work generated consumed says 10 items, we shoudl print out the work ids and tasks that each work item is implementing

work accepted: 10 items
- work-task-1: blah blah
- work-task-2: blah blah
- work-task-3: blah blah

- work-task-2 - depends on -> work-task-3

## owkrstation started/completed:
- the workstation started/completed items shoudl print out the work items completed/consumed.

like
workstation completed: plan-parallel-work (item-1, item-2)

# TODO

- run the tests to validate whether parameterless runs will default properly for the right work

you run -a "@you/goal" --to "implement something", works with the default config that directs to codex/whatever else, test it with both codex, and cursor small models.

# COLORS
we should not use the same set of colors all the time, the colors look flat. we should have more variance. right now its all shades of blue/purple yellow for some reason.


# COLORS implementation

# Delete cursor, and rename cursor ACP as the only provider

we don't want to implement a bespoke cursor thing when ACP will work, switch over rather than rely on PTY.

# implement a worktree flag
the worktree flag should make the run execute within a worktree

you run -a "@you/goal" --to "fix the bug, then merge the PR to github using the gh CLI" --provider codex --worktree "my-worktree"

# bug with execution output
```
PS C:\Users\andre\work\portos\infinite-you> ./bin/you run -a "@you/plan-parallel" --planner-provider codex --planer-model gpt-5.6-sol --executor-provider cursor-acp --execute-model composer-2.5 --merge-provider codex --merge-model gpt-5.6-sol --to "implement the backend service restructure denoted in docs/internal/packaged-service-structure/package-service-workers.md"
{"code":"INVOCATION_ARGUMENT_UNKNOWN_ARGUMENT","family":"INTERNAL_SERVER_ERROR","message":"unknown named argument \"planer-model\" (factory \"@you/plan-parallel\")"}
PS C:\Users\andre\work\portos\infinite-you> ./bin/you run -a "@you/plan-parallel" --planner-provider codex --planner-model gpt-5.6-sol --executor-provider cursor-acp --execute-model composer-2.5 --merge-provider codex --merge-model gpt-5.6-sol --to "implement the backend service restructure denoted in docs/internal/packaged-service-structure/package-service-workers.md"
{"code":"INVOCATION_ARGUMENT_UNKNOWN_ARGUMENT","family":"INTERNAL_SERVER_ERROR","message":"unknown named argument \"execute-model\" (factory \"@you/plan-parallel\")"}
```
should be BAD_REQUESTS, NOT_ISES

# durabl sessions
cd .\.you-agent-factory\durable-sessions\

we should not put this here, we should put this at the global directory, and for now we should just not record them as we don't thin kwe need them quite yet.

##  bug with plan parallel output

```
PS C:\Users\andre\work\portos\infinite-you> ./bin/you run -a "@you/plan-parallel" --planner-provider codex --planner-model gpt-5.6-sol --executor-provider cursor-acp --executor-model composer-2.5 --merge-provider codex --merge-model gpt-5.6-sol --to "implement the backend service restructure denoted in docs/internal/packaged-service-structure/package-service-workers.md"
[0] factory started
[4] work accepted: implement the backend service restructure denoted in docs/internal/packaged-service-structure/package-service-workers.md
[5] workstation started: plan-parallel-work
[9] work accepted: 10 items
[35] workstation completed: plan-parallel-work
[36] workstation started: execute-planned-task
[37] workstation started: execute-planned-task
[42] workstation completed: execute-planned-task
[43] workstation started: execute-planned-task
[47] workstation completed: execute-planned-task
[48] workstation started: execute-planned-task
[52] workstation completed: execute-planned-task
[55] workstation completed: execute-planned-task
[56] workstation started: execute-planned-task
[57] workstation started: execute-planned-task
[62] workstation completed: execute-planned-task
[63] workstation started: execute-planned-task
[67] workstation completed: execute-planned-task
[70] workstation completed: execute-planned-task
[71] workstation started: execute-planned-task
[75] workstation completed: execute-planned-task
[76] workstation started: execute-planned-task
[80] workstation completed: execute-planned-task
[81] workstation started: execute-planned-task
[85] workstation completed: execute-planned-task
[86] workstation started: merge-plan-results
[90] workstation completed: merge-plan-results

--- primary result ---
No completed generated task results are available in this thread—only the root agent exists, with no delegated outputs. I can’t produce a faithful merged response without inventing content. Please provide or attach the task results to merge.

```

why did the primary result output like that? so strange, need to fix.
