import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FactoryVisualizationLayoutV1 } from "@you-agent-factory/client";
import { useState } from "react";
import { expect, fn, userEvent, within } from "storybook/test";

import {
  FactoryTopologyReplay,
  type FactoryTopologyReplayMessages,
  type FactoryTopologyReplayProjection,
  type FactoryTopologyReplayProps,
} from "./factory-topology-replay";
import {
  allNodesEmptyLayout,
  embeddedPixel,
  responsiveLayout,
} from "./testing/factory-topology-responsive-layouts";

const messages: FactoryTopologyReplayMessages = {
  activeDispatches: (count) =>
    `${count} active ${count === 1 ? "Dispatch" : "Dispatches"}`,
  annotationsHidden: "Show annotations",
  annotationsVisible: "Hide annotations",
  empty: "No Factory topology is available at this tick.",
  failed: "The Factory topology could not be shown.",
  inactiveDispatches: "No active Dispatch",
  imageFailed: "The annotation image could not be shown.",
  imageLoading: "Loading annotation image.",
  legendActiveRoute: "Active route",
  legendInactiveRoute: "Inactive route",
  legendLabel: "Topology legend",
  loading: "Loading Factory topology.",
  nodeLabel: (kind, label) => `${kind}: ${label}`,
  regionLabel: "Factory topology at selected tick",
  resourceOccupancy: (occupied, capacity) =>
    `${occupied} of ${capacity} capacity occupied`,
  resourceOccupancyUnavailable: "Occupancy unavailable",
  retry: "Try again",
  selectedNode: "Selected by host",
  viewportControlsLabel: "Topology viewport controls",
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

export const DensePreparedProjection: Story = {
  args: {
    selectedNodeId: "workstation:review",
    state: { projection: createProjection(true), status: "ready" },
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
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

export const AnnotationVisibility: Story = {
  args: {
    layout: annotationLayout(),
    state: { projection: createProjection(), status: "ready" },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("img", { name: "Review process diagram" }),
    ).toBeVisible();
    const toggle = canvas.getByRole("button", {
      name: messages.annotationsVisible,
    });
    await userEvent.click(toggle);
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    await expect(
      canvas.queryByRole("img", { name: "Review process diagram" }),
    ).not.toBeInTheDocument();
  },
};

export const FullChrome: Story = {
  args: {
    chrome: { preset: "full" },
    layout: annotationLayout(),
  },
};

export const MinimalChrome: Story = {
  args: {
    chrome: { preset: "minimal" },
    layout: annotationLayout(),
  },
};

export const NoChrome: Story = {
  args: {
    chrome: { preset: "none" },
    layout: annotationLayout(),
  },
};

/** Emulator-ready boundary: caller-prepared projection plus sidecar only. */
export const EmulatorReadyDenseAnnotations: Story = {
  args: {
    layout: annotationLayout(),
    state: { projection: createProjection(true), status: "ready" },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("img", { name: "Review process diagram" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "workstation: Review" }),
    ).toBeVisible();
  },
};

export const InactiveNodeEmptyState: Story = {
  args: {
    layout: {
      nodeEmptyStates: [
        {
          content: { kind: "text", text: "No review work is waiting." },
          nodeId: "workstation:review",
        },
      ],
      schemaVersion: "factory-visualization-layout/v1",
    },
    state: { projection: createInactiveProjection(), status: "ready" },
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("No review work is waiting."),
    ).toBeVisible();
  },
};

export const EmulatorReadyInactiveEmptyState: Story = {
  ...InactiveNodeEmptyState,
};

export const ResponsiveAnnotationsAndEmptyState: Story = {
  args: {
    layout: responsiveLayout(),
    state: { projection: createInactiveProjection(), status: "ready" },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText("Long review guidance wraps inside the topology."),
    ).toBeVisible();
    await expect(
      canvas.getByRole("img", { name: "Review flow overview" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "workstation: Review" }),
    ).toHaveTextContent("No review work is waiting.");
    await expect(
      canvas.getByRole("button", { name: "worker: Alice" }),
    ).toHaveTextContent("Worker availability illustration");
  },
};

export const RuntimeTelemetryPrecedence: Story = {
  args: {
    layout: allNodesEmptyLayout(createTelemetryProjection()),
    state: { projection: createTelemetryProjection(), status: "ready" },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByText(/Configured empty content/)).toBeNull();
    await expect(canvas.getByText("2 of 4 capacity occupied")).toBeVisible();
    await expect(canvas.getByText("7 Work in this state")).toBeVisible();
  },
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
          workIds: ["work-4", "work-2", "work-1", "work-3"],
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

function createInactiveProjection(): FactoryTopologyReplayProjection {
  const projection = createProjection();
  projection.activity = {
    ...projection.activity,
    activeDispatchOverlays: [],
    activeWorkstationNodeIds: [],
  };
  projection.load = {
    ...projection.load,
    workStateCounts: projection.load.workStateCounts.map((count) => ({
      ...count,
      count: 0,
    })),
  };
  return projection;
}

function createTelemetryProjection(): FactoryTopologyReplayProjection {
  const projection = createProjection();
  projection.activity.activeDispatchOverlays[0].resourceNodeIds = [];
  return projection;
}

function annotationLayout(): FactoryVisualizationLayoutV1 {
  return {
    annotations: [
      {
        body: "Read the validation notes before routing work.",
        id: "review-note",
        kind: "note",
        position: { x: 80, y: 40 },
        tone: "info",
      },
      {
        altText: "Review process diagram",
        id: "review-diagram",
        kind: "image",
        position: { x: 900, y: 10 },
        size: { height: 80, width: 120 },
        source: embeddedPixel(),
      },
    ],
    schemaVersion: "factory-visualization-layout/v1",
  };
}
