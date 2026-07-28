# Packaged goal one-shot invocation lacks API work/event correlation

Packaged `@you/goal` session invocations (CLI `you run --with-server` and API
`POST /factory-sessions/~default/invocations`) complete with a terminal
`InvocationResponse`, but often do not leave inspectable artifacts on public
read surfaces:

- `GET /factory-sessions/~default/work` stays empty after success.
- Default-session factory event streams stop at prelude events (`RUN_REQUEST`,
  `INITIAL_STRUCTURE_REQUEST`, `SESSION_STARTED`, `FACTORY_STATE_RESPONSE`) without
  `requestId` / `traceIds` / `workIds` on context.
- `PUT /factory-sessions/~default/work-requests/{requestId}` after a successful
  invocation returns `201 Created` (not idempotent replay), proving the invocation
  request id was not admitted through the same observable work-request path as
  `POST /work`.
- `POST /work` on the same live server **does** emit `WORK_REQUEST` with
  `context.requestId` and leaves listed goal work; invocation does not.

Cross-surface inspectability tests currently rely on weaker live-session signals
(`RUNNING` via `sawFactoryActive`) unless product aligns invocation submission
with the public work-request/event surfaces used by `POST /work`.
