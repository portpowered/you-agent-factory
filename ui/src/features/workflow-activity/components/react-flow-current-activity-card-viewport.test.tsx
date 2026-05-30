import { fireEvent, render, screen } from "@testing-library/react";
import type { Edge, Node } from "@xyflow/react";
import type { ReactNode } from "react";

import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

vi.mock("@xyflow/react", () => ({
  Background: () => <div data-testid="graph-background" />,
  Controls: () => <div data-testid="graph-controls" />,
  ReactFlow: ({
      children,
      edges,
      nodes,
      onEdgeClick,
      onNodeClick,
    }: {
      children: ReactNode;
      edges?: Edge[];
      nodes?: Node[];
      onEdgeClick?: (_event: unknown, edge: Edge) => void;
      onNodeClick?: (_event: unknown, node: { id: string }) => void;
    }) => (
      <div data-testid="mock-react-flow">
        {(nodes ?? []).map((node) => (
          <button
            key={node.id}
            onClick={() => onNodeClick?.(null, { id: node.id })}
            type="button"
          >
            {node.id}
          </button>
        ))}
        {(edges ?? []).map((edge) => (
          <button
            key={edge.id}
            onClick={() => onEdgeClick?.(null, edge)}
            type="button"
          >
            {edge.id}
          </button>
        ))}
        {children}
      </div>
    ),
}));

const importController: CurrentActivityImportController = {
  activateImport: vi.fn().mockResolvedValue(undefined),
  activationState: { status: "idle" },
  clearActivationError: vi.fn(),
  clearError: vi.fn(),
  closeImportPreview: vi.fn(),
  dropState: { status: "idle" },
  importPreviewState: { status: "idle" },
  onDragEnter: vi.fn(),
  onDragLeave: vi.fn(),
  onDragOver: vi.fn(),
  onDrop: vi.fn(),
};

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: viewport coverage keeps related mocked React Flow click paths together.
describe("CurrentActivityGraphViewport", () => {
  it.each([
    {
      expectedNodeId: "work-type:story",
      kind: "work-type",
      renderedNodeId: "work-type:story",
    },
    {
      expectedNodeId: "worker:writer",
      kind: "worker",
      renderedNodeId: "place:worker:writer",
    },
    {
      expectedNodeId: "resource:gpu",
      kind: "resource",
      renderedNodeId: "resource:gpu",
    },
  ])("maps rendered $kind nodes to factory graph ids before editor node deletion", ({
    expectedNodeId,
    kind,
    renderedNodeId,
  }) => {
    const handleEditorNodeClick = vi.fn();

    renderViewport({
      activeTool: "delete",
      editorMode: true,
      nodes: [
        {
          data: {
            factoryGraphNodeId: expectedNodeId,
            kind,
          },
          id: renderedNodeId,
          position: { x: 0, y: 0 },
        },
      ],
      onEditorNodeClick: handleEditorNodeClick,
    });

    fireEvent.click(screen.getByRole("button", { name: renderedNodeId }));

    expect(handleEditorNodeClick).toHaveBeenCalledWith(expectedNodeId);
  });

  it.each([
    {
      expectedEdgeId: "worker-resource:resource:gpu->worker:writer",
      renderedEdgeId: "worker-resource:resource:gpu->place:worker:writer",
      source: "resource:gpu",
      sourceFactoryGraphNodeId: "resource:gpu",
      target: "place:worker:writer",
      targetFactoryGraphNodeId: "worker:writer",
    },
    {
      expectedEdgeId: "workstation-resource:resource:gpu->workstation:review",
      renderedEdgeId: "workstation-resource:resource:gpu->workstation:review",
      source: "resource:gpu",
      sourceFactoryGraphNodeId: "resource:gpu",
      target: "workstation:review",
      targetFactoryGraphNodeId: "workstation:review",
    },
    {
      expectedEdgeId: "worker-assignment:worker:writer->workstation:review",
      renderedEdgeId:
        "worker-assignment:place:worker:writer->workstation:review",
      source: "place:worker:writer",
      sourceFactoryGraphNodeId: "worker:writer",
      target: "workstation:review",
      targetFactoryGraphNodeId: "workstation:review",
    },
  ])("maps rendered edge endpoints to factory graph ids before edge deletion", ({
    expectedEdgeId,
    renderedEdgeId,
    source,
    sourceFactoryGraphNodeId,
    target,
    targetFactoryGraphNodeId,
  }) => {
    const handleEditorEdgeClick = vi.fn();

    renderViewport({
      activeTool: "delete",
      editorMode: true,
      edges: [
        {
          id: renderedEdgeId,
          source,
          target,
        },
      ],
      nodes: [
        {
          data: {
            factoryGraphNodeId: sourceFactoryGraphNodeId,
            kind: "resource",
          },
          id: source,
          position: { x: 0, y: 0 },
        },
        {
          data: {
            factoryGraphNodeId: targetFactoryGraphNodeId,
            kind: "worker",
          },
          id: target,
          position: { x: 0, y: 0 },
        },
      ],
      onEditorEdgeClick: handleEditorEdgeClick,
    });

    fireEvent.click(screen.getByRole("button", { name: renderedEdgeId }));

    expect(handleEditorEdgeClick).toHaveBeenCalledWith(expectedEdgeId);
  });

  it("keeps canonical edge ids unchanged before editor edge deletion", () => {
    const handleEditorEdgeClick = vi.fn();

    renderViewport({
      activeTool: "delete",
      editorMode: true,
      edges: [
        {
          id: "workstation-output:workstation:review->work-state:story:done",
          source: "workstation:review",
          target: "work-state:story:done",
        },
      ],
      nodes: [],
      onEditorEdgeClick: handleEditorEdgeClick,
    });

    fireEvent.click(
      screen.getByRole("button", {
        name: "workstation-output:workstation:review->work-state:story:done",
      }),
    );

    expect(handleEditorEdgeClick).toHaveBeenCalledWith(
      "workstation-output:workstation:review->work-state:story:done",
    );
  });

  it("does not delete graph entities while outside editor mode", () => {
    const handleEditorNodeClick = vi.fn();
    const handleEditorEdgeClick = vi.fn();

    renderViewport({
      activeTool: "delete",
      editorMode: false,
      edges: [
        {
          id: "worker-resource:resource:gpu->place:worker:writer",
          source: "resource:gpu",
          target: "place:worker:writer",
        },
      ],
      nodes: [
        {
          data: {
            factoryGraphNodeId: "work-type:story",
            kind: "work-type",
          },
          id: "work-type:story",
          position: { x: 0, y: 0 },
        },
      ],
      onEditorEdgeClick: handleEditorEdgeClick,
      onEditorNodeClick: handleEditorNodeClick,
    });

    fireEvent.click(screen.getByRole("button", { name: "work-type:story" }));
    fireEvent.click(
      screen.getByRole("button", {
        name: "worker-resource:resource:gpu->place:worker:writer",
      }),
    );

    expect(handleEditorNodeClick).not.toHaveBeenCalled();
    expect(handleEditorEdgeClick).not.toHaveBeenCalled();
  });
});

function renderViewport({
  activeTool = null,
  edges = [],
  editorMode = false,
  nodes = [],
  onEditorEdgeClick = vi.fn(),
  onEditorNodeClick = vi.fn(),
}: {
  activeTool?: "add" | "connect" | "delete" | null;
  edges?: Edge[];
  editorMode?: boolean;
  nodes?: Node[];
  onEditorEdgeClick?: (edgeId: string) => void;
  onEditorNodeClick?: (nodeId: string) => void;
} = {}) {
  return render(
    <CurrentActivityGraphViewport
      activeTool={activeTool}
      canInteractWithEditor={true}
      canSaveDraft={false}
      editorMode={editorMode}
      edges={edges}
      graphKey="test-graph"
      handleDiscardPendingChanges={vi.fn()}
      handleNodesChange={vi.fn()}
      handleSaveDraft={vi.fn()}
      hasPendingChanges={false}
      headingID="test-heading"
      imports={importController}
      initialFitViewKey="full-graph"
      initialFitViewOptions={{ padding: 0.18 }}
      nodeTypes={{}}
      nodes={nodes}
      onEditorEdgeClick={onEditorEdgeClick}
      onEditorNodeClick={onEditorNodeClick}
      onSelectTool={vi.fn()}
      setStoredNodePosition={vi.fn()}
    />,
  );
}
