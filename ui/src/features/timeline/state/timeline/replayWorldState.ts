import type {
  FactoryEvent,
  FactoryRelation,
} from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import {
  applyScriptRequest,
  applyScriptResponse,
  inferenceAttemptsForDispatch,
  legacyInferencePayloadDispatchID,
  legacyInferencePayloadTransitionID,
  resolveDispatchTransitionID,
  syncCompletedDispatchAttempt,
} from "./replayWorldStateSupport";
import type {
  DispatchRequestEvent,
  DispatchResponseEvent,
  FactoryStateResponseEvent,
  InferenceRequestEvent,
  InferenceResponseEvent,
  InitialStructureRequestEvent,
  RelationshipChangeRequestEvent,
  RunRequestEvent,
  ScriptRequestEvent,
  ScriptResponseEvent,
  WorkRequestEvent,
} from "./replayWorldStateTypes";
import {
  dashboardDiagnosticsFromEvent,
  firstRequestID,
  legacyDispatchRequestPayload,
  legacyDispatchResponsePayload,
  responseCompletion,
  recordFailedCompletion,
  completionToProviderSession,
  completionToTraceDispatch,
} from "./replayCompletion";
import {
  eventWorkTypeID,
  factoryWorkToItem,
  initialPlaceForWork,
  normalizeFactoryPayload,
  outputPlaceForWorkstation,
  resolveWorkstationName,
  seedResourceOccupancy as seedResourceOccupancyBase,
  topologyWorker,
  workerModelProvider,
} from "./replayFactoryTopology";
import {
  addRelation,
  addToken,
  addTraceDispatch as addTraceDispatchBase,
  addTraceRequest,
  addTraceWork,
  consumeResourceUnits,
  releaseResourceUnits,
  removeWorkToken,
  resourceTokenID,
  traceToken,
} from "./replayGraphState";
import { orderedEvents, uniqueSorted } from "./shared";
import { dashboardTransitionID, isSystemTimeWorkItem } from "./systemTime";
import type {
  WorldState,
} from "./types";
import { workRef } from "./workItemRef";

function emptyWorldState(tick: number): WorldState {
  return {
    activeDispatches: {},
    completedDispatches: [],
    factoryState: "UNKNOWN",
    failedWorkDetailsByWorkID: {},
    failedWorkItemsByID: {},
    inferenceAttemptsByDispatchID: {},
    occupancyByID: {},
    providerSessions: [],
    relationsByWorkID: {},
    scriptRequestsByDispatchID: {},
    scriptResponsesByDispatchID: {},
    terminalWorkByID: {},
    tick,
    topology: {},
    tracesByID: {},
    workItemsByID: {},
    workRequestsByID: {},
  };
}

function seedResourceOccupancy(state: WorldState): void {
  seedResourceOccupancyBase(state, addToken, resourceTokenID);
}

function addTraceDispatch(state: WorldState, traceID: string, completion: Parameters<typeof completionToTraceDispatch>[0]): void {
  addTraceDispatchBase(state, traceID, completion, completionToTraceDispatch);
}

export function reconstructWorldState(
  events: FactoryEvent[],
  selectedTick: number,
): WorldState {
  const state = emptyWorldState(selectedTick);
  for (const event of orderedEvents(events)) {
    if (event.context.tick <= selectedTick) {
      applyEvent(state, event);
    }
  }
  return state;
}

function applyEvent(state: WorldState, event: FactoryEvent): void {
  switch (event.type) {
    case FACTORY_EVENT_TYPES.runRequest:
      state.topology = normalizeFactoryPayload((event as RunRequestEvent).payload);
      seedResourceOccupancy(state);
      return;
    case FACTORY_EVENT_TYPES.runResponse:
      return;
    case FACTORY_EVENT_TYPES.initialStructureRequest:
      state.topology = normalizeFactoryPayload((event as InitialStructureRequestEvent).payload);
      seedResourceOccupancy(state);
      return;
    case FACTORY_EVENT_TYPES.workRequest:
      applyWorkRequest(state, event as WorkRequestEvent);
      return;
    case FACTORY_EVENT_TYPES.relationshipChangeRequest:
      applyRelationshipChange(state, event as RelationshipChangeRequestEvent);
      return;
    case FACTORY_EVENT_TYPES.dispatchRequest:
      applyRequest(state, event as DispatchRequestEvent);
      return;
    case FACTORY_EVENT_TYPES.inferenceRequest:
      applyInferenceRequest(state, event as InferenceRequestEvent);
      return;
    case FACTORY_EVENT_TYPES.inferenceResponse:
      applyInferenceResponse(state, event as InferenceResponseEvent);
      return;
    case FACTORY_EVENT_TYPES.scriptRequest:
      applyScriptRequest(state, event as ScriptRequestEvent);
      return;
    case FACTORY_EVENT_TYPES.scriptResponse:
      applyScriptResponse(state, event as ScriptResponseEvent);
      return;
    case FACTORY_EVENT_TYPES.dispatchResponse:
      applyResponse(state, event as DispatchResponseEvent);
      return;
    case FACTORY_EVENT_TYPES.factoryStateResponse:
      state.factoryState = (event as FactoryStateResponseEvent).payload.state;
      return;
  }
}

function applyWorkRequest(state: WorldState, event: WorkRequestEvent): void {
  const requestID = event.context.requestId ?? firstRequestID(event.payload.works) ?? "";
  const traceID = event.context.traceIds?.[0];
  const workItems = (event.payload.works ?? []).map((work) =>
    factoryWorkToItem(state, work, initialPlaceForWork(state, eventWorkTypeID(work) ?? "")),
  );
  state.workRequestsByID[requestID] = {
    request_id: requestID,
    source: event.payload.source,
    trace_id: traceID,
    type: event.payload.type,
    work_items: workItems,
  };
  for (const item of workItems) {
    state.workItemsByID[item.id] = item;
    if (isSystemTimeWorkItem(item)) {
      continue;
    }
    addToken(state, item.place_id, item.id, item.id);
    addTraceWork(state, item);
    addTraceRequest(state, item.trace_id ?? traceID, requestID);
  }
}

function applyRelationshipChange(state: WorldState, event: RelationshipChangeRequestEvent): void {
  const relation = event.payload.relation as FactoryRelation;
  const targetWorkID =
    relation.targetWorkId ??
    (relation as FactoryRelation & { target_work_id?: string }).target_work_id;
  addRelation(state, {
    ...relation,
    request_id: relation.request_id ?? event.context.requestId,
    source_work_id:
      relation.source_work_id ??
      event.context.workIds?.find((workID) => workID !== targetWorkID),
    trace_id: relation.trace_id ?? event.context.traceIds?.[0],
  });
}

function applyRequest(state: WorldState, event: DispatchRequestEvent): void {
  const legacyPayload = legacyDispatchRequestPayload(event.payload);
  const worker = topologyWorker(state.topology, event.payload.transitionId);
  const dispatchID =
    event.context.dispatchId ??
    (typeof legacyPayload.dispatchId === "string" ? legacyPayload.dispatchId : undefined);
  if (!dispatchID) {
    return;
  }

  const workItems = (legacyPayload.inputs ?? event.payload.inputs).map((work) =>
    factoryWorkToItem(state, work),
  );
  for (const item of workItems) {
    removeWorkToken(state, item.id);
    state.workItemsByID[item.id] = item;
    if (isSystemTimeWorkItem(item)) {
      continue;
    }
    addTraceWork(state, item);
  }
  const publicWorkItems = workItems.filter((item) => !isSystemTimeWorkItem(item));
  state.activeDispatches[dispatchID] = {
    consumedTokens: workItems.map((item) => traceToken(item, event.context.eventTime)),
    currentChainingTraceID:
      event.context.currentChainingTraceId ??
      event.payload.currentChainingTraceId ??
      legacyPayload.current_chaining_trace_id ??
      publicWorkItems.find((item) => item.current_chaining_trace_id)
        ?.current_chaining_trace_id ??
      publicWorkItems[0]?.trace_id,
    dispatchID,
    model: legacyPayload.worker?.model ?? worker?.model,
    modelProvider:
      workerModelProvider(legacyPayload.worker) ?? worker?.model_provider,
    previousChainingTraceIDs: event.context.previousChainingTraceIds
      ? [...event.context.previousChainingTraceIds]
      : event.payload.previousChainingTraceIds
        ? [...event.payload.previousChainingTraceIds]
        : legacyPayload.previous_chaining_trace_ids
          ? [...legacyPayload.previous_chaining_trace_ids]
          : undefined,
    provider: legacyPayload.worker?.executorProvider ?? worker?.provider,
    resources: consumeResourceUnits(state, event.payload.resources),
    startedAt: event.context.eventTime,
    systemOnly:
      event.payload.transitionId === "__system_time:expire" && publicWorkItems.length === 0,
    traceIDs: uniqueSorted(publicWorkItems.map((item) => item.trace_id ?? "")),
    transitionID: dashboardTransitionID(event.payload.transitionId),
    workItems: publicWorkItems.map(workRef),
    workstationName: resolveWorkstationName(
      state.topology,
      event.payload.transitionId,
      legacyPayload.workstation?.name,
    ),
  };
}

function applyInferenceRequest(state: WorldState, event: InferenceRequestEvent): void {
  const { payload } = event;
  const dispatchID =
    event.context.dispatchId ?? legacyInferencePayloadDispatchID(payload);
  if (!dispatchID || !payload.inferenceRequestId) {
    return;
  }
  const attempts = inferenceAttemptsForDispatch(state, dispatchID);
  const current = attempts[payload.inferenceRequestId];
  const transitionID =
    legacyInferencePayloadTransitionID(payload) ??
    current?.transition_id ??
    resolveDispatchTransitionID(state, dispatchID);
  if (!transitionID) {
    return;
  }
  attempts[payload.inferenceRequestId] = {
    ...current,
    attempt: payload.attempt,
    dispatch_id: dispatchID,
    inference_request_id: payload.inferenceRequestId,
    prompt: payload.prompt,
    request_time: event.context.eventTime,
    transition_id: dashboardTransitionID(transitionID),
    working_directory: payload.workingDirectory,
    worktree: payload.worktree,
  };
}

function applyInferenceResponse(state: WorldState, event: InferenceResponseEvent): void {
  const { payload } = event;
  const dispatchID =
    event.context.dispatchId ?? legacyInferencePayloadDispatchID(payload);
  if (!dispatchID || !payload.inferenceRequestId) {
    return;
  }
  const attempts = inferenceAttemptsForDispatch(state, dispatchID);
  const current = attempts[payload.inferenceRequestId];
  const transitionID =
    legacyInferencePayloadTransitionID(payload) ??
    current?.transition_id ??
    resolveDispatchTransitionID(state, dispatchID);
  if (!transitionID) {
    return;
  }
  attempts[payload.inferenceRequestId] = {
    ...current,
    attempt: payload.attempt,
    diagnostics: dashboardDiagnosticsFromEvent(payload.diagnostics),
    dispatch_id: dispatchID,
    duration_millis: payload.durationMillis,
    error_class: payload.errorClass,
    exit_code: payload.exitCode,
    inference_request_id: payload.inferenceRequestId,
    outcome: payload.outcome,
    prompt: current?.prompt ?? "",
    provider_session: payload.providerSession,
    request_time: current?.request_time ?? "",
    response: payload.response,
    response_time: event.context.eventTime,
    transition_id: dashboardTransitionID(transitionID),
  };
  syncCompletedDispatchAttempt(state, dispatchID, attempts[payload.inferenceRequestId], {
    completionToProviderSession,
    uniqueSorted,
  });
}

function applyResponse(state: WorldState, event: DispatchResponseEvent): void {
  const legacyPayload = legacyDispatchResponsePayload(event.payload);
  const dispatchID =
    event.context.dispatchId ??
    (typeof legacyPayload.dispatchId === "string" ? legacyPayload.dispatchId : undefined);
  if (!dispatchID) {
    return;
  }

  const active = state.activeDispatches[dispatchID];
  delete state.activeDispatches[dispatchID];
  releaseResourceUnits(state, active?.resources ?? [], event.payload.outputResources);
  const outputItems = (event.payload.outputWork ?? []).map((work) =>
    factoryWorkToItem(
      state,
      work,
      outputPlaceForWorkstation(
        state.topology,
        event.payload.transitionId,
        event.payload.outcome,
        eventWorkTypeID(work) ?? "",
        work.state ?? "",
      ),
    ),
  );
  for (const item of outputItems) {
    state.workItemsByID[item.id] = item;
    if (isSystemTimeWorkItem(item)) {
      continue;
    }
    addTraceWork(state, item);
    addToken(state, item.place_id, item.id, item.id);
  }
  const completion = responseCompletion(state, event, active, dispatchID);
  state.completedDispatches = [...state.completedDispatches, completion];
  if (
    completion.terminalWork &&
    completion.outcome !== "FAILED" &&
    completion.terminalWork.status !== "FAILED"
  ) {
    state.terminalWorkByID[completion.terminalWork.work_item.id] = completion.terminalWork;
  }
  if (event.payload.outcome === "FAILED") {
    recordFailedCompletion(state, completion);
  }
  if (completion.providerSession?.id) {
    state.providerSessions = [
      ...state.providerSessions,
      completionToProviderSession(completion),
    ];
  }
  for (const traceID of completion.traceIDs) {
    addTraceDispatch(state, traceID, completion);
  }
}


