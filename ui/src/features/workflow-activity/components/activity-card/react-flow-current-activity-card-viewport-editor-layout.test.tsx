import { fireEvent, render, screen } from "@testing-library/react";
import type { Node } from "@xyflow/react";
import { useEffect, type ReactNode } from "react";

import type { CurrentActivityImportController } from "../../hooks/current-activity-import-controller";
import { CurrentActivityGraphViewport } from "../react-flow-current-activity-card-viewport";

vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual("@xyflow/react");

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
        setViewport: (
          viewport: { x: number; y: number; zoom: number },
          options?: { duration?: number },
        ) => Promise<boolean>;
      }) => void;
      onMoveEnd?: (
        _event: unknown,
        viewport: { x: number; y: number; zoom: number },
      ) => void;
      onNodeDragStart?: (
        _event: unknown,
        node: Node,
        nodes: Node[],
      ) => void;
      onNodeDragStop?: (
        _event: unknown,
        node: Node,
        nodes: Node[],
      ) => void;
    }) => {
      useEffect(() => {
        onInit?.({
          fitView: async () => {
            onMoveEnd?.(null, { x: 0, y: 0, zoom: 1 });
            return true;
          },
          setViewport: async (viewport) => {
            onMoveEnd?.(null, viewport);
            return true;
          },
        });
      }, [onInit, onMoveEnd]);

      return (
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
              const draggedNodes = (nodes ?? []).filter((node) => node.selected);
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

const DEFAULT_GRAPH_RECT = {
  bottom: 720,
  height: 720,
  left: 0,
  right: 1280,
  top: 0,
  width: 1280,
  x: 0,
  y: 0,
  toJSON: () => ({}),
} as DOMRect;

function installViewportMeasurementMocks() {
  const originalBoundingClientRect =
    HTMLElement.prototype.getBoundingClientRect;
  const originalResizeObserver = globalThis.ResizeObserver;

  HTMLElement.prototype.getBoundingClientRect = () => DEFAULT_GRAPH_RECT;
  globalThis.ResizeObserver = class {
    public constructor(private readonly callback: ResizeObserverCallback) {}

    public disconnect(): void {}

    public observe(target: Element): void {
      this.callback(
        [
          {
            contentRect: DEFAULT_GRAPH_RECT,
            target,
          } as ResizeObserverEntry,
        ],
        this as unknown as ResizeObserver,
      );
    }

    public unobserve(): void {}
  } as unknown as typeof ResizeObserver;

  return () => {
    HTMLElement.prototype.getBoundingClientRect = originalBoundingClientRect;
    globalThis.ResizeObserver = originalResizeObserver;
  };
}

describe("CurrentActivityGraphViewport canonical viewport sync", () => {
  let restoreViewportMeasurementMocks = () => {};

  beforeEach(() => {
    restoreViewportMeasurementMocks = installViewportMeasurementMocks();
  });

  afterEach(() => {
    restoreViewportMeasurementMocks();
  });

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
    const setStoredViewport = vi.fn();
    const updateLayoutViewport = vi.fn();

    renderViewport({
      editorMode: true,
      setStoredViewport,
      updateLayoutViewport,
    });

    fireEvent.click(screen.getByRole("button", { name: "pan-viewport" }));

    expect(updateLayoutViewport).toHaveBeenCalledWith({
      x: 12,
      y: 34,
      zoom: 1.25,
    });
    expect(setStoredViewport).toHaveBeenCalledWith("test-graph", {
      x: 12,
      y: 34,
      zoom: 1.25,
    });
  });

  it("persists viewport panning in observe mode for editor handoff", () => {
    const setStoredViewport = vi.fn();

    renderViewport({
      editorMode: false,
      setStoredViewport,
    });

    fireEvent.click(screen.getByRole("button", { name: "pan-viewport" }));

    expect(setStoredViewport).toHaveBeenCalledWith("test-graph", {
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
  let restoreViewportMeasurementMocks = () => {};

  beforeEach(() => {
    restoreViewportMeasurementMocks = installViewportMeasurementMocks();
  });

  afterEach(() => {
    restoreViewportMeasurementMocks();
  });

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

    fireEvent.click(screen.getByRole("button", { name: "drag-selected-nodes" }));

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

    fireEvent.click(screen.getByRole("button", { name: "drag-selected-nodes" }));

    expect(moveLayoutNode).toHaveBeenCalledWith("workstation:draft", {
      x: 20,
      y: 14,
    });
  });

  it("falls back to stored node positions when editor layout handlers are absent", () => {
    const setStoredNodePosition = vi.fn();

    renderViewport({
      editorMode: false,
      nodes: [
        {
          data: { factoryGraphNodeId: "workstation:draft" },
          id: "workstation:draft",
          position: { x: 0, y: 0 },
          selected: true,
        },
      ],
      setStoredNodePosition,
    });

    fireEvent.click(screen.getByRole("button", { name: "drag-selected-nodes" }));

    expect(setStoredNodePosition).toHaveBeenCalledWith(
      "test-graph",
      "workstation:draft",
      { x: 16, y: 8 },
    );
  });
});

function renderViewport({
  canRedoLayout = false,
  canUndoLayout = false,
  canonicalLayoutViewport = null,
  editorMode = false,
  moveLayoutNode = vi.fn(),
  moveLayoutNodesByDelta = vi.fn(),
  nodes = [],
  onRedoLayout = vi.fn(),
  onUndoLayout = vi.fn(),
  setStoredNodePosition = vi.fn(),
  setStoredViewport = vi.fn(),
  updateLayoutViewport = vi.fn(),
}: {
  canRedoLayout?: boolean;
  canUndoLayout?: boolean;
  canonicalLayoutViewport?: { x: number; y: number; zoom: number } | null;
  editorMode?: boolean;
  moveLayoutNode?: (
    nodeId: string,
    position: { x: number; y: number },
  ) => void;
  moveLayoutNodesByDelta?: (
    nodeIds: readonly string[],
    delta: { x: number; y: number },
    resolvedPositionsByNodeId: ReadonlyMap<string, { x: number; y: number }>,
  ) => void;
  nodes?: Node[];
  onRedoLayout?: () => void;
  onUndoLayout?: () => void;
  setStoredNodePosition?: (
    graphKey: string,
    nodeId: string,
    position: { x: number; y: number },
  ) => void;
  setStoredViewport?: (graphKey: string, viewport: {
    x: number;
    y: number;
    zoom: number;
  }) => void;
  updateLayoutViewport?: (viewport: {
    x: number;
    y: number;
    zoom: number;
  }) => void;
} = {}) {
  const flowContainerRef = { current: null as HTMLElement | null };
  const flowInstanceRef = {
    current: null as {
      fitView: () => Promise<boolean>;
      setViewport: (
        viewport: { x: number; y: number; zoom: number },
        options?: { duration?: number },
      ) => Promise<boolean>;
    } | null,
  };

  return render(
    <CurrentActivityGraphViewport
      activeTool={null}
      canInteractWithEditor={true}
      canRedoLayout={canRedoLayout}
      canSaveDraft={false}
      canUndoLayout={canUndoLayout}
      canonicalLayoutViewport={canonicalLayoutViewport}
      editorUnavailableClassifierWorkstationName={undefined}
      editorMode={editorMode}
      edges={[]}
      flowContainerRef={flowContainerRef}
      flowInstanceRef={flowInstanceRef}
      graphKey="test-graph"
      handleDiscardPendingChanges={vi.fn()}
      handleEditorModeToggle={vi.fn()}
      handleNodesChange={vi.fn()}
      handleSaveDraft={vi.fn()}
      hasPendingChanges={false}
      hiddenNodeClasses={new Set()}
      hideShowMenuOpen={false}
      onClearPreferences={vi.fn()}
      onSelectVisibilityPreset={vi.fn()}
      preferencesDirty={false}
      visibilityPreset="all"
      headingID="test-heading"
      imports={importController}
      initialFitViewKey="full-graph"
      initialFitViewOptions={{ padding: 0.18 }}
      moveLayoutNode={moveLayoutNode}
      moveLayoutNodesByDelta={moveLayoutNodesByDelta}
      nodeTypes={{}}
      nodes={nodes}
      onEditorEdgeClick={vi.fn()}
      onEditorNodeClick={vi.fn()}
      onHideShowMenuOpenChange={vi.fn()}
      onRedoLayout={onRedoLayout}
      onToggleHiddenNodeClass={vi.fn()}
      onUndoLayout={onUndoLayout}
      onSelectTool={vi.fn()}
      setStoredNodePosition={setStoredNodePosition}
      setStoredViewport={setStoredViewport}
      updateLayoutViewport={updateLayoutViewport}
    />,
  );
}
