import type {
  DashboardFailedWorkDetail,
  DashboardInferenceAttempt,
  DashboardProviderSessionAttempt,
  DashboardRuntime,
  DashboardSessionBracket,
  DashboardSnapshot,
  DashboardTrace,
  DashboardTraceMutation,
  DashboardTraceToken,
  DashboardWorkDiagnostics,
  DashboardWorkItemRef,
  DashboardWorkMoveOperation,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard";
import type {
  FactoryDefinition,
  FactoryPlace,
  FactoryProviderSession,
  FactoryRelation,
  FactoryTerminalWork,
  FactoryWorkItem,
} from "../../../../api/events";
import type { HostedFactoryReplayProjection } from "./projections/projectFactoryReplay";
import type { WorkPayloadLineageProjection } from "./workPayloadLineage";

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
  resourceEvidenceAvailable?: boolean;
  resources: ResourceUnit[];
  startedAt: string;
  startedTick?: number;
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
  feedbackTextBlobID?: string;
  selectedClassificationLabel?: string;
  inputItems: DashboardWorkItemRef[];
  outcome: string;
  outputItems: DashboardWorkItemRef[];
  outputMutations: DashboardTraceMutation[];
  providerSession?: FactoryProviderSession;
  responseText?: string;
  responseTextBlobID?: string;
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
  stderrTextBlobID?: string;
  stdout: string;
  stdoutTextBlobID?: string;
  transition_id: string;
}

export interface ReplayTextBacked {
  promptTextBlobID?: string;
  responseTextBlobID?: string;
}

export interface ReplayTextBlobState {
  textBlobsByID: Record<string, string>;
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
    continue_place_ids?: string[];
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
    Record<string, DashboardInferenceAttempt & ReplayTextBacked>
  >;
  occupancyByID: Record<string, PlaceOccupancy>;
  providerSessions: DashboardProviderSessionAttempt[];
  relationsByWorkID: Record<string, FactoryRelation[]>;
  scriptRequestsByDispatchID: Record<
    string,
    Record<string, WorldScriptRequest>
  >;
  scriptResponsesByDispatchID: Record<
    string,
    Record<string, WorldScriptResponse>
  >;
  terminalWorkByID: Record<string, FactoryTerminalWork>;
  tracesByID: Record<string, DashboardTrace>;
  tracesByWorkID: Record<string, DashboardTrace>;
  workStateChangesByWorkID?: Record<string, DashboardWorkMoveOperation[]>;
  workItemsByID: Record<string, FactoryWorkItem>;
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>;
  workRequestsByID: Record<string, TimelineWorkRequestPayload>;
}

export interface ReplayJavaScriptCheckpointRef {
  id: string;
  label?: string;
  summary?: string;
}

export interface ReplayJavaScriptDispatch {
  artifact_ids?: string[];
  dispatch_kind?: string;
  id: string;
  label?: string;
  phase?: string;
  status: string;
}

export interface ReplaySessionArtifact {
  content_type?: string;
  id: string;
  kind?: string;
  label?: string;
  visibility?: string;
}

export interface ReplayJavaScriptRuntime {
  checkpoints: ReplayJavaScriptCheckpointRef[];
  child_dispatch_counts: {
    completed: number;
    queued: number;
    running: number;
  };
  dispatches: ReplayJavaScriptDispatch[];
  phase?: string;
  phases: string[];
  script_status?: string;
}

export interface ReplayWorldState extends TimelineWorldViewBase {
  factory?: FactoryDefinition;
  factory_state: string;
  javascriptRuntime?: ReplayJavaScriptRuntime;
  payloadLineage: WorkPayloadLineageProjection;
  runtime: DashboardRuntime;
  sessionArtifacts: ReplaySessionArtifact[];
  sessionBracket?: DashboardSessionBracket;
  textBlobsByID: Record<string, string>;
  tick_count: number;
  topology: ProjectedInitialStructure;
  uptime_seconds: number;
}

export interface WorldState extends DashboardSnapshot {
  factoryReplay: HostedFactoryReplayProjection;
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
