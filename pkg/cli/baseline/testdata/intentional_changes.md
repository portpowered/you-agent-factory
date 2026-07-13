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
| _none_ | _none_ | `you run --workflow` removed; workflow selection uses dedicated workflow commands. |

## Planned moves

| From | To | Rationale |
| --- | --- | --- |
| _none_ | _none_ | Factory config validate, flatten, and expand now live under `you factory config`. |
