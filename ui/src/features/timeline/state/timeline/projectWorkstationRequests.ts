import type {
  DashboardInferenceAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard";
import {
  type TimelineScriptRequest,
  type TimelineScriptResponse,
  type TimelineWorkstationRequest,
  type TimelineWorkstationRequestCounts,
  toDashboardRuntimeWorkstationRequest,
} from "./workstationRequestModel";
import {
  attemptHasError,
  attemptHasResponse,
  dispatchHasCustomerWork,
  latestWorkstationAttempt,
  latestWorkstationScriptResponse,
  outputWorkItemsFromCompletion,
  projectWorkstationDispatchRequest,
  requestIDsByWorkItemID,
  resolveWorkingDirectory,
  resolveWorkstationRequestProvider,
  resolveWorktree,
  scriptResponseErrored,
  workstationRequestMetadata,
  workstationResponseMetadata,
  workstationScriptRequestForProjection,
  workItemsFromTokens,
} from "./projectWorkstationRequestHelpers";
import { uniqueSorted } from "./shared";
import type {
  TimelineWorkRequestPayload,
  WorldCompletion,
  WorldDispatch,
  WorldScriptRequest,
  WorldScriptResponse,
} from "./types";

export function projectRuntimeWorkstationRequests({
  activeDispatches,
  attemptsByDispatchID,
  completedDispatches,
  scriptRequestsByDispatchID,
  scriptResponsesByDispatchID,
}: {
  activeDispatches: WorldDispatch[];
  attemptsByDispatchID: Record<string, Record<string, DashboardInferenceAttempt>>;
  completedDispatches: WorldCompletion[];
  scriptRequestsByDispatchID: Record<string, Record<string, WorldScriptRequest>>;
  scriptResponsesByDispatchID: Record<string, Record<string, WorldScriptResponse>>;
}): Record<string, DashboardRuntimeWorkstationRequest> | undefined {
  const requests: Record<string, TimelineWorkstationRequest> = {};

  for (const dispatch of activeDispatches) {
    const latestScriptResponse = latestWorkstationScriptResponse(
      scriptResponsesByDispatchID[dispatch.dispatchID],
    );
    const latestScriptRequest = workstationScriptRequestForProjection(
      latestScriptResponse,
      scriptRequestsByDispatchID[dispatch.dispatchID],
    );
    requests[dispatch.dispatchID] = workstationRequestFromActiveDispatch(
      dispatch,
      attemptsByDispatchID[dispatch.dispatchID],
      latestScriptRequest,
    );
  }

  for (const completion of completedDispatches) {
    const latestScriptResponse = latestWorkstationScriptResponse(
      scriptResponsesByDispatchID[completion.dispatchID],
    );
    const latestScriptRequest = workstationScriptRequestForProjection(
      latestScriptResponse,
      scriptRequestsByDispatchID[completion.dispatchID],
    );
    requests[completion.dispatchID] = workstationRequestFromCompletion(
      completion,
      attemptsByDispatchID[completion.dispatchID],
      latestScriptRequest,
      latestScriptResponse,
    );
  }

  for (const dispatchID of Object.keys(requests)) {
    requests[dispatchID] = {
      ...requests[dispatchID],
      counts: workstationRequestCounts(
        attemptsByDispatchID[dispatchID],
        scriptRequestsByDispatchID[dispatchID],
        scriptResponsesByDispatchID[dispatchID],
      ),
    };
  }

  return Object.keys(requests).length > 0
    ? Object.fromEntries(
        Object.entries(requests).map(([dispatchID, request]) => [
          dispatchID,
          toDashboardRuntimeWorkstationRequest(request),
        ]),
      )
    : undefined;
}

export function projectWorkstationDispatchRequestsByID({
  activeDispatches,
  completedDispatches,
  inferenceAttemptsByDispatchID,
  runtimeRequestsByDispatchID,
  scriptRequestsByDispatchID,
  scriptResponsesByDispatchID,
  workRequestsByID,
}: {
  activeDispatches: Record<string, WorldDispatch>;
  completedDispatches: WorldCompletion[];
  inferenceAttemptsByDispatchID: Record<string, Record<string, DashboardInferenceAttempt>>;
  runtimeRequestsByDispatchID: Record<string, DashboardRuntimeWorkstationRequest>;
  scriptRequestsByDispatchID: Record<string, Record<string, WorldScriptRequest>>;
  scriptResponsesByDispatchID: Record<string, Record<string, WorldScriptResponse>>;
  workRequestsByID: Record<string, TimelineWorkRequestPayload>;
}): Record<string, DashboardWorkstationRequest> {
  const requestIDsByWorkID = requestIDsByWorkItemID(workRequestsByID);
  const dispatchRequests = new Map<string, DashboardWorkstationRequest>();

  for (const dispatch of Object.values(activeDispatches)) {
    if (!dispatchHasCustomerWork(dispatch)) {
      continue;
    }

    dispatchRequests.set(
      dispatch.dispatchID,
      projectWorkstationDispatchRequest(
        dispatch,
        undefined,
        runtimeRequestsByDispatchID[dispatch.dispatchID],
        inferenceAttemptsByDispatchID,
        scriptRequestsByDispatchID,
        scriptResponsesByDispatchID,
        requestIDsByWorkID,
        workstationRequestCounts,
      ),
    );
  }

  for (const completion of completedDispatches) {
    if (!dispatchHasCustomerWork(completion)) {
      continue;
    }

    dispatchRequests.set(
      completion.dispatchID,
      projectWorkstationDispatchRequest(
        completion,
        completion,
        runtimeRequestsByDispatchID[completion.dispatchID],
        inferenceAttemptsByDispatchID,
        scriptRequestsByDispatchID,
        scriptResponsesByDispatchID,
        requestIDsByWorkID,
        workstationRequestCounts,
      ),
    );
  }

  return Object.fromEntries(
    [...dispatchRequests.entries()].sort(([left], [right]) => left.localeCompare(right)),
  );
}

function workstationRequestFromActiveDispatch(
  dispatch: WorldDispatch,
  attempts: Record<string, DashboardInferenceAttempt> | undefined,
  latestScriptRequest: WorldScriptRequest | undefined,
): TimelineWorkstationRequest {
  const inputWorkItems = workItemsFromTokens(dispatch.consumedTokens, dispatch.workItems);
  const latestAttempt = latestWorkstationAttempt(attempts);
  return {
    counts: workstationRequestCounts(undefined, undefined, undefined),
    dispatchId: dispatch.dispatchID,
    request: {
      consumedTokens: dispatch.consumedTokens,
      currentChainingTraceId: dispatch.currentChainingTraceID,
      inputWorkItems,
      inputWorkTypeIds: uniqueSorted(inputWorkItems.map((item) => item.work_type_id ?? "")),
      model: dispatch.model,
      previousChainingTraceIds: dispatch.previousChainingTraceIDs
        ? [...dispatch.previousChainingTraceIDs]
        : undefined,
      prompt: latestAttempt?.prompt,
      provider: resolveWorkstationRequestProvider(undefined, undefined, dispatch),
      requestMetadata: workstationRequestMetadata(undefined),
      requestTime: latestAttempt?.request_time,
      scriptRequest: timelineScriptRequest(latestScriptRequest),
      startedAt: dispatch.startedAt,
      traceIds: uniqueSorted(dispatch.traceIDs),
      workingDirectory: resolveWorkingDirectory(latestAttempt, undefined),
      worktree: resolveWorktree(latestAttempt, undefined),
    },
    transitionId: dispatch.transitionID,
    workstationName: dispatch.workstationName,
  };
}

function workstationRequestFromCompletion(
  completion: WorldCompletion,
  attempts: Record<string, DashboardInferenceAttempt> | undefined,
  latestScriptRequest: WorldScriptRequest | undefined,
  latestScriptResponse: WorldScriptResponse | undefined,
): TimelineWorkstationRequest {
  const inputWorkItems = workItemsFromTokens(completion.consumedTokens, completion.workItems);
  const latestAttempt = latestWorkstationAttempt(attempts);
  return {
    counts: workstationRequestCounts(undefined, undefined, undefined),
    dispatchId: completion.dispatchID,
    request: {
      consumedTokens: completion.consumedTokens,
      currentChainingTraceId: completion.currentChainingTraceID,
      inputWorkItems,
      inputWorkTypeIds: uniqueSorted(inputWorkItems.map((item) => item.work_type_id ?? "")),
      model: completion.diagnostics?.provider?.model,
      previousChainingTraceIds: completion.previousChainingTraceIDs
        ? [...completion.previousChainingTraceIDs]
        : undefined,
      prompt: latestAttempt?.prompt,
      provider: resolveWorkstationRequestProvider(
        completion.diagnostics,
        completion.providerSession,
      ),
      requestMetadata: workstationRequestMetadata(completion.diagnostics),
      requestTime: latestAttempt?.request_time,
      scriptRequest: timelineScriptRequest(latestScriptRequest),
      startedAt: completion.startedAt,
      traceIds: uniqueSorted(completion.traceIDs),
      workingDirectory: resolveWorkingDirectory(latestAttempt, completion.diagnostics),
      worktree: resolveWorktree(latestAttempt, completion.diagnostics),
    },
    response: {
      diagnostics: completion.diagnostics,
      durationMillis: completion.durationMillis,
      endTime: completion.endTime,
      errorClass: latestAttempt?.error_class,
      failureMessage: completion.failureMessage,
      failureReason: completion.failureReason,
      feedback: completion.feedback,
      outcome: completion.outcome,
      outputMutations: completion.outputMutations,
      outputWorkItems: outputWorkItemsFromCompletion(completion),
      providerSession: completion.providerSession,
      responseMetadata: workstationResponseMetadata(completion.diagnostics),
      responseText:
        latestAttempt?.response ?? (latestScriptResponse ? undefined : completion.responseText),
      scriptResponse: timelineScriptResponse(latestScriptResponse),
    },
    transitionId: completion.transitionID,
    workstationName: completion.workstationName,
  };
}

function workstationRequestCounts(
  attempts: Record<string, DashboardInferenceAttempt> | undefined,
  scriptRequests: Record<string, WorldScriptRequest> | undefined,
  scriptResponses: Record<string, WorldScriptResponse> | undefined,
): TimelineWorkstationRequestCounts {
  const counts: TimelineWorkstationRequestCounts = {
    dispatchedCount: 0,
    erroredCount: 0,
    respondedCount: 0,
  };
  for (const attempt of Object.values(attempts ?? {})) {
    if (attempt.inference_request_id) {
      counts.dispatchedCount += 1;
    }
    if (attemptHasError(attempt)) {
      counts.erroredCount += 1;
      continue;
    }
    if (attemptHasResponse(attempt)) {
      counts.respondedCount += 1;
    }
  }
  for (const request of Object.values(scriptRequests ?? {})) {
    if (request.script_request_id) {
      counts.dispatchedCount += 1;
    }
  }
  for (const response of Object.values(scriptResponses ?? {})) {
    if (!response.response_time) {
      continue;
    }
    if (scriptResponseErrored(response)) {
      counts.erroredCount += 1;
      continue;
    }
    counts.respondedCount += 1;
  }
  return counts;
}

function timelineScriptRequest(
  request: WorldScriptRequest | undefined,
): TimelineScriptRequest | undefined {
  if (!request) {
    return undefined;
  }
  return {
    args: request.args.length > 0 ? [...request.args] : undefined,
    attempt: request.attempt,
    command: request.command,
    scriptRequestId: request.script_request_id,
  };
}

function timelineScriptResponse(
  response: WorldScriptResponse | undefined,
): TimelineScriptResponse | undefined {
  if (!response) {
    return undefined;
  }
  return {
    attempt: response.attempt,
    durationMillis: response.duration_millis,
    exitCode: response.exit_code,
    failureType: response.failure_type,
    outcome: response.outcome,
    scriptRequestId: response.script_request_id,
    stderr: response.stderr,
    stdout: response.stdout,
  };
}


