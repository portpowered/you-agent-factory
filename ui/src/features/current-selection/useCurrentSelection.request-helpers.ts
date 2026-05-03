import type {
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../api/dashboard/types";

export type DispatchWorkstationRequest =
  | DashboardRuntimeWorkstationRequest
  | DashboardWorkstationRequest;

export type WorkstationRequestLike =
  | DashboardRuntimeWorkstationRequest
  | DashboardWorkstationRequest;

function isProjectedWorkstationRequest(
  request: WorkstationRequestLike,
): request is DashboardWorkstationRequest {
  return "workstation_node_id" in request;
}

export function resolveProjectedWorkstationRequestsByDispatchID(
  snapshot: DashboardSnapshot | null | undefined,
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest> | undefined,
): Record<string, DashboardWorkstationRequest> | undefined {
  if (workstationRequestsByDispatchID && Object.keys(workstationRequestsByDispatchID).length > 0) {
    return workstationRequestsByDispatchID;
  }

  if (!snapshot?.runtime.workstation_requests_by_dispatch_id) {
    return undefined;
  }

  return Object.fromEntries(
    Object.entries(snapshot.runtime.workstation_requests_by_dispatch_id).map(
      ([dispatchID, request]) => [dispatchID, toDashboardWorkstationRequest(request)],
    ),
  );
}

export function filterProviderSessionAttempts(
  attempts: DashboardProviderSessionAttempt[] | undefined,
  predicate: (attempt: DashboardProviderSessionAttempt) => boolean,
): DashboardProviderSessionAttempt[] {
  if (!attempts) {
    return [];
  }

  return attempts.filter(
    (attempt) => attempt.provider_session?.id !== undefined && predicate(attempt),
  );
}

function filterDispatchAttempts(
  attempts: DashboardProviderSessionAttempt[] | undefined,
  predicate: (attempt: DashboardProviderSessionAttempt) => boolean,
): DashboardProviderSessionAttempt[] {
  return attempts ? attempts.filter(predicate) : [];
}

export function buildSelectedWorkDispatchAttempts({
  attempts,
  workID,
  workstationRequestsByDispatchID,
}: {
  attempts: DashboardProviderSessionAttempt[] | undefined;
  workID: string;
  workstationRequestsByDispatchID?: Record<string, DispatchWorkstationRequest>;
}): DashboardProviderSessionAttempt[] {
  const matchingAttempts = filterDispatchAttempts(
    attempts,
    (attempt) => attempt.work_items?.some((workItem) => workItem.work_id === workID) ?? false,
  );
  const requests = sortDispatchRequests(
    Object.values(workstationRequestsByDispatchID ?? {}).filter((request) =>
      requestWorkItems(request).some((workItem) => workItem.work_id === workID),
    ),
  );

  if (requests.length === 0) {
    return matchingAttempts;
  }

  const attemptsByDispatchID = new Map<string, DashboardProviderSessionAttempt>();
  for (const attempt of matchingAttempts) {
    attemptsByDispatchID.set(attempt.dispatch_id, attempt);
  }

  for (const request of requests) {
    const dispatchAttempt = dispatchAttemptFromRequest(request);
    const existingAttempt = attemptsByDispatchID.get(dispatchAttempt.dispatch_id);
    attemptsByDispatchID.set(
      dispatchAttempt.dispatch_id,
      existingAttempt ? mergeDispatchAttempts(existingAttempt, dispatchAttempt) : dispatchAttempt,
    );
  }

  const orderedDispatchIDs = [
    ...requests.map((request) => requestDispatchID(request)),
    ...matchingAttempts.map((attempt) => attempt.dispatch_id),
  ];

  return [...new Set(orderedDispatchIDs)]
    .map((dispatchID) => attemptsByDispatchID.get(dispatchID))
    .filter((attempt): attempt is DashboardProviderSessionAttempt => attempt !== undefined);
}

export function sortWorkstationRequests<TRequest extends WorkstationRequestLike>(
  requests: TRequest[],
): TRequest[] {
  return [...requests].sort((left, right) => {
    const leftStartedAt = requestStartedAt(left);
    const rightStartedAt = requestStartedAt(right);
    if (leftStartedAt !== rightStartedAt) {
      return rightStartedAt.localeCompare(leftStartedAt);
    }
    return requestDispatchID(left).localeCompare(requestDispatchID(right));
  });
}

export function selectWorkstationRequestsForWork<TRequest extends WorkstationRequestLike>(
  workstationRequestsByDispatchID: Record<string, TRequest> | undefined,
  workID: string,
): TRequest[] {
  return sortWorkstationRequests(
    Object.values(workstationRequestsByDispatchID ?? {}).filter((request) =>
      requestReferencesWorkItem(request, workID),
    ),
  );
}

export function selectLatestProviderSessionAttemptsByDispatch(
  attempts: DashboardProviderSessionAttempt[] | undefined,
  requests: WorkstationRequestLike[],
): DashboardProviderSessionAttempt[] {
  if (!attempts) {
    return [];
  }

  const latestAttemptsByDispatchID = new Map<string, DashboardProviderSessionAttempt>();
  for (const attempt of attempts) {
    if (!attempt.provider_session?.id) {
      continue;
    }
    latestAttemptsByDispatchID.set(attempt.dispatch_id, attempt);
  }

  return requests.flatMap((request) => {
    const matchingAttempt = latestAttemptsByDispatchID.get(requestDispatchID(request));
    return matchingAttempt ? [matchingAttempt] : [];
  });
}

export function requestWorkItems(request: DispatchWorkstationRequest): DashboardWorkItemRef[] {
  return isProjectedWorkstationRequest(request)
    ? request.work_items
    : [...requestInputWorkItems(request), ...requestOutputWorkItems(request)];
}

export function requestDispatchID(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): string {
  return isProjectedWorkstationRequest(request)
    ? request.dispatch_id
    : request.dispatchId ?? request.dispatch_id ?? "";
}

export function toDashboardWorkstationRequest(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): DashboardWorkstationRequest {
  if ("workstation_node_id" in request) {
    return request;
  }

  return {
    dispatch_id: request.dispatchId ?? request.dispatch_id ?? "",
    dispatched_request_count: request.counts.dispatchedCount ?? request.counts.dispatched_count ?? 0,
    errored_request_count: request.counts.erroredCount ?? request.counts.errored_count ?? 0,
    failure_message: request.response?.failureMessage ?? request.response?.failure_message,
    failure_reason: request.response?.failureReason ?? request.response?.failure_reason,
    inference_attempts: [],
    model: request.request.model,
    outcome: request.response?.outcome,
    prompt: request.request.prompt,
    provider: request.request.provider,
    provider_session: request.response?.providerSession ?? request.response?.provider_session,
    request_metadata: request.request.requestMetadata ?? request.request.request_metadata,
    responded_request_count: request.counts.respondedCount ?? request.counts.responded_count ?? 0,
    response: request.response?.responseText ?? request.response?.response_text,
    response_metadata: request.response?.responseMetadata ?? request.response?.response_metadata,
    script_request: request.request.scriptRequest ?? request.request.script_request,
    script_response: request.response?.scriptResponse ?? request.response?.script_response,
    started_at: request.request.startedAt ?? request.request.started_at,
    total_duration_millis: request.response?.durationMillis ?? request.response?.duration_millis,
    trace_ids: request.request.traceIds ?? request.request.trace_ids,
    transition_id: request.transitionId ?? request.transition_id ?? "",
    work_items: requestWorkItems(request),
    working_directory: request.request.workingDirectory ?? request.request.working_directory,
    workstation_name: request.workstationName ?? request.workstation_name,
    workstation_node_id: request.transitionId ?? request.transition_id ?? "",
    worktree: request.request.worktree,
  };
}

function sortDispatchRequests(requests: DispatchWorkstationRequest[]): DispatchWorkstationRequest[] {
  return [...requests].sort((left, right) => {
    if (requestStartedAt(left) !== requestStartedAt(right)) {
      return requestStartedAt(right).localeCompare(requestStartedAt(left));
    }
    return requestDispatchID(left).localeCompare(requestDispatchID(right));
  });
}

function dispatchAttemptFromRequest(
  request: DispatchWorkstationRequest,
): DashboardProviderSessionAttempt {
  return {
    diagnostics: requestDiagnostics(request),
    dispatch_id: requestDispatchID(request),
    failure_message: requestFailureMessage(request),
    failure_reason: requestFailureReason(request),
    outcome: requestOutcome(request),
    provider_session: requestProviderSession(request),
    transition_id: requestTransitionID(request),
    work_items: requestWorkItems(request),
    workstation_name: requestWorkstationName(request),
  };
}

function mergeDispatchAttempts(
  existingAttempt: DashboardProviderSessionAttempt,
  derivedAttempt: DashboardProviderSessionAttempt,
): DashboardProviderSessionAttempt {
  return {
    ...derivedAttempt,
    ...existingAttempt,
    diagnostics: existingAttempt.diagnostics ?? derivedAttempt.diagnostics,
    failure_message: existingAttempt.failure_message ?? derivedAttempt.failure_message,
    failure_reason: existingAttempt.failure_reason ?? derivedAttempt.failure_reason,
    outcome: existingAttempt.outcome || derivedAttempt.outcome,
    provider_session: existingAttempt.provider_session ?? derivedAttempt.provider_session,
    work_items: existingAttempt.work_items ?? derivedAttempt.work_items,
    workstation_name: existingAttempt.workstation_name ?? derivedAttempt.workstation_name,
  };
}

function requestStartedAt(request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest): string {
  return isProjectedWorkstationRequest(request)
    ? request.started_at ?? ""
    : request.request.startedAt ?? request.request.started_at ?? "";
}

function requestTransitionID(request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest): string {
  return isProjectedWorkstationRequest(request)
    ? request.transition_id
    : request.transitionId ?? request.transition_id ?? "";
}

function requestWorkstationName(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): string | undefined {
  return isProjectedWorkstationRequest(request)
    ? request.workstation_name
    : request.workstationName ?? request.workstation_name;
}

function requestReferencesWorkItem(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
  workID: string,
): boolean {
  return requestRelatedWorkItems(request).some((workItem) => workItem.work_id === workID);
}

function requestRelatedWorkItems(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): DashboardWorkItemRef[] {
  return dedupeWorkItems([
    ...requestWorkItems(request),
    ...requestInputWorkItems(request),
    ...requestOutputWorkItems(request),
  ]);
}

function dedupeWorkItems(workItems: DashboardWorkItemRef[]): DashboardWorkItemRef[] {
  const workItemsByID = new Map<string, DashboardWorkItemRef>();
  for (const workItem of workItems) {
    workItemsByID.set(workItem.work_id, workItem);
  }
  return [...workItemsByID.values()];
}

function requestInputWorkItems(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): DashboardWorkItemRef[] {
  return isProjectedWorkstationRequest(request)
    ? request.request_view?.input_work_items ?? []
    : request.request.inputWorkItems ?? request.request.input_work_items ?? [];
}

function requestOutputWorkItems(
  request: DashboardRuntimeWorkstationRequest | DashboardWorkstationRequest,
): DashboardWorkItemRef[] {
  return isProjectedWorkstationRequest(request)
    ? request.response_view?.output_work_items ?? []
    : request.response?.outputWorkItems ?? request.response?.output_work_items ?? [];
}

function requestProviderSession(request: DispatchWorkstationRequest) {
  return "request" in request
    ? request.response?.providerSession ?? request.response?.provider_session
    : request.provider_session;
}

function requestOutcome(request: DispatchWorkstationRequest): string {
  return "request" in request ? request.response?.outcome ?? "PENDING" : request.outcome ?? "PENDING";
}

function requestFailureReason(request: DispatchWorkstationRequest): string | undefined {
  return "request" in request
    ? request.response?.failureReason ?? request.response?.failure_reason
    : request.failure_reason;
}

function requestFailureMessage(request: DispatchWorkstationRequest): string | undefined {
  return "request" in request
    ? request.response?.failureMessage ?? request.response?.failure_message
    : request.failure_message;
}

function requestDiagnostics(request: DispatchWorkstationRequest) {
  return "request" in request ? request.response?.diagnostics : undefined;
}
