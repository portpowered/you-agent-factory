import type {
  CanonicalFactoryDefinition,
  FactoryGraphEdge,
  FactoryGraphNode,
  FactoryGraphNodeReference,
  FactoryGraphTopology,
  FactoryGraphWorkStateReference,
  FactoryWorkstationIO,
} from "./factory-graph-draft-types";
import { buildEdge, buildNode, nodeKeyId } from "./factory-graph-draft-types";

type NodeMap = Map<string, FactoryGraphNode>;
type EdgeMap = Map<string, FactoryGraphEdge>;
type WorkstationRouteInput =
  | FactoryWorkstationIO
  | FactoryWorkstationIO[]
  | null
  | undefined;

interface FactoryGraphEntityIndex {
  resourceIdsByName: ReadonlyMap<string, string>;
  workerIdsByName: ReadonlyMap<string, string>;
  workStateIdsByWorkTypeName: ReadonlyMap<string, ReadonlyMap<string, string>>;
  workTypeIdsByName: ReadonlyMap<string, string>;
}

export function buildFactoryGraphTopologyFromDefinition(
  factoryDefinition: CanonicalFactoryDefinition,
): FactoryGraphTopology {
  const nodes = new Map<string, FactoryGraphNode>();
  const edges = new Map<string, FactoryGraphEdge>();
  const entityIndex = indexFactoryGraphEntities(factoryDefinition);

  appendSupportingFileNodes(factoryDefinition, nodes);
  appendResourceNodes(factoryDefinition, entityIndex, nodes);
  appendWorkerNodes(factoryDefinition, entityIndex, nodes, edges);
  appendWorkTypeNodes(factoryDefinition, entityIndex, nodes, edges);
  appendWorkstationNodes(factoryDefinition, entityIndex, nodes, edges);

  return {
    edges: Array.from(edges.values()).sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    nodes: Array.from(nodes.values()).sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
  };
}

function appendSupportingFileNodes(
  factoryDefinition: CanonicalFactoryDefinition,
  nodes: NodeMap,
) {
  for (const bundledFile of factoryDefinition.supportingFiles?.bundledFiles ??
    []) {
    const targetPath = bundledFile.targetPath?.trim();
    if (!targetPath) {
      continue;
    }

    const key: FactoryGraphNodeReference = {
      id: targetPath,
      kind: "doc",
      name: targetPath,
      sourceFileType: bundledFile.type,
    };
    nodes.set(nodeKeyId(key), buildNode(key));
  }
}

function appendResourceNodes(
  factoryDefinition: CanonicalFactoryDefinition,
  entityIndex: FactoryGraphEntityIndex,
  nodes: NodeMap,
) {
  for (const resource of factoryDefinition.resources ?? []) {
    const key: FactoryGraphNodeReference = {
      id: entityIndex.resourceIdsByName.get(resource.name),
      kind: "resource",
      name: resource.name,
    };
    nodes.set(nodeKeyId(key), buildNode(key));
  }
}

function appendWorkerNodes(
  factoryDefinition: CanonicalFactoryDefinition,
  entityIndex: FactoryGraphEntityIndex,
  nodes: NodeMap,
  edges: EdgeMap,
) {
  for (const worker of factoryDefinition.workers ?? []) {
    const workerKey: FactoryGraphNodeReference = {
      id: entityIndex.workerIdsByName.get(worker.name),
      kind: "worker",
      name: worker.name,
    };
    nodes.set(nodeKeyId(workerKey), buildNode(workerKey));

    for (const resource of worker.resources ?? []) {
      const resourceKey: FactoryGraphNodeReference = {
        id: entityIndex.resourceIdsByName.get(resource.name),
        kind: "resource",
        name: resource.name,
      };
      nodes.set(nodeKeyId(resourceKey), buildNode(resourceKey));
      const edge = buildEdge("worker-resource", resourceKey, workerKey);
      edges.set(edge.id, edge);
    }
  }
}

function appendWorkTypeNodes(
  factoryDefinition: CanonicalFactoryDefinition,
  entityIndex: FactoryGraphEntityIndex,
  nodes: NodeMap,
  edges: EdgeMap,
) {
  for (const workType of factoryDefinition.workTypes ?? []) {
    const workTypeKey: FactoryGraphNodeReference = {
      id: entityIndex.workTypeIdsByName.get(workType.name),
      kind: "work-type",
      name: workType.name,
    };
    nodes.set(nodeKeyId(workTypeKey), buildNode(workTypeKey));

    for (const state of workType.states) {
      const workStateKey: FactoryGraphWorkStateReference = {
        kind: "work-state",
        stateId: entityIndex.workStateIdsByWorkTypeName
          .get(workType.name)
          ?.get(state.name),
        stateName: state.name,
        workTypeId: entityIndex.workTypeIdsByName.get(workType.name),
        workTypeName: workType.name,
      };
      nodes.set(nodeKeyId(workStateKey), buildNode(workStateKey));
      const edge = buildEdge("work-type-state", workTypeKey, workStateKey);
      edges.set(edge.id, edge);
    }
  }
}

function appendWorkstationNodes(
  factoryDefinition: CanonicalFactoryDefinition,
  entityIndex: FactoryGraphEntityIndex,
  nodes: NodeMap,
  edges: EdgeMap,
) {
  for (const workstation of factoryDefinition.workstations ?? []) {
    const workstationKey: FactoryGraphNodeReference = {
      id: workstation.id,
      kind: "workstation",
      name: workstation.name,
    };
    nodes.set(nodeKeyId(workstationKey), buildNode(workstationKey));

    appendWorkstationWorkerEdge(
      workstation,
      workstationKey,
      entityIndex,
      nodes,
      edges,
    );
    appendWorkstationResourceEdges(
      workstation,
      workstationKey,
      entityIndex,
      nodes,
      edges,
    );
    appendWorkstationStateEdges(
      workstation.inputs,
      "workstation-input",
      workstationKey,
      entityIndex,
      nodes,
      edges,
    );
    appendWorkstationStateEdges(
      workstation.outputs,
      "workstation-output",
      workstationKey,
      entityIndex,
      nodes,
      edges,
    );
    appendWorkstationStateEdges(
      workstation.onContinue,
      "workstation-on-continue",
      workstationKey,
      entityIndex,
      nodes,
      edges,
    );
    appendWorkstationStateEdges(
      workstation.onFailure,
      "workstation-on-failure",
      workstationKey,
      entityIndex,
      nodes,
      edges,
    );
    appendWorkstationStateEdges(
      workstation.onRejection,
      "workstation-on-rejection",
      workstationKey,
      entityIndex,
      nodes,
      edges,
    );
  }
}

function appendWorkstationWorkerEdge(
  workstation: NonNullable<CanonicalFactoryDefinition["workstations"]>[number],
  workstationKey: FactoryGraphNodeReference,
  entityIndex: FactoryGraphEntityIndex,
  nodes: NodeMap,
  edges: EdgeMap,
) {
  const workerName = (workstation.worker ?? "").trim();
  if (workerName.length === 0) {
    return;
  }

  const workerKey: FactoryGraphNodeReference = {
    id: entityIndex.workerIdsByName.get(workerName),
    kind: "worker",
    name: workerName,
  };
  nodes.set(nodeKeyId(workerKey), buildNode(workerKey));
  const workerEdge = buildEdge("worker-assignment", workerKey, workstationKey);
  edges.set(workerEdge.id, workerEdge);
}

function appendWorkstationResourceEdges(
  workstation: NonNullable<CanonicalFactoryDefinition["workstations"]>[number],
  workstationKey: FactoryGraphNodeReference,
  entityIndex: FactoryGraphEntityIndex,
  nodes: NodeMap,
  edges: EdgeMap,
) {
  for (const resource of workstation.resources ?? []) {
    const resourceKey: FactoryGraphNodeReference = {
      id: entityIndex.resourceIdsByName.get(resource.name),
      kind: "resource",
      name: resource.name,
    };
    nodes.set(nodeKeyId(resourceKey), buildNode(resourceKey));
    const edge = buildEdge("workstation-resource", resourceKey, workstationKey);
    edges.set(edge.id, edge);
  }
}

function appendWorkstationStateEdges(
  items: WorkstationRouteInput,
  kind:
    | "workstation-input"
    | "workstation-on-continue"
    | "workstation-on-failure"
    | "workstation-on-rejection"
    | "workstation-output",
  workstationKey: FactoryGraphNodeReference,
  entityIndex: FactoryGraphEntityIndex,
  nodes: NodeMap,
  edges: EdgeMap,
) {
  for (const item of workstationRouteIOs(items)) {
    const edge = buildWorkstationIOEdge(
      kind,
      item,
      workstationKey,
      entityIndex,
      nodes,
    );
    edges.set(edge.id, edge);
  }
}

function workstationRouteIOs(
  items: WorkstationRouteInput,
): FactoryWorkstationIO[] {
  if (!items) {
    return [];
  }
  return Array.isArray(items) ? items : [items];
}

function buildWorkstationIOEdge(
  kind:
    | "workstation-input"
    | "workstation-on-continue"
    | "workstation-on-failure"
    | "workstation-on-rejection"
    | "workstation-output",
  io: FactoryWorkstationIO,
  workstationKey: FactoryGraphNodeReference,
  entityIndex: FactoryGraphEntityIndex,
  nodes: NodeMap,
): FactoryGraphEdge {
  const workStateKey: FactoryGraphWorkStateReference = {
    kind: "work-state",
    stateId: entityIndex.workStateIdsByWorkTypeName
      .get(io.workType)
      ?.get(io.state),
    stateName: io.state,
    workTypeId: entityIndex.workTypeIdsByName.get(io.workType),
    workTypeName: io.workType,
  };
  nodes.set(nodeKeyId(workStateKey), buildNode(workStateKey));
  return kind === "workstation-input"
    ? buildEdge(kind, workStateKey, workstationKey)
    : buildEdge(kind, workstationKey, workStateKey);
}

function indexFactoryGraphEntities(
  factoryDefinition: CanonicalFactoryDefinition,
): FactoryGraphEntityIndex {
  const resourceIdsByName = new Map<string, string>();
  for (const resource of factoryDefinition.resources ?? []) {
    if (resource.id?.trim()) {
      resourceIdsByName.set(resource.name, resource.id);
    }
  }

  const workerIdsByName = new Map<string, string>();
  for (const worker of factoryDefinition.workers ?? []) {
    if (worker.id?.trim()) {
      workerIdsByName.set(worker.name, worker.id);
    }
  }

  const workTypeIdsByName = new Map<string, string>();
  const workStateIdsByWorkTypeName = new Map<
    string,
    ReadonlyMap<string, string>
  >();
  for (const workType of factoryDefinition.workTypes ?? []) {
    if (workType.id?.trim()) {
      workTypeIdsByName.set(workType.name, workType.id);
    }
    const stateIdsByName = new Map<string, string>();
    for (const state of workType.states) {
      if (state.id?.trim()) {
        stateIdsByName.set(state.name, state.id);
      }
    }
    workStateIdsByWorkTypeName.set(workType.name, stateIdsByName);
  }

  return {
    resourceIdsByName,
    workerIdsByName,
    workStateIdsByWorkTypeName,
    workTypeIdsByName,
  };
}
