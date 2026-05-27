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

export function buildFactoryGraphTopologyFromDefinition(
  factoryDefinition: CanonicalFactoryDefinition,
): FactoryGraphTopology {
  const nodes = new Map<string, FactoryGraphNode>();
  const edges = new Map<string, FactoryGraphEdge>();

  appendResourceNodes(factoryDefinition, nodes);
  appendWorkerNodes(factoryDefinition, nodes, edges);
  appendWorkTypeNodes(factoryDefinition, nodes, edges);
  appendWorkstationNodes(factoryDefinition, nodes, edges);

  return {
    edges: Array.from(edges.values()).sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    nodes: Array.from(nodes.values()).sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
  };
}

function appendResourceNodes(
  factoryDefinition: CanonicalFactoryDefinition,
  nodes: NodeMap,
) {
  for (const resource of factoryDefinition.resources ?? []) {
    const key: FactoryGraphNodeReference = {
      kind: "resource",
      name: resource.name,
    };
    nodes.set(nodeKeyId(key), buildNode(key));
  }
}

function appendWorkerNodes(
  factoryDefinition: CanonicalFactoryDefinition,
  nodes: NodeMap,
  edges: EdgeMap,
) {
  for (const worker of factoryDefinition.workers ?? []) {
    const workerKey: FactoryGraphNodeReference = {
      kind: "worker",
      name: worker.name,
    };
    nodes.set(nodeKeyId(workerKey), buildNode(workerKey));

    for (const resource of worker.resources ?? []) {
      const resourceKey: FactoryGraphNodeReference = {
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
  nodes: NodeMap,
  edges: EdgeMap,
) {
  for (const workType of factoryDefinition.workTypes ?? []) {
    const workTypeKey: FactoryGraphNodeReference = {
      kind: "work-type",
      name: workType.name,
    };
    nodes.set(nodeKeyId(workTypeKey), buildNode(workTypeKey));

    for (const state of workType.states) {
      const workStateKey: FactoryGraphWorkStateReference = {
        kind: "work-state",
        stateName: state.name,
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
  nodes: NodeMap,
  edges: EdgeMap,
) {
  for (const workstation of factoryDefinition.workstations ?? []) {
    const workstationKey: FactoryGraphNodeReference = {
      kind: "workstation",
      name: workstation.name,
    };
    nodes.set(nodeKeyId(workstationKey), buildNode(workstationKey));

    appendWorkstationWorkerEdge(workstation, workstationKey, nodes, edges);
    appendWorkstationResourceEdges(workstation, workstationKey, nodes, edges);
    appendWorkstationStateEdges(
      workstation.inputs,
      "workstation-input",
      workstationKey,
      nodes,
      edges,
    );
    appendWorkstationStateEdges(
      workstation.outputs,
      "workstation-output",
      workstationKey,
      nodes,
      edges,
    );
    appendWorkstationStateEdges(
      workstation.onContinue,
      "workstation-on-continue",
      workstationKey,
      nodes,
      edges,
    );
    appendWorkstationStateEdges(
      workstation.onFailure,
      "workstation-on-failure",
      workstationKey,
      nodes,
      edges,
    );
    appendWorkstationStateEdges(
      workstation.onRejection,
      "workstation-on-rejection",
      workstationKey,
      nodes,
      edges,
    );
  }
}

function appendWorkstationWorkerEdge(
  workstation: NonNullable<CanonicalFactoryDefinition["workstations"]>[number],
  workstationKey: FactoryGraphNodeReference,
  nodes: NodeMap,
  edges: EdgeMap,
) {
  const workerKey: FactoryGraphNodeReference = {
    kind: "worker",
    name: workstation.worker,
  };
  nodes.set(nodeKeyId(workerKey), buildNode(workerKey));
  if (workstation.worker.trim().length === 0) {
    return;
  }

  const workerEdge = buildEdge("worker-assignment", workerKey, workstationKey);
  edges.set(workerEdge.id, workerEdge);
}

function appendWorkstationResourceEdges(
  workstation: NonNullable<CanonicalFactoryDefinition["workstations"]>[number],
  workstationKey: FactoryGraphNodeReference,
  nodes: NodeMap,
  edges: EdgeMap,
) {
  for (const resource of workstation.resources ?? []) {
    const resourceKey: FactoryGraphNodeReference = {
      kind: "resource",
      name: resource.name,
    };
    nodes.set(nodeKeyId(resourceKey), buildNode(resourceKey));
    const edge = buildEdge("workstation-resource", resourceKey, workstationKey);
    edges.set(edge.id, edge);
  }
}

function appendWorkstationStateEdges(
  items: FactoryWorkstationIO[] | undefined,
  kind:
    | "workstation-input"
    | "workstation-on-continue"
    | "workstation-on-failure"
    | "workstation-on-rejection"
    | "workstation-output",
  workstationKey: FactoryGraphNodeReference,
  nodes: NodeMap,
  edges: EdgeMap,
) {
  for (const item of items ?? []) {
    const edge = buildWorkstationIOEdge(kind, item, workstationKey, nodes);
    edges.set(edge.id, edge);
  }
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
  nodes: NodeMap,
): FactoryGraphEdge {
  const workStateKey: FactoryGraphWorkStateReference = {
    kind: "work-state",
    stateName: io.state,
    workTypeName: io.workType,
  };
  nodes.set(nodeKeyId(workStateKey), buildNode(workStateKey));
  return kind === "workstation-input"
    ? buildEdge(kind, workStateKey, workstationKey)
    : buildEdge(kind, workstationKey, workStateKey);
}
