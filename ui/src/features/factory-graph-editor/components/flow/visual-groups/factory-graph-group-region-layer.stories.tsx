import type { Meta, StoryObj } from "@storybook/react-vite";
import { Background, ReactFlow, ReactFlowProvider } from "@xyflow/react";

import "../../../../../styles.css";
import {
  type FactoryGraphGroupRegionInput,
  FactoryGraphGroupRegionLayer,
} from "@you-agent-factory/factory-graph";

const meta = {
  title: "Factory Graph/Group Regions",
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const supportedColorGroups: FactoryGraphGroupRegionInput[] = [
  "neutral",
  "primary",
  "info",
  "success",
  "warning",
  "danger",
].map((color, index) => ({
  bounds: {
    height: 110,
    width: 220,
    x: 32 + (index % 3) * 250,
    y: 54 + Math.floor(index / 3) * 160,
  },
  color,
  id: `group-${color}`,
  label: color,
}));

const overlappingNodes = [
  {
    data: { label: "Draft" },
    id: "draft",
    position: { x: 130, y: 150 },
  },
  {
    data: { label: "Review" },
    id: "review",
    position: { x: 350, y: 235 },
  },
];

function GroupRegionCanvas({
  groups,
  nodes = [],
}: {
  groups: readonly FactoryGraphGroupRegionInput[];
  nodes?: typeof overlappingNodes;
}) {
  return (
    <div className="h-[32rem] w-full bg-background p-4">
      <ReactFlowProvider>
        <ReactFlow
          defaultViewport={{ x: 0, y: 0, zoom: 1 }}
          edges={[]}
          nodes={nodes}
          nodesDraggable={false}
          proOptions={{ hideAttribution: true }}
        >
          <FactoryGraphGroupRegionLayer groups={groups} />
          <Background />
        </ReactFlow>
      </ReactFlowProvider>
    </div>
  );
}

export const Empty: Story = {
  render: () => <GroupRegionCanvas groups={[]} />,
};

export const SupportedColors: Story = {
  render: () => <GroupRegionCanvas groups={supportedColorGroups} />,
};

export const LegacyOutlineAlias: Story = {
  render: () => (
    <GroupRegionCanvas
      groups={[
        {
          bounds: { height: 220, width: 420, x: 80, y: 110 },
          color: "outline",
          id: "group-outline",
          label: "Legacy outline reads as neutral",
        },
      ]}
    />
  ),
};

export const UnsupportedLegacyFallback: Story = {
  render: () => (
    <GroupRegionCanvas
      groups={[
        {
          bounds: { height: 220, width: 420, x: 80, y: 110 },
          color: "legacy-purple",
          id: "group-legacy",
          label: "Unsupported value uses neutral fallback",
        },
      ]}
    />
  ),
};

export const OverlappingContent: Story = {
  render: () => (
    <GroupRegionCanvas
      groups={[
        {
          bounds: { height: 260, width: 500, x: 50, y: 90 },
          color: "info",
          id: "group-overlap-info",
          label: "Workflow context",
        },
        {
          bounds: { height: 210, width: 360, x: 220, y: 180 },
          color: "warning",
          id: "group-overlap-warning",
          label: "Review lane",
        },
      ]}
      nodes={overlappingNodes}
    />
  ),
};

export const LongLabel: Story = {
  render: () => (
    <GroupRegionCanvas
      groups={[
        {
          bounds: { height: 220, width: 380, x: 80, y: 110 },
          color: "success",
          id: "group-long-label",
          label:
            "A long authored group label remains readable without assuming a fixed viewport",
        },
      ]}
    />
  ),
};
