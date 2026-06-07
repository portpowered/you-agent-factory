# Invocation Contract

Factory invocation is a shared CLI and API contract. The entrypoint resolves one
logical input, submits work to one live factory session, then returns one primary
result selected by factory configuration.

## Input Ownership

Input resolution is owned by shared backend logic, not by individual transports.
The API carries text-first input as canonical `WorkContent` on
`InvocationRequest`. The CLI adapts positional text and non-TTY stdin into the
same logical text input. Supplying more than one source for the same logical
slot is a request error with `INVOCATION_INPUT_SOURCE_CONFLICT`.

The OpenAPI source-kind enum documents `fileRef` and `audioStream` as reserved
future categories. Current runtimes accept text only.

## Return Policy Ownership

Factories may declare `invocationReturn` at the factory level. That policy is
session configuration: every invocation of that active factory uses it unless a
future contract explicitly introduces a per-invocation override.

When `invocationReturn` is omitted, the default is
`SUBMITTED_WORK_TERMINAL`. That fallback follows the work item originally
submitted by the invocation until it reaches terminal output, then returns that
work content as `InvocationResponse.primaryResult`.

`EXPLICIT` policy selects content by configured `workTypeName`,
`terminalState`, and optional `workName` within the invocation submit scope.
Validation of whether those names resolve belongs to factory validation, before
runtime invocation.

## CLI and API Equivalence

`you run --factory` and `POST /factory-sessions/{session_id}/invocations` must
use the same input resolver and primary-result selector. Transport code may only
adapt its carrier into the shared resolver input and format the shared response.
It must not invent independent conflict, fallback, or primary-output rules.
