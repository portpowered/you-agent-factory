import type {
  SelectedWorkRequestHistoryItem,
} from "./detail-card-types";

export function isProjectedWorkstationRequest(
  request: SelectedWorkRequestHistoryItem,
): request is Extract<SelectedWorkRequestHistoryItem, { workstation_node_id: string }> {
  return "workstation_node_id" in request;
}

export function requestCounts(request: SelectedWorkRequestHistoryItem) {
  if (isProjectedWorkstationRequest(request)) {
    return {
      dispatchedCount:
        request.counts?.dispatched_count ?? request.dispatched_request_count ?? 0,
      erroredCount: request.counts?.errored_count ?? request.errored_request_count ?? 0,
      respondedCount:
        request.counts?.responded_count ?? request.responded_request_count ?? 0,
    };
  }

  return {
    dispatchedCount: request.counts.dispatched_count,
    erroredCount: request.counts.errored_count,
    respondedCount: request.counts.responded_count,
  };
}

export function requestInputWorkItems(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.request_view?.input_work_items ?? []
    : request.request.input_work_items ?? [];
}

export function requestOutputWorkItems(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.response_view?.output_work_items ?? []
    : request.response?.output_work_items ?? [];
}

export function requestPrompt(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.prompt ?? request.request_view?.prompt
    : request.request.prompt;
}

export function requestProvider(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.provider ?? request.request_view?.provider
    : request.request.provider;
}

export function requestModel(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.model ?? request.request_view?.model
    : request.request.model;
}

export function requestTraceIDs(request: SelectedWorkRequestHistoryItem) {
  const traceIDs = isProjectedWorkstationRequest(request)
    ? [...(request.trace_ids ?? []), ...(request.request_view?.trace_ids ?? [])]
    : request.request.trace_ids ?? [];

  return [...new Set(traceIDs.filter(Boolean))];
}

export function requestStartedAt(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.started_at ?? request.request_view?.started_at ?? request.request_view?.request_time
    : request.request.started_at ?? request.request.request_time;
}

export function requestDurationMillis(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.total_duration_millis ??
        request.response_view?.duration_millis ??
        request.script_response?.duration_millis ??
        request.response_view?.script_response?.duration_millis
    : request.response?.duration_millis ?? request.response?.script_response?.duration_millis;
}

export function requestWorkingDirectory(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.working_directory ?? request.request_view?.working_directory
    : request.request.working_directory;
}

export function requestWorktree(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.worktree ?? request.request_view?.worktree
    : request.request.worktree;
}

export function requestOutcome(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.outcome ??
        request.response_view?.outcome ??
        request.script_response?.outcome ??
        request.response_view?.script_response?.outcome
    : request.response?.outcome ?? request.response?.script_response?.outcome;
}

export function requestProviderSession(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.provider_session ?? request.response_view?.provider_session
    : request.response?.provider_session;
}

export function requestResponseText(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.response ?? request.response_view?.response_text
    : request.response?.response_text;
}

export function requestFailureReason(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.failure_reason ?? request.response_view?.failure_reason
    : request.response?.failure_reason;
}

export function requestFailureMessage(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.failure_message ?? request.response_view?.failure_message
    : request.response?.failure_message;
}

export function requestErrorClass(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.response_view?.error_class
    : request.response?.error_class;
}

export function requestScriptRequest(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.script_request ?? request.request_view?.script_request
    : request.request.script_request;
}

export function requestScriptResponse(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.script_response ?? request.response_view?.script_response
    : request.response?.script_response;
}

export function hasResponseDetails(request: SelectedWorkRequestHistoryItem) {
  return Boolean(
    requestOutcome(request) ||
      requestScriptResponse(request) ||
      requestProviderSession(request)?.id ||
      requestResponseText(request) ||
      requestFailureReason(request) ||
      requestFailureMessage(request) ||
      requestErrorClass(request) ||
      requestOutputWorkItems(request).length > 0,
  );
}

export function dedupeWorkItems<TWorkItem extends { work_id: string }>(workItems: TWorkItem[]) {
  return [...new Map(workItems.map((workItem) => [workItem.work_id, workItem])).values()];
}
