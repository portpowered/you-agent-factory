import type {
  DashboardActiveExecution,
  DashboardInferenceAttempt,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardTrace,
  DashboardTraceDispatch,
  DashboardWorkItemRef,
  DashboardWorkstationNode,
} from "../../../api/dashboard";

export interface SelectedWorkItemExecutionDetails {
  dispatchID?: string;
  elapsedStartTimestamp?: string;
  inferenceAttempts: DashboardInferenceAttempt[];
  traceIDs: string[];
  workstationRequest?: DashboardRuntimeWorkstationRequest;
  workstationName?: string;
  workID: string;
}

export interface SelectWorkItemExecutionDetailsInput {
  activeExecution?: DashboardActiveExecution;
  dispatchID?: string;
  inferenceAttemptsByDispatchID?: Record<string, Record<string, DashboardInferenceAttempt>>;
  providerSessions?: DashboardProviderSessionAttempt[];
  selectedNode?: DashboardWorkstationNode;
  trace?: DashboardTrace;
  workItem: DashboardWorkItemRef;
  workstationRequestsByDispatchID?: Record<
    string,
    DashboardRuntimeWorkstationRequest
  >;
}


export function selectWorkItemExecutionDetails({
  activeExecution,
  dispatchID,
  inferenceAttemptsByDispatchID,
  providerSessions,
  selectedNode,
  trace,
  workItem,
  workstationRequestsByDispatchID,
}: SelectWorkItemExecutionDetailsInput): SelectedWorkItemExecutionDetails {
  const selectedDispatchID = dispatchID ?? activeExecution?.dispatch_id;
  const matchingAttempt = selectMatchingProviderAttempt(
    providerSessions ?? [],
    workItem.work_id,
    selectedDispatchID,
  );
  const matchingTraceDispatch = selectMatchingTraceDispatch(
    trace,
    workItem.work_id,
    selectedDispatchID,
  );
  const workstationRequest = selectWorkstationRequest(
    workstationRequestsByDispatchID,
    selectedDispatchID,
    matchingAttempt,
    matchingTraceDispatch,
  );
  const resolvedDispatchID =
    workstationRequest?.dispatchId ??
    selectedDispatchID ??
    matchingAttempt?.dispatch_id ??
    matchingTraceDispatch?.dispatch_id;

  return {
    dispatchID: resolvedDispatchID,
    elapsedStartTimestamp:
      workstationRequest?.request.startedAt ??
      activeExecution?.started_at ??
      matchingTraceDispatch?.start_time,
    inferenceAttempts: selectInferenceAttempts(
      inferenceAttemptsByDispatchID,
      resolvedDispatchID,
    ),
    traceIDs: selectTraceIDs(
      workstationRequest,
      activeExecution,
      workItem,
      trace,
      matchingTraceDispatch,
    ),
    workstationRequest,
    workstationName:
      workstationRequest?.workstationName ??
      activeExecution?.workstation_name ??
      selectedNode?.workstation_name ??
      matchingAttempt?.workstation_name ??
      matchingTraceDispatch?.workstation_name,
    workID: workItem.work_id,
  };
}

function selectWorkstationRequest(
  workstationRequestsByDispatchID:
    | Record<string, DashboardRuntimeWorkstationRequest>
    | undefined,
  dispatchID: string | undefined,
  matchingAttempt: DashboardProviderSessionAttempt | undefined,
  matchingTraceDispatch: DashboardTraceDispatch | undefined,
): DashboardRuntimeWorkstationRequest | undefined {
  const requestDispatchID =
    dispatchID ?? matchingAttempt?.dispatch_id ?? matchingTraceDispatch?.dispatch_id;
  if (!requestDispatchID) {
    return undefined;
  }
  return workstationRequestsByDispatchID?.[requestDispatchID];
}

function selectInferenceAttempts(
  attemptsByDispatchID: Record<string, Record<string, DashboardInferenceAttempt>> | undefined,
  dispatchID: string | undefined,
): DashboardInferenceAttempt[] {
  if (!dispatchID) {
    return [];
  }
  return Object.values(attemptsByDispatchID?.[dispatchID] ?? {}).sort((left, right) => {
    if (left.attempt !== right.attempt) {
      return left.attempt - right.attempt;
    }
    return left.inference_request_id.localeCompare(right.inference_request_id);
  });
}

function selectMatchingProviderAttempt(
  attempts: DashboardProviderSessionAttempt[],
  workID: string,
  dispatchID: string | undefined,
): DashboardProviderSessionAttempt | undefined {
  const matchingWorkAttempts = attempts.filter((attempt) =>
    attempt.work_items?.some((item) => item.work_id === workID),
  );
  const dispatchAttempts =
    dispatchID === undefined
      ? []
      : matchingWorkAttempts.filter((attempt) => attempt.dispatch_id === dispatchID);
  const candidates = dispatchAttempts.length > 0 ? dispatchAttempts : matchingWorkAttempts;
  return candidates[candidates.length - 1];
}

function selectMatchingTraceDispatch(
  trace: DashboardTrace | undefined,
  workID: string,
  dispatchID: string | undefined,
): DashboardTraceDispatch | undefined {
  const dispatches = trace?.dispatches ?? [];
  const dispatchMatches =
    dispatchID === undefined
      ? []
      : dispatches.filter((dispatch) => dispatch.dispatch_id === dispatchID);
  const workMatches = dispatches.filter((dispatch) => dispatch.work_ids?.includes(workID));
  const candidates = dispatchMatches.length > 0 ? dispatchMatches : workMatches;
  return candidates[candidates.length - 1];
}

function selectTraceIDs(
  workstationRequest: DashboardRuntimeWorkstationRequest | undefined,
  activeExecution: DashboardActiveExecution | undefined,
  workItem: DashboardWorkItemRef,
  trace: DashboardTrace | undefined,
  matchingTraceDispatch: DashboardTraceDispatch | undefined,
): string[] {
  return uniqueSorted([
    ...(workstationRequest?.request.traceIds ?? []),
    ...(activeExecution?.trace_ids ?? []),
    workItem.trace_id ?? "",
    trace?.trace_id ?? "",
    matchingTraceDispatch?.trace_id ?? "",
  ]);
}

function uniqueSorted(values: string[]): string[] {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))].sort();
}
