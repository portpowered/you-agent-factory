# Intentional CLI removals and moves

This ledger records production commands deliberately moved or removed by the
CLI command-shape migration. It is distinct from the executable baseline
fixtures in this directory. The completed entries are historical migration
evidence; they do not keep an old command callable.

The `planned_*` fields remain empty after reconciliation. The machine-readable
`completed_*` fields and the tables below record the exact cutover.

## Completed removals

| Surface | Identifier | Rationale |
| --- | --- | --- |
| command | `you session dispatches` | Retired the low-value Session dispatch inventory without a CLI successor; REST and MCP dispatch reads remain available. |

## Completed moves

| From | To | Rationale |
| --- | --- | --- |
| `you factory query` | `you factory show` | Renamed the Current Factory read to the canonical single-resource show verb without changing its operation. |
| `you work visualize` | `you work render` | Renamed the local Work dependency transformation to the precise render verb without changing its output or side effects. |

The CLI compatibility command manifest (`contracts/cli/deprecated-commands.json`)
and CLI compatibility inventory (`contracts/cli/deprecated.json`) remain empty
by design. These changes are breaking path changes, not compatibility aliases.
