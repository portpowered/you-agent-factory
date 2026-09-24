# ACP peer cause packet — 2026-09-24

## Disposition

**UNRESOLVED — missing child/process and Work/Event observations.** The retained
capture is complete and untruncated, but it does not identify either child
peer's terminal JSON-RPC response (or its absence), child stderr, process exit
or termination, or a reliable join from the failing selector to Work and
Factory Event timestamps. No shared cause or correction is inferred.

## Retained run and artifact

- PR #2645: `OPEN`, head `ff1ffd2d92a53915af6ada5c4320b8e011da8797`, base
  `41f5c1926eafa94f467e63b34c21ce30b0b0e242`.
- Hosted CI run `35873309829`, attempt 2, Linux Backend Functional Coverage
  job `107241161711` — `failure`; dependent Verification Policy job
  `107243332217` — `failure`.
- Artifact [`10758933879`](https://github.com/portpowered/you-agent-factory/actions/runs/35873309829/artifacts/10758933879)
  (`functional-test-diagnostics`): 1,055,761 bytes, not expired. Its raw
  failures index names the exact head, reports `captureStatus=complete`,
  `capHit=false`, captured/observed bytes both 163,876, event range 1–310, and
  no omitted event or byte range.
- Raw file: `functional-failure-acp-e45ba3a6c3e51726.jsonl`; SHA-256
  `6CEF9C94B7BE6D04F4B4AFC414347DAEC5230060EE49D72893D6E562BB9A9D73`.
- The raw index supplies this focused selector reproducer:

  ```text
  go test -p=12 -covermode=count -timeout=10m0s -json -x "-run=^TestProvidersACPSerializesConcurrentPromptsOnOneStdioConnection$|^TestYouRunMapsSkipPermissionsToSDKGoldenPermissionSelection$|^TestYouRunMapsSkipPermissionsToSDKGoldenPermissionSelection$/^default_rejects$" github.com/portpowered/infinite-you/tests/functional/providers/acp
  ```

  The retained failure came from the hosted Backend Functional Coverage job
  with coverage instrumentation and controlled local ACP peers. This is not a
  changed-head hosted pass.

## Observed failures and missing fields

| Selector | Observed at the Factory/test boundary | Missing for causal attribution |
| --- | --- | --- |
| `TestProvidersACPSerializesConcurrentPromptsOnOneStdioConnection` | Started at `2026-09-23T15:05:34.205227322Z`; failed at `15:05:49.132876940Z` (`5.90s`). The returned invocation has `syncOutcome=COMPLETED`, `sessionStatus=FAILED`, `resultStatus=UNAVAILABLE`, `error=nil`, session `dur-sess-941696b29d714e45bb9ba37dd80c4d32`. A log says `connection closed cause="peer connection closed"` at `15:05:43.513356320Z`. | No per-request peer RPC response/transcript, no proof whether a terminal response was absent, no child stderr capture, no child process identity or wait/exit/signal result, no Work ID, and no correlated Factory Event identity/time sequence. The connection-closed message does not establish why the peer closed. |
| `TestYouRunMapsSkipPermissionsToSDKGoldenPermissionSelection/default_rejects` | The assertion output at `2026-09-23T15:05:51.907006249Z` is `completed work = 0, want 1`; the subtest failed at `15:05:51.982004028Z` (`2.84s`). The sibling `skipPermissions_allows` passed in `2.74s`. | No per-request peer response/transcript, child stderr, process identity or wait/exit/signal result, failing Work ID, or correlated Factory Event identity/time sequence. The raw JSONL contains unstructured dispatch logs while ACP tests run in parallel; their attribution to this Work is not reliable. An earlier handoff comment reports an `unknown` dispatch / `Internal error` and empty fixture stderr, but this raw artifact does not provide the join needed to independently assign those fields to this selector. |

The artifact index's `exitStatus=1` is the captured Go test command's status,
not a child ACP peer exit result. Its complete capture rules out truncation as
the reason these fields are missing: the current observer does not record them
with the needed identity and correlation.

## Controls and changed-condition decision

- The exact-head Linux focused selector passed with coverage instrumentation
  (`go test -p=12 -covermode=count -count=1 -timeout=10m -v -run=...`), as
  recorded in [the #2645 follow-up](https://github.com/portpowered/you-agent-factory/pull/2645#issuecomment-5798480139).
  A Windows/amd64 focused selector on current main also passed in
  [the retained diagnosis](https://github.com/portpowered/you-agent-factory/pull/2645#issuecomment-5800402494).
  These controls do not identify the hosted child cause.
- PR #2644 changed only `contracts/testdata/baseline/cli-commands.json`, not an
  ACP-relevant condition. No unchanged hosted rerun is justified. Any further
  full-load observation must add the missing child/Event observer and stay
  within the PRD's run budget; request a plan/budget delta first if it would
  exceed that cap.

## Story 002 — correction disposition

**No correction is made on the retained evidence.** Story 001 did not establish
a shared cause: the complete capture lacks peer RPC outcome, child stderr and
termination, and the Work/Factory Event join for both failures. The no-correction
disposition means there is no causally supported repair to implement; it does
not claim that the failures have no shared defect. The focused passing controls
do not resolve that uncertainty, and no corrective behavior was changed, so the
Story 002 correction-path normal/race and injected-failure regression checks do
not apply to this disposition.

The smallest next gate remains the bounded child-process and public Work/Event
observer identified below, followed by one authorized changed-condition
coverage-like reproduction. Until then, changing provider/runtime behavior or
claiming the named failure regression would be unsupported.

## Smallest next gate

Add a bounded, redacted observer at the ACP child-process boundary that records
per-child process identity, request/response or explicit no-response, stderr,
wait/exit/termination outcome, and monotonic/UTC timestamps. Join those records
to the failing Work ID, Factory Session ID, and Factory Event IDs/timestamps
through the public event surface. Then use one authorized coverage-like
changed-condition reproduction only if the observer change makes that run
meaningful. Reassess cause after that evidence; do not change provider/runtime
behavior or infer a correction from the current package-level failures.

## Limits and provenance

- No paid/provider calls, new hosted run, source-code change, or corrective
  implementation was made for this packet.
- The referenced `docs/temp/projects/factory-reliability/source-plan.md` is
  absent from this worktree. The current `prd.json` contains the retained head,
  budget, scope, acceptance criteria, and evidence procedure used here.
- Existing handoff: [PR #2645 comment
  #5800402494](https://github.com/portpowered/you-agent-factory/pull/2645#issuecomment-5800402494)
  assigns Factory Reliability (ACP/CI) to capture child protocol/process
  evidence and correlate it with Work/Factory Events before LocalAI proceeds
  with the TTS gate.
