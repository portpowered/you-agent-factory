import type {
  DashboardInferenceAttempt,
  DashboardProviderSessionAttempt,
  DashboardTraceDispatch,
} from "../../api/dashboard";
import type {
  DispatchResponsePayload,
  FactoryWorkDiagnostics,
  InferenceRequestPayload,
  InferenceResponsePayload,
} from "../../api/events";
import type { LegacyDispatchResponsePayloadCompat } from "./replayWorldStateTypes";
import type {
  WorldCompletion,
  WorldScriptRequest,
  WorldScriptResponse,
  WorldState,
} from "./types";

interface CompletedAttemptSyncHelpers {
  completionToProviderSession: (completion: WorldCompletion) => DashboardProviderSessionAttempt;
  uniqueSorted: (values: string[]) => string[];
}

export function inferenceAttemptsForDispatch(
  state: WorldState,
  dispatchID: string,
): Record<string, DashboardInferenceAttempt> {
  const existing = state.inferenceAttemptsByDispatchID[dispatchID];
  if (existing) {
    return existing;
  }
  const attempts: Record<string, DashboardInferenceAttempt> = {};
  state.inferenceAttemptsByDispatchID[dispatchID] = attempts;
  return attempts;
}

export function resolveDispatchTransitionID(
  state: WorldState,
  dispatchID: string,
): string | undefined {
  const activeTransitionID = state.activeDispatches[dispatchID]?.transitionID;
  if (activeTransitionID) {
    return activeTransitionID;
  }
  for (let index = state.completedDispatches.length - 1; index >= 0; index -= 1) {
    const completion = state.completedDispatches[index];
    if (completion.dispatchID === dispatchID) {
      return completion.transitionID;
    }
  }
  return undefined;
}

export function syncCompletedDispatchAttempt(
  state: WorldState,
  dispatchID: string,
  attempt: DashboardInferenceAttempt,
  helpers: CompletedAttemptSyncHelpers,
): void {
  for (let index = state.completedDispatches.length - 1; index >= 0; index -= 1) {
    const completion = state.completedDispatches[index];
    if (completion.dispatchID !== dispatchID) {
      continue;
    }

    completion.diagnostics = attempt.diagnostics ?? completion.diagnostics;
    completion.providerSession = attempt.provider_session ?? completion.providerSession;

    if (
      completion.providerSession?.id &&
      !state.providerSessions.some(
        (providerSession) =>
          providerSession.dispatch_id === completion.dispatchID &&
          providerSession.provider_session?.id === completion.providerSession?.id,
      )
    ) {
      state.providerSessions = [
        ...state.providerSessions,
        helpers.completionToProviderSession(completion),
      ];
    }

    for (const traceID of completion.traceIDs) {
      const trace = state.tracesByID[traceID];
      if (!trace) {
        continue;
      }
      trace.dispatches = trace.dispatches.map((dispatch: DashboardTraceDispatch) =>
        dispatch.dispatch_id === completion.dispatchID
          ? completionToTraceDispatch(helpers, completion)
          : dispatch,
      );
    }

    break;
  }
}

export function completionToTraceDispatch(
  helpers: CompletedAttemptSyncHelpers,
  completion: WorldCompletion,
): DashboardTraceDispatch {
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
    token_names: helpers.uniqueSorted(
      completion.workItems.map((item) => item.display_name?.trim() || item.work_id),
    ),
    work_types: helpers.uniqueSorted(
      completion.workItems.map((item) => item.work_type_id ?? ""),
    ),
    workstation_name: completion.workstationName,
  };
}

export function legacyInferencePayloadDispatchID(
  payload: InferenceRequestPayload | InferenceResponsePayload,
): string | undefined {
  const dispatchID = (payload as { dispatchId?: unknown }).dispatchId;
  return typeof dispatchID === "string" && dispatchID.length > 0 ? dispatchID : undefined;
}

export function legacyInferencePayloadTransitionID(
  payload: InferenceRequestPayload | InferenceResponsePayload,
): string | undefined {
  const transitionID = (payload as { transitionId?: unknown }).transitionId;
  return typeof transitionID === "string" && transitionID.length > 0
    ? transitionID
    : undefined;
}

export function scriptRequestsForDispatch(
  state: WorldState,
  dispatchID: string,
): Record<string, WorldScriptRequest> {
  const existing = state.scriptRequestsByDispatchID[dispatchID];
  if (existing) {
    return existing;
  }
  const requests: Record<string, WorldScriptRequest> = {};
  state.scriptRequestsByDispatchID[dispatchID] = requests;
  return requests;
}

export function scriptResponsesForDispatch(
  state: WorldState,
  dispatchID: string,
): Record<string, WorldScriptResponse> {
  const existing = state.scriptResponsesByDispatchID[dispatchID];
  if (existing) {
    return existing;
  }
  const responses: Record<string, WorldScriptResponse> = {};
  state.scriptResponsesByDispatchID[dispatchID] = responses;
  return responses;
}

export function legacyDispatchResponsePayload(
  payload: DispatchResponsePayload,
): LegacyDispatchResponsePayloadCompat {
  return payload as DispatchResponsePayload & LegacyDispatchResponsePayloadCompat;
}

export function applyScriptRequest(
  state: WorldState,
  event: {
    payload: {
      dispatchId?: string;
      scriptRequestId?: string;
      args: string[];
      attempt: number;
      command: string;
      transitionId: string;
    };
    context: { eventTime: string };
  },
): void {
  const { payload } = event;
  if (!payload.dispatchId || !payload.scriptRequestId) {
    return;
  }
  const requests = scriptRequestsForDispatch(state, payload.dispatchId);
  requests[payload.scriptRequestId] = {
    args: [...payload.args],
    attempt: payload.attempt,
    command: payload.command,
    dispatch_id: payload.dispatchId,
    request_time: event.context.eventTime,
    script_request_id: payload.scriptRequestId,
    transition_id: payload.transitionId,
  };
}

export function applyScriptResponse(
  state: WorldState,
  event: {
    payload: {
      dispatchId?: string;
      scriptRequestId?: string;
      attempt: number;
      durationMillis: number;
      exitCode?: number;
      failureType?: string;
      outcome: string;
      stderr: string;
      stdout: string;
      transitionId: string;
    };
    context: { eventTime: string };
  },
): void {
  const { payload } = event;
  if (!payload.dispatchId || !payload.scriptRequestId) {
    return;
  }
  const responses = scriptResponsesForDispatch(state, payload.dispatchId);
  responses[payload.scriptRequestId] = {
    attempt: payload.attempt,
    dispatch_id: payload.dispatchId,
    duration_millis: payload.durationMillis,
    exit_code: payload.exitCode,
    failure_type: payload.failureType,
    outcome: payload.outcome,
    response_time: event.context.eventTime,
    script_request_id: payload.scriptRequestId,
    stderr: payload.stderr,
    stdout: payload.stdout,
    transition_id: payload.transitionId,
  };
}
