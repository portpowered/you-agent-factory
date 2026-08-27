# Review-stage latency findings

## Scope and safety

This investigation uses read-only observations from the live Factory and GitHub.
The Factory endpoint was `http://localhost:7437`.
No command contacted port 7438 or changed Work, Worker Sessions, pull requests, or CI.

This investigation measures ten recent reviewer Worker Sessions for one completed review lane
and interprets them against the checked-in review, CI-wait, and visit-budget policy.

## Method

The live Work list identified review Work and its current state.
The Work-scoped Worker Session query supplied stable session identities and recorded timestamps.
GitHub queries supplied pull-request, comment, commit, workflow-run, and job timestamps.

All timestamps below use UTC.
Session duration is `endedAt - startedAt`, checked against `durationMillis`.
Inter-invocation idle is the next sampled `startedAt` minus the preceding `endedAt`.
GitHub CI time is workflow `updatedAt - createdAt`.

Worker Session lineage can include both task and review Work IDs.
The preceding session `14a36f57-b4be-448a-b312-b8e730311507` was excluded from the review sample.
It began after the blocking review comment and produced the feedback fix commit.
The sampled sessions begin with the subsequent reviewer visits.

## Measured review sessions

All ten sessions belong to Work `work-review-19`, named `ci-deflake-durable-session-browser-harness-readiness-timeout`.
They match [PR #2319](https://github.com/portpowered/you-agent-factory/pull/2319) by exact Work and head-branch name.

| # | Worker Session ID | Started | Ended | Wall time | Idle before | Attribution supported by evidence |
| ---: | --- | --- | --- | ---: | ---: | --- |
| 1 | `881f4c19-cf82-4d81-8567-b886c55afdb4` | 02:18:30.443 | 02:19:09.593 | 39.156 s | 123.699 s after excluded fix session | CI-status visit; required checks pending |
| 2 | `28ca5651-5ece-499f-8cf7-4b231af808a0` | 02:19:11.959 | 02:19:57.791 | 45.838 s | 2.365 s | CI-status visit; workflow still running |
| 3 | `bd6c6495-24d6-49de-a1e1-c0a668aba55d` | 02:20:00.434 | 02:20:53.471 | 53.050 s | 2.643 s | CI-status visit; coverage jobs still running |
| 4 | `b7295fef-686c-4b1f-a6cb-0a3a8905ee19` | 02:20:55.458 | 02:21:43.898 | 48.444 s | 1.987 s | CI-status visit; coverage jobs still running |
| 5 | `9f22da37-21cf-4e10-a61c-3bc6623cb37a` | 02:21:46.242 | 02:22:27.633 | 41.397 s | 2.343 s | CI-status visit; coverage jobs still running |
| 6 | `63160e47-2aa5-498a-9033-ac967a23a6ac` | 02:22:31.125 | 02:23:13.466 | 42.346 s | 3.492 s | CI-status visit; coverage jobs still running |
| 7 | `ab1a8900-add9-475f-a0bb-597df45d638e` | 02:23:15.678 | 02:24:10.072 | 54.399 s | 2.212 s | CI-status visit; functional coverage still running |
| 8 | `bec76c23-1e37-4420-9442-275b55ab20b0` | 02:24:12.970 | 02:24:31.265 | 18.299 s | 2.898 s | CI-status visit; functional coverage still running |
| 9 | `f85c4900-0c07-425d-adb5-065021927710` | 02:24:33.362 | 02:25:09.242 | 35.884 s | 2.097 s | Terminal-CI detection visit |
| 10 | `f3641625-42c0-4f47-8c46-ef61660cf5cc` | 02:25:11.432 | 02:38:58.025 | 826.593 s | 2.189 s | Independent verification, final review comment, and merge |

The ten sessions total 1,205.406 seconds of Worker Session wall time.
The nine gaps within the sample total 22.228 seconds.
The median session duration is 44.092 seconds.

## Correlated GitHub timeline

PR #2319 opened at 01:25:12 and merged at 02:38:02.
Its initial CI run `33030035140` ran from 01:25:16 through 01:37:15.
A blocking review comment was posted at 02:08:11.
The feedback fix commit `7bffa8abccba75de86c93ad9430528f03f7160a5` was committed at 02:15:10.
The author posted the resolution at 02:15:53.

Fresh CI run `33032728213` ran from 02:15:42 through 02:25:05, or 563 seconds.
Its longest required job was Backend Functional Coverage, from 02:16:18 through 02:24:53.
Backend Unit Coverage ran through 02:23:54.
Verification Policy then ran from 02:24:55 through 02:25:04.

The first nine sampled reviewer visits overlap that fresh CI interval.
Their combined Worker Session wall time is 378.813 seconds.
From the first sampled start until CI completion, the observable CI wait is 394.557 seconds.
The final review session began 6.432 seconds after the workflow completed.

The final review comment was posted at 02:37:52.
GitHub recorded the merge at 02:38:02.
The session ended at 02:38:58, after the merge result was observed.

No merge-conflict or rebase cycle is supported by the sampled PR evidence.
The measured head change was a direct blocking-feedback fix, not a rebase.
Human thinking time is not separately measurable because Worker Session timestamps combine reasoning and tool execution.

## Policy interpretation

The checked-in review policy requires correctness-first review, applicable quality checks,
criterion-by-criterion evaluation, and independent runtime proof when a change affects observable
CLI, API, UI, event, or lifecycle behavior. It explicitly rejects substituting the implementer's
evidence or a green test suite for that proof. For a documentation-only lane, the reviewer instead
records why runtime proof is not applicable. These requirements explain why the final review visit
can legitimately contain substantial repository inspection, builds, tests, and behavioral exercise;
removing them would reduce review rigor.

The configured flow sends a completed process visit to `task:awaiting-ci`, then through the
agentless `ci-wait` script before `task:in-review`. The script polls required checks every 120
seconds and releases the task only when checks are terminal, regardless of verdict. A reviewer
hold returns to `awaiting-ci`, so CI waiting should consume script time rather than reviewer-agent
time or another logical review cycle. The nine observed reviewer sessions during pending CI show
that the measured lane did not realize that intended behavior; the timestamps alone cannot say
whether the cause was a gate race, stale deployed configuration, or another routing defect.

Both process and review are `REPEATER` workstations. Their `VISIT_COUNT` guards allow 12 logical
round trips with a 24-raw-visit backstop across the two workstations. This permits legitimate
feedback/fix cycles, while making needless pending-CI redispatch expensive before the guard stops
the lane. In this sample, the nine pending-CI review visits consumed nine raw review visits but
only one PR feedback/fix cycle is evidenced.

Policy sources:

- `factory/workstations/review/AGENTS.md`, especially Steps 2, 2.1, and 2.2, defines independent
  checks, the CI hold route, and conditional runtime proof.
- `factory/scripts/ci-wait.py` defines terminal-check gating, the 120-second poll interval, and the
  bounded requeue behavior.
- `factory/factory.json` defines the `REPEATER` routes and the paired `VISIT_COUNT` limits
  (`maxVisits: 12`, `maxRawVisits: 24`).
- `factory/docs/standards/review-standards.md` requires independent evidence, convergence, and
  review ownership of terminal CI and merge.

## Ranked contributors and recommendations

The ranks use elapsed wall time where an interval can be bounded. Worker time is reported
separately because overlapping Worker Sessions and CI must not be added together. Percentages use
the 1,205.406 seconds of sampled Worker Session time, not the full PR lifetime. This is one PR and
ten visits, so savings are hypotheses to validate across more lanes, not fleet forecasts.

| Rank | Contributor | Measured evidence | Concrete optimization | Estimated saving and validation |
| ---: | --- | --- | --- | --- |
| 1 | Post-initial-CI delay before blocking feedback | Initial CI ended at 01:37:15; the blocking review comment arrived at 02:08:11: 1,856 s (30 m 56 s), affecting 1/1 PR. No matching session boundary is retained, so queue delay versus reviewer execution is unmeasurable. | Emit durable review-ready, dispatch-start, and first-review-action timestamps, then alert when review-ready Work remains undispatched beyond a chosen service objective. | Bound: 0-1,856 s per affected lane; the full 1,856 s is recoverable only if later telemetry proves the interval was idle. Validate over at least 20 lanes by comparing review-ready-to-dispatch p50/p90 before and after alerting. No rigor reduction. |
| 2 | Final independent review and merge visit | Session 10 lasted 826.593 s (13 m 46.593 s), 68.6% of all sampled Worker Session time; it produced the final review comment and merge. Its transcript is unavailable, so thinking, tool execution, required rebuilds, and runtime proof cannot be separated. | Provide the reviewer a generated evidence index containing changed surfaces, exact acceptance criteria, current-head CI links, and implementer commands, while preserving the reviewer's independent source inspection and reruns. | Hypothesis: save 120-240 s (15-29% of this visit) in evidence discovery, leaving 586.593-706.593 s. Validate with session transcripts and median final-review time across at least 20 comparable PRs. **Human tradeoff:** skipping independent builds, behavioral exercise, or review coverage could save more, but requires explicit operator approval because it reduces mandated rigor. |
| 3 | Fresh-head CI critical path and pending-CI redispatch | Run 33032728213 lasted 563 s (9 m 23 s); Backend Functional Coverage was the longest required job at 515 s. From the first sampled reviewer start to CI completion was 394.557 s. During that overlap, nine reviewer visits consumed 378.813 s (31.4% of sampled Worker time), with a 42.346 s median; these are compute costs, not an additional 378.813 s of wall time. | First enforce and instrument the existing `ci-wait` invariant so pending checks cannot dispatch review. Separately profile and shard/cache Backend Functional Coverage without dropping required checks. | Gate enforcement can save up to 378.813 s of reviewer Worker time and nine raw visits on a lane with this failure mode, but little wall time because CI remains mandatory. A 25% reduction in the 515 s critical job would save at most about 129 s of CI wall time if it remains critical. Validate via zero pending-CI reviewer dispatches and hosted job p50/p90 over 20 runs. **Human tradeoff:** removing required checks could save up to the 563 s run, but reduces verification rigor and requires operator approval. |
| 4 | Blocking-feedback correction cycle | Blocking comment to fix commit was 419 s (6 m 59 s); comment to author resolution was 462 s (7 m 42 s), affecting 1/1 PR and one evidenced feedback/fix cycle. | Make blocking comments machine-readable with criterion, reproduction, exact requested correction, and focused verification command so the executor can begin without rediscovery. | Hypothesis: save 60-120 s (13-26% of the 462 s cycle). Validate against comment-to-fix-commit medians for at least 20 single-fix cycles. No rigor reduction. |

Inter-invocation REPEATER idle is not a top contributor in this sample: the nine gaps inside the
sample total only 22.228 seconds (2.470 seconds per gap on average), and the final review began
6.432 seconds after CI completed. Rebase or merge-conflict cost is measured as zero supported
cycles, not as proof that such cycles never occur: GitHub shows one direct feedback-fix commit and
no rebase/conflict event. Cross-lane conflict prevalence and savings are therefore unmeasurable
from this sample.

## Reproducible evidence commands

Run these commands from a checkout with authenticated `gh` access and the live Factory available.
They are read-only.

```powershell
you --server http://localhost:7437 work list
you --server http://localhost:7437 --json work list --all --counts --max-results 200
you --server http://localhost:7437 worker-sessions list --work-id work-review-19 --output json
you --server http://localhost:7437 worker-sessions read --worker-session-id 881f4c19-cf82-4d81-8567-b886c55afdb4 --output json

gh pr view 2319 --repo portpowered/infinite-you --json number,title,state,createdAt,updatedAt,mergedAt,headRefName,headRefOid,mergeCommit,url
gh run list --repo portpowered/infinite-you --branch ci-deflake-durable-session-browser-harness-readiness-timeout --limit 30 --json databaseId,createdAt,startedAt,updatedAt,status,conclusion,headSha,event,url,workflowName
gh run view 33032728213 --repo portpowered/infinite-you --json jobs
gh api repos/portpowered/infinite-you/issues/2319/comments --paginate
gh api repos/portpowered/infinite-you/pulls/2319/reviews --paginate
gh api repos/portpowered/infinite-you/pulls/2319/comments --paginate
gh api repos/portpowered/infinite-you/pulls/2319/commits --paginate
```

The `portpowered/infinite-you` repository locator resolves to the canonical PR URLs under `portpowered/you-agent-factory`.
The normalized timestamps in this report omit comment bodies and unrelated personal data.

## Measurement limits

The fleet-wide Worker Session query returned `INTERNAL_ERROR` during collection.
Work-scoped queries remained available and supplied the required sample.
This sample measures ten sessions from one recent review lane, not ten independent pull requests.
It is suitable for visit-level timing but not for estimating cross-lane prevalence.

The transcript is unavailable for the final 826.593-second session.
Its final-review classification relies on the session interval, PR comment, merge timestamp, and Work state.
GitHub returned no submitted review objects or inline review comments for this pull request.
The issue comments are therefore the authoritative review-conversation timestamps.
