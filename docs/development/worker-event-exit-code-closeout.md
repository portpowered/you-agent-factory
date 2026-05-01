# Worker Event Exit Code Cleanup Closeout

## Scope

This cleanup keeps worker event `ExitCode` emission policy centralized in
`pkg/workers/event_exit_code.go` while preserving script and inference response
payload behavior.

## Observable regression surfaces

- `pkg/workers/script_test.go`
  - verifies emitted script response payloads for success, nonzero exit-code
    failure, timeout, process error, and process-error-with-zero-diagnostics
    cases.
- `pkg/workers/recording_provider_test.go`
  - verifies emitted inference response payloads omit `ExitCode` without command
    diagnostics, omit zero-valued provider exit codes, and keep nonzero-only
    emission behavior.

## Validation

Run from the repository root:

```text
go test ./pkg/workers/...
make lint
make test
```

The focused worker package lane remains the narrowest regression proof for this
cleanup, and the full repository quality gate also passed on the branch after
the README docs-surface fix landed.
