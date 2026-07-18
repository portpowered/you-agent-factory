# `@you-agent-factory/client`

Transport-neutral Factory contracts and recording utilities for TypeScript
consumers. The package has no React, browser, dashboard, or network dependency.

## Stable contracts

The root entry point exports the generated `components`, `paths`, and
`operations` type namespaces together with stable `FactoryEvent`,
`FactoryEventType`, and `FactoryDefinition` aliases. `FACTORY_EVENT_TYPES`
provides the runtime event constants from the same generated contract.

## Factory recordings

`parseFactoryRecording` validates unknown input and returns a trusted
`FactoryRecording`; invalid input throws `FactoryRecordingValidationError` with
machine-readable issue codes and input paths. Use `safeParseFactoryRecording`
for a discriminated result that never throws for invalid customer input. Both
recording and replay parsing enforce the generated canonical Factory and
discriminated Factory-event JSON Schemas, including formats, numeric limits,
required payload fields, and additional-property rules.

The packaged
[`customer-support.factory-recording.v1.json`](examples/customer-support.factory-recording.v1.json)
is a backend-free example of the public `factory-recording/v1` contract. It
includes both a top-level Factory definition and a canonical topology event.

## Replay helpers

Replay consumers can use `orderFactoryEvents` for deterministic tick-first
ordering, `createFactoryEventCursor` for reconnect positions, and
`parseFactoryEventReplayText` (or its non-throwing safe-parser counterpart) to
validate and order canonical JSON `data` frames from captured SSE text. These
helpers are pure and do not open or retain a live transport.

## Contract generation

Run `bun run generate` from this directory after the checked-in dashboard
OpenAPI TypeScript artifact changes. Run `bun run check:generated` to verify
freshness without writing files. Both commands treat the generated OpenAPI
TypeScript artifact and published Factory/Factory-event schemas as read-only
inputs and write, when requested, only to
`ui/packages/client/src/generated`.

## Distribution verification

Run `bun run build` to emit the ESM runtime and declarations in `dist`. Run
`bun run check:pack` to build a registry-format tarball and verify its exact
inventory, export targets, and declared runtime dependency boundary. Run
`bun run check:installed-consumer` to install that tarball in a clean temporary
project, typecheck the public contracts, and execute the recording, ordering,
cursor, and replay helpers against the packaged customer example. `bun run
verify` runs these checks after generation freshness, package typecheck, and
focused tests.
