# Retire ScriptWrap Build Args Shim Closeout

This closeout records the repository-root verification bundle for the
`ralph/retire-scriptwrap-build-args-shim` cleanup lane.

## Scope

This lane is intentionally limited to worker inference provider command
assembly:

- retire the dead `ScriptWrapProvider.buildArgs(...)` forwarding shim
- keep provider CLI argument ownership in
  `pkg/workers/provider_behavior.go`
- keep provider flag assertions at the provider-behavior seam
- keep `Infer(...)` coverage focused on command assembly and prompt transport

The lane does not include unrelated functional-test cleanup, artifact-contract
dedupe, or contract-guard skip-policy work.

## Canonical Verification Bundle

Run these commands from the repository root:

```bash
go test ./pkg/workers -timeout 300s
make test
make lint
```

## Notes

- `go test ./pkg/workers -timeout 300s` is the focused worker verification pass
  for the provider-behavior seam and `Infer(...)` command-assembly regressions.
- `make test` provides the repository-wide short-suite regression bundle after
  the worker cleanup lands.
- `make lint` reruns vet plus the repository-owned dead-code and public-surface
  guards so the narrow cleanup does not leave stale helper seams behind.
- In this Windows worktree, `make test` and `make lint` can emit trailing
  file-lock warnings after successful completion. Treat the command exit status
  and the reported Go package results as the authoritative signal for this
  lane.
