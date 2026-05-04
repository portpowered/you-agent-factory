import type {
  DispatchRequestPayload,
  DispatchResponsePayload,
  FactoryEvent,
  FactoryStateResponsePayload,
  InferenceRequestPayload,
  InferenceResponsePayload,
  InitialStructureRequestPayload,
  RelationshipChangeRequestPayload,
  RunRequestPayload,
  ScriptRequestPayload,
  ScriptResponsePayload,
  WorkRequestPayload,
} from "../../../../api/events";

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
