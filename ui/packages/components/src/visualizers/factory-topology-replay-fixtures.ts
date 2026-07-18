import type {
  FactoryTopologyReplayMessages,
  FactoryTopologyReplayProjection,
} from "./factory-topology-replay-types";

export const factoryTopologyReplayMessages: FactoryTopologyReplayMessages = {
  activeDispatchCount: (count) =>
    `${count} active Dispatch${count === 1 ? "" : "es"}`,
  connectionLabel: (kind) => kind.replaceAll("-", " "),
  emptyDescription: "No prepared Factory topology is available.",
  emptyTitle: "No topology available",
  failedDescription: "The prepared topology could not be displayed.",
  failedTitle: "Unable to display topology",
  handleLabel: (handleId, role) => `${role} ${handleId.replaceAll("-", " ")}`,
  inactiveDispatch: "No active Dispatches",
  loadingDescription: "Preparing the selected Factory topology.",
  loadingTitle: "Loading topology",
  nodeKind: (kind) => kind.replaceAll("-", " "),
  occupancy: (occupied, capacity) => `${occupied} of ${capacity} occupied`,
  occupancyUnavailable: "Occupancy unavailable",
  regionLabel: "Factory topology",
  retryLabel: "Try again",
  selectedNode: "Selected",
  selectedTick: (tick) => `Logical tick ${tick}`,
  workStateCount: (count) => `${count} Work`,
};

export const factoryTopologyReplayProjection: FactoryTopologyReplayProjection =
  {
    activity: {
      activeDispatches: [
        {
          id: "dispatch-review",
          resourceIds: ["gpu"],
          startedTick: 41,
          transitionId: "review",
          workIds: ["work-1"],
          workerId: "reviewer",
          workstationId: "review",
          workstationNodeId: "workstation:review",
        },
      ],
      activeWorkstationIds: ["review"],
      issues: [],
      resourceOccupancy: [
        {
          availableQuantity: 1,
          capacity: 2,
          evidence: "known",
          occupiedQuantity: 1,
          resourceId: "gpu",
          resourceNodeId: "resource:gpu",
        },
      ],
      selectedTick: 42,
    },
    topology: {
      connections: [
        {
          id: "work-type:task->work-state:queued",
          kind: "work-type-state",
          source: {
            handleId: "work-type-state-source",
            nodeId: "work-type:task",
          },
          target: {
            handleId: "work-type-state-target",
            nodeId: "work-state:queued",
          },
        },
        {
          id: "work-state:queued->workstation:review",
          kind: "workstation-input",
          source: {
            handleId: "workstation-input-source",
            nodeId: "work-state:queued",
          },
          target: {
            handleId: "workstation-input-target",
            nodeId: "workstation:review",
          },
        },
        {
          id: "worker:reviewer->workstation:review",
          kind: "worker-assignment",
          source: {
            handleId: "worker-assignment-source",
            nodeId: "worker:reviewer",
          },
          target: {
            handleId: "worker-assignment-target",
            nodeId: "workstation:review",
          },
        },
        {
          id: "resource:gpu->workstation:review",
          kind: "workstation-resource",
          source: {
            handleId: "workstation-resource-source",
            nodeId: "resource:gpu",
          },
          target: {
            handleId: "workstation-resource-target",
            nodeId: "workstation:review",
          },
        },
      ],
      issues: [],
      nodes: [
        {
          entityId: "task",
          handles: [{ id: "work-type-state-source", role: "source" }],
          id: "work-type:task",
          kind: "work-type",
          label: "Task",
        },
        {
          category: "INITIAL",
          entityId: "queued",
          handles: [
            { id: "work-type-state-target", role: "target" },
            { id: "workstation-input-source", role: "source" },
          ],
          id: "work-state:queued",
          kind: "work-state",
          label: "Queued",
          workTypeId: "task",
        },
        {
          entityId: "review",
          handles: [
            { id: "workstation-input-target", role: "target" },
            { id: "worker-assignment-target", role: "target" },
            { id: "workstation-resource-target", role: "target" },
          ],
          id: "workstation:review",
          kind: "workstation",
          label: "Review",
        },
        {
          entityId: "reviewer",
          handles: [{ id: "worker-assignment-source", role: "source" }],
          id: "worker:reviewer",
          kind: "worker",
          label: "Reviewer",
        },
        {
          capacity: 2,
          entityId: "gpu",
          handles: [{ id: "workstation-resource-source", role: "source" }],
          id: "resource:gpu",
          kind: "resource",
          label: "GPU",
        },
      ],
      selectedTick: 42,
    },
    workStateCounts: [{ count: 1234, nodeId: "work-state:queued" }],
  };
