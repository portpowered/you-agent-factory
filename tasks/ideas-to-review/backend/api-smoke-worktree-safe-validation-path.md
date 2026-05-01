# Make `api-smoke` Work From Nested Worktrees

## Problem

`make api-smoke` currently starts with:

```make
cd ../../api && node run-quiet-api-command.js validate:main ../libraries/agent-factory/api/openapi-main.yaml
```

That relative hop assumes a package layout outside this repository root. In
this worktree (`.claude/worktrees/api-clean`), the command resolves to a
non-existent `../../api` directory, so the canonical API verification target
fails before it reaches the real bundle and smoke checks.

## Why It Matters

- OpenAPI cleanup stories are supposed to rely on `make api-smoke` as the
  canonical proof path.
- A broken entry command forces maintainers to reassemble the equivalent
  validation steps manually, which is error-prone and easy to drift.
- This affects any future work done from nested worktrees, not just this
  schema-standardization lane.

## Suggested Direction

- Rewrite `api-smoke` to execute from the repository root, matching
  `bundle-api` and `generate-api`.
- Reuse `node scripts/run-quiet-api-command.js validate:main ./api/openapi-main.yaml`
  instead of shelling into an external `../../api` directory.
- Keep the rest of the target unchanged so the second regenerate pass and
  focused OpenAPI/runtime smoke tests still define the canonical lane.
