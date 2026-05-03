import type {
  DashboardRuntimeWorkstationRequest,
  DashboardRuntimeWorkstationRequestCounts,
  DashboardRuntimeWorkstationRequestRequest,
  DashboardRuntimeWorkstationRequestResponse,
  DashboardScriptRequest,
  DashboardScriptResponse,
  DashboardTraceToken,
  DashboardWorkDiagnostics,
  DashboardWorkItemRef,
  DashboardTraceMutation,
  DashboardProviderSession,
} from "../../api/dashboard";

export interface TimelineScriptRequest {
  args?: string[];
  attempt?: number;
  command?: string;
  scriptRequestId?: string;
}

export interface TimelineScriptResponse {
  attempt?: number;
  durationMillis?: number;
  exitCode?: number;
  failureType?: string;
  outcome?: string;
  scriptRequestId?: string;
  stderr?: string;
  stdout?: string;
}

export interface TimelineWorkstationRequestCounts {
  dispatchedCount: number;
  erroredCount: number;
  respondedCount: number;
}

export interface TimelineWorkstationRequestRequest {
  consumedTokens?: DashboardTraceToken[];
  currentChainingTraceId?: string;
  inputWorkItems?: DashboardWorkItemRef[];
  inputWorkTypeIds?: string[];
  model?: string;
  previousChainingTraceIds?: string[];
  prompt?: string;
  provider?: string;
  requestMetadata?: Record<string, string>;
  requestTime?: string;
  scriptRequest?: TimelineScriptRequest;
  startedAt?: string;
  traceIds?: string[];
  workingDirectory?: string;
  worktree?: string;
}

export interface TimelineWorkstationRequestResponse {
  diagnostics?: DashboardWorkDiagnostics | DashboardRuntimeWorkstationRequestResponse["diagnostics"];
  durationMillis?: number;
  endTime?: string;
  errorClass?: string;
  failureMessage?: string;
  failureReason?: string;
  feedback?: string;
  outcome?: string;
  outputMutations?: DashboardTraceMutation[];
  outputWorkItems?: DashboardWorkItemRef[];
  providerSession?: DashboardProviderSession;
  responseMetadata?: Record<string, string>;
  responseText?: string;
  scriptResponse?: TimelineScriptResponse;
}

export interface TimelineWorkstationRequest {
  counts: TimelineWorkstationRequestCounts;
  dispatchId: string;
  request: TimelineWorkstationRequestRequest;
  response?: TimelineWorkstationRequestResponse;
  transitionId: string;
  workstationName?: string;
}

export function toDashboardScriptRequest(
  request: TimelineScriptRequest | undefined,
): DashboardScriptRequest | undefined {
  if (!request) {
    return undefined;
  }

  return {
    args: request.args ? [...request.args] : undefined,
    attempt: request.attempt,
    command: request.command,
    scriptRequestId: request.scriptRequestId,
    script_request_id: request.scriptRequestId,
  };
}

export function toDashboardScriptResponse(
  response: TimelineScriptResponse | undefined,
): DashboardScriptResponse | undefined {
  if (!response) {
    return undefined;
  }

  return {
    attempt: response.attempt,
    durationMillis: response.durationMillis,
    duration_millis: response.durationMillis,
    exitCode: response.exitCode,
    exit_code: response.exitCode,
    failureType: response.failureType,
    failure_type: response.failureType,
    outcome: response.outcome,
    scriptRequestId: response.scriptRequestId,
    script_request_id: response.scriptRequestId,
    stderr: response.stderr,
    stdout: response.stdout,
  };
}

export function toDashboardRuntimeWorkstationRequestCounts(
  counts: TimelineWorkstationRequestCounts,
): DashboardRuntimeWorkstationRequestCounts {
  return {
    dispatchedCount: counts.dispatchedCount,
    dispatched_count: counts.dispatchedCount,
    erroredCount: counts.erroredCount,
    errored_count: counts.erroredCount,
    respondedCount: counts.respondedCount,
    responded_count: counts.respondedCount,
  };
}

export function toDashboardRuntimeWorkstationRequestRequest(
  request: TimelineWorkstationRequestRequest,
): DashboardRuntimeWorkstationRequestRequest {
  return {
    consumedTokens: request.consumedTokens,
    consumed_tokens: request.consumedTokens,
    currentChainingTraceId: request.currentChainingTraceId,
    current_chaining_trace_id: request.currentChainingTraceId,
    inputWorkItems: request.inputWorkItems,
    input_work_items: request.inputWorkItems,
    inputWorkTypeIds: request.inputWorkTypeIds,
    input_work_type_ids: request.inputWorkTypeIds,
    model: request.model,
    previousChainingTraceIds: request.previousChainingTraceIds
      ? [...request.previousChainingTraceIds]
      : undefined,
    previous_chaining_trace_ids: request.previousChainingTraceIds
      ? [...request.previousChainingTraceIds]
      : undefined,
    prompt: request.prompt,
    provider: request.provider,
    requestMetadata: request.requestMetadata ? { ...request.requestMetadata } : undefined,
    request_metadata: request.requestMetadata ? { ...request.requestMetadata } : undefined,
    requestTime: request.requestTime,
    request_time: request.requestTime,
    scriptRequest: toDashboardScriptRequest(request.scriptRequest),
    script_request: toDashboardScriptRequest(request.scriptRequest),
    startedAt: request.startedAt,
    started_at: request.startedAt,
    traceIds: request.traceIds ? [...request.traceIds] : undefined,
    trace_ids: request.traceIds ? [...request.traceIds] : undefined,
    workingDirectory: request.workingDirectory,
    working_directory: request.workingDirectory,
    worktree: request.worktree,
  };
}

export function toDashboardRuntimeWorkstationRequestResponse(
  response: TimelineWorkstationRequestResponse | undefined,
): DashboardRuntimeWorkstationRequestResponse | undefined {
  if (!response) {
    return undefined;
  }

  return {
    diagnostics: response.diagnostics,
    durationMillis: response.durationMillis,
    duration_millis: response.durationMillis,
    endTime: response.endTime,
    end_time: response.endTime,
    errorClass: response.errorClass,
    error_class: response.errorClass,
    failureMessage: response.failureMessage,
    failure_message: response.failureMessage,
    failureReason: response.failureReason,
    failure_reason: response.failureReason,
    feedback: response.feedback,
    outcome: response.outcome,
    outputMutations: response.outputMutations,
    output_mutations: response.outputMutations,
    outputWorkItems: response.outputWorkItems,
    output_work_items: response.outputWorkItems,
    providerSession: response.providerSession,
    provider_session: response.providerSession,
    responseMetadata: response.responseMetadata ? { ...response.responseMetadata } : undefined,
    response_metadata: response.responseMetadata ? { ...response.responseMetadata } : undefined,
    responseText: response.responseText,
    response_text: response.responseText,
    scriptResponse: toDashboardScriptResponse(response.scriptResponse),
    script_response: toDashboardScriptResponse(response.scriptResponse),
  };
}

export function toDashboardRuntimeWorkstationRequest(
  request: TimelineWorkstationRequest,
): DashboardRuntimeWorkstationRequest {
  return {
    counts: toDashboardRuntimeWorkstationRequestCounts(request.counts),
    dispatchId: request.dispatchId,
    dispatch_id: request.dispatchId,
    request: toDashboardRuntimeWorkstationRequestRequest(request.request),
    response: toDashboardRuntimeWorkstationRequestResponse(request.response),
    transitionId: request.transitionId,
    transition_id: request.transitionId,
    workstationName: request.workstationName,
    workstation_name: request.workstationName,
  };
}
