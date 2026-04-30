# Factory Workstation Relevant Files

This file inventories the checked-in guidance surfaces that active repository
workstation prompts should cite when they tell operators where to read or write
work inputs.

## Active prompt guidance inventory

| Surface | Role in the workflow | Current contract |
| --- | --- | --- |
| `factory/README.md` | Checked-in workflow overview | Describes the repository-local workflow and canonical inbox directories under `factory/inputs/`. |
| `factory/logs/meta/asks.md` | Canonical customer-ask backlog | The only checked-in maintainer backlog that owns customer asks; backlog edits belong here. |
| `docs/development/root-factory-artifact-contract-inventory.md` | Checked-in artifact inventory | Documents which root-level factory artifacts are checked in, generated, or obsolete. |
| `docs/guides/batch-inputs.md` | Canonical batch request guide | Defines when to author `FACTORY_REQUEST_BATCH` JSON and where those files belong. |
| `factory/inputs/idea/default/` | Standalone idea inbox | Checked-in inbox kept present by `.gitkeep`; standalone idea submissions land here as markdown files. |
| `factory/inputs/BATCH/default/` | Ordered or mixed-work-type request inbox | Canonical placement for `FACTORY_REQUEST_BATCH` JSON when operators need dependency ordering or mixed work types. |
| `factory/workstations/cleaner/AGENTS.md` | Active cleanup workstation prompt | Should cite only checked-in workflow docs and the live idea or batch inboxes. |
| `factory/workstations/ideafy/AGENTS.md` | Active ideation workstation prompt | Should separate standalone idea markdown output from batch JSON output and cite the batch guide when ordering matters. |

## Notes for future iterations

- Treat `factory/inputs/idea/default/` as the live standalone idea inbox, not as a checked-in template catalog; clean checkouts may only contain `.gitkeep`.
- Treat `factory/logs/meta/asks.md` as the only checked-in customer-ask backlog; if another path mentions asks, use this file as the ownership source of truth.
- When prompt instructions need ordered or multi-item follow-up work, point them to `docs/guides/batch-inputs.md` and `factory/inputs/BATCH/default/` instead of overloading the markdown idea inbox.
- Keep workstation prompts repository-local and public-surface neutral: cite checked-in docs or `factory/` paths in this repo, never absolute paths to a different checkout or merge-conflict marker text.
