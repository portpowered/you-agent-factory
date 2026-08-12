import {
  type components,
  FactoryEventSchemaVersion as GENERATED_FACTORY_EVENT_SCHEMA_VERSIONS,
  FactoryEventType as GENERATED_FACTORY_EVENT_TYPES,
  type operations,
  type paths,
} from "./generated/openapi.js";

export type { components, operations, paths };
// Re-export the generated workstation taxonomy so presentation packages can
// consume the same runtime values as the public Factory contract.
export {
  WorkstationGuardType,
  WorkstationKind,
  WorkstationType,
} from "./generated/openapi.js";

type FactorySchemas = components["schemas"];

/** The canonical versioned event envelope exposed by the public client. */
export type FactoryEvent = FactorySchemas["FactoryEvent"];

/** The supported canonical Factory event type vocabulary. */
export type FactoryEventType = FactorySchemas["FactoryEventType"];

/** The customer-authored Factory topology definition. */
export type FactoryDefinition = FactorySchemas["Factory"];

/**
 * Runtime Factory event constants generated from the canonical OpenAPI input.
 * The object retains the generated keys and literal values without duplicating
 * the event vocabulary in handwritten source.
 */
export const FACTORY_EVENT_TYPES =
  GENERATED_FACTORY_EVENT_TYPES satisfies Record<
    keyof typeof GENERATED_FACTORY_EVENT_TYPES,
    FactoryEventType
  >;

/** Runtime versions accepted by the canonical Factory event envelope. */
export const FACTORY_EVENT_SCHEMA_VERSIONS =
  GENERATED_FACTORY_EVENT_SCHEMA_VERSIONS;
