## Audit result

The decomposition is structurally far from complete. Twelve of the 13 product-service roots still violate at least one packaged-service rule. The active Providers/Workers package-target, ownership, and boundary ledgers have since been reconciled with the live refactor; the broader service-shape debt remains.

The governing rule requires exactly one named interface per service/subservice root, no exported root functions, and only `internal`, `wire`, and `transports` child directories ([general-backend-standards.md](C:/Users/andre/work/portos/infinite-you/docs/internal/standards/code/general-backend-standards.md:144)).

This note was originally captured before the Providers/Workers inventory repair and was updated on 2026-07-31 UTC to record that repair, the System Initialization and Provider Sessions contract seals, and the remaining scope.

## 1. Product services still incomplete

These are live counts from production `.go` files, not just the stale baseline:

| Service | Root interfaces | Root exported functions | Noncanonical root directories | Status |
|---|---:|---:|---|---|
| `automations` | 1 | 0 | — | Root contract sealed; nested subservice debt remains |
| `factory_definitions` | 31 | 19 | `clonetests`, `definition`, `systeminitializationtests` | Major decomposition remaining |
| `factory_runtime` | 36 | 50 | `testdata` | Major decomposition remaining |
| `factory_sessions` | 4 | 13 | — | Contract/transport consolidation remaining |
| `factory_visualization` | 4 | 20 | — | Contract not sealed |
| `models` | 15 | 18 | — | Contract not sealed |
| `operator_settings` | 4 | 28 | `testdata` | Root and document implementation debt |
| `provider_sessions` | 1 | 0 | — | Contract sealed |
| `providers` | 3 | 3 | `inference` | Active refactor is incomplete |
| `recordings` | 16 | 19 | — | Root directory cleaned, contract still broad |
| `system_initialization` | 1 | 0 | — | Contract sealed |
| `work` | 19 | 138 | `testdata` | Implementation moved, public root still extremely broad |
| `workers` | 24 | 106 | — | Top-level directories largely cleaned, contract remains broad |

`pkg/services/edges` is the documented architecture exception, but it currently has no service interface and an exported `Merge` helper. It should be addressed under FND-06 rather than treated as an ordinary product service.

The structure checker currently reports:

- 21 new violations.
- 20 stale baseline entries.
- Approximately 362 live service-shape violations after accounting for that drift:
  - 249 exported-function violations.
  - 24 interface-count violations.
  - 89 unexpected-directory violations.
- The committed baseline itself contains 611 total entries, including legacy functional-test debt ([package-structure-baseline.json](C:/Users/andre/work/portos/infinite-you/docs/internal/baselines/package-structure-baseline.json:1261)).

## 2. Remaining unconverted implementation clusters

### Factory Definitions

Legacy/public packages still needing removal or privatization:

- `definition`
- `clonetests`
- `systeminitializationtests`
- `internal/testcomposition`

Noncanonical subservice children:

- `authoring_layout`: `authoredlayout`, `expand`, `flatten`, `persist`, `prepare`
- `catalog`: `namedfactories`, `namedpaths`, `persistence`, `resource`
- `compilation`: `canonical`, `loadedsource`, `loading`, `runtimeconfig`, `runtimetests`
- `distribution`: `goal`, `packageassets`, `packagedcatalog`, `packagedinstallation`, `promptassets`, `review`, `scaffoldfacts`, `subagent`, `tts`
- `invocation_policy`: `decisionenvelope`, `invocationinterpolation`, `invocationoutput`, `invocationworktype`, `quorumpolicy`, `ttsobservability`, `workpropagation`, `workstationexecution`
- `snapshots_portability`: `capture`, `editable`, `materialize`, `portableconfig`, `prepare`, `replayconfig`
- `validation`: `authoredmodel`, `impl`

These should either become real nested services under `internal/services/<parent>/internal/services/<child>` or ordinary private implementation under `<parent>/internal`, depending on whether they own an independent service contract.

### Factory Runtime

The target manifest still calls for moving:

- `internal/factorystatus`
- `internal/legacysnapshot`
- `internal/rootobservation`
- `internal/service`
- `internal/orchestrators/petri`
- JavaScript `preview`, `runtime`, `source`, and `validation`

Noncanonical subservice children remain under orchestration:

- `context`
- `definitionmapping`
- `engine`
- `javascript`
- `metrics`
- `orchestrationowner`
- `orchestratorcontract`
- `replayhooks`
- `runtime`
- `runtimecontract`
- `scheduler`
- `state`
- `subsystems`
- `throttle`
- `token`
- `token_transformer`
- `tooling`

Additional contract violations exist in `checkpoint_recovery` and `orchestration`, each declaring more than one interface.

### Factory Sessions

The old runtime-opening hierarchy remains:

- `internal/runtimeopening`
- `internal/runtimeopening/invocation`
- `internal/runtimeopening/operatordefaults`

These should converge on `internal/services/runtime_opening`.

The service root also exposes `DefinitionActivationGatewayProvider`, `ExecutionService`, `ModelsCLIPresentationCollaborator`, and `Service`; only the canonical `Service` interface should remain.

### Models

Still mapped for movement:

- `internal/catalog` → `internal/services/catalog`
- `internal/host` → `internal/services/runtime_host`
- `internal/inference` → `internal/services/inference`

The `inference` subservice and nested `runtime_host/.../leases` subservice also declare multiple interfaces.

### Operator Settings

Remaining implementation packages:

- `internal/construct`
- `internal/identityinputinventory`
- `internal/service`
- `internal/testlink`
- `internal/testproviders`

Nested debt:

- `document/identityinventory`
- `resolution/defaults`
- `document` still exposes numerous implementation functions.

### Providers

This is the most visibly in-flight area.

The manifest still lists the entire legacy `execution/internal/provider` tree for flattening, including:

- `adapter`, `adapter/testkit`
- `agy`
- `claude`
- `codex`
- `commandenv`
- `conductor`
- `cursor`
- `gemini`
- `inferencecontract`, `inferencecontract/testkit`
- `kiro`
- `opencode`
- `pi`
- `providersroot`
- `registry`
- `structured`
- `agypty`

The live tree has moved again, and the authoritative package-target and ownership
ledgers now reflect that state:

- `providers/internal/services/acp/**` and `providers/internal/services/builtins/**` are recorded under Providers.
- Deleted provider adapter paths and the deleted `workers/cliprovider` path are no longer inventoried.
- The remaining `execution/internal/provider/**` entries are explicit transitional Providers move rows.
- The public `providers/inference` directory remains a structure violation and is deferred to the Providers root-contract packet.

The inventory repair is complete; further provider work is root-contract sealing
and legacy execution-tree flattening, not reclassifying the live package ledger.

### Recordings

The public transitional directories have now been removed, despite the older committed prose still listing them. Remaining nested wrappers are:

- `artifacts_export/artifacts`
- `canonical_ledger/events`
- `projection_query/projections`
- `replay/replay`

`artifacts_export` also declares three interfaces, and `recording_lifecycle` retains an exported constructor.

The root contract is still too broad: 16 named interfaces in `contracts.go` and `portable_recording.go`.

### Work

The old public `service` and `stateaccessrecordings` packages are gone. Remaining structural debt:

- `testdata` at the service root.
- `internal/contenturl`
- `internal/invocationreturnpolicy`
- `internal/requestadmission`
- `internal/service`
- `state_access/lineagegraph`
- `state_access/stateaccessquery`

The implementation folds described in the inventory are substantially complete, but the root still exposes 19 interfaces and a very large helper surface. The inventory’s statement that these are all “thin committed contracts” conflicts with the newer normative one-interface/no-exported-functions rule.

### Workers

The old public directories recorded in the baseline have mostly been deleted; those baseline entries are stale. Remaining structural work is now internal:

- `internal`
- `internal/diagnostics`
- `internal/interface`
- `internal/testhelpers`
- `runners/{agents,inference,process,runner,testing}`
- `runtime_assembly/construction`
- `workstations/{draftvalidation,envdiagnostics,execution,executor,inferencefailure,invocation,poolboundary,prompting,skippermissions,worktree}`

`workstation_pool_boundary_impl.go` remains explicitly documented as a temporary root implementation exception. It should be relocated once a cycle-free bridge exists.

### Automations and Visualization

These do not have significant top-level directory migration debt, but they are not contract-complete:

- Automations subservices `filesystem_watchers` and `script_pollers` expose multiple interfaces; `script_pollers` also exports substantial implementation behavior.
- Visualization’s two projection/lifecycle subservices each expose four interfaces.

## 3. Broken dependency and test contracts

The live checks found these concrete violations:

1. Cross-service implementation imports:

   - The former `pkg/services/edges/definition.go` import of `provider_sessions/wire` has been replaced with local exact effect shapes.
   - The former `pkg/services/operator_settings/wire/register.go` import of `providers/wire` has been moved to the canonical `pkg/wire` composition boundary; Operator Settings now accepts only the Providers root contract.

   The remaining rule is enforced by `ownershipboundarycheck`; no deletion-only baseline entries remain.

2. Petri implementation leakage:

   - `pkg-boundary` rejects a Petri public-surface baseline entry covering  
     `factory_runtime/internal/services/orchestration/definitionmapping/maptests/config_mapper_equivalence_test.go`.

3. Functional test bypass:

   - `tests/functional/providers/acp/daemon_concurrency_test.go` imports `providers/wire`.
   - It should construct through `root.BuildProcess` and substitute effects through `edges.Edges`.

4. Package-target inventory drift:

   - Repaired in this pass; `package-target-manifest-check` now holds with 515 live inventory rows.

5. Ownership inventory drift:

   - Repaired in this pass; `ownership-inventory-check` now holds with the frozen 546-package inventory and two path-lease packets.

6. Legacy functional layout:

   - The baseline retains 41 `tests/functional/runtime_api` files.
   - It also records 66 deprecated runtime API tests, 100 tests missing a domain subsection, and 44 unclassified functional-test domains.

7. Integration packets remain blocked:

   - PSS-I01: root/Wire/process composition.
   - PSS-I02: HTTP composition.
   - PSS-I03: CLI composition.
   - PSS-I04: MCP composition.
   - PSS-I05: event-backbone convergence.

   This is reflected directly in the packet ledger ([path-lease-packet-manifest.json](C:/Users/andre/work/portos/infinite-you/docs/internal/projects/packaged-service-structure/path-lease-packet-manifest.json:144)).

## 4. Logic still misplaced in mapping/HTTP/transports

### Definite service/domain logic

1. CLI derives runtime success directly from Petri state.

   [run_clean_invocation.go](C:/Users/andre/work/portos/infinite-you/pkg/transports/cli/run/run_clean_invocation.go:270) searches terminal tokens and dispatch history, examines token colors and place categories, and chooses invocation success/failure. This belongs behind Factory Session/Work result projection, not in CLI.

2. Global HTTP loads and interprets packaged factories.

   [handlers_models.go](C:/Users/andre/work/portos/infinite-you/pkg/transports/http/handlers_models.go:35) loads the internal catalog, decodes definitions twice, requires description/examples, generates YAML, derives slugs, and constructs discovery records. Factory Definitions distribution/catalog service should return a transport-neutral catalog result.

3. Provider Sessions bypasses its service-local HTTP adapter.

   The global server holds `providersessions.Service` directly ([server.go](C:/Users/andre/work/portos/infinite-you/pkg/transports/http/server.go:28)) and reimplements handler, error, logging, and response projection in [handlers_provider_session.go](C:/Users/andre/work/portos/infinite-you/pkg/transports/http/handlers_provider_session.go:12), even though `pkg/services/provider_sessions/transports/http` already exists.

4. Work HTTP logic is duplicated.

   `factory_sessions/transports/http/handlers_work_write.go` duplicates request union validation, state normalization, staging, Work construction, response mapping, and error classification already present in `work/transports/http/admission_mapping.go`.

   The duplicate policy starts around [handlers_work_write.go](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_sessions/transports/http/handlers_work_write.go:159). Factory Sessions should delegate Work routes to the Work HTTP adapter.

5. Factory configuration normalization is still a policy engine.

   [openapi_factory.go](C:/Users/andre/work/portos/infinite-you/pkg/transports/mapping/factoryconfig/openapi_factory.go:192) canonicalizes all factory enums, applies compatibility aliases, rejects retired fields, interprets workstation usage, normalizes resource requirements, and validates operation identifiers. Plain generated-type translation is appropriate here; canonicalization and compatibility policy belong in Factory Definitions compilation/validation.

6. Workflow validation outcome policy lives in mapping.

   [factory_preview.go](C:/Users/andre/work/portos/infinite-you/pkg/transports/mapping/factory_preview.go:321) determines which diagnostics are blocking based on source resolution, conflict codes, artifact-root allowance, and policy findings. Factory Runtime orchestration/preview should return the already-classified result.

7. Operator Settings defaults and validation are applied during mapping.

   [globalconfig/decode.go](C:/Users/andre/work/portos/infinite-you/pkg/transports/mapping/globalconfig/decode.go:51) installs runtime defaults and validates normalized artifact settings before calling `Config.Normalize`. The mapper should decode generated fields; Operator Settings should own defaults and normalization.

8. Automation domain validation sits in HTTP mapping.

   [convergence_mapping.go](C:/Users/andre/work/portos/infinite-you/pkg/services/automations/transports/http/convergence_mapping.go:206) defines required desired/observed identity fields and lifecycle interpretation. Structural decoding belongs in HTTP, but required identity invariants and lifecycle validation belong in Automations.

9. Factory Sessions HTTP owns factory/workstation policy.

   The 1,037-line factory handler performs bundled-document target selection, workstation lookup, prompt-contract construction, durable/live session detection, and list merging. Examples begin in [handlers_factory.go](C:/Users/andre/work/portos/infinite-you/pkg/services/factory_sessions/transports/http/handlers_factory.go:709). These should be service results or service-local projection operations.

### Likely service logic requiring a contract decision

- Dispatch retry/failure classification in `factorysession/factory_session_projection.go`.
- Current-factory save preparation in `mapping/validationentry`.
- Compatibility fallback such as “legacy agent-run default” in `factory_validation.go`.
- CLI invocation target selection and primary-result interpretation in `run/factory_invocation_input.go`.
- CLI runtime path/default construction in `run/run.go` and `root_work.go`.
- HTTP error families where the mapping depends on internal failure causes rather than a typed service error.

### Legitimate transport behavior

These should remain at the boundary:

- Generated OpenAPI ↔ service request/result field copying.
- JSON decoding and unknown-field rejection.
- HTTP status/header writing.
- SSE framing and cursor serialization.
- CLI human/JSON rendering.
- MCP schema/catalog publication.
- Static dashboard asset serving.
- Pure optional-pointer adapters.

Large mapping files are not automatically misplaced, but several are too large to audit confidently: Factory config mapping is over 3,800 lines across its main files, Factory Session mapping exceeds 3,000 lines, and several HTTP handlers exceed 700–1,000 lines.

## 5. Recommended execution order

1. Finish and freeze the active Providers/Workers move.

   The ACP, built-ins, deleted provider/worker paths, authoritative inventories,
   and ownership-boundary repairs are complete. Remaining work is the provider
   root-contract/legacy execution flattening and the broader Workers root
   contract.

2. Repair the authoritative inventories.

   Complete. The following gates hold:

   - `package-target-manifest-check`
   - `ownership-inventory-check`
   - `ownership-boundary-check`

3. Seal the smallest service roots.

   Recommended order:

   - `provider_sessions`
   - `automations`
   - `factory_visualization`
   - `models`
   - `operator_settings`

   Move effect ports and helper constructors away from roots, then expose only the singular `Service`.

4. Complete nested subservice normalization.

   Address Recordings and Work wrappers, then Factory Definitions and Factory Runtime’s larger internal trees.

5. Consolidate HTTP ownership under PSS-I02.

   - Delegate Provider Session endpoints to its adapter.
   - Delegate Work endpoints to Work HTTP.
   - Move packaged catalog behavior to Factory Definitions.
   - Reduce Factory Sessions HTTP to decode → call service → encode.

6. Remove mapping-owned policies.

   Start with CLI Petri result inference, Factory configuration canonicalization, preview blocking classification, and Operator Settings defaults.

7. Complete CLI/MCP composition packets.

   CLI should consume service-owned invocation/result contracts. MCP should publish service-local tools without owning domain validation or orchestration.

8. Converge event contracts under PSS-I05.

   Eliminate the remaining Petri public-surface exception and settle Factory Event ownership in Recordings/event-backbone contracts.

9. Migrate functional tests.

   Break up `tests/functional/runtime_api`, remove service-wire imports, and use `root.BuildProcess` with `edges.Edges`.

10. Regenerate deletion-only baselines last.

   Then run the enforcement targets defined in the [Makefile](C:/Users/andre/work/portos/infinite-you/Makefile:568), followed by `make verify-fast` and `make lint`.

The inventory-reconciliation and owner-boundary portions of the immediate path
are complete. The remaining path is service-root contract sealing → HTTP/CLI
integration cutovers; the broader deletion-only package-structure baseline
should remain deferred until those migrations are proven.
