# Validation report: C06 ACP provider functional-test optimization

## Environment and artifact

- Commit/build identifier: `149f18ea545276ac506d467b9b9140590142a3d8` (rebased implementation head under validation; this report adds no runtime or test behavior).
- Environment and configuration: Windows `10.0.26100`, `go1.25.0 windows/amd64`, `GOAMD64=v1`, default `GOCACHE=C:\Users\andre\AppData\Local\go-build`, repository `go.mod`, no `GOFLAGS`.
- Customer entry point: local-real `root.BuildProcess` and `Process.Execute`, with the public `you run` CLI and loopback Factory Session HTTP boundary used by the ACP functional tests.
- Real and substituted dependencies: real application composition, real local filesystem, loopback HTTP, OS subprocesses, and stdio pipes; controlled raw JSON-RPC ACP peers and command counters are injected only through `serviceedges.Edges`; the MockWorkers proof uses a failing counted ACP command edge and a recording provider runner.
- Cost/call budget used: zero remote or paid provider calls; maximum cost `$0`.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Final package behavior | PASS | `go test -count=1 -timeout=20m ./tests/functional/providers/acp` passed in 65.365s; the rerun after one contaminated attempt passed. | Remote vendor behavior and unsupported OS semantics. |
| Repeat cleanup | PASS | Two exact `go test -count=3 -timeout=45m ./tests/functional/providers/acp` runs passed in 228.319s and 193.358s; each run exercised all package cases three times. | Host-wide process state outside package-owned resources. |
| Supported race | PASS | `go test -race -count=1 -timeout=45m ./tests/functional/providers/acp` passed in 121.186s. | Race behavior on unsupported platforms. |
| Topology reduction and isolation | PASS | The reviewed evidence artifact records 42 pre-migration root constructions versus 15 after migration, 17 shared eligible peer starts, and 21 retained isolated peer starts. | Whole-project construction and latency totals. |
| PR package timing | PASS (directional) | PR Backend Functional Coverage run `33167279703` reported ACP 13.147s versus the approximately 57s baseline; review-stage CI supplies the next same-head terminal timing. | The final review-fix head has not been allowed to reach terminal CI in this implementation-stage report. |
| CLEAN-009 clean-room reproduction | PASS | A detached worktree at `149f18ea54` passed the manifest audit, the ACP package in 66.021s, and the moved Workers mock witness in 0.548s; `git status --short` was empty before removal. | Remote or paid ACP calls are intentionally excluded. |
| Implementation-stage delivery | BLOCKED until handoff | The implementation head still needs its final push and required CI start; merge and terminal CI are review-stage responsibilities. | This report was captured before the final push. |

## Customer journey

1. Build one real application root with `support.BuildProcess` and a loopback API server, then invoke the public `you run` path with a controlled local ACP stdio peer. The shared matrix completed the eligible catalog, compatibility, prompt, content, event, failure, JavaScript, and mixed-routing journeys.
2. The shared fixture opened explicit public Factory Sessions for each prepared scenario, observed terminal Work and response outcomes, Provider Session and Worker Session lineage, ordered Factory Events, catalog persistence, and route-specific call/start counters.
3. The isolated cases retained fresh roots where process, stdio, negotiation, command-selection, environment, crash, disconnect, shutdown, golden-wire, or exact process-count identity was the customer-visible property. Their cleanup joined `Process.Execute`, closed the application, observed the listener unreachable, and removed the owned home.
4. The ACP-selected MockWorkers journey now runs in `tests/functional/workers/mock/javascript_acp_test.go`; its public execution succeeds while both the counted ACP command edge and the recording provider runner remain at zero.
5. The detached clean-room checkout repeated the scenario-contract audit and both runtime ownership cells without modifying the tracked tree.

## Cross-task integration and usability

- Documentation discoverability: the case-level matrix and topology evidence remain in `c06-providers-acp-evidence.md`; this report provides the required CLEAN-009 loopback record.
- Permission and error behavior: pinned permission/error, start failure, negotiation, authentication, partial failure, cancellation, and diagnostic assertions remain on their public response and event surfaces.
- Persistence/reload behavior: catalog add/delete/list and packaged initialization scenarios retain settings-backed persistence and idempotency assertions.
- Accessibility/keyboard/responsive behavior: not applicable; this lane changes Go functional-test ownership and has no UI surface.
- Operational signals: root/process construction counters, ACP peer starts, provider-runner calls, Factory Session IDs, listener reachability, joined processes, and owned-path absence are asserted at the relevant boundary.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| C06-F001 | blocking, resolved | `go run ./cmd/functionalscenarioproject -check contracts/functional-scenarios.json` on the review head | Every reviewed evidence symbol exists at its declared path. | The old catalog symbol was stale; the manifest now points to `TestACPSharedProcess`, which directly declares the three catalog scenario IDs. The checker reports 154 reviewed scenarios current. | `contracts/functional-scenario-evidence.json`, `contracts/functional-scenarios.json`, `tests/functional/providers/acp/shared_process_test.go`. |
| C06-F002 | blocking, resolved | Exact full-package repeat gate and focused disconnected-connection witness. | All repeated lifecycle and replacement assertions pass without cross-test interference. | The first post-feedback repeat attempt exposed an ordering-sensitive disconnect/concurrency failure; explicit daemon stops were added to retained tests, and the impossible wait for an intentionally live disconnected child was removed. Both subsequent exact count-three package runs passed. | `tests/functional/providers/acp/daemon_*.go`; exact repeat results above. |
| C06-F003 | blocking, resolved | `go test -count=1 -timeout=10m ./tests/functional/workers/mock -run '^TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected$'`. | MockWorkers remains a fake and no live ACP/provider command is reached. | The assertion now lives in the Workers mock cell and passes with zero counted ACP starts and zero provider-runner calls. | `tests/functional/workers/mock/javascript_acp_test.go`; clean-room run above. |
| C06-F004 | blocking, resolved | Read-only clean-room checkout and classification audit. | The current reviewed manifest and runtime ownership reproduce from a clean tracked tree. | Detached `149f18ea54` passed the audit, ACP package in 66.021s, Workers mock witness in 0.548s, and ended clean. | Clean-room procedure and results above. |
| C06-F005 | review-stage | After the final push, inspect the required PR checks once for start state and let review own terminal CI. | The final head has required CI started and its terminal timing/result is available to review. | The historical PR run proves directional package improvement; same-head terminal CI is intentionally not claimed before handoff. | PR #2391 and run `33167279703`; no new CI run has been started for `149f18ea54` at report capture. |

## Verdict

PASS for the local implementation and clean-room validation; delivery remains a review-stage handoff pending final push and required CI start.

## Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion: implementation-stage delivery and same-head PR timing evidence.
- Root-cause evidence or remaining uncertainty: the current local artifact cannot start CI until the final head is pushed; terminal CI and merge are explicitly outside this executor's scope. The earlier PR result supplies directional timing but is from the pre-feedback head.
- Smallest recommended correction/prerequisite: push the reviewed implementation head, confirm required CI has started, and attach the CI timing/result in a PR comment rather than a commit.
- Dependencies and retest scope: review-owned terminal CI, conflict check, and merge; if a required job fails on this head, rerun that failed job once and apply only failures caused by this diff.
