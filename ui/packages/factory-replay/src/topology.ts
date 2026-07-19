import type { FactoryDefinition } from "@you-agent-factory/client";

import { canonicalizeFactoryEvents } from "./replay.js";
import { projectFactoryTopologyConnection } from "./topology-connection.js";
import type {
  FactoryTopologyAtTickInput,
  FactoryTopologyConnection,
  FactoryTopologyConnectionKind,
  FactoryTopologyHandle,
  FactoryTopologyHandleId,
  FactoryTopologyNode,
  FactoryTopologyNodeKind,
  FactoryTopologyProjection,
  FactoryTopologyProjectionInput,
  FactoryTopologyProjectionIssue,
} from "./topology-contract.js";
import { FACTORY_TOPOLOGY_RELATIONSHIPS } from "./topology-contract.js";
import {
  factoryTopologyEntityId,
  factoryTopologyNodeId,
} from "./topology-identity.js";

export { projectFactoryTopologyConnection } from "./topology-connection.js";
export type {
  FactoryResourceTopologyNode,
  FactoryTopologyAtTickInput,
  FactoryTopologyConnection,
  FactoryTopologyConnectionCandidate,
  FactoryTopologyConnectionEndpoint,
  FactoryTopologyConnectionKind,
  FactoryTopologyConnectionResult,
  FactoryTopologyHandle,
  FactoryTopologyHandleId,
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
export { FACTORY_TOPOLOGY_RELATIONSHIPS } from "./topology-contract.js";
export {
  factoryTopologyEntityId,
  factoryTopologyNodeId,
} from "./topology-identity.js";

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

function handlesFor(kind: FactoryTopologyNodeKind): FactoryTopologyHandle[] {
  const handles = new Map<FactoryTopologyHandleId, FactoryTopologyHandle>();
  for (const relationship of Object.values(FACTORY_TOPOLOGY_RELATIONSHIPS)) {
    if (relationship.source.nodeKind === kind) {
      handles.set(relationship.source.handleId, {
        id: relationship.source.handleId,
        role: "source",
      });
    }
    if (relationship.target.nodeKind === kind) {
      handles.set(relationship.target.handleId, {
        id: relationship.target.handleId,
        role: "target",
      });
    }
  }
  return [...handles.values()].sort((left, right) =>
    left.id.localeCompare(right.id),
  );
}

function sortedByIdentity<T extends { id?: string; name: string }>(
  values: readonly T[] | undefined,
): T[] {
  return [...(values ?? [])].sort((left, right) => {
    const identityDifference = factoryTopologyEntityId(
      left.id,
      left.name,
    ).localeCompare(factoryTopologyEntityId(right.id, right.name));
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
  sourceNodeId: string | undefined,
  targetNodeId: string | undefined,
  sourceReference: string,
  targetReference: string,
): void {
  const result = projectFactoryTopologyConnection([...context.nodes.values()], {
    kind,
    sourceNodeId,
    sourceReference,
    targetNodeId,
    targetReference,
  });
  if (!result.ok) {
    context.issues.set(result.issue.id, result.issue);
    return;
  }
  context.connections.set(result.connection.id, result.connection);
}

function appendNodes(
  factory: FactoryDefinition,
  context: TopologyContext,
): void {
  for (const resource of sortedByIdentity(factory.resources)) {
    const id = factoryTopologyEntityId(resource.id, resource.name);
    context.resourcesByName.set(resource.name, id);
    addNode(context, {
      capacity: resource.capacity,
      entityId: id,
      handles: handlesFor("resource"),
      id: factoryTopologyNodeId("resource", id),
      kind: "resource",
      label: resource.name,
    });
  }
  for (const worker of sortedByIdentity(factory.workers)) {
    const id = factoryTopologyEntityId(worker.id, worker.name);
    context.workersByName.set(worker.name, id);
    addNode(context, {
      entityId: id,
      handles: handlesFor("worker"),
      id: factoryTopologyNodeId("worker", id),
      kind: "worker",
      label: worker.name,
    });
  }
  for (const workType of sortedByIdentity(factory.workTypes)) {
    const id = factoryTopologyEntityId(workType.id, workType.name);
    context.workTypesByName.set(workType.name, id);
    addNode(context, {
      entityId: id,
      handles: handlesFor("work-type"),
      id: factoryTopologyNodeId("work-type", id),
      kind: "work-type",
      label: workType.name,
    });
    const states = new Map<string, string>();
    for (const state of sortedByIdentity(workType.states)) {
      const stateId = `${id}:${factoryTopologyEntityId(state.id, state.name)}`;
      states.set(state.name, stateId);
      addNode(context, {
        category: state.type,
        entityId: stateId,
        handles: handlesFor("work-state"),
        id: factoryTopologyNodeId("work-state", stateId),
        kind: "work-state",
        label: state.name,
        workTypeId: id,
      });
    }
    context.workStatesByName.set(workType.name, states);
  }
  for (const workstation of sortedByIdentity(factory.workstations)) {
    const id = factoryTopologyEntityId(workstation.id, workstation.name);
    addNode(context, {
      entityId: id,
      handles: handlesFor("workstation"),
      id: factoryTopologyNodeId("workstation", id),
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
        resourceEntityId && factoryTopologyNodeId("resource", resourceEntityId),
        workerEntityId && factoryTopologyNodeId("worker", workerEntityId),
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
        workTypeEntityId &&
          factoryTopologyNodeId("work-type", workTypeEntityId),
        stateEntityId && factoryTopologyNodeId("work-state", stateEntityId),
        workType.name,
        `${workType.name}:${state.name}`,
      );
    }
  }
}

function sortedRoutes(
  routes: readonly WorkstationIO[] | WorkstationIO | null | undefined,
): WorkstationIO[] {
  const normalizedRoutes = !routes
    ? []
    : Array.isArray(routes)
      ? routes
      : [routes as WorkstationIO];
  return [...normalizedRoutes].sort((left, right) =>
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
      const stateNodeId =
        stateEntityId && factoryTopologyNodeId("work-state", stateEntityId);
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
    const workstationNodeId = factoryTopologyNodeId(
      "workstation",
      factoryTopologyEntityId(workstation.id, workstation.name),
    );
    const workerEntityId = context.workersByName.get(workstation.worker);
    if (workstation.worker.trim()) {
      addConnection(
        context,
        "worker-assignment",
        workerEntityId && factoryTopologyNodeId("worker", workerEntityId),
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
        resourceEntityId && factoryTopologyNodeId("resource", resourceEntityId),
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
      ok: false,
      selectedTick: input.selectedTick,
    };
  }
  const context = createContext();
  appendNodes(input.factory, context);
  appendFoundationConnections(input.factory, context);
  appendWorkstationConnections(input.factory, context);
  const issues = [...context.issues.values()].sort((left, right) =>
    left.id.localeCompare(right.id),
  );
  if (issues.length > 0) {
    return {
      connections: [],
      issues,
      nodes: [],
      ok: false,
      selectedTick: input.selectedTick,
    };
  }
  return {
    connections: [...context.connections.values()].sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    issues,
    nodes: [...context.nodes.values()].sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    ok: true,
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
