import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";

import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
  type FactoryTopologyReplayProjection,
} from "./factory-topology-replay";

const messages: FactoryTopologyReplayMessages = {
  activeDispatches: (count) =>
    `${count} active ${count === 1 ? "Dispatch" : "Dispatches"}`,
  inactiveDispatches: "No active Dispatch",
  nodeLabel: (kind, label) => `${kind}: ${label}`,
  regionLabel: "Factory topology at selected tick",
  resourceOccupancy: (occupied, capacity) =>
    `${occupied} of ${capacity} capacity occupied`,
  resourceOccupancyUnavailable: "Occupancy unavailable",
  selectedNode: "Selected by host",
  workStateCount: (count) => `${count} Work in this state`,
  workStateCountUnavailable: "Work count unavailable",
};

const meta = {
  title: "Factory Visualizers/FactoryTopologyReplay",
  component: FactoryTopologyReplay,
  args: {
    messages,
    onSelectNode: fn(),
    projection: createProjection(),
  },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof FactoryTopologyReplay>;

export default meta;
type Story = StoryObj<typeof meta>;

export const SparsePreparedProjection: Story = {
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const workstation = canvas.getByRole("button", {
      name: "workstation: Review",
    });
    await userEvent.click(workstation);
    await expect(args.onSelectNode).toHaveBeenCalled();
    await expect(workstation).not.toHaveAttribute("aria-pressed", "true");
  },
};

export const DensePreparedProjection: Story = {
  args: {
    projection: createProjection(true),
    selectedNodeId: "workstation:review",
  },
};

function createProjection(dense = false): FactoryTopologyReplayProjection {
  const extraWorkers = dense
    ? Array.from({ length: 9 }, (_, index) => ({
        entityId: `worker-${index + 2}`,
        handles: [],
        id: `worker:worker-${index + 2}`,
        kind: "worker" as const,
        label: `Worker ${index + 2}`,
      }))
    : [];
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
          startedTick: 17,
          workerNodeId: "worker:alice",
          workstationNodeId: "workstation:review",
        },
      ],
      activeWorkstationNodeIds: ["workstation:review"],
      issues: [],
      resourceOccupancy: [],
      selectedTick: 18,
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
      selectedTick: 18,
      workStateCounts: [
        {
          count: 7,
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
          label: "GPU pool",
        },
        {
          entityId: "alice",
          handles: [{ id: "worker-assignment-source", role: "source" }],
          id: "worker:alice",
          kind: "worker",
          label: "Alice",
        },
        ...extraWorkers,
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
      selectedTick: 18,
    },
  };
}
