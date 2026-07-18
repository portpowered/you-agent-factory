# Isolate durable runtime state for `RootRunFunctionalHost`

## Problem

`RootRunFunctionalHost` supplies an isolated `SystemRoot`, but a real runtime
submission still resolves durable-session persistence from the Go test
package's working directory. A root-run functional test that submits work can
therefore create `tests/functional/<package>/.you-agent-factory/durable-sessions`
outside its temporary fixture roots.

## Why review this

The root-run host is the intended replacement for legacy functional API
composition. Every migrated runtime scenario can reproduce this artifact,
which weakens isolated-test cleanup and risks cross-test persistence leakage.

## Suggested direction

Add an explicit, process-input-safe project-root selection to the root-run
functional host (or the corresponding runtime construction path) so durable
session persistence resolves under a caller-owned temporary root without
changing the process-global working directory. Cover it with a root-run host
test that submits work and proves no `.you-agent-factory` directory appears in
the test package working directory.
