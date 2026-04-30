# Close Live Guard And Instruction Drift Closeout

Date: 2026-04-30
Scope: final verification for `prd.json` `US-005` on branch `ralph/close-live-guard-and-instruction-drift`.

## Summary

This closeout proves the narrow repository-stability cleanup landed without
reintroducing stale maintainer-path assumptions, and that the current branch
state still passes with the already-landed hidden-directory guard behavior it
inherits after the rebase onto `main`.

- The current branch state keeps the broad handwritten-source `pkg/config`
  contract guard on the shared `internal/contractguard.ShouldSkipDir(...)`
  helper, with `pkg/api/generated` still passed explicitly at the guard call
  site.
- The active workstation prompts point only at checked-in guidance that exists
  in this checkout, including the current idea inbox shape and
  `docs/guides/batch-inputs.md`.
- The maintainer development guide now treats the repository root that contains
  `go.mod` and `Makefile` as the canonical execution surface, so no
  compatibility shims for `libraries/agent-factory` were required.

## Validation

Commands run from the repository root:

```powershell
go test ./pkg/api ./pkg/petri ./pkg/interfaces ./pkg/config -count=1
make test
make lint
```

Results on 2026-04-30:

- `go test ./pkg/api ./pkg/petri ./pkg/interfaces ./pkg/config -count=1`
  passed.
- `make test` passed, including the short package suite and
  `tests/functional_test`.
- `make lint` passed, including `go vet`, deadcode baseline validation, and the
  public-surface check.

## What This Proves

- The focused guard bundle still passes with the current shared hidden-directory
  skip behavior already present on this branch after the rebase onto `main`.
- The active prompt and maintainer-doc updates did not require any additional
  repository-path translation layer to stay executable from this checkout root.
- Historical cleanup reports and archival artifacts remained untouched; only
  active operator guidance and focused process inventories changed in this lane.
