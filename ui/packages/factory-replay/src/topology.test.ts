import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { describe, expect, it } from "vitest";

import {
  FACTORY_TOPOLOGY_RELATIONSHIPS,
  projectFactoryTopology,
  projectFactoryTopologyAtTick,
  projectFactoryTopologyConnection,
} from "./index.js";

const factory: FactoryDefinition = {
  name: "publishing",
  resources: [
    { capacity: 2, id: "gpu-stable", name: "gpu" },
    { capacity: 3, name: "network" },
  ],
  workers: [
    {
      id: "writer-stable",
      name: "writer",
      resources: [{ capacity: 1, name: "gpu" }],
    },
  ],
  workTypes: [
    {
      id: "story-stable",
      name: "story",
      states: [
        { id: "ready-stable", name: "ready", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
        { name: "failed", type: "FAILED" },
      ],
    },
  ],
  workstations: [
    {
      id: "review-stable",
      inputs: [{ state: "ready", workType: "story" }],
      name: "review",
      onContinue: [{ state: "ready", workType: "story" }],
      onFailure: [{ state: "failed", workType: "story" }],
      onRejection: [{ state: "ready", workType: "story" }],
      outputs: [{ state: "done", workType: "story" }],
      resources: [{ capacity: 1, name: "network" }],
      worker: "writer",
    },
  ],
};

function topologyEvent(
  id: string,
  type: "INITIAL_STRUCTURE_REQUEST" | "FACTORY_CHANGE",
  tick: number,
  sequence: number,
  topology: FactoryDefinition,
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-07-18T12:00:0${sequence}Z`,
      sequence,
      tick,
    },
    id,
    payload: { factory: topology },
    schemaVersion: "agent-factory.event.v1",
    type,
  };
}

function reversedFactory(source: FactoryDefinition): FactoryDefinition {
  return {
    ...structuredClone(source),
    resources: structuredClone(source.resources)?.reverse(),
    workers: structuredClone(source.workers)
      ?.reverse()
      .map((worker) => ({
        ...worker,
        resources: worker.resources?.reverse(),
      })),
    workTypes: structuredClone(source.workTypes)
      ?.reverse()
      .map((workType) => ({
        ...workType,
        states: workType.states.reverse(),
      })),
    workstations: structuredClone(source.workstations)
      ?.reverse()
      .map((workstation) => ({
        ...workstation,
        inputs: workstation.inputs.reverse(),
        outputs: workstation.outputs?.reverse(),
        resources: workstation.resources?.reverse(),
      })),
  };
}

describe("projectFactoryTopology", () => {
  it("projects every entity kind and canonical connection deterministically", () => {
    const original = structuredClone(factory);
    const first = projectFactoryTopology({ factory, selectedTick: 4 });
    const shuffled = projectFactoryTopology({
      factory: reversedFactory(factory),
      selectedTick: 4,
    });

    expect(shuffled).toEqual(first);
    expect(factory).toEqual(original);
    expect(first.nodes.map((node) => node.id)).toEqual([
      "resource:gpu-stable",
      "resource:network",
      "work-state:story-stable:done",
      "work-state:story-stable:failed",
      "work-state:story-stable:ready-stable",
      "work-type:story-stable",
      "worker:writer-stable",
      "workstation:review-stable",
    ]);
    expect(
      new Set(first.connections.map((connection) => connection.kind)),
    ).toEqual(
      new Set([
        "worker-assignment",
        "worker-resource",
        "workstation-input",
        "workstation-on-continue",
        "workstation-on-failure",
        "workstation-on-rejection",
        "workstation-output",
        "workstation-resource",
        "work-type-state",
      ]),
    );
    expect(first.issues).toEqual([]);
  });

  it("uses names as stable identities when optional public IDs are absent", () => {
    const result = projectFactoryTopology({
      factory: {
        name: "minimal",
        resources: [{ capacity: 1, name: "slot" }],
        workers: [{ name: "runner" }],
        workTypes: [
          {
            name: "task",
            states: [{ name: "queued", type: "INITIAL" }],
          },
        ],
        workstations: [
          {
            inputs: [{ state: "queued", workType: "task" }],
            name: "process",
            worker: "runner",
          },
        ],
      },
      selectedTick: 2,
    });

    expect(result.nodes.map(({ entityId, id }) => ({ entityId, id }))).toEqual([
      { entityId: "slot", id: "resource:slot" },
      { entityId: "task:queued", id: "work-state:task:queued" },
      { entityId: "task", id: "work-type:task" },
      { entityId: "runner", id: "worker:runner" },
      { entityId: "process", id: "workstation:process" },
    ]);
    expect(result.issues).toEqual([]);
  });

  it("returns a valid empty projection for an empty Factory", () => {
    expect(
      projectFactoryTopology({ factory: { name: "empty" }, selectedTick: 0 }),
    ).toEqual({
      connections: [],
      issues: [],
      nodes: [],
      ok: true,
      selectedTick: 0,
    });
  });
});

describe("Factory topology semantic relationships", () => {
  it("derives every edge endpoint from the handles declared by its nodes", () => {
    const result = projectFactoryTopology({ factory, selectedTick: 4 });
    const nodes = new Map(result.nodes.map((node) => [node.id, node]));

    expect(result.ok).toBe(true);
    expect(new Set(result.connections.map(({ kind }) => kind))).toEqual(
      new Set(Object.keys(FACTORY_TOPOLOGY_RELATIONSHIPS)),
    );
    expect(new Set(result.connections.map(({ id }) => id))).toHaveLength(
      result.connections.length,
    );
    for (const connection of result.connections) {
      const relationship = FACTORY_TOPOLOGY_RELATIONSHIPS[connection.kind];
      const source = nodes.get(connection.source.nodeId);
      const target = nodes.get(connection.target.nodeId);
      expect(source?.kind).toBe(relationship.source.nodeKind);
      expect(target?.kind).toBe(relationship.target.nodeKind);
      expect(connection.source.handleId).toBe(relationship.source.handleId);
      expect(connection.target.handleId).toBe(relationship.target.handleId);
      expect(source?.handles).toContainEqual({
        id: connection.source.handleId,
        role: "source",
      });
      expect(target?.handles).toContainEqual({
        id: connection.target.handleId,
        role: "target",
      });
    }
  });

  it("keeps semantic connection and handle identities stable across ticks", () => {
    const earlier = projectFactoryTopology({ factory, selectedTick: 2 });
    const later = projectFactoryTopology({ factory, selectedTick: 9 });

    expect(later.nodes).toEqual(earlier.nodes);
    expect(later.connections).toEqual(earlier.connections);
  });

  it("fails closed when a canonical relationship references an unknown node", () => {
    const result = projectFactoryTopology({
      factory: {
        name: "invalid",
        workers: [
          {
            name: "runner",
            resources: [{ capacity: 1, name: "missing" }],
          },
        ],
      },
      selectedTick: 3,
    });

    expect(result.ok).toBe(false);
    expect(result.nodes).toEqual([]);
    expect(result.connections).toEqual([]);
    expect(result.issues).toMatchObject([
      {
        code: "INVALID_CONNECTION_ENDPOINT",
        connectionKind: "worker-resource",
        endpoint: "source",
        endpointReason: "MISSING_NODE",
        sourceReference: "missing",
        targetReference: "runner",
      },
    ]);
  });
});

describe("projectFactoryTopologyConnection", () => {
  const projection = projectFactoryTopology({ factory, selectedTick: 4 });
  const workType = projection.nodes.find((node) => node.kind === "work-type");
  const workState = projection.nodes.find((node) => node.kind === "work-state");

  if (!workType || !workState) {
    throw new Error("expected topology connection fixtures");
  }

  const candidate = {
    kind: "work-type-state",
    sourceNodeId: workType.id,
    sourceReference: workType.label,
    targetNodeId: workState.id,
    targetReference: workState.label,
  };

  it("reports an unsupported semantic relationship", () => {
    const result = projectFactoryTopologyConnection(projection.nodes, {
      ...candidate,
      kind: "generic-output",
    });

    expect(result).toMatchObject({
      issue: {
        code: "UNSUPPORTED_CONNECTION_KIND",
        connectionKind: "generic-output",
      },
      ok: false,
    });
  });

  it("reports a missing declared endpoint handle", () => {
    const nodesWithoutSourceHandle = projection.nodes.map((node) =>
      node.id === workType.id ? { ...node, handles: [] } : node,
    );
    const result = projectFactoryTopologyConnection(
      nodesWithoutSourceHandle,
      candidate,
    );

    expect(result).toMatchObject({
      issue: {
        code: "INVALID_CONNECTION_ENDPOINT",
        endpoint: "source",
        endpointReason: "MISSING_HANDLE",
        handleId: "work-type-state-source",
        nodeId: workType.id,
      },
      ok: false,
    });
  });

  it("reports invalid relationship direction instead of binding generic handles", () => {
    const result = projectFactoryTopologyConnection(projection.nodes, {
      ...candidate,
      sourceNodeId: workState.id,
      targetNodeId: workType.id,
    });

    expect(result).toMatchObject({
      issue: {
        code: "INVALID_CONNECTION_ENDPOINT",
        endpoint: "source",
        endpointReason: "NODE_KIND_MISMATCH",
        expectedNodeKind: "work-type",
        nodeId: workState.id,
      },
      ok: false,
    });
  });

  it("reports a missing target node with the connection identity", () => {
    const result = projectFactoryTopologyConnection(projection.nodes, {
      ...candidate,
      targetNodeId: "work-state:missing",
    });

    expect(result).toMatchObject({
      issue: {
        code: "INVALID_CONNECTION_ENDPOINT",
        connectionId: `work-type-state:${workType.id}->work-state:missing`,
        endpoint: "target",
        endpointReason: "MISSING_NODE",
      },
      ok: false,
    });
  });
});

describe("projectFactoryTopologyAtTick", () => {
  it("reconstructs topology additions and replacements at the selected tick", () => {
    const initial: FactoryDefinition = { name: "initial" };
    const added = structuredClone(factory);
    const replacement = structuredClone(factory);
    const review = replacement.workstations?.[0];
    if (!review) throw new Error("expected review workstation fixture");
    replacement.workstations = [
      {
        ...review,
        outputs: [{ state: "failed", workType: "story" }],
      },
    ];
    const events = [
      topologyEvent("replace", "FACTORY_CHANGE", 3, 2, replacement),
      topologyEvent("initial", "INITIAL_STRUCTURE_REQUEST", 1, 0, initial),
      topologyEvent("add", "FACTORY_CHANGE", 2, 1, added),
    ];

    expect(projectFactoryTopologyAtTick({ events, tick: 1 }).nodes).toEqual([]);
    expect(
      projectFactoryTopologyAtTick({ events, tick: 2 }).connections.some(
        (connection) =>
          connection.id ===
          "workstation-output:workstation:review-stable->work-state:story-stable:done",
      ),
    ).toBe(true);
    const latest = projectFactoryTopologyAtTick({ events, tick: 3 });
    expect(
      latest.connections.some(
        (connection) =>
          connection.id ===
          "workstation-output:workstation:review-stable->work-state:story-stable:failed",
      ),
    ).toBe(true);
    expect(latest.selectedTick).toBe(3);
  });

  it("uses canonical same-tick order and reports absent topology explicitly", () => {
    const events = [
      topologyEvent("later", "FACTORY_CHANGE", 5, 2, factory),
      topologyEvent("earlier", "FACTORY_CHANGE", 5, 1, { name: "empty" }),
    ];

    expect(
      projectFactoryTopologyAtTick({ events, tick: 5 }).nodes,
    ).toHaveLength(8);
    expect(projectFactoryTopologyAtTick({ events: [], tick: 5 })).toEqual({
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
      selectedTick: 5,
    });
  });
});
