import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
  FactoryGraphDraftEdgeChange,
  FactoryGraphNodeReference,
  FactoryGraphWorkStateReference,
  FactoryWorker,
  FactoryWorkState,
  FactoryWorkstation,
  FactoryWorkstationIO,
} from "./factory-graph-draft-types";
import { buildEdge, edgeChangeId } from "./factory-graph-draft-types";
import { validateFactoryGraphDraft } from "./factory-graph-draft-validation";

const DEFAULT_RESOURCE_REQUIREMENT_CAPACITY = 1;

export function buildPendingFactoryDefinition(
  baseFactoryDefinition: CanonicalFactoryDefinition,
  draft: FactoryGraphDraft,
): CanonicalFactoryDefinition | null {
  if (validateFactoryGraphDraft(baseFactoryDefinition, draft).length > 0) {
    return null;
  }

  return buildDraftAppliedFactoryDefinition(baseFactoryDefinition, draft);
}

export function buildDraftAppliedFactoryDefinition(
  baseFactoryDefinition: CanonicalFactoryDefinition,
  draft: FactoryGraphDraft,
): CanonicalFactoryDefinition {
  const nextFactoryDefinition = structuredClone(baseFactoryDefinition);
  nextFactoryDefinition.resources = applyNamedEntityChanges(
    baseFactoryDefinition.resources,
    draft.removals.resources,
    draft.additions.resources,
  );
  nextFactoryDefinition.workers = applyNamedEntityChanges(
    baseFactoryDefinition.workers,
    draft.removals.workers,
    draft.additions.workers,
  ).map((worker) => applyWorkerResourceChanges(worker, draft));
  nextFactoryDefinition.workstations = applyNamedEntityChanges(
    baseFactoryDefinition.workstations,
    draft.removals.workstations,
    draft.additions.workstations,
  ).map((workstation) => applyWorkstationEdgeChanges(workstation, draft));
  nextFactoryDefinition.workTypes = applyNamedEntityChanges(
    baseFactoryDefinition.workTypes,
    draft.removals.workTypes,
    draft.additions.workTypes,
  ).map((workType) => ({
    ...workType,
    states: applyWorkStateChanges(workType.states, draft, workType.name),
  }));

  return nextFactoryDefinition;
}

export function applyNamedEntityChanges<T extends { name: string }>(
  baseItems: T[] | undefined,
  removals: string[],
  additions: T[],
): T[] {
  const retainedItems = (baseItems ?? []).filter(
    (item) => !removals.includes(item.name),
  );
  return [...retainedItems, ...additions.map((item) => structuredClone(item))];
}

export function applyWorkStateChanges(
  baseStates: FactoryWorkState[],
  draft: FactoryGraphDraft,
  workTypeName: string,
): FactoryWorkState[] {
  const removedStateNames = new Set(
    draft.removals.workStates
      .filter((state) => state.workTypeName === workTypeName)
      .map((state) => state.stateName),
  );
  const retainedStates = baseStates.filter(
    (state) => !removedStateNames.has(state.name),
  );
  const addedStates = draft.additions.workStates
    .filter((state) => state.workTypeName === workTypeName)
    .map((state) => structuredClone(state.state));

  return [...retainedStates, ...addedStates];
}

export function applyWorkstationEdgeChanges(
  workstation: FactoryWorkstation,
  draft: FactoryGraphDraft,
): FactoryWorkstation {
  const workstationKey: FactoryGraphNodeReference = {
    kind: "workstation",
    name: workstation.name,
  };
  const nextWorkstation = structuredClone(workstation);

  nextWorkstation.inputs = applyIOEdgeChanges(
    nextWorkstation.inputs,
    draft,
    workstationKey,
    "workstation-input",
  );
  nextWorkstation.outputs = applyOptionalIOEdgeChanges(
    nextWorkstation.outputs,
    draft,
    workstationKey,
    "workstation-output",
  );
  nextWorkstation.onContinue = applyOptionalIOEdgeChanges(
    nextWorkstation.onContinue,
    draft,
    workstationKey,
    "workstation-on-continue",
  );
  nextWorkstation.onFailure = applyOptionalIOEdgeChanges(
    nextWorkstation.onFailure,
    draft,
    workstationKey,
    "workstation-on-failure",
  );
  nextWorkstation.onRejection = applyOptionalIOEdgeChanges(
    nextWorkstation.onRejection,
    draft,
    workstationKey,
    "workstation-on-rejection",
  );
  nextWorkstation.resources = applyOptionalResourceEdgeChanges(
    nextWorkstation.resources,
    draft,
    workstationKey,
    "workstation-resource",
  );

  const removedAssignment = draft.edgeChanges.removals.some(
    (edge) =>
      edge.kind === "worker-assignment" &&
      edge.target.kind === "workstation" &&
      edge.target.name === workstation.name &&
      edge.source.kind === "worker" &&
      edge.source.name === nextWorkstation.worker,
  );
  const addedAssignment = draft.edgeChanges.additions.find(
    (edge) =>
      edge.kind === "worker-assignment" &&
      edge.target.kind === "workstation" &&
      edge.target.name === workstation.name &&
      edge.source.kind === "worker",
  );

  if (removedAssignment && !addedAssignment) {
    nextWorkstation.worker = "";
  }
  if (addedAssignment?.source.kind === "worker") {
    nextWorkstation.worker = addedAssignment.source.name;
  }

  return nextWorkstation;
}

export function applyWorkerResourceChanges(
  worker: FactoryWorker,
  draft: FactoryGraphDraft,
): FactoryWorker {
  const workerKey: FactoryGraphNodeReference = {
    kind: "worker",
    name: worker.name,
  };
  const nextWorker = structuredClone(worker);
  nextWorker.resources = applyOptionalResourceEdgeChanges(
    nextWorker.resources,
    draft,
    workerKey,
    "worker-resource",
  );
  return nextWorker;
}

function applyIOEdgeChanges(
  baseItems: FactoryWorkstationIO[],
  draft: FactoryGraphDraft,
  workstation: FactoryGraphNodeReference,
  kind:
    | "workstation-input"
    | "workstation-on-continue"
    | "workstation-on-failure"
    | "workstation-on-rejection"
    | "workstation-output",
): FactoryWorkstationIO[] {
  const removedEdgeIds = new Set(
    draft.edgeChanges.removals
      .filter((edge) => edgeTouchesWorkstation(edge, workstation, kind))
      .map(edgeChangeId),
  );
  const retained = baseItems.filter(
    (item) =>
      !removedEdgeIds.has(
        edgeChangeId(
          buildIOEdge(kind, workstation, {
            kind: "work-state",
            stateName: item.state,
            workTypeName: item.workType,
          }),
        ),
      ),
  );
  const additions = draft.edgeChanges.additions
    .filter((edge) => edgeTouchesWorkstation(edge, workstation, kind))
    .map((edge) => edgeToIO(kind, edge));

  return dedupeIOEntries([...retained, ...additions]);
}

function applyOptionalIOEdgeChanges(
  baseItems: FactoryWorkstationIO[] | undefined,
  draft: FactoryGraphDraft,
  workstation: FactoryGraphNodeReference,
  kind:
    | "workstation-output"
    | "workstation-on-continue"
    | "workstation-on-failure"
    | "workstation-on-rejection",
) {
  const nextItems = applyIOEdgeChanges(
    baseItems ?? [],
    draft,
    workstation,
    kind,
  );
  return nextItems.length > 0 ? nextItems : undefined;
}

function applyOptionalResourceEdgeChanges(
  baseItems:
    | NonNullable<FactoryWorkstation["resources"]>
    | NonNullable<FactoryWorker["resources"]>
    | undefined,
  draft: FactoryGraphDraft,
  target: FactoryGraphNodeReference,
  kind: "worker-resource" | "workstation-resource",
) {
  const removedResourceNames = new Set(
    draft.edgeChanges.removals
      .filter((edge) => isResourceEdgeForTarget(edge, kind, target))
      .map((edge) => edge.source.name),
  );
  const retained = (baseItems ?? []).filter(
    (resource) => !removedResourceNames.has(resource.name),
  );
  const additions = draft.edgeChanges.additions
    .filter((edge) => isResourceEdgeForTarget(edge, kind, target))
    .map((edge) => ({
      capacity: DEFAULT_RESOURCE_REQUIREMENT_CAPACITY,
      name: edge.source.name,
    }));

  const nextResources = dedupeNamedResources([...retained, ...additions]);
  return nextResources.length > 0 ? nextResources : undefined;
}

function edgeTouchesWorkstation(
  edge: FactoryGraphDraftEdgeChange,
  workstation: FactoryGraphNodeReference,
  kind:
    | "workstation-input"
    | "workstation-on-continue"
    | "workstation-on-failure"
    | "workstation-on-rejection"
    | "workstation-output",
) {
  return (
    edge.kind === kind &&
    ((edge.target.kind === "workstation" &&
      edge.target.name === workstation.name) ||
      (edge.source.kind === "workstation" &&
        edge.source.name === workstation.name))
  );
}

function isResourceEdgeForTarget(
  edge: FactoryGraphDraftEdgeChange,
  kind: "worker-resource" | "workstation-resource",
  target: FactoryGraphNodeReference,
): edge is FactoryGraphDraftEdgeChange & {
  source: FactoryGraphNodeReference & { kind: "resource" };
} {
  return (
    edge.kind === kind &&
    edge.source.kind === "resource" &&
    edge.target.kind === target.kind &&
    edge.target.name === target.name
  );
}

function buildIOEdge(
  kind:
    | "workstation-input"
    | "workstation-on-continue"
    | "workstation-on-failure"
    | "workstation-on-rejection"
    | "workstation-output",
  workstation: FactoryGraphNodeReference,
  workState: FactoryGraphWorkStateReference,
) {
  return kind === "workstation-input"
    ? buildEdge(kind, workState, workstation)
    : buildEdge(kind, workstation, workState);
}

function edgeToIO(
  kind:
    | "workstation-input"
    | "workstation-on-continue"
    | "workstation-on-failure"
    | "workstation-on-rejection"
    | "workstation-output",
  edgeChange: FactoryGraphDraftEdgeChange,
): FactoryWorkstationIO {
  const workState =
    kind === "workstation-input" ? edgeChange.source : edgeChange.target;
  if (workState.kind !== "work-state") {
    return { state: "", workType: "" };
  }
  return {
    state: workState.stateName,
    workType: workState.workTypeName,
  };
}

function dedupeIOEntries(items: FactoryWorkstationIO[]): FactoryWorkstationIO[] {
  const seen = new Set<string>();
  return items.filter((item) => {
    const key = `${item.workType}:${item.state}`;
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}

function dedupeNamedResources<
  T extends {
    capacity: number;
    name: string;
  },
>(resources: T[]): T[] {
  const seen = new Set<string>();
  return resources.filter((resource) => {
    if (seen.has(resource.name)) {
      return false;
    }
    seen.add(resource.name);
    return true;
  });
}
