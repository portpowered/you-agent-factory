---
author: Agent Factory Team
last-modified: 2026-08-20
doc-id: agent-factory/proposals/dispatch-performance-cost-mvp-how-to
---

# Measure Dispatch Performance and Cost (MVP)

> Proposed customer guide. The metrics commands described here are a target
> CLI contract and are not yet part of the shipped CLI. Promote this material
> into the appropriate `docs/reference/` topics when the API and CLI behavior
> is implemented.
>
> This is the MVP slice of the factory performance program. Its sibling,
> `session-cost-customer-how-to.md`, specifies the cost lens in detail. The
> follow-on scope — work-centric metrics, flow depth, bottlenecks,
> distributions, trends, comparisons, gating, quality, and capacity — lives in
> `session-and-work-analytics-customer-how-to.md`.

Use dispatch performance reports to answer three questions:

1. What did a Factory Session cost?
2. How long did dispatches wait in queue and execute?
3. How reliably did dispatch attempts complete — by workstation, worker,
   provider, and model?

## Command overview

```text
you metrics session <session-id> [--lens cost]
  [--by-worker] [--by-dispatch]
you metrics summary [filters] [--lens cost]
  [--group-by workstation|worker|provider|model]
```

The command families have deliberately different scopes:

- `you metrics session` explains one Factory Session.
- `you metrics summary` analyzes a cohort of dispatch executions across
  sessions.
- `--lens cost` adds the monetary lens specified by
  `session-cost-customer-how-to.md` without creating another top-level
  command family.

Use global `--json` for machine-readable output. `--verbose` remains reserved
for command diagnostics on stderr and does not add analytical dimensions.

The MVP ships the `--lens` flag with `cost` as its only lens. The follow-on
plan adds the full lens taxonomy (`overview`, `flow`, `reliability`,
`quality`, `capacity`) as new lens values on this same flag; the default
report content specified here does not change shape when those lenses arrive.

Common filters for `you metrics summary`:

```text
--factory <name>
--workstation <name>
--worker <name>
--provider <id>
--model <id>
--status <status>
--since <duration-or-time>
--until <time>
```

## 1. Explain one Factory Session

Start with the default operational summary:

```bash
you metrics session <session-id>
```

Example:

```text
Factory Session fs-review-042 metrics as of 2026-08-20 14:32 PDT

STATUS                         SUCCEEDED
ELAPSED WALL TIME              18m 42s
DISTINCT WORK ITEMS                  8
DISPATCH ATTEMPTS                   19
WORKER SESSIONS                     17
MAX CONCURRENT EXECUTIONS            4
SUMMED EXECUTION TIME           41m 08s
SUMMED QUEUE TIME                6m 21s
RETRIES                              2

ATTEMPT OUTCOMES     COUNT
Accepted                15
Rejected                 2
Failed                   1
Canceled                 1
```

Summed execution time can exceed elapsed wall time because workers execute in
parallel. It answers "how much execution occurred?" rather than "how long did
the customer wait?"

For a running session, durations end at the report's `as of` time. Running and
queued attempts are shown separately and are not presented as completed
duration samples.

## 2. Attribute session time to workers

Add `--by-worker` to group dispatch execution by configured Worker identity:

```bash
you metrics session <session-id> --by-worker
```

```text
WORKER             SESSIONS  ATTEMPTS  QUEUE P50  RUN P50  RUN P95  SUCCESS
planner                   2         2        8s     1m 42s    2m 01s    100.0%
reviewer                  9        11       31s     2m 14s    4m 48s     81.8%
repairer                  6         6       12s     1m 09s    2m 36s     83.3%
```

`SESSIONS` counts distinct Worker Sessions. `ATTEMPTS` counts dispatch
executions, including retries. These values can differ when a Worker Session
continues across several dispatch attempts or when an interrupted attempt is
reconciled.

The default grouping is the authored Worker. Use `--by-dispatch` when every
execution attempt matters.

## 3. Inspect queue and execution time per dispatch

Use `--by-dispatch` for the most granular session timeline:

```bash
you metrics session <session-id> --by-dispatch
```

```text
DISPATCH       WORK       WORKSTATION  WORKER    ATTEMPT  QUEUE  EXECUTION  OUTCOME
dispatch-101   work-17    plan         planner         1     8s      1m 42s  ACCEPTED
dispatch-102   work-17    review       reviewer        1    44s      3m 16s  REJECTED
dispatch-108   work-17    repair       repairer        1    12s      1m 09s  ACCEPTED
dispatch-113   work-17    review       reviewer        2    19s      2m 07s  ACCEPTED
```

This view distinguishes:

- queue time: dispatch queued until worker execution starts;
- execution time: worker execution starts until the attempt terminates;
- attempt number: repeated execution for the same Work and stage;
- outcome: accepted, continued, rejected, failed, interrupted, or canceled.

Failed and canceled attempts still contribute execution and queue time. They
must not disappear from totals merely because they did not produce successful
Work.

## 4. Compare dispatch performance across sessions

Use `you metrics summary` for a population rather than one execution:

```bash
you metrics summary --factory customer-support --since 30d
```

```text
Dispatch summary
Window: 2026-07-21T00:00:00Z through 2026-08-20T14:32:00Z
Filter: factory=customer-support

FACTORY SESSIONS                       286
TERMINAL FACTORY SESSIONS              271
OPEN FACTORY SESSIONS                   15
DISPATCH ATTEMPTS                    6,902
SUCCESS RATE                         95.4%
FAILURE RATE                          2.9%
RETRY RATE                            6.2%

METRIC                         P50       P90       P95       MAX    SAMPLES
Session wall time            12m 08s   31m 42s   44m 10s   2h 18m       271
Dispatch queue time               18s     1m 41s    3m 09s      22m     6,902
Dispatch execution time       1m 06s    4m 14s    6m 37s      41m     6,755
```

The report always prints its time window, filters, and sample counts. Open
observations are counted but excluded from terminal-duration distributions
unless the command explicitly says otherwise.

## 5. Group by workstation, worker, provider, or model

`--group-by` breaks the same dispatch metrics down by one dimension:

```bash
you metrics summary \
  --factory customer-support \
  --since 14d \
  --group-by workstation
```

```text
WORKSTATION  ATTEMPTS  QUEUE P50  QUEUE P95  RUN P50  RUN P95  SUCCESS  FAILURE
triage          2,204         7s         31s       42s    1m 38s     98.1%     1.2%
review            918        39s       4m 12s    2m 17s    8m 44s     91.4%     6.8%
repair            311        18s       1m 09s    3m 02s   10m 27s     89.7%     8.4%
```

Grouping by `worker` uses the authored Worker identity. Grouping by `model` or
`provider` explains execution-channel differences — for example, whether one
provider's dispatches queue longer, run slower, or fail more often than
another's. Individual Worker Session IDs remain a drilldown dimension, not a
useful default cohort.

## 6. Add the cost lens

Cost composes with both scopes through `--lens cost`:

```bash
you metrics session <session-id> --lens cost --by-worker --by-dispatch
you metrics summary --factory customer-support --since 30d \
  --lens cost --group-by model
```

The session-scoped cost report — accrued valuation, token categories,
forecasting, coverage semantics, and attribution — is specified in
`session-cost-customer-how-to.md`. The summary scope applies the same
valuation rules to a cohort and reports cost totals per group alongside the
timing and outcome columns. The Costs service owns all monetary calculation.

## 7. Use JSON for further analysis

Every view supports global `--json`:

```bash
you --json metrics summary \
  --factory customer-support \
  --since 30d \
  --group-by provider > dispatch-summary.json
```

JSON consumers should retain:

- effective filters and time boundaries;
- the report's `asOf` timestamp;
- sample and excluded counts;
- metric definitions and units; and
- warnings or unavailable classifications.

Do not compare unlabeled values from reports produced under different filters
or metric definitions.

## Metric definitions

| Metric | Definition |
|---|---|
| Session wall time | Session start until terminal event, or `asOf` for a running session. |
| Dispatch queue time | Dispatch queued until worker execution starts. |
| Dispatch execution time | Worker execution start until the attempt terminates. |
| Summed execution time | Sum of dispatch execution durations; may exceed wall time under concurrency. |
| Retry | A new attempt explicitly retrying a failed, timed-out, or interrupted dispatch. |
| Success rate | Accepted or continued terminal attempts divided by all terminal attempts. |
| Failure rate | Failed terminal attempts divided by all terminal attempts. |

Rework, revisits, iterations, first-pass yield, cycle time, critical path, and
throughput are deliberately absent: they require the work-centric and
flow-classification machinery specified in the follow-on plan.

## Missing and partial data

Reports must distinguish zero from unavailable. Requesting a report never
turns missing data into zero:

- A dispatch with no start timestamp has unknown queue and execution time; it
  is counted in attempt totals and excluded from duration distributions, and
  the report states how many samples were excluded.
- A running attempt has elapsed time but no completed duration sample.
- Cost coverage follows `session-cost-customer-how-to.md`: unpriced usage is
  labeled, not valued at zero.

Each report prints sample counts and exclusion counts inline. A standalone
`you metrics coverage` command is follow-on scope; the inline discipline is
not.

## Record now, report later

The follow-on plan's comparisons are worthless without history, so the MVP
must ensure the canonical event ledger records facts the MVP itself never
reports:

- the immutable digest of each session's effective compiled Factory
  definition (required later by revision comparison); and
- Work trace lineage (`traceId`, `currentChainingTraceId`, predecessor trace
  IDs; required later by cross-session Work joins).

Dispatch queued, execution-start, and terminal timestamps are both recorded
and reported by the MVP.

## Surface stability

The `you metrics` namespace, `--lens`, `--group-by`, `--by-worker`,
`--by-dispatch`, and global `--json` ship in the MVP in their final shape. The
follow-on plan only adds commands, lens values, group-by dimensions, and
filters; it does not reshape output specified here. JSON field names and units
in MVP reports are a stable contract.

## Product boundary

The CLI presents one `you metrics` namespace even though the implementation
uses separate services. The metrics transport delegates operational and cohort
queries to Analytics and monetary valuation to Costs, then composes the
requested lenses. The MVP stands up the Analytics service with dispatch-scope
queries only. Factory Sessions, Worker Sessions, Work, and Recordings remain
the authoritative sources of lifecycle, usage, and lineage facts rather than
calculating analytical results themselves.
