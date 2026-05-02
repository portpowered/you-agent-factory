import { create } from "zustand";

import type {
  DashboardActiveExecution,
  DashboardFailedWorkDetail,
  DashboardInferenceAttempt,
  DashboardPlaceKind,
  DashboardProviderDiagnostic,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardScriptRequest,
  DashboardScriptResponse,
  DashboardSnapshot,
  DashboardTrace,
  DashboardTraceDispatch,
  DashboardTraceMutation,
  DashboardTraceToken,
  DashboardWorkDiagnostics,
  DashboardWorkRelation,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
  DashboardWorkstationEdge,
  StateCategory,
} from "../api/dashboard";
import type {
  FactoryDefinition,
  DispatchResponsePayload,
  DispatchRequestPayload,
  FactoryEvent,
  FactoryPlace,
  FactoryProviderSession,
  FactoryRelation,
  FactoryResource,
  FactoryStateResponsePayload,
  FactoryTerminalWork,
  FactoryWork,
  FactoryWorkDiagnostics,
  FactoryWorkItem,
  FactoryWorker,
  FactoryWorkstation,
  FactoryWorkType,
  WorkstationIO,
  InitialStructureRequestPayload,
  InferenceRequestPayload,
  InferenceResponsePayload,
  RelationshipChangeRequestPayload,
  RunRequestPayload,
  ScriptRequestPayload,
  ScriptResponsePayload,
  WorkRequestPayload,
} from "../api/events";
import { FACTORY_EVENT_TYPES } from "../api/events";

export type FactoryTimelineMode = "current" | "fixed";

interface PlaceOccupancy {
  placeID: string;
  resourceTokenIDs: string[];
  tokenCount: number;
  workItemIDs: string[];
}

interface ResourceUnit {
  placeID: string;
  resourceID: string;
  tokenID: string;
}

interface ProjectedInitialStructure {
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

interface WorldDispatch {
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

interface WorldCompletion extends WorldDispatch {
  diagnostics?: DashboardWorkDiagnostics;
  durationMillis: number;
  endTime: string;
  feedback?: string;
  responseText?: string;
  failureMessage?: string;
  failureReason?: string;
  inputItems: DashboardWorkItemRef[];
  outcome: string;
  outputItems: DashboardWorkItemRef[];
  outputMutations: DashboardTraceMutation[];
  providerSession?: FactoryProviderSession;
  terminalWork?: FactoryTerminalWork;
}

interface TimelineWorkRequestPayload {
  parentLineage?: string[];
  request_id: string;
  source?: string;
  trace_id?: string;
  type: WorkRequestPayload["type"];
  work_items?: FactoryWorkItem[];
}

interface WorldScriptRequest {
  args: string[];
  attempt: number;
  command: string;
  dispatch_id: string;
  request_time: string;
  script_request_id: string;
  transition_id: string;
}

interface WorldScriptResponse {
  attempt: number;
  dispatch_id: string;
  duration_millis: number;
  exit_code?: number;
  failure_type?: ScriptResponsePayload["failureType"];
  outcome: ScriptResponsePayload["outcome"];
  response_time: string;
  script_request_id: string;
  stderr: string;
  stdout: string;
  transition_id: string;
}

interface WorldState {
  activeDispatches: Record<string, WorldDispatch>;
  completedDispatches: WorldCompletion[];
  factoryState: string;
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
  tick: number;
  topology: ProjectedInitialStructure;
  tracesByID: Record<string, DashboardTrace>;
  workItemsByID: Record<string, FactoryWorkItem>;
  workRequestsByID: Record<string, TimelineWorkRequestPayload>;
}

interface LegacyDispatchRequestPayloadCompat {
  current_chaining_trace_id?: string;
  dispatchId?: string;
  inputs?: Array<FactoryWork | { workId: string }>;
  previous_chaining_trace_ids?: string[];
  worker?: FactoryWorker;
  workstation?: FactoryWorkstation;
}

interface LegacyDispatchResponsePayloadCompat {
  current_chaining_trace_id?: string;
  diagnostics?: FactoryWorkDiagnostics;
  dispatchId?: string;
  previous_chaining_trace_ids?: string[];
  providerSession?: FactoryProviderSession;
  workstation?: FactoryWorkstation;
}

interface LegacyFactoryWorkCompat {
  current_chaining_trace_id?: string;
  previous_chaining_trace_ids?: string[];
  trace_id?: string;
  work_id?: string;
  work_type_id?: string;
}

export interface FactoryTimelineSnapshot {
  dashboard: DashboardSnapshot;
  relationsByWorkID: Record<string, FactoryRelation[]>;
  tracesByWorkID: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>;
  workRequestsByID: Record<string, TimelineWorkRequestPayload>;
}

interface FactoryTimelineState {
  events: FactoryEvent[];
  latestTick: number;
  mode: FactoryTimelineMode;
  receivedEventIDs: string[];
  selectedTick: number;
  worldViewCache: Record<number, FactoryTimelineSnapshot>;
  appendEvent: (event: FactoryEvent) => void;
  appendEvents: (events: FactoryEvent[]) => void;
  replaceEvents: (events: FactoryEvent[]) => void;
  reset: () => void;
  selectTick: (tick: number) => void;
  setCurrentMode: () => void;
}

type InitialStructureRequestEvent = FactoryEvent<InitialStructureRequestPayload>;
type RunRequestEvent = FactoryEvent<RunRequestPayload>;
type WorkRequestEvent = FactoryEvent<WorkRequestPayload>;
type RelationshipChangeRequestEvent = FactoryEvent<RelationshipChangeRequestPayload>;
type DispatchRequestEvent = FactoryEvent<DispatchRequestPayload>;
type InferenceRequestEvent = FactoryEvent<InferenceRequestPayload>;
type InferenceResponseEvent = FactoryEvent<InferenceResponsePayload>;
type ScriptRequestEvent = FactoryEvent<ScriptRequestPayload>;
type ScriptResponseEvent = FactoryEvent<ScriptResponsePayload>;
type DispatchResponseEvent = FactoryEvent<DispatchResponsePayload>;
type FactoryStateResponseEvent = FactoryEvent<FactoryStateResponsePayload>;

const EMPTY_DASHBOARD: DashboardSnapshot = {
  factory_state: "UNKNOWN",
  runtime: {
    in_flight_dispatch_count: 0,
    session: {
      completed_count: 0,
      dispatched_count: 0,
      failed_count: 0,
      has_data: false,
    },
  },
  tick_count: 0,
  topology: {
    edges: [],
    submit_work_types: [],
    workstation_node_ids: [],
    workstation_nodes_by_id: {},
  },
  uptime_seconds: 0,
};

const EMPTY_TIMELINE_STATE = {
  events: [],
  latestTick: 0,
  mode: "current" as const,
  receivedEventIDs: [],
  selectedTick: 0,
  worldViewCache: {
    0: {
      dashboard: EMPTY_DASHBOARD,
      relationsByWorkID: {},
      tracesByWorkID: {},
      workstationRequestsByDispatchID: {},
      workRequestsByID: {},
    },
  },
};

const SYSTEM_TIME_WORK_TYPE_ID = "__system_time";
const SYSTEM_TIME_PENDING_STATE = "pending";
const SYSTEM_TIME_PENDING_PLACE_ID = `${SYSTEM_TIME_WORK_TYPE_ID}:${SYSTEM_TIME_PENDING_STATE}`;
const SYSTEM_TIME_EXPIRY_TRANSITION_ID = `${SYSTEM_TIME_WORK_TYPE_ID}:expire`;
const DASHBOARD_TIME_WORK_TYPE_ID = "time";
const DASHBOARD_TIME_PENDING_PLACE_ID = `${DASHBOARD_TIME_WORK_TYPE_ID}:${SYSTEM_TIME_PENDING_STATE}`;
const DASHBOARD_TIME_EXPIRY_TRANSITION_ID = `${DASHBOARD_TIME_WORK_TYPE_ID}:expire`;

function emptyWorldState(tick: number): WorldState {
  return {
    activeDispatches: {},
    completedDispatches: [],
    factoryState: "UNKNOWN",
    failedWorkDetailsByWorkID: {},
    failedWorkItemsByID: {},
    inferenceAttemptsByDispatchID: {},
    occupancyByID: {},
    providerSessions: [],
    relationsByWorkID: {},
    scriptRequestsByDispatchID: {},
    scriptResponsesByDispatchID: {},
    terminalWorkByID: {},
    tick,
    topology: {},
    tracesByID: {},
    workItemsByID: {},
    workRequestsByID: {},
  };
}

function uniqueSorted(values: Array<string | null | undefined>): string[] {
  return [
    ...new Set(
      values.filter(
        (value): value is string => typeof value === "string" && value.length > 0,
      ),
    ),
  ].sort();
}

function factoryWorkTypes(factory: FactoryDefinition): FactoryWorkType[] {
  return factory.workTypes ?? [];
}

function factoryWorkers(factory: FactoryDefinition): FactoryWorker[] {
  return factory.workers ?? [];
}

function factoryWorkstations(factory: FactoryDefinition): FactoryWorkstation[] {
  return factory.workstations ?? [];
}

function ioWorkType(io: { workType?: string }): string {
  return io.workType ?? "";
}

function workstationFailureIO(
  workstation: FactoryWorkstation,
): WorkstationIO | undefined {
  return workstation.onFailure;
}

function workstationRejectionIO(
  workstation: FactoryWorkstation,
): WorkstationIO | undefined {
  return workstation.onRejection;
}

function workstationSchedulingKind(workstation: FactoryWorkstation): string | undefined {
  return workstation.behavior ?? workstation.type;
}

function workerModelProvider(worker: FactoryWorker | undefined): string | undefined {
  return worker?.modelProvider;
}

interface RelationKeyFields {
  request_id?: string;
  required_state?: string;
  source_work_id?: string;
  target_work_id?: string;
  type: string;
}

function relationKey(relation: RelationKeyFields): string {
  return [
    relation.type,
    relation.source_work_id ?? "",
    relation.target_work_id,
    relation.required_state ?? "",
    relation.request_id ?? "",
  ].join("|");
}

function dedupeRelations(
  relations: DashboardWorkRelation[],
): DashboardWorkRelation[] {
  const seen = new Set<string>();
  return relations
    .filter((relation) => {
      const key = relationKey(relation);
      if (seen.has(key)) {
        return false;
      }
      seen.add(key);
      return true;
    })
    .sort((left, right) => relationKey(left).localeCompare(relationKey(right)));
}

function orderedEvents(events: FactoryEvent[]): FactoryEvent[] {
  return [...events].sort((left, right) => {
    if (left.context.tick !== right.context.tick) {
      return left.context.tick - right.context.tick;
    }
    if (left.context.sequence !== right.context.sequence) {
      return left.context.sequence - right.context.sequence;
    }
    if (left.context.eventTime !== right.context.eventTime) {
      return left.context.eventTime.localeCompare(right.context.eventTime);
    }
    return left.id.localeCompare(right.id);
  });
}

function workRef(item: FactoryWorkItem): DashboardWorkItemRef {
  return {
    ...(item.current_chaining_trace_id
      ? { current_chaining_trace_id: item.current_chaining_trace_id }
      : {}),
    display_name: item.display_name,
    ...(item.previous_chaining_trace_ids
      ? {
          previous_chaining_trace_ids: [
            ...item.previous_chaining_trace_ids,
          ],
        }
      : {}),
    trace_id: item.trace_id,
    work_id: item.id,
    work_type_id: dashboardWorkTypeID(item.work_type_id),
  };
}

function toDashboardRelation(relation: FactoryRelation): DashboardWorkRelation {
  const legacyRelation = relation as FactoryRelation & {
    required_state?: string;
    source_work_name?: string;
    target_work_id?: string;
    target_work_name?: string;
  };
  return {
    request_id: relation.request_id,
    required_state: relation.requiredState ?? legacyRelation.required_state,
    source_work_id: relation.source_work_id,
    source_work_name: relation.sourceWorkName ?? legacyRelation.source_work_name,
    target_work_id: relation.targetWorkId ?? legacyRelation.target_work_id ?? "",
    target_work_name: relation.targetWorkName ?? legacyRelation.target_work_name,
    trace_id: relation.trace_id,
    type: relation.type,
  };
}

function emptyTrace(traceID: string): DashboardTrace {
  return {
    dispatches: [],
    relations: [],
    request_ids: [],
    trace_id: traceID,
    transition_ids: [],
    work_ids: [],
    work_items: [],
    workstation_sequence: [],
  };
}

function addTraceWork(state: WorldState, item: FactoryWorkItem): void {
  if (!item.trace_id) {
    return;
  }
  const trace = state.tracesByID[item.trace_id] ?? emptyTrace(item.trace_id);
  trace.work_ids = uniqueSorted([...trace.work_ids, item.id]);
  trace.work_items = dedupeWorkRefs([...(trace.work_items ?? []), workRef(item)]);
  state.tracesByID[item.trace_id] = trace;
}

function addTraceRequest(
  state: WorldState,
  traceID: string | undefined,
  requestID: string,
): void {
  if (!traceID || !requestID) {
    return;
  }
  const trace = state.tracesByID[traceID] ?? emptyTrace(traceID);
  trace.request_ids = uniqueSorted([...(trace.request_ids ?? []), requestID]);
  state.tracesByID[traceID] = trace;
}

function addTraceRelation(
  state: WorldState,
  traceID: string | undefined,
  relation: FactoryRelation,
): void {
  if (!traceID) {
    return;
  }
  const trace = state.tracesByID[traceID] ?? emptyTrace(traceID);
  trace.relations = dedupeRelations([
    ...(trace.relations ?? []),
    toDashboardRelation(relation),
  ]);
  state.tracesByID[traceID] = trace;
}

function addRelation(state: WorldState, relation: FactoryRelation): void {
  const targetWorkID =
    relation.targetWorkId ??
    (relation as FactoryRelation & { target_work_id?: string }).target_work_id;
  if (!relation.source_work_id || !targetWorkID) {
    return;
  }
  const relations = state.relationsByWorkID[relation.source_work_id] ?? [];
  if (
    !relations.some((current) => relationKey(current) === relationKey(relation))
  ) {
    state.relationsByWorkID[relation.source_work_id] = [
      ...relations,
      relation,
    ].sort((left, right) =>
      relationKey(left).localeCompare(relationKey(right)),
    );
  }
  addTraceRelation(state, relation.trace_id, relation);
  addTraceRelation(
    state,
    state.workItemsByID[relation.source_work_id]?.trace_id,
    relation,
  );
  addTraceRelation(
    state,
    state.workItemsByID[targetWorkID]?.trace_id,
    relation,
  );
}

function addTraceDispatch(
  state: WorldState,
  traceID: string,
  completion: WorldCompletion,
): void {
  if (!traceID) {
    return;
  }
  const trace = state.tracesByID[traceID] ?? emptyTrace(traceID);
  trace.dispatches = [
    ...trace.dispatches,
    completionToTraceDispatch(completion),
  ];
  trace.transition_ids = uniqueSorted([
    ...trace.transition_ids,
    completion.transitionID,
  ]);
  trace.workstation_sequence = [
    ...trace.workstation_sequence,
    completion.workstationName ?? completion.transitionID,
  ];
  state.tracesByID[traceID] = trace;
}

function addToken(
  state: WorldState,
  placeID: string | undefined,
  tokenID: string,
  workItemID?: string,
): void {
  if (!placeID || !tokenID) {
    return;
  }
  const occupancy = state.occupancyByID[placeID] ?? {
    placeID,
    resourceTokenIDs: [],
    tokenCount: 0,
    workItemIDs: [],
  };
  if (workItemID) {
    occupancy.workItemIDs = uniqueSorted([
      ...occupancy.workItemIDs,
      workItemID,
    ]);
  } else {
    occupancy.resourceTokenIDs = uniqueSorted([
      ...occupancy.resourceTokenIDs,
      tokenID,
    ]);
  }
  occupancy.tokenCount =
    occupancy.resourceTokenIDs.length + occupancy.workItemIDs.length;
  state.occupancyByID[placeID] = occupancy;
}

function removeWorkToken(state: WorldState, workID: string): void {
  const placeID = state.workItemsByID[workID]?.place_id;
  if (!placeID) {
    return;
  }
  const occupancy = state.occupancyByID[placeID];
  if (!occupancy) {
    return;
  }
  occupancy.workItemIDs = occupancy.workItemIDs.filter((id) => id !== workID);
  occupancy.tokenCount =
    occupancy.resourceTokenIDs.length + occupancy.workItemIDs.length;
  if (occupancy.tokenCount === 0) {
    delete state.occupancyByID[placeID];
  }
}

function removeResourceToken(
  state: WorldState,
  placeID: string,
  tokenID: string,
): void {
  const occupancy = state.occupancyByID[placeID];
  if (!occupancy) {
    return;
  }
  occupancy.resourceTokenIDs = occupancy.resourceTokenIDs.filter(
    (id) => id !== tokenID,
  );
  occupancy.tokenCount =
    occupancy.resourceTokenIDs.length + occupancy.workItemIDs.length;
  if (occupancy.tokenCount === 0) {
    delete state.occupancyByID[placeID];
  }
}

function firstAvailableResourceTokenID(
  state: WorldState,
  resourceID: string,
): string | undefined {
  return state.occupancyByID[resourceAvailablePlaceID(resourceID)]
    ?.resourceTokenIDs[0];
}

function resourceAvailablePlaceID(resourceID: string): string {
  return placeID(resourceID, "available");
}

function resourceTokenID(resourceID: string, index: number): string {
  return `${resourceID}:resource:${index}`;
}

function resourceIDsFromEvent(
  resources: FactoryResource[] | undefined,
): string[] {
  return (resources ?? [])
    .map((resource) => resource.name)
    .filter((name) => name.length > 0);
}

function consumeResourceUnits(
  state: WorldState,
  resources: FactoryResource[] | undefined,
): ResourceUnit[] {
  return resourceIDsFromEvent(resources).map((resourceID) => {
    const placeID = resourceAvailablePlaceID(resourceID);
    const tokenID = firstAvailableResourceTokenID(state, resourceID) ?? "";
    if (tokenID) {
      removeResourceToken(state, placeID, tokenID);
    }
    return { placeID, resourceID, tokenID };
  });
}

function releaseResourceUnits(
  state: WorldState,
  consumed: ResourceUnit[],
  resources: FactoryResource[] | undefined,
): void {
  const released = new Set<number>();
  for (const resourceID of resourceIDsFromEvent(resources)) {
    const index = consumed.findIndex(
      (unit, candidateIndex) =>
        !released.has(candidateIndex) && unit.resourceID === resourceID,
    );
    if (index < 0) {
      continue;
    }
    released.add(index);
    const unit = consumed[index];
    if (!unit?.tokenID) {
      continue;
    }
    addToken(
      state,
      unit.placeID || resourceAvailablePlaceID(unit.resourceID),
      unit.tokenID,
    );
  }
}

function traceToken(
  item: FactoryWorkItem,
  eventTime: string,
): DashboardTraceToken {
  return {
    created_at: eventTime,
    entered_at: eventTime,
    name: item.display_name,
    place_id: dashboardPlaceID(item.place_id ?? ""),
    tags: item.tags,
    token_id: item.id,
    trace_id: item.trace_id,
    work_id: item.id,
    work_type_id: dashboardWorkTypeID(item.work_type_id),
  };
}

function applyEvent(state: WorldState, event: FactoryEvent): void {
  switch (event.type) {
    case FACTORY_EVENT_TYPES.runRequest:
      state.topology = normalizeFactoryPayload(
        (event as RunRequestEvent).payload,
      );
      seedResourceOccupancy(state);
      return;
    case FACTORY_EVENT_TYPES.runResponse:
      return;
    case FACTORY_EVENT_TYPES.initialStructureRequest:
      state.topology = normalizeFactoryPayload(
        (event as InitialStructureRequestEvent).payload,
      );
      seedResourceOccupancy(state);
      return;
    case FACTORY_EVENT_TYPES.workRequest:
      applyWorkRequest(state, event as WorkRequestEvent);
      return;
    case FACTORY_EVENT_TYPES.relationshipChangeRequest:
      applyRelationshipChange(state, event as RelationshipChangeRequestEvent);
      return;
    case FACTORY_EVENT_TYPES.dispatchRequest:
      applyRequest(state, event as DispatchRequestEvent);
      return;
    case FACTORY_EVENT_TYPES.inferenceRequest:
      applyInferenceRequest(state, event as InferenceRequestEvent);
      return;
    case FACTORY_EVENT_TYPES.inferenceResponse:
      applyInferenceResponse(state, event as InferenceResponseEvent);
      return;
    case FACTORY_EVENT_TYPES.scriptRequest:
      applyScriptRequest(state, event as ScriptRequestEvent);
      return;
    case FACTORY_EVENT_TYPES.scriptResponse:
      applyScriptResponse(state, event as ScriptResponseEvent);
      return;
    case FACTORY_EVENT_TYPES.dispatchResponse:
      applyResponse(state, event as DispatchResponseEvent);
      return;
    case FACTORY_EVENT_TYPES.factoryStateResponse:
      state.factoryState = (event as FactoryStateResponseEvent).payload.state;
      return;
  }
}

function normalizeFactoryPayload(
  payload: Pick<InitialStructureRequestPayload, "factory">,
): ProjectedInitialStructure {
  const factory = payload.factory;
  const workTypes = factoryWorkTypes(factory)
    .filter((workType) => !isSystemTimeWorkType(workType.name))
    .map((workType) => ({
      id: workType.name,
      name: workType.name,
      states: (workType.states ?? []).map((state) => ({
        category: state.type,
        value: state.name,
      })),
    }));
  const workTypeIDs = new Set(workTypes.map((workType) => workType.id));
  const places = new Map<string, FactoryPlace>();
  for (const workType of workTypes) {
    for (const state of workType.states ?? []) {
      const id = placeID(workType.id, state.value);
      places.set(id, {
        category: state.category,
        id,
        state: state.value,
        type_id: workType.id,
      });
    }
  }
  for (const workstation of factoryWorkstations(factory)) {
    const failure = workstationFailureIO(workstation);
    const rejection = workstationRejectionIO(workstation);
    for (const io of [
      ...workstation.inputs,
      ...workstation.outputs,
      ...(failure ? [failure] : []),
      ...(rejection ? [rejection] : []),
    ]) {
      const workTypeID = ioWorkType(io);
      if (workTypeIDs.has(workTypeID) || isSystemTimeWorkType(workTypeID)) {
        continue;
      }
      const id = placeIDFromIO(io);
      places.set(id, {
        id,
        state: io.state,
        type_id: workTypeID,
      });
    }
  }
  for (const resource of factory.resources ?? []) {
    const id = resourceAvailablePlaceID(resource.name);
    places.set(id, {
      category: "PROCESSING",
      id,
      state: "available",
      type_id: resource.name,
    });
  }
  return {
    places: [...places.values()].sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    resources: (factory.resources ?? []).map((resource) => ({
      capacity: resource.capacity ?? 0,
      id: resource.name,
      name: resource.name,
    })),
    workers: factoryWorkers(factory).map((worker) => ({
      id: worker.name,
      model: worker.model,
      model_provider: workerModelProvider(worker),
      name: worker.name,
      provider: worker.executorProvider,
    })),
    work_types: workTypes,
    workstations: factoryWorkstations(factory)
      .filter((workstation) => !isSystemTimeWorkstation(workstation))
      .map(projectWorkstationTopology),
  };
}

function seedResourceOccupancy(state: WorldState): void {
  for (const resource of state.topology.resources ?? []) {
    const place = (state.topology.places ?? [])
      .filter((candidate) => candidate.type_id === resource.id)
      .sort((left, right) => {
        if (left.state === "available" && right.state !== "available") {
          return -1;
        }
        if (right.state === "available" && left.state !== "available") {
          return 1;
        }
        return left.id.localeCompare(right.id);
      })[0];
    if (!place) {
      continue;
    }
    for (let index = 0; index < resource.capacity; index += 1) {
      addToken(state, place.id, resourceTokenID(resource.id, index));
    }
  }
}

function projectWorkstationTopology(
  workstation: FactoryWorkstation,
): NonNullable<ProjectedInitialStructure["workstations"]>[number] {
  const inputs = workstation.inputs.filter(isPublicWorkstationIO);
  const outputs = workstation.outputs.filter(isPublicWorkstationIO);
  const failure = workstationFailureIO(workstation);
  const rejection = workstationRejectionIO(workstation);

  return {
    failure_place_ids:
      failure && isPublicWorkstationIO(failure)
        ? [placeIDFromIO(failure)]
        : undefined,
    id: workstation.id ?? workstation.name,
    input_place_ids: inputs.map(placeIDFromIO),
    kind: workstationSchedulingKind(workstation),
    name: workstation.name,
    output_place_ids: outputs.map(placeIDFromIO),
    rejection_place_ids:
      rejection && isPublicWorkstationIO(rejection)
        ? [placeIDFromIO(rejection)]
        : undefined,
    worker_id: workstation.worker,
  };
}

function placeIDFromIO(io: { state: string; workType?: string }): string {
  return placeID(ioWorkType(io), io.state);
}

function placeID(workTypeID: string, state: string): string {
  return `${workTypeID}:${state}`;
}

function isPublicWorkstationIO(io: { workType?: string }): boolean {
  return !isSystemTimeWorkType(ioWorkType(io));
}

function isSystemTimeWorkType(workTypeID: string | undefined): boolean {
  return workTypeID === SYSTEM_TIME_WORK_TYPE_ID;
}

function isSystemTimePlace(placeIDValue: string | undefined): boolean {
  return placeIDValue === SYSTEM_TIME_PENDING_PLACE_ID;
}

function isSystemTimeWorkstation(workstation: FactoryWorkstation): boolean {
  return (
    (workstation.id ?? workstation.name) === SYSTEM_TIME_EXPIRY_TRANSITION_ID
  );
}

function isSystemTimeWorkItem(item: FactoryWorkItem): boolean {
  return isSystemTimeWorkType(item.work_type_id);
}

function dashboardTransitionID(transitionID: string): string {
  return transitionID === SYSTEM_TIME_EXPIRY_TRANSITION_ID
    ? DASHBOARD_TIME_EXPIRY_TRANSITION_ID
    : transitionID;
}

function dashboardWorkstationName(
  transitionID: string,
  name: string | undefined,
): string | undefined {
  if (
    transitionID === SYSTEM_TIME_EXPIRY_TRANSITION_ID &&
    (name === undefined ||
      name === "" ||
      name === SYSTEM_TIME_EXPIRY_TRANSITION_ID)
  ) {
    return DASHBOARD_TIME_EXPIRY_TRANSITION_ID;
  }
  return name;
}

function topologyWorkstation(
  topology: ProjectedInitialStructure,
  transitionID: string,
): FactoryWorkstationShape | undefined {
  return (topology.workstations ?? []).find(
    (workstation) =>
      workstation.id === transitionID || workstation.name === transitionID,
  );
}

function resolveWorkstationName(
  topology: ProjectedInitialStructure,
  transitionID: string,
  name: string | undefined,
): string | undefined {
  return dashboardWorkstationName(
    transitionID,
    name ?? topologyWorkstation(topology, transitionID)?.name,
  );
}

function topologyWorker(
  topology: ProjectedInitialStructure,
  transitionID: string,
): NonNullable<ProjectedInitialStructure["workers"]>[number] | undefined {
  const workstation = topologyWorkstation(topology, transitionID);
  if (!workstation?.worker_id) {
    return undefined;
  }
  return (topology.workers ?? []).find((worker) => worker.id === workstation.worker_id);
}

function dashboardPlaceID(placeIDValue: string): string {
  return isSystemTimePlace(placeIDValue)
    ? DASHBOARD_TIME_PENDING_PLACE_ID
    : placeIDValue;
}

function dashboardWorkTypeID(workTypeID: string): string {
  return isSystemTimeWorkType(workTypeID)
    ? DASHBOARD_TIME_WORK_TYPE_ID
    : workTypeID;
}

function factoryWorkToItem(
  state: WorldState,
  work: FactoryWork | { workId: string },
  placeIDOverride?: string,
): FactoryWorkItem {
  const legacyWork = work as (FactoryWork | { workId: string }) & LegacyFactoryWorkCompat;
  const workID =
    ("workId" in work && typeof work.workId === "string" ? work.workId : undefined) ??
    legacyWork.work_id ??
    ("name" in work && typeof work.name === "string" ? work.name : undefined) ??
    "";
  const existing = state.workItemsByID[workID];
  const workTypeID =
    ("workTypeName" in work && typeof work.workTypeName === "string"
      ? work.workTypeName
      : undefined) ??
    ("work_type_name" in work && typeof work.work_type_name === "string"
      ? work.work_type_name
      : undefined) ??
    legacyWork.work_type_id ??
    existing?.work_type_id;
  const placeIDValue =
    placeIDOverride ??
    existing?.place_id ??
    (isSystemTimeWorkType(workTypeID)
      ? SYSTEM_TIME_PENDING_PLACE_ID
      : undefined);
  return {
    current_chaining_trace_id:
      legacyWork.current_chaining_trace_id ?? existing?.current_chaining_trace_id,
    display_name:
      ("name" in work && typeof work.name === "string" ? work.name : undefined) ||
      existing?.display_name,
    id: workID,
    place_id: placeIDValue,
    previous_chaining_trace_ids:
      legacyWork.previous_chaining_trace_ids ?? existing?.previous_chaining_trace_ids,
    tags: ("tags" in work ? work.tags : undefined) ?? existing?.tags,
    trace_id:
      ("traceId" in work && typeof work.traceId === "string" ? work.traceId : undefined) ??
      ("trace_id" in work && typeof work.trace_id === "string" ? work.trace_id : undefined) ??
      existing?.trace_id,
    work_type_id: workTypeID ?? existing?.work_type_id ?? "",
  };
}

function eventWorkTypeID(work: FactoryWork): string | undefined {
  return (
    work.workTypeName ??
    (work as FactoryWork & { work_type_name?: string }).work_type_name ??
    (work as FactoryWork & LegacyFactoryWorkCompat).work_type_id
  );
}

function legacyDispatchRequestPayload(
  payload: DispatchRequestPayload,
): LegacyDispatchRequestPayloadCompat {
  return payload as DispatchRequestPayload & LegacyDispatchRequestPayloadCompat;
}

function legacyDispatchResponsePayload(
  payload: DispatchResponsePayload,
): LegacyDispatchResponsePayloadCompat {
  return payload as DispatchResponsePayload & LegacyDispatchResponsePayloadCompat;
}

function dashboardDiagnosticsFromEvent(
  diagnostics: FactoryWorkDiagnostics | undefined,
): DashboardWorkDiagnostics | undefined {
  if (!diagnostics) {
    return undefined;
  }
  return {
    provider: diagnostics.provider
      ? {
          model: diagnostics.provider.model,
          provider: diagnostics.provider.provider,
          request_metadata: diagnostics.provider.requestMetadata,
          response_metadata: diagnostics.provider.responseMetadata,
        }
      : undefined,
    rendered_prompt: diagnostics.renderedPrompt
      ? {
          system_prompt_hash: diagnostics.renderedPrompt.systemPromptHash,
          user_message_hash: diagnostics.renderedPrompt.userMessageHash,
          variables: diagnostics.renderedPrompt.variables,
        }
      : undefined,
  };
}

function initialPlaceForWork(
  state: WorldState,
  workTypeID: string,
): string | undefined {
  return state.topology.places?.find(
    (place) => place.type_id === workTypeID && place.category === "INITIAL",
  )?.id;
}

function outputPlaceForProjectedWorkstation(
  topology: ProjectedInitialStructure,
  transitionID: string,
  outcome: string,
  workTypeID: string,
): string | undefined {
  const workstation = topologyWorkstation(topology, transitionID);
  if (!workstation) {
    return undefined;
  }

  const routePlaceIDs =
    outcome === "FAILED" && workstation.failure_place_ids?.length
      ? workstation.failure_place_ids
      : outcome === "REJECTED" && workstation.rejection_place_ids?.length
        ? workstation.rejection_place_ids
        : workstation.output_place_ids ?? [];

  return routePlaceIDs.find(
    (placeIDValue) =>
      topology.places?.find((place) => place.id === placeIDValue)?.type_id ===
      workTypeID,
  );
}

function outputPlaceForWorkstation(
  topology: ProjectedInitialStructure,
  transitionID: string,
  outcome: string,
  workTypeID: string,
  workState?: string,
): string | undefined {
  if (workState) {
    const explicitPlace = topology.places?.find(
      (place) => place.type_id === workTypeID && place.state === workState,
    )?.id;
    if (explicitPlace) {
      return explicitPlace;
    }
  }

  return outputPlaceForProjectedWorkstation(
    topology,
    transitionID,
    outcome,
    workTypeID,
  );
}

function terminalWorkFromItems(
  state: WorldState,
  items: FactoryWorkItem[],
  outcome: string,
): FactoryTerminalWork | undefined {
  const publicItems = items.filter((item) => !isSystemTimeWorkItem(item));
  const terminal = publicItems.find(
    (item) => placeCategory(state, item.place_id) === "TERMINAL",
  );
  const failed = publicItems.find(
    (item) => placeCategory(state, item.place_id) === "FAILED",
  );
  if (outcome === "FAILED" && failed) {
    return { status: "FAILED", work_item: failed };
  }
  if (terminal) {
    return { status: "TERMINAL", work_item: terminal };
  }
  return undefined;
}

function placeCategory(
  state: WorldState,
  placeIDValue: string | undefined,
): string | undefined {
  return state.topology.places?.find((place) => place.id === placeIDValue)
    ?.category;
}

function firstRequestID(works: FactoryWork[] | undefined): string | undefined {
  return works?.find((work) => work.requestId)?.requestId;
}

function applyWorkRequest(state: WorldState, event: WorkRequestEvent): void {
  const requestID =
    event.context.requestId ?? firstRequestID(event.payload.works) ?? "";
  const traceID = event.context.traceIds?.[0];
  const workItems = (event.payload.works ?? []).map((work) =>
    factoryWorkToItem(
      state,
      work,
      initialPlaceForWork(state, eventWorkTypeID(work) ?? ""),
    ),
  );
  state.workRequestsByID[requestID] = {
    request_id: requestID,
    source: event.payload.source,
    trace_id: traceID,
    type: event.payload.type,
    work_items: workItems,
  };
  for (const item of workItems) {
    state.workItemsByID[item.id] = item;
    if (isSystemTimeWorkItem(item)) {
      continue;
    }
    addToken(state, item.place_id, item.id, item.id);
    addTraceWork(state, item);
    addTraceRequest(state, item.trace_id ?? traceID, requestID);
  }
}

function applyRelationshipChange(
  state: WorldState,
  event: RelationshipChangeRequestEvent,
): void {
  const relation = event.payload.relation as FactoryRelation;
  const targetWorkID =
    relation.targetWorkId ??
    (relation as FactoryRelation & { target_work_id?: string }).target_work_id;
  addRelation(state, {
    ...relation,
    request_id: relation.request_id ?? event.context.requestId,
    source_work_id:
      relation.source_work_id ??
      event.context.workIds?.find(
        (workID) => workID !== targetWorkID,
      ),
    trace_id: relation.trace_id ?? event.context.traceIds?.[0],
  });
}

function applyRequest(state: WorldState, event: DispatchRequestEvent): void {
  const legacyPayload = legacyDispatchRequestPayload(event.payload);
  const worker = topologyWorker(state.topology, event.payload.transitionId);
  const dispatchID =
    event.context.dispatchId ??
    (typeof legacyPayload.dispatchId === "string" ? legacyPayload.dispatchId : undefined);
  if (!dispatchID) {
    return;
  }

  const workItems = (legacyPayload.inputs ?? event.payload.inputs).map((work) =>
    factoryWorkToItem(state, work),
  );
  for (const item of workItems) {
    removeWorkToken(state, item.id);
    state.workItemsByID[item.id] = item;
    if (isSystemTimeWorkItem(item)) {
      continue;
    }
    addTraceWork(state, item);
  }
  const publicWorkItems = workItems.filter(
    (item) => !isSystemTimeWorkItem(item),
  );
  state.activeDispatches[dispatchID] = {
    consumedTokens: workItems.map((item) =>
      traceToken(item, event.context.eventTime),
    ),
    currentChainingTraceID:
      event.context.currentChainingTraceId ??
      event.payload.currentChainingTraceId ??
      legacyPayload.current_chaining_trace_id ??
      publicWorkItems.find((item) => item.current_chaining_trace_id)
        ?.current_chaining_trace_id ??
      publicWorkItems[0]?.trace_id,
    dispatchID,
    model: legacyPayload.worker?.model ?? worker?.model,
    modelProvider: workerModelProvider(legacyPayload.worker) ?? worker?.model_provider,
    previousChainingTraceIDs: event.context.previousChainingTraceIds
      ? [...event.context.previousChainingTraceIds]
      : event.payload.previousChainingTraceIds
      ? [...event.payload.previousChainingTraceIds]
      : legacyPayload.previous_chaining_trace_ids
        ? [...legacyPayload.previous_chaining_trace_ids]
      : undefined,
    provider: legacyPayload.worker?.executorProvider ?? worker?.provider,
    resources: consumeResourceUnits(state, event.payload.resources),
    startedAt: event.context.eventTime,
    systemOnly:
      event.payload.transitionId === SYSTEM_TIME_EXPIRY_TRANSITION_ID &&
      publicWorkItems.length === 0,
    traceIDs: uniqueSorted(publicWorkItems.map((item) => item.trace_id ?? "")),
    transitionID: dashboardTransitionID(event.payload.transitionId),
    workItems: publicWorkItems.map(workRef),
    workstationName: resolveWorkstationName(
      state.topology,
      event.payload.transitionId,
      legacyPayload.workstation?.name,
    ),
  };
}

function applyInferenceRequest(
  state: WorldState,
  event: InferenceRequestEvent,
): void {
  const { payload } = event;
  const dispatchID =
    event.context.dispatchId ?? legacyInferencePayloadDispatchID(payload);
  if (!dispatchID || !payload.inferenceRequestId) {
    return;
  }
  const attempts = inferenceAttemptsForDispatch(state, dispatchID);
  const current = attempts[payload.inferenceRequestId];
  const transitionID =
    legacyInferencePayloadTransitionID(payload) ??
    current?.transition_id ??
    resolveDispatchTransitionID(state, dispatchID);
  if (!transitionID) {
    return;
  }
  attempts[payload.inferenceRequestId] = {
    ...current,
    attempt: payload.attempt,
    dispatch_id: dispatchID,
    inference_request_id: payload.inferenceRequestId,
    prompt: payload.prompt,
    request_time: event.context.eventTime,
    transition_id: dashboardTransitionID(transitionID),
    working_directory: payload.workingDirectory,
    worktree: payload.worktree,
  };
}

function applyInferenceResponse(
  state: WorldState,
  event: InferenceResponseEvent,
): void {
  const { payload } = event;
  const dispatchID =
    event.context.dispatchId ?? legacyInferencePayloadDispatchID(payload);
  if (!dispatchID || !payload.inferenceRequestId) {
    return;
  }
  const attempts = inferenceAttemptsForDispatch(state, dispatchID);
  const current = attempts[payload.inferenceRequestId];
  const transitionID =
    legacyInferencePayloadTransitionID(payload) ??
    current?.transition_id ??
    resolveDispatchTransitionID(state, dispatchID);
  if (!transitionID) {
    return;
  }
  attempts[payload.inferenceRequestId] = {
    ...current,
    attempt: payload.attempt,
    diagnostics: dashboardDiagnosticsFromEvent(payload.diagnostics),
    dispatch_id: dispatchID,
    duration_millis: payload.durationMillis,
    error_class: payload.errorClass,
    exit_code: payload.exitCode,
    inference_request_id: payload.inferenceRequestId,
    outcome: payload.outcome,
    prompt: current?.prompt ?? "",
    provider_session: payload.providerSession,
    request_time: current?.request_time ?? "",
    response: payload.response,
    response_time: event.context.eventTime,
    transition_id: dashboardTransitionID(transitionID),
  };
  syncCompletedDispatchAttempt(state, dispatchID, attempts[payload.inferenceRequestId]);
}

function inferenceAttemptsForDispatch(
  state: WorldState,
  dispatchID: string,
): Record<string, DashboardInferenceAttempt> {
  const existing = state.inferenceAttemptsByDispatchID[dispatchID];
  if (existing) {
    return existing;
  }
  const attempts: Record<string, DashboardInferenceAttempt> = {};
  state.inferenceAttemptsByDispatchID[dispatchID] = attempts;
  return attempts;
}

function resolveDispatchTransitionID(
  state: WorldState,
  dispatchID: string,
): string | undefined {
  const activeTransitionID = state.activeDispatches[dispatchID]?.transitionID;
  if (activeTransitionID) {
    return activeTransitionID;
  }
  for (let index = state.completedDispatches.length - 1; index >= 0; index -= 1) {
    const completion = state.completedDispatches[index];
    if (completion.dispatchID === dispatchID) {
      return completion.transitionID;
    }
  }
  return undefined;
}

function syncCompletedDispatchAttempt(
  state: WorldState,
  dispatchID: string,
  attempt: DashboardInferenceAttempt,
): void {
  for (let index = state.completedDispatches.length - 1; index >= 0; index -= 1) {
    const completion = state.completedDispatches[index];
    if (completion.dispatchID !== dispatchID) {
      continue;
    }

    completion.diagnostics = attempt.diagnostics ?? completion.diagnostics;
    completion.providerSession =
      attempt.provider_session ?? completion.providerSession;

    if (
      completion.providerSession?.id &&
      !state.providerSessions.some(
        (providerSession) =>
          providerSession.dispatch_id === completion.dispatchID &&
          providerSession.provider_session?.id === completion.providerSession?.id,
      )
    ) {
      state.providerSessions = [
        ...state.providerSessions,
        completionToProviderSession(completion),
      ];
    }

    for (const traceID of completion.traceIDs) {
      const trace = state.tracesByID[traceID];
      if (!trace) {
        continue;
      }
      trace.dispatches = trace.dispatches.map((dispatch) =>
        dispatch.dispatch_id === completion.dispatchID
          ? completionToTraceDispatch(completion)
          : dispatch,
      );
    }

    break;
  }
}

function legacyInferencePayloadDispatchID(
  payload: InferenceRequestPayload | InferenceResponsePayload,
): string | undefined {
  const dispatchID = (payload as { dispatchId?: unknown }).dispatchId;
  return typeof dispatchID === "string" && dispatchID.length > 0
    ? dispatchID
    : undefined;
}

function legacyInferencePayloadTransitionID(
  payload: InferenceRequestPayload | InferenceResponsePayload,
): string | undefined {
  const transitionID = (payload as { transitionId?: unknown }).transitionId;
  return typeof transitionID === "string" && transitionID.length > 0
    ? transitionID
    : undefined;
}
function applyScriptRequest(
  state: WorldState,
  event: ScriptRequestEvent,
): void {
  const { payload } = event;
  if (!payload.dispatchId || !payload.scriptRequestId) {
    return;
  }
  const requests = scriptRequestsForDispatch(state, payload.dispatchId);
  requests[payload.scriptRequestId] = {
    args: [...payload.args],
    attempt: payload.attempt,
    command: payload.command,
    dispatch_id: payload.dispatchId,
    request_time: event.context.eventTime,
    script_request_id: payload.scriptRequestId,
    transition_id: payload.transitionId,
  };
}

function applyScriptResponse(
  state: WorldState,
  event: ScriptResponseEvent,
): void {
  const { payload } = event;
  if (!payload.dispatchId || !payload.scriptRequestId) {
    return;
  }
  const responses = scriptResponsesForDispatch(state, payload.dispatchId);
  responses[payload.scriptRequestId] = {
    attempt: payload.attempt,
    dispatch_id: payload.dispatchId,
    duration_millis: payload.durationMillis,
    exit_code: payload.exitCode,
    failure_type: payload.failureType,
    outcome: payload.outcome,
    response_time: event.context.eventTime,
    script_request_id: payload.scriptRequestId,
    stderr: payload.stderr,
    stdout: payload.stdout,
    transition_id: payload.transitionId,
  };
}

function scriptRequestsForDispatch(
  state: WorldState,
  dispatchID: string,
): Record<string, WorldScriptRequest> {
  const existing = state.scriptRequestsByDispatchID[dispatchID];
  if (existing) {
    return existing;
  }
  const requests: Record<string, WorldScriptRequest> = {};
  state.scriptRequestsByDispatchID[dispatchID] = requests;
  return requests;
}

function scriptResponsesForDispatch(
  state: WorldState,
  dispatchID: string,
): Record<string, WorldScriptResponse> {
  const existing = state.scriptResponsesByDispatchID[dispatchID];
  if (existing) {
    return existing;
  }
  const responses: Record<string, WorldScriptResponse> = {};
  state.scriptResponsesByDispatchID[dispatchID] = responses;
  return responses;
}

function applyResponse(state: WorldState, event: DispatchResponseEvent): void {
  const legacyPayload = legacyDispatchResponsePayload(event.payload);
  const dispatchID =
    event.context.dispatchId ??
    (typeof legacyPayload.dispatchId === "string" ? legacyPayload.dispatchId : undefined);
  if (!dispatchID) {
    return;
  }

  const active = state.activeDispatches[dispatchID];
  delete state.activeDispatches[dispatchID];
  releaseResourceUnits(
    state,
    active?.resources ?? [],
    event.payload.outputResources,
  );
  const outputItems = (event.payload.outputWork ?? []).map((work) =>
    factoryWorkToItem(
      state,
      work,
      outputPlaceForWorkstation(
        state.topology,
        event.payload.transitionId,
        event.payload.outcome,
        eventWorkTypeID(work) ?? "",
        work.state,
      ),
    ),
  );
  for (const item of outputItems) {
    state.workItemsByID[item.id] = item;
    if (isSystemTimeWorkItem(item)) {
      continue;
    }
    addTraceWork(state, item);
    addToken(state, item.place_id, item.id, item.id);
  }
  const completion = responseCompletion(state, event, active, dispatchID);
  state.completedDispatches = [...state.completedDispatches, completion];
  if (
    completion.terminalWork &&
    completion.outcome !== "FAILED" &&
    completion.terminalWork.status !== "FAILED"
  ) {
    state.terminalWorkByID[completion.terminalWork.work_item.id] =
      completion.terminalWork;
  }
  if (event.payload.outcome === "FAILED") {
    recordFailedCompletion(state, completion);
  }
  if (completion.providerSession?.id) {
    state.providerSessions = [
      ...state.providerSessions,
      completionToProviderSession(completion),
    ];
  }
  for (const traceID of completion.traceIDs) {
    addTraceDispatch(state, traceID, completion);
  }
}

function responseCompletion(
  state: WorldState,
  event: DispatchResponseEvent,
  active: WorldDispatch | undefined,
  dispatchID: string,
): WorldCompletion {
  const legacyPayload = legacyDispatchResponsePayload(event.payload);
  const outputItems = (event.payload.outputWork ?? []).map((work) =>
    factoryWorkToItem(
      state,
      work,
      outputPlaceForWorkstation(
        state.topology,
        event.payload.transitionId,
        event.payload.outcome,
        eventWorkTypeID(work) ?? "",
        work.state,
      ),
    ),
  );
  const outputRefs = outputItems
    .filter((item) => !isSystemTimeWorkItem(item))
    .map((item) => {
      state.workItemsByID[item.id] = item;
      addTraceWork(state, item);
      return workRef(item);
    });
  const terminalWork = terminalWorkFromItems(
    state,
    outputItems,
    event.payload.outcome,
  );
  const latestAttempt = latestWorkstationAttempt(
    state.inferenceAttemptsByDispatchID[dispatchID],
  );
  const terminalRefs = terminalWork ? [workRef(terminalWork.work_item)] : [];
  const workItemsByID = new Map(
    [...(active?.workItems ?? []), ...outputRefs, ...terminalRefs].map(
      (item) => [item.work_id, item],
    ),
  );
  const traceIDs = uniqueSorted([
    ...(active?.traceIDs ?? []),
    ...(event.context.traceIds ?? []),
    ...(event.context.workIds ?? []).map(
      (workID) => state.workItemsByID[workID]?.trace_id ?? "",
    ),
  ]);
  return {
    consumedTokens: active?.consumedTokens ?? [],
    currentChainingTraceID:
      event.context.currentChainingTraceId ??
      event.payload.currentChainingTraceId ??
      legacyPayload.current_chaining_trace_id ??
      active?.currentChainingTraceID ??
      traceIDs[0],
    diagnostics:
      latestAttempt?.diagnostics ??
      dashboardDiagnosticsFromEvent(legacyPayload.diagnostics),
    dispatchID,
    durationMillis: event.payload.durationMillis ?? 0,
    endTime: event.context.eventTime,
    feedback: event.payload.feedback,
    responseText: event.payload.output,
    failureMessage: event.payload.failureMessage,
    failureReason: event.payload.failureReason,
    inputItems: active?.workItems ?? [],
    outcome: event.payload.outcome,
    outputItems: uniqueSortedWorkRefs([...outputRefs, ...terminalRefs]),
    outputMutations: [],
    previousChainingTraceIDs:
      event.context.previousChainingTraceIds ??
      event.payload.previousChainingTraceIds ??
      legacyPayload.previous_chaining_trace_ids ??
      active?.previousChainingTraceIDs,
    providerSession: latestAttempt?.provider_session ?? legacyPayload.providerSession,
    resources: active?.resources ?? [],
    startedAt: active?.startedAt ?? "",
    systemOnly:
      active?.systemOnly ??
      (event.payload.transitionId === SYSTEM_TIME_EXPIRY_TRANSITION_ID &&
        workItemsByID.size === 0),
    terminalWork,
    traceIDs,
    transitionID: dashboardTransitionID(event.payload.transitionId),
    workItems: [...workItemsByID.values()].sort((left, right) =>
      left.work_id.localeCompare(right.work_id),
    ),
    workstationName: resolveWorkstationName(
      state.topology,
      event.payload.transitionId,
      legacyPayload.workstation?.name,
    ),
  };
}

function recordFailedCompletion(
  state: WorldState,
  completion: WorldCompletion,
): void {
  const workItems =
    completion.terminalWork !== undefined
      ? [workRef(completion.terminalWork.work_item)]
      : completion.workItems;
  for (const item of workItems) {
    const existing =
      state.workItemsByID[item.work_id] ?? completion.terminalWork?.work_item;
    if (!existing) {
      continue;
    }
    state.workItemsByID[existing.id] = existing;
    state.failedWorkItemsByID[existing.id] = existing;
    state.failedWorkDetailsByWorkID[existing.id] = {
      dispatch_id: completion.dispatchID,
      failure_message: completion.failureMessage,
      failure_reason: completion.failureReason,
      transition_id: completion.transitionID,
      work_item: workRef(existing),
      workstation_name: completion.workstationName,
    };
  }
}

function completionToProviderSession(
  completion: WorldCompletion,
): DashboardProviderSessionAttempt {
  return {
    diagnostics: completion.diagnostics,
    dispatch_id: completion.dispatchID,
    failure_message: completion.failureMessage,
    failure_reason: completion.failureReason,
    outcome: completion.outcome,
    provider_session: completion.providerSession,
    transition_id: completion.transitionID,
    work_items: completion.workItems.map(cloneWorkItemRef),
    workstation_name: completion.workstationName,
  };
}

function completionToTraceDispatch(
  completion: WorldCompletion,
): DashboardTraceDispatch {
  return {
    consumed_tokens: completion.consumedTokens,
    current_chaining_trace_id: completion.currentChainingTraceID,
    diagnostics: completion.diagnostics,
    dispatch_id: completion.dispatchID,
    duration_millis: completion.durationMillis,
    end_time: completion.endTime,
    failure_message: completion.failureMessage,
    failure_reason: completion.failureReason,
    input_items: completion.inputItems,
    outcome: completion.outcome,
    output_items: completion.outputItems,
    output_mutations: completion.outputMutations,
    previous_chaining_trace_ids: completion.previousChainingTraceIDs,
    provider_session: completion.providerSession,
    start_time: completion.startedAt,
    trace_id: completion.traceIDs[0],
    trace_ids: completion.traceIDs,
    transition_id: completion.transitionID,
    work_ids: completion.workItems.map((item) => item.work_id),
    token_names: uniqueSorted(
      completion.workItems.map((item) => item.display_name?.trim() || item.work_id),
    ),
    work_types: uniqueSorted(
      completion.workItems.map((item) => item.work_type_id ?? ""),
    ),
    workstation_name: completion.workstationName,
  };
}

function reconstructWorldState(
  events: FactoryEvent[],
  selectedTick: number,
): WorldState {
  const state = emptyWorldState(selectedTick);
  for (const event of orderedEvents(events)) {
    if (event.context.tick <= selectedTick) {
      applyEvent(state, event);
    }
  }
  return state;
}

function projectSnapshot(state: WorldState): FactoryTimelineSnapshot {
  const runtime = projectRuntime(state);
  const dashboard: DashboardSnapshot = {
    factory_state: state.factoryState,
    runtime,
    tick_count: state.tick,
    topology: projectTopology(state.topology),
    uptime_seconds: 0,
  };
  const tracesByWorkID = Object.fromEntries(
    Object.values(state.tracesByID).flatMap((trace) =>
      trace.work_ids.map((workID) => [workID, trace] as const),
    ),
  );
  return {
    dashboard,
    relationsByWorkID: cloneRelationsByWorkID(state.relationsByWorkID),
    tracesByWorkID: cloneTracesByWorkID(tracesByWorkID),
    workstationRequestsByDispatchID: cloneWorkstationDispatchRequestsByID(
      projectWorkstationDispatchRequestsByID(
        state,
        runtime.workstation_requests_by_dispatch_id ?? {},
      ),
    ),
    workRequestsByID: cloneWorkRequestsByID(state.workRequestsByID),
  };
}

function cloneRelationsByWorkID(
  relationsByWorkID: Record<string, FactoryRelation[]>,
): Record<string, FactoryRelation[]> {
  return Object.fromEntries(
    Object.entries(relationsByWorkID).map(([workID, relations]) => [
      workID,
      relations.map((relation) => ({ ...relation })),
    ]),
  );
}

function dedupeWorkRefs(workItems: DashboardWorkItemRef[]): DashboardWorkItemRef[] {
  const itemsByID = new Map<string, DashboardWorkItemRef>();

  for (const item of workItems) {
    if (!item.work_id) {
      continue;
    }
    itemsByID.set(item.work_id, item);
  }

  return [...itemsByID.values()].sort((left, right) => left.work_id.localeCompare(right.work_id));
}

function uniqueSortedWorkRefs(workItems: DashboardWorkItemRef[]): DashboardWorkItemRef[] {
  return dedupeWorkRefs(workItems);
}

function cloneWorkItemRef(item: DashboardWorkItemRef): DashboardWorkItemRef {
  return {
    ...item,
    previous_chaining_trace_ids: item.previous_chaining_trace_ids
      ? [...item.previous_chaining_trace_ids]
      : undefined,
  };
}

function cloneTraceToken(token: DashboardTraceToken): DashboardTraceToken {
  return {
    ...token,
    tags: token.tags ? { ...token.tags } : undefined,
  };
}

function cloneTraceMutation(
  mutation: DashboardTraceMutation,
): DashboardTraceMutation {
  return {
    ...mutation,
    resulting_token: mutation.resulting_token
      ? cloneTraceToken(mutation.resulting_token)
      : undefined,
  };
}

function cloneTraceDispatch(
  dispatch: DashboardTraceDispatch,
): DashboardTraceDispatch {
  return {
    ...dispatch,
    consumed_tokens: dispatch.consumed_tokens?.map(cloneTraceToken),
    input_items: dispatch.input_items?.map(cloneWorkItemRef),
    output_items: dispatch.output_items?.map(cloneWorkItemRef),
    output_mutations: dispatch.output_mutations?.map(cloneTraceMutation),
    previous_chaining_trace_ids: dispatch.previous_chaining_trace_ids
      ? [...dispatch.previous_chaining_trace_ids]
      : undefined,
    provider_session: dispatch.provider_session
      ? { ...dispatch.provider_session }
      : undefined,
    token_names: dispatch.token_names ? [...dispatch.token_names] : undefined,
    trace_ids: dispatch.trace_ids ? [...dispatch.trace_ids] : undefined,
    work_ids: dispatch.work_ids ? [...dispatch.work_ids] : undefined,
    work_types: dispatch.work_types ? [...dispatch.work_types] : undefined,
  };
}

function cloneTrace(trace: DashboardTrace): DashboardTrace {
  return {
    ...trace,
    dispatches: trace.dispatches.map(cloneTraceDispatch),
    relations: trace.relations?.map((relation) => ({ ...relation })),
    request_ids: trace.request_ids ? [...trace.request_ids] : undefined,
    transition_ids: [...trace.transition_ids],
    work_ids: [...trace.work_ids],
    work_items: trace.work_items?.map(cloneWorkItemRef),
    workstation_sequence: [...trace.workstation_sequence],
  };
}

function cloneTracesByWorkID(
  tracesByWorkID: Record<string, DashboardTrace>,
): Record<string, DashboardTrace> {
  return Object.fromEntries(
    Object.entries(tracesByWorkID).map(([workID, trace]) => [
      workID,
      cloneTrace(trace),
    ]),
  );
}

function cloneProviderSessionAttempts(
  attempts: DashboardProviderSessionAttempt[],
): DashboardProviderSessionAttempt[] {
  return attempts.map((attempt) => ({
    ...attempt,
    diagnostics: attempt.diagnostics
      ? {
          ...attempt.diagnostics,
          provider: attempt.diagnostics.provider
            ? {
                ...attempt.diagnostics.provider,
                request_metadata: attempt.diagnostics.provider.request_metadata
                  ? { ...attempt.diagnostics.provider.request_metadata }
                  : undefined,
                response_metadata: attempt.diagnostics.provider.response_metadata
                  ? { ...attempt.diagnostics.provider.response_metadata }
                  : undefined,
              }
            : undefined,
          rendered_prompt: attempt.diagnostics.rendered_prompt
            ? {
                ...attempt.diagnostics.rendered_prompt,
                variables: attempt.diagnostics.rendered_prompt.variables
                  ? { ...attempt.diagnostics.rendered_prompt.variables }
                  : undefined,
              }
            : undefined,
        }
      : undefined,
    provider_session: attempt.provider_session
      ? { ...attempt.provider_session }
      : undefined,
    work_items: attempt.work_items?.map(cloneWorkItemRef),
  }));
}

function cloneFailedWorkDetailsByWorkID(
  failedWorkDetailsByWorkID: Record<string, DashboardFailedWorkDetail>,
): Record<string, DashboardFailedWorkDetail> | undefined {
  const entries = Object.entries(failedWorkDetailsByWorkID).map(
    ([workID, detail]) => [
      workID,
      {
        ...detail,
        work_item: cloneWorkItemRef(detail.work_item),
      },
    ],
  );

  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}

function cloneWorkRequestsByID(
  workRequestsByID: Record<string, TimelineWorkRequestPayload>,
): Record<string, TimelineWorkRequestPayload> {
  return Object.fromEntries(
    Object.entries(workRequestsByID).map(([requestID, request]) => [
      requestID,
      {
        ...request,
        work_items: request.work_items?.map((item) => ({
          ...item,
          tags: item.tags ? { ...item.tags } : undefined,
        })),
      },
    ]),
  );
}

function cloneWorkstationDispatchRequestsByID(
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>,
): Record<string, DashboardWorkstationRequest> {
  return Object.fromEntries(
    Object.entries(workstationRequestsByDispatchID).map(([dispatchID, request]) => [
      dispatchID,
      {
        ...request,
        inference_attempts: request.inference_attempts.map((attempt) => ({ ...attempt })),
        counts: request.counts ? { ...request.counts } : undefined,
        request_metadata: request.request_metadata
          ? { ...request.request_metadata }
          : undefined,
        request_view: cloneRuntimeWorkstationRequestRequest(request.request_view),
        response_metadata: request.response_metadata
          ? { ...request.response_metadata }
          : undefined,
        response_view: cloneRuntimeWorkstationRequestResponse(request.response_view),
        script_request: request.script_request
          ? {
              ...request.script_request,
              args: request.script_request.args
                ? [...request.script_request.args]
                : undefined,
            }
          : undefined,
        script_response: request.script_response ? { ...request.script_response } : undefined,
        trace_ids: request.trace_ids ? [...request.trace_ids] : undefined,
        work_items: request.work_items.map(cloneWorkItemRef),
      },
    ]),
  );
}

function cloneRuntimeWorkstationRequestRequest(
  request: DashboardRuntimeWorkstationRequest["request"] | undefined,
): DashboardRuntimeWorkstationRequest["request"] | undefined {
  if (!request) {
    return undefined;
  }

  return {
    ...request,
    consumedTokens: request.consumedTokens?.map((token) => ({
      ...token,
      tags: token.tags ? { ...token.tags } : undefined,
    })),
    inputWorkItems: request.inputWorkItems?.map(cloneWorkItemRef),
    inputWorkTypeIds: request.inputWorkTypeIds
      ? [...request.inputWorkTypeIds]
      : undefined,
    requestMetadata: request.requestMetadata
      ? { ...request.requestMetadata }
      : undefined,
    traceIds: request.traceIds ? [...request.traceIds] : undefined,
  };
}

function cloneRuntimeWorkstationRequestResponse(
  response: DashboardRuntimeWorkstationRequest["response"] | undefined,
): DashboardRuntimeWorkstationRequest["response"] | undefined {
  if (!response) {
    return undefined;
  }

  const diagnostics = response.diagnostics as
    | (DashboardWorkDiagnostics & {
        provider?: DashboardWorkDiagnostics["provider"] & {
          requestMetadata?: Record<string, string>;
          responseMetadata?: Record<string, string>;
        };
        renderedPrompt?: {
          systemPromptHash?: string;
          userMessageHash?: string;
          variables?: Record<string, string>;
        };
      })
    | undefined;

  return {
    ...response,
    diagnostics: diagnostics
      ? {
          ...diagnostics,
          provider: diagnostics.provider
            ? {
                ...diagnostics.provider,
                requestMetadata: diagnostics.provider.requestMetadata
                  ? { ...diagnostics.provider.requestMetadata }
                  : undefined,
                responseMetadata: diagnostics.provider.responseMetadata
                  ? { ...diagnostics.provider.responseMetadata }
                  : undefined,
              }
            : undefined,
          renderedPrompt: diagnostics.renderedPrompt
            ? {
                ...diagnostics.renderedPrompt,
                variables: diagnostics.renderedPrompt.variables
                  ? { ...diagnostics.renderedPrompt.variables }
                  : undefined,
              }
            : undefined,
        }
      : undefined,
    outputMutations: response.outputMutations?.map((mutation) => ({
      ...mutation,
      resulting_token: mutation.resulting_token
        ? {
            ...mutation.resulting_token,
            tags: mutation.resulting_token.tags
              ? { ...mutation.resulting_token.tags }
              : undefined,
          }
        : undefined,
    })),
    outputWorkItems: response.outputWorkItems?.map(cloneWorkItemRef),
    providerSession: response.providerSession
      ? { ...response.providerSession }
      : undefined,
    responseMetadata: response.responseMetadata
      ? { ...response.responseMetadata }
      : undefined,
  };
}

function cloneInferenceAttemptsByDispatchID(
  attemptsByDispatchID: Record<
    string,
    Record<string, DashboardInferenceAttempt>
  >,
): Record<string, Record<string, DashboardInferenceAttempt>> | undefined {
  const entries = Object.entries(attemptsByDispatchID).map(
    ([dispatchID, attempts]) =>
      [
        dispatchID,
        Object.fromEntries(
          Object.entries(attempts).map(([requestID, attempt]) => [
            requestID,
            {
              ...attempt,
              diagnostics: attempt.diagnostics
                ? {
                    ...attempt.diagnostics,
                    provider: attempt.diagnostics.provider
                      ? {
                          ...attempt.diagnostics.provider,
                          request_metadata: attempt.diagnostics.provider.request_metadata
                            ? { ...attempt.diagnostics.provider.request_metadata }
                            : undefined,
                          response_metadata: attempt.diagnostics.provider.response_metadata
                            ? { ...attempt.diagnostics.provider.response_metadata }
                            : undefined,
                        }
                      : undefined,
                    rendered_prompt: attempt.diagnostics.rendered_prompt
                      ? {
                          ...attempt.diagnostics.rendered_prompt,
                          variables: attempt.diagnostics.rendered_prompt.variables
                            ? { ...attempt.diagnostics.rendered_prompt.variables }
                            : undefined,
                        }
                      : undefined,
                  }
                : undefined,
              provider_session: attempt.provider_session
                ? { ...attempt.provider_session }
                : undefined,
            },
          ]),
        ),
      ] as const,
  );
  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}

function projectTopology(
  topology: ProjectedInitialStructure,
): DashboardSnapshot["topology"] {
  const workstations = [...(topology.workstations ?? [])].sort((left, right) =>
    left.id.localeCompare(right.id),
  );
  const placesByID = Object.fromEntries(
    (topology.places ?? []).map((place) => [place.id, place]),
  );
  const workTypeIDs = new Set(
    (topology.work_types ?? []).map((workType) => workType.id),
  );
  const resourceIDs = new Set(
    (topology.resources ?? []).map((resource) => resource.id),
  );
  return {
    edges: buildTopologyEdges(workstations, placesByID, workTypeIDs),
    submit_work_types: projectSubmitWorkTypes(topology),
    workstation_node_ids: workstations.map((workstation) => workstation.id),
    workstation_nodes_by_id: Object.fromEntries(
      workstations.map((workstation) => [
        workstation.id,
        projectWorkstation(workstation, placesByID, workTypeIDs, resourceIDs),
      ]),
    ),
  };
}

function projectSubmitWorkTypes(
  topology: ProjectedInitialStructure,
): NonNullable<DashboardSnapshot["topology"]["submit_work_types"]> {
  return uniqueSorted(
    (topology.work_types ?? [])
      .filter(isSubmitEligibleWorkType)
      .map((workType) => resolveConfiguredWorkTypeName(workType.id, workType.name)),
  ).map((workTypeName) => ({ work_type_name: workTypeName }));
}

export function resolveConfiguredWorkTypeName(
  id: string,
  name?: string,
): string {
  return name || id;
}

function isSubmitEligibleWorkType(
  workType: NonNullable<ProjectedInitialStructure["work_types"]>[number],
): boolean {
  if (isSystemTimeWorkType(workType.id)) {
    return false;
  }
  return (workType.states ?? []).some((state) => state.category === "INITIAL");
}

function buildTopologyEdges(
  workstations: FactoryWorkstationShape[],
  placesByID: Record<string, FactoryPlace>,
  workTypeIDs: Set<string>,
): DashboardWorkstationEdge[] {
  const inputsByPlace = new Map<string, string[]>();
  for (const workstation of workstations) {
    for (const placeID of workstation.input_place_ids ?? []) {
      const place = placesByID[placeID];
      if (!place || !workTypeIDs.has(place.type_id)) {
        continue;
      }
      inputsByPlace.set(
        placeID,
        [...(inputsByPlace.get(placeID) ?? []), workstation.id].sort(),
      );
    }
  }

  const edges: DashboardWorkstationEdge[] = [];
  const seen = new Set<string>();
  for (const workstation of workstations) {
    edges.push(
      ...buildTopologyEdgesForPlaces(
        workstation.id,
        workstation.output_place_ids ?? [],
        "accepted",
        inputsByPlace,
        placesByID,
        seen,
      ),
      ...buildTopologyEdgesForPlaces(
        workstation.id,
        workstation.rejection_place_ids ?? [],
        "rejected",
        inputsByPlace,
        placesByID,
        seen,
      ),
      ...buildTopologyEdgesForPlaces(
        workstation.id,
        workstation.failure_place_ids ?? [],
        "failed",
        inputsByPlace,
        placesByID,
        seen,
      ),
    );
  }
  return edges;
}

function buildTopologyEdgesForPlaces(
  sourceID: string,
  placeIDs: string[],
  outcome: NonNullable<DashboardWorkstationEdge["outcome_kind"]>,
  inputsByPlace: Map<string, string[]>,
  placesByID: Record<string, FactoryPlace>,
  seen: Set<string>,
): DashboardWorkstationEdge[] {
  return uniqueSorted(placeIDs).flatMap((placeID) => {
    const place = placesByID[placeID];
    if (!place) {
      return [];
    }
    return (inputsByPlace.get(placeID) ?? []).flatMap((destID) => {
      const edgeID = `${sourceID}:${destID}:${placeID}:${outcome}`;
      if (seen.has(edgeID)) {
        return [];
      }
      seen.add(edgeID);
      return [
        {
          edge_id: edgeID,
          from_node_id: sourceID,
          outcome_kind: outcome,
          state_category: place.category as StateCategory | undefined,
          state_value: place.state,
          to_node_id: destID,
          via_place_id: placeID,
          work_type_id: place.type_id,
        },
      ];
    });
  });
}

function projectWorkstation(
  workstation: FactoryWorkstationShape,
  placesByID: Record<string, FactoryPlace>,
  workTypeIDs: Set<string>,
  resourceIDs: Set<string>,
): DashboardSnapshot["topology"]["workstation_nodes_by_id"][string] {
  const outputPlaceIDs = [
    ...(workstation.output_place_ids ?? []),
    ...(workstation.rejection_place_ids ?? []),
    ...(workstation.failure_place_ids ?? []),
  ];
  return {
    input_place_ids: workstation.input_place_ids,
    input_places: placeRefs(
      workstation.input_place_ids ?? [],
      placesByID,
      workTypeIDs,
      resourceIDs,
    ),
    input_work_type_ids: workTypeIDsForPlaces(
      workstation.input_place_ids ?? [],
      placesByID,
      workTypeIDs,
    ),
    node_id: workstation.id,
    output_place_ids: outputPlaceIDs,
    output_places: placeRefs(
      outputPlaceIDs,
      placesByID,
      workTypeIDs,
      resourceIDs,
    ),
    output_work_type_ids: workTypeIDsForPlaces(
      outputPlaceIDs,
      placesByID,
      workTypeIDs,
    ),
    transition_id: workstation.id,
    worker_type: workstation.worker_id,
    workstation_kind: workstation.kind,
    workstation_name: workstation.name,
  };
}

type FactoryWorkstationShape = NonNullable<
  ProjectedInitialStructure["workstations"]
>[number];

function placeRefs(
  ids: string[],
  placesByID: Record<string, FactoryPlace>,
  workTypeIDs: Set<string>,
  resourceIDs: Set<string>,
): NonNullable<
  DashboardSnapshot["topology"]["workstation_nodes_by_id"][string]["input_places"]
> {
  return ids.flatMap((id) => {
    const place = placesByID[id];
    if (!place) {
      return [];
    }
    const kind: DashboardPlaceKind = workTypeIDs.has(place.type_id)
      ? "work_state"
      : resourceIDs.has(place.type_id)
        ? "resource"
        : "constraint";
    return [
      {
        kind,
        place_id: place.id,
        state_category: place.category as StateCategory | undefined,
        state_value: place.state,
        type_id: place.type_id,
      },
    ];
  });
}

function workTypeIDsForPlaces(
  ids: string[],
  placesByID: Record<string, FactoryPlace>,
  workTypeIDs: Set<string>,
): string[] {
  return uniqueSorted(
    ids
      .map((id) => placesByID[id]?.type_id ?? "")
      .filter((id) => workTypeIDs.has(id)),
  );
}

function projectRuntime(state: WorldState): DashboardSnapshot["runtime"] {
  const activeDispatches = Object.values(state.activeDispatches);
  const customerActiveDispatches = activeDispatches.filter(
    dispatchHasCustomerWork,
  );
  const customerCompletedDispatches = state.completedDispatches.filter(
    dispatchHasCustomerWork,
  );
  const activeExecutions = Object.fromEntries(
    customerActiveDispatches.map((dispatch) => [
      dispatch.dispatchID,
      projectActiveExecution(dispatch),
    ]),
  );
  const activeDispatchIDs = Object.keys(activeExecutions).sort();
  return {
    active_dispatch_ids: activeDispatchIDs,
    active_executions_by_dispatch_id: activeExecutions,
    active_workstation_node_ids: uniqueSorted(
      activeDispatches.map((dispatch) => dispatch.transitionID),
    ),
    inference_attempts_by_dispatch_id: cloneInferenceAttemptsByDispatchID(
      state.inferenceAttemptsByDispatchID,
    ),
    in_flight_dispatch_count: activeDispatchIDs.length,
    place_token_counts: Object.fromEntries(
      Object.values(state.occupancyByID)
        .filter((occupancy) => !isSystemTimePlace(occupancy.placeID))
        .map((occupancy) => [occupancy.placeID, occupancy.tokenCount]),
    ),
    current_work_items_by_place_id: projectCurrentWorkItemsByPlaceID(state),
    place_occupancy_work_items_by_place_id:
      projectOccupancyWorkItemsByPlaceID(state),
    workstation_requests_by_dispatch_id: projectWorkstationRequests(
      customerActiveDispatches,
      customerCompletedDispatches,
      state.inferenceAttemptsByDispatchID,
      state.scriptRequestsByDispatchID,
      state.scriptResponsesByDispatchID,
    ),
    session: {
      completed_count: customerCompletedDispatches.filter(
        (dispatch) => dispatch.outcome === "ACCEPTED",
      ).length,
      completed_work_labels: uniqueSorted(
        Object.values(state.terminalWorkByID).map(
          (work) => work.work_item.display_name ?? work.work_item.id,
        ),
      ),
      dispatched_count:
        activeDispatchIDs.length + customerCompletedDispatches.length,
      failed_by_work_type: countFailedByWorkType(state.failedWorkItemsByID),
      failed_count: Object.keys(state.failedWorkItemsByID).length,
      failed_work_details_by_work_id: cloneFailedWorkDetailsByWorkID(
        state.failedWorkDetailsByWorkID,
      ),
      failed_work_labels: uniqueSorted(
        Object.values(state.failedWorkItemsByID).map(
          (work) => work.display_name ?? work.id,
        ),
      ),
      has_data:
        activeDispatchIDs.length > 0 ||
        customerCompletedDispatches.length > 0 ||
        Object.values(state.workItemsByID).some(
          (item) => !isSystemTimeWorkItem(item),
        ),
      provider_sessions: cloneProviderSessionAttempts(state.providerSessions),
    },
    workstation_activity_by_node_id: projectActivity(customerActiveDispatches),
  };
}

function dispatchHasCustomerWork(dispatch: WorldDispatch): boolean {
  return !dispatch.systemOnly;
}

function projectWorkstationRequests(
  activeDispatches: WorldDispatch[],
  completedDispatches: WorldCompletion[],
  attemptsByDispatchID: Record<
    string,
    Record<string, DashboardInferenceAttempt>
  >,
  scriptRequestsByDispatchID: Record<string, Record<string, WorldScriptRequest>>,
  scriptResponsesByDispatchID: Record<
    string,
    Record<string, WorldScriptResponse>
  >,
): Record<string, DashboardRuntimeWorkstationRequest> | undefined {
  const requests: Record<string, DashboardRuntimeWorkstationRequest> = {};

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

  return Object.keys(requests).length > 0 ? requests : undefined;
}

function workstationRequestFromActiveDispatch(
  dispatch: WorldDispatch,
  attempts: Record<string, DashboardInferenceAttempt> | undefined,
  latestScriptRequest: WorldScriptRequest | undefined,
): DashboardRuntimeWorkstationRequest {
  const inputWorkItems = workItemsFromTokens(
    dispatch.consumedTokens,
    dispatch.workItems,
  );
  const latestAttempt = latestWorkstationAttempt(attempts);
  return {
    counts: workstationRequestCounts(undefined, undefined, undefined),
    dispatchId: dispatch.dispatchID,
    dispatch_id: dispatch.dispatchID,
    request: {
      consumedTokens: dispatch.consumedTokens,
      consumed_tokens: dispatch.consumedTokens,
      currentChainingTraceId: dispatch.currentChainingTraceID,
      current_chaining_trace_id: dispatch.currentChainingTraceID,
      inputWorkItems: inputWorkItems,
      input_work_items: inputWorkItems,
      inputWorkTypeIds: uniqueSorted(
        inputWorkItems.map((item) => item.work_type_id ?? ""),
      ),
      input_work_type_ids: uniqueSorted(
        inputWorkItems.map((item) => item.work_type_id ?? ""),
      ),
      model: dispatch.model,
      previousChainingTraceIds: dispatch.previousChainingTraceIDs
        ? [...dispatch.previousChainingTraceIDs]
        : undefined,
      previous_chaining_trace_ids: dispatch.previousChainingTraceIDs
        ? [...dispatch.previousChainingTraceIDs]
        : undefined,
      prompt: latestAttempt?.prompt,
      provider: resolveWorkstationRequestProvider(
        undefined,
        undefined,
        dispatch,
      ),
      requestMetadata: workstationRequestMetadata(undefined),
      request_metadata: workstationRequestMetadata(undefined),
      requestTime: latestAttempt?.request_time,
      request_time: latestAttempt?.request_time,
      scriptRequest: dashboardScriptRequest(latestScriptRequest),
      script_request: dashboardScriptRequest(latestScriptRequest),
      startedAt: dispatch.startedAt,
      started_at: dispatch.startedAt,
      traceIds: uniqueSorted(dispatch.traceIDs),
      trace_ids: uniqueSorted(dispatch.traceIDs),
      workingDirectory: resolveWorkingDirectory(latestAttempt, undefined),
      working_directory: resolveWorkingDirectory(latestAttempt, undefined),
      worktree: resolveWorktree(latestAttempt, undefined),
    },
    transitionId: dispatch.transitionID,
    transition_id: dispatch.transitionID,
    workstationName: dispatch.workstationName,
    workstation_name: dispatch.workstationName,
  };
}

function workstationRequestFromCompletion(
  completion: WorldCompletion,
  attempts: Record<string, DashboardInferenceAttempt> | undefined,
  latestScriptRequest: WorldScriptRequest | undefined,
  latestScriptResponse: WorldScriptResponse | undefined,
): DashboardRuntimeWorkstationRequest {
  const inputWorkItems = workItemsFromTokens(
    completion.consumedTokens,
    completion.workItems,
  );
  const latestAttempt = latestWorkstationAttempt(attempts);
  return {
    counts: workstationRequestCounts(undefined, undefined, undefined),
    dispatchId: completion.dispatchID,
    dispatch_id: completion.dispatchID,
    request: {
      consumedTokens: completion.consumedTokens,
      consumed_tokens: completion.consumedTokens,
      currentChainingTraceId: completion.currentChainingTraceID,
      current_chaining_trace_id: completion.currentChainingTraceID,
      inputWorkItems: inputWorkItems,
      input_work_items: inputWorkItems,
      inputWorkTypeIds: uniqueSorted(
        inputWorkItems.map((item) => item.work_type_id ?? ""),
      ),
      input_work_type_ids: uniqueSorted(
        inputWorkItems.map((item) => item.work_type_id ?? ""),
      ),
      model: completion.diagnostics?.provider?.model,
      previousChainingTraceIds: completion.previousChainingTraceIDs
        ? [...completion.previousChainingTraceIDs]
        : undefined,
      previous_chaining_trace_ids: completion.previousChainingTraceIDs
        ? [...completion.previousChainingTraceIDs]
        : undefined,
      prompt: latestAttempt?.prompt,
      provider: resolveWorkstationRequestProvider(
        completion.diagnostics,
        completion.providerSession,
      ),
      requestMetadata: workstationRequestMetadata(completion.diagnostics),
      request_metadata: workstationRequestMetadata(completion.diagnostics),
      requestTime: latestAttempt?.request_time,
      request_time: latestAttempt?.request_time,
      scriptRequest: dashboardScriptRequest(latestScriptRequest),
      script_request: dashboardScriptRequest(latestScriptRequest),
      startedAt: completion.startedAt,
      started_at: completion.startedAt,
      traceIds: uniqueSorted(completion.traceIDs),
      trace_ids: uniqueSorted(completion.traceIDs),
      workingDirectory: resolveWorkingDirectory(
        latestAttempt,
        completion.diagnostics,
      ),
      working_directory: resolveWorkingDirectory(
        latestAttempt,
        completion.diagnostics,
      ),
      worktree: resolveWorktree(latestAttempt, completion.diagnostics),
    },
    response: {
      diagnostics: completion.diagnostics,
      durationMillis: completion.durationMillis,
      duration_millis: completion.durationMillis,
      endTime: completion.endTime,
      end_time: completion.endTime,
      errorClass: latestAttempt?.error_class,
      error_class: latestAttempt?.error_class,
      feedback: completion.feedback,
      failureMessage: completion.failureMessage,
      failure_message: completion.failureMessage,
      failureReason: completion.failureReason,
      failure_reason: completion.failureReason,
      outcome: completion.outcome,
      outputMutations: completion.outputMutations,
      output_mutations: completion.outputMutations,
      outputWorkItems: outputWorkItemsFromCompletion(completion),
      output_work_items: outputWorkItemsFromCompletion(completion),
      providerSession: completion.providerSession,
      provider_session: completion.providerSession,
      responseMetadata: workstationResponseMetadata(completion.diagnostics),
      response_metadata: workstationResponseMetadata(completion.diagnostics),
      responseText:
        latestAttempt?.response ??
        (latestScriptResponse ? undefined : completion.responseText),
      response_text:
        latestAttempt?.response ??
        (latestScriptResponse ? undefined : completion.responseText),
      scriptResponse: dashboardScriptResponse(latestScriptResponse),
      script_response: dashboardScriptResponse(latestScriptResponse),
    },
    transitionId: completion.transitionID,
    transition_id: completion.transitionID,
    workstationName: completion.workstationName,
    workstation_name: completion.workstationName,
  };
}

function workstationRequestCounts(
  attempts: Record<string, DashboardInferenceAttempt> | undefined,
  scriptRequests: Record<string, WorldScriptRequest> | undefined,
  scriptResponses: Record<string, WorldScriptResponse> | undefined,
): DashboardRuntimeWorkstationRequest["counts"] {
  const counts = {
    dispatchedCount: 0,
    erroredCount: 0,
    respondedCount: 0,
    dispatched_count: 0,
    errored_count: 0,
    responded_count: 0,
  };
  for (const attempt of Object.values(attempts ?? {})) {
    if (attempt.inference_request_id) {
      counts.dispatchedCount += 1;
      counts.dispatched_count += 1;
    }
    if (attemptHasError(attempt)) {
      counts.erroredCount += 1;
      counts.errored_count += 1;
      continue;
    }
    if (attemptHasResponse(attempt)) {
      counts.respondedCount += 1;
      counts.responded_count += 1;
    }
  }
  for (const request of Object.values(scriptRequests ?? {})) {
    if (request.script_request_id) {
      counts.dispatchedCount += 1;
      counts.dispatched_count += 1;
    }
  }
  for (const response of Object.values(scriptResponses ?? {})) {
    if (!response.response_time) {
      continue;
    }
    if (scriptResponseErrored(response)) {
      counts.erroredCount += 1;
      counts.errored_count += 1;
      continue;
    }
    counts.respondedCount += 1;
    counts.responded_count += 1;
  }
  return counts;
}

function latestWorkstationAttempt(
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

function latestWorkstationScriptRequest(
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

function latestWorkstationScriptResponse(
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

function workstationScriptRequestForProjection(
  response: WorldScriptResponse | undefined,
  requests: Record<string, WorldScriptRequest> | undefined,
): WorldScriptRequest | undefined {
  if (response?.script_request_id && requests?.[response.script_request_id]) {
    return requests[response.script_request_id];
  }
  return latestWorkstationScriptRequest(requests);
}

function dashboardScriptRequest(
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

function dashboardScriptResponse(
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

function scriptResponseErrored(response: WorldScriptResponse): boolean {
  return (
    response.failure_type !== undefined ||
    response.outcome === "FAILED_EXIT_CODE" ||
    response.outcome === "PROCESS_ERROR" ||
    response.outcome === "TIMED_OUT"
  );
}

function resolveWorkstationRequestProvider(
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

function resolveWorkingDirectory(
  attempt: DashboardInferenceAttempt | undefined,
  diagnostics: DashboardWorkDiagnostics | undefined,
): string | undefined {
  return (
    attempt?.working_directory ??
    diagnostics?.provider?.request_metadata?.working_directory
  );
}

function resolveWorktree(
  attempt: DashboardInferenceAttempt | undefined,
  diagnostics: DashboardWorkDiagnostics | undefined,
): string | undefined {
  return attempt?.worktree ?? diagnostics?.provider?.request_metadata?.worktree;
}

function workstationRequestMetadata(
  diagnostics: DashboardWorkDiagnostics | undefined,
): Record<string, string> | undefined {
  const requestMetadata = diagnostics?.provider?.request_metadata;
  return requestMetadata ? { ...requestMetadata } : undefined;
}

function workstationResponseMetadata(
  diagnostics: DashboardWorkDiagnostics | undefined,
): Record<string, string> | undefined {
  const responseMetadata = diagnostics?.provider?.response_metadata;
  return responseMetadata ? { ...responseMetadata } : undefined;
}

function workItemsFromTokens(
  tokens: DashboardTraceToken[],
  fallback: DashboardWorkItemRef[],
): DashboardWorkItemRef[] {
  const fallbackByID = new Map(
    fallback.map((item) => [item.work_id, item] as const),
  );
  const workItems = Object.values(
    Object.fromEntries(
      tokens
        .filter(
          (token) =>
            token.work_id && token.work_type_id !== DASHBOARD_TIME_WORK_TYPE_ID,
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
    : [...fallback].sort((left, right) =>
        left.work_id.localeCompare(right.work_id),
      );
}

function outputWorkItemsFromCompletion(
  completion: WorldCompletion,
): DashboardWorkItemRef[] | undefined {
  const workItems: Record<string, DashboardWorkItemRef> = Object.fromEntries(
    completion.outputItems
      .filter((item) => item.work_type_id !== DASHBOARD_TIME_WORK_TYPE_ID)
      .map((item) => [item.work_id, item] as const),
  );
  for (const mutation of completion.outputMutations) {
    const token = mutation.resulting_token;
    if (
      !token ||
      !token.work_id ||
      token.work_type_id === DASHBOARD_TIME_WORK_TYPE_ID
    ) {
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
    completion.terminalWork.work_item.work_type_id !==
      DASHBOARD_TIME_WORK_TYPE_ID
  ) {
    const item = completion.terminalWork.work_item;
    workItems[item.id] = workRef(item);
  }
  const values = Object.values(workItems).sort((left, right) =>
    left.work_id.localeCompare(right.work_id),
  );
  return values.length > 0 ? values : undefined;
}

function projectCurrentWorkItemsByPlaceID(
  state: WorldState,
): Record<string, DashboardWorkItemRef[]> {
  const workTypeIDs = new Set(
    (state.topology.work_types ?? []).map((workType) => workType.id),
  );
  const entries = (state.topology.places ?? [])
    .filter((place) => workTypeIDs.has(place.type_id))
    .filter(
      (place) => place.category !== "TERMINAL" && place.category !== "FAILED",
    )
    .sort((left, right) => left.id.localeCompare(right.id))
    .map((place) => {
      const occupancy = state.occupancyByID[place.id];
      const workItems = uniqueSorted(occupancy?.workItemIDs ?? [])
        .map((workID) => state.workItemsByID[workID])
        .filter((item): item is FactoryWorkItem => item !== undefined)
        .filter((item) => state.failedWorkItemsByID[item.id] === undefined)
        .filter((item) => state.terminalWorkByID[item.id] === undefined)
        .map(workRef);
      return [place.id, workItems] as const;
    });
  return Object.fromEntries(entries);
}

function projectOccupancyWorkItemsByPlaceID(
  state: WorldState,
): Record<string, DashboardWorkItemRef[]> {
  const entries = (state.topology.places ?? [])
    .sort((left, right) => left.id.localeCompare(right.id))
    .map((place) => {
      const occupancy = state.occupancyByID[place.id];
      const workItems = uniqueSorted(occupancy?.workItemIDs ?? [])
        .map((workID) => state.workItemsByID[workID])
        .filter((item): item is FactoryWorkItem => item !== undefined)
        .map(workRef);
      return [place.id, workItems] as const;
    });
  return Object.fromEntries(entries);
}

function projectActiveExecution(
  dispatch: WorldDispatch,
): DashboardActiveExecution {
  return {
    consumed_tokens: dispatch.consumedTokens,
    dispatch_id: dispatch.dispatchID,
    model: dispatch.model,
    model_provider: dispatch.modelProvider,
    provider: dispatch.provider,
    started_at: dispatch.startedAt,
    trace_ids: [...dispatch.traceIDs],
    transition_id: dispatch.transitionID,
    work_items: dispatch.workItems.map(cloneWorkItemRef),
    work_type_ids: uniqueSorted(
      dispatch.workItems.map((item) => item.work_type_id ?? ""),
    ),
    workstation_name: dispatch.workstationName,
    workstation_node_id: dispatch.transitionID,
  };
}

function projectActivity(
  activeDispatches: WorldDispatch[],
): DashboardSnapshot["runtime"]["workstation_activity_by_node_id"] {
  return Object.fromEntries(
    activeDispatches.map((dispatch) => [
      dispatch.transitionID,
      {
        active_dispatch_ids: [dispatch.dispatchID],
        active_work_items: dispatch.workItems.map(cloneWorkItemRef),
        trace_ids: [...dispatch.traceIDs],
        workstation_node_id: dispatch.transitionID,
      },
    ]),
  );
}

function countFailedByWorkType(
  values: Record<string, FactoryWorkItem>,
): Record<string, number> | undefined {
  const counts: Record<string, number> = {};
  for (const item of Object.values(values)) {
    if (isSystemTimeWorkItem(item)) {
      continue;
    }
    counts[item.work_type_id] = (counts[item.work_type_id] ?? 0) + 1;
  }
  return Object.keys(counts).length > 0 ? counts : undefined;
}

function sortInferenceAttempts(
  attempts: DashboardInferenceAttempt[],
): DashboardInferenceAttempt[] {
  return [...attempts].sort((left, right) => {
    if (left.attempt !== right.attempt) {
      return left.attempt - right.attempt;
    }
    return left.inference_request_id.localeCompare(right.inference_request_id);
  });
}

function attemptHasResponse(attempt: DashboardInferenceAttempt): boolean {
  return attempt.response_time !== undefined && !attemptHasError(attempt);
}

function attemptHasError(attempt: DashboardInferenceAttempt): boolean {
  return attempt.error_class !== undefined || attempt.outcome === "FAILED";
}

function requestIDsByWorkItemID(
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

function projectWorkstationDispatchRequest(
  dispatch: WorldDispatch,
  completion: WorldCompletion | undefined,
  runtimeRequest: DashboardRuntimeWorkstationRequest | undefined,
  inferenceAttemptsByDispatchID: Record<string, Record<string, DashboardInferenceAttempt>>,
  scriptRequestsByDispatchID: Record<string, Record<string, WorldScriptRequest>>,
  scriptResponsesByDispatchID: Record<string, Record<string, WorldScriptResponse>>,
  requestIDsByWorkID: Record<string, string[]>,
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
    dispatched_request_count: counts.dispatchedCount ?? counts.dispatched_count ?? 0,
    errored_request_count: counts.erroredCount ?? counts.errored_count ?? 0,
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
    provider_session:
      responseView?.providerSession ??
      completion?.providerSession,
    request_view: requestView,
    request_id: requestIDs[0],
    request_metadata: requestView?.requestMetadata ?? diagnostics?.requestMetadata,
    responded_request_count: counts.respondedCount ?? counts.responded_count ?? 0,
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

function projectWorkstationDispatchRequestsByID(
  state: WorldState,
  runtimeRequestsByDispatchID: Record<string, DashboardRuntimeWorkstationRequest>,
): Record<string, DashboardWorkstationRequest> {
  const requestIDsByWorkID = requestIDsByWorkItemID(state.workRequestsByID);
  const dispatchRequests = new Map<string, DashboardWorkstationRequest>();

  for (const dispatch of Object.values(state.activeDispatches)) {
    if (!dispatchHasCustomerWork(dispatch)) {
      continue;
    }

    dispatchRequests.set(
      dispatch.dispatchID,
      projectWorkstationDispatchRequest(
        dispatch,
        undefined,
        runtimeRequestsByDispatchID[dispatch.dispatchID],
        state.inferenceAttemptsByDispatchID,
        state.scriptRequestsByDispatchID,
        state.scriptResponsesByDispatchID,
        requestIDsByWorkID,
      ),
    );
  }

  for (const completion of state.completedDispatches) {
    if (!dispatchHasCustomerWork(completion)) {
      continue;
    }

    dispatchRequests.set(
      completion.dispatchID,
      projectWorkstationDispatchRequest(
        completion,
        completion,
        runtimeRequestsByDispatchID[completion.dispatchID],
        state.inferenceAttemptsByDispatchID,
        state.scriptRequestsByDispatchID,
        state.scriptResponsesByDispatchID,
        requestIDsByWorkID,
      ),
    );
  }

  return Object.fromEntries(
    [...dispatchRequests.entries()].sort(([left], [right]) => left.localeCompare(right)),
  );
}

export function buildFactoryTimelineSnapshot(
  events: FactoryEvent[],
  selectedTick: number,
): FactoryTimelineSnapshot {
  return projectSnapshot(reconstructWorldState(events, selectedTick));
}

function cacheWithSnapshot(
  events: FactoryEvent[],
  cache: Record<number, FactoryTimelineSnapshot>,
  tick: number,
): Record<number, FactoryTimelineSnapshot> {
  return cache[tick]
    ? cache
    : { ...cache, [tick]: buildFactoryTimelineSnapshot(events, tick) };
}

function appendTimelineEvents(
  current: Pick<
    FactoryTimelineState,
    | "events"
    | "latestTick"
    | "mode"
    | "receivedEventIDs"
    | "selectedTick"
    | "worldViewCache"
  >,
  incomingEvents: FactoryEvent[],
): Pick<
  FactoryTimelineState,
  "events" | "latestTick" | "mode" | "receivedEventIDs" | "selectedTick" | "worldViewCache"
> {
  const receivedEventIDs = new Set(current.receivedEventIDs);
  const nextEvents = incomingEvents.filter((event) => !receivedEventIDs.has(event.id));

  if (nextEvents.length === 0) {
    return {
      events: current.events,
      latestTick: current.latestTick,
      mode: current.mode,
      receivedEventIDs: current.receivedEventIDs,
      selectedTick: current.selectedTick,
      worldViewCache: current.worldViewCache,
    };
  }

  const events = orderedEvents([...current.events, ...nextEvents]);
  const latestTick = nextEvents.reduce(
    (maxTick, event) => Math.max(maxTick, event.context.tick),
    current.latestTick,
  );
  const selectedTick = current.mode === "current" ? latestTick : current.selectedTick;

  return {
    events,
    latestTick,
    mode: current.mode,
    receivedEventIDs: [...current.receivedEventIDs, ...nextEvents.map((event) => event.id)],
    selectedTick,
    worldViewCache: cacheWithSnapshot(events, {}, selectedTick),
  };
}

export const useFactoryTimelineStore = create<FactoryTimelineState>((set) => ({
  ...EMPTY_TIMELINE_STATE,
  appendEvent: (event) => {
    set((current) => appendTimelineEvents(current, [event]));
  },
  appendEvents: (events) => {
    set((current) => appendTimelineEvents(current, events));
  },
  replaceEvents: (events) => {
    const ordered = orderedEvents(events);
    const latestTick = Math.max(
      0,
      ...ordered.map((event) => event.context.tick),
    );
    set({
      events: ordered,
      latestTick,
      mode: "current",
      receivedEventIDs: ordered.map((event) => event.id),
      selectedTick: latestTick,
      worldViewCache: cacheWithSnapshot(ordered, {}, latestTick),
    });
  },
  reset: () => {
    set(EMPTY_TIMELINE_STATE);
  },
  selectTick: (tick) => {
    set((current) => ({
      mode: "fixed",
      selectedTick: tick,
      worldViewCache: cacheWithSnapshot(
        current.events,
        current.worldViewCache,
        tick,
      ),
    }));
  },
  setCurrentMode: () => {
    set((current) => ({
      mode: "current",
      selectedTick: current.latestTick,
      worldViewCache: cacheWithSnapshot(
        current.events,
        current.worldViewCache,
        current.latestTick,
      ),
    }));
  },
}));
