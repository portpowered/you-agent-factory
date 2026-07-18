import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { describe, expect, it } from "vitest";

import {
  projectFactoryTopology,
  projectFactoryTopologyAtTick,
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
      onFailure: [{ state: "failed", workType: "story" }],
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
        "workstation-on-failure",
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
    ).toEqual({ connections: [], issues: [], nodes: [], selectedTick: 0 });
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
      selectedTick: 5,
    });
  });
});
