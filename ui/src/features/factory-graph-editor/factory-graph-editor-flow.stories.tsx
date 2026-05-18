import { Background, Controls, ReactFlow } from "@xyflow/react";
import { expect, within } from "storybook/test";

import "../../styles.css";
import type { FactoryGraphTopology } from "./factory-graph-draft-types";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "./factory-graph-editor-flow";

const PENDING_REMOVAL_TOPOLOGY: FactoryGraphTopology = {
  edges: [
    {
      id: "worker-assignment:worker:writer->workstation:review",
      kind: "worker-assignment",
      source: { kind: "worker", name: "writer" },
      sourceId: "worker:writer",
      target: { kind: "workstation", name: "review" },
      targetId: "workstation:review",
    },
    {
      id: "workstation-output:review->story:complete",
      kind: "workstation-output",
      source: { kind: "workstation", name: "review" },
      sourceId: "workstation:review",
      target: {
        kind: "work-state",
        stateName: "complete",
        workTypeName: "story",
      },
      targetId: "work-state:story:complete",
    },
  ],
  nodes: [
    {
      id: "worker:writer",
      key: { kind: "worker", name: "writer" },
      kind: "worker",
      label: "writer",
    },
    {
      id: "workstation:review",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
    },
    {
      id: "work-state:story:complete",
      key: {
        kind: "work-state",
        stateName: "complete",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:complete",
    },
  ],
};

function PendingRemovalStory() {
  const flow = buildFactoryGraphEditorFlowModel({
    pendingAdditionNodeIds: new Set<string>(),
    pendingRemovalEdgeIds: new Set([
      "worker-assignment:worker:writer->workstation:review",
      "workstation-output:review->story:complete",
    ]),
    pendingRemovalNodeIds: new Set(["workstation:review"]),
    topology: PENDING_REMOVAL_TOPOLOGY,
  });

  return (
    <div className="h-[520px] w-full rounded-[1.5rem] border border-af-overlay/12 bg-af-surface/72 p-4">
      <ReactFlow
        defaultEdgeOptions={{ selectable: false }}
        edges={flow.edges}
        fitView={true}
        nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
        nodes={flow.nodes}
        nodesDraggable={false}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}

export default {
  title: "Agent Factory/Dashboard/Factory Graph Editor Flow",
  tags: ["test"],
};

export const PendingRemoval = {
  render: () => <PendingRemovalStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const reviewNode = await canvas.findByText("review");
    const removingBadge = await canvas.findByText("Removing");
    const edgePath = canvasElement.querySelector(".react-flow__edge-path");

    await expect(reviewNode).toBeVisible();
    await expect(removingBadge).toBeVisible();
    await expect(canvas.getByText("writer")).toBeVisible();
    await expect(canvas.getByText("story:complete")).toBeVisible();
    if (!(edgePath instanceof SVGPathElement)) {
      throw new Error("Expected a React Flow edge path for pending removal.");
    }
    await expect(edgePath.getAttribute("style") ?? "").toContain(
      "var(--color-af-danger-ink)",
    );
    await expect(edgePath.getAttribute("style") ?? "").toContain(
      "stroke-dasharray: 7, 5",
    );
  },
};
