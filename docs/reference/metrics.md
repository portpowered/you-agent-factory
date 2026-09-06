---
author: You Agent Factory Team
last-modified: 2026-09-05
doc-id: agent-factory/metrics
---

# Metrics

`you docs metrics` describes `you metrics`, which reports retained Factory Runtime metrics. Use it to inspect token usage, dispatch completion, failure reasons, provider attribution, and available latency.

## Start With A Live Factory Session

Run this sequence from any directory while the Factory API is reachable. It
does not require Factory source files in the current directory.

```bash
# List the public IDs accepted by the metrics session filter.
you session list --scope live

# Capture one ID from the structured live-session response.
SESSION_ID="$(you --json session list --scope live | jq -er '.sessions[0].id')"
test -n "$SESSION_ID"

# Compare all retained sessions with the selected live Factory Session.
ALL_METRICS="$(you --json metrics)"
SCOPED_METRICS="$(you --json metrics --session "$SESSION_ID")"
jq -n --argjson all "$ALL_METRICS" --argjson scoped "$SCOPED_METRICS" '
  {
    unscoped: $all.totals,
    scoped: $scoped.totals,
    scoped_totals_are_not_larger:
      ($scoped.totals.input_tokens <= $all.totals.input_tokens and
       $scoped.totals.output_tokens <= $all.totals.output_tokens and
       $scoped.totals.completed_dispatches <= $all.totals.completed_dispatches)
  }'

# Inspect the selected scope in each supported grouping.
you metrics --group-by workstation --session "$SESSION_ID"
you metrics --group-by worker --session "$SESSION_ID"
you metrics --group-by provider --session "$SESSION_ID"

# Use the global --json flag for one machine-readable provider report.
you --json metrics --group-by provider --session "$SESSION_ID"
```

The `jq -e` lookup fails when the live-session list is empty. Start or attach a
Factory, then repeat the list command. For a non-default API port, use the
same server selection for both commands: `you session list --port <port>` and
`you --server <uri> metrics`.

The unscoped report covers all retained Factory Sessions. The scoped report
uses the public ID returned by the live list and resolves its retained metrics
scope through Factory Sessions. A resolved scope with no recognized records is
a verified empty success with zero totals. It is different from a failed
scope lookup.

## Live And Persisted Factory Session IDs

The base metrics command accepts the live IDs returned by:

```bash
you session list --scope live
```

Do not copy a `dur-sess-*` value from `you session list --scope persisted` into
`you metrics --session`. Persisted Factory Session identities belong to the
durable inspection surface. They are not interchangeable with the public live
selector used to resolve retained runtime metrics.

If a live ID is unknown, or if it is known but has no retained metrics scope,
the command fails before rendering a report. The HTTP operation has the same
behavior:

| Code | HTTP status | Meaning | Next action |
|------|-------------|---------|-------------|
| `METRICS_SESSION_NOT_FOUND` | `404` | The requested live Factory Session ID is not known. | Run `you session list --scope live` and choose an ID from that response. |
| `METRICS_SESSION_SCOPE_UNAVAILABLE` | `503` | The live Factory Session is known, but no retained metrics scope is available. | Run `you session list --scope live` and choose a live ID with retained metrics. |

Both failures leave stdout empty. The CLI exits non-zero and writes the coded
diagnostic to stderr. A direct request uses the
`GET /metrics?session_id=<live-id>` route and returns the same code and
recovery message.

## Run The Command

Run `you metrics` to report metrics from all Factory Sessions. The default grouping is `workstation`.

```bash
you metrics
you metrics --group-by workstation
you metrics --group-by worker
you metrics --group-by provider
you metrics --group-by provider --session "$SESSION_ID"
you --json metrics
you --json metrics --group-by worker --session "$SESSION_ID"
```

The `--group-by` flag accepts these values:

- `workstation` groups results by workstation. This is the default.
- `worker` groups results by worker.
- `provider` groups results by provider.

An unsupported value returns an error before the command queries metrics.

## Scope One Factory Session

Use `--session "$SESSION_ID"` with an exact live Factory Session identifier.
The filter applies to totals, groups, failure reasons, and latency samples.

Without `--session`, the command reports all recorded Factory Sessions.

## Session-scoped dispatch and cost MVP

From an empty working directory, inspect one server-scoped Factory Session:

```bash
you --server <uri> metrics session <factory-session-id>
you --json --server <uri> metrics session <factory-session-id>
you --json --server <uri> metrics session <factory-session-id> --lens cost --by-worker --by-dispatch
```

The session report uses the selected live Factory Session scope. It shows
canonical attempts, outcomes, queue and execution samples, and exclusions.
Running, unavailable, and unpriced values remain explicit. `--lens cost`
composes the existing exact `GET /metrics/costs` report for the same session.
`--by-worker` and `--by-dispatch` show only proven canonical identities.
Worker detail rows are grouped by authored Worker. Each row reports the
distinct `sessions` count and deterministic `worker_session_ids`; when an
authored Worker or session cannot be proven, the row is labeled `unavailable`.

Unknown sessions, unavailable scope, timeout, cancellation, malformed replay,
or any requested read failure return a coded error with empty stdout. The
command never falls back to local metrics artifacts or unscoped history.

The existing `you metrics --session` and `you metrics costs --session`
compatibility workflows remain unchanged.

## Human Output

Human output includes totals and the requested deterministic breakdown. It
labels `Input tokens`, `Output tokens`, `Completed dispatches`, and `Failures by reason`.

Latency uses milliseconds. The command reports p50 and p95 values when samples exist. It prints `no samples` when a latency value has no samples.

The base metrics command does not emit a numeric price. Use `you metrics costs`
for runtime cost status and exact priced amounts.

The provider breakdown includes every completed dispatch. Its completed
dispatch counts, including the unavailable attribution group, sum to the
selected total. Human output labels that group
`Unavailable (provider attribution not proven)`.

Retained facts use one `unavailable` JSON provider key when the provider is
missing, contains only `${...}` template text, or has conflicting concrete
evidence. A `${...}` string is never exposed as a provider key. Token,
failure, and latency measures remain attached only to facts supported by that
provider evidence.

## JSON Output

Use the global `--json` flag for one machine-readable JSON document on stdout:

```bash
you --json metrics --group-by provider --session "$SESSION_ID"
```

The document names its scope, grouping, units, cost availability, totals,
groups, failure reasons, and latency percentiles.

The `groups` array contains only the requested grouping. Groups are sorted by key for deterministic output. Missing latency samples use `samples: 0`, `p50: null`, and `p95: null`.

The top-level fields are:

| Field | Meaning |
|-------|---------|
| `scope` | `all_factory_sessions` for an unscoped query or `factory_session` with the selected live ID. |
| `group_by` | The requested `workstation`, `worker`, or `provider` grouping. |
| `units` | `tokens`, `count`, and `milliseconds`. |
| `cost.availability` | The base metrics cost state. It remains `unavailable`. Use `you metrics costs` for runtime cost status. |
| `totals` | Token, completion, failure, and latency aggregates for the selected scope. |
| `groups` | Rows for only the requested grouping, sorted by key. |

An unscoped JSON result uses `"kind": "all_factory_sessions"` and a null `factory_session_id`. Empty results use zero counts, empty failure maps, an empty groups array, and null percentile values.

The HTTP `GET /metrics` report also exposes retained `usage_rows` and the
three canonical breakdown arrays. The CLI selects one of those breakdowns
under `groups` for the requested `--group-by` value.

## Runtime Cost Rollups

Use `you metrics costs` to inspect exact cost rollups through the running
Factory API. This is a separate command from the local `you metrics` artifact
reader:

```bash
you metrics costs
you metrics costs --session "$SESSION_ID"
you --json --server http://localhost:7437 metrics costs
```

The optional `--session` filter selects one Factory Session. Use a live ID from
`you session list --scope live` when the report must match the base metrics
workflow. Global `--server` selects the Factory API; global `--json` emits the
API-shaped report. A route failure returns an error without a partial success
document.

### Provider-Owned And Operator Price Facts

Costs uses two pricing authorities. Providers owns the immutable built-in table
shipped with the application. Operator Settings owns the optional `priceTable`
in `~/.you-agent-factory/config.json`. Omit `priceTable` to use an empty
operator table.

The effective table uses the exact normalized provider/model pair as its key.
An operator row supplements a missing built-in pair. A matching operator row
replaces the complete built-in row. Costs never combines fields from both rows.

When `priceTable` is present, `currency` must be `USD` and `models` must be an
array. An operator row must include `inputPerMillionTokens` and
`outputPerMillionTokens`. `cachedInputPerMillionTokens` and
`reasoningOutputPerMillionTokens` are optional. If an operator omits one of
these optional rates, that measured class remains `UNPRICED`.

The Operator Settings decoder trims provider and model values. It canonicalizes
supported provider aliases and does not guess or alias model identifiers.
Duplicate normalized provider/model pairs are rejected. Missing provider/model
identity or a missing effective row remains `UNPRICED`.
Cached input is deducted from total input. Reasoning output is deducted from
total output before the corresponding rates are applied.

To configure Claude pricing, add a complete `priceTable` object to the operator
settings document:

```json
{
  "priceTable": {
    "currency": "USD",
    "models": [
      {
        "provider": "claude",
        "model": "claude-sonnet-4-6",
        "inputPerMillionTokens": "3",
        "outputPerMillionTokens": "15",
        "cachedInputPerMillionTokens": "0.30",
        "reasoningOutputPerMillionTokens": "15"
      }
    ]
  }
}
```

The required rates value uncached input and non-reasoning output tokens. The
optional rates value cached input and reasoning output tokens. Rates are exact
non-negative USD decimals per one million tokens.

The example values are operator-authored valuation facts. They are not live
vendor prices, billing records, or billing limits. Costs does not fetch vendor
pricing or query provider billing.

To remove an override, remove its model from `priceTable.models`. Use an empty
`models` array when no operator rows remain. The next costs query falls back to
the built-in row when one exists. A pair without either row becomes `UNPRICED`.

The built-in table records a source URL and an ISO-8601 as-of date for each
shipped row. The current shipped row is `codex/gpt-5-codex`; its source is the
[OpenAI GPT-5-Codex pricing page](https://developers.openai.com/api/docs/models/gpt-5-codex),
dated 2026-08-21.

### Cost Report Fields And Status

The JSON report and every rollup contain `status`, `currency`, nullable exact
decimal `known_cost`, `token_totals`, `unpriced_dispatch_count`, and
deterministically ordered `unpriced_pairs` as separate fields. `token_totals`
always contains `total_tokens`, `input_tokens`, `output_tokens`,
`cached_input_tokens`, and `reasoning_output_tokens`; total tokens is input
plus output and does not double-count the subclasses. Every money field is an
exact decimal string; unknown amounts are null or absent according to the
field contract and are never substituted with zero. The deprecated
`priced_subtotal` remains available for compatibility. Arrays are
deterministic. `coverage` reports encountered, priced, and unpriced rows and
distinct provider/models.

`PRICED` means every usage row is valued, `PARTIAL` means some rows are valued,
`UNPRICED` means usage exists but no row is valued, and `NO_USAGE` means no
canonical usage rows were found. Each `line_items` row retains provider/model
identity and all observed input, cached-input, output, and reasoning-output
counts, plus an actionable `reason` when it is unpriced. Rollups cover all four
dimensions: Work, Worker Session, provider/model, and Factory Session.

Each `PRICED` `line_items` row includes `price_source`. The value is exactly
`BUILT_IN` or `OPERATOR_SUPPLIED`, and identifies the complete row used for
valuation. An `UNPRICED` row omits `price_source` and `priced_amount`. It keeps
its provider/model identity, observed token counts, and actionable `reason`.

Human output repeats this provenance beside each priced line item:

```text
  PRICED provider=CLAUDE model=claude-sonnet-4-6
    Priced amount (USD): 0.0081
    Price source: OPERATOR_SUPPLIED
```

Human output makes coverage explicit: fully priced usage shows the exact
rounded USD amount and token totals, unpriced usage shows `?? unknown` and
never `$0.00`, and mixed usage uses the form `$X.XX + ?? unknown`. Mixed and
unpriced reports also list the unpriced dispatch count and each provider/model
pair, including unknown identities. The command does not fetch live vendor
pricing, query provider billing, or enforce billing limits.

## Related Topics

- `you docs sessions` — discover and inspect Factory Sessions.
- `you docs run` — run a Factory and retain its runtime records.
- `you docs record-replay` — understand recorded runtime artifacts.
