import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent, within } from "storybook/test";

import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
  type FactoryTopologyReplayProjection,
  type FactoryTopologyReplayProps,
} from "./factory-topology-replay";

const messages: FactoryTopologyReplayMessages = {
  activeDispatches: (count) =>
    `${count} active ${count === 1 ? "Dispatch" : "Dispatches"}`,
  activeWorkDuration: (ticks) => `${ticks} logical ticks`,
  activeWorkOverflow: (count) => `+${count} active Work`,
  activeWorkRows: (count) => `${count} active Work rows`,
  empty: "No Factory topology is available at this tick.",
  failed: "The Factory topology could not be shown.",
  hideNodeKinds: "Hide node kinds",
  inactiveDispatches: "No active Dispatch",
  legendLabel: "Topology legend",
  loading: "Loading Factory topology.",
  nodeLabel: (kind, label) => `${kind}: ${label}`,
  regionLabel: "Factory topology at selected tick",
  resourceOccupancy: (occupied, capacity) =>
    `${occupied} of ${capacity} capacity occupied`,
  resourceOccupancyUnavailable: "Occupancy unavailable",
  retry: "Try again",
  selectedNode: "Selected by host",
  showNodeKinds: "Show node kinds",
  workStateCount: (count) => `${count} Work in this state`,
  workStateCountUnavailable: "Work count unavailable",
};

const meta = {
  title: "Factory Visualizers/FactoryTopologyReplay",
  component: FactoryTopologyReplay,
  args: {
    messages,
    onSelectNode: fn(),
    state: { projection: createProjection(), status: "ready" },
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

export const FullChrome: Story = {
  args: { chrome: { preset: "full" } },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByLabelText(messages.legendLabel)).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: messages.hideNodeKinds }),
    ).toBeVisible();
  },
};

export const MinimalChrome: Story = {
  args: { chrome: { preset: "minimal" } },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByLabelText(messages.legendLabel)).toBeNull();
    await expect(
      canvas.queryByRole("button", { name: messages.hideNodeKinds }),
    ).toBeNull();
  },
};

export const NoChromeWithVisibilityOverride: Story = {
  args: {
    chrome: { preset: "none", visibilityControls: true },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const visibilityControl = canvas.getByRole("button", {
      name: messages.hideNodeKinds,
    });
    await userEvent.click(visibilityControl);
    await expect(visibilityControl).toHaveAttribute("aria-pressed", "false");
    await expect(
      canvas.getByRole("button", { name: messages.showNodeKinds }),
    ).toHaveFocus();
    await expect(canvas.queryByLabelText(messages.legendLabel)).toBeNull();
  },
};

export const DensePreparedProjection: Story = {
  args: {
    selectedNodeId: "workstation:review",
    state: { projection: createProjection(true), status: "ready" },
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("group", { name: "5 active Work rows" }),
    ).toBeVisible();
    await expect(canvas.getByText("+2 active Work")).toBeVisible();
    const workstation = canvas.getByRole("button", {
      name: "workstation: Review",
    });

    for (
      let index = 0;
      index < 50 && workstation !== document.activeElement;
      index++
    ) {
      await userEvent.tab();
    }
    await expect(workstation).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onSelectNode).toHaveBeenCalledWith(
      expect.objectContaining({ id: "workstation:review" }),
    );
  },
};

export const TouchPanePanning: Story = {
  args: {
    state: { projection: createProjection(true), status: "ready" },
  },
  render: (args) => <SelectionHost {...args} />,
};

export const Loading: Story = {
  args: { state: { status: "loading" } },
};

export const Empty: Story = {
  args: { state: { status: "empty" } },
};

export const FailedWithRetry: Story = {
  args: { onRetry: fn(), state: { status: "failed" } },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: messages.retry }));
    await expect(args.onRetry).toHaveBeenCalled();
  },
};

export const Recovered: Story = {
  args: { state: { projection: createProjection(), status: "ready" } },
};

export const FailureRecovery: Story = {
  args: { onRetry: fn(), state: { status: "failed" } },
  render: (args) => <RecoveryHost {...args} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: messages.retry }));
    await expect(
      canvas.getByRole("region", { name: messages.regionLabel }),
    ).toHaveAttribute("data-endpoints-valid", "true");
  },
};

function RecoveryHost(props: FactoryTopologyReplayProps) {
  const [state, setState] = useState(props.state);
  return (
    <FactoryTopologyReplay
      {...props}
      onRetry={() => {
        props.onRetry?.();
        setState({ projection: createProjection(), status: "ready" });
      }}
      state={state}
    />
  );
}

function SelectionHost(props: FactoryTopologyReplayProps) {
  const [selectedNodeId, setSelectedNodeId] = useState<string>();
  return (
    <FactoryTopologyReplay
      {...props}
      onSelectNode={(node) => {
        props.onSelectNode?.(node);
        setSelectedNodeId(node.id);
      }}
      selectedNodeId={selectedNodeId}
    />
  );
}

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
          ...(dense
            ? {
                workIds: ["work-1", "work-2", "work-3", "work-4", "work-5"],
              }
            : {}),
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
