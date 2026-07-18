# `@you-agent-factory/client`

Framework-neutral TypeScript contracts for You Agent Factory consumers.

The package exports the generated OpenAPI `components`, `paths`, and
`operations` namespaces together with stable `FactoryEvent`,
`FactoryEventType`, `FactoryDefinition`, and `FactoryRecording` aliases.
Generated symbols remain available through `@you-agent-factory/client/generated/openapi`.

`parseFactoryRecording` validates unknown input against the canonical recording
schema and replay-safe cross-event invariants, returning the original typed
recording on success. It throws `FactoryRecordingValidationError` with
structured issues on failure. `safeParseFactoryRecording` performs the same
validation without throwing and returns a discriminated success or failure
result.
