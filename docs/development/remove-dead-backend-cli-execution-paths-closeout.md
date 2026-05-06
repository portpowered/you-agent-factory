# Remove Dead Backend And CLI Execution Paths Closeout

This closeout records the verification bundle for `US-002` on
`ralph/remove-deadcode-2026-may`.

## Scope

- keep provider-specific CLI argument ownership in
  `pkg/workers/provider_behavior.go`
- keep `GET /work` pagination ownership in `pkg/api/handlers.go:ListWork`
  plus the generated `ListWorkParams` and `PaginationContext` contract
- remove the extra exported verbose logging wrapper so command-runner verbose
  records flow through the explicit `logging.Logger` contract owner
- refresh the accepted deadcode baseline entries that were stale for replay
  event-stream helpers so the repository quality gate matches current live
  reachability

## Canonical Owners

- Provider CLI args: `pkg/workers/provider_behavior.go`
- Public work pagination: `pkg/api/handlers.go:ListWork`
- Verbose runtime diagnostics: `pkg/logging/logger.go` and
  `pkg/logging/runtime_logger.go`

## Canonical Verification Bundle

Run these commands from the repository root:

```bash
go test ./...
make typecheck
make lint
make test
```

## Notes

- The provider build-args forwarding shim was already absent in the branch
  state for this iteration, so no additional worker command-owner code removal
  was required beyond keeping the provider-behavior seam covered.
- The list-work legacy pagination shim was already collapsed into the generated
  route parameter path before this iteration; the observable route coverage
  remains in `pkg/api/server_test.go`.
- `make lint` for this lane required removing stale accepted deadcode entries
  for replay event-stream helpers because the current analyzer no longer
  reports them as unreachable.
