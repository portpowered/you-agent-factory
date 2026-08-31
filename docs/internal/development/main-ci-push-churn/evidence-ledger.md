# Main CI push-churn evidence ledger

This ledger belongs to `reduce-main-ci-compute-from-push-churn-001`. It records
the reproducible measurement contract and ownership decisions; report output,
run links, and hosted CI evidence stay in the implementation pull request
conversation and are deliberately not committed here.

## Measurement contract

Run the report from a clean checkout with a read-capable `GH_TOKEN`:

```text
node scripts/ci/main-ci-churn-report.mjs \
  --repository <owner/repository> \
  --workflow <workflow-file-or-id> \
  --branch main \
  --since <RFC3339> \
  --until <RFC3339> \
  --run-limit <1..1000> \
  --merge-limit <1..1000> \
  --format <json|markdown>
```

The command writes one complete versioned report to stdout only. It reads:

- completed `push` runs for the selected workflow and branch from the Actions
  workflow-runs API;
- every job page for every selected run, summing only explicit
  `started_at`/`completed_at` durations; and
- merged pull-request search results qualified by repository, base branch, and
  calendar dates, then counting only results whose API `merged_at` is inside
  the exact timestamp window. Search `total_count` and bounded pagination make
  truncation observable without relying on mutable closed-PR ordering.

The adapter uses a bounded page size and page budget, validates API
`total_count` where available, deduplicates an identical boundary record, and
fails closed for a missing/truncated page, inconsistent duplicate, malformed
timestamp or identifier, authorization failure, timeout, or request-budget
exhaustion. No response body is written to a repository file.

## Metric definitions

- Main CI job-seconds are the sum of completed, non-skipped job durations for
  the selected completed workflow runs. Skipped jobs contribute zero because
  they consume no runner time; negative durations on any non-skipped job fail
  the report. `mainCi.totalJobSeconds / mergedChanges.total` is
  `normalized.mainCiJobSecondsPerMergedChange`.
- A cancelled run is a started cancellation when `run_started_at` is present;
  otherwise it is a queued cancellation. `startedRate` is started cancelled
  runs divided by all selected completed runs. The burn position is the mean
  job-seconds of started cancelled runs divided by the mean job-seconds of
  successful runs.
- Main push volume is the selected push-triggered workflow-run volume. Bot
  pushes use the Actions actor `type: Bot` or a login ending in `[bot]`; absent
  actor identity is non-bot.
- A baseline-bot merge is a merge of
  `automation/shared-ci-baselines` or the exact
  `chore(ci): reconcile shared CI baselines` title. Other bot merges use the
  pull-request author or merger identity. Non-bot is the remainder. This keeps
  the baseline-bot ratio tied to the existing automation branch rather than to
  an arbitrary actor chosen for a report.
- Empty denominators are rendered as `null` in JSON and `n/a` in Markdown;
  they are not represented as zero measurements.

## Ownership and gates

| Surface | Owner in this story | Proof | Excluded from this story |
| --- | --- | --- | --- |
| `scripts/ci/main-ci-churn-report.mjs` | Main CI churn report | `MEASURE-UNIT-001` and one read-only live report | Workflow policy, baseline generation, test content |
| `scripts/ci/main-ci-churn-report.test.mjs` | Complete controlled case matrix | `node --test scripts/ci/main-ci-churn-report.test.mjs` | Meta tests that inspect source inventories |
| `Makefile` test target | CI-tooling registration | `make test-ci-workflows` | Any test/gate removal or weakening |
| This ledger | Methodology and decision record | Review inspection | Run output, artifacts, tokens, CI transcripts |

The report does not change workflow files, baseline snapshots, tests outside
its focused suite, thresholds, gates, generated clients, or product behavior.
The later workflow stories own event-specific cancellation and scheduled
baseline reconciliation.

## Workflow contention refresh before implementation

The required live refresh was run before editing the report. It found these
open workflow-touching pull requests:

- [#2503](https://github.com/portpowered/you-agent-factory/pull/2503) —
  `ci.yml`
- [#2345](https://github.com/portpowered/you-agent-factory/pull/2345) —
  `ci.yml`
- [#2236](https://github.com/portpowered/you-agent-factory/pull/2236) —
  `ci.yml`, `published-backend-conformance.yml`
- [#2222](https://github.com/portpowered/you-agent-factory/pull/2222) —
  `ci.yml`

This story does not edit those shared workflow surfaces. The later workflow
stories must refresh the same inventory before editing and immediately before
the final rebase.

## Evidence slots

The implementation PR comment must contain the exact command, schema version,
explicit before window, run and merge limits, JSON or Markdown report, and all
run/merge links. A report is complete only when its selected run and pull
request pages have been consumed without a truncation or dependency failure.
The report is a before/measurement artifact only; post-rollout directional
improvement remains `OPS-POST-001`.
