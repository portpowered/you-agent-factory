import type {
  DashboardSnapshot,
  DashboardRuntime,
  DashboardFailedWorkDetail,
  DashboardInferenceAttempt,
  DashboardProviderSessionAttempt,
  DashboardTrace,
  DashboardTraceMutation,
  DashboardTraceToken,
  DashboardWorkDiagnostics,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard";
import type {
  FactoryPlace,
  FactoryRelation,
  FactoryWorkItem,
  FactoryProviderSession,
  FactoryTerminalWork,
} from "../../../../api/events";

export interface ResourceUnit {
  placeID: string;
  resourceID: string;
  tokenID: string;
}

export interface WorldDispatch {
  consumedTokens: DashboardTraceToken[];
  currentChainingTraceID?: string;
  dispatchID: string;
  model?: string;
  modelProvider?: string;
  previousChainingTraceIDs?: string[];
  provider?: string;
  resources: ResourceUnit[];
  startedAt: string;
  systemOnly: boolean;
  traceIDs: string[];
  transitionID: string;
  workItems: DashboardWorkItemRef[];
  workstationName?: string;
}

export interface WorldCompletion extends WorldDispatch {
  diagnostics?: DashboardWorkDiagnostics;
  durationMillis: number;
  endTime: string;
  failureMessage?: string;
  failureReason?: string;
  feedback?: string;
  inputItems: DashboardWorkItemRef[];
  outcome: string;
  outputItems: DashboardWorkItemRef[];
  outputMutations: DashboardTraceMutation[];
  providerSession?: FactoryProviderSession;
  responseText?: string;
  terminalWork?: FactoryTerminalWork;
}

export interface TimelineWorkRequestPayload {
  parentLineage?: string[];
  request_id: string;
  source?: string;
  trace_id?: string;
  type: string;
  work_items?: Array<{
    id: string;
    name?: string;
    tags?: Record<string, string>;
    trace_id?: string;
    work_type_id: string;
  }>;
}

export interface WorldScriptRequest {
  args: string[];
  attempt: number;
  command: string;
  dispatch_id: string;
  request_time: string;
  script_request_id: string;
  transition_id: string;
}

export interface WorldScriptResponse {
  attempt: number;
  dispatch_id: string;
  duration_millis: number;
  exit_code?: number;
  failure_type?: string;
  outcome: string;
  response_time: string;
  script_request_id: string;
  stderr: string;
  stdout: string;
  transition_id: string;
}

export interface PlaceOccupancy {
  placeID: string;
  resourceTokenIDs: string[];
  tokenCount: number;
  workItemIDs: string[];
}

export interface ProjectedInitialStructure {
  resources?: { capacity: number; id: string; name?: string }[];
  workers?: {
    id: string;
    name?: string;
    provider?: string;
    model_provider?: string;
    model?: string;
  }[];
  work_types?: {
    id: string;
    name?: string;
    states?: { category: string; value: string }[];
  }[];
  workstations?: {
    failure_place_ids?: string[];
    id: string;
    input_place_ids?: string[];
    kind?: string;
    name: string;
    output_place_ids?: string[];
    rejection_place_ids?: string[];
    worker_id?: string;
  }[];
  places?: FactoryPlace[];
}

export interface TimelineWorldViewBase {
  activeDispatches: Record<string, WorldDispatch>;
  completedDispatches: WorldCompletion[];
  failedWorkDetailsByWorkID: Record<string, DashboardFailedWorkDetail>;
  failedWorkItemsByID: Record<string, FactoryWorkItem>;
  inferenceAttemptsByDispatchID: Record<
    string,
    Record<string, DashboardInferenceAttempt>
  >;
  occupancyByID: Record<string, PlaceOccupancy>;
  providerSessions: DashboardProviderSessionAttempt[];
  relationsByWorkID: Record<string, FactoryRelation[]>;
  scriptRequestsByDispatchID: Record<string, Record<string, WorldScriptRequest>>;
  scriptResponsesByDispatchID: Record<
    string,
    Record<string, WorldScriptResponse>
  >;
  terminalWorkByID: Record<string, FactoryTerminalWork>;
  tracesByID: Record<string, DashboardTrace>;
  tracesByWorkID: Record<string, DashboardTrace>;
  workItemsByID: Record<string, FactoryWorkItem>;
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>;
  workRequestsByID: Record<string, TimelineWorkRequestPayload>;
}

export interface ReplayWorldState extends TimelineWorldViewBase {
  factory_state: string;
  runtime: DashboardRuntime;
  tick_count: number;
  topology: ProjectedInitialStructure;
  uptime_seconds: number;
}

export interface WorldState extends DashboardSnapshot {
  relationsByWorkID: Record<string, FactoryRelation[]>;
  tracesByWorkID: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>;
  workRequestsByID: Record<string, TimelineWorkRequestPayload>;
}

export function emptyWorldRuntime(): DashboardRuntime {
  return {
    in_flight_dispatch_count: 0,
    session: {
      completed_count: 0,
      dispatched_count: 0,
      failed_count: 0,
      has_data: false,
    },
  };
}


