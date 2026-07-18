import type { components } from "./generated/openapi.js";

/** A canonical event emitted by the Factory runtime. */
export type FactoryEvent = components["schemas"]["FactoryEvent"];

/** The customer-visible canonical Factory event vocabulary. */
export type FactoryEventType = components["schemas"]["FactoryEventType"];

/** A complete authored Factory definition. */
export type FactoryDefinition = components["schemas"]["Factory"];

/** A chapter-free, ordered event recording for one Factory Session. */
export type FactoryRecording = components["schemas"]["FactoryRecording"];
