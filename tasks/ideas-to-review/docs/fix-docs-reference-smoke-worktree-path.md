## Problem

`make docs-reference-smoke` fails from nested worktrees because the target shells into `docs/` and runs `go run ../markdown-linter/cmd/markdown-linter ...`, but this repository does not contain that relative path. In a `.claude/worktrees/...` checkout the command exits with:

`directory ../markdown-linter/cmd/markdown-linter outside main module or its selected dependencies`

That makes the documented docs verification lane unusable for any branch that edits `docs/reference`.

## Why It Matters

- This is a repeated failure mode for future CLI/docs stories, not a one-off content issue.
- It blocks a documented quality gate even when the markdown change itself is valid.
- It is easy to miss in review because the failure is in the workflow wiring, not the edited doc.

## Suggested Direction

- Make `docs-reference-smoke` invoke the markdown linter through a module-relative path that exists in the repo.
- Add one small smoke test or CI assertion for the target so future path drift is caught before contributors hit it in worktrees.
