import { describe, expect, it } from "vitest";

import {
  type FactoryWorkProgressProjection,
  projectFactoryActivityAtTick,
  projectFactoryLoadAtTick,
  projectFactoryTopologyAtTick,
  projectFactoryWorkProgressAtTick,
} from "../../../../../../packages/factory-replay/src/index.js";
import type { FactoryEvent } from "../../../../../api/events";
import { buildFactoryTimelineSnapshot } from "../buildSnapshot";

const factory = {
  name: "publishing",
  resources: [{ capacity: 2, id: "gpu-stable", name: "gpu" }],
  workers: [
    {
      id: "writer-stable",
      modelProvider: "CODEX" as const,
      name: "writer",
      resources: [{ capacity: 1, name: "gpu" }],
      type: "MODEL_WORKER" as const,
    },
  ],
  workTypes: [
    {
      id: "story-stable",
      name: "story",
      states: [
        { id: "ready-stable", name: "ready", type: "INITIAL" as const },
        { id: "done-stable", name: "done", type: "TERMINAL" as const },
      ],
    },
  ],
  workstations: [
    {
      behavior: "STANDARD" as const,
      id: "review-stable",
      inputs: [{ state: "ready", workType: "story" }],
      name: "review",
      outputs: [{ state: "done", workType: "story" }],
      resources: [{ capacity: 1, name: "gpu" }],
      worker: "writer",
    },
    {
      behavior: "STANDARD" as const,
      id: "publish-stable",
      inputs: [{ state: "done", workType: "story" }],
      name: "publish",
      outputs: [],
      worker: "writer",
    },
  ],
};

function event(
  id: string,
  type: FactoryEvent["type"],
  tick: number,
  sequence: number,
  payload: FactoryEvent["payload"],
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-07-18T10:00:0${sequence}Z`,
      sequence,
      tick,
      ...context,
    },
    id,
    payload,
    type,
  };
}

function hostedReplayEvents(): FactoryEvent[] {
  return [
    event("topology", "INITIAL_STRUCTURE_REQUEST", 1, 0, { factory }),
    event("work", "WORK_REQUEST", 2, 0, {
      type: "FACTORY_REQUEST_BATCH",
      works: [
        { name: "Draft one", workId: "work-1", workTypeName: "story" },
        { name: "Draft two", workId: "work-2", workTypeName: "story" },
      ],
    }),
    event(
      "start",
      "DISPATCH_REQUEST",
      3,
      1,
      {
        inputs: [
          {
            name: "Draft",
            state: { name: "ready", type: "INITIAL" },
            workId: "work-1",
            workTypeName: "story",
          },
          {
            name: "Draft two",
            state: { name: "ready", type: "INITIAL" },
            workId: "work-2",
            workTypeName: "story",
          },
        ],
        resources: [{ capacity: 2, name: "gpu" }],
        transitionId: "review",
      },
      { dispatchId: "dispatch-1", workIds: ["work-1", "work-2"] },
    ),
    event(
      "finish",
      "DISPATCH_RESPONSE",
      3,
      2,
      {
        durationMillis: 1,
        outcome: "ACCEPTED",
        outputResources: [{ capacity: 2, name: "gpu" }],
        outputWork: [
          {
            name: "Draft",
            state: { name: "done", type: "TERMINAL" },
            workId: "work-1",
            workTypeName: "story",
          },
          {
            name: "Draft two",
            state: { name: "done", type: "TERMINAL" },
            workId: "work-2",
            workTypeName: "story",
          },
        ],
        transitionId: "review",
      },
      { dispatchId: "dispatch-1", workIds: ["work-1", "work-2"] },
    ),
  ];
}

function progressPartition(projection: FactoryWorkProgressProjection) {
  return {
    active: projection.active.map((work) => work.id),
    completed: projection.completed.map((work) => work.id),
    counts: projection.counts,
    failed: projection.failed.map((work) => work.id),
    queued: projection.queued.map((work) => work.id),
    total: projection.total,
    unclassified: projection.unclassified.map((work) => work.id),
  };
}

describe("hosted shared Factory replay projections", () => {
  it("uses the shared topology, activity, occupancy, and Work progress meanings", () => {
    const events = hostedReplayEvents().slice(0, 3);
    const snapshot = buildFactoryTimelineSnapshot(events, 3);
    const hosted = snapshot.factoryReplay;

    expect(hosted.topology).toEqual(
      projectFactoryTopologyAtTick({ events, tick: 3 }),
    );
    expect(hosted.activity).toEqual(
      projectFactoryActivityAtTick({ events, tick: 3 }),
    );
    expect(hosted.load).toEqual(projectFactoryLoadAtTick({ events, tick: 3 }));
    expect(progressPartition(hosted.workProgress)).toEqual(
      progressPartition(projectFactoryWorkProgressAtTick({ events, tick: 3 })),
    );
    expect(hosted.activity.resourceOccupancy).toEqual([
      expect.objectContaining({
        availableQuantity: 1,
        occupiedQuantity: 1,
        resourceId: "gpu-stable",
      }),
    ]);
    expect(hosted.workProgress.active.map((work) => work.id)).toEqual([
      "work-1",
      "work-2",
    ]);
    expect(snapshot.topology.workstation_node_ids).toEqual([
      "publish-stable",
      "review-stable",
    ]);
    expect(snapshot.topology.edges).toEqual([
      expect.objectContaining({
        from_node_id: "review-stable",
        to_node_id: "publish-stable",
      }),
    ]);
    expect(snapshot.runtime.active_dispatch_ids).toEqual(["dispatch-1"]);
    expect(snapshot.runtime.active_workstation_node_ids).toEqual([
      "review-stable",
    ]);
    expect(
      snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-1"]
        ?.workstation_node_id,
    ).toBe("review-stable");
    expect(snapshot.runtime.place_token_counts?.["gpu:available"]).toBe(1);

    const handlesByNode = new Map(
      hosted.topology.nodes.map((node) => [
        node.id,
        new Set(node.handles.map((handle) => handle.id)),
      ]),
    );
    for (const connection of hosted.topology.connections) {
      expect(handlesByNode.get(connection.source.nodeId)).toContain(
        connection.source.handleId,
      );
      expect(handlesByNode.get(connection.target.nodeId)).toContain(
        connection.target.handleId,
      );
    }
  });

  it("recomputes all shared projections when a later event has the same tick", () => {
    const events = hostedReplayEvents();
    const active = buildFactoryTimelineSnapshot(events.slice(0, 3), 3);
    const completed = buildFactoryTimelineSnapshot(events, 3);

    expect(active.factoryReplay.activity.activeDispatchOverlays).toHaveLength(
      1,
    );
    expect(active.factoryReplay.workProgress.counts.active).toBe(2);
    expect(completed.factoryReplay.activity).toEqual(
      projectFactoryActivityAtTick({ events, tick: 3 }),
    );
    expect(progressPartition(completed.factoryReplay.workProgress)).toEqual(
      progressPartition(projectFactoryWorkProgressAtTick({ events, tick: 3 })),
    );
    expect(completed.factoryReplay.activity.activeDispatchOverlays).toEqual([]);
    expect(completed.factoryReplay.activity.resourceOccupancy).toEqual([
      expect.objectContaining({
        availableQuantity: 2,
        occupiedQuantity: 0,
      }),
    ]);
    expect(
      completed.factoryReplay.workProgress.completed.map((work) => work.id),
    ).toEqual(["work-1", "work-2"]);
    expect(completed.factoryReplay.workProgress.total).toBe(2);
    expect(completed.runtime.active_dispatch_ids).toEqual([]);
    expect(completed.runtime.active_workstation_node_ids).toEqual([]);
    expect(completed.runtime.place_token_counts?.["gpu:available"]).toBe(2);
    expect(completed.runtime.session.completed_count).toBe(2);
  });
});
