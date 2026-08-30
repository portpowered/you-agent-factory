# Validation report: Worker Sessions route on long-running Factories

## Environment and artifact

- Commit/build identifier: source commit `6300480d5c4e3e5c2ba4aa02b3f4547987a0af1a`; built with `go build -buildvcs=false -o <temp>/you.exe ./cmd/factory`; binary SHA-256 `D5FB53A9F156622F8FA587D15C1F1AD3FF704C47025E79395560B2B5D6BF1A2C`.
- Environment and configuration: Windows `10.0.26200`, `go1.25.0`, `windows/amd64`; isolated `HOME`, `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, and XDG directories under a unique temporary root; default Factory Session `~default`; loopback listener `127.0.0.1:21939`; no recording was enabled.
- Customer entry point: the built `you.exe` started a live Factory with `run --dir <temp>/factory --continuously --with-server --listen 127.0.0.1:<port> --no-record --quiet`, followed by the public Worker Sessions CLI and REST requests below.
- Real and substituted dependencies: the built production binary, generated HTTP router, live Factory runtime, CLI HTTP client, and public SSE stream were real. The copied local fixture used a Windows `cmd.exe /c echo default-output` `SCRIPT_WORKER` as a deterministic provider substitute; no remote or paid provider was used.
- Cost/call budget used: one submitted Work, one local `cmd.exe` invocation, zero external provider calls, and zero paid/API calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Actual built-binary human CLI | PASS | `you.exe worker-sessions list --work-id <work-id> --server http://127.0.0.1:21939` exited `0`; human output visibly contained the submitted Work ID and Worker Session ID. | Human output is a point-in-time live view and may show an in-flight state while the Work is settling. |
| Actual built-binary JSON CLI and REST parity | PASS | `--output json` exited `0`; matching REST returned HTTP `200`. Each returned one observation and matched on `workerSessionId`, `attemptId`, `factorySessionId`, `workId`, `workIds`, `state`, `confirmationState`, and `durationBasis`. | No production proxy or multi-instance deployment was involved. |
| Public association and Work correlation | PASS | Factory Event SSE returned HTTP `200`, `text/event-stream`, eight retained events, and one `DISPATCH_WORKER_SESSION_ASSOCIATION`. The association was correlated to the Work through the public `DISPATCH_REQUEST` context and dispatch ID. | The bounded run used one Work and one dispatch. |
| Factory health and stream availability | PASS | `/status` returned HTTP `200` with `factoryState=RUNNING` before and after the Work; runtime status was `IDLE`. The Factory Event SSE remained readable and contained the retained event snapshot. | No long-duration retention or reconnect behavior was exercised. |
| Clean shutdown | PASS | Built `server stop` exited `0`; the Factory process exited `0`; the loopback port was immediately reusable; the isolated temporary Factory root was removed by the harness; no remote/provider process was started. | This does not prove absence of unrelated processes owned by the host. |

Observed public identity values:

- Work ID: `batch-request-4e08f2ef-dc6e-4354-a893-9c4a9253cf9a-worker-sessions-loopback-work`
- Dispatch/attempt ID: `0b3633d4-3bb1-42ce-9ec1-7a69972d706b`
- Worker Session ID: `0b3633d4-3bb1-42ce-9ec1-7a69972d706b`
- Factory Session: `~default`
- State: `COMPLETED`
- Work IDs: `[batch-request-4e08f2ef-dc6e-4354-a893-9c4a9253cf9a-worker-sessions-loopback-work]`
- Confirmation state: `UNCONFIRMED`
- Failure: `null`
- Duration basis: `RECORDED_TIMESTAMPS`

## Customer journey

1. Build the actual customer binary with `go build -buildvcs=false -o <temp>/you.exe ./cmd/factory`; the command completed successfully and produced the SHA-256 identified above.
2. Start that binary in a temporary Factory root with the live `~default` Factory Session and a free loopback port. `/status` returned HTTP `200`, `factoryState=RUNNING`, and `runtimeStatus=IDLE` before admission.
3. Submit one `task` Work through the public CLI using the isolated fixture payload. The submission returned the Work ID recorded above.
4. Read `GET /factory-sessions/~default/events` as SSE. The response was HTTP `200` with `text/event-stream`; the retained public events yielded the dispatch association and its Worker Session ID.
5. Run the exact public CLI forms:

   ```text
   you.exe worker-sessions list --work-id <work-id> --server http://127.0.0.1:<port>
   you.exe worker-sessions list --work-id <work-id> --server http://127.0.0.1:<port> --output json
   ```

   The human form exited `0` and visibly contained the Work and Worker Session identities. The JSON form exited `0` and returned the exact completed observation above.

6. Issue the matching public request:

   ```text
   GET /factory-sessions/~default/worker-sessions?workId=<work-id>
   ```

   It returned HTTP `200` with one observation value-equal to the CLI JSON observation on the fields listed above.
7. Run `you.exe --server http://127.0.0.1:<port> server stop`. It exited `0`; the server process exited cleanly and the port was reusable.

## Cross-task integration and usability

- Documentation discoverability: the exact customer CLI, REST, `/status`, SSE, expected values, and operator stop conditions are recorded here and in `long-running-route-evidence.md`; the public session reference remains the customer-facing route guide.
- Permission and error behavior: no new permission boundary was introduced; this success path used only a local isolated Factory and did not expose response bodies, credentials, or provider transcripts.
- Persistence/reload behavior: not exercised by this loopback; the supported pause/resume and bounded route matrix remain evidenced in the story 002 ledger.
- Accessibility/keyboard/responsive behavior: not applicable to this CLI/HTTP behavior slice.
- Operational signals: `/status`, Factory Event SSE, process exit, and port reuse all provided the expected operational signals.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| None | — | The bounded actual-binary journey above | A reachable, correlated Worker Session read with clean shutdown | Observed; no route failure or identity mismatch | This report and the retained public-event correlation |

The historical approximately 2.5-day daemon condition was not reproduced by this short local run. That is an explicit limitation of this gate, not a claim that the historical duration has been proved safe.

## Verdict

PASS

## Delta-plan request [Required for FAIL/BLOCKED]

Not applicable: the bounded loopback passed and no correction was required by this gate.

## GATE-OPERATOR-LONG-RUN

After merge, run the following exact commands against a named healthy deployment after the operator-selected long-duration window (the reported approximately 2.5-day window should be named in the run record):

```text
you worker-sessions list --work-id <work-id> --server <server-url>
you worker-sessions list --work-id <work-id> --server <server-url> --output json
GET <server-url>/factory-sessions/~default/worker-sessions?workId=<work-id>
GET <server-url>/status
GET <server-url>/factory-sessions/~default/events
```

Expected values are one observation whose `workerSessionId`, `attemptId`, `workId`, `workIds`, Factory Session alias, and `state` match the public `DISPATCH_WORKER_SESSION_ASSOCIATION` joined to its `DISPATCH_REQUEST`; the human CLI must contain that Worker Session ID, JSON and REST must be successful and equal on the shared fields, `/status` must remain healthy, and SSE must return a healthy event stream.

Stop the gate and open a new correction lane if any command returns `FACTORY_UNREACHABLE`, a non-success status, a mismatched identity, a stale observation, an unhealthy `/status`, a failed SSE stream, or an owned process/listener that does not shut down cleanly. Do not silently repair the deployment while preserving the result.

Limitation: this report proves only the bounded local actual-binary path with one deterministic Work. It does not prove multi-day uptime, production proxies or infrastructure, multiple concurrent deployments, persistence across restart, or terminal CI; review owns terminal CI and merge.
