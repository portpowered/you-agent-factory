import { fireEvent, render, screen } from "@testing-library/react";
import type { Node } from "@xyflow/react";
import { type ReactNode, useEffect } from "react";

import type { CurrentActivityImportController } from "../../hooks/current-activity-import-controller";
import { CurrentActivityGraphViewport } from "../react-flow-current-activity-card-viewport";

vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual("@xyflow/react");
  const { ReactFlowProvider } = actual;

  return {
    ...actual,
    Background: () => <div data-testid="graph-background" />,
    Controls: () => <div data-testid="graph-controls" />,
    ReactFlow: ({
      className,
      children,
      defaultViewport,
      fitView,
      nodes,
      onInit,
      onMoveEnd,
      onNodeDragStart,
      onNodeDragStop,
    }: {
      className?: string;
      children: ReactNode;
      defaultViewport?: { x: number; y: number; zoom: number };
      fitView?: boolean;
      nodes?: Node[];
      onInit?: (instance: {
        fitView: () => Promise<boolean>;
        getViewport: () => { x: number; y: number; zoom: number };
        setViewport: (
          viewport: { x: number; y: number; zoom: number },
          options?: { duration?: number },
        ) => Promise<boolean>;
      }) => void;
      onMoveEnd?: (
        _event: unknown,
        viewport: { x: number; y: number; zoom: number },
      ) => void;
      onNodeDragStart?: (_event: unknown, node: Node, nodes: Node[]) => void;
      onNodeDragStop?: (_event: unknown, node: Node, nodes: Node[]) => void;
    }) => {
      useEffect(() => {
        onInit?.({
          fitView: async () => {
            onMoveEnd?.(null, { x: 0, y: 0, zoom: 1 });
            return true;
          },
          getViewport: () => ({ x: 0, y: 0, zoom: 1 }),
          setViewport: async (viewport) => {
            onMoveEnd?.(null, viewport);
            return true;
          },
        });
      }, [onInit, onMoveEnd]);

      return (
        <ReactFlowProvider>
          <div
            className={className}
            data-default-viewport={JSON.stringify(defaultViewport ?? null)}
            data-fit-view={String(fitView ?? false)}
            data-testid="mock-react-flow"
          >
            <button
              onClick={() => onMoveEnd?.(null, { x: 12, y: 34, zoom: 1.25 })}
              type="button"
            >
              pan-viewport
            </button>
            <button
              onClick={() => {
                const draggedNodes = (nodes ?? []).filter(
                  (node) => node.selected,
                );
                const primaryNode = draggedNodes[0] ?? nodes?.[0];
                if (!primaryNode) {
                  return;
                }

                onNodeDragStart?.(null, primaryNode, nodes ?? []);
                onNodeDragStop?.(
                  null,
                  {
                    ...primaryNode,
                    position: {
                      x: primaryNode.position.x + 16,
                      y: primaryNode.position.y + 8,
                    },
                  },
                  nodes ?? [],
                );
              }}
              type="button"
            >
              drag-selected-nodes
            </button>
            {children}
          </div>
        </ReactFlowProvider>
      );
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

const DEFAULT_VIEWPORT_MEASUREMENT = {
  height: 720,
  ready: true,
  width: 1280,
} as const;

describe("CurrentActivityGraphViewport canonical viewport sync", () => {
  it("projects saved canonical viewport into React Flow and skips fitView", () => {
    renderViewport({
      canonicalLayoutViewport: { x: 80, y: 120, zoom: 1.4 },
    });

    const reactFlow = screen.getByTestId("mock-react-flow");
    expect(reactFlow.getAttribute("data-fit-view")).toBe("false");
    expect(reactFlow.getAttribute("data-default-viewport")).toBe(
      JSON.stringify({ x: 80, y: 120, zoom: 1.4 }),
    );
  });

  it("persists viewport panning into canonical layout in editor mode", () => {
    const updateLayoutViewport = vi.fn();

    renderViewport({
      editorMode: true,
      updateLayoutViewport,
    });

    fireEvent.click(screen.getByRole("button", { name: "pan-viewport" }));

    expect(updateLayoutViewport).toHaveBeenCalledWith({
      x: 12,
      y: 34,
      zoom: 1.25,
    });
  });

  it("reports the latest placement viewport for add-node placement", () => {
    const updatePlacementViewport = vi.fn();

    renderViewport({
      updatePlacementViewport,
    });

    fireEvent.click(screen.getByRole("button", { name: "pan-viewport" }));

    expect(updatePlacementViewport).toHaveBeenLastCalledWith({
      height: 720,
      viewport: { x: 12, y: 34, zoom: 1.25 },
      width: 1280,
    });
  });

  it("persists viewport panning into canonical layout in observe mode", () => {
    const updateLayoutViewport = vi.fn();

    renderViewport({
      editorMode: false,
      updateLayoutViewport,
    });

    fireEvent.click(screen.getByRole("button", { name: "pan-viewport" }));

    expect(updateLayoutViewport).toHaveBeenCalledWith({
      x: 12,
      y: 34,
      zoom: 1.25,
    });
  });

  it("skips persisting viewport changes triggered by canonical fitView sync", () => {
    const updateLayoutViewport = vi.fn();

    renderViewport({
      editorMode: true,
      updateLayoutViewport,
    });

    expect(updateLayoutViewport).not.toHaveBeenCalled();
  });

  it("skips persisting viewport changes triggered by saved canonical viewport sync", () => {
    const updateLayoutViewport = vi.fn();

    renderViewport({
      canonicalLayoutViewport: { x: 80, y: 120, zoom: 1.4 },
      editorMode: true,
      updateLayoutViewport,
    });

    expect(updateLayoutViewport).not.toHaveBeenCalled();
  });
});

describe("CurrentActivityGraphViewport editor layout interactions", () => {
  it("routes undo and redo keyboard shortcuts through layout history handlers", () => {
    const onUndoLayout = vi.fn();
    const onRedoLayout = vi.fn();

    renderViewport({
      canRedoLayout: true,
      canUndoLayout: true,
      editorMode: true,
      onRedoLayout,
      onUndoLayout,
    });

    const canvas = screen.getByRole("region", { name: "Work graph viewport" });
    fireEvent.keyDown(canvas, { code: "KeyZ", ctrlKey: true, key: "z" });
    fireEvent.keyDown(canvas, {
      code: "KeyZ",
      ctrlKey: true,
      key: "z",
      shiftKey: true,
    });

    expect(onUndoLayout).toHaveBeenCalledTimes(1);
    expect(onRedoLayout).toHaveBeenCalledTimes(1);
  });

  it("records multi-node drag deltas into canonical layout state", () => {
    const moveLayoutNodesByDelta = vi.fn();

    renderViewport({
      editorMode: true,
      moveLayoutNodesByDelta,
      nodes: [
        {
          data: { factoryGraphNodeId: "workstation:draft" },
          id: "workstation:draft",
          position: { x: 10, y: 20 },
          selected: true,
        },
        {
          data: { factoryGraphNodeId: "work-state:story:queued" },
          id: "work-state:story:queued",
          position: { x: 40, y: 60 },
          selected: true,
        },
      ],
    });

    fireEvent.click(
      screen.getByRole("button", { name: "drag-selected-nodes" }),
    );

    expect(moveLayoutNodesByDelta).toHaveBeenCalledWith(
      ["workstation:draft", "work-state:story:queued"],
      { x: 16, y: 8 },
      expect.any(Map),
    );
  });

  it("records single-node drag movement into canonical layout state", () => {
    const moveLayoutNode = vi.fn();

    renderViewport({
      editorMode: true,
      moveLayoutNode,
      nodes: [
        {
          data: { factoryGraphNodeId: "workstation:draft" },
          id: "workstation:draft",
          position: { x: 4, y: 6 },
          selected: false,
        },
      ],
    });

    fireEvent.click(
      screen.getByRole("button", { name: "drag-selected-nodes" }),
    );

    expect(moveLayoutNode).toHaveBeenCalledWith("workstation:draft", {
      x: 20,
      y: 14,
    });
  });
});

function renderViewport({
  canRedoLayout = false,
  canUndoLayout = false,
  canonicalLayoutViewport = null,
  editorMode = false,
  includeMoveLayoutNode = true,
  moveLayoutNode = vi.fn(),
  moveLayoutNodesByDelta = vi.fn(),
  nodes = [],
  onRedoLayout = vi.fn(),
  onUndoLayout = vi.fn(),
  updateLayoutViewport = vi.fn(),
  updatePlacementViewport = vi.fn(),
}: {
  canRedoLayout?: boolean;
  canUndoLayout?: boolean;
  canonicalLayoutViewport?: { x: number; y: number; zoom: number } | null;
  editorMode?: boolean;
  includeMoveLayoutNode?: boolean;
  moveLayoutNode?: (nodeId: string, position: { x: number; y: number }) => void;
  moveLayoutNodesByDelta?: (
    nodeIds: readonly string[],
    delta: { x: number; y: number },
    resolvedPositionsByNodeId: ReadonlyMap<string, { x: number; y: number }>,
  ) => void;
  nodes?: Node[];
  onRedoLayout?: () => void;
  onUndoLayout?: () => void;
  updateLayoutViewport?: (viewport: {
    x: number;
    y: number;
    zoom: number;
  }) => void;
  updatePlacementViewport?: (viewport: {
    height: number;
    viewport: { x: number; y: number; zoom: number };
    width: number;
  }) => void;
} = {}) {
  const flowContainerRef = { current: null as HTMLElement | null };
  const flowInstanceRef = {
    current: null as {
      fitView: () => Promise<boolean>;
      getViewport: () => { x: number; y: number; zoom: number };
      setViewport: (
        viewport: { x: number; y: number; zoom: number },
        options?: { duration?: number },
      ) => Promise<boolean>;
    } | null,
  };

  return render(
    <CurrentActivityGraphViewport
      addControls={{ updatePlacementViewport }}
      editorControls={{
        activeTool: null,
        canInteract: true,
        discardPendingChanges: vi.fn(),
        isEditing: editorMode,
        selectTool: vi.fn(),
        toggleMode: vi.fn(),
        unavailableClassifierWorkstationName: undefined,
      }}
      edges={[]}
      flowContainerRef={flowContainerRef}
      flowInstanceRef={flowInstanceRef}
      viewportMeasurement={DEFAULT_VIEWPORT_MEASUREMENT}
      handleNodesChange={vi.fn()}
      hasPendingChanges={false}
      headingID="test-heading"
      imports={importController}
      layoutControls={{
        canMoveLayout: includeMoveLayoutNode,
        canRedo: canRedoLayout,
        canUndo: canUndoLayout,
        canonicalViewport: canonicalLayoutViewport,
        initialFitViewKey: "full-graph",
        initialFitViewOptions: { padding: 0.18 },
        moveNode: moveLayoutNode,
        moveNodesByDelta: moveLayoutNodesByDelta,
        redo: onRedoLayout,
        reset: vi.fn(),
        undo: onUndoLayout,
        updateViewport: updateLayoutViewport,
      }}
      nodeTypes={{}}
      nodes={nodes}
      onEditorEdgeClick={vi.fn()}
      onEditorNodeClick={vi.fn()}
      saveControls={{ canSave: false, requestConfirmation: vi.fn() }}
      visibilityControls={{
        hiddenNodeClasses: new Set(),
        isDirty: false,
        isMenuOpen: false,
        preset: "all",
        resetPreferences: vi.fn(),
        setMenuOpen: vi.fn(),
        setPreset: vi.fn(),
        toggleHiddenNodeClass: vi.fn(),
      }}
    />,
  );
}
