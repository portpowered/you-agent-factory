# CLI hosted invocation does not project terminal work to `/work`

## Observation

Functional investigation for `visibility_test.go` showed:

- `startInvocationServer` + API `postInvocation` populates terminal work on `GET /factory-sessions/~default/work` (`assertTerminalWorkPrimaryText` passes).
- `you run --factory --with-server invoke` via `Process.Execute` returns a successful `InvocationResponse` on stdout and exposes live session identity over HTTP, but `/work` remains empty for the hosted one-shot host even while the server is accepting requests.

## Impact

Stories that require **the same CLI-started run** to be visible through API **work** reads cannot rely on `assertTerminalWorkPrimaryText` on the hosted invocation host today. Tests either accept CLI outcome fields as the work-adjacent public facts, or split CLI and service-mode API paths (which no longer proves same-run visibility).

## Possible directions

- Project submitted-work terminal tokens to `/work` for hosted service-mode invocations the same way continuous service-mode API invocations do.
- Add a remote CLI invocation path (`you --server URL run ... invoke`) that delegates to `POST /factory-sessions/{id}/invocations` on an already-running host.
