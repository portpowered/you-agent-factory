import type { Meta, StoryObj } from "@storybook/react-vite";
import { Background, ReactFlow, ReactFlowProvider } from "@xyflow/react";
import { useState } from "react";

import "../../../../../styles.css";
import {
  type FactoryGraphGroupRegionInput,
  FactoryGraphGroupRegionLayer,
} from "@you-agent-factory/factory-graph";
import type { FactoryLayoutGroup } from "../../../lib/layout/visual-groups/factory-graph-layout-groups";
import { FactoryGraphVisualGroupLayer } from "./factory-graph-visual-group-layer";

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

function EditableGroupCanvas() {
  const [groups, setGroups] = useState<FactoryLayoutGroup[]>([
    {
      bounds: { height: 240, width: 500, x: 50, y: 90 },
      color: "info",
      id: "group-editable",
      label: "Selected editable group",
      nodeIds: ["draft", "missing-node"],
    },
    {
      bounds: { height: 170, width: 300, x: 300, y: 230 },
      color: "warning",
      id: "group-empty",
      label: "Empty membership",
      nodeIds: [],
    },
  ]);
  const [selectedGroupId, setSelectedGroupId] = useState("group-editable");

  return (
    <div className="h-[32rem] w-full bg-background p-4">
      <ReactFlowProvider>
        <ReactFlow
          defaultViewport={{ x: 0, y: 0, zoom: 1 }}
          edges={[]}
          nodes={overlappingNodes}
          nodesDraggable={false}
          proOptions={{ hideAttribution: true }}
        >
          <FactoryGraphVisualGroupLayer
            canEdit
            groupAriaLabel={(group) =>
              `${group.label ?? group.id}; use arrow keys to move`
            }
            groupOutlineAriaLabel={(group, edge) =>
              `${group.label ?? group.id} ${edge} outline; use arrow keys to move`
            }
            groups={groups}
            onMoveGroup={(groupId, delta) =>
              setGroups((current) =>
                current.map((group) =>
                  group.id === groupId
                    ? {
                        ...group,
                        bounds: {
                          ...group.bounds,
                          x: group.bounds.x + delta.x,
                          y: group.bounds.y + delta.y,
                        },
                      }
                    : group,
                ),
              )
            }
            onResizeGroup={(groupId, bounds) =>
              setGroups((current) =>
                current.map((group) =>
                  group.id === groupId ? { ...group, bounds } : group,
                ),
              )
            }
            onSelectGroup={setSelectedGroupId}
            resizeHandleAriaLabel={(corner) => `Resize ${corner}`}
            selectedGroupId={selectedGroupId}
          />
          <Background />
        </ReactFlow>
      </ReactFlowProvider>
    </div>
  );
}

export const Empty: Story = {
  render: () => <GroupRegionCanvas groups={[]} />,
};

export const ReadOnly: Story = {
  render: () => (
    <GroupRegionCanvas
      groups={supportedColorGroups.slice(0, 2)}
      nodes={overlappingNodes}
    />
  ),
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

export const EditableSelectedFocusedAndResized: Story = {
  parameters: {
    docs: {
      description: {
        story:
          "The selected group exposes a label, four outline affordances, and explicit resize handles. Focus the label or outline and use arrow keys to move; focus a resize handle and use arrow keys to resize.",
      },
    },
  },
  render: () => <EditableGroupCanvas />,
};

export const EmptyAndMissingMembership: Story = {
  render: () => (
    <GroupRegionCanvas
      groups={[
        {
          bounds: { height: 180, width: 300, x: 70, y: 110 },
          color: "neutral",
          id: "group-empty-membership",
          label: "Empty membership",
          nodeIds: [],
        },
        {
          bounds: { height: 180, width: 300, x: 400, y: 110 },
          color: "danger",
          id: "group-missing-members",
          label: "Missing member IDs stay safe",
          nodeIds: ["legacy-node-no-longer-present"],
        },
      ]}
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
