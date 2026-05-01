# Development Guide Relevant Files

This inventory records the checked-in files and directories that the maintainer development guide should describe when it gives repository-root workflow instructions.

| Path | Role | Notes |
| --- | --- | --- |
| `go.mod` | Repository-root marker | Maintainer commands and worktree-aware tests should treat the directory containing `go.mod` as the canonical repository root. |
| `Makefile` | Root command surface | The development guide should describe quality and generation commands as root-level invocations instead of teaching a nested package workflow. |
| `api/` | Authored API contract workspace | OpenAPI validation and bundling start from the repository root, then shell into `api/` only where the documented workflow requires it. |
| `cmd/factory/` | CLI entrypoint | Root-level build and smoke commands compile or execute the `factory` binary from this source tree. |
| `docs/development/*-closeout.md` | Cleanup verification artifacts | Narrow cleanup lanes record the exact root-level validation bundle here when maintainers need durable proof beyond `progress.txt`. |
| `docs/development/development.md` | Active maintainer guide | Must describe the real repository-root layout used in this checkout and avoid stale `libraries/agent-factory` instructions. |
| `factory/` | Maintainer workflow surface | Contains checked-in operator guidance and active inbox directories that the development guide may reference for workflow-related tasks. |
| `pkg/` | Go implementation surface | Package-specific test commands in the guide should reference the real package paths under this root. |
| `tests/` | Smoke and fixture surface | Functional and release-facing checks run from the repository root against these checked-in fixtures. |
| `ui/` | Embedded dashboard workspace | UI build, test, and Storybook commands remain part of the same repository-root workflow. |

## Reusable Rules

- When maintainer docs describe command execution, anchor the instructions to the repository root that contains `go.mod` and `Makefile`.
- If a workflow temporarily changes directories, state that it starts from the repository root and why the subdirectory hop is required.
- When GitHub Actions or other automation is added, prefer repository-owned root commands or package scripts that the maintainer guide already documents instead of inventing CI-only command sequences.
- When a cleanup lane closes with path or contract-alignment work, record the exact root-level verification commands in a `docs/development/*-closeout.md` artifact so the proof survives beyond `progress.txt`.
