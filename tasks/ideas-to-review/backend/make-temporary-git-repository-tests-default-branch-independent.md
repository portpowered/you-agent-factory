# Make temporary Git repository tests default-branch independent

## Problem

Tests that create temporary Git repositories can inherit the host's
`init.defaultBranch`. At least two `internal/contractstaging` tests initialize a
repository and later hard-code `git checkout master`. They fail on hosts whose
system Git configuration initializes repositories on `main`, blocking
`make test` and `make verify-fast` even though the product change does not touch
contract staging.

Observed failures:

- `TestMergeCommitTipResolvesSourceCommitWithoutFalseShallowFailure`
- `TestMergeCommitInRevListWithoutPathChangesResolvesSourceCommit`

## Proposed direction

Make the shared temporary-repository setup choose and return an explicit branch
name, or initialize with an explicit branch and have tests use that value. Keep
the fixture hermetic rather than overriding a contributor's global or system
Git configuration.

Audit nearby temporary Git fixtures for the same assumption while keeping the
change limited to test infrastructure; do not rename the repository's real base
branch as part of this work.

## Acceptance evidence

- The affected contract-staging tests pass with `init.defaultBranch=main` and
  with no configured default branch.
- `go test ./internal/contractstaging -count=1` passes.
- `make test` no longer fails because a temporary repository lacks `master`.
