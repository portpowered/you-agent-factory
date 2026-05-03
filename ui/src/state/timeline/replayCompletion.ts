import type {
  DashboardInferenceAttempt,
  DashboardProviderSessionAttempt,
  DashboardTraceDispatch,
  DashboardWorkDiagnostics,
} from "../../api/dashboard";
import type {
  DispatchRequestPayload,
  DispatchResponsePayload,
  FactoryEvent,
  FactoryProviderSession,
  FactoryTerminalWork,
  FactoryWork,
  FactoryWorkDiagnostics,
  FactoryWorkItem,
  FactoryWorker,
} from "../../api/events";
import { cloneWorkItemRef, uniqueSortedWorkRefs } from "./cloneTimelineSnapshot";
import { addTraceWork } from "./replayGraphState";
import {
  eventWorkTypeID,
  factoryWorkToItem,
  outputPlaceForWorkstation,
  resolveWorkstationName,
} from "./replayFactoryTopology";
import { uniqueSorted } from "./shared";
import {
  dashboardTransitionID,
  isSystemTimeWorkItem,
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
} from "./systemTime";
import type { ProjectedInitialStructure, WorldCompletion, WorldDispatch, WorldState } from "./types";
import { workRef } from "./workItemRef";

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
  providerSession?: FactoryProviderSession;
  workstation?: { name?: string };
}

export function latestWorkstationAttempt(
  attempts: Record<string, DashboardInferenceAttempt> | undefined,
): DashboardInferenceAttempt | undefined {
  return Object.values(attempts ?? {}).sort((left, right) => {
    if (left.attempt !== right.attempt) {
      return right.attempt - left.attempt;
    }
    if ((left.request_time ?? "") !== (right.request_time ?? "")) {
      return (right.request_time ?? "").localeCompare(left.request_time ?? "");
    }
    return right.inference_request_id.localeCompare(left.inference_request_id);
  })[0];
}

export function legacyDispatchRequestPayload(
  payload: DispatchRequestPayload,
): LegacyDispatchRequestPayloadCompat {
  return payload as DispatchRequestPayload & LegacyDispatchRequestPayloadCompat;
}

export function legacyDispatchResponsePayload(
  payload: DispatchResponsePayload,
): LegacyDispatchResponsePayloadCompat {
  return payload as DispatchResponsePayload & LegacyDispatchResponsePayloadCompat;
}

export function dashboardDiagnosticsFromEvent(
  diagnostics: FactoryWorkDiagnostics | undefined,
): DashboardWorkDiagnostics | undefined {
  if (!diagnostics) {
    return undefined;
  }

  return {
    provider: diagnostics.provider
      ? {
          model: diagnostics.provider.model,
          provider: diagnostics.provider.provider,
          request_metadata: diagnostics.provider.requestMetadata,
          response_metadata: diagnostics.provider.responseMetadata,
        }
      : undefined,
    rendered_prompt: diagnostics.renderedPrompt
      ? {
          system_prompt_hash: diagnostics.renderedPrompt.systemPromptHash,
          user_message_hash: diagnostics.renderedPrompt.userMessageHash,
          variables: diagnostics.renderedPrompt.variables,
        }
      : undefined,
  };
}

export function firstRequestID(works: FactoryWork[] | undefined): string | undefined {
  return works?.find((work) => work.requestId)?.requestId;
}

function placeCategory(state: WorldState, placeIDValue: string | undefined): string | undefined {
  return state.topology.places?.find((place) => place.id === placeIDValue)?.category;
}

function terminalWorkFromItems(
  state: WorldState,
  items: FactoryWorkItem[],
  outcome: string,
): FactoryTerminalWork | undefined {
  const publicItems = items.filter((item) => !isSystemTimeWorkItem(item));
  const terminal = publicItems.find((item) => placeCategory(state, item.place_id) === "TERMINAL");
  const failed = publicItems.find((item) => placeCategory(state, item.place_id) === "FAILED");
  if (outcome === "FAILED" && failed) {
    return { status: "FAILED", work_item: failed };
  }
  if (terminal) {
    return { status: "TERMINAL", work_item: terminal };
  }
  return undefined;
}

export function responseCompletion(
  state: WorldState,
  event: FactoryEvent<DispatchResponsePayload>,
  active: WorldDispatch | undefined,
  dispatchID: string,
): WorldCompletion {
  const legacyPayload = legacyDispatchResponsePayload(event.payload);
  const outputItems = (event.payload.outputWork ?? []).map((work) =>
    factoryWorkToItem(
      state,
      work,
      outputPlaceForWorkstation(
        state.topology,
        event.payload.transitionId,
        event.payload.outcome,
        eventWorkTypeID(work) ?? "",
        work.state,
      ),
    ),
  );
  const outputRefs = outputItems
    .filter((item) => !isSystemTimeWorkItem(item))
    .map((item) => {
      state.workItemsByID[item.id] = item;
      addTraceWork(state, item);
      return workRef(item);
    });
  const terminalWork = terminalWorkFromItems(state, outputItems, event.payload.outcome);
  const latestAttempt = latestWorkstationAttempt(state.inferenceAttemptsByDispatchID[dispatchID]);
  const terminalRefs = terminalWork ? [workRef(terminalWork.work_item)] : [];
  const workItemsByID = new Map(
    [...(active?.workItems ?? []), ...outputRefs, ...terminalRefs].map((item) => [
      item.work_id,
      item,
    ]),
  );
  const traceIDs = uniqueSorted([
    ...(active?.traceIDs ?? []),
    ...(event.context.traceIds ?? []),
    ...(event.context.workIds ?? []).map((workID) => state.workItemsByID[workID]?.trace_id ?? ""),
  ]);

  return {
    consumedTokens: active?.consumedTokens ?? [],
    currentChainingTraceID:
      event.context.currentChainingTraceId ??
      event.payload.currentChainingTraceId ??
      legacyPayload.current_chaining_trace_id ??
      active?.currentChainingTraceID ??
      traceIDs[0],
    diagnostics:
      latestAttempt?.diagnostics ?? dashboardDiagnosticsFromEvent(legacyPayload.diagnostics),
    dispatchID,
    durationMillis: event.payload.durationMillis ?? 0,
    endTime: event.context.eventTime,
    feedback: event.payload.feedback,
    responseText: event.payload.output,
    failureMessage: event.payload.failureMessage,
    failureReason: event.payload.failureReason,
    inputItems: active?.workItems ?? [],
    outcome: event.payload.outcome,
    outputItems: uniqueSortedWorkRefs([...outputRefs, ...terminalRefs]),
    outputMutations: [],
    previousChainingTraceIDs:
      event.context.previousChainingTraceIds ??
      event.payload.previousChainingTraceIds ??
      legacyPayload.previous_chaining_trace_ids ??
      active?.previousChainingTraceIDs,
    providerSession: latestAttempt?.provider_session ?? legacyPayload.providerSession,
    resources: active?.resources ?? [],
    startedAt: active?.startedAt ?? "",
    systemOnly:
      active?.systemOnly ??
      (event.payload.transitionId === SYSTEM_TIME_EXPIRY_TRANSITION_ID && workItemsByID.size === 0),
    terminalWork,
    traceIDs,
    transitionID: dashboardTransitionID(event.payload.transitionId),
    workItems: [...workItemsByID.values()].sort((left, right) => left.work_id.localeCompare(right.work_id)),
    workstationName: resolveWorkstationName(
      state.topology,
      event.payload.transitionId,
      legacyPayload.workstation?.name,
    ),
  };
}

export function recordFailedCompletion(state: WorldState, completion: WorldCompletion): void {
  const workItems =
    completion.outputItems.length > 0
      ? completion.outputItems
      : completion.terminalWork !== undefined
        ? [workRef(completion.terminalWork.work_item)]
        : completion.workItems;

  for (const item of workItems) {
    const existing = state.workItemsByID[item.work_id] ?? completion.terminalWork?.work_item;
    if (!existing) {
      continue;
    }
    state.workItemsByID[existing.id] = existing;
    state.failedWorkItemsByID[existing.id] = existing;
    state.failedWorkDetailsByWorkID[existing.id] = {
      dispatch_id: completion.dispatchID,
      failure_message: completion.failureMessage,
      failure_reason: completion.failureReason,
      transition_id: completion.transitionID,
      work_item: workRef(existing),
      workstation_name: completion.workstationName,
    };
  }
}

export function completionToProviderSession(
  completion: WorldCompletion,
): DashboardProviderSessionAttempt {
  return {
    diagnostics: completion.diagnostics,
    dispatch_id: completion.dispatchID,
    failure_message: completion.failureMessage,
    failure_reason: completion.failureReason,
    outcome: completion.outcome,
    provider_session: completion.providerSession,
    transition_id: completion.transitionID,
    work_items: completion.workItems.map(cloneWorkItemRef),
    workstation_name: completion.workstationName,
  };
}

export function completionToTraceDispatch(completion: WorldCompletion): DashboardTraceDispatch {
  return {
    consumed_tokens: completion.consumedTokens,
    current_chaining_trace_id: completion.currentChainingTraceID,
    diagnostics: completion.diagnostics,
    dispatch_id: completion.dispatchID,
    duration_millis: completion.durationMillis,
    end_time: completion.endTime,
    failure_message: completion.failureMessage,
    failure_reason: completion.failureReason,
    input_items: completion.inputItems,
    outcome: completion.outcome,
    output_items: completion.outputItems,
    output_mutations: completion.outputMutations,
    previous_chaining_trace_ids: completion.previousChainingTraceIDs,
    provider_session: completion.providerSession,
    start_time: completion.startedAt,
    trace_id: completion.traceIDs[0],
    trace_ids: completion.traceIDs,
    transition_id: completion.transitionID,
    work_ids: completion.workItems.map((item) => item.work_id),
    token_names: uniqueSorted(
      completion.workItems.map((item) => item.display_name?.trim() || item.work_id),
    ),
    work_types: uniqueSorted(completion.workItems.map((item) => item.work_type_id ?? "")),
    workstation_name: completion.workstationName,
  };
}
