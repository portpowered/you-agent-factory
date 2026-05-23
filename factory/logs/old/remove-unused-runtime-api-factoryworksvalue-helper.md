# Cleanup Idea: Remove Unused Runtime API `factoryWorksValue` Helper

## Why

At `HEAD` `06d4ad31`, `tests/functional/runtime_api/runtime_support_test.go`
still defines a local `factoryWorksValue(...)` helper with no call sites.

The orphan is already acknowledged in
`docs/internal/development/deadcode-baseline.txt`, which means the repository
is knowingly carrying dead test support code that can now be deleted cleanly.

This is the best current cleanup seam because it is:

- dead code first
- narrow
- implementation-ready
- non-overlapping with the active runtime-log, classifier, and model-resource
  lanes

## What To Change

Keep the cleanup constrained to the unreachable helper and its baseline entry.

1. Delete `factoryWorksValue(...)` from
   `tests/functional/runtime_api/runtime_support_test.go`.
2. Remove the matching unreachable-function entry from
   `docs/internal/development/deadcode-baseline.txt`.
3. Leave the separate `functionallong` helper owners in
   `tests/functional/internal/support` untouched.

## Constraints

- Do not broaden the change into the other deadcode-baseline findings.
- Do not change production behavior.
- Do not rewrite the runtime API functional assertions.
- Prefer direct deletion over introducing a replacement wrapper.

## Acceptance Criteria

- `tests/functional/runtime_api/runtime_support_test.go` no longer defines
  `factoryWorksValue(...)`.
- `docs/internal/development/deadcode-baseline.txt` no longer lists
  `tests/functional/runtime_api/runtime_support_test.go:68:6: unreachable func: factoryWorksValue`.
- Relevant runtime API and deadcode verification no longer report that helper.

## Verification

- `go test ./tests/functional/runtime_api/...`
- `go run golang.org/x/tools/cmd/deadcode@v0.25.1 -test ./...`
