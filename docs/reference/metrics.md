---
author: You Agent Factory Team
last-modified: 2026-08-21
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

## Human Output

Human output includes totals and the requested deterministic breakdown. It
labels `Input tokens`, `Output tokens`, `Completed dispatches`, and `Failures by reason`.

Latency uses milliseconds. The command reports p50 and p95 values when samples exist. It prints `no samples` when a latency value has no samples.

Cost is always `unavailable`. The command does not emit a numeric price.

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
| `cost.availability` | The base metrics cost state. It remains `unavailable`. |
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

### Provider-Owned Price Facts

Costs uses the immutable, Providers-owned price table shipped with the
application. It is not operator configuration, and editing
`~/.you-agent-factory/config.json` does not change cost valuation. Maintainers
add a row only for a provider/model pair observed in live metrics after finding
an authoritative vendor price. Every row records USD-per-million-token rates,
the source URL, and an ISO-8601 as-of date. The current shipped row is
`codex/gpt-5-codex`; its source is the
[OpenAI GPT-5-Codex pricing page](https://developers.openai.com/api/docs/models/gpt-5-codex),
dated 2026-08-21.

Input and output rates are required for every row. Cached-input and
reasoning-output rates are optional; a measured class with no explicit rate is
`UNPRICED`. When a subclass intentionally uses the standard input or output
rate, the provider data declares that equality explicitly. An omitted token
measurement is distinct from an explicit zero, and an explicit zero rate is
valid. Missing provider/model identity or an absent price row remains
`UNPRICED`. Cached input is deducted from total input, and reasoning output is
deducted from total output before the corresponding rates are applied.

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
