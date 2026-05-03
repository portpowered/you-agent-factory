import type { components } from "../generated/openapi";

export const FACTORY_EVENTS_ENDPOINT = "/events";

export const FACTORY_EVENT_TYPES = {
  dispatchResponse: "DISPATCH_RESPONSE",
  dispatchRequest: "DISPATCH_REQUEST",
  factoryStateResponse: "FACTORY_STATE_RESPONSE",
  inferenceRequest: "INFERENCE_REQUEST",
  inferenceResponse: "INFERENCE_RESPONSE",
  initialStructureRequest: "INITIAL_STRUCTURE_REQUEST",
  relationshipChangeRequest: "RELATIONSHIP_CHANGE_REQUEST",
  runResponse: "RUN_RESPONSE",
  runRequest: "RUN_REQUEST",
  scriptRequest: "SCRIPT_REQUEST",
  scriptResponse: "SCRIPT_RESPONSE",
  workRequest: "WORK_REQUEST",
} as const satisfies Record<string, FactoryEventType>;

type FactorySchemas = components["schemas"];
type GeneratedFactoryEvent = FactorySchemas["FactoryEvent"];

export type FactoryEventType = FactorySchemas["FactoryEventType"];

export type FactoryEventContext = FactorySchemas["FactoryEventContext"];

export interface FactoryEvent<TPayload = FactoryEventPayload> {
  context: FactoryEventContext;
  id: string;
  payload: TPayload;
  schemaVersion?: GeneratedFactoryEvent["schemaVersion"];
  type: FactoryEventType;
}

export type FactoryEventPayload =
  | RunRequestPayload
  | RunResponsePayload
  | InitialStructureRequestPayload
  | WorkRequestPayload
  | RelationshipChangeRequestPayload
  | DispatchRequestPayload
  | InferenceRequestPayload
  | InferenceResponsePayload
  | ScriptRequestPayload
  | ScriptResponsePayload
  | DispatchResponsePayload
  | FactoryStateResponsePayload
  | Record<string, unknown>;

export interface RunRequestPayload {
  diagnostics?: Record<string, unknown>;
  factory: FactoryDefinition;
  recordedAt: string;
  wallClock?: Record<string, unknown>;
}

export type RunResponsePayload = FactorySchemas["RunResponseEventPayload"];

export type InitialStructureRequestPayload =
  FactorySchemas["InitialStructureRequestEventPayload"];

export type FactoryDefinition = FactorySchemas["Factory"];

export type FactoryInputType = FactorySchemas["InputType"];

export type WorkRequestPayload = FactorySchemas["WorkRequestEventPayload"];

export type RelationshipChangeRequestPayload =
  FactorySchemas["RelationshipChangeRequestEventPayload"];

export type DispatchRequestPayload = FactorySchemas["DispatchRequestEventPayload"];

export type InferenceRequestPayload = FactorySchemas["InferenceRequestEventPayload"];

export type InferenceResponsePayload = FactorySchemas["InferenceResponseEventPayload"];

export type InferenceOutcome = FactorySchemas["InferenceOutcome"];

export type ScriptRequestPayload = FactorySchemas["ScriptRequestEventPayload"];

export type ScriptResponsePayload = FactorySchemas["ScriptResponseEventPayload"];

export type ScriptExecutionOutcome = FactorySchemas["ScriptExecutionOutcome"];

export type ScriptFailureType = FactorySchemas["ScriptFailureType"];

export type DispatchResponsePayload = FactorySchemas["DispatchResponseEventPayload"];

export type FactoryStateResponsePayload =
  FactorySchemas["FactoryStateResponseEventPayload"];

export type FactoryResource = FactorySchemas["Resource"];

export type FactoryWorker = FactorySchemas["Worker"];

export type FactoryWorkType = FactorySchemas["WorkType"];

export type FactoryStateDefinition = FactorySchemas["WorkState"];

export type FactoryWorkstation = FactorySchemas["Workstation"];

export interface FactoryPlace {
  category?: string;
  id: string;
  state: string;
  type_id: string;
}

export type WorkstationIO = FactorySchemas["WorkstationIO"];

export type FactoryWork = FactorySchemas["Work"];

export interface FactoryWorkItem {
  current_chaining_trace_id?: string;
  display_name?: string;
  id: string;
  parent_id?: string;
  place_id?: string;
  previous_chaining_trace_ids?: string[];
  tags?: Record<string, string>;
  trace_id?: string;
  work_type_id: string;
}

export type FactoryRelation = FactorySchemas["Relation"] & {
  request_id?: string;
  source_work_id?: string;
  trace_id?: string;
};

export type FactoryProviderFailure = FactorySchemas["ProviderFailureMetadata"];

export type FactoryProviderSession = FactorySchemas["ProviderSessionMetadata"];

export type FactoryWorkDiagnostics = FactorySchemas["SafeWorkDiagnostics"];

export type FactoryRenderedPromptDiagnostic =
  FactorySchemas["RenderedPromptDiagnostic"];

export type FactoryProviderDiagnostic = FactorySchemas["ProviderDiagnostic"];

export interface FactoryTerminalWork {
  status: string;
  work_item: FactoryWorkItem;
}

