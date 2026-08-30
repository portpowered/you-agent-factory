# Petri guard functional deflake evidence

Status: operator-amended pre-fix characterization complete. The repository
code was not repaired in this lane because the only observed contention race is
owned by the sibling deflake lane.

The unchanged branch was tested at commit
`905d5fe06e54f2cc18fa9d5ed4f08d72e5d85739` with Go 1.25.0. The owned package
is `tests/functional/orchestration/petri/guards`; it contains 15 top-level
tests and uses one production-composed process with controlled command-runner
edges.

## Failed CI artifact attribution

The three named Backend Functional Coverage runs were inspected:

| Run | Package result | Recoverable from the retained artifact |
| --- | --- | --- |
| 33292135439 | `.../orchestration/petri/guards` failed | `functional-tests.md` names `TestGuardPartialCompletionPreservesIndependentOutcome`; no raw assertion event was retained |
| 33293101655 | the same package failed | the same test name; no raw assertion event was retained |
| 33301183038 | the same package failed | the same test name; no raw assertion event was retained |

The three `functional-tests.md` rows contain the package-level failure and a
truncated `logicaltarget/discovery.go:91` diagnostic selected by the renderer.
The downloaded artifacts do not contain the underlying `test2json` stream, so
that diagnostic is not treated as the test assertion. Local raw output below
supplies the missing assertion.

## Pre-fix attempt ledger

All valid local attempts ran before any repair. Windows runs used Go
`windows/amd64` on a 24-logical-CPU host; Linux runs used Go `linux/amd64`
under Ubuntu WSL2 on the same host. The contended runs set `GOMAXPROCS=4`.
Busy-worker PIDs were recorded by the runner and were killed and waited for;
every attempt reported zero remaining load workers.

| Attempt | Topology and command shape | Result | Cleanup/lifecycle |
| --- | --- | --- | --- |
| U-001 | Unloaded full package, `go test ./tests/functional/orchestration/petri/guards -count=1 -json` | pass; 15 tests | `processStarts=1 apiStarts=1 sessionsOpened=16 sessionsClosed=16 routes=0` |
| C-001 | Focused named test under 16 PowerShell busy workers | 1 pass | load workers remaining: 0 |
| C-002 | Ten focused `-count=1 -json` processes under 8 busy workers per run | 10 passes | each run cleaned 8/8 workers |
| C-003 | Five full-package coverage-shaped runs under 8 busy workers; PowerShell passed the literal `-coverprofile=$profilePath` name | 5 passes | each run cleaned 8/8 workers; the stray diagnostic profile was removed and this run was not used as attribution evidence |
| C-004 | Target full package packed with 11 concurrent functional-package Go processes | 1 pass | target shared fixture reported `processStarts=1 apiStarts=1 sessionsOpened=16 sessionsClosed=16 routes=0`; process cleanup: 0 remaining |
| C-005 | Linux focused named test under 16 busy workers | 1 pass | process cleanup: 0 remaining |
| C-006 | Linux target package packed with 11 concurrent functional-package Go processes | 1 pass | process cleanup: 0 remaining |
| C-007 | Linux named test repeated 30 times under 12 `taskset -c 0-3 yes` workers; test process also pinned to CPUs 0-3 | 27 passes, 3 failures (iterations 5, 22, and 28) | raw run reported `processStarts=1 apiStarts=1 sessionsOpened=30 sessionsClosed=30 routes=0`; all 12 load PIDs were cleaned |

The retained raw stream for C-007 was kept outside the repository at
`[LOCAL_TEMP]/petri-guards-linux-count-u0BQYw/test.json`; it is not committed.
The exact invocation was:

```text
taskset -c 0-3 env GOMAXPROCS=4 go test ./tests/functional/orchestration/petri/guards \
  -run '^TestGuardPartialCompletionPreservesIndependentOutcome$' \
  -json -count=30 -timeout=10m
```

## Operator-amendment whole-package campaign

The operator amendment required the same contention topology across the whole
owned package before deciding whether this lane had another repair to make.
These runs used the unchanged test and runtime sources at commit
`b2461393c2d47b68b3081a73031142984edbb20c` (the ledger-only commit), Go
`1.25.0 linux/amd64` under Ubuntu WSL2, `GOMAXPROCS=4`, the test process
pinned to CPUs 0-3, and twelve PID-tracked `taskset -c 0-3 yes` workers. The
persistent runs recorded every worker being killed and waited for, with zero
remaining load PIDs. C-008's ephemeral artifact did not retain its cleanup
line.

| Attempt | Exact command shape | Result | Cleanup/lifecycle |
| --- | --- | --- | --- |
| C-008 | Whole package, `-json -count=20 -timeout=30m`, same pinned four-CPU topology; first run retained only in ephemeral WSL `/tmp` | package exit 1; raw artifact was unavailable after WSL teardown, so no test identity is claimed from this attempt | cleanup output was not retained; this attempt is not used to attribute an additional failure |
| C-009 | Whole package, `-json -count=20 -timeout=30m`, same pinned four-CPU topology; retained at `[LOCAL_TEMP]/petri-guards-whole-gOfOv7/test.json` | package pass; 340/340 test and subtest pass events; no test fail event | `processStarts=1 apiStarts=1 sessionsOpened=320 sessionsClosed=320 routes=0`; zero load PIDs |
| C-010 | Whole package, `-json -count=30 -timeout=45m`, same pinned four-CPU topology; retained at `[LOCAL_TEMP]/petri-guards-whole-campaign-20260830/test.json` | package exit 1; 509/510 test and subtest pass events; exactly one test fail event, on repetition 20 of `TestGuardPartialCompletionPreservesIndependentOutcome`; all other events passed 30/30 | `processStarts=1 apiStarts=1 sessionsOpened=480 sessionsClosed=480 routes=0`; zero load PIDs |

C-010's retained `test2json` failure event was:

```text
{"Action":"fail","Package":"github.com/portpowered/infinite-you/tests/functional/orchestration/petri/guards","Test":"TestGuardPartialCompletionPreservesIndependentOutcome","Elapsed":0.09}
```

Its exact assertion remained the unchanged assertion at
`guard_cases_test.go:131`:

```text
guard_cases_test.go:131: Work "guard-partial-complete" state = &generated.WorkState{Id:(*string)(nil), Name:"failed", Type:"FAILED"}, want "complete"; item=generated.Work{...}
```

The retained structured records immediately before that assertion identify
`work_id=guard-partial-complete`, `event_name=command_runner.completed`,
`outcome=failed`, `exit_code=31`, followed by
`event_name=factory_runtime.dispatch_result`, `outcome=FAILED`, and
`failure_message=provider execution failed`. No other test emitted a fail
event or assertion failure in C-010. Together with C-007, this establishes one
observed contention race in the owned package: the controlled responder binds
the `COMPLETE` and exit-code-31 outcomes by concurrent command-arrival order
instead of Work ID. The package's other guard scenarios did not reproduce an
additional contention failure in the whole-package campaign.

The C-010 stream proves attribution and the scope decision; it does not prove
repair correctness or post-fix stability. The sibling lane owns the
Work-ID-specific responder binding repair. This lane therefore intentionally
does not modify `guard_cases_test.go`, `shared_fixture_test.go`, production
Factory Runtime, CI, or any sibling package. The retained deliverable for this
operator-amended lane is this sanitized ledger alone.

## Exact failure and causal trace

The first C-007 failure was the fifth repeated test invocation. Its retained
`test2json` events were:

```text
{"Action":"fail","Package":"github.com/portpowered/infinite-you/tests/functional/orchestration/petri/guards","Test":"TestGuardPartialCompletionPreservesIndependentOutcome","Elapsed":0.05}
```

The preceding assertion output at `guard_cases_test.go:131` was:

```text
guard_cases_test.go:131: Work "guard-partial-complete" state = &generated.WorkState{Id:(*string)(nil), Name:"failed", Type:"FAILED"}, want "complete"; item=generated.Work{ChainingTraceDepth:(*int)(0xc000412dd0), ConfirmationState:(*generated.ConfirmationState)(0xc000891120), Content:(*[]generated.WorkContentPart)(nil), CurrentChainingTraceId:(*string)(0xc000891130), ExpectedArtifacts:(*[]generated.WorkExpectedArtifact)(nil), FailureDetail:(*generated.FailureDetail)(0xc000f21bc0), HumanApproval:(*generated.HumanApproval)(nil), Name:"partial-complete", Payload:interface {}(nil), PreviousChainingTraceIds:(*[]string)(nil), Relations:(*[]generated.Relation)(nil), RequestId:(*string)(0xc000891140), State:(*generated.WorkState)(0xc00058dec0), StopSummary:(*generated.FactoryStopSummary)(nil), StructuredResult:interface {}(nil), SupersededBy:(*string)(nil), Tags:(*generated.StringMap)(0xc0004285a8), TraceId:(*string)(0xc000891180), WorkId:(*string)(0xc000891190), WorkTypeName:(*string)(0xc0008911a0)}
```

The raw structured output immediately before that assertion, reduced to its
stable fields, was:

```text
command runner: request failed; work_id=guard-partial-complete; event=command_runner.completed; outcome=failed; status=failed; failure_reason=non_zero_exit; command=codex
transitioner: result failed; work_id=guard-partial-complete; event=factory_runtime.dispatch_result; outcome=FAILED; status=error; failure_message=provider execution failed
```

The sequence is:

1. The test seeds `guard-partial-complete` and `guard-partial-failed` at
   `guard_cases_test.go:117-118`.
2. It configures two provider responses at `guard_cases_test.go:120-125`:
   `COMPLETE`, followed by exit code 31. The response helper at
   `shared_fixture_test.go:513-530` assigns those responses by the order in
   which concurrent provider calls acquire its mutex; it does not bind a
   response to a Work ID.
3. In the failing fifth iteration, the raw runtime output proves that the
   second/nonzero response was applied to `guard-partial-complete`. Since the
   helper's first response is `COMPLETE`, the other dispatch necessarily
   acquired the first response. This is the causal response-order inversion,
   not a missing terminal-status event.
4. `supportWaitForGuardTerminal` then observes terminal status and
   `readSharedGuardSession` reads the public Work projection
   (`shared_fixture_test.go:608-640`). That projection faithfully exposes the
   failed Work, and the unchanged exact assertion at line 131 rejects the
   wrong Work-ID/result pairing.

The evidence therefore identifies the first missing causal fact as a
Work-ID-specific provider-result binding in the controlled edge. CPU packing
is the width/scheduling inference that changes which dispatch enters the
sequence first; the raw test2json failure and Work-ID runtime events are the
direct evidence. No production Factory Runtime or shared support helper was
changed, and the repair remains with the sibling lane under the operator
amendment.

Remaining unproven edges are the deterministic repair, the 20-run repaired
contention gate, the full package and race gates on the repaired head, the
clean-room loopback, and PR CI.
