/** @type {Record<import("./index.d.ts").FactoryTopologyNodeKind, import("./index.d.ts").FactoryTopologyHandle[]>} */
const HANDLES_BY_KIND = {
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

const CONNECTION_HANDLES = {
  "worker-assignment": [
    "worker-assignment-source",
    "worker-assignment-target",
  ],
  "worker-resource": ["worker-resource-source", "worker-input-target"],
  "workstation-input": [
    "workstation-input-source",
    "workstation-input-target",
  ],
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

/** @param {string | undefined} explicitID @param {string} name */
function entityID(explicitID, name) {
  return explicitID?.trim() || name;
}

/** @param {import("./index.d.ts").FactoryTopologyNodeKind} kind @param {string} id */
function nodeID(kind, id) {
  return `${kind}:${id}`;
}

/**
 * @param {import("./index.d.ts").FactoryTopologyNodeKind} kind
 * @returns {import("./index.d.ts").FactoryTopologyHandle[]}
 */
function handlesFor(kind) {
  return HANDLES_BY_KIND[kind].map((handle) => ({ ...handle }));
}

/**
 * @typedef {NonNullable<import("@you-agent-factory/client").FactoryDefinition["workstations"]>[number]["inputs"][number]} WorkstationIO
 * @typedef {import("@you-agent-factory/client").FactoryDefinition} FactoryDefinition
 * @typedef {{
 *   connections: Map<string, import("./index.d.ts").FactoryTopologyConnection>,
 *   issues: Map<string, import("./index.d.ts").FactoryTopologyProjectionIssue>,
 *   nodes: Map<string, import("./index.d.ts").FactoryTopologyNode>,
 *   resourcesByName: Map<string, string>,
 *   workersByName: Map<string, string>,
 *   workStatesByName: Map<string, Map<string, string>>,
 *   workTypesByName: Map<string, string>,
 * }} TopologyContext
 */

/** @param {unknown} value @returns {WorkstationIO[]} */
function asRouteList(value) {
  if (!value) return [];
  return /** @type {WorkstationIO[]} */ (Array.isArray(value) ? value : [value]);
}

/** @returns {TopologyContext} */
function createTopologyContext() {
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

/** @param {TopologyContext} context @param {import("./index.d.ts").FactoryTopologyNode} node */
function addNode(context, node) {
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

/**
 * @param {TopologyContext} context
 * @param {import("./index.d.ts").FactoryTopologyConnectionKind} kind
 * @param {string | undefined} sourceId
 * @param {string | undefined} targetId
 * @param {string} sourceReference
 * @param {string} targetReference
 */
function addConnection(
  context,
  kind,
  sourceId,
  targetId,
  sourceReference,
  targetReference,
) {
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

/** @param {FactoryDefinition} factory @param {TopologyContext} context */
function appendResourceAndWorkerNodes(factory, context) {
  for (const resource of factory.resources ?? []) {
    const id = entityID(resource.id, resource.name);
    context.resourcesByName.set(resource.name, id);
    addNode(context, {
      capacity: resource.capacity,
      entityId: id,
      handles: handlesFor("resource"),
      id: nodeID("resource", id),
      kind: "resource",
      label: resource.name,
    });
  }
  for (const worker of factory.workers ?? []) {
    const id = entityID(worker.id, worker.name);
    context.workersByName.set(worker.name, id);
    addNode(context, {
      entityId: id,
      handles: handlesFor("worker"),
      id: nodeID("worker", id),
      kind: "worker",
      label: worker.name,
    });
  }
}

/** @param {FactoryDefinition} factory @param {TopologyContext} context */
function appendWorkTypeNodes(factory, context) {
  for (const workType of factory.workTypes ?? []) {
    const id = entityID(workType.id, workType.name);
    context.workTypesByName.set(workType.name, id);
    addNode(context, {
      entityId: id,
      handles: handlesFor("work-type"),
      id: nodeID("work-type", id),
      kind: "work-type",
      label: workType.name,
    });
    const states = new Map();
    for (const state of workType.states ?? []) {
      const idForState = `${id}:${entityID(state.id, state.name)}`;
      states.set(state.name, idForState);
      addNode(context, {
        category: state.type,
        entityId: idForState,
        handles: handlesFor("work-state"),
        id: nodeID("work-state", idForState),
        kind: "work-state",
        label: state.name,
        workTypeId: id,
      });
    }
    context.workStatesByName.set(workType.name, states);
  }
}

/** @param {FactoryDefinition} factory @param {TopologyContext} context */
function appendWorkstationNodes(factory, context) {
  for (const workstation of factory.workstations ?? []) {
    const id = entityID(workstation.id, workstation.name);
    addNode(context, {
      entityId: id,
      handles: handlesFor("workstation"),
      id: nodeID("workstation", id),
      kind: "workstation",
      label: workstation.name,
    });
  }
}

/** @param {FactoryDefinition} factory @param {TopologyContext} context */
function appendWorkerAndWorkTypeConnections(factory, context) {
  for (const worker of factory.workers ?? []) {
    const workerId = context.workersByName.get(worker.name);
    for (const resource of worker.resources ?? []) {
      const resourceId = context.resourcesByName.get(resource.name);
      addConnection(
        context,
        "worker-resource",
        resourceId && nodeID("resource", resourceId),
        workerId && nodeID("worker", workerId),
        resource.name,
        worker.name,
      );
    }
  }
  for (const workType of factory.workTypes ?? []) {
    const workTypeId = context.workTypesByName.get(workType.name);
    for (const state of workType.states ?? []) {
      const stateId = context.workStatesByName
        .get(workType.name)
        ?.get(state.name);
      addConnection(
        context,
        "work-type-state",
        workTypeId && nodeID("work-type", workTypeId),
        stateId && nodeID("work-state", stateId),
        workType.name,
        `${workType.name}:${state.name}`,
      );
    }
  }
}

/**
 * @param {TopologyContext} context
 * @param {NonNullable<FactoryDefinition["workstations"]>[number]} workstation
 * @param {string} workstationId
 */
function appendWorkstationRoutes(context, workstation, workstationId) {
  /** @type {Array<[import("./index.d.ts").FactoryTopologyConnectionKind, WorkstationIO[]]>} */
  const routes = [
    ["workstation-input", asRouteList(workstation.inputs)],
    ["workstation-output", asRouteList(workstation.outputs)],
    ["workstation-on-continue", asRouteList(workstation.onContinue)],
    ["workstation-on-failure", asRouteList(workstation.onFailure)],
    ["workstation-on-rejection", asRouteList(workstation.onRejection)],
    [
      "workstation-output",
      (workstation.classificationRoutes ?? []).flatMap(
        (route) => route.outputs ?? [],
      ),
    ],
  ];
  for (const [kind, ios] of routes) {
    for (const io of ios) {
      const stateId = context.workStatesByName
        .get(io.workType)
        ?.get(io.state);
      const workStateId = stateId && nodeID("work-state", stateId);
      const input = kind === "workstation-input";
      addConnection(
        context,
        kind,
        input ? workStateId : workstationId,
        input ? workstationId : workStateId,
        input ? `${io.workType}:${io.state}` : workstation.name,
        input ? workstation.name : `${io.workType}:${io.state}`,
      );
    }
  }
}

/** @param {FactoryDefinition} factory @param {TopologyContext} context */
function appendWorkstationConnections(factory, context) {
  for (const workstation of factory.workstations ?? []) {
    const workstationId = nodeID(
      "workstation",
      entityID(workstation.id, workstation.name),
    );
    const workerId = context.workersByName.get(workstation.worker);
    if (workstation.worker?.trim()) {
      addConnection(
        context,
        "worker-assignment",
        workerId && nodeID("worker", workerId),
        workstationId,
        workstation.worker,
        workstation.name,
      );
    }
    for (const resource of workstation.resources ?? []) {
      const resourceId = context.resourcesByName.get(resource.name);
      addConnection(
        context,
        "workstation-resource",
        resourceId && nodeID("resource", resourceId),
        workstationId,
        resource.name,
        workstation.name,
      );
    }
    appendWorkstationRoutes(context, workstation, workstationId);
  }
}

/**
 * Project one public Factory definition without mutating caller-owned data.
 *
 * @param {import("./index.d.ts").FactoryTopologyProjectionInput} input
 * @returns {import("./index.d.ts").FactoryTopologyProjection}
 */
export function projectFactoryTopology(input) {
  const factory = input.factory;
  if (!factory) {
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
  const context = createTopologyContext();
  appendResourceAndWorkerNodes(factory, context);
  appendWorkTypeNodes(factory, context);
  appendWorkstationNodes(factory, context);
  appendWorkerAndWorkTypeConnections(factory, context);
  appendWorkstationConnections(factory, context);

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
