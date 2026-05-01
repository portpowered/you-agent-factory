import type { components } from "./generated/openapi";

export type FactoryEvent = components["schemas"]["FactoryEvent"];
export type FactoryEventContext = components["schemas"]["FactoryEventContext"];
export type DispatchRequestEventPayload = components["schemas"]["DispatchRequestEventPayload"];
export type DispatchResponseEventPayload = components["schemas"]["DispatchResponseEventPayload"];
export type FactoryStateResponseEventPayload = components["schemas"]["FactoryStateResponseEventPayload"];
export type InitialStructureRequestEventPayload = components["schemas"]["InitialStructureRequestEventPayload"];
export type RunRequestEventPayload = components["schemas"]["RunRequestEventPayload"];
export type WorkRequestEventPayload = components["schemas"]["WorkRequestEventPayload"];

export const FACTORY_EVENT_TYPES = {
  dispatchRequest: "DISPATCH_REQUEST",
  dispatchResponse: "DISPATCH_RESPONSE",
  factoryStateResponse: "FACTORY_STATE_RESPONSE",
  inferenceRequest: "INFERENCE_REQUEST",
  inferenceResponse: "INFERENCE_RESPONSE",
  initialStructureRequest: "INITIAL_STRUCTURE_REQUEST",
  relationshipChangeRequest: "RELATIONSHIP_CHANGE_REQUEST",
  runRequest: "RUN_REQUEST",
  runResponse: "RUN_RESPONSE",
  scriptRequest: "SCRIPT_REQUEST",
  scriptResponse: "SCRIPT_RESPONSE",
  workRequest: "WORK_REQUEST",
} as const satisfies Record<string, FactoryEvent["type"]>;
