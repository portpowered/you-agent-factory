# Planned intentional CLI removals and moves

This ledger records production commands and `you run` flags deliberately planned
for removal or relocation during upcoming CLI migrations. It is distinct from the
executable baseline fixtures in this directory; later migration stories should
update those fixtures while citing the matching ledger entry.

Each entry must still exist in today's production command-tree and/or run-flag
baselines. The hermetic ledger test fails if an entry drifts away from the live
contract without an intentional ledger update.

## Planned removals

| Surface | Identifier | Rationale |
| --- | --- | --- |
| `you run` flag | `workflow` | Remove `you run --workflow`; workflow selection uses dedicated workflow commands. |
| command | `you factory save` | Remove `you factory save` from the production CLI. |

## Planned moves

| From | To | Rationale |
| --- | --- | --- |
| `you config expand` | `you factory config expand` | Relocate config expand under factory config. |
| `you config flatten` | `you factory config flatten` | Relocate config flatten under factory config. |
| `you factory validate` | `you factory config validate` | Reparent factory validate under factory config. `you config validate` is not registered today; this entry covers the planned validate relocation from the systematic CLI plan. |
