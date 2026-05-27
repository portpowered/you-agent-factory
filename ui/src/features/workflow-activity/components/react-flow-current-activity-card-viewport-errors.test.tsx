import { cleanup, render } from "@testing-library/react";
import type { Edge, Node } from "@xyflow/react";
import type { ReactNode } from "react";

import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

let reactFlowErrorToReport: { errorId: string; message: string } | null = null;

vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual("@xyflow/react");

  return {
    ...actual,
    Background: () => <div data-testid="graph-background" />,
    Controls: () => <div data-testid="graph-controls" />,
    ReactFlow: ({
      children,
      edges,
      nodes,
      onError,
    }: {
      children: ReactNode;
      edges?: Edge[];
      nodes?: Node[];
      onError?: (errorId: string, message: string) => void;
    }) => {
      if (reactFlowErrorToReport) {
        onError?.(
          reactFlowErrorToReport.errorId,
          reactFlowErrorToReport.message,
        );
      }
      reportMissingEdgeHandles({
        edges: edges ?? [],
        nodes: nodes ?? [],
        onError,
      });

      return (
        <div data-testid="mock-react-flow">
          <ul aria-label="Rendered graph edges">
            {(edges ?? []).map((edge) => (
              <li key={edge.id}>{edge.id}</li>
            ))}
          </ul>
          {children}
        </div>
      );
    },
  };
});

function reportMissingEdgeHandles({
  edges,
  nodes,
  onError,
}: {
  edges: Edge[];
  nodes: Node[];
  onError?: (errorId: string, message: string) => void;
}) {
  for (const edge of edges) {
    const sourceNode = nodes.find((node) => node.id === edge.source);
    const targetNode = nodes.find((node) => node.id === edge.target);
    if (!sourceNode || !targetNode) {
      onError?.(
        "006",
        `Couldn't create edge for missing node, edge id: ${edge.id}.`,
      );
      continue;
    }

    if (!nodeHasHandle(sourceNode, edge.sourceHandle)) {
      onError?.(
        "008",
        `Couldn't create edge for source handle id: "${edge.sourceHandle}", edge id: ${edge.id}.`,
      );
      continue;
    }

    if (!nodeHasHandle(targetNode, edge.targetHandle)) {
      onError?.(
        "008",
        `Couldn't create edge for target handle id: "${edge.targetHandle}", edge id: ${edge.id}.`,
      );
    }
  }
}

function nodeHasHandle(node: Node, handleId: string | null | undefined) {
  const handles = (node.data as { handles?: Array<{ id: string }> }).handles;
  return Boolean(handleId && handles?.some((handle) => handle.id === handleId));
}

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

afterEach(() => {
  cleanup();
  reactFlowErrorToReport = null;
});

describe("CurrentActivityGraphViewport React Flow errors", () => {
  it("throws when React Flow reports an edge endpoint handle mismatch", () => {
    reactFlowErrorToReport = {
      errorId: "008",
      message:
        'Couldn\'t create edge for source handle id: "out-review", edge id: hidden-edge.',
    };

    expect(() => renderViewport()).toThrow(
      /React Flow factory graph endpoint error 008/,
    );
  });

  it("renders semantic endpoint edges when source and target handles exist", () => {
    const { getByRole } = renderViewport({
      edges: [
        {
          id: "workstation-output:workstation:review->place:story:done",
          source: "workstation:review",
          sourceHandle: "workstation-output-source",
          target: "place:story:done",
          targetHandle: "workstation-output-target",
        },
      ],
      nodes: [
        semanticNode("workstation:review", "workstation-output-source"),
        semanticNode("place:story:done", "workstation-output-target"),
      ],
    });

    expect(
      getByRole("list", { name: "Rendered graph edges" }).textContent,
    ).toContain("workstation-output:workstation:review->place:story:done");
  });

  it("throws when legacy observe-mode endpoints target semantic graph nodes", () => {
    expect(() =>
      renderViewport({
        edges: [
          {
            id: "workstation-output:workstation:review->place:story:done",
            source: "workstation:review",
            sourceHandle: "out-0",
            target: "place:story:done",
            targetHandle: "in-0",
          },
        ],
        nodes: [
          semanticNode("workstation:review", "workstation-output-source"),
          semanticNode("place:story:done", "workstation-output-target"),
        ],
      }),
    ).toThrow(/React Flow factory graph endpoint error 008/);
  });
});

function semanticNode(id: string, handleId: string): Node {
  return {
    data: {
      handles: [
        {
          id: handleId,
          label: handleId,
          side: handleId.endsWith("-source") ? "right" : "left",
          type: handleId.endsWith("-source") ? "source" : "target",
        },
      ],
    },
    id,
    position: { x: 0, y: 0 },
  };
}

function renderViewport({
  edges = [],
  nodes = [],
}: {
  edges?: Edge[];
  nodes?: Node[];
} = {}) {
  return render(
    <CurrentActivityGraphViewport
      activeTool={null}
      canInteractWithEditor={false}
      canSaveDraft={false}
      editorMode={false}
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
      onSelectTool={vi.fn()}
      setStoredNodePosition={vi.fn()}
    />,
  );
}
