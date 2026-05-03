import type {
  DashboardInferenceAttempt,
  DashboardProviderDiagnostic,
  DashboardRuntimeWorkstationRequest,
  DashboardScriptRequest,
  DashboardScriptResponse,
  DashboardTraceToken,
  DashboardWorkDiagnostics,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard";
import type { FactoryProviderSession, } from "../../../../api/events";
import { uniqueSorted } from "./shared";
import type {
  TimelineWorkRequestPayload,
  WorldCompletion,
  WorldDispatch,
  WorldScriptRequest,
  WorldScriptResponse,
} from "./types";
import { workRef } from "./workItemRef";

const DASHBOARD_TIME_WORK_TYPE_ID = "time";

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

export function latestWorkstationScriptRequest(
  requests: Record<string, WorldScriptRequest> | undefined,
): WorldScriptRequest | undefined {
  return Object.values(requests ?? {}).sort((left, right) => {
    if (left.attempt !== right.attempt) {
      return right.attempt - left.attempt;
    }
    if (left.request_time !== right.request_time) {
      return right.request_time.localeCompare(left.request_time);
    }
    return right.script_request_id.localeCompare(left.script_request_id);
  })[0];
}

export function latestWorkstationScriptResponse(
  responses: Record<string, WorldScriptResponse> | undefined,
): WorldScriptResponse | undefined {
  return Object.values(responses ?? {}).sort((left, right) => {
    if (left.attempt !== right.attempt) {
      return right.attempt - left.attempt;
    }
    if (left.response_time !== right.response_time) {
      return right.response_time.localeCompare(left.response_time);
    }
    return right.script_request_id.localeCompare(left.script_request_id);
  })[0];
}

export function workstationScriptRequestForProjection(
  response: WorldScriptResponse | undefined,
  requests: Record<string, WorldScriptRequest> | undefined,
): WorldScriptRequest | undefined {
  if (response?.script_request_id && requests?.[response.script_request_id]) {
    return requests[response.script_request_id];
  }
  return latestWorkstationScriptRequest(requests);
}

export function dashboardScriptRequest(
  request: WorldScriptRequest | undefined,
): DashboardScriptRequest | undefined {
  if (!request) {
    return undefined;
  }
  return {
    args: request.args.length > 0 ? [...request.args] : undefined,
    attempt: request.attempt,
    command: request.command,
    script_request_id: request.script_request_id,
  };
}

export function dashboardScriptResponse(
  response: WorldScriptResponse | undefined,
): DashboardScriptResponse | undefined {
  if (!response) {
    return undefined;
  }
  return {
    attempt: response.attempt,
    duration_millis: response.duration_millis,
    exit_code: response.exit_code,
    failure_type: response.failure_type,
    outcome: response.outcome,
    script_request_id: response.script_request_id,
    stderr: response.stderr,
    stdout: response.stdout,
  };
}

export function scriptResponseErrored(response: WorldScriptResponse): boolean {
  return (
    response.failure_type !== undefined ||
    response.outcome === "FAILED_EXIT_CODE" ||
    response.outcome === "PROCESS_ERROR" ||
    response.outcome === "TIMED_OUT"
  );
}

export function resolveWorkstationRequestProvider(
  diagnostics: DashboardWorkDiagnostics | undefined,
  providerSession?: FactoryProviderSession,
  dispatch?: WorldDispatch,
): string | undefined {
  return (
    diagnostics?.provider?.provider ??
    providerSession?.provider ??
    dispatch?.modelProvider ??
    dispatch?.provider
  );
}

export function resolveWorkingDirectory(
  attempt: DashboardInferenceAttempt | undefined,
  diagnostics: DashboardWorkDiagnostics | undefined,
): string | undefined {
  return (
    attempt?.working_directory ??
    diagnostics?.provider?.request_metadata?.working_directory
  );
}

export function resolveWorktree(
  attempt: DashboardInferenceAttempt | undefined,
  diagnostics: DashboardWorkDiagnostics | undefined,
): string | undefined {
  return attempt?.worktree ?? diagnostics?.provider?.request_metadata?.worktree;
}

export function workstationRequestMetadata(
  diagnostics: DashboardWorkDiagnostics | undefined,
): Record<string, string> | undefined {
  const requestMetadata = diagnostics?.provider?.request_metadata;
  return requestMetadata ? { ...requestMetadata } : undefined;
}

export function workstationResponseMetadata(
  diagnostics: DashboardWorkDiagnostics | undefined,
): Record<string, string> | undefined {
  const responseMetadata = diagnostics?.provider?.response_metadata;
  return responseMetadata ? { ...responseMetadata } : undefined;
}

export function workItemsFromTokens(
  tokens: DashboardTraceToken[],
  fallback: DashboardWorkItemRef[],
): DashboardWorkItemRef[] {
  const fallbackByID = new Map(fallback.map((item) => [item.work_id, item] as const));
  const workItems = Object.values(
    Object.fromEntries(
      tokens
        .filter(
          (token) => token.work_id && token.work_type_id !== DASHBOARD_TIME_WORK_TYPE_ID,
        )
        .map((token) => [
          token.work_id,
          {
            ...(fallbackByID.get(token.work_id)?.current_chaining_trace_id
              ? {
                  current_chaining_trace_id:
                    fallbackByID.get(token.work_id)?.current_chaining_trace_id,
                }
              : {}),
            display_name: token.name,
            ...(fallbackByID.get(token.work_id)?.previous_chaining_trace_ids
              ? {
                  previous_chaining_trace_ids:
                    fallbackByID.get(token.work_id)?.previous_chaining_trace_ids,
                }
              : {}),
            trace_id: token.trace_id,
            work_id: token.work_id,
            work_type_id: token.work_type_id,
          } satisfies DashboardWorkItemRef,
        ]),
    ),
  );
  return workItems.length > 0
    ? workItems.sort((left, right) => left.work_id.localeCompare(right.work_id))
    : [...fallback].sort((left, right) => left.work_id.localeCompare(right.work_id));
}

export function outputWorkItemsFromCompletion(
  completion: WorldCompletion,
): DashboardWorkItemRef[] | undefined {
  const workItems: Record<string, DashboardWorkItemRef> = Object.fromEntries(
    completion.outputItems
      .filter((item) => item.work_type_id !== DASHBOARD_TIME_WORK_TYPE_ID)
      .map((item) => [item.work_id, item] as const),
  );
  for (const mutation of completion.outputMutations) {
    const token = mutation.resulting_token;
    if (!token?.work_id || token.work_type_id === DASHBOARD_TIME_WORK_TYPE_ID) {
      continue;
    }
    workItems[token.work_id] = {
      display_name: token.name,
      trace_id: token.trace_id,
      work_id: token.work_id,
      work_type_id: token.work_type_id,
    };
  }
  if (
    completion.terminalWork?.work_item &&
    completion.terminalWork.work_item.work_type_id !== DASHBOARD_TIME_WORK_TYPE_ID
  ) {
    const item = completion.terminalWork.work_item;
    workItems[item.id] = workRef(item);
  }
  const values = Object.values(workItems).sort((left, right) =>
    left.work_id.localeCompare(right.work_id),
  );
  return values.length > 0 ? values : undefined;
}

export function sortInferenceAttempts(
  attempts: DashboardInferenceAttempt[],
): DashboardInferenceAttempt[] {
  return [...attempts].sort((left, right) => {
    if (left.attempt !== right.attempt) {
      return left.attempt - right.attempt;
    }
    return left.inference_request_id.localeCompare(right.inference_request_id);
  });
}

export function attemptHasResponse(attempt: DashboardInferenceAttempt): boolean {
  return attempt.response_time !== undefined && !attemptHasError(attempt);
}

export function attemptHasError(attempt: DashboardInferenceAttempt): boolean {
  return attempt.error_class !== undefined || attempt.outcome === "FAILED";
}

export function requestIDsByWorkItemID(
  workRequestsByID: Record<string, TimelineWorkRequestPayload>,
): Record<string, string[]> {
  const requestIDsByWorkID: Record<string, string[]> = {};

  for (const [requestID, request] of Object.entries(workRequestsByID)) {
    for (const item of request.work_items ?? []) {
      requestIDsByWorkID[item.id] = uniqueSorted([
        ...(requestIDsByWorkID[item.id] ?? []),
        requestID,
      ]);
    }
  }

  return requestIDsByWorkID;
}

export function projectWorkstationDispatchRequest(
  dispatch: WorldDispatch,
  completion: WorldCompletion | undefined,
  runtimeRequest: DashboardRuntimeWorkstationRequest | undefined,
  inferenceAttemptsByDispatchID: Record<string, Record<string, DashboardInferenceAttempt>>,
  scriptRequestsByDispatchID: Record<string, Record<string, WorldScriptRequest>>,
  scriptResponsesByDispatchID: Record<string, Record<string, WorldScriptResponse>>,
  requestIDsByWorkID: Record<string, string[]>,
  workstationRequestCounts: (
    attempts: Record<string, DashboardInferenceAttempt> | undefined,
    scriptRequests: Record<string, WorldScriptRequest> | undefined,
    scriptResponses: Record<string, WorldScriptResponse> | undefined,
  ) => { dispatchedCount?: number; erroredCount?: number; respondedCount?: number },
): DashboardWorkstationRequest {
  const inferenceAttempts = sortInferenceAttempts(
    Object.values(inferenceAttemptsByDispatchID[dispatch.dispatchID] ?? {}),
  );
  const latestAttempt = inferenceAttempts[inferenceAttempts.length - 1];
  const latestScriptResponse = latestWorkstationScriptResponse(
    scriptResponsesByDispatchID[dispatch.dispatchID],
  );
  const latestScriptRequest = workstationScriptRequestForProjection(
    latestScriptResponse,
    scriptRequestsByDispatchID[dispatch.dispatchID],
  );
  const projectedCounts = workstationRequestCounts(
    inferenceAttemptsByDispatchID[dispatch.dispatchID],
    scriptRequestsByDispatchID[dispatch.dispatchID],
    scriptResponsesByDispatchID[dispatch.dispatchID],
  );
  const requestIDs = uniqueSorted(
    dispatch.workItems.flatMap((item) => requestIDsByWorkID[item.work_id] ?? []),
  );
  const requestView = runtimeRequest?.request;
  const responseView = runtimeRequest?.response;
  const diagnostics = (
    responseView?.diagnostics?.provider ??
    latestAttempt?.diagnostics?.provider ??
    completion?.diagnostics?.provider
  ) as
    | (DashboardProviderDiagnostic & {
        requestMetadata?: Record<string, string>;
        responseMetadata?: Record<string, string>;
      })
    | undefined;
  const counts = runtimeRequest?.counts ?? projectedCounts;

  return {
    counts,
    dispatch_id: dispatch.dispatchID,
    dispatched_request_count: counts.dispatchedCount ?? 0,
    errored_request_count: counts.erroredCount ?? 0,
    failure_message: responseView?.failureMessage ?? completion?.failureMessage,
    failure_reason: responseView?.failureReason ?? completion?.failureReason,
    inference_attempts: inferenceAttempts,
    model: requestView?.model ?? diagnostics?.model ?? completion?.model ?? dispatch.model,
    outcome: responseView?.outcome ?? completion?.outcome,
    prompt: requestView?.prompt ?? latestAttempt?.prompt,
    provider:
      requestView?.provider ??
      diagnostics?.provider ??
      latestAttempt?.provider_session?.provider ??
      completion?.providerSession?.provider ??
      dispatch.provider,
    provider_session: responseView?.providerSession ?? completion?.providerSession,
    request_view: requestView,
    request_id: requestIDs[0],
    request_metadata: requestView?.requestMetadata ?? diagnostics?.requestMetadata,
    responded_request_count: counts.respondedCount ?? 0,
    response:
      responseView?.responseText ??
      latestAttempt?.response ??
      (latestScriptResponse ? undefined : completion?.responseText),
    response_metadata: responseView?.responseMetadata ?? diagnostics?.responseMetadata,
    response_view: responseView,
    script_request: dashboardScriptRequest(latestScriptRequest),
    script_response: dashboardScriptResponse(latestScriptResponse),
    started_at: requestView?.startedAt ?? dispatch.startedAt,
    total_duration_millis: responseView?.durationMillis ?? completion?.durationMillis,
    trace_ids: requestView?.traceIds ?? completion?.traceIDs ?? dispatch.traceIDs,
    transition_id: dispatch.transitionID,
    work_items: completion?.workItems ?? dispatch.workItems,
    working_directory: requestView?.workingDirectory ?? latestAttempt?.working_directory,
    workstation_name: completion?.workstationName ?? dispatch.workstationName,
    workstation_node_id: dispatch.transitionID,
    worktree: requestView?.worktree ?? latestAttempt?.worktree,
  };
}

export function dispatchHasCustomerWork(dispatch: WorldDispatch): boolean {
  return !dispatch.systemOnly;
}


