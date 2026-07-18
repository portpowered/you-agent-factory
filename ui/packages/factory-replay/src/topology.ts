import type { FactoryDefinition } from "@you-agent-factory/client";

import { canonicalizeFactoryEvents } from "./replay.js";
import type {
  FactoryTopologyAtTickInput,
  FactoryTopologyConnection,
  FactoryTopologyConnectionKind,
  FactoryTopologyHandle,
  FactoryTopologyNode,
  FactoryTopologyNodeKind,
  FactoryTopologyProjection,
  FactoryTopologyProjectionInput,
  FactoryTopologyProjectionIssue,
} from "./topology-contract.js";

export type {
  FactoryResourceTopologyNode,
  FactoryTopologyAtTickInput,
  FactoryTopologyConnection,
  FactoryTopologyConnectionEndpoint,
  FactoryTopologyConnectionKind,
  FactoryTopologyHandle,
  FactoryTopologyNode,
  FactoryTopologyNodeKind,
  FactoryTopologyProjection,
  FactoryTopologyProjectionInput,
  FactoryTopologyProjectionIssue,
  FactoryWorkerTopologyNode,
  FactoryWorkStateTopologyNode,
  FactoryWorkstationTopologyNode,
  FactoryWorkTypeTopologyNode,
} from "./topology-contract.js";

const HANDLES_BY_KIND: Record<
  FactoryTopologyNodeKind,
  readonly FactoryTopologyHandle[]
> = {
  resource: [
    { id: "worker-resource-source", role: "source" },
    { id: "workstation-resource-source", role: "source" },
  ],
  worker: [
    { id: "worker-input-target", role: "target" },
    { id: "worker-assignment-source", role: "source" },
  ],
  "work-state": [
    { id: "workstation-input-source", role: "source" },
    { id: "work-state-input-target", role: "target" },
    { id: "work-type-state-target", role: "target" },
  ],
  "work-type": [{ id: "work-type-state-source", role: "source" }],
  workstation: [
    { id: "workstation-input-target", role: "target" },
    { id: "worker-assignment-target", role: "target" },
    { id: "workstation-resource-target", role: "target" },
    { id: "workstation-output-source", role: "source" },
    { id: "workstation-on-continue-source", role: "source" },
    { id: "workstation-on-failure-source", role: "source" },
    { id: "workstation-on-rejection-source", role: "source" },
  ],
};

const CONNECTION_HANDLES: Record<
  FactoryTopologyConnectionKind,
  readonly [string, string]
> = {
  "worker-assignment": ["worker-assignment-source", "worker-assignment-target"],
  "worker-resource": ["worker-resource-source", "worker-input-target"],
  "workstation-input": ["workstation-input-source", "workstation-input-target"],
  "workstation-on-continue": [
    "workstation-on-continue-source",
    "work-state-input-target",
  ],
  "workstation-on-failure": [
    "workstation-on-failure-source",
    "work-state-input-target",
  ],
  "workstation-on-rejection": [
    "workstation-on-rejection-source",
    "work-state-input-target",
  ],
  "workstation-output": [
    "workstation-output-source",
    "work-state-input-target",
  ],
  "workstation-resource": [
    "workstation-resource-source",
    "workstation-resource-target",
  ],
  "work-type-state": ["work-type-state-source", "work-type-state-target"],
};

type Workstation = NonNullable<FactoryDefinition["workstations"]>[number];
type WorkstationIO = Workstation["inputs"][number];

interface TopologyContext {
  connections: Map<string, FactoryTopologyConnection>;
  issues: Map<string, FactoryTopologyProjectionIssue>;
  nodes: Map<string, FactoryTopologyNode>;
  resourcesByName: Map<string, string>;
  workersByName: Map<string, string>;
  workStatesByName: Map<string, Map<string, string>>;
  workTypesByName: Map<string, string>;
}

function entityId(explicitId: string | undefined, name: string): string {
  return explicitId?.trim() || name;
}

function nodeId(kind: FactoryTopologyNodeKind, id: string): string {
  return `${kind}:${id}`;
}

function handlesFor(kind: FactoryTopologyNodeKind): FactoryTopologyHandle[] {
  return HANDLES_BY_KIND[kind].map((handle) => ({ ...handle }));
}

function sortedByIdentity<T extends { id?: string; name: string }>(
  values: readonly T[] | undefined,
): T[] {
  return [...(values ?? [])].sort((left, right) => {
    const identityDifference = entityId(left.id, left.name).localeCompare(
      entityId(right.id, right.name),
    );
    return identityDifference || left.name.localeCompare(right.name);
  });
}

function createContext(): TopologyContext {
  return {
    connections: new Map(),
    issues: new Map(),
    nodes: new Map(),
    resourcesByName: new Map(),
    workersByName: new Map(),
    workStatesByName: new Map(),
    workTypesByName: new Map(),
  };
}

function addNode(context: TopologyContext, node: FactoryTopologyNode): void {
  if (context.nodes.has(node.id)) {
    const id = `duplicate-entity:${node.id}`;
    context.issues.set(id, {
      code: "DUPLICATE_ENTITY_ID",
      id,
      message: `Factory topology contains duplicate node id ${node.id}.`,
      nodeId: node.id,
    });
    return;
  }
  context.nodes.set(node.id, node);
}

function addConnection(
  context: TopologyContext,
  kind: FactoryTopologyConnectionKind,
  sourceId: string | undefined,
  targetId: string | undefined,
  sourceReference: string,
  targetReference: string,
): void {
  if (
    !sourceId ||
    !targetId ||
    !context.nodes.has(sourceId) ||
    !context.nodes.has(targetId)
  ) {
    const id = `unresolved-connection:${kind}:${sourceReference}->${targetReference}`;
    context.issues.set(id, {
      code: "UNRESOLVED_CONNECTION",
      connectionKind: kind,
      id,
      message: `Cannot resolve ${kind} connection from ${sourceReference} to ${targetReference}.`,
      sourceReference,
      targetReference,
    });
    return;
  }
  const [sourceHandle, targetHandle] = CONNECTION_HANDLES[kind];
  const id = `${kind}:${sourceId}->${targetId}`;
  context.connections.set(id, {
    id,
    kind,
    source: { handleId: sourceHandle, nodeId: sourceId },
    target: { handleId: targetHandle, nodeId: targetId },
  });
}

function appendNodes(
  factory: FactoryDefinition,
  context: TopologyContext,
): void {
  for (const resource of sortedByIdentity(factory.resources)) {
    const id = entityId(resource.id, resource.name);
    context.resourcesByName.set(resource.name, id);
    addNode(context, {
      capacity: resource.capacity,
      entityId: id,
      handles: handlesFor("resource"),
      id: nodeId("resource", id),
      kind: "resource",
      label: resource.name,
    });
  }
  for (const worker of sortedByIdentity(factory.workers)) {
    const id = entityId(worker.id, worker.name);
    context.workersByName.set(worker.name, id);
    addNode(context, {
      entityId: id,
      handles: handlesFor("worker"),
      id: nodeId("worker", id),
      kind: "worker",
      label: worker.name,
    });
  }
  for (const workType of sortedByIdentity(factory.workTypes)) {
    const id = entityId(workType.id, workType.name);
    context.workTypesByName.set(workType.name, id);
    addNode(context, {
      entityId: id,
      handles: handlesFor("work-type"),
      id: nodeId("work-type", id),
      kind: "work-type",
      label: workType.name,
    });
    const states = new Map<string, string>();
    for (const state of sortedByIdentity(workType.states)) {
      const stateId = `${id}:${entityId(state.id, state.name)}`;
      states.set(state.name, stateId);
      addNode(context, {
        category: state.type,
        entityId: stateId,
        handles: handlesFor("work-state"),
        id: nodeId("work-state", stateId),
        kind: "work-state",
        label: state.name,
        workTypeId: id,
      });
    }
    context.workStatesByName.set(workType.name, states);
  }
  for (const workstation of sortedByIdentity(factory.workstations)) {
    const id = entityId(workstation.id, workstation.name);
    addNode(context, {
      entityId: id,
      handles: handlesFor("workstation"),
      id: nodeId("workstation", id),
      kind: "workstation",
      label: workstation.name,
    });
  }
}

function appendFoundationConnections(
  factory: FactoryDefinition,
  context: TopologyContext,
): void {
  for (const worker of sortedByIdentity(factory.workers)) {
    const workerEntityId = context.workersByName.get(worker.name);
    for (const resource of sortedByIdentity(worker.resources)) {
      const resourceEntityId = context.resourcesByName.get(resource.name);
      addConnection(
        context,
        "worker-resource",
        resourceEntityId && nodeId("resource", resourceEntityId),
        workerEntityId && nodeId("worker", workerEntityId),
        resource.name,
        worker.name,
      );
    }
  }
  for (const workType of sortedByIdentity(factory.workTypes)) {
    const workTypeEntityId = context.workTypesByName.get(workType.name);
    for (const state of sortedByIdentity(workType.states)) {
      const stateEntityId = context.workStatesByName
        .get(workType.name)
        ?.get(state.name);
      addConnection(
        context,
        "work-type-state",
        workTypeEntityId && nodeId("work-type", workTypeEntityId),
        stateEntityId && nodeId("work-state", stateEntityId),
        workType.name,
        `${workType.name}:${state.name}`,
      );
    }
  }
}

function sortedRoutes(
  routes: readonly WorkstationIO[] | undefined,
): WorkstationIO[] {
  return [...(routes ?? [])].sort((left, right) =>
    `${left.workType}:${left.state}`.localeCompare(
      `${right.workType}:${right.state}`,
    ),
  );
}

function appendRoutes(
  context: TopologyContext,
  workstation: Workstation,
  workstationNodeId: string,
): void {
  const routes: Array<
    readonly [
      FactoryTopologyConnectionKind,
      readonly WorkstationIO[] | undefined,
    ]
  > = [
    ["workstation-input", workstation.inputs],
    ["workstation-output", workstation.outputs],
    ["workstation-on-continue", workstation.onContinue],
    ["workstation-on-failure", workstation.onFailure],
    ["workstation-on-rejection", workstation.onRejection],
    [
      "workstation-output",
      workstation.classificationRoutes?.flatMap((route) => route.outputs),
    ],
  ];
  for (const [kind, routeList] of routes) {
    for (const route of sortedRoutes(routeList)) {
      const stateEntityId = context.workStatesByName
        .get(route.workType)
        ?.get(route.state);
      if (
        !stateEntityId &&
        route.state === "available" &&
        context.resourcesByName.has(route.workType)
      ) {
        continue;
      }
      const stateNodeId = stateEntityId && nodeId("work-state", stateEntityId);
      const isInput = kind === "workstation-input";
      addConnection(
        context,
        kind,
        isInput ? stateNodeId : workstationNodeId,
        isInput ? workstationNodeId : stateNodeId,
        isInput ? `${route.workType}:${route.state}` : workstation.name,
        isInput ? workstation.name : `${route.workType}:${route.state}`,
      );
    }
  }
}

function appendWorkstationConnections(
  factory: FactoryDefinition,
  context: TopologyContext,
): void {
  for (const workstation of sortedByIdentity(factory.workstations)) {
    const workstationNodeId = nodeId(
      "workstation",
      entityId(workstation.id, workstation.name),
    );
    const workerEntityId = context.workersByName.get(workstation.worker);
    if (workstation.worker.trim()) {
      addConnection(
        context,
        "worker-assignment",
        workerEntityId && nodeId("worker", workerEntityId),
        workstationNodeId,
        workstation.worker,
        workstation.name,
      );
    }
    for (const resource of sortedByIdentity(workstation.resources)) {
      const resourceEntityId = context.resourcesByName.get(resource.name);
      addConnection(
        context,
        "workstation-resource",
        resourceEntityId && nodeId("resource", resourceEntityId),
        workstationNodeId,
        resource.name,
        workstation.name,
      );
    }
    appendRoutes(context, workstation, workstationNodeId);
  }
}

/** Project one public Factory definition without mutating caller-owned data. */
export function projectFactoryTopology(
  input: FactoryTopologyProjectionInput,
): FactoryTopologyProjection {
  if (!input.factory) {
    return {
      connections: [],
      issues: [
        {
          code: "MISSING_FACTORY",
          id: "missing-factory",
          message: "No Factory topology is available at the selected tick.",
        },
      ],
      nodes: [],
      selectedTick: input.selectedTick,
    };
  }
  const context = createContext();
  appendNodes(input.factory, context);
  appendFoundationConnections(input.factory, context);
  appendWorkstationConnections(input.factory, context);
  return {
    connections: [...context.connections.values()].sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    issues: [...context.issues.values()].sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    nodes: [...context.nodes.values()].sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    selectedTick: input.selectedTick,
  };
}

/** Reconstruct the last canonical Factory topology effective at one tick. */
export function projectFactoryTopologyAtTick(
  input: FactoryTopologyAtTickInput,
): FactoryTopologyProjection {
  let factory: FactoryDefinition | undefined;
  for (const event of canonicalizeFactoryEvents(input.events)) {
    if (event.context.tick > input.tick) break;
    if (
      event.type !== "INITIAL_STRUCTURE_REQUEST" &&
      event.type !== "FACTORY_CHANGE"
    ) {
      continue;
    }
    const payload = event.payload as { factory?: unknown };
    if (payload.factory && typeof payload.factory === "object") {
      factory = payload.factory as FactoryDefinition;
    }
  }
  return projectFactoryTopology({ factory, selectedTick: input.tick });
}
