# Packaged goal one-shot invocation lacks API work/event correlation

Packaged `@you/goal` session invocations (CLI `you run --with-server` and API
`POST /factory-sessions/~default/invocations`) complete with a terminal
`InvocationResponse`, but often do not leave inspectable artifacts on public
read surfaces:

- `GET /factory-sessions/~default/work` stays empty after success.
- Default-session factory event streams stop at prelude events (`RUN_REQUEST`,
  `INITIAL_STRUCTURE_REQUEST`, `SESSION_STARTED`, `FACTORY_STATE_RESPONSE`) without
  `requestId` / `traceIds` / `workIds` on context.
- `GET /factory-sessions/~default/work/{id}` for derived batch work ids returns
  not found after successful completion.

Cross-surface inspectability tests currently rely on weaker live-session signals
(`RUNNING`, uncorrelated `SessionStarted`) unless product starts emitting
goal-complete work listing and/or identity-correlated factory events for
invocation runs.
