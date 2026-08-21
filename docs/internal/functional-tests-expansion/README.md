# Functional test migration ledger

The committed functional-test migration ledger and destination checklist live
here:

- `migration-ledger-inventory.json` is the machine-readable inventory checked
  against the live `tests/functional` tree.
- `test-file-checklist.md` is the destination-cell inventory used to validate
  ledger routing and packaged-Factory invocation coverage.

`migrationledgercheck` resolves its default paths from the repository root.
The ignored `docs/temp/functional-tests-expansion/` planner mirror is not a
fallback; the committed pair above is the canonical evidence.
