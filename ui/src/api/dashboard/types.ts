import type { InferenceOutcome } from "../events";
import type { components } from "../generated/openapi";

type FactorySchemas = components["schemas"];

export type StateCategory = "INITIAL" | "PROCESSING" | "TERMINAL" | "FAILED";
export type DashboardPlaceKind =
  | "work_state"
  | "resource"
  | "constraint"
  | "limit";
export type DashboardEdgeOutcomeKind =
  | "accepted"
  | "continue"
  | "rejected"
  | "failed";

export interface DashboardPlaceRef {
  place_id: string;
  type_id?: string;
  state_value?: string;
  kind: DashboardPlaceKind;
  state_category?: StateCategory;
}

export interface DashboardWorkstationNode {
  node_id: string;
  transition_id: string;
  workstation_name: string;
  worker_type?: string;
  workstation_kind?: string;
  provider?: string;
  model_provider?: string;
  model?: string;
  input_places?: DashboardPlaceRef[];
  output_places?: DashboardPlaceRef[];
  input_place_ids?: string[];
  output_place_ids?: string[];
  input_work_type_ids?: string[];
  output_work_type_ids?: string[];
}

export interface DashboardWorkstationEdge {
  edge_id: string;
  from_node_id: string;
  to_node_id: string;
  via_place_id: string;
  work_type_id?: string;
  state_value?: string;
  state_category?: StateCategory;
  outcome_kind?: DashboardEdgeOutcomeKind;
}

export interface DashboardSubmitWorkType {
  work_type_name: string;
}

export interface DashboardTopology {
  submit_work_types?: DashboardSubmitWorkType[];
  workstation_node_ids: string[];
  workstation_nodes_by_id: Record<string, DashboardWorkstationNode>;
  edges: DashboardWorkstationEdge[];
}

export interface DashboardWorkItemRef {
  current_chaining_trace_id?: string;
  work_id: string;
  work_type_id?: string;
  display_name?: string;
  previous_chaining_trace_ids?: string[];
  trace_id?: string;
}

export interface DashboardProviderSession {
  provider?: string;
  kind?: string;
  id?: string;
  local_jsonl_path?: string;
  session_log_url?: string;
}

export interface DashboardWorkDiagnostics {
  rendered_prompt?: DashboardRenderedPromptDiagnostic;
  provider?: DashboardProviderDiagnostic;
}

export interface DashboardRenderedPromptDiagnostic {
  system_prompt_hash?: string;
  user_message_hash?: string;
  variables?: Record<string, string>;
}

export interface DashboardProviderDiagnostic {
  provider?: string;
  model?: string;
  request_metadata?: Record<string, string>;
  response_metadata?: Record<string, string>;
}

export interface DashboardProviderSessionAttempt {
  dispatch_id: string;
  transition_id: string;
  workstation_name?: string;
  work_items?: DashboardWorkItemRef[];
  outcome: string;
  failure_reason?: string;
  failure_message?: string;
  provider_session?: DashboardProviderSession;
  diagnostics?: DashboardWorkDiagnostics;
}

export interface DashboardFailedWorkDetail {
  work_item: DashboardWorkItemRef;
  dispatch_id: string;
  transition_id: string;
  workstation_name?: string;
  failure_reason?: string;
  failure_message?: string;
}

export interface DashboardActiveExecution {
  dispatch_id: string;
  workstation_node_id: string;
  transition_id: string;
  workstation_name?: string;
  started_at: string;
  provider?: string;
  model_provider?: string;
  model?: string;
  diagnostics?: DashboardWorkDiagnostics;
  work_type_ids?: string[];
  work_items?: DashboardWorkItemRef[];
  trace_ids?: string[];
  consumed_tokens?: DashboardTraceToken[];
  output_mutations?: DashboardTraceMutation[];
}

export interface DashboardInferenceAttempt {
  attempt: number;
  diagnostics?: DashboardWorkDiagnostics;
  dispatch_id: string;
  duration_millis?: number;
  error_class?: string;
  exit_code?: number;
  inference_request_id: string;
  outcome?: InferenceOutcome;
  prompt: string;
  provider_session?: DashboardProviderSession;
  request_time: string;
  response?: string;
  response_time?: string;
  transition_id: string;
  working_directory?: string;
  worktree?: string;
}

export interface DashboardScriptRequest {
  scriptRequestId?: string;
  script_request_id?: string;
  attempt?: number;
  command?: string;
  args?: string[];
}

export interface DashboardScriptResponse {
  scriptRequestId?: string;
  script_request_id?: string;
  attempt?: number;
  outcome?: string;
  stdout?: string;
  stderr?: string;
  durationMillis?: number;
  duration_millis?: number;
  exitCode?: number;
  exit_code?: number;
  failureType?: string;
  failure_type?: string;
}

export interface DashboardRuntimeWorkstationRequestCounts {
  dispatchedCount?: number;
  respondedCount?: number;
  erroredCount?: number;
  dispatched_count?: number;
  responded_count?: number;
  errored_count?: number;
}

export interface DashboardRuntimeWorkstationRequestRequest {
  startedAt?: string;
  started_at?: string;
  requestTime?: string;
  request_time?: string;
  inputWorkItems?: DashboardWorkItemRef[];
  input_work_items?: DashboardWorkItemRef[];
  inputWorkTypeIds?: string[];
  input_work_type_ids?: string[];
  currentChainingTraceId?: string;
  current_chaining_trace_id?: string;
  previousChainingTraceIds?: string[];
  previous_chaining_trace_ids?: string[];
  traceIds?: string[];
  trace_ids?: string[];
  consumedTokens?: DashboardTraceToken[];
  consumed_tokens?: DashboardTraceToken[];
  prompt?: string;
  workingDirectory?: string;
  working_directory?: string;
  worktree?: string;
  provider?: string;
  model?: string;
  requestMetadata?: Record<string, string>;
  request_metadata?: Record<string, string>;
  scriptRequest?: DashboardScriptRequest;
  script_request?: DashboardScriptRequest;
}

export interface DashboardRuntimeWorkstationRequestResponse {
  outcome?: string;
  feedback?: string;
  failureReason?: string;
  failure_reason?: string;
  failureMessage?: string;
  failure_message?: string;
  responseText?: string;
  response_text?: string;
  errorClass?: string;
  error_class?: string;
  providerSession?: DashboardProviderSession;
  provider_session?: DashboardProviderSession;
  diagnostics?: DashboardWorkDiagnostics | FactorySchemas["FactoryWorldWorkDiagnostics"];
  responseMetadata?: Record<string, string>;
  response_metadata?: Record<string, string>;
  scriptResponse?: DashboardScriptResponse;
  script_response?: DashboardScriptResponse;
  endTime?: string;
  end_time?: string;
  durationMillis?: number;
  duration_millis?: number;
  outputWorkItems?: DashboardWorkItemRef[];
  output_work_items?: DashboardWorkItemRef[];
  outputMutations?: DashboardTraceMutation[];
  output_mutations?: DashboardTraceMutation[];
}

export interface DashboardRuntimeWorkstationRequest {
  dispatchId?: string;
  dispatch_id?: string;
  transitionId?: string;
  transition_id?: string;
  workstationName?: string;
  workstation_name?: string;
  counts: DashboardRuntimeWorkstationRequestCounts;
  request: DashboardRuntimeWorkstationRequestRequest;
  response?: DashboardRuntimeWorkstationRequestResponse;
}

export interface DashboardWorkstationRequest {
  counts?: DashboardRuntimeWorkstationRequestCounts;
  dispatch_id: string;
  dispatched_request_count: number;
  errored_request_count: number;
  failure_message?: string;
  failure_reason?: string;
  inference_attempts: DashboardInferenceAttempt[];
  model?: string;
  outcome?: string;
  prompt?: string;
  provider?: string;
  provider_session?: DashboardProviderSession;
  request_view?: DashboardRuntimeWorkstationRequestRequest;
  request_id?: string;
  request_metadata?: Record<string, string>;
  responded_request_count: number;
  response_view?: DashboardRuntimeWorkstationRequestResponse;
  response?: string;
  response_metadata?: Record<string, string>;
  script_request?: DashboardScriptRequest;
  script_response?: DashboardScriptResponse;
  started_at?: string;
  total_duration_millis?: number;
  trace_ids?: string[];
  transition_id: string;
  work_items: DashboardWorkItemRef[];
  working_directory?: string;
  workstation_name?: string;
  workstation_node_id: string;
  worktree?: string;
}

export interface DashboardWorkstationActivity {
  workstation_node_id: string;
  active_dispatch_ids?: string[];
  active_work_items?: DashboardWorkItemRef[];
  trace_ids?: string[];
}

export interface DashboardThrottlePause {
  lane_id: string;
  provider: string;
  model: string;
  paused_at?: string;
  paused_until: string;
  recover_at: string;
  affected_transition_ids?: string[];
  affected_workstation_names?: string[];
  affected_worker_types?: string[];
  affected_work_type_ids?: string[];
}

export interface DashboardSessionRuntime {
  has_data: boolean;
  dispatched_count: number;
  completed_count: number;
  failed_count: number;
  provider_sessions?: DashboardProviderSessionAttempt[];
  dispatched_by_work_type?: Record<string, number>;
  completed_by_work_type?: Record<string, number>;
  failed_by_work_type?: Record<string, number>;
  completed_work_labels?: string[];
  failed_work_labels?: string[];
  failed_work_details_by_work_id?: Record<string, DashboardFailedWorkDetail>;
}

export interface DashboardRuntime {
  in_flight_dispatch_count: number;
  active_dispatch_ids?: string[];
  active_executions_by_dispatch_id?: Record<string, DashboardActiveExecution>;
  active_workstation_node_ids?: string[];
  inference_attempts_by_dispatch_id?: Record<
    string,
    Record<string, DashboardInferenceAttempt>
  >;
  workstation_requests_by_dispatch_id?: Record<
    string,
    DashboardRuntimeWorkstationRequest
  >;
  workstation_activity_by_node_id?: Record<
    string,
    DashboardWorkstationActivity
  >;
  place_token_counts?: Record<string, number>;
  current_work_items_by_place_id?: Record<string, DashboardWorkItemRef[]>;
  place_occupancy_work_items_by_place_id?: Record<
    string,
    DashboardWorkItemRef[]
  >;
  active_throttle_pauses?: DashboardThrottlePause[];
  session: DashboardSessionRuntime;
}

export interface DashboardSnapshot {
  factory_state: string;
  uptime_seconds: number;
  tick_count: number;
  topology: DashboardTopology;
  runtime: DashboardRuntime;
}

export interface DashboardStreamState {
  status: "connecting" | "live" | "offline";
  message: string;
}

export interface DashboardTraceToken {
  token_id: string;
  place_id: string;
  name?: string;
  work_id: string;
  work_type_id: string;
  trace_id?: string;
  tags?: Record<string, string>;
  created_at: string;
  entered_at: string;
}

export interface DashboardTraceMutation {
  type: string;
  token_id: string;
  from_place?: string;
  to_place?: string;
  reason?: string;
  resulting_token?: DashboardTraceToken;
}

export interface DashboardTraceDispatch {
  current_chaining_trace_id?: string;
  dispatch_id: string;
  transition_id: string;
  workstation_name?: string;
  input_items?: DashboardWorkItemRef[];
  output_items?: DashboardWorkItemRef[];
  previous_chaining_trace_ids?: string[];
  work_ids?: string[];
  work_types?: string[];
  token_names?: string[];
  trace_id?: string;
  trace_ids?: string[];
  outcome: string;
  failure_reason?: string;
  failure_message?: string;
  provider_session?: DashboardProviderSession;
  diagnostics?: DashboardWorkDiagnostics;
  start_time: string;
  end_time: string;
  duration_millis: number;
  consumed_tokens?: DashboardTraceToken[];
  output_mutations?: DashboardTraceMutation[];
}

export interface DashboardTrace {
  trace_id: string;
  work_ids: string[];
  work_items?: DashboardWorkItemRef[];
  request_ids?: string[];
  relations?: DashboardWorkRelation[];
  transition_ids: string[];
  workstation_sequence: string[];
  dispatches: DashboardTraceDispatch[];
}

export interface DashboardWorkRelation {
  type: string;
  source_work_id?: string;
  source_work_name?: string;
  target_work_id: string;
  target_work_name?: string;
  required_state?: string;
  request_id?: string;
  trace_id?: string;
}
