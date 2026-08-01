# Functional test migration ledger

The committed functional-test migration ledger and its destination checklist
live beside this routing note:

- `migration-ledger-inventory.json` is the machine-readable inventory checked
  against the live `tests/functional` tree.
- `test-file-checklist.md` is the destination-cell inventory used to validate
  ledger routing and packaged-Factory invocation coverage.

These files are intentionally under `docs/internal/`, not `docs/temp/`, so
`migrationledgercheck` validates the committed repository state in every
worktree and CI checkout.

## Canonical checker routing

`migrationledgercheck` resolves its default paths from the repository root:

- ledger: `docs/internal/functional-tests-expansion/migration-ledger-inventory.json`
- destination checklist: `docs/internal/functional-tests-expansion/test-file-checklist.md`

The checker also accepts absolute paths without prefixing them with the
repository root. The ignored `docs/temp/functional-tests-expansion/` planner
mirror is not a fallback; with the default routing, the live `tests/functional`
tree is scanned against the committed pair above.

Run `go run ./cmd/migrationledgercheck` from the repository root to validate
that committed pair against the live tree and packaged-Factory invocation
matrix.
