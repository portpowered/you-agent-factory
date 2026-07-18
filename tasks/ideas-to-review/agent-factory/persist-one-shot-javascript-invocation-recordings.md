# Persist one-shot JavaScript invocation recordings before session release

## Problem

`you --json run --factory <workflow.js> --record <path> ...` can complete a
JavaScript Factory Session successfully without producing the requested
recording. The one-shot invocation path releases the default Factory Session
and cancels its bootstrap runtime before the normal shutdown recording path can
read and persist that session.

This makes the documented `--record` behavior unreliable for real-CLI
JavaScript invocation tests and for customers who need durable public event
evidence from a successful one-shot run.

## Suggested direction

- Give the one-shot invocation lifecycle an explicit recording flush before
  `CloseFactorySession` removes the session.
- Keep recording ownership in the service/session lifecycle rather than adding
  CLI-side access to runtime internals.
- Add a real-binary regression test that supplies an isolated explicit path,
  completes a JavaScript invocation, and parses the resulting public artifact.
- Preserve the current clean stdout/stderr contract and return a typed failure
  if the requested recording cannot be written.

## Discovery evidence

Observed on 2026-07-18 UTC while implementing
`ftest-javascript-pipeline-real-cli-boundary-001`: the subprocess exited zero
with one `SUCCEEDED` / `FINAL` result, but neither the explicit `--record` path
nor the isolated default recording root contained an artifact after process
exit.
