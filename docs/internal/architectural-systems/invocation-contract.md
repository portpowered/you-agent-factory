# Invocation Contract

Factory invocation is a shared CLI and API contract. The entrypoint resolves one
logical input, submits work to one live factory session, then returns one primary
result selected by factory configuration.

## Input Ownership

Input resolution is owned by shared backend logic, not by individual transports.
The API carries legacy text-first input as canonical `WorkContent` on
`InvocationRequest.content`, and it may also carry structured
`InvocationRequest.args` for factories with `invocationSignature`. The CLI
adapts positional text, named signature arguments, and non-TTY stdin into the
same shared resolver. Supplying more than one source for the same logical slot
is a request error with `INVOCATION_INPUT_SOURCE_CONFLICT` or the corresponding
structured-argument conflict code.

The OpenAPI source-kind enum documents `fileRef` and `audioStream` as reserved
future categories. Current compatibility content still accepts text only.

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
adapt positional text, stdin, API content, or API args into the shared resolver
input and format the shared response. It must not invent independent conflict,
fallback, or primary-output rules.

Pure resolver and selection policy lives in `pkg/services/work/invocation`; generated
OpenAPI and CLI values are mapped at their transport boundaries, while live
submission and waiting stay outside the Work package.

## Invocation output and observation

Supported one-shot invocations expose three stdout modes on the CLI and a
session-scoped SSE counterpart on the API:

| Mode | How to select | What stdout or SSE carries |
| --- | --- | --- |
| Primary result (default) | Default `you run --factory` or `--named` | Only the configured `primaryResult` on success |
| Human response-stream | `--output response-stream` | Human-readable progress, then the same primary result |
| NDJSON automation | Global `--json` with `--output response-stream` | One JSON record per non-empty stdout line; streamed events use `recordType=response_event` with a nested public `FactoryResponseEvent`; an available invocation ends with exactly one terminal `recordType=invocation_result` record |

The session API exposes the same public `FactoryResponseEvent` contract on
`GET /factory-sessions/{session_id}/response-events`. That route is ephemeral
observation separate from canonical `GET /factory-sessions/{session_id}/events`
Factory event replay. Reconnect with `after_sequence`; stale retained cursors
begin with `STREAM_GAP` instead of silently skipping loss. Response-event
history is bounded and session-scoped — it does not promise durable
process-restart replay beyond the retention window.

Provider streaming fidelity varies: native-streaming providers may emit
incremental public response events; final-only providers may emit only terminal
semantic snapshots. Authoritative invocation success remains `primaryResult` and
canonical Factory event facts even when intermediate observation is sparse.

For copyable examples, cursor semantics, typed HTTP outcomes, and provider
fidelity guidance, use `you docs run`, `you docs sessions`, and `you docs
workers`.
