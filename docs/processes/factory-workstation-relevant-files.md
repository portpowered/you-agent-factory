# Factory Workstation Relevant Files

This file inventories the checked-in guidance surfaces that active repository
workstation prompts should cite when they tell operators where to read or write
work inputs.

## Active prompt guidance inventory

| Surface | Role in the workflow | Current contract |
| --- | --- | --- |
| `factory/README.md` | Checked-in workflow overview | Describes the repository-local workflow and canonical inbox directories under `factory/inputs/`. |
| `factory/logs/meta/asks.md` | Canonical customer-ask backlog | The only checked-in maintainer backlog that owns customer asks; meta and cleaner prompts should read asks here, and `factory/meta/asks.md` remains a redirect-only legacy stub rather than a peer source of truth. |
| `docs/development/root-factory-artifact-contract-inventory.md` | Checked-in artifact inventory | Documents which root-level factory artifacts are checked in, generated, or obsolete. |
| `docs/guides/batch-inputs.md` | Canonical batch request guide | Defines when to author `FACTORY_REQUEST_BATCH` JSON and where those files belong. |
| `factory/inputs/idea/default/` | Standalone idea inbox | Checked-in inbox kept present by `.gitkeep`; standalone idea submissions land here as markdown files. |
| `factory/inputs/task/default/` | Standalone task inbox | Checked-in inbox kept present by `.gitkeep`; standalone task submissions land here as markdown files. |
| `factory/inputs/BATCH/default/` | Ordered or mixed-work-type request inbox | Canonical placement for `FACTORY_REQUEST_BATCH` JSON when operators need dependency ordering or mixed work types. |
| `factory/workstations/cleaner/AGENTS.md` | Active cleanup workstation prompt | Should cite only checked-in workflow docs and the live idea or batch inboxes. |
| `factory/workstations/ideafy/AGENTS.md` | Active ideation workstation prompt | Should default to one standalone idea markdown output and reserve batch JSON output for dependency-ordered or mixed-work-type follow-up. |

## Notes for future iterations

- Treat `factory/inputs/idea/default/` as the live standalone idea inbox, not as a checked-in template catalog; clean checkouts may only contain `.gitkeep`.
- Treat `factory/inputs/task/default/` as the live standalone task inbox, not as a checked-in template catalog; clean checkouts may only contain `.gitkeep`.
- Treat `factory/logs/meta/asks.md` as the only checked-in customer-ask backlog; if another path mentions asks, use this file as the ownership source of truth.
- Before redispatching a checked-in workflow-input markdown file, verify that the lane is not already landed on `main`; stale inbox residue should be treated as cleanup, not as a fresh request.
- When a legacy maintainer path must remain for compatibility, reduce it to a redirect-only stub that names the canonical checked-in surface and carries no duplicated backlog content.
- If a legacy checked-in path remains as a redirect-only stub, classify that stub explicitly in `docs/development/root-factory-artifact-contract-inventory.md` and `internal/testpath/artifact_contract.go` so the redirect contract stays test-enforced.
- If a redirect-only stub protects a canonical maintainer surface, add a content-level regression test for the stub text in `pkg/testutil/artifact_contract_test.go`; inventory classification alone does not prevent drift back into a live duplicate surface.
- When prompt instructions need ordered or multi-item follow-up work, point them to `docs/guides/batch-inputs.md` and `factory/inputs/BATCH/default/` instead of overloading the markdown idea inbox.
- Keep workstation prompts repository-local and public-surface neutral: cite checked-in docs or `factory/` paths in this repo, never absolute paths to a different checkout or merge-conflict marker text.
- When a workstation prompt can emit either ideas or batch requests, state the default as one standalone idea file and name the exact condition that permits `FACTORY_REQUEST_BATCH` output.
- When maintainer prompts need the customer backlog, point them to `factory/logs/meta/asks.md` explicitly and keep any legacy duplicate path as a redirect-only stub rather than a peer control-plane surface.
- Treat workstation `working_directory` values that begin with `/` as canonical portable runtime paths rooted under the active runtime base or factory directory, not as Unix host-absolute paths.
