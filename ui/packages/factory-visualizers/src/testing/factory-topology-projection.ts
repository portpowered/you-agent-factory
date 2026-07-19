import type { FactoryTopologyReplayProjection } from "../factory-topology-replay";

export function createFactoryTopologyProjection(): FactoryTopologyReplayProjection {
  return {
    activity: {
      activeDispatchOverlays: [
        {
          connectionIds: ["worker-assignment"],
          dispatchId: "dispatch-1",
          evidence: {
            resources: "known",
            route: "known",
            work: "known",
            worker: "known",
            workstation: "known",
          },
          id: "overlay:dispatch-1",
          resourceNodeIds: ["resource:gpu"],
          startedTick: 7,
          workIds: ["work-4", "work-2", "work-1", "work-3"],
          workerNodeId: "worker:alice",
          workstationNodeId: "workstation:review",
        },
      ],
      activeWorkstationNodeIds: ["workstation:review"],
      issues: [],
      resourceOccupancy: [],
      selectedTick: 8,
    },
    load: {
      issues: [],
      resourceOccupancy: [
        {
          availableQuantity: 2,
          capacity: 4,
          capacityEvidence: "known",
          evidence: "known",
          occupiedQuantity: 2,
          resourceId: "gpu",
          resourceNodeId: "resource:gpu",
        },
      ],
      selectedTick: 8,
      workStateCounts: [
        {
          count: 3,
          evidence: "known",
          workStateId: "queued",
          workStateNodeId: "work-state:task:queued",
          workTypeId: "task",
        },
      ],
    },
    topology: {
      connections: [
        {
          id: "worker-assignment",
          kind: "worker-assignment",
          source: {
            handleId: "worker-assignment-source",
            nodeId: "worker:alice",
          },
          target: {
            handleId: "worker-assignment-target",
            nodeId: "workstation:review",
          },
        },
      ],
      issues: [],
      nodes: [
        {
          capacity: 4,
          entityId: "gpu",
          handles: [],
          id: "resource:gpu",
          kind: "resource",
          label: "GPU",
        },
        {
          entityId: "alice",
          handles: [{ id: "worker-assignment-source", role: "source" }],
          id: "worker:alice",
          kind: "worker",
          label: "Alice",
        },
        {
          category: "INITIAL",
          entityId: "task:queued",
          handles: [],
          id: "work-state:task:queued",
          kind: "work-state",
          label: "Queued",
          workTypeId: "task",
        },
        {
          entityId: "review",
          handles: [{ id: "worker-assignment-target", role: "target" }],
          id: "workstation:review",
          kind: "workstation",
          label: "Review",
        },
      ],
      ok: true,
      selectedTick: 8,
    },
  };
}
