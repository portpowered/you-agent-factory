import type {
  DashboardInferenceAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardRuntimeWorkstationRequestResponse,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard";
import type { FactoryWorkItem } from "../../../../api/events";
import {
  attemptHasError,
  attemptHasResponse,
  dispatchHasCustomerWork,
  latestWorkstationScriptResponse,
  outputWorkItemsFromCompletion,
  projectWorkstationDispatchRequest,
  requestIDsByWorkItemID,
  scriptResponseErrored,
  workstationScriptRequestForProjection,
} from "./projectWorkstationRequestHelpers";
import { uniqueSorted } from "./shared";
import type {
  TimelineWorkRequestPayload,
  WorldCompletion,
  WorldDispatch,
  WorldScriptRequest,
  WorldScriptResponse,
} from "./types";
import { consumedWorkItemRefsForDispatch } from "./workItemRef";
import type { WorkPayloadLineageProjection } from "./workPayloadLineage";
import {
  type TimelineScriptRequest,
  type TimelineScriptResponse,
  type TimelineWorkstationRequest,
  type TimelineWorkstationRequestCounts,
  toDashboardRuntimeWorkstationRequest,
} from "./workstationRequestModel";

export function projectRuntimeWorkstationRequests({
  activeDispatches,
  attemptsByDispatchID,
  completedDispatches,
  payloadLineage,
  scriptRequestsByDispatchID,
  scriptResponsesByDispatchID,
  textBlobsByID,
  workItemsByID,
}: {
  activeDispatches: WorldDispatch[];
  attemptsByDispatchID: Record<
    string,
    Record<string, DashboardInferenceAttempt>
  >;
  completedDispatches: WorldCompletion[];
  payloadLineage: WorkPayloadLineageProjection;
  scriptRequestsByDispatchID: Record<
    string,
    Record<string, WorldScriptRequest>
  >;
  scriptResponsesByDispatchID: Record<
    string,
    Record<string, WorldScriptResponse>
  >;
  textBlobsByID?: Record<string, string>;
  workItemsByID: Record<string, FactoryWorkItem>;
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
      payloadLineage,
      workItemsByID,
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
      payloadLineage,
      textBlobsByID,
      workItemsByID,
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
  textBlobsByID,
  workRequestsByID,
}: {
  activeDispatches: Record<string, WorldDispatch>;
  completedDispatches: WorldCompletion[];
  inferenceAttemptsByDispatchID: Record<
    string,
    Record<string, DashboardInferenceAttempt>
  >;
  runtimeRequestsByDispatchID: Record<
    string,
    DashboardRuntimeWorkstationRequest
  >;
  scriptRequestsByDispatchID: Record<
    string,
    Record<string, WorldScriptRequest>
  >;
  scriptResponsesByDispatchID: Record<
    string,
    Record<string, WorldScriptResponse>
  >;
  workRequestsByID: Record<string, TimelineWorkRequestPayload>;
  textBlobsByID?: Record<string, string>;
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
        textBlobsByID,
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
        textBlobsByID,
      ),
    );
  }

  return Object.fromEntries(
    [...dispatchRequests.entries()].sort(([left], [right]) =>
      left.localeCompare(right),
    ),
  );
}

function workstationRequestFromActiveDispatch(
  dispatch: WorldDispatch,
  _attempts: Record<string, DashboardInferenceAttempt> | undefined,
  latestScriptRequest: WorldScriptRequest | undefined,
  payloadLineage: WorkPayloadLineageProjection,
  workItemsByID: Record<string, FactoryWorkItem>,
): TimelineWorkstationRequest {
  const inputWorkItems = consumedWorkItemRefsForDispatch(
    payloadLineage,
    dispatch.dispatchID,
    dispatch.consumedTokens,
    workItemsByID,
  );
  return {
    counts: workstationRequestCounts(undefined, undefined, undefined),
    dispatchId: dispatch.dispatchID,
    request: {
      consumedTokens: dispatch.consumedTokens,
      currentChainingTraceId: dispatch.currentChainingTraceID,
      inputWorkItems,
      inputWorkTypeIds: uniqueSorted(
        inputWorkItems.map((item) => item.work_type_id ?? ""),
      ),
      previousChainingTraceIds: dispatch.previousChainingTraceIDs
        ? [...dispatch.previousChainingTraceIDs]
        : undefined,
      scriptRequest: timelineScriptRequest(latestScriptRequest),
      startedAt: dispatch.startedAt,
      traceIds: uniqueSorted(dispatch.traceIDs),
    },
    transitionId: dispatch.transitionID,
    workstationName: dispatch.workstationName,
  };
}

function workstationRequestFromCompletion(
  completion: WorldCompletion,
  _attempts: Record<string, DashboardInferenceAttempt> | undefined,
  latestScriptRequest: WorldScriptRequest | undefined,
  latestScriptResponse: WorldScriptResponse | undefined,
  payloadLineage: WorkPayloadLineageProjection,
  textBlobsByID: Record<string, string> | undefined,
  workItemsByID: Record<string, FactoryWorkItem>,
): TimelineWorkstationRequest {
  const inputWorkItems = consumedWorkItemRefsForDispatch(
    payloadLineage,
    completion.dispatchID,
    completion.consumedTokens,
    workItemsByID,
  );
  return {
    counts: workstationRequestCounts(undefined, undefined, undefined),
    dispatchId: completion.dispatchID,
    request: {
      consumedTokens: completion.consumedTokens,
      currentChainingTraceId: completion.currentChainingTraceID,
      inputWorkItems,
      inputWorkTypeIds: uniqueSorted(
        inputWorkItems.map((item) => item.work_type_id ?? ""),
      ),
      previousChainingTraceIds: completion.previousChainingTraceIDs
        ? [...completion.previousChainingTraceIDs]
        : undefined,
      scriptRequest: timelineScriptRequest(latestScriptRequest),
      startedAt: completion.startedAt,
      traceIds: uniqueSorted(completion.traceIDs),
    },
    response: {
      durationMillis: completion.durationMillis,
      endTime: completion.endTime,
      failureDetail:
        completion.failureReason && completion.failureMessage
          ? {
              reason: timelineFailureReason(completion.failureReason),
              message: completion.failureMessage,
            }
          : undefined,
      feedback:
        completion.feedbackTextBlobID && textBlobsByID
          ? textBlobsByID[completion.feedbackTextBlobID]
          : completion.feedback,
      outcome: completion.outcome,
      outputMutations: completion.outputMutations,
      outputWorkItems: outputWorkItemsFromCompletion(
        payloadLineage,
        completion,
        workItemsByID,
      ),
      scriptResponse: timelineScriptResponse(
        latestScriptResponse,
        textBlobsByID,
      ),
    },
    transitionId: completion.transitionID,
    workstationName: completion.workstationName,
  };
}

function timelineFailureReason(
  reason: string,
): NonNullable<
  DashboardRuntimeWorkstationRequestResponse["failureDetail"]
>["reason"] {
  switch (reason) {
    case "auth_failure":
    case "permanent_bad_request":
    case "throttled":
    case "internal_server_error":
    case "timeout":
    case "misconfigured":
      return reason;
    default:
      return "unknown";
  }
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
  textBlobsByID?: Record<string, string>,
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
    stderr: response.stderrTextBlobID
      ? (textBlobsByID?.[response.stderrTextBlobID] ?? response.stderr)
      : response.stderr,
    stdout: response.stdoutTextBlobID
      ? (textBlobsByID?.[response.stdoutTextBlobID] ?? response.stdout)
      : response.stdout,
  };
}
