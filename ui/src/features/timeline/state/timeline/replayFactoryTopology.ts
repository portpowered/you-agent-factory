import type {
  FactoryDefinition,
  FactoryPlace,
  FactoryWork,
  FactoryWorkItem,
  FactoryWorker,
  FactoryWorkstation,
  FactoryWorkType,
  InitialStructureRequestPayload,
  WorkstationIO,
} from "../../../../api/events";
import { dashboardWorkstationName, isSystemTimeWorkType, isSystemTimeWorkstation, SYSTEM_TIME_PENDING_PLACE_ID } from "./systemTime";
import type { ProjectedInitialStructure, WorldState } from "./types";

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

function workstationFailureIO(workstation: FactoryWorkstation): WorkstationIO | undefined {
  return workstation.onFailure;
}

function workstationRejectionIO(workstation: FactoryWorkstation): WorkstationIO | undefined {
  return workstation.onRejection;
}

function workstationSchedulingKind(workstation: FactoryWorkstation): string | undefined {
  return workstation.behavior ?? workstation.type;
}

export function workerModelProvider(worker: FactoryWorker | undefined): string | undefined {
  return worker?.modelProvider;
}

function placeID(workTypeID: string, state: string): string {
  return `${workTypeID}:${state}`;
}

function placeIDFromIO(io: { state: string; workType?: string }): string {
  return placeID(ioWorkType(io), io.state);
}

function isPublicWorkstationIO(io: { workType?: string }): boolean {
  return !isSystemTimeWorkType(ioWorkType(io));
}

function projectWorkstationTopology(
  workstation: FactoryWorkstation,
): NonNullable<ProjectedInitialStructure["workstations"]>[number] {
  const inputs = workstation.inputs.filter(isPublicWorkstationIO);
  const outputs = workstation.outputs.filter(isPublicWorkstationIO);
  const failure = workstationFailureIO(workstation);
  const rejection = workstationRejectionIO(workstation);

  return {
    failure_place_ids: failure && isPublicWorkstationIO(failure) ? [placeIDFromIO(failure)] : undefined,
    id: workstation.id ?? workstation.name,
    input_place_ids: inputs.map(placeIDFromIO),
    kind: workstationSchedulingKind(workstation),
    name: workstation.name,
    output_place_ids: outputs.map(placeIDFromIO),
    rejection_place_ids:
      rejection && isPublicWorkstationIO(rejection) ? [placeIDFromIO(rejection)] : undefined,
    worker_id: workstation.worker,
  };
}

export function normalizeFactoryPayload(
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
      places.set(id, { id, state: io.state, type_id: workTypeID });
    }
  }
  for (const resource of factory.resources ?? []) {
    const id = `${resource.name}:available`;
    places.set(id, { category: "PROCESSING", id, state: "available", type_id: resource.name });
  }
  return {
    places: [...places.values()].sort((left, right) => left.id.localeCompare(right.id)),
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

export function seedResourceOccupancy(
  state: WorldState,
  addToken: (state: WorldState, placeID: string | undefined, tokenID: string, workItemID?: string) => void,
  resourceTokenID: (resourceID: string, index: number) => string,
): void {
  for (const resource of state.topology.resources ?? []) {
    const place = (state.topology.places ?? [])
      .filter((candidate) => candidate.type_id === resource.id)
      .sort((left, right) => {
        if (left.state === "available" && right.state !== "available") return -1;
        if (right.state === "available" && left.state !== "available") return 1;
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

type FactoryWorkstationShape = NonNullable<ProjectedInitialStructure["workstations"]>[number];

function topologyWorkstation(
  topology: ProjectedInitialStructure,
  transitionID: string,
): FactoryWorkstationShape | undefined {
  return (topology.workstations ?? []).find(
    (workstation) => workstation.id === transitionID || workstation.name === transitionID,
  );
}

export function resolveWorkstationName(
  topology: ProjectedInitialStructure,
  transitionID: string,
  name: string | undefined,
): string | undefined {
  return dashboardWorkstationName(
    transitionID,
    name ?? topologyWorkstation(topology, transitionID)?.name,
  );
}

export function topologyWorker(
  topology: ProjectedInitialStructure,
  transitionID: string,
): NonNullable<ProjectedInitialStructure["workers"]>[number] | undefined {
  const workstation = topologyWorkstation(topology, transitionID);
  if (!workstation?.worker_id) {
    return undefined;
  }
  return (topology.workers ?? []).find((worker) => worker.id === workstation.worker_id);
}

export function factoryWorkToItem(
  state: WorldState,
  work: FactoryWork | { workId: string },
  placeIDOverride?: string,
): FactoryWorkItem {
  const legacyWork = work as (FactoryWork | { workId: string }) & {
    current_chaining_trace_id?: string;
    previous_chaining_trace_ids?: string[];
    trace_id?: string;
    work_id?: string;
    work_type_id?: string;
  };
  const workID =
    ("workId" in work && typeof work.workId === "string" ? work.workId : undefined) ??
    legacyWork.work_id ??
    ("name" in work && typeof work.name === "string" ? work.name : undefined) ??
    "";
  const existing = state.workItemsByID[workID];
  const workTypeID =
    ("workTypeName" in work && typeof work.workTypeName === "string" ? work.workTypeName : undefined) ??
    ("work_type_name" in work && typeof work.work_type_name === "string" ? work.work_type_name : undefined) ??
    legacyWork.work_type_id ??
    existing?.work_type_id;
  const placeIDValue = placeIDOverride ?? existing?.place_id ?? (isSystemTimeWorkType(workTypeID) ? SYSTEM_TIME_PENDING_PLACE_ID : undefined);
  return {
    current_chaining_trace_id: legacyWork.current_chaining_trace_id ?? existing?.current_chaining_trace_id,
    display_name: ("name" in work && typeof work.name === "string" ? work.name : undefined) || existing?.display_name,
    id: workID,
    place_id: placeIDValue,
    previous_chaining_trace_ids: legacyWork.previous_chaining_trace_ids ?? existing?.previous_chaining_trace_ids,
    tags: ("tags" in work ? work.tags : undefined) ?? existing?.tags,
    trace_id:
      ("traceId" in work && typeof work.traceId === "string" ? work.traceId : undefined) ??
      ("trace_id" in work && typeof work.trace_id === "string" ? work.trace_id : undefined) ??
      existing?.trace_id,
    work_type_id: workTypeID ?? existing?.work_type_id ?? "",
  };
}

export function eventWorkTypeID(work: FactoryWork): string | undefined {
  return (
    work.workTypeName ??
    (work as FactoryWork & { work_type_name?: string }).work_type_name ??
    (work as FactoryWork & { work_type_id?: string }).work_type_id
  );
}

export function initialPlaceForWork(
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
    (placeIDValue) => topology.places?.find((place) => place.id === placeIDValue)?.type_id === workTypeID,
  );
}

export function outputPlaceForWorkstation(
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
  return outputPlaceForProjectedWorkstation(topology, transitionID, outcome, workTypeID);
}


