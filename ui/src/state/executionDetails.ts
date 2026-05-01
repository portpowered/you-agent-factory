import type {
  DashboardActiveExecution,
  DashboardInferenceAttempt,
  DashboardProviderSession,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardTrace,
  DashboardTraceDispatch,
  DashboardWorkDiagnostics,
  DashboardWorkItemRef,
  DashboardWorkstationNode,
} from "../api/dashboard";

type ExecutionDetailSource =
  | "active-execution"
  | "provider-diagnostics"
  | "provider-session"
  | "trace"
  | "workstation-request"
  | "workstation";

export type ExecutionDetailValue =
  | { status: "available"; source: ExecutionDetailSource; value: string }
  | { status: "pending" }
  | { status: "unavailable" };

export type ModelDetailValue = ExecutionDetailValue | { status: "omitted" };

export type PromptDiagnosticDetails =
  | {
      status: "available";
      source: "diagnostics";
      systemPromptHash?: string;
      promptSource?: string;
    }
  | { status: "pending" }
  | { status: "unavailable" };

export interface SelectedWorkItemExecutionDetails {
  dispatchID?: string;
  elapsedStartTimestamp?: string;
  inferenceAttempts: DashboardInferenceAttempt[];
  model: ModelDetailValue;
  prompt: PromptDiagnosticDetails;
  provider: ExecutionDetailValue;
  providerSessionData?: DashboardProviderSession;
  providerSession: ExecutionDetailValue;
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

interface RuntimeDiagnosticsSource {
  diagnostics?: DashboardWorkDiagnostics;
  source: ExecutionDetailSource;
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
  const hasActiveRun =
    activeExecution !== undefined &&
    (activeExecution.dispatch_id === selectedDispatchID ||
      executionIncludesWorkItem(activeExecution, workItem.work_id));
  const diagnosticsSource = selectDiagnosticsSource(
    workstationRequest,
    activeExecution,
    matchingAttempt,
    matchingTraceDispatch,
  );
  const resolvedDispatchID =
    workstationRequest?.dispatch_id ??
    selectedDispatchID ??
    matchingAttempt?.dispatch_id ??
    matchingTraceDispatch?.dispatch_id;

  return {
    dispatchID: resolvedDispatchID,
    elapsedStartTimestamp:
      workstationRequest?.request.started_at ??
      activeExecution?.started_at ??
      matchingTraceDispatch?.start_time,
    inferenceAttempts: selectInferenceAttempts(
      inferenceAttemptsByDispatchID,
      resolvedDispatchID,
    ),
    model: selectModelValue(
      workstationRequest,
      diagnosticsSource,
      activeExecution,
      hasActiveRun,
    ),
    prompt: selectPromptDetails(
      workstationRequest,
      diagnosticsSource?.diagnostics,
      hasActiveRun,
    ),
    provider: selectProviderValue(
      workstationRequest,
      diagnosticsSource,
      activeExecution,
      selectedNode,
      matchingAttempt,
      matchingTraceDispatch,
      hasActiveRun,
    ),
    providerSession: selectProviderSessionValue(
      workstationRequest,
      matchingAttempt,
      matchingTraceDispatch,
      hasActiveRun,
    ),
    providerSessionData:
      workstationRequest?.response?.provider_session ??
      matchingAttempt?.provider_session ??
      matchingTraceDispatch?.provider_session,
    traceIDs: selectTraceIDs(
      workstationRequest,
      activeExecution,
      workItem,
      trace,
      matchingTraceDispatch,
    ),
    workstationRequest,
    workstationName:
      workstationRequest?.workstation_name ??
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

function executionIncludesWorkItem(
  execution: DashboardActiveExecution,
  workID: string,
): boolean {
  return execution.work_items?.some((item) => item.work_id === workID) ?? false;
}

function selectDiagnosticsSource(
  workstationRequest: DashboardRuntimeWorkstationRequest | undefined,
  activeExecution: DashboardActiveExecution | undefined,
  matchingAttempt: DashboardProviderSessionAttempt | undefined,
  matchingTraceDispatch: DashboardTraceDispatch | undefined,
): RuntimeDiagnosticsSource | undefined {
  if (workstationRequest?.response?.diagnostics) {
    return {
      diagnostics: workstationRequest.response.diagnostics,
      source: "workstation-request",
    };
  }
  if (activeExecution?.diagnostics) {
    return { diagnostics: activeExecution.diagnostics, source: "active-execution" };
  }
  if (matchingAttempt?.diagnostics) {
    return { diagnostics: matchingAttempt.diagnostics, source: "provider-diagnostics" };
  }
  if (matchingTraceDispatch?.diagnostics) {
    return { diagnostics: matchingTraceDispatch.diagnostics, source: "trace" };
  }
  return undefined;
}

function selectModelValue(
  workstationRequest: DashboardRuntimeWorkstationRequest | undefined,
  diagnosticsSource: RuntimeDiagnosticsSource | undefined,
  activeExecution: DashboardActiveExecution | undefined,
  hasActiveRun: boolean,
): ModelDetailValue {
  const projectedModel = workstationRequest?.request.model?.trim();
  if (projectedModel) {
    return {
      source: "workstation-request",
      status: "available",
      value: projectedModel,
    };
  }
  if (workstationRequest) {
    return hasActiveRun ? { status: "pending" } : { status: "unavailable" };
  }
  const diagnosticModel = diagnosticsSource?.diagnostics?.provider?.model?.trim();
  if (diagnosticsSource && diagnosticModel) {
    return {
      source: diagnosticsSource.source,
      status: "available",
      value: diagnosticModel,
    };
  }
  const activeModel = activeExecution?.model?.trim();
  if (activeModel) {
    return { source: "active-execution", status: "available", value: activeModel };
  }
  return hasActiveRun ? { status: "pending" } : { status: "omitted" };
}

function selectProviderValue(
  workstationRequest: DashboardRuntimeWorkstationRequest | undefined,
  diagnosticsSource: RuntimeDiagnosticsSource | undefined,
  activeExecution: DashboardActiveExecution | undefined,
  selectedNode: DashboardWorkstationNode | undefined,
  matchingAttempt: DashboardProviderSessionAttempt | undefined,
  matchingTraceDispatch: DashboardTraceDispatch | undefined,
  hasActiveRun: boolean,
): ExecutionDetailValue {
  const projectedProvider = workstationRequest?.request.provider?.trim();
  if (projectedProvider) {
    return {
      source: "workstation-request",
      status: "available",
      value: projectedProvider,
    };
  }
  if (workstationRequest) {
    return hasActiveRun ? { status: "pending" } : { status: "unavailable" };
  }
  const diagnosticProvider = diagnosticsSource?.diagnostics?.provider?.provider?.trim();
  if (diagnosticsSource && diagnosticProvider) {
    return {
      source: diagnosticsSource.source,
      status: "available",
      value: diagnosticProvider,
    };
  }
  const providerSessionProvider =
    matchingAttempt?.provider_session?.provider?.trim() ??
    matchingTraceDispatch?.provider_session?.provider?.trim();
  if (providerSessionProvider) {
    return {
      source: "provider-session",
      status: "available",
      value: providerSessionProvider,
    };
  }
  const activeProvider = activeExecution?.provider?.trim();
  if (activeProvider) {
    return { source: "active-execution", status: "available", value: activeProvider };
  }
  const workstationProvider = selectedNode?.provider?.trim();
  if (workstationProvider) {
    return { source: "workstation", status: "available", value: workstationProvider };
  }
  return hasActiveRun ? { status: "pending" } : { status: "unavailable" };
}

function selectProviderSessionValue(
  workstationRequest: DashboardRuntimeWorkstationRequest | undefined,
  matchingAttempt: DashboardProviderSessionAttempt | undefined,
  matchingTraceDispatch: DashboardTraceDispatch | undefined,
  hasActiveRun: boolean,
): ExecutionDetailValue {
  const projectedProviderSessionID =
    workstationRequest?.response?.provider_session?.id?.trim();
  if (projectedProviderSessionID) {
    return {
      source: "workstation-request",
      status: "available",
      value: projectedProviderSessionID,
    };
  }
  if (workstationRequest) {
    return hasActiveRun ? { status: "pending" } : { status: "unavailable" };
  }
  const providerSessionID =
    matchingAttempt?.provider_session?.id?.trim() ??
    matchingTraceDispatch?.provider_session?.id?.trim();
  if (providerSessionID) {
    return { source: "provider-session", status: "available", value: providerSessionID };
  }
  return hasActiveRun ? { status: "pending" } : { status: "unavailable" };
}

function selectPromptDetails(
  workstationRequest: DashboardRuntimeWorkstationRequest | undefined,
  diagnostics: DashboardWorkDiagnostics | undefined,
  hasActiveRun: boolean,
): PromptDiagnosticDetails {
  const promptSource =
    workstationRequest?.request.request_metadata?.prompt_source?.trim() ??
    workstationRequest?.request.request_metadata?.source?.trim() ??
    diagnostics?.rendered_prompt?.variables?.prompt_source?.trim() ??
    diagnostics?.provider?.request_metadata?.prompt_source?.trim() ??
    diagnostics?.provider?.request_metadata?.source?.trim();
  const systemPromptHash = diagnostics?.rendered_prompt?.system_prompt_hash?.trim();
  if (systemPromptHash || promptSource) {
    return {
      promptSource,
      source: "diagnostics",
      status: "available",
      systemPromptHash,
    };
  }
  if (workstationRequest) {
    return hasActiveRun ? { status: "pending" } : { status: "unavailable" };
  }
  return hasActiveRun ? { status: "pending" } : { status: "unavailable" };
}

function selectTraceIDs(
  workstationRequest: DashboardRuntimeWorkstationRequest | undefined,
  activeExecution: DashboardActiveExecution | undefined,
  workItem: DashboardWorkItemRef,
  trace: DashboardTrace | undefined,
  matchingTraceDispatch: DashboardTraceDispatch | undefined,
): string[] {
  return uniqueSorted([
    ...(workstationRequest?.request.trace_ids ?? []),
    ...(activeExecution?.trace_ids ?? []),
    workItem.trace_id ?? "",
    trace?.trace_id ?? "",
    matchingTraceDispatch?.trace_id ?? "",
  ]);
}

function uniqueSorted(values: string[]): string[] {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))].sort();
}
