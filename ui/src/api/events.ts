export const FACTORY_EVENT_TYPES = {
  runRequest: "RUN_REQUEST",
  initialStructureRequest: "INITIAL_STRUCTURE_REQUEST",
  workRequest: "WORK_REQUEST",
  relationshipChangeRequest: "RELATIONSHIP_CHANGE_REQUEST",
  dispatchRequest: "DISPATCH_REQUEST",
  inferenceRequest: "INFERENCE_REQUEST",
  inferenceResponse: "INFERENCE_RESPONSE",
  scriptRequest: "SCRIPT_REQUEST",
  scriptResponse: "SCRIPT_RESPONSE",
  dispatchResponse: "DISPATCH_RESPONSE",
  factoryStateResponse: "FACTORY_STATE_RESPONSE",
  runResponse: "RUN_RESPONSE",
} as const;

export type FactoryEventType = (typeof FACTORY_EVENT_TYPES)[keyof typeof FACTORY_EVENT_TYPES];

export type FactoryEvent = {
  context: {
    dispatchId?: string;
    eventTime: string;
    requestId?: string;
    sequence: number;
    tick: number;
    traceIds?: string[];
    workIds?: string[];
  };
  id: string;
  payload: Record<string, unknown>;
  type: FactoryEventType;
};
