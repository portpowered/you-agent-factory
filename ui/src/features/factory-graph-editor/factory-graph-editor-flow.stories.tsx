import { Background, Controls, ReactFlow } from "@xyflow/react";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";

import "../../styles.css";
import type { FactoryGraphTopology } from "./factory-graph-draft-types";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "./factory-graph-editor-flow";
import type { FactoryGraphConnectionEndpoint } from "./factory-graph-editor-connections";

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
    canEditConnections: false,
    pendingAdditionEdgeIds: new Set<string>(),
    pendingConnectionSource: null,
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

const CONNECTABLE_TOPOLOGY: FactoryGraphTopology = {
  edges: [],
  nodes: [
    {
      id: "work-state:story:queued",
      key: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:queued",
    },
    {
      id: "workstation:review",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
    },
  ],
};

function ConnectionAnchorsStory() {
  const [pendingConnectionSource, setPendingConnectionSource] =
    useState<FactoryGraphConnectionEndpoint | null>(null);
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections: true,
    onConnectionAnchorClick: (endpoint) => {
      setPendingConnectionSource((currentSource) =>
        currentSource &&
        currentSource.nodeId === endpoint.nodeId &&
        currentSource.anchorId === endpoint.anchorId
          ? null
          : endpoint,
      );
    },
    pendingAdditionEdgeIds: new Set<string>(),
    pendingConnectionSource,
    pendingAdditionNodeIds: new Set<string>(),
    pendingRemovalEdgeIds: new Set<string>(),
    pendingRemovalNodeIds: new Set<string>(),
    topology: CONNECTABLE_TOPOLOGY,
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

const PENDING_EDGE_CHANGES_TOPOLOGY: FactoryGraphTopology = {
  edges: [
    {
      id: "workstation-output:workstation:review->work-state:story:done",
      kind: "workstation-output",
      source: { kind: "workstation", name: "review" },
      sourceId: "workstation:review",
      target: {
        kind: "work-state",
        stateName: "done",
        workTypeName: "story",
      },
      targetId: "work-state:story:done",
    },
    {
      id: "workstation-on-failure:workstation:review->work-state:story:queued",
      kind: "workstation-on-failure",
      source: { kind: "workstation", name: "review" },
      sourceId: "workstation:review",
      target: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
      targetId: "work-state:story:queued",
    },
  ],
  nodes: [
    {
      id: "workstation:review",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
    },
    {
      id: "work-state:story:done",
      key: {
        kind: "work-state",
        stateName: "done",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:done",
    },
    {
      id: "work-state:story:queued",
      key: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:queued",
    },
  ],
};

function PendingEdgeChangesStory() {
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections: false,
    pendingAdditionEdgeIds: new Set([
      "workstation-on-failure:workstation:review->work-state:story:queued",
    ]),
    pendingConnectionSource: null,
    pendingAdditionNodeIds: new Set<string>(),
    pendingRemovalEdgeIds: new Set([
      "workstation-output:workstation:review->work-state:story:done",
    ]),
    pendingRemovalNodeIds: new Set<string>(),
    topology: PENDING_EDGE_CHANGES_TOPOLOGY,
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

export const ConnectionAnchors = {
  render: () => <ConnectionAnchorsStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const failureSource = await canvas.findByRole("button", {
      name: "Choose review Failure connection source",
    });
    const failureTarget = await canvas.findByRole("button", {
      name: "Connect to story:queued Failure anchor",
    });
    const continueTarget = await canvas.findByRole("button", {
      name: "Connect to story:queued Continue anchor",
    });

    await expect(failureSource).toBeVisible();
    await expect(failureTarget).toBeVisible();
    await expect(continueTarget).toBeVisible();

    await userEvent.click(failureSource);
    await expect(failureSource).toHaveAttribute("aria-pressed", "true");
  },
};

export const PendingEdgeChanges = {
  render: () => <PendingEdgeChangesStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const edgeLabels = canvas.getAllByText(/route$/);
    const edgePaths = Array.from(
      canvasElement.querySelectorAll(".react-flow__edge-path"),
    );

    await expect(canvas.getByText("review")).toBeVisible();
    await expect(edgeLabels).toHaveLength(2);
    await expect(edgePaths).toHaveLength(2);
    await expect(edgePaths[0]?.getAttribute("style") ?? "").toContain(
      "var(--color-af-danger-ink)",
    );
    await expect(edgePaths[0]?.getAttribute("style") ?? "").toContain(
      "stroke-dasharray: 7, 5",
    );
    await expect(edgePaths[1]?.getAttribute("style") ?? "").toContain(
      "var(--color-af-warning-ink)",
    );
    await expect(edgePaths[1]?.getAttribute("style") ?? "").toContain(
      "stroke-dasharray: 9, 4",
    );
  },
};
