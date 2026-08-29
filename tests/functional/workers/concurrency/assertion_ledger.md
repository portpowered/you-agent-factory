# Concurrency assertion ledger

The executable spine is `TestConcurrencySharedProcess` in
`shared_process_test.go`. The normal cases share one root-built process and
immutable directory-to-runner routes; the Scope-4 configuration probe and
forced-cleanup probe retain separate process boundaries.

| Acceptance row | Executable witness | Observable property |
| --- | --- | --- |
| Retained concurrency row | `TestConcurrencySharedProcess/Cancel/CC-04` | Two Factory Sessions overlap with distinct session/runtime/dispatch/attempt correlations; canceling A emits only A cancellation, B completes, B later completes, exactly three provider calls are observed, and active calls return to zero. |
| CC-01 | `TestConcurrencySharedProcess/Capacity/CC-01` | Capacity one holds A, leaves B pending with no second start, admits B after release, and records maximum active calls exactly one. |
| CC-02 | `TestConcurrencySharedProcess/Capacity/CC-02` | Capacity two admits exactly two overlapping calls, leaves the third pending, then admits it after one release without fixture-wide serialization. |
| CC-03 | `TestConcurrencySharedProcess/Concurrent/CC-03` | Two explicit sessions overlap and retain distinct session, dispatch, Work, input-marker, and output-marker identities. |
| CC-04 | `TestConcurrencySharedProcess/Cancel/CC-04` | Session-scoped cancellation affects only A; B remains usable and its initial and later outputs retain B’s marker and correlations. |
| CC-05 | `TestConcurrencySharedProcess/Cancel/CC-05` | Operator-authorized characterization: top-level cancel returns HTTP 404 `NOT_FOUND` for the runtime Worker Session, so the runtime-owned call remains active and B stays queued until the controlled edge releases A; B then starts exactly once, completes, and contains no A marker. |
| CC-06 | `TestConcurrencySharedProcess/Capacity/CC-06` | Identical request replay reuses request/trace/Work identity and produces one Work Request event, one provider call, and one terminal effect. |
| CC-07 | `TestConcurrencySharedProcess/Capacity/CC-07` | Operator-authorized characterization: changed-body request-ID replay returns HTTP 201 with the original response, leaves the original Work snapshot unchanged, and creates no extra event or provider call. |
| CC-08 | `TestConcurrencySharedProcess/Capacity/CC-08` | Empty Work admission returns the current validation response before provider dispatch; provider calls remain zero and the explicit session remains usable. |
| CC-09 | `TestConcurrencySharedProcess/Capacity/CC-09` | An isolated malformed Worker reference fails validation before dispatch with zero provider calls; a fresh valid route in the shared package process remains usable. |
| CC-10 | `TestConcurrencySharedProcess/Concurrent/CC-10` | Capacity two runs one typed provider failure and one success without crossing markers, identities, or outcomes; active calls return to zero. |
| CC-11 | `TestConcurrencySharedProcess/Timeout/CC-11` | Deterministic timeout releases capacity, admits one successor exactly once, preserves successor output, and returns active calls to zero. |
| CC-12 | `TestConcurrencySharedProcess/Concurrent/CC-12` | Each explicit session’s public request/dispatch/terminal event sequence and provider starts are monotonic and correlated; no global FIFO is asserted. |
| CC-13 | `TestConcurrencySharedProcess/Cancel/CC-13` | Canceled/failed/saturated session state is closed and removed before a fresh explicit session runs with new session/runtime/dispatch/attempt/Work identities and no stale route state. |
| CC-14 | `TestConcurrencySharedProcess/Cleanup/CC-14` | A forced-failure subprocess preserves the original failure while checking session/dispatch/route/stream/call/process/listener/port/root cleanup. |

CC-05 and CC-07 are operator-authorized characterization evidence. They record
the current public behavior without changing the cancellation or idempotency
contracts; whether those contracts should change is owned outside this lane.
