import type { Meta, StoryObj } from "@storybook/react-vite";
import { Position, ReactFlowProvider } from "@xyflow/react";

import {
  GraphEdge,
  GraphNodeButton,
  type GraphNodeHandle,
  GraphNodeShell,
  GraphViewportSurface,
} from "./index";

const genericHandles: GraphNodeHandle[] = [
  {
    id: "input-target",
    buttonAriaLabel: "Input connection",
    label: "Input",
    side: "left",
    tone: "input",
    type: "target",
  },
  {
    id: "output-source",
    buttonAriaLabel: "Output connection",
    label: "Output",
    side: "right",
    tone: "output",
    type: "source",
  },
];

function GraphEdgeExample({
  edgeData,
  viewportClassName,
  viewportWidthClass,
}: {
  edgeData: {
    alwaysShowLabel?: boolean;
    label?: string;
    waypoints?: { x: number; y: number }[];
  };
  viewportClassName?: string;
  viewportWidthClass?: string;
}) {
  return (
    <ReactFlowProvider>
      <div className={viewportWidthClass ?? "w-full max-w-3xl"}>
        <GraphViewportSurface
          aria-label="Graph edge example"
          className={`h-64 ${viewportClassName ?? ""}`}
        >
          <GraphNodeShell handles={[genericHandles[0]]} nodeKind="source">
            <GraphNodeButton>Source node</GraphNodeButton>
          </GraphNodeShell>
          <GraphNodeShell
            className="absolute right-4 top-16 w-56"
            handles={[genericHandles[1]]}
            nodeKind="target"
          >
            <GraphNodeButton>Target node</GraphNodeButton>
          </GraphNodeShell>
          <svg
            aria-hidden="true"
            className="pointer-events-none absolute inset-0 h-full w-full"
          >
            <GraphEdge
              data={edgeData}
              id="story-edge"
              interactionWidth={20}
              selected={Boolean(edgeData.alwaysShowLabel)}
              source="source-node"
              sourcePosition={Position.Right}
              sourceX={120}
              sourceY={72}
              style={{ stroke: "var(--color-outline)", strokeWidth: 2 }}
              target="target-node"
              targetPosition={Position.Left}
              targetX={420}
              targetY={120}
            />
          </svg>
        </GraphViewportSurface>
      </div>
    </ReactFlowProvider>
  );
}

function GraphHandleExample({
  shellState = "default",
  viewportWidthClass,
}: {
  shellState?: "default" | "selected";
  viewportWidthClass?: string;
}) {
  return (
    <ReactFlowProvider>
      <div className={viewportWidthClass ?? "w-72"}>
        <GraphNodeShell
          handles={genericHandles}
          nodeKind="example"
          state={shellState}
          stateLabel={shellState === "selected" ? "Selected node" : undefined}
        >
          <GraphNodeButton graphState={shellState}>
            {shellState === "selected" ? "Selected node" : "Example node"}
          </GraphNodeButton>
        </GraphNodeShell>
      </div>
    </ReactFlowProvider>
  );
}

const meta = {
  title: "Graphs/GraphEdgesAndHandles",
  component: GraphEdgeExample,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof GraphEdgeExample>;

export default meta;

type Story = StoryObj<typeof meta>;

export const BezierEdge: Story = {
  args: {
    edgeData: {
      alwaysShowLabel: true,
      label: "Bezier edge",
    },
  },
};

export const WaypointEdge: Story = {
  args: {
    edgeData: {
      alwaysShowLabel: true,
      label: "Routed edge",
      waypoints: [
        { x: 260, y: 72 },
        { x: 260, y: 120 },
      ],
    },
  },
};

export const SourceTargetHandles: Story = {
  args: {
    edgeData: {},
  },
  render: () => <GraphHandleExample />,
};

export const SelectedNodeHandles: Story = {
  args: {
    edgeData: {},
  },
  render: () => <GraphHandleExample shellState="selected" />,
};

export const DesktopViewport: Story = {
  args: {
    edgeData: {
      alwaysShowLabel: true,
      label: "Desktop edge",
      waypoints: [
        { x: 260, y: 72 },
        { x: 260, y: 120 },
      ],
    },
    viewportClassName: "h-72",
    viewportWidthClass: "w-[48rem]",
  },
};

export const NarrowViewport: Story = {
  args: {
    edgeData: {
      alwaysShowLabel: true,
      label: "Narrow edge",
    },
    viewportClassName: "h-56",
    viewportWidthClass: "w-80 max-w-full",
  },
  render: (args) => (
    <div className="space-y-6">
      <GraphEdgeExample {...args} />
      <GraphHandleExample viewportWidthClass="w-80 max-w-full" />
    </div>
  ),
};
