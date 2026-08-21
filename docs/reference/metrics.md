---
author: You Agent Factory Team
last-modified: 2026-08-21
doc-id: agent-factory/metrics
---

# Metrics

`you docs metrics` describes `you metrics`, which reports recorded Factory Runtime metrics. Use it to inspect token usage, dispatch completion, failure reasons, and available latency.

## Run The Command

Run `you metrics` to report metrics from all Factory Sessions. The default grouping is `workstation`.

```bash
you metrics
you metrics --group-by workstation
you metrics --group-by worker
you metrics --group-by provider
you metrics --group-by provider --session session-123
you metrics --json
you metrics --json --group-by worker --session session-123
```

The `--group-by` flag accepts these values:

- `workstation` groups results by workstation. This is the default.
- `worker` groups results by worker.
- `provider` groups results by provider.

An unsupported value returns an error before the command queries metrics.

## Scope One Factory Session

Use `--session <id>` with the exact Factory Session identifier. The filter applies to totals, groups, failure reasons, and latency samples.

Without `--session`, the command reports all recorded Factory Sessions.

## Human Output

Human output includes totals and the requested deterministic breakdown. It labels input and output token counts, completed dispatch counts, and failure counts by reason.

Latency uses milliseconds. The command reports p50 and p95 values when samples exist. It prints `no samples` when a latency value has no samples.

Cost is always `unavailable`. The command does not emit a numeric price.

Example:

```text
Scope: Factory Session session-123
Group by: provider
Cost: unavailable

Totals:
  Input tokens: 1200
  Output tokens: 450
  Completed dispatches: 8
  Failures by reason:
    timeout: 1
  Dispatch latency (milliseconds): p50=820, p95=1400, samples=8
  Provider latency (milliseconds): p50=790, p95=1320, samples=8

Breakdown by provider: 1 rows
  codex:
    Input tokens: 1200
    Output tokens: 450
    Completed dispatches: 8
```

## JSON Output

Use `--json` for one machine-readable JSON document on stdout. The document names its scope, grouping, units, cost availability, totals, groups, failure reasons, and latency percentiles.

The `groups` array contains only the requested grouping. Groups are sorted by key for deterministic output. Missing latency samples use `samples: 0`, `p50: null`, and `p95: null`.

```json
{
  "scope": {
    "kind": "factory_session",
    "factory_session_id": "session-123"
  },
  "group_by": "provider",
  "units": {
    "tokens": "tokens",
    "counts": "count",
    "latency": "milliseconds"
  },
  "cost": {
    "availability": "unavailable"
  },
  "totals": {
    "input_tokens": 1200,
    "output_tokens": 450,
    "completed_dispatches": 8,
    "failures_by_reason": {
      "timeout": 1
    },
    "dispatch_latency": {
      "unit": "milliseconds",
      "samples": 8,
      "p50": 820,
      "p95": 1400
    },
    "provider_latency": {
      "unit": "milliseconds",
      "samples": 8,
      "p50": 790,
      "p95": 1320
    }
  },
  "groups": [
    {
      "key": "codex",
      "aggregate": {
        "input_tokens": 1200,
        "output_tokens": 450,
        "completed_dispatches": 8,
        "failures_by_reason": {
          "timeout": 1
        },
        "dispatch_latency": {
          "unit": "milliseconds",
          "samples": 8,
          "p50": 820,
          "p95": 1400
        },
        "provider_latency": {
          "unit": "milliseconds",
          "samples": 8,
          "p50": 790,
          "p95": 1320
        }
      }
    }
  ]
}
```

An unscoped JSON result uses `"kind": "all_factory_sessions"` and a null `factory_session_id`. Empty results use zero counts, empty failure maps, an empty groups array, and null percentile values.

## Runtime Cost Rollups

Use `you metrics costs` to inspect exact cost rollups through the running
Factory API. This is a separate command from the local `you metrics` artifact
reader:

```bash
you metrics costs
you metrics costs --session session-123
you --server http://localhost:7437 metrics costs --json
```

The optional `--session` filter selects one Factory Session. Global `--server`
selects the Factory API; global `--json` emits the API-shaped report. A route
failure returns an error without a partial success document.

### Configure The Price Table

The operator price table lives in `~/.you-agent-factory/config.json` under
`priceTable`. Rates are USD per one million tokens and must be decimal strings:

```json
{
  "priceTable": {
    "currency": "USD",
    "models": [
      {
        "provider": "CODEX",
        "model": "gpt-5",
        "inputPerMillionTokens": "2.5",
        "cachedInputPerMillionTokens": "0.5",
        "outputPerMillionTokens": "5.25",
        "reasoningOutputPerMillionTokens": "10"
      }
    ]
  }
}
```

Input and output rates are required for every table entry. Cached-input and
reasoning-output rates are optional, but a measured token class with no rate
is `UNPRICED`. An omitted token measurement is distinct from an explicit zero;
an explicit zero rate is valid and produces an exact `"0"` amount. Missing
provider/model identity or an absent model entry also remains `UNPRICED`.
Cached input is deducted from total input, and reasoning output is deducted
from total output before the corresponding rates are applied.

### Cost Report Fields And Status

The JSON report contains `scope`, `currency`, `status`, `priced_subtotal`,
`coverage`, `line_items`, `work_items`, `worker_sessions`, `provider_models`,
and `factory_sessions`. Every money field is an exact decimal string; absent
amounts are omitted rather than reported as zero. Arrays are deterministic.
`coverage` reports `encountered_rows`, `priced_rows`, `unpriced_rows`, and the
matching encountered/priced/unpriced distinct `provider_models` counts.

`PRICED` means every usage row is valued, `PARTIAL` means some rows are valued,
`UNPRICED` means usage exists but no row is valued, and `NO_USAGE` means no
canonical usage rows were found. Each `line_items` row retains provider/model
identity and all observed input, cached-input, output, and reasoning-output
counts, plus an actionable `reason` when it is unpriced. Rollups cover all four
dimensions: Work, Worker Session, provider/model, and Factory Session.

The table is operator-authored reporting configuration. The command does not
fetch live vendor pricing, query provider billing, or enforce billing limits.

## Related Topics

- `you docs sessions` — discover and inspect Factory Sessions.
- `you docs run` — run a Factory and retain its runtime records.
- `you docs record-replay` — understand recorded runtime artifacts.
