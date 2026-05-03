import type {
  DispatchRequestPayload,
  DispatchResponsePayload,
  FactoryEvent,
  FactoryStateResponsePayload,
  FactoryWork,
  FactoryWorkDiagnostics,
  FactoryWorker,
  InferenceRequestPayload,
  InferenceResponsePayload,
  InitialStructureRequestPayload,
  RelationshipChangeRequestPayload,
  RunRequestPayload,
  ScriptRequestPayload,
  ScriptResponsePayload,
  WorkRequestPayload,
} from "../../api/events";
import type { WorldCompletion } from "./types";

export type InitialStructureRequestEvent = FactoryEvent<InitialStructureRequestPayload>;
export type RunRequestEvent = FactoryEvent<RunRequestPayload>;
export type WorkRequestEvent = FactoryEvent<WorkRequestPayload>;
export type RelationshipChangeRequestEvent = FactoryEvent<RelationshipChangeRequestPayload>;
export type DispatchRequestEvent = FactoryEvent<DispatchRequestPayload>;
export type InferenceRequestEvent = FactoryEvent<InferenceRequestPayload>;
export type InferenceResponseEvent = FactoryEvent<InferenceResponsePayload>;
export type ScriptRequestEvent = FactoryEvent<ScriptRequestPayload>;
export type ScriptResponseEvent = FactoryEvent<ScriptResponsePayload>;
export type DispatchResponseEvent = FactoryEvent<DispatchResponsePayload>;
export type FactoryStateResponseEvent = FactoryEvent<FactoryStateResponsePayload>;

export interface LegacyDispatchRequestPayloadCompat {
  current_chaining_trace_id?: string;
  dispatchId?: string;
  inputs?: Array<FactoryWork | { workId: string }>;
  previous_chaining_trace_ids?: string[];
  worker?: FactoryWorker;
  workstation?: { name?: string };
}

export interface LegacyDispatchResponsePayloadCompat {
  current_chaining_trace_id?: string;
  diagnostics?: FactoryWorkDiagnostics;
  dispatchId?: string;
  previous_chaining_trace_ids?: string[];
  providerSession?: WorldCompletion["providerSession"];
  workstation?: { name?: string };
}
