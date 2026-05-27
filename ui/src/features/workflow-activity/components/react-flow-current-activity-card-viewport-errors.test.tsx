import { cleanup, render } from "@testing-library/react";
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
      onError,
    }: {
      children: ReactNode;
      onError?: (errorId: string, message: string) => void;
    }) => {
      if (reactFlowErrorToReport) {
        onError?.(
          reactFlowErrorToReport.errorId,
          reactFlowErrorToReport.message,
        );
      }

      return <div data-testid="mock-react-flow">{children}</div>;
    },
  };
});

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
});

function renderViewport() {
  return render(
    <CurrentActivityGraphViewport
      activeTool={null}
      canInteractWithEditor={false}
      canSaveDraft={false}
      editorMode={false}
      edges={[]}
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
      nodes={[]}
      onSelectTool={vi.fn()}
      setStoredNodePosition={vi.fn()}
    />,
  );
}
