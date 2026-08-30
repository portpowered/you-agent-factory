# Validation report: Guard terminal Work against late dispatch results

## Environment and artifact

- Commit/build identifier: validation worktree based on `4d6dbad7df9d3e09ceceeb3357d902d77c88659b`, with the Story004 runtime, replay, and test changes listed below applied.
- Environment and configuration: Windows `windows/amd64`; Go `go1.25.0`; Node `v22.12.0`; npm `10.9.0`; Bun `1.4.0`; GNU Make `4.4.1`. Generated artifacts were regenerated and remained clean; intentional source and test changes were present.
- Customer entry point: root-composed `Process.Execute` using the public CLI commands.
- Real and substituted dependencies: real `root.BuildProcess`, `pkg/wire`, services, CLI, local canonical event history, and generated contracts; controlled `ProviderCommandRunner` for the provider edge; no remote provider calls.
- Cost/call budget used: USD 0; one controlled provider call in the public witness; no paid or remote calls.
- GATE-REPLAY substitution authorized by `operatorAmendment1`: the temporary integrated validator produced `C:\Users\andre\AppData\Local\Temp\runtime-guard-late-dispatch-38168.replay.json`, SHA-256 `cef6ae42df4a7742c82b0ae1b4f4bcb73412054bd832398f5ded760ef71d66e7`, containing 19 canonical events. The missing `factory/logs/agent-fails.replay.json` was not fabricated.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Clean public move and late-result journey | PASS | `go test ./tests/functional/work/root_composition -run '^TestOperatorMoveTerminalWorkRejectsLateDispatchResult$' -count=1 -timeout 120s` exited 0. The root-composed test observed one `SUPERSEDED` cancellation, one provider call, terminal target and parent, healthy dependent, zero in-flight Work, no post-move session start, and one ordered `DISPATCH_RESULT_IGNORED` with `WORK_ALREADY_TERMINAL`, `CANCELED`, and terminal `complete` state. | Production queue topology and remote provider cancellation fidelity. |
| Canonical recording, replay, reconnect, and recovery | PASS | The temporary integrated validator ran `go test ./tests/functional/work/root_composition -run '^TestStory004RecordReplayLateDispatchValidation$' -count=1 -timeout 180s -v` and exited 0. The produced recording's exact event order is `RUN_REQUEST`, `INITIAL_STRUCTURE_REQUEST`, `SESSION_STARTED`, `FACTORY_STATE_RESPONSE`, `WORK_REQUEST`, two `RELATIONSHIP_CHANGE_REQUEST` events, `DISPATCH_REQUEST`, `DISPATCH_WORKER_SESSION_ASSOCIATION`, `MODEL_REQUEST`, two `WORK_STATE_CHANGE` events, `MODEL_RESPONSE`, `AGENT_RUN_RESPONSE`, `DISPATCH_RESULT_IGNORED`, `FACTORY_STATE_RESPONSE`, `RUN_RESPONSE`, `SESSION_RESULT_UPDATED`, `SESSION_COMPLETED`. Replay retained the ignored diagnostic at sequence 14/tick 5, reconstructed terminal target/parent and healthy dependent state, and reconnect retained the diagnostic in order. Recovery skipped stale active-dispatch placement and did not reactivate or cascade. | Replay's state-change hook intentionally does not re-emit replay-applied `WORK_STATE_CHANGE` inputs; exact move-event order is proved by the produced canonical recording, while replay proves the resulting state and diagnostic. |
| Race/repeat and normal-result behavior | PASS | `go test -race ./pkg/services/factory_runtime/internal/services/dispatch_planning/internal/service ./pkg/services/factory_runtime/internal/services/orchestration/engine ./pkg/services/factory_runtime/internal/services/orchestration/runtime -run 'Test.*(Invalidate|MoveWork|LateDispatch)' -count=20 -timeout 600s` exited 0. Focused normal routing, diagnostic, projection, reconnect, and replay tests also exited 0. | Long-duration production soak. |
| Contract generation and API alignment | PASS | `make interfaces-all` and `make api-smoke` exited 0. Regeneration and drift checks passed; bundled OpenAPI, generated Go/TypeScript/package artifacts, and live generated client/server alignment passed. | None for the exercised generated contract checks. |
| Fast repository verification | PASS | `make verify-fast` exited 0: Bun UI tests 216 passed/0 failed, Vitest dashboard tests 3,108 passed/0 failed, and the short Go suite reported 168 passed/0 failed with one environment capability skip. | The skipped Go subtest is an environment capability check, not a lane behavior assertion. |
| Repository lint and static gates | PASS for lane scope; observed-not-owned baseline drift recorded | `make lint` exited 1 only at `deadcode`: Windows reported 3,072 findings versus the checked-in Unix baseline of 3,074. `ui-lint`, `ui-deadcode`, `vet`, `backend-size`, `pkg-maint`, package/boundary/structure, contract, catalog, and format checks passed. The current-only findings are the platform-specific staging/terminal-port helper names; no lane-specific current-only finding remains. Per `operatorAmendment1`, this platform baseline drift is not repaired here. | A supported-baseline owner may reconcile platform-specific deadcode counts. |
| Structured VAL-001 loopback | PASS | This report records the amended environment, substitution, criteria, customer journey, exact evidence, findings, verdict, and remaining unproven edges using the validation-loopback structure. | Terminal CI, post-handoff conflicts, merge, and production rollout remain review/operations owned. |
| Delivery handoff | READY FOR FINAL HANDOFF | The local implementation and validation gates are complete. The required final rebase, push, PR creation/update, CI start, and review-comment check remain the last process actions for this iteration; terminal CI and merge remain review-owned. | Terminal CI result, conflict resolution after handoff, and merge. |

## Customer journey

1. Build the real process through `root.BuildProcess` and start the local API with a controlled `ProviderCommandRunner`.
2. Submit the target, joined parent, and dependent through public CLI batch submission; the controlled provider reaches its channel gate.
3. Move the parent and target to `complete` through public CLI. The command edge receives exactly one `SUPERSEDED` cancellation.
4. Release the late failed provider result. The public Work list remains exactly target=`complete`, parent=`complete`, dependent=`init`; no generated cascade Work, extra provider call, or session reactivation occurs.
5. Read the canonical event stream. Exactly one diagnostic is observed after the target move, with `WORK_ALREADY_TERMINAL`, `CANCELED`, terminal `complete` state, dispatch identity, and no raw worker/customer payload.
6. Record the incident through the temporary integrated validator, then replay the produced canonical artifact in service mode, reconnect from the retained event cursor, and recover the runtime. The 19-event recording preserves exact live values/order; replay and reconnect retain the ordered ignored diagnostic, terminal target/parent, healthy dependent, and no cascade/reactivation. The test harness and artifact are local-only; no named missing fixture was invented.

## Cross-task integration and usability

- Documentation discoverability: this report is at `docs/internal/development/runtime-guard-terminal-tokens-against-late-dispatch-results-validation.md`; the prior characterization evidence remains at `docs/internal/development/runtime-guard-terminal-tokens-against-late-dispatch-results-evidence.md`.
- Permission and error behavior: the exercised CLI move and controlled cancellation path returned success; no public permission surface changed.
- Persistence/reload behavior: canonical serialization, replay state application, reconnect, and restored terminal-work recovery all passed; replay diagnostics are state no-ops and do not reactivate Work.
- Accessibility/keyboard/responsive behavior: not applicable; this story changes backend/event behavior and no UI surface was intentionally changed. The fast UI gate passed generated-event consumer coverage.
- Operational signals: the live witness records one bounded ignored diagnostic and no sensitive payload; no remote or paid dependency was used.
- Shared staging cleanup: `.you--full-flow.staging-owner/.owner.json` recorded PID 6224; `Get-Process -Id 6224 -ErrorAction SilentlyContinue` returned no process. The retained owner directory was moved to `C:\Users\andre\.you-agent-factory\reclaimed-staging-owner-20260830-001` outside the scan root and remains recoverable. The harness should self-heal a dead owner marker; this lane does not change the harness.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| VAL-001-001 | Resolved by operator amendment | Check for `factory/logs/agent-fails.replay.json`, then run the lane-produced integrated validator. | GATE-REPLAY needs a canonical recording and exact replay/reconnect/recovery proof. | The named fixture is absent from the compared refs and worktree, so the amended gate uses the lane-produced recording instead. | 19-event artifact at the path and SHA-256 recorded above; integrated validator exited 0; no fabricated fixture. |
| VAL-001-002 | Resolved environment | Inspect `.you--full-flow.staging-owner/.owner.json`, verify PID 6224, and inspect the scan root after reclaim. | Packaged-factory staging is available without a live owner conflict. | PID 6224 was absent; the owner directory was retained and moved outside the scan root. | Process check returned no process; recoverable retained path recorded above. |
| VAL-001-003 | Resolved | Run `make verify-fast`. | Generated event vocabulary and timeline humanizer cover every current canonical event. | All 216 Bun tests, 3,108 Vitest tests, and the short Go suite passed. | Fast gate exit 0. |
| VAL-001-004 | Observed, not owned | Run `make lint`. | Repository static gates use the supported platform baseline. | All lane-owned size/maintainability and other checks pass; only Windows deadcode count 3,072 versus Unix baseline 3,074 fails. | Lint exit 1 with platform-only current/baseline helper swap; no lane-specific finding. |
| VAL-001-005 | Non-blocking baseline context | Run the old long replay-fixture selections. | Unchanged fixture expectations remain owned by their fixture/contract lane. | The old selections still expose unrelated worker-name/stdout/relation expectation drift; they are not the amended GATE-REPLAY path and were not repaired. | Prior isolated-home output retained as observed-not-owned context. |
| VAL-001-006 | Resolved by amended scope | Drive, record, replay, reconnect, and recover the guarded late-dispatch scenario locally. | The highest feasible integrated runtime proof completes without remote calls. | The produced canonical recording completed the amended proof; replay retained the diagnostic and terminal/healthy projection with no cascade or reactivation. | Integrated validator exit 0 and artifact hash recorded above. |

## Verdict

PASS for the amended VAL-001 behavioral and repository-scope criteria. The final implementation-stage delivery handoff is the remaining process action; review owns terminal CI, conflicts after handoff, and merge.

## Remaining unproven edges

- Production queue-specific stale-result rate and queue location (`OPS-ROLLOUT`).
- Real remote provider cancellation and long-duration production soak.
- Terminal CI, post-handoff conflicts, and merge (`REVIEW-CI-MERGE`).
