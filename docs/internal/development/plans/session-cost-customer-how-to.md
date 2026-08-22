---
author: Agent Factory Team
last-modified: 2026-08-20
doc-id: agent-factory/proposals/session-cost-how-to
---

# Measure Factory Session Costs

> Proposed customer guide. The cost lens and report flags described here
> are a target CLI contract and are not yet part of the shipped CLI. Promote
> this material into `docs/reference/sessions.md` when the corresponding API
> and CLI behavior is implemented.

Use Factory Session cost reports to answer three questions:

1. How much has this Factory Session consumed so far?
2. How much could its currently pending work cost?
3. Which providers, models, Worker Sessions, or dispatches account for that
   usage?

A session-scoped cost report values one Factory Session execution; the
separate Costs service owns the calculation. A Factory definition does not
have an accrued cost until it runs in a Factory Session.

This guide is one half of the dispatch performance and cost MVP. For dispatch
queue and execution timing and attempt outcome rates, use the proposed MVP
guide in `dispatch-performance-and-cost-mvp-customer-how-to.md`. For work
cycle time, iterations, rework, bottlenecks, distributions, and cross-session
Factory comparisons, use the proposed follow-on guide in
`session-and-work-analytics-customer-how-to.md`.

## Command overview

```text
you session list [--scope live|persisted|all] [--terminal]
you metrics session <session-id> --lens cost \
  [--forecast] [--by-worker] [--by-dispatch]
```

The default cost report shows accrued cost and token usage grouped by provider
and model. Add `--forecast` for the projected incremental cost of work that is
already queued or in flight. Add `--by-worker` or `--by-dispatch` when you need
to attribute the total to individual execution units.

Use global `--json` for machine-readable output:

```bash
you --json metrics session <session-id> --lens cost
```

`--verbose` remains reserved for command diagnostics on stderr. It does not
change the contents of a cost report.

## 1. Find the Factory Session

List sessions currently known to the running host:

```bash
you session list
```

The default scope is `live`. Use it when you are monitoring work that is
running now:

```bash
you session list --scope live
```

Use `--scope all` when you do not yet know whether a session is live or
persisted:

```bash
you session list --scope all
```

Copy the Factory Session ID from the list output. Use that exact ID for every
later cost, status, dispatch, artifact, and result command.

## 2. Measure accrued cost

Read the current cost report for one session:

```bash
you metrics session <session-id> --lens cost
```

For example:

```bash
you metrics session dur-sess-release-review-001 --lens cost
```

The default report should contain:

- the accrued monetary valuation, grouped by currency;
- whether that valuation is actual, estimated, mixed, or incomplete;
- total input, cached-input, output, and reasoning-output tokens;
- input/output share of total, cached/uncached share of input, and reasoning
  share of output;
- cost and token totals by billing provider; and
- cost and token totals by resolved model.

Example output:

```text
Factory session dur-sess-release-review-001 cost as of 2026-08-10 11:42 PDT

Accrued valuation:  $1.284300 USD (mixed actual/estimated)
Usage coverage:     9 of 10 Worker Sessions
Pricing coverage:   8 of 10 Worker Sessions
Unpriced:           2 Worker Sessions

TOKENS              COUNT    SHARE
Input             142,000    85.5% of total
  Cached           61,000    43.0% of input
  Uncached         81,000    57.0% of input
Output              24,000    14.5% of total
  Reasoning          9,000    37.5% of output

BILLING PROVIDER  COST        TOKENS    SHARE OF KNOWN COST
openai             $1.102300    142,000    85.8%
anthropic          $0.182000     24,000    14.2%

MODEL                         COST        INPUT     OUTPUT
openai/gpt-5.2                $0.942300     99,000     18,000
openai/gpt-5.2-mini           $0.160000     22,000      3,000
anthropic/claude-sonnet       $0.182000     21,000      3,000
```

The reported amount is the priced portion of the session. If usage or pricing
is unavailable, the command must identify the missing coverage instead of
treating it as zero cost.

Token categories are not all peers. Cached input is a subset of input, and
reasoning output is normally a subset of output. The report therefore uses
explicit denominators rather than adding every displayed row into one pie.
When a provider's usage semantics do not support a reliable denominator, the
corresponding percentage is shown as unavailable.

## 3. Measure projected session cost

Add `--forecast` to include the estimated incremental cost of dispatches that
are already queued or in flight:

```bash
you metrics session <session-id> --lens cost --forecast
```

Example:

```text
Accrued valuation:  $1.284300 USD
Pending estimate:   $0.030000-$0.090000 USD
Projected total:    $1.314300-$1.374300 USD
Forecast basis:     2 queued dispatches, resolved models, configured token limits
```

The forecast is a range, not a promise. A Factory can run workers concurrently,
take guarded branches, or retry failed work, so there may be no single exact
"next" cost.

The pending estimate covers only work the runtime already knows about. It does
not attempt to predict unselected future branches. When a queued dispatch has
no resolved model, price, or usable token limit, the report marks the forecast
as partial or unavailable.

For a terminal historical session, `--forecast` reports that there is no
pending work. The accrued amount remains the session's historical valuation.

## 4. Enumerate persisted and historical sessions

List Factory Sessions retained outside the live workspace:

```bash
you session list --scope persisted
```

Persisted sessions can include terminal sessions and recoverable interrupted
sessions. To list only sessions whose lifecycle has ended, add `--terminal`:

```bash
you session list --scope persisted --terminal
```

`--terminal` is shorthand for the terminal lifecycle states:

- `SUCCEEDED`
- `FAILED`
- `CANCELED`
- `TIMED_OUT`
- `INTERRUPTED`
- `TERMINATED`

The CLI should prefer `--scope persisted --terminal` over a separate
`--historical` flag. Persistence and lifecycle are different filters: a
persisted session can still be recoverable, while `--terminal` clearly states
that the customer wants completed history.

After selecting a historical session, inspect it with the same cost command:

```bash
you metrics session <historical-session-id> --lens cost
```

Historical cost reports are reconstructed from the session's canonical usage
history. Their applied price snapshot must not change when the current price
book changes.

## 5. Read reports with multiple currencies

The default report does not silently convert currencies. If a session contains
valuations in more than one currency, it shows one subtotal per ISO 4217
currency code:

```text
Accrued valuation:
  $1.284300 USD
  €0.240000 EUR
Combined total:     unavailable (multiple currencies)
```

Customers may compare or convert those subtotals outside the CLI. A future
explicit conversion option must report the exchange-rate source and effective
time; without those facts, adding unlike currencies would produce a misleading
total.

## 6. Attribute cost to Worker Sessions

Use `--by-worker` to identify which Worker Sessions account for the total:

```bash
you metrics session <session-id> --lens cost --by-worker
```

The report adds one row per Worker Session, including its state, provider,
model, token usage, valuation, and coverage status:

```text
WORKER SESSION             STATE       PROVIDER/MODEL          TOKENS    COST
dispatch-plan              COMPLETED   openai/gpt-5.2          42,100    $0.318200
dispatch-review            COMPLETED   anthropic/claude-sonnet 18,400    $0.182000
dispatch-repair            FAILED      openai/gpt-5.2-mini      7,900    $0.041100
```

Failed, canceled, and retried workers can still incur usage. Their cost must
remain in the report when the provider performed billable work.

## 7. Attribute cost to dispatches

Use `--by-dispatch` when you need orchestration-level attribution:

```bash
you metrics session <session-id> --lens cost --by-dispatch
```

This view groups usage by Factory Session dispatch. It is useful for comparing
workflow phases, workstations, retries, or repeated steps:

```text
DISPATCH                    PHASE       STATUS      TOKENS    COST
dispatch-plan               planning    COMPLETED   42,100    $0.318200
dispatch-review             review      COMPLETED   18,400    $0.182000
dispatch-repair             repair      FAILED       7,900    $0.041100
```

Worker Session and dispatch are distinct identities. A dispatch describes an
orchestration step; a Worker Session describes one supervised worker execution
context. Use both flags when both sections are needed:

```bash
you metrics session <session-id> --lens cost --by-worker --by-dispatch
```

## 8. Use JSON in scripts and reports

Request the API-shaped report with global `--json`:

```bash
you --json metrics session <session-id> --lens cost
```

Include forecasts and detailed attribution when needed:

```bash
you --json metrics session <session-id> --lens cost \
  --forecast \
  --by-worker \
  --by-dispatch
```

List terminal persisted sessions as JSON before selecting an ID:

```bash
you --json session list --scope persisted --terminal
```

Automation should read monetary amounts as decimal strings and retain the
reported currency, valuation kind, price-book version, and coverage fields. Do
not assume an absent or unpriced amount is zero.

## How to interpret the report

| Field | Meaning |
|---|---|
| Accrued valuation | Priced usage recorded so far, grouped by currency. |
| Actual | The provider reported a monetary charge. |
| Estimated | The CLI valued reported tokens using a recorded price snapshot. |
| Mixed | The total contains more than one valuation kind. |
| Unpriced | Usage exists, but no defensible monetary valuation is available. |
| Usage coverage | Worker Sessions for which normalized usage was recorded. |
| Pricing coverage | Worker Sessions whose recorded usage could be valued. |
| Pending estimate | Estimated incremental cost of known queued or in-flight work. |
| Projected total | Accrued valuation plus the pending estimate range. |
| Cached share | Cached-input tokens divided by total input tokens. |
| Uncached share | Non-cached input tokens divided by total input tokens. |
| Reasoning share | Reasoning-output tokens divided by total output tokens. |

Provider subscriptions and locally hosted models require special care. A
subscription-backed invocation may have no marginal per-request bill even when
its tokens have a retail-equivalent value. A local model may consume hardware
resources without a configured dollar rate. The report should label these
cases rather than declaring them free.

## Common workflows

Check a running session:

```bash
you session list --scope live
you metrics session <session-id> --lens cost --forecast
```

Find a prior execution and inspect its final cost:

```bash
you session list --scope persisted --terminal
you metrics session <session-id> --lens cost
```

Investigate an unexpectedly expensive session:

```bash
you metrics session <session-id> --lens cost --by-worker --by-dispatch
GET /factory-sessions/<session-id>/dispatches
```

Export a complete machine-readable report:

```bash
you --json metrics session <session-id> --lens cost \
  --forecast \
  --by-worker \
  --by-dispatch > session-cost.json
```
