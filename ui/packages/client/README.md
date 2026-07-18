# `@you-agent-factory/client`

Transport-neutral Factory contracts and recording utilities for TypeScript
consumers. The package has no React, browser, dashboard, or network dependency.

## Stable contracts

The root entry point exports the generated `components`, `paths`, and
`operations` type namespaces together with stable `FactoryEvent`,
`FactoryEventType`, and `FactoryDefinition` aliases. `FACTORY_EVENT_TYPES`
provides the runtime event constants from the same generated contract.

## Contract generation

Run `bun run generate` from this directory after the checked-in dashboard
OpenAPI TypeScript artifact changes. Run `bun run check:generated` to verify
freshness without writing files. Both commands treat
`ui/src/api/generated/openapi.ts` as read-only input and write, when requested,
only to `ui/packages/client/src/generated`.
