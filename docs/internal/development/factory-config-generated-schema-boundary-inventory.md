# Factory Config Generated-Schema Boundary Inventory

This inventory captures the current handwritten factory-config structs that still sit on, or immediately behind, the public config boundary. It distinguishes the remaining runtime-owned models from the small boundary-only compatibility structs so the generated-schema deserialization follow-up stories can remove the right surface without rewriting unrelated runtime state.

## Current Decoding Entrypoints

1. File loading starts in `libraries/agent-factory/pkg/service/factory.go` via `loadFactoryConfigForMode`, which calls `config.LoadRuntimeConfig`.
2. `config.LoadRuntimeConfig` calls `loadFactoryConfig`, and `loadFactoryConfig` calls `FactoryConfigMapper.Expand`.
3. `FactoryConfigMapper.Expand` normalizes incoming JSON keys, rejects retired fields such as `workstations[*].join`, then unmarshals into the generated `pkg/api/generated.Factory` model before mapping into internal runtime structs.
4. The HTTP-transported config path currently in scope is replay and event transport, not a standalone config mutation route: `service.loadFactoryConfigForMode` calls `replay.RuntimeConfigFromGeneratedFactory` when a replay artifact or `RUN_REQUEST` payload carries a generated `factoryapi.Factory`.

## Boundary-Only Handwritten Structs

| Struct | Owner file | Role | Primary call sites | Overlap with generated schema |
| --- | --- | --- | --- | --- |
| `rawOpenAPIFactory` | `libraries/agent-factory/pkg/config/openapi_factory.go` | Auxiliary boundary-only decode helper | `applyOpenAPICronCompatibility` -> `buildRawOpenAPIWorkstationCronIndex` | Overlaps only the `Factory.workstations[].cron` subset of `pkg/api/generated.Factory`; it exists to re-read raw JSON after the main generated decode. |
| `rawOpenAPIWorkstation` | `libraries/agent-factory/pkg/config/openapi_factory.go` | Auxiliary boundary-only decode helper for workstation ID, name, and cron metadata | `buildRawOpenAPIWorkstationCronIndex` | Overlaps `pkg/api/generated.Workstation` fields `id`, `name`, and `cron`; it intentionally ignores the rest of the generated workstation contract. |
| `cronConfigPayload` | `libraries/agent-factory/pkg/interfaces/factory_config.go` | Internal helper struct used by `CronConfig.UnmarshalJSON` | `CronConfig.UnmarshalJSON` | Overlaps the supported fields on `pkg/api/generated.WorkstationCron` (`schedule`, `trigger_at_start`, `jitter`, `expiry_window`) while leaving room to detect retired `interval`. |

These structs are boundary-only compatibility helpers. They are not the primary decode target for `factory.json`, but they still parse raw boundary JSON to preserve retired-field validation on cron config.

## Runtime-Owned Handwritten Structs Still Adjacent To The Boundary

| Struct | Owner file | Role | Primary call sites | Overlap with generated schema |
| --- | --- | --- | --- | --- |
| `interfaces.FactoryConfig` | `libraries/agent-factory/pkg/interfaces/factory_config.go` | Internal runtime topology model produced after boundary decode | `FactoryConfigMapper.Expand`, `config.LoadRuntimeConfig`, `replay.factoryConfigFromGeneratedAPI`, `ConfigMapper.Map` | Broadly overlaps `pkg/api/generated.Factory`, but remains runtime-owned because it also carries internal conventions such as snake_case field tags, merged runtime definitions, and the domain shapes consumed by mapper and validator code. |
| `interfaces.FactoryWorkstationConfig` | `libraries/agent-factory/pkg/interfaces/factory_config.go` | Internal runtime workstation model | `workstationInternalFromAPI`, `runtimeWorkstationDefinition`, `replay.workstationConfigFromGeneratedAPI`, `replay.replayWorkstationConfigFromGenerated` | Broadly overlaps `pkg/api/generated.Workstation`, but also carries runtime-only concerns such as merged stop-word handling, inline AGENTS.md definition fields, canonical `type` defaulting during runtime definition merging, and the exact shape used by runtime definition merging. |
| `interfaces.WorkerConfig` | `libraries/agent-factory/pkg/interfaces/worker_config.go` | Internal runtime worker model | `config.WorkerConfigFromOpenAPI`, `runtimeWorkerDefinition`, `replay.RuntimeConfigFromGeneratedFactory`, `replay.generatedWorkerFromReplayConfig` | Broadly overlaps `pkg/api/generated.Worker`, but remains runtime-owned because worker loading and executor construction consume the internal struct directly. Generated replay and file/config boundaries now use the same explicit config mappers instead of JSON round trips. |
| `interfaces.CronConfig` | `libraries/agent-factory/pkg/interfaces/factory_config.go` | Internal runtime cron model with retired-field detection | `workstationCronInternalFromAPI`, `rawOpenAPIWorkstation.Cron`, config validation | Overlaps `pkg/api/generated.WorkstationCron`, but adds `unsupportedInterval` state so validation can reject the retired `interval` field deterministically. |

These structs should stay internal after decoding, but they still overlap heavily with the generated public contract. The next cleanup stories should keep them runtime-owned and move any remaining public-boundary behavior into explicit generated-to-runtime mapper code.

## Shared Entry Points That Must Stay Converged

- `libraries/agent-factory/pkg/config/factory_config_mapping.go:FactoryConfigMapper.Expand` is the canonical public decode step for `factory.json`, `config flatten`, and `config expand`.
- `libraries/agent-factory/pkg/config/runtime_config.go:loadFactoryConfig` is the file-loading entrypoint that all normal runtime startup paths use before worker and workstation AGENTS.md merging.
- `libraries/agent-factory/pkg/service/factory.go:loadFactoryConfigForMode` is the service-level fork between on-disk config loading and replay-embedded generated config loading.
- `libraries/agent-factory/pkg/replay/generated_factory_runtime.go:RuntimeConfigFromGeneratedFactory` is the transport-side entrypoint for generated config carried through replay artifacts and `RUN_REQUEST` payloads.

## Inventory Conclusions

- There is no longer a primary `factory.json` decode that starts by unmarshalling directly into `interfaces.FactoryConfig`; the main file path already starts from generated `pkg/api/generated.Factory`.
- The remaining handwritten overlap is concentrated in runtime-owned structs plus cron compatibility helpers that still read raw JSON after the generated decode.
- Replay transport now reuses the explicit `pkg/config` generated-to-runtime mappers for worker and workstation reconstruction, so file and replay config decoding share the same owned conversion layer even though replay still has runtime-specific stop-word restoration logic.
- Future removal work should converge on explicit generated-to-runtime mappers in `pkg/config` and shrink or eliminate the raw cron compatibility structs once retired-field validation is preserved elsewhere.
