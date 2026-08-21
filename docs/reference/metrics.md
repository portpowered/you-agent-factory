---
author: You Agent Factory Team
last-modified: 2026-08-20
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

## Related Topics

- `you docs sessions` — discover and inspect Factory Sessions.
- `you docs run` — run a Factory and retain its runtime records.
- `you docs record-replay` — understand recorded runtime artifacts.
