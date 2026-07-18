import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { describe, expect, it } from "vitest";

import {
  factoryDispatchOverlayId,
  factoryWorkProjectionId,
  projectFactoryActivity,
  projectFactoryActivityAtTick,
} from "./index.js";

const factory: FactoryDefinition = {
  name: "publishing",
  resources: [{ capacity: 2, id: "gpu-stable", name: "gpu" }],
  workers: [{ id: "writer-stable", name: "writer" }],
  workTypes: [
    {
      id: "story-stable",
      name: "story",
      states: [
        { id: "ready-stable", name: "ready", type: "INITIAL" },
        { id: "done-stable", name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workstations: [
    {
      id: "review-stable",
      inputs: [{ state: "ready", workType: "story" }],
      name: "review",
      outputs: [{ state: "done", workType: "story" }],
      resources: [{ capacity: 1, name: "gpu" }],
      worker: "writer",
    },
  ],
};

function event(
  id: string,
  type: FactoryEvent["type"],
  tick: number,
  sequence: number,
  payload: unknown,
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-07-18T23:00:${String(sequence).padStart(2, "0")}Z`,
      sequence,
      tick,
      ...context,
    },
    id,
    payload,
    schemaVersion: "agent-factory.event.v1",
    type,
  } as FactoryEvent;
}

function topologyEvent(
  selectedFactory: FactoryDefinition = factory,
  tick = 1,
  sequence = 0,
): FactoryEvent {
  return event(
    `topology-${tick}-${sequence}`,
    tick === 1 ? "INITIAL_STRUCTURE_REQUEST" : "FACTORY_CHANGE",
    tick,
    sequence,
    { factory: selectedFactory },
  );
}

function workRequest(workId: string, tick = 2, sequence = 0): FactoryEvent {
  return event(`work-${workId}`, "WORK_REQUEST", tick, sequence, {
    type: "WORK_REQUEST",
    works: [{ name: workId, workId, workTypeName: "story" }],
  });
}

function dispatch(
  id: string,
  type: "DISPATCH_REQUEST" | "DISPATCH_RESPONSE",
  tick: number,
  sequence: number,
  dispatchId: string,
  workIds: string[] = [],
  resources: unknown = [{ capacity: 2, name: "gpu" }],
): FactoryEvent {
  return event(
    id,
    type,
    tick,
    sequence,
    type === "DISPATCH_REQUEST"
      ? {
          inputs: workIds.map((workId) => ({ workId })),
          resources,
          transitionId: "review",
        }
      : { outcome: "CONTINUE", transitionId: "review" },
    { dispatchId, workIds },
  );
}

describe("projectFactoryActivityAtTick", () => {
  it("maps active Dispatch activity to stable topology and Work identities", () => {
    const events = [
      dispatch("start", "DISPATCH_REQUEST", 3, 0, "dispatch-1", ["work-1"]),
      workRequest("work-1"),
      topologyEvent(),
    ];
    const original = structuredClone(events);
    const result = projectFactoryActivityAtTick({ events, tick: 3 });

    expect(events).toEqual(original);
    expect(result.activeDispatchOverlays).toEqual([
      {
        connectionIds: [
          "worker-assignment:worker:writer-stable->workstation:review-stable",
          "workstation-input:work-state:story-stable:ready-stable->workstation:review-stable",
          "workstation-resource:resource:gpu-stable->workstation:review-stable",
        ],
        dispatchId: "dispatch-1",
        evidence: {
          resources: "known",
          route: "known",
          work: "known",
          worker: "known",
          workstation: "known",
        },
        id: "dispatch:dispatch-1",
        resourceIds: ["gpu-stable"],
        resourceNodeIds: ["resource:gpu-stable"],
        startedTick: 3,
        transitionId: "review",
        workerId: "writer-stable",
        workerNodeId: "worker:writer-stable",
        workIds: ["work-1"],
        workProjectionIds: ["work:work-1"],
        workstationId: "review-stable",
        workstationNodeId: "workstation:review-stable",
      },
    ]);
    expect(result.activeWorkstationNodeIds).toEqual([
      "workstation:review-stable",
    ]);
    expect(result.resourceOccupancy[0]).toMatchObject({
      availableQuantity: 1,
      evidence: "known",
      occupiedQuantity: 1,
    });
    expect(result.issues).toEqual([]);
  });

  it("applies same-tick completion and restart in canonical sequence order", () => {
    const base = [topologyEvent(), workRequest("work-1")];
    const startThenFinish = projectFactoryActivityAtTick({
      events: [
        ...base,
        dispatch("start", "DISPATCH_REQUEST", 3, 0, "dispatch-1", ["work-1"]),
        dispatch("finish", "DISPATCH_RESPONSE", 3, 1, "dispatch-1"),
      ],
      tick: 3,
    });
    const finishThenStart = projectFactoryActivityAtTick({
      events: [
        ...base,
        dispatch("finish", "DISPATCH_RESPONSE", 3, 0, "dispatch-1"),
        dispatch("restart", "DISPATCH_REQUEST", 3, 1, "dispatch-1", ["work-1"]),
      ],
      tick: 3,
    });

    expect(startThenFinish.activeDispatchOverlays).toEqual([]);
    expect(startThenFinish.resourceOccupancy[0].occupiedQuantity).toBe(0);
    expect(finishThenStart.activeDispatchOverlays).toHaveLength(1);
    expect(finishThenStart.resourceOccupancy[0].occupiedQuantity).toBe(1);
  });
});

describe("projectFactoryActivityAtTick lifecycle", () => {
  it("removes completed, interrupted, and terminally reconciled Dispatches", () => {
    const start = dispatch("start", "DISPATCH_REQUEST", 2, 0, "dispatch-1", [
      "work-1",
    ]);
    const terminalEvents: FactoryEvent[] = [
      dispatch("finish", "DISPATCH_RESPONSE", 3, 0, "dispatch-1"),
      event(
        "interrupt",
        "DISPATCH_INTERRUPTED",
        3,
        0,
        {
          interruptedAt: "2026-07-18T23:00:00Z",
          observedStatus: "RUNNING",
          reason: "cancelled",
          retryPlanned: false,
        },
        { dispatchId: "dispatch-1" },
      ),
      event(
        "reconciled",
        "DISPATCH_RECONCILED",
        3,
        0,
        { reconciledStatus: "FAILED", reconciliationSource: "RECOVERY" },
        { dispatchId: "dispatch-1" },
      ),
    ];

    for (const terminal of terminalEvents) {
      const result = projectFactoryActivityAtTick({
        events: [topologyEvent(), start, terminal],
        tick: 3,
      });
      expect(result.activeDispatchOverlays).toEqual([]);
      expect(result.resourceOccupancy[0].occupiedQuantity).toBe(0);
    }
  });

  it("keeps simultaneous Dispatches independent and deterministic", () => {
    const events = [
      topologyEvent(),
      dispatch("start-b", "DISPATCH_REQUEST", 2, 2, "dispatch-b", ["work-b"]),
      dispatch("start-a", "DISPATCH_REQUEST", 2, 1, "dispatch-a", ["work-a"]),
    ];
    const first = projectFactoryActivityAtTick({ events, tick: 2 });
    const shuffled = projectFactoryActivityAtTick({
      events: structuredClone(events).reverse(),
      tick: 2,
    });

    expect(shuffled).toEqual(first);
    expect(
      first.activeDispatchOverlays.map(({ dispatchId }) => dispatchId),
    ).toEqual(["dispatch-a", "dispatch-b"]);
    expect(first.resourceOccupancy[0].occupiedQuantity).toBe(2);
  });

  it("uses the topology replacement effective at the selected tick", () => {
    const replacement = structuredClone(factory);
    const workstation = replacement.workstations?.[0];
    const worker = replacement.workers?.[0];
    if (!workstation || !worker) throw new Error("expected activity fixture");
    workstation.id = "review-v2";
    worker.id = "writer-v2";
    const events = [
      topologyEvent(),
      workRequest("work-1"),
      dispatch("start", "DISPATCH_REQUEST", 3, 0, "dispatch-1", ["work-1"]),
      topologyEvent(replacement, 4, 0),
    ];

    expect(
      projectFactoryActivityAtTick({ events, tick: 3 })
        .activeDispatchOverlays[0],
    ).toMatchObject({
      workerNodeId: "worker:writer-stable",
      workstationNodeId: "workstation:review-stable",
    });
    expect(
      projectFactoryActivityAtTick({ events, tick: 4 })
        .activeDispatchOverlays[0],
    ).toMatchObject({
      workerNodeId: "worker:writer-v2",
      workstationNodeId: "workstation:review-v2",
    });
  });

  it("preserves a Dispatch when optional event evidence is absent", () => {
    const result = projectFactoryActivityAtTick({
      events: [
        topologyEvent(),
        event(
          "incomplete-start",
          "DISPATCH_REQUEST",
          2,
          0,
          { transitionId: "review" },
          { dispatchId: "dispatch-incomplete" },
        ),
      ],
      tick: 2,
    });

    expect(result.activeDispatchOverlays[0]).toMatchObject({
      dispatchId: "dispatch-incomplete",
      evidence: {
        resources: "unavailable",
        route: "unavailable",
        work: "unavailable",
        worker: "known",
        workstation: "known",
      },
      workstationNodeId: "workstation:review-stable",
    });
    expect(result.activeDispatchOverlays[0]?.workIds).toBeUndefined();
    expect(result.activeDispatchOverlays[0]?.resourceIds).toBeUndefined();
  });
});

describe("projectFactoryActivity incomplete evidence", () => {
  it("retains identifiable overlays without inventing optional references", () => {
    const result = projectFactoryActivity({
      activeDispatches: [
        {
          id: "dispatch-partial",
          startedTick: 4,
          transitionId: "missing-transition",
        },
      ],
      factory,
      selectedTick: 4,
    });

    expect(result.activeDispatchOverlays).toEqual([
      {
        connectionIds: [],
        dispatchId: "dispatch-partial",
        evidence: {
          resources: "unavailable",
          route: "unavailable",
          work: "unavailable",
          worker: "unavailable",
          workstation: "unavailable",
        },
        id: "dispatch:dispatch-partial",
        startedTick: 4,
        transitionId: "missing-transition",
      },
    ]);
    expect(result.resourceOccupancy[0].evidence).toBe("unavailable");
    expect(result.issues.map((issue) => issue.code)).toEqual([
      "UNRESOLVED_WORKSTATION",
    ]);
  });

  it("reports unresolved worker, resource, and route evidence deterministically", () => {
    const incompleteFactory = structuredClone(factory);
    const workstation = incompleteFactory.workstations?.[0];
    if (!workstation) throw new Error("expected activity fixture");
    workstation.worker = "missing-worker";
    const result = projectFactoryActivity({
      activeDispatches: [
        {
          id: "dispatch-invalid",
          inputRoutes: [{ stateName: "missing-state", workTypeId: "story" }],
          resourceNames: ["missing-resource"],
          startedTick: 5,
          transitionId: "review",
          workIds: ["work-1"],
        },
      ],
      factory: incompleteFactory,
      selectedTick: 5,
    });

    expect(result.activeDispatchOverlays[0]).toMatchObject({
      connectionIds: [],
      resourceIds: [],
      resourceNodeIds: [],
      workIds: ["work-1"],
      workstationNodeId: "workstation:review-stable",
    });
    expect(result.issues.map((issue) => issue.code)).toEqual([
      "UNRESOLVED_RESOURCE",
      "UNRESOLVED_ROUTE",
      "UNAVAILABLE_TOPOLOGY_PATH",
      "UNRESOLVED_WORKER",
    ]);
  });

  it("exports stable Dispatch and Work reference helpers", () => {
    expect(factoryDispatchOverlayId("dispatch-1")).toBe("dispatch:dispatch-1");
    expect(factoryWorkProjectionId("work-1")).toBe("work:work-1");
  });
});
