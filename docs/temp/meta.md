# Current world state

## System architecture and priorities

1. Schema-Driven CLI and Clean CLI UX are accepted complete; CLI continues only
   as package-structure migration (family adapters Factory-terminal; CLI-MOD
   terminal).
2. Backend refactoring is active at live `executor-slot` capacity 16 and
   `concurrent-planners` 1.
3. #1352 batch-ingress starvation is fixed and verified (PR #1424 / `5b832986e`).
4. Keep one semantic owner for Wire/root, OpenAPI/generation, and broad
   functional-test topology. Shared coverage/ownership/package-target manifest
   churn may overlap and is reconciled during each PR's merge loop — **do not
   treat `pkg/wire` as a planner hold gate**.
5. Concrete IMP-PROV adapter groups are closed: IMP-PROV-05 (#1587 /
   `693c366b2`) and IMP-PROV-06 Agy/PTY (#1609 / `c5c9fa190`) Factory-terminal.
   LWR-PROV Factory-terminal (#1625 / `274bc4a92`). Providers HTTP/CLI/MCP
   adapters are all Factory-terminal (#1639 / #1640 / #1638). Providers root on
   tip is `wire/`, `internal/`, `transports/`.
6. Work/Settings HTTP/CLI/MCP adapters are Factory-terminal. CUT-SES-* /
   CUT-VIS-* consumer cuts that were gated on Petri are Factory-terminal.
   CLN-RUN-FOLD-SERVICE and CLN-RUN-FOLD-ENGINE-PIPELINE (#1602 / `a9d50a34b`)
   are Factory-terminal. FUN-automations (#1578), IMP-RUN-04 (#1580),
   FUN-sessions (#1593), FUN-models (#1581), FUN-visualization (#1583),
   FUN-provider-sessions (#1632 / `806ef3022`), CLN-DEF-FOLD (#1579),
   CLN-SET-FOLD (#1592), CLN-WORK-LEGACY (#1597), INV-DEF-INVOCATION-POLICY
   (#1605), PSES vertical INV→CLN→DEL (#1598 / #1616), DEL-RUN-SERVICE
   (`655e4167e`), DEL-RUN-ENGINE-PIPELINE (#1637 / `6e48c875f`),
   CLN-DEF-FOLD-COMPILATION (#1607 / `d1956e624`), WRK-LEGACY (#1599 /
   `bbcb4fef5`), Work vertical INV→CLN→DEL (#1641 / `fdeb6696f`),
   CUT-RUN-REC (#1644 / `87370d39c`), CLI-PROV (#1640 / `83ea40dc6`),
   FUN-runtime (#1645 / `d6e3a57b4`), Providers adapters, collateral DEF
   residual folds (#1606/#1613/#1611/#1610/#1608), DEL-DEF (#1603 /
   `1d4e73fbf`), Settings vertical INV→CLN→DEL (#1646 / `869e22efb`),
   FUN-work (#1650 / `f646bec54`), Recordings vertical INV→CLN→DEL
   (#1647 / `4f64dc9a8`), RET-DEF-ROOT-WIRE (#1651 / `2f4ce21ae`),
   FUN-settings (#1652 / `ec021038c`), FUN-recordings (#1655 / `1d225ba7e`),
   DEL-DEF-RESIDUAL (#1657 / `e8d81d5c6`), WRK-CONTRACT (#1656 /
   `7183487f4`), and DEL-WRK (#1659 / `9562acafa`) are accepted/terminal as
   noted. Origin tip also includes ACP executor providers (#1648 /
   `0d61b69fc`) — outside PSS pause admissions.

## Customer pause (authoritative)

`docs/temp/customer-ask.md` Current state: **pause new admissions; finish only
whatever remains.** Do not submit new idea batches. Leave already-queued and
in-flight remaining work to drain naturally. After the queue drains, keep
holding until the customer re-opens work.

## Scheduling preference — vertical deletion over horizontal construction

Maximize throughput, but when an owner has finished IMP + LWR + its owner-local
adapters and still has transitional root shape debt, **prefer a vertical
completion chunk** for that owner over admitting more horizontal construction
elsewhere. **Superseded while customer pause is active** — no new vertical
admissions.

### Mandatory shape of every vertical completion chunk

1. **`INV-<OWNER>-TOPLEVEL`**
2. **`CUT-*` consumer edges**
3. **`CLN-*-FOLD-*` / `CLN-*-LEGACY-*` / `CLN-*-CONTRACT-ROOTS`**
4. **`DEL-*`** — prefer subpackage DEL slices (Runtime SERVICE then ENGINE)
5. **`FUN-*` public-process proof**

Do not invent horizontal adapters for owners that are already adapter-complete.

## Ideafy / meta-planner payload rigor (mandatory)

Every submitted idea payload MUST follow `factory/docs/idea-payload-template.md`
(`ask`, `systemShapeRequirements`, `testCoverageRequirements`,
`acceptanceCriteria`, `outOfScope`, `antiPatterns`, `currentProgress`,
long-form `requestedOutcome`, `changedPathLease`). Include the full delivery
contract (CI green, blocking comments addressed, conflicts reconciled, PR
merged, then Factory complete).

## Living service-root shape inventory

Canonical product-service root directories are only `wire/`, `internal/`, and
`transports/` plus a thin contract file set at the service root.

### Factory Definitions (post DEL-DEF + RET-DEF + DEL-DEF-RESIDUAL)

DEL-DEF Factory-terminal (#1603 / `1d4e73fbf`). RET-DEF-ROOT-WIRE
Factory-terminal (#1651 / `2f4ce21ae`): tip `pkg/wire` constructs Definitions
through `factory_definitions/wire` only. DEL-DEF-RESIDUAL Factory-terminal
(#1657 / `e8d81d5c6`): residual emptied top-level packages deleted. Public root
on tip still has unexpected transitional siblings
`clonetests/`, `definition/`, `service/`, `systeminitializationtests/` plus
`wire/`/`internal/`/`transports/` — **not** wire/internal/transports-clean.
Functional proofs exist under `tests/functional/factory_definitions/transports`
(CLI named_lifecycle / validate_persist / yaml_parity) but not yet a
root_composition FUN packet. FUN-definitions gated on clean root **and**
customer pause — do not admit.

### Provider Sessions (post FUN)

On tip after FUN-provider-sessions (#1632 / `806ef3022`), public
`provider_sessions` children are `wire/`, `internal/`, `transports/`. Functional
proofs live under `tests/functional/provider_sessions`. No further PSES vertical
packet required.

### Factory Runtime residual debt (post FUN + CUT-RUN-REC)

On tip after FUN-runtime (#1645 / `d6e3a57b4`) and CUT-RUN-REC (#1644 /
`87370d39c`) Factory-complete, public `factory_runtime` directories are `wire/`,
`internal/`, `transports/`, plus residual unexpected `testdata/`. Functional
suite under `tests/functional/factory_runtime`. No further Runtime FUN/CUT
packet required at current tip shape.

### Work residual (post DEL-WORK + FUN-work)

On tip after DEL-WORK (#1641 / `fdeb6696f`) and FUN-work (#1650 /
`f646bec54`), public `pkg/services/work` children are `wire/`, `internal/`,
`transports/`, plus residual unexpected `testdata/`. Functional suite under
`tests/functional/work`. No further Work FUN packet required.

### Workers residual (post INV→CLN→DEL-WRK Factory-terminal)

WRK-LEGACY Factory-terminal (#1599 / `bbcb4fef5`); WRK-CONTRACT Factory-terminal
(#1656 / `7183487f4`); DEL-WRK Factory-terminal (#1659 / `9562acafa`):
idea/plan/task/review all `complete`. Tip deletes emptied transitional shims
(construction/diagnostics/draftvalidation/execution/executor/inferencefailure/
interface/services agents+inference/skippermissions/workstationpool). Public
Workers directories on tip still include unexpected transitional siblings
`agypty/`, `cliprovider/`, `envdiagnostics/`, `executor/`, `invocation/`,
`process/`, `prompting/`, `provider/`, `provider_test/`, `runner/`, `service/`,
`services/`, `worktree/` plus `wire/`/`internal/`; **no `transports/` yet**.
Do **not** treat build-beside `internal/` as migration complete. FUN-workers
dual-gated on clean canonical root **and** customer re-open — do not admit
under pause. Existing functional proofs under `tests/functional/workers`
(agent/inference/mock/script/transports CLI) are not a FUN-workers substitute.

### Providers residual (post LWR + HTTP/CLI/MCP adapters)

On tip after HTTP-PROV (#1639), CLI-PROV (#1640), MCP-PROV (#1638), Providers
public children are `wire/`, `internal/`, `transports/`. Adapter-complete; do
not invent further horizontal adapters. Tip also carries ACP executor provider
integration (#1648) under that shape.

### Settings (post DEL-SET + FUN-settings)

DEL-SET Factory-terminal (#1646 / `869e22efb`). FUN-settings Factory-terminal
(#1652 / `ec021038c`). Public `operator_settings` directories are `wire/`,
`internal/`, `transports/`, plus residual unexpected `testdata/` and thin root
contracts. Functional proofs under `tests/functional/operator_settings`. No
further Settings FUN packet required.

### Recordings (post DEL-REC + FUN-recordings)

DEL-REC Factory-terminal (#1647 / `4f64dc9a8`). FUN-recordings
Factory-terminal (#1655 / `1d225ba7e`): idea/plan/task/review all
`complete`. Public `recordings` directories are exactly `wire/`, `internal/`,
`transports/` plus thin root contracts. Public-process proofs under
`tests/functional/recordings` (root_composition + peer boundary). No further
Recordings FUN packet required.

### Automations / Factory Sessions

Both public roots are `wire/`, `internal/`, `transports/` on tip. DEL-AUTO and
FUN-automations Factory-terminal; Automations-leased root-wire hold cleared.

### Active vertical chunks

| Request | Contents | Notes |
|---|---|---|
| `planner-wave-fun-recordings-20260728` | FUN-recordings **Factory-terminal** (#1655 / `1d225ba7e`) | tip inventory confirmed |
| `planner-wave-fun-settings-20260728` | FUN-settings **Factory-terminal** (#1652 / `ec021038c`) | inventory confirmed |
| `planner-wave-fun-work-20260728` | FUN-work **Factory-terminal** (#1650) | |
| `planner-wave-def-migration-repair-20260728` | INV→IMP-R→CUT→BOOT→CLN→DEL-DEF **Factory-terminal** | |
| `planner-wave-def-residual-folds-20260728` | INV + residual folds + DEL-DEF-RESIDUAL **Factory-terminal** (#1657 / `e8d81d5c6`) | tip DEF still multi-sibling |
| `planner-wave-ret-def-root-wire-20260728` | RET-DEF-ROOT-WIRE **Factory-terminal** (#1651 / `2f4ce21ae`) | wire forbid verified |
| `planner-wave-set-toplevel-cleanup-20260728` | INV-SET→CLN-SET→DEL-SET **Factory-terminal** (#1646) | |
| `planner-wave-wrk-toplevel-cleanup-20260728` | INV→CLN-FOLD→CLN-LEGACY→WRK-CONTRACT→DEL-WRK **Factory-terminal** (#1659 / `9562acafa`) | Workers still multi-sibling; no transports/; hold FUN under pause |
| `planner-wave-fun-run-cut-run-rec-20260728` | FUN-runtime + CUT-RUN-REC **Factory-terminal** | |
| `planner-wave-pses-toplevel-cleanup-20260728` | INV→CLN→DEL-PSES **terminal** (#1616) | |
| `planner-wave-fun-pses-20260728` | FUN-provider-sessions **terminal** (#1632) | |
| `planner-wave-prov-adapters-20260728` | HTTP/CLI/MCP-PROV **terminal** | |

## Operational notes

- Session `~default` ACTIVE. Local `main` at origin tip `9562acafa`
  (#1659 DEL-WRK). Planner-owned `docs/temp/**` dirty; expected.
- This pass (`work-thoughts-546` cron:though-retrigger): honor customer pause;
  state-type filters show only this cron PROCESSING + superseded failed cron
  `work-thoughts-151`; tip inventories reconfirmed; submit **no** new idea
  batch.
- Remaining live ~0/16 executor tasks. Free ~16 unused under pause.
- Hold IMP-WRK-07 / PSS-I0* and all new FUN/CLN/DEL admissions under customer
  pause. Do not admit FUN-workers|definitions — pause forbids new admissions;
  Definitions root still not wire/internal/transports-clean; Workers still
  transitional (unexpected top-level packages remain; no `transports/`).
- Recordings / PSES already wire/internal/transports + FUN-terminal; no further
  fold→DEL packet.
- Next residual owner debt after pause re-open: Workers residual CLN/transports
  path (or FUN only after clean root), then Definitions residual clean-root /
  FUN-definitions. Do not invent short packets.
- Failed priority Work otherwise: superseded cron `work-thoughts-151` only.
- Open non-PSS PRs (#1658/#1654/#1653/#1202) are outside pause admissions.
- Do not revive monolithic `pss-del-run` or deleted Work/Settings/Recordings
  transitional packages.
- Do not invent WRK protocol adapters, DEF horizontal adapters, or reopen
  accepted CLI UX.
- No deliberate `you work move` this pass.

# Progressive change notes

- Cron ideafy (2026-07-28T21:00-07:00): honor customer pause; tip unchanged
  `9562acafa`; queue drained (only this cron + failed cron-151 non-complete);
  Workers still multi-sibling (no transports/); DEF still unclean; submit
  **no** new batch.
- Cron ideafy (2026-07-28T20:00-07:00): honor customer pause; tip unchanged
  `9562acafa`; queue drained (only this cron + failed cron-151 non-complete);
  Workers still multi-sibling (no transports/); DEF still unclean; submit
  **no** new batch.
- Cron ideafy (2026-07-28T19:00-07:00): honor customer pause; tip unchanged
  `9562acafa`; queue drained (only this cron + failed cron-151 non-complete);
  Workers still multi-sibling (no transports/); DEF still unclean; submit
  **no** new batch.
- Cron ideafy (2026-07-28T18:00-07:00): honor customer pause; tip unchanged
  `9562acafa`; queue drained (only this cron + failed cron-151 non-complete);
  Workers still multi-sibling (no transports/); DEF still unclean; submit
  **no** new batch.
- Cron ideafy (2026-07-28T17:01-07:00): honor customer pause; tip unchanged
  `9562acafa`; full queue drained (720 complete except this cron + failed
  cron-151); Workers still multi-sibling (no transports/); DEF still unclean;
  submit **no** new batch.
- Cron ideafy (2026-07-28T16:01-07:00): honor customer pause; tip unchanged
  `9562acafa`; full queue drained (719 complete except this cron + failed
  cron-151); Workers still multi-sibling (no transports/); DEF still unclean;
  submit **no** new batch.
- Cron ideafy (2026-07-28T15:00-07:00): honor customer pause; tip unchanged
  `9562acafa`; full queue drained (718 complete except this cron + failed
  cron-151); Workers still multi-sibling (no transports/); DEF still unclean;
  submit **no** new batch.
- Cron ideafy (2026-07-28T14:01-07:00): honor customer pause; tip unchanged
  `9562acafa`; queue drained ~0/16; Workers still multi-sibling (no
  transports/); DEF still unclean; submit **no** new batch.
- WRK toplevel loopback (2026-07-28T13:25-07:00): accept DEL-WRK Factory-
  terminal (#1659 / `9562acafa`); tip Workers still multi-sibling transitional
  (no transports/); Recordings/PSES already clean+FUN-terminal; customer pause
  → submit **no** new batch; hold FUN-workers|definitions.
- Cron ideafy (2026-07-28T13:01-07:00): honor customer pause; tip still
  `7183487f4`; DEL-WRK remaining with active `pss-del-wrk` worktree commits
  (no open PR yet); no recoverable stranded Work; submit **no** new batch.
- Cron ideafy (2026-07-28T12:01-07:00): accept WRK-CONTRACT Factory-terminal
  (#1656 / `7183487f4`); tip Workers still multi-sibling transitional (no
  transports/); remaining DEL-WRK `work-task-535` `task:init`; customer pause
  → submit **no** new batch; leave DEL-WRK drain.
- DEF residual loopback (2026-07-28T11:15-07:00): accept DEL-DEF-RESIDUAL
  Factory-terminal (#1657 / `e8d81d5c6`); tip DEF still has
  clonetests/definition/service/systeminitializationtests + wire/internal/
  transports — not clean; customer pause overrides FUN-definitions preference;
  leave WRK-CONTRACT #1656 / queued DEL-WRK; submit **no** new batch.
- Cron ideafy (2026-07-28T11:01-07:00): honor customer pause; FF main
  `1d225ba7e`→`0d61b69fc` (#1648 ACP landed on tip); no recoverable stranded
  Work; leave WRK-CONTRACT #1656 / DEL-DEF-RESIDUAL #1657 / queued DEL-WRK;
  submit **no** new batch.
- FUN-recordings loopback (2026-07-28T10:05-07:00): accept FUN-recordings
  Factory-terminal (#1655 / `1d225ba7e`); tip recordings =
  wire/internal/transports + `tests/functional/recordings`; honor customer
  pause over loopback refill preference; hold all new admissions; leave
  WRK-CONTRACT #1656 / DEL-DEF-RESIDUAL #1657 / queued DEL-WRK to drain.
- Cron ideafy / customer pause (2026-07-28T10:03-07:00): honor
  `customer-ask.md` pause; accept FUN-recordings GitHub-merged (#1655 /
  `1d225ba7e`); tip recordings = wire/internal/transports +
  `tests/functional/recordings`; hold all new admissions; leave
  WRK-CONTRACT #1656 / DEL-DEF-RESIDUAL #1657 / FUN Factory consume / queued
  DEL-WRK to drain.
- FUN-settings loopback (2026-07-28T10:00-07:00): accept FUN-settings
  Factory-terminal (#1652 / `ec021038c`); hold refill.
- RET-DEF loopback (2026-07-28T09:06-07:00): accept RET-DEF-ROOT-WIRE
  Factory-terminal (#1651 / `2f4ce21ae`); hold refill.
- Cron ideafy (2026-07-28T09:02-07:00): preferred ready set empty → hold.
- REC toplevel loopback (2026-07-28T08:53-07:00): accept DEL-REC; admit
  FUN-recordings.
- FUN-work loopback (2026-07-28T08:38-07:00): accept FUN-work; hold refill.
- SET toplevel loopback (2026-07-28T08:17-07:00): accept DEL-SET; admit
  FUN-settings.
- Policy: no planner-level `pkg/wire` hold; universal merge-before-complete
  delivery contract; customer pause overrides refill preference.
