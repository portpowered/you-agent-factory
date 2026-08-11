---
author: Agent Factory Team
last-modified: 2026-08-10
doc-id: agent-factory/proposals/session-work-analytics-how-to
---

# Measure Factory Performance

> Proposed customer guide. The metrics commands described here
> are a target CLI contract and are not yet part of the shipped CLI. Promote
> this material into the appropriate `docs/reference/` topics when the API and
> CLI behavior is implemented.

Use performance reports to move from a single execution to longer-term Factory
improvement:

1. Explain where one Factory Session spent its time.
2. Follow one piece of Work through its attempts, iterations, and sessions.
3. Find workers, workstations, and states that create recurring delays.
4. Compare cohorts to determine whether a Factory graph change improved the
   result.

Cost remains a separate backend concern but is one customer-facing metrics
lens. Use `you metrics session <session-id> --lens cost` for monetary and token
valuation. Other lenses cover time, throughput, reliability, quality, rework,
and distributions.

## Command overview

```text
you metrics session <session-id> [--lens <lens>]...
  [--by-work] [--by-worker] [--by-dispatch]
you metrics work <work-id> --session <session-id> [--lineage] [--timeline]
you metrics worker-session <worker-session-id>

you metrics summary [filters] [--lens <lens>]...
you metrics bottlenecks [filters]
you metrics distribution --metric <metric> [filters] [--group-by <dimension>]
you metrics trend --metric <metric> [filters] --interval day|week|month
you metrics explain session <session-id> --metric <metric>
you metrics coverage [filters]
you metrics regressions [filters] --against previous|revision:<digest>
you metrics check --policy <path> [filters]
you metrics catalog [--lens <lens>]

you metrics compare revisions --factory <factory> \
  --baseline <factory-revision> --candidate <factory-revision> [filters]
you metrics compare periods \
  --baseline <from>..<until> --candidate <from>..<until> [filters]
```

The command families have deliberately different scopes:

- `you metrics session` explains one Factory Session.
- `you metrics work` explains one Work item or its cross-session lineage.
- `you metrics summary`, `trend`, and related commands analyze cohorts.
- `--lens cost` adds the focused monetary lens without creating another
  top-level command family.

Use global `--json` for machine-readable output. `--verbose` remains reserved
for command diagnostics on stderr and does not add analytical dimensions.

## Choose one or more lenses

The scope says which executions to measure. A lens says which questions to
answer. `--lens` is repeatable so customers can compose one report without
learning separate command families:

```bash
you metrics session <session-id> \
  --lens flow \
  --lens reliability \
  --lens cost
```

| Lens | Questions answered |
|---|---|
| `overview` | What happened, how much Work completed, and where did time go? |
| `flow` | How long did Work wait and execute, and what was the critical path? |
| `reliability` | How often did attempts fail, retry, revisit, or require rework? |
| `cost` | What token usage and monetary valuation were recorded or forecast? |
| `quality` | Did the result pass customer-defined acceptance or evaluation criteria? |
| `capacity` | Were configured workers or workstations saturated? |

`overview` is the default for entity and summary commands. Cost, quality, and
capacity remain explicitly unavailable when their required facts are absent;
requesting a lens never turns missing data into zero.

## 1. Explain one Factory Session

Start with the default operational summary:

```bash
you metrics session <session-id>
```

Example:

```text
Factory Session fs-review-042 metrics as of 2026-08-10 14:32 PDT

STATUS                         SUCCEEDED
ELAPSED WALL TIME              18m 42s
ACTIVE SESSION TIME            17m 03s
PAUSED TIME                     1m 39s
DISTINCT WORK ITEMS                  8
DISPATCH ATTEMPTS                   19
WORKER SESSIONS                     17
MAX CONCURRENT EXECUTIONS            4
SUMMED EXECUTION TIME           41m 08s
SUMMED QUEUE TIME                6m 21s
RETRIES                              2
WORK REVISITS                        3
FIRST-PASS YIELD                  62.5%

TIME CONTRIBUTION       DURATION    SHARE OF WALL TIME
Queue                     6m 21s     34.0%
Critical-path execution  11m 04s     59.2%
Paused                     1m 17s      6.8%
Unclassified                  0s      0.0%
```

Summed execution time can exceed elapsed wall time because workers execute in
parallel. It answers "how much execution occurred?" rather than "how long did
the customer wait?" The critical path explains the portion of wall time that
actually constrained session completion.

For a running session, durations end at the report's `as of` time. Running and
queued attempts are shown separately and are not presented as completed
duration samples.

## 2. Attribute session time to workers

Add `--by-worker` to group dispatch execution by configured Worker identity:

```bash
you metrics session <session-id> --by-worker
```

```text
WORKER             SESSIONS  ATTEMPTS  QUEUE P50  RUN P50  RUN P95  SUCCESS  REVISITS
planner                   2         2        8s     1m 42s    2m 01s    100.0%       0
reviewer                  9        11       31s     2m 14s    4m 48s     81.8%       3
repairer                  6         6       12s     1m 09s    2m 36s     83.3%       0
```

`SESSIONS` counts distinct Worker Sessions. `ATTEMPTS` counts dispatch
executions, including retries. These values can differ when a Worker Session
continues across several dispatch attempts or when an interrupted attempt is
reconciled.

The default grouping is the authored Worker. Use `--by-dispatch` when every
execution attempt matters, or `--by-work` when the customer-visible Work item
is the useful unit.

Combine views when investigating a specific session:

```bash
you metrics session <session-id> --by-work --by-worker --by-dispatch
```

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

## 4. Explain one piece of Work

Work identifiers are interpreted in a Factory Session scope. Start with the
session in which the Work was observed:

```bash
you metrics work <work-id> --session <session-id>
```

```text
Work work-17 metrics

TYPE                         release-review
CURRENT STATE                shipped (TERMINAL)
CYCLE TIME                   13m 48s
TIME IN QUEUE                 1m 23s
TIME IN EXECUTION             8m 14s
TIME IN STATES                4m 11s
DISPATCH ATTEMPTS                  4
WORKER SESSIONS                    4
RETRIES                            0
STATE REVISITS                     1
FIRST-PASS                         no
FACTORY SESSIONS                   1
```

Cycle time begins when the Work is admitted and ends when it first reaches a
terminal Work state. For non-terminal Work, the command reports elapsed cycle
time as of the query time and labels it `RUNNING`.

Add `--timeline` to show state residence, queue, and worker execution in event
order:

```bash
you metrics work <work-id> --session <session-id> --timeline
```

## 5. Follow Work across Factory Sessions

A Work ID identifies one Work record. Use `--lineage` when the customer wants
the broader logical piece of work represented by its trace and chaining
lineage:

```bash
you metrics work <work-id> --session <origin-session-id> --lineage
```

```text
Lineage trace-incident-284

FACTORY SESSIONS                   3
WORK RECORDS                       7
DISPATCH ATTEMPTS                 16
END-TO-END CYCLE TIME         2h 18m
EXECUTION TIME                1h 06m
QUEUE TIME                      31m
RETRIES                           2
STATE REVISITS                    4
TERMINAL RESULT            SUCCEEDED
```

Cross-session association must use recorded Work lineage such as `traceId`,
`currentChainingTraceId`, and predecessor trace IDs. Similar names or payloads
are not sufficient evidence that two Work records represent the same logical
piece of work.

## 6. Read the operational summary over time

Use `you metrics summary` for a population rather than one execution:

```bash
you metrics summary --factory customer-support --since 30d
```

```text
Analytics summary
Window: 2026-07-11T00:00:00Z through 2026-08-10T14:32:00Z
Filter: factory=customer-support

FACTORY SESSIONS                       286
TERMINAL FACTORY SESSIONS              271
OPEN FACTORY SESSIONS                   15
WORK ADMITTED                        2,814
WORK COMPLETED                       2,641
WORK COMPLETION RATE                 93.9%
FIRST-PASS YIELD                     78.4%
RETRY RATE                            6.2%
REVISIT RATE                         11.7%
THROUGHPUT                       88.0/day

METRIC                         P50       P90       P95       MAX    SAMPLES
Session wall time            12m 08s   31m 42s   44m 10s   2h 18m       271
Work cycle time               4m 21s   13m 08s   18m 40s   1h 02m     2,641
Dispatch queue time               18s     1m 41s    3m 09s      22m     6,902
Dispatch execution time       1m 06s    4m 14s    6m 37s      41m     6,755
```

The report always prints its time window, filters, sample counts, and coverage.
Open observations are counted but excluded from terminal-duration
distributions unless the command explicitly says otherwise.

Common filters include:

```text
--factory <name>
--factory-revision <digest>
--work-type <name>
--workstation <name>
--worker <name>
--provider <id>
--model <id>
--status <status>
--since <duration-or-time>
--until <time>
```

## 7. Measure Work flow and rework

Use the Work view to compare cycle time, first-pass yield, retries, and repeated
processing:

```bash
you metrics summary \
  --factory customer-support \
  --since 30d \
  --lens flow \
  --lens reliability \
  --group-by work-type
```

```text
WORK TYPE        COMPLETED  CYCLE P50  CYCLE P95  FIRST PASS  RETRY  REVISIT
question             1,842      2m 18s      8m 04s       91.2%    2.1%     4.8%
incident               603     11m 42s     38m 51s       62.7%   10.6%    22.4%
release-review         196     18m 09s     51m 20s       54.1%   14.8%    31.6%
```

The default report uses `revisit`, not `rework`, for graph-derived loops. A
revisit means Work entered a previously visited non-terminal state or repeated
an already visited processing stage. Some Factory graphs intentionally iterate,
so a revisit is not automatically waste.

Report a true `rework rate` only when the Factory definition or recorded
outcome explicitly classifies a route or state as rework. When that
classification is absent, the CLI prints `REWORK: unclassified` instead of
renaming every loop as rework.

## 8. Compare workers and workstations

Use the workers view for execution capacity and reliability:

```bash
you metrics summary \
  --factory customer-support \
  --since 14d \
  --lens flow \
  --lens reliability \
  --group-by workstation
```

```text
WORKSTATION  ATTEMPTS  QUEUE P50  QUEUE P95  RUN P50  RUN P95  FAILURE  REVISIT
triage          2,204         7s         31s       42s    1m 38s      1.2%     2.4%
review            918        39s       4m 12s    2m 17s    8m 44s      6.8%    18.1%
repair            311        18s       1m 09s    3m 02s   10m 27s      8.4%    12.9%
```

Grouping by `worker` uses the authored Worker identity. Grouping by `model` or
`provider` explains execution-channel differences. Individual Worker Session
IDs remain a drilldown dimension, not a useful default cohort.

## 9. Find likely bottlenecks

Use the bottleneck report to rank constrained stages and show the evidence for
the ranking:

```bash
you metrics bottlenecks --factory customer-support --since 14d
```

```text
RANK  STAGE    SIGNALS                                  QUEUE P95  CYCLE SHARE  SAMPLES
1     review   high queue, high critical-path share       4m 12s        41.8%      918
2     repair   long-tail execution, elevated failures     1m 09s        19.6%      311
3     approve  low throughput, frequent blocked time         22s        14.1%      204
```

A bottleneck row is a diagnosis candidate, not proof of causation. The report
should expose its component signals:

- queue-time distribution;
- execution-time distribution;
- time spent on the critical path;
- throughput and arrival rate;
- failure, retry, and revisit contribution;
- blocked or approval waiting time; and
- capacity saturation, only when configured capacity is known.

Unknown capacity must not be presented as zero utilization.

## 10. Inspect one distribution

Use a distribution command instead of relying on a single average:

```bash
you metrics distribution \
  --metric work.cycle-time \
  --factory customer-support \
  --since 30d \
  --group-by factory-revision
```

Default distribution output includes sample count, minimum, p50, p90, p95,
maximum, arithmetic mean, and coverage. JSON output additionally contains the
exact bucket boundaries or quantile method used by the service.

Useful metric names include:

```text
session.wall-time
session.active-time
work.cycle-time
work.queue-time
dispatch.queue-time
dispatch.execution-time
work.iterations
work.revisits
worker.failure-rate
worker.retry-rate
```

## 11. Follow a trend

Use trend reports to see whether a metric changed gradually or only during a
short-lived event:

```bash
you metrics trend \
  --metric work.cycle-time \
  --factory customer-support \
  --since 90d \
  --interval week \
  --group-by factory-revision
```

```text
WEEK          REVISION       SAMPLES  P50       P95       COMPLETION
2026-06-08    sha256:81a...      402   8m 14s    28m 09s        89.6%
2026-06-15    sha256:81a...      438   7m 58s    27m 41s        90.1%
2026-06-22    sha256:b93...      451   6m 02s    19m 10s        94.3%
2026-06-29    sha256:b93...      469   5m 47s    18m 32s        94.8%
```

Do not infer that the revision caused the change merely because the dates
align. Compare workload mix, providers, models, and sample sizes before drawing
that conclusion.

## 12. Compare Factory graph revisions

Every Factory Session used for revision comparison must record an immutable
digest of its effective compiled Factory definition. A mutable Factory name or
the server's current edit version is not enough.

Compare two revisions of the same Factory:

```bash
you metrics compare revisions \
  --factory customer-support \
  --baseline sha256:81a... \
  --candidate sha256:b93... \
  --since 90d
```

```text
Factory revision comparison: customer-support

METRIC                    BASELINE     CANDIDATE       CHANGE       ASSESSMENT
Completed Work                 840           920        +9.5%       context
Completion rate              89.8%         94.6%       +4.8 pp      improved
First-pass yield             68.1%         81.7%      +13.6 pp      improved
Work cycle p50              8m 14s        5m 51s       -29.0%       improved
Work cycle p95             28m 09s       18m 47s       -33.3%       improved
Queue time p95              5m 44s        2m 38s       -54.1%       improved
Retry rate                    9.2%          5.1%       -4.1 pp      improved
Revisit rate                 18.6%         10.4%       -8.2 pp      improved

Coverage: baseline 96.8%, candidate 98.1%
Warnings: candidate processed 12% fewer incident-type Work items
```

Rate changes use percentage points (`pp`). Duration and count changes use
relative percentages. The report includes sample sizes, coverage, and workload
mix warnings so a small or materially different cohort is not presented as a
conclusive improvement.

## 13. Compare time periods

Use period comparison when there was an operational change without a new graph
revision, such as worker capacity, provider, or model changes:

```bash
you metrics compare periods \
  --factory customer-support \
  --baseline 2026-06-01..2026-06-30 \
  --candidate 2026-07-01..2026-07-31
```

Period boundaries are interpreted as UTC in machine-readable input unless an
explicit offset is present. Human output prints the effective boundaries and
timezone before the results.

## 14. Use JSON for further analysis

Every view supports global `--json`:

```bash
you --json metrics bottlenecks \
  --factory customer-support \
  --since 30d > bottlenecks.json
```

JSON consumers should retain:

- effective filters and time boundaries;
- the report's `asOf` timestamp;
- sample and excluded counts;
- event and duration coverage;
- Factory revision digests;
- metric definitions and units;
- quantile method; and
- warnings or unavailable classifications.

Do not compare unlabeled values from reports produced under different filters,
metric definitions, or revision identities.

## Metric definitions

| Metric | Definition |
|---|---|
| Session wall time | Session start until terminal event, or `asOf` for a running session. |
| Active session time | Wall time minus recorded paused intervals. |
| Work cycle time | Work admission until its first terminal Work state. |
| Dispatch queue time | Dispatch queued until worker execution starts. |
| Dispatch execution time | Worker execution start until the attempt terminates. |
| Summed execution time | Sum of dispatch execution durations; may exceed wall time under concurrency. |
| Critical-path execution | Execution intervals that constrain observed end-to-end completion. |
| Retry | A new attempt explicitly retrying a failed, timed-out, or interrupted dispatch. |
| Revisit | Work repeats a previously visited non-terminal state or processing stage. |
| Iteration | A completed pass through a declared iterative stage or loop. |
| First-pass yield | Terminal successful Work without a retry, revisit, or classified rework step. |
| Throughput | Terminal Work count divided by the effective observation window. |
| Rework | A route or state explicitly classified as rework by the Factory contract. |

## Missing and partial data

Reports must distinguish zero from unavailable. Each report includes coverage
for the facts it needs, such as queue timestamps, execution timestamps, Work
lineage, terminal events, and Factory revision identity.

Examples:

- A dispatch with no start timestamp has unknown queue and execution time.
- Running Work has elapsed cycle time but no completed cycle-time sample.
- Work without trace lineage cannot be joined safely across sessions.
- A session without an effective Factory digest appears under `unknown
  revision` and is excluded from revision comparison.
- A workstation without configured capacity has no saturation percentage.

## Explain a surprising result

Use `explain` to rank the recorded contributors to one metric:

```bash
you metrics explain session <session-id> --metric session.wall-time
```

```text
Largest observed contributors to session.wall-time

1. review queue                         4m 12s  22.5% of wall time
2. reviewer execution on work-17       3m 16s  17.5% of wall time
3. approval blocked interval            2m 41s  14.4% of wall time
4. repair-to-review revisit             1m 28s   7.9% of wall time

Coverage: 97.6% of wall time classified
```

An explanation describes observed contribution and correlation. It must not
claim that changing the highest-ranked contributor will cause an equal
improvement.

## Check metrics data health

Use coverage before trusting a comparison or automating a threshold:

```bash
you metrics coverage --factory customer-support --since 30d
```

```text
FACT                         COVERAGE  MISSING  EFFECT
Factory revision identity       98.2%       51  revision comparisons exclude rows
Dispatch queued timestamp       99.7%       19  queue distributions are partial
Worker start timestamp          97.9%      145  execution distributions are partial
Work trace lineage              92.4%      214  cross-session Work joins are partial
Quality outcome                 61.8%    1,075  quality comparisons are not representative
```

Coverage is itself a first-class metric. Reports should link their warnings to
the relevant coverage row instead of merely printing `partial`.

## Detect and gate regressions

Find recent changes that exceed configured noise and sample-size thresholds:

```bash
you metrics regressions \
  --factory customer-support \
  --since 7d \
  --against previous
```

```text
METRIC                  BASELINE  CURRENT  CHANGE   CONFIDENCE  STATUS
Work cycle p95           18m 47s  24m 13s  +28.9%   sufficient  regressed
First-pass yield           81.7%    80.9%  -0.8 pp  insufficient unchanged
Review queue p95          2m 38s   4m 09s  +57.6%   sufficient  regressed
```

For CI or Factory promotion, evaluate a checked-in policy:

```bash
you metrics check \
  --policy ./factory/metrics-policy.yaml \
  --factory-revision <candidate-revision>
```

A policy can require minimum sample size and coverage, set upper or lower
bounds, and choose whether insufficient evidence fails the check. Human output
explains every failed rule; JSON returns stable rule identifiers for
automation. This makes performance and reliability regressions reviewable in
the same workflow as Factory graph changes.

## Keep quality beside speed and cost

Factories should not appear to improve merely because they became faster or
cheaper while producing worse results. The quality lens should accept recorded
customer outcomes such as approval, evaluator score, escaped defect, or task
success:

```bash
you metrics compare revisions \
  --factory customer-support \
  --baseline <old-revision> \
  --candidate <new-revision> \
  --lens flow \
  --lens cost \
  --lens quality
```

Quality metrics require a named rubric and version. Scores from different
rubric versions are shown separately unless the customer explicitly defines
them as comparable.

## Discover available metrics and dimensions

Use the catalog instead of guessing metric names:

```bash
you metrics catalog
you metrics catalog --lens reliability
```

The catalog reports each metric's stable identifier, unit, direction of
improvement, required facts, supported groupings, and availability. This also
supports shell completion for `--metric`, `--group-by`, and `--lens`.

## Additional high-value extensions

The unified namespace leaves room for several capabilities without adding new
top-level command families:

- `you metrics watch session <id>` refreshes the same session report while it
  runs, preserving a stable table layout and `asOf` time.
- Named cohorts let teams save reviewed filters such as production incidents
  and reuse them in summaries, trends, comparisons, and checks.
- `--match-by work-type,priority` makes revision comparisons balance known
  workload dimensions before reporting a change.
- `--top`, `--sort`, and repeated `--where` filters support drilldown without
  exporting every event.
- CSV and NDJSON output complement API-shaped JSON for notebooks and data
  warehouses.
- Revision and deployment annotations appear on trend reports so changes can
  be correlated with observed movement.
- A future capacity forecast can estimate queue growth under a specified Work
  arrival rate, while clearly separating simulation from observed metrics.

## Common workflows

Explain why one session was slow:

```bash
you metrics session <session-id> --by-worker --by-dispatch
```

Follow a piece of Work through repeated or cross-session processing:

```bash
you metrics work <work-id> --session <session-id> --lineage --timeline
```

Find recurring operational bottlenecks:

```bash
you metrics bottlenecks --factory <factory-name> --since 30d
```

Check whether a graph revision improved performance:

```bash
you metrics compare revisions \
  --factory <factory-name> \
  --baseline <old-revision> \
  --candidate <new-revision> \
  --since 90d
```

Inspect operational and monetary lenses for the same session:

```bash
you metrics session <session-id> \
  --lens flow \
  --lens reliability \
  --lens cost \
  --by-work \
  --by-worker
```

## Product boundary

The CLI presents one `you metrics` namespace even though the implementation
uses separate services. The metrics transport delegates operational and cohort
queries to Analytics and monetary valuation to Costs, then composes requested
lenses. Factory Sessions, Worker Sessions, Work, and Recordings remain the
authoritative sources of lifecycle, usage, and lineage facts rather than
calculating analytical results themselves.
