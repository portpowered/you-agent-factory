import type {
  DashboardTrace,
  DashboardTraceDispatch,
  DashboardTraceToken,
  DashboardWorkRelation,
} from "../../api/dashboard";
import type { FactoryRelation, FactoryResource, FactoryWorkItem } from "../../api/events";
import { uniqueSortedWorkRefs } from "./cloneTimelineSnapshot";
import { uniqueSorted } from "./shared";
import { dashboardPlaceID, dashboardWorkTypeID } from "./systemTime";
import type { ResourceUnit, WorldCompletion, WorldState } from "./types";
import { workRef } from "./workItemRef";

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

function dedupeRelations(relations: DashboardWorkRelation[]): DashboardWorkRelation[] {
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

export function addTraceWork(state: WorldState, item: FactoryWorkItem): void {
  if (!item.trace_id) {
    return;
  }
  const trace = state.tracesByID[item.trace_id] ?? emptyTrace(item.trace_id);
  trace.work_ids = uniqueSorted([...trace.work_ids, item.id]);
  trace.work_items = uniqueSortedWorkRefs([...(trace.work_items ?? []), workRef(item)]);
  state.tracesByID[item.trace_id] = trace;
}

export function addTraceRequest(
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
  trace.relations = dedupeRelations([...(trace.relations ?? []), toDashboardRelation(relation)]);
  state.tracesByID[traceID] = trace;
}

export function addRelation(state: WorldState, relation: FactoryRelation): void {
  const targetWorkID =
    relation.targetWorkId ??
    (relation as FactoryRelation & { target_work_id?: string }).target_work_id;
  if (!relation.source_work_id || !targetWorkID) {
    return;
  }
  const relations = state.relationsByWorkID[relation.source_work_id] ?? [];
  if (!relations.some((current) => relationKey(current) === relationKey(relation))) {
    state.relationsByWorkID[relation.source_work_id] = [...relations, relation].sort(
      (left, right) => relationKey(left).localeCompare(relationKey(right)),
    );
  }
  addTraceRelation(state, relation.trace_id, relation);
  addTraceRelation(state, state.workItemsByID[relation.source_work_id]?.trace_id, relation);
  addTraceRelation(state, state.workItemsByID[targetWorkID]?.trace_id, relation);
}

export function addTraceDispatch(
  state: WorldState,
  traceID: string,
  completion: WorldCompletion,
  completionToTraceDispatch: (completion: WorldCompletion) => DashboardTraceDispatch,
): void {
  if (!traceID) {
    return;
  }
  const trace = state.tracesByID[traceID] ?? emptyTrace(traceID);
  trace.dispatches = [...trace.dispatches, completionToTraceDispatch(completion)];
  trace.transition_ids = uniqueSorted([...trace.transition_ids, completion.transitionID]);
  trace.workstation_sequence = [
    ...trace.workstation_sequence,
    completion.workstationName ?? completion.transitionID,
  ];
  state.tracesByID[traceID] = trace;
}

export function addToken(
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
    occupancy.workItemIDs = uniqueSorted([...occupancy.workItemIDs, workItemID]);
  } else {
    occupancy.resourceTokenIDs = uniqueSorted([...occupancy.resourceTokenIDs, tokenID]);
  }
  occupancy.tokenCount = occupancy.resourceTokenIDs.length + occupancy.workItemIDs.length;
  state.occupancyByID[placeID] = occupancy;
}

export function removeWorkToken(state: WorldState, workID: string): void {
  const placeID = state.workItemsByID[workID]?.place_id;
  if (!placeID) {
    return;
  }
  const occupancy = state.occupancyByID[placeID];
  if (!occupancy) {
    return;
  }
  occupancy.workItemIDs = occupancy.workItemIDs.filter((id) => id !== workID);
  occupancy.tokenCount = occupancy.resourceTokenIDs.length + occupancy.workItemIDs.length;
  if (occupancy.tokenCount === 0) {
    delete state.occupancyByID[placeID];
  }
}

function removeResourceToken(state: WorldState, placeID: string, tokenID: string): void {
  const occupancy = state.occupancyByID[placeID];
  if (!occupancy) {
    return;
  }
  occupancy.resourceTokenIDs = occupancy.resourceTokenIDs.filter((id) => id !== tokenID);
  occupancy.tokenCount = occupancy.resourceTokenIDs.length + occupancy.workItemIDs.length;
  if (occupancy.tokenCount === 0) {
    delete state.occupancyByID[placeID];
  }
}

function firstAvailableResourceTokenID(
  state: WorldState,
  resourceAvailablePlaceID: (resourceID: string) => string,
  resourceID: string,
): string | undefined {
  return state.occupancyByID[resourceAvailablePlaceID(resourceID)]?.resourceTokenIDs[0];
}

export function resourceAvailablePlaceID(resourceID: string): string {
  return `${resourceID}:available`;
}

export function resourceTokenID(resourceID: string, index: number): string {
  return `${resourceID}:resource:${index}`;
}

function resourceIDsFromEvent(resources: FactoryResource[] | undefined): string[] {
  return (resources ?? []).map((resource) => resource.name).filter((name) => name.length > 0);
}

export function consumeResourceUnits(
  state: WorldState,
  resources: FactoryResource[] | undefined,
): ResourceUnit[] {
  return resourceIDsFromEvent(resources).map((resourceID) => {
    const placeID = resourceAvailablePlaceID(resourceID);
    const tokenID = firstAvailableResourceTokenID(state, resourceAvailablePlaceID, resourceID) ?? "";
    if (tokenID) {
      removeResourceToken(state, placeID, tokenID);
    }
    return { placeID, resourceID, tokenID };
  });
}

export function releaseResourceUnits(
  state: WorldState,
  consumed: ResourceUnit[],
  resources: FactoryResource[] | undefined,
): void {
  const released = new Set<number>();
  for (const resourceID of resourceIDsFromEvent(resources)) {
    const index = consumed.findIndex(
      (unit, candidateIndex) => !released.has(candidateIndex) && unit.resourceID === resourceID,
    );
    if (index < 0) {
      continue;
    }
    released.add(index);
    const unit = consumed[index];
    if (!unit?.tokenID) {
      continue;
    }
    addToken(state, unit.placeID || resourceAvailablePlaceID(unit.resourceID), unit.tokenID);
  }
}

export function traceToken(item: FactoryWorkItem, eventTime: string): DashboardTraceToken {
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
