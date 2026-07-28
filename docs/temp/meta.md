# Current world state

## System architecture and priorities

1. Schema-Driven CLI and Clean CLI UX are accepted complete; CLI continues only
   as package-structure migration (CLI-MOD still live).
2. Backend refactoring is active at live `executor-slot` capacity 16 and
   `concurrent-planners` 1.
3. #1352 batch-ingress starvation is fixed and verified (PR #1424 / `5b832986e`).
4. Keep one semantic owner for Wire/root, OpenAPI/generation, and broad
   functional-test topology. Shared coverage/ownership/package-target manifest
   churn may overlap and is reconciled during each PR's merge loop — **do not
   treat `pkg/wire` as a planner hold gate**.
5. Serialize concrete provider adapter groups: IMP-PROV-03 terminal; IMP-PROV-04
   Cursor/OpenCode live (`work-task-304`). Hold IMP-PROV-05/06 and LWR-PROV
   until earlier groups finish. Hold IMP-WRK-07 while hosted_logic remains
   polling-only. **IMP-RUN-04 is dependency-ready once DEC-RUN-REC-DURABILITY
   is Factory-complete** — cite
   [`dec-run-rec-durability.md`](projects/packaged-service-structure/dec-run-rec-durability.md);
   durability ownership is no longer an open verbal hold.
6. Work HTTP/CLI/MCP adapters are Factory-terminal. Settings HTTP/CLI/MCP are
   Factory-terminal (CLI-SET #1521 / `fdefd3dc7`). CUT-SES-WRK is Factory-
   terminal (#1555). CLN-RUN-PETRI-ROOT is Factory-terminal (#1523).

## Scheduling preference — vertical deletion over horizontal construction

Maximize throughput, but when an owner has finished IMP + LWR + its owner-local
adapters and still has transitional root shape debt, **prefer a vertical
completion chunk** for that owner over admitting more horizontal construction
elsewhere.

### Mandatory shape of every vertical completion chunk

Every vertical owner chunk MUST include all of the following, in order, unless
a prior accepted packet already proved that step complete against the live tree
(not merely against an old checklist checkbox):

1. **`INV-<OWNER>-TOPLEVEL`** — inventory every live top-level child under
   `pkg/services/<owner>/` and every root-level contract surface that is not the
   committed thin root contract. Remap unexpected directories and excess
   contract roots to `move`/`delete` with explicit destinations. Regenerate
   package-target + ownership inventories. Prove with generator/unit tests.
2. **`CUT-*` consumer edges** for that owner (only the edges that still reach
   non-root or transitional surfaces).
3. **`CLN-*-FOLD-*` / `CLN-*-LEGACY-*`** — fold transitional top-level packages
   (especially `service/`, plus every other unexpected sibling and excess root
   `.go` contract clusters) into `owner/internal` or
   `owner/internal/services/<subservice>`. Retarget owner `wire/` (and root
   `pkg/wire` construction for that owner when needed) so production callers
   stop importing transitional paths. **Do not hold this step waiting for
   another owner to finish `pkg/wire`.**
4. **`DEL-*`** — delete emptied transitional paths; lower
   structure/ownership/package-target/coverage baselines so the debt cannot
   return as `retain`. **Prefer subpackage DEL slices over one monolithic DEL
   that absorbs many moves** (especially Runtime).
5. **`FUN-*`** — public-process proof before marking the owner track complete.

Do not invent horizontal adapters for owners that are already adapter-complete.

## Ideafy / meta-planner payload rigor (mandatory)

Every submitted idea payload MUST include: `title`, `packetId`,
`currentProgress`, long-form `requestedOutcome`, `changedPathLease`,
behavioral `acceptanceCriteria` (5–12 bullets, ≥2 behavioral), and
`antiPatterns` for migration/cleanup packets. Include the full delivery
contract (CI green, blocking comments addressed, conflicts reconciled, PR
merged, then Factory complete).

## Living service-root shape inventory

Canonical product-service root directories are only `wire/`, `internal/`, and
`transports/` plus a thin contract file set at the service root.

### Active vertical chunks

| Request | Contents | Notes |
|---|---|---|
| `planner-wave-auto-fold-cut-del-20260727` | CUT-AUTO + CLN-AUTO-FOLD **terminal**; DEL-AUTO `task:init` | FUN-automations after DEL |
| `planner-wave-run-vertical-fold-20260727` | Petri CLN + CUT-SES-RUN + CUT-RUN-WRK **terminal**; CLN-RUN-FOLD-SERVICE live; ENGINE/DEL blocked | **split DEL-RUN before it starts** |
| `planner-wave-def-migration-repair-20260728` | INV/CLN-DEF-CONTRACTS + IMP-DEF-02R/03R/05R **terminal**; CUT-DEF-SES + IMP-DEF-06R live | DEF repair in flight |
| `planner-wave-set-cuts-20260728` | CUT-SET-PROV + BOOT-SET task PROCESSING | residual SET fold after CUTs |
| `planner-wave-ses-work-rec-vis-cuts-20260727` | CUT-WORK-REC **terminal**; CUT-SES-WORK + CUT-VIS-REC live | |
| `planner-wave-imp-prov-04-20260727` | IMP-PROV-04 `task:init` | unlock PROV-05 after terminal |
| `planner-wave-lwr-set-mod-adapters-20260727` | LWR-SET + HTTP/MCP-MOD terminal; CLI-MOD live | |
| `planner-wave-wrk-toplevel-cleanup-20260728` | INV-WRK task PROCESSING; CLN/DEL blocked | |
| `planner-wave-rec-toplevel-cleanup-20260728` | INV-REC **terminal**; CLN-REC-FOLD `task:init` after dirty-root repair | |
| `planner-wave-pses-toplevel-cleanup-20260728` | INV-PSES task PROCESSING; CLN/DEL blocked | |
| `planner-wave-vis-run-durability-nested-20260728` | CUT-VIS-RUN + DEC-RUN-REC-DURABILITY + INV-NESTED-OWNER-MOVE-RULES live | submitted this pass |

## Operational notes

- Session `~default` ACTIVE. Root fast-forwarded to `origin/main` `83d6f79ac`
  (includes Petri CLN #1523 + INV-REC #1564).
- Repaired `work-plan-332` CLN-REC-FOLD `failed→init→complete`; `work-task-336`
  at `task:init`.
- Hold CUT-RUN-REC while Runtime CLN-RUN-FOLD-SERVICE/ENGINE own
  `factory_runtime` implementation leases.
- Hold Settings residual INV→fold until CUT-SET-PROV/BOOT-SET terminal.
- Hold FUN-automations until DEL-AUTO Factory complete.
- Hold IMP-PROV-05/06 / LWR-PROV / IMP-WRK-07 / PSS-I0*.
- **IMP-RUN-04:** dependency-ready after DEC-RUN-REC-DURABILITY Factory-complete;
  prefer admitting IMP-RUN-04 when CTR-RUN/CTR-REC remain terminal and capacity
  exists. Implementation is a separate packet — not shipped by the decision packet.
- Before monolithic `pss-del-run` starts: replace with subpackage DEL slices.
- Failed priority Work: superseded cron `work-thoughts-151` only.
- `docs/temp/**` is planner-owned and ignored except durable PSS/FT expansion paths.

# Progressive change notes

- DEC-RUN-REC-DURABILITY (in flight): plan/checklist/meta retarget IMP-RUN-04 from
  durability-open hold to dependency-ready after decision Factory-complete.
- Ideafy pass (00:05 local): FF main; repair CLN-REC-FOLD dirty-root setup;
  accept Petri CLN + INV-REC + CUT-WORK-REC terminals; submit CUT-VIS-RUN +
  DEC-RUN-REC-DURABILITY + INV-NESTED-OWNER-MOVE-RULES; hold CUT-RUN-REC while
  Runtime folds live; record DEL-RUN split obligation.
- Prior: CUT-SES-WRK / Settings adapters / Recordings+PSES INV verticals live.
- Policy: no planner-level `pkg/wire` hold; universal merge-before-complete
  delivery contract.
