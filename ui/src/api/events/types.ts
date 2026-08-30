import type { components } from "../generated/openapi";

export const FACTORY_EVENTS_ENDPOINT = "/events";

export const FACTORY_EVENT_TYPES = {
  factoryChange: "FACTORY_CHANGE",
  factoryChangeRequest: "FACTORY_CHANGE_REQUEST",
  factoryChangeFailed: "FACTORY_CHANGE_FAILED",
  dispatchResponse: "DISPATCH_RESPONSE",
  dispatchRequest: "DISPATCH_REQUEST",
  humanApprovalRequested: "HUMAN_APPROVAL_REQUESTED",
  dispatchWorkerSessionAssociation: "DISPATCH_WORKER_SESSION_ASSOCIATION",
  modelRequest: "MODEL_REQUEST",
  modelResponse: "MODEL_RESPONSE",
  factoryStateResponse: "FACTORY_STATE_RESPONSE",
  inferenceRequest: "INFERENCE_REQUEST",
  inferenceResponse: "INFERENCE_RESPONSE",
  initialStructureRequest: "INITIAL_STRUCTURE_REQUEST",
  relationshipChangeRequest: "RELATIONSHIP_CHANGE_REQUEST",
  runResponse: "RUN_RESPONSE",
  runRequest: "RUN_REQUEST",
  scriptRequest: "SCRIPT_REQUEST",
  scriptResponse: "SCRIPT_RESPONSE",
  agentRunResponse: "AGENT_RUN_RESPONSE",
  workRequest: "WORK_REQUEST",
  workStateChange: "WORK_STATE_CHANGE",
  sessionStarted: "SESSION_STARTED",
  sessionPaused: "SESSION_PAUSED",
  sessionResumed: "SESSION_RESUMED",
  sessionLifecycleControl: "SESSION_LIFECYCLE_CONTROL",
  sessionResultUpdated: "SESSION_RESULT_UPDATED",
  sessionCompleted: "SESSION_COMPLETED",
  orchestratorPhaseChanged: "ORCHESTRATOR_PHASE_CHANGED",
  orchestratorCheckpointWritten: "ORCHESTRATOR_CHECKPOINT_WRITTEN",
  dispatchQueued: "DISPATCH_QUEUED",
  dispatchInterrupted: "DISPATCH_INTERRUPTED",
  dispatchReconciled: "DISPATCH_RECONCILED",
  dispatchResultIgnored: "DISPATCH_RESULT_IGNORED",
  javascriptCheckpointRef: "JAVASCRIPT_CHECKPOINT_REF",
  javascriptPhaseChange: "JAVASCRIPT_PHASE_CHANGE",
  artifactCreated: "ARTIFACT_CREATED",
} as const satisfies Record<string, FactoryEventType>;

type FactorySchemas = components["schemas"];
type GeneratedFactoryEvent = FactorySchemas["FactoryEvent"];

/** Event types are open so an older dashboard can retain newer events. */
export type FactoryEventType =
  | FactorySchemas["FactoryEventType"]
  | (string & {});

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
  | FactoryChangePayload
  | FactoryChangeRequestPayload
  | FactoryChangeFailedPayload
  | WorkRequestPayload
  | RelationshipChangeRequestPayload
  | DispatchRequestPayload
  | InferenceRequestPayload
  | InferenceResponsePayload
  | ScriptRequestPayload
  | ScriptResponsePayload
  | DispatchResponsePayload
  | WorkStateChangeEventPayload
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

export type FactoryChangePayload = FactorySchemas["FactoryChangeEventPayload"];

export type FactoryChangeRequestPayload =
  FactorySchemas["FactoryChangeRequestEventPayload"];

export type FactoryChangeFailedPayload =
  FactorySchemas["FactoryChangeFailedEventPayload"];

export type FactoryDefinition = FactorySchemas["Factory"];

export type WorkRequestPayload = FactorySchemas["WorkRequestEventPayload"];

export type RelationshipChangeRequestPayload =
  FactorySchemas["RelationshipChangeRequestEventPayload"];

export type DispatchRequestPayload =
  FactorySchemas["DispatchRequestEventPayload"];

export type InferenceRequestPayload =
  FactorySchemas["InferenceRequestEventPayload"];

export type InferenceResponsePayload =
  FactorySchemas["InferenceResponseEventPayload"];

export type InferenceOutcome = FactorySchemas["InferenceOutcome"];

export type ScriptRequestPayload = FactorySchemas["ScriptRequestEventPayload"];

export type ScriptResponsePayload =
  FactorySchemas["ScriptResponseEventPayload"];

export type DispatchResponsePayload =
  FactorySchemas["DispatchResponseEventPayload"];

export type WorkStateChangeEventPayload =
  FactorySchemas["WorkStateChangeEventPayload"];

export type FactoryStateResponsePayload =
  FactorySchemas["FactoryStateResponseEventPayload"];

export type FactoryResource = FactorySchemas["Resource"];

export type FactoryWorker = FactorySchemas["Worker"];

export type FactoryWorkType = FactorySchemas["WorkType"];

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
  chaining_trace_depth?: number;
  content?: FactorySchemas["Work"]["content"];
  current_chaining_trace_id?: string;
  display_name?: string;
  id: string;
  parent_id?: string;
  place_id?: string;
  previous_chaining_trace_ids?: string[];
  state?: string;
  tags?: Record<string, string>;
  trace_id?: string;
  work_type_id: string;
}

export type FactoryRelation = FactorySchemas["Relation"] & {
  request_id?: string;
  source_work_id?: string;
  trace_id?: string;
};

export type FactoryProviderSession = FactorySchemas["ProviderSessionMetadata"];

export type FactoryWorkDiagnostics = FactorySchemas["SafeWorkDiagnostics"];

export interface FactoryTerminalWork {
  status: string;
  work_item: FactoryWorkItem;
}
