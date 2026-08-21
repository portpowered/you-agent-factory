# CLI command-shape path exceptions

This breaking CLI migration standardizes three low-level command paths. The
old spellings are not compatibility aliases and now fail as unknown commands.

## Old → new command map

| Retired path | Target or disposition |
| --- | --- |
| `you factory query` | `you factory show` |
| `you work visualize` | `you work render` |
| `you session dispatches` | Removed without a CLI successor |

The two renamed commands retain their existing arguments, flags, output,
diagnostics, side effects, and exit behavior. The removed Session inventory does
not change Factory Session REST/MCP dispatch reads or their service contracts.

For dispatch inspection, use `you session show SESSION_ID` for status and
summary, scoped `you metrics --session SESSION_ID --group-by workstation|worker|provider`
for aggregates, the existing REST/MCP dispatch read for exact durable records,
and `you worker-sessions list --work-id WORK_ID` for Work-specific drill-down.
