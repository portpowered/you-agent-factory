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
