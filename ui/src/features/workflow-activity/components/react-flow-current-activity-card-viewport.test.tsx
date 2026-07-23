// biome-ignore lint/style/noExcessiveLinesPerFile: viewport coverage keeps related mocked React Flow click paths together.
import { fireEvent, render, screen } from "@testing-library/react";
import type { Edge, Node } from "@xyflow/react";
import { type PointerEventHandler, type ReactNode, useEffect } from "react";

import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

const setViewport = vi.fn().mockResolvedValue(true);

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
      deleteKeyCode,
      edges,
      fitView,
      nodes,
      onEdgeClick,
      onInit,
      onNodeClick,
      onPointerCancelCapture,
      onPointerDownCapture,
      onPointerMoveCapture,
      onPointerUpCapture,
      onPaneClick: _onPaneClick,
      onSelectionChange: _onSelectionChange,
      onSelectionStart: _onSelectionStart,
      panOnDrag,
      panOnScroll,
      selectionOnDrag,
      zoomOnScroll,
    }: {
      className?: string;
      children: ReactNode;
      defaultViewport?: { x: number; y: number; zoom: number };
      deleteKeyCode?: string | null;
      edges?: Edge[];
      fitView?: boolean;
      nodes?: Node[];
      onEdgeClick?: (_event: unknown, edge: Edge) => void;
      onInit?: (instance: {
        fitView: () => Promise<boolean>;
        getViewport: () => { x: number; y: number; zoom: number };
        setViewport: () => Promise<boolean>;
      }) => void;
      onKeyDown?: (event: KeyboardEvent) => void;
      onNodeClick?: (_event: unknown, node: { id: string }) => void;
      onPointerCancelCapture?: PointerEventHandler<HTMLDivElement>;
      onPointerDownCapture?: PointerEventHandler<HTMLDivElement>;
      onPointerMoveCapture?: PointerEventHandler<HTMLDivElement>;
      onPointerUpCapture?: PointerEventHandler<HTMLDivElement>;
      onPaneClick?: () => void;
      onSelectionChange?: (params: { edges: Edge[]; nodes: Node[] }) => void;
      onSelectionStart?: (event: { shiftKey: boolean }) => void;
      panOnDrag?: boolean | number[];
      panOnScroll?: boolean;
      selectionOnDrag?: boolean;
      zoomOnScroll?: boolean;
    }) => {
      useEffect(() => {
        onInit?.({
          fitView: async () => true,
          getViewport: () => ({ x: 0, y: 0, zoom: 1 }),
          setViewport,
        });
      }, [onInit]);

      return (
        <div
          className={className}
          data-default-viewport={JSON.stringify(defaultViewport ?? null)}
          data-delete-key-code={
            deleteKeyCode === null
              ? "null"
              : deleteKeyCode === undefined
                ? "unset"
                : String(deleteKeyCode)
          }
          data-fit-view={String(fitView ?? false)}
          data-pan-on-drag={String(panOnDrag ?? "unset")}
          data-pan-on-scroll={String(panOnScroll ?? "unset")}
          data-selection-on-drag={String(selectionOnDrag ?? "unset")}
          data-testid="mock-react-flow"
          data-zoom-on-scroll={String(zoomOnScroll ?? "unset")}
          onPointerCancelCapture={onPointerCancelCapture}
          onPointerDownCapture={onPointerDownCapture}
          onPointerMoveCapture={onPointerMoveCapture}
          onPointerUpCapture={onPointerUpCapture}
        >
          <div
            className="react-flow__pane"
            data-testid="mock-react-flow-pane"
          />
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: viewport coverage keeps related mocked React Flow click paths together.
describe("CurrentActivityGraphViewport", () => {
  const originalBoundingClientRect =
    HTMLElement.prototype.getBoundingClientRect;
  const originalResizeObserver = globalThis.ResizeObserver;

  beforeEach(() => {
    setViewport.mockClear();
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
  });

  afterEach(() => {
    HTMLElement.prototype.getBoundingClientRect = originalBoundingClientRect;
    globalThis.ResizeObserver = originalResizeObserver;
  });

  it("renders hide/show controls in observer mode without editor tools", () => {
    renderViewport({ editorMode: false });

    expect(
      screen.getByRole("button", {
        name: "Edit mode",
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: "Show or hide",
      }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add" })).toBeNull();
  });

  it.each([{ editorMode: false }, { editorMode: true }])(
    "does not render the top-right work-state phase legend when editorMode is $editorMode",
    ({ editorMode }) => {
      const { container } = renderViewport({ editorMode });

      expect(
        container.querySelector("[data-factory-graph-work-state-phase-legend]"),
      ).toBeNull();
    },
  );

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
  ])(
    "maps rendered $kind nodes to factory graph ids before editor node deletion",
    async ({ expectedNodeId, kind, renderedNodeId }) => {
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

      fireEvent.click(
        await screen.findByRole("button", { name: renderedNodeId }),
      );

      expect(handleEditorNodeClick).toHaveBeenCalledWith(expectedNodeId);
    },
  );

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
  ])(
    "maps rendered edge endpoints to factory graph ids before edge deletion",
    async ({
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

      fireEvent.click(
        await screen.findByRole("button", { name: renderedEdgeId }),
      );

      expect(handleEditorEdgeClick).toHaveBeenCalledWith(expectedEdgeId);
    },
  );

  it("keeps canonical edge ids unchanged before editor edge deletion", async () => {
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
      await screen.findByRole("button", {
        name: "workstation-output:workstation:review->work-state:story:done",
      }),
    );

    expect(handleEditorEdgeClick).toHaveBeenCalledWith(
      "workstation-output:workstation:review->work-state:story:done",
    );
  });

  it("uses factory graph edge data as the canonical editor edge id", async () => {
    const handleEditorEdgeClick = vi.fn();

    renderViewport({
      activeTool: "delete",
      editorMode: true,
      edges: [
        {
          data: {
            factoryGraphEdgeId:
              "workstation-input:work-state:story:queued->workstation:review",
          },
          id: "workstation-resource:resource:story->workstation:review",
          source: "resource:story",
          target: "workstation:review",
        },
      ],
      nodes: [],
      onEditorEdgeClick: handleEditorEdgeClick,
    });

    fireEvent.click(
      await screen.findByRole("button", {
        name: "workstation-resource:resource:story->workstation:review",
      }),
    );

    expect(handleEditorEdgeClick).toHaveBeenCalledWith(
      "workstation-input:work-state:story:queued->workstation:review",
    );
  });

  it("does not delete graph entities while outside editor mode", async () => {
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

    fireEvent.click(
      await screen.findByRole("button", { name: "work-type:story" }),
    );
    fireEvent.click(
      await screen.findByRole("button", {
        name: "worker-resource:resource:gpu->place:worker:writer",
      }),
    );

    expect(handleEditorNodeClick).not.toHaveBeenCalled();
    expect(handleEditorEdgeClick).not.toHaveBeenCalled();
  });

  it("uses selection-first React Flow gesture defaults and clears selection on Escape", async () => {
    const clearGraphSelection = vi.fn();
    const handleGraphSelectionChange = vi.fn();

    renderViewport({
      clearGraphSelection,
      handleGraphSelectionChange,
    });

    const reactFlow = await screen.findByTestId("mock-react-flow");
    expect(reactFlow.getAttribute("data-selection-on-drag")).toBe("true");
    expect(reactFlow.getAttribute("data-pan-on-drag")).toBe("");
    expect(reactFlow.getAttribute("data-pan-on-scroll")).toBe("true");
    expect(reactFlow.getAttribute("data-zoom-on-scroll")).toBe("false");
    expect(reactFlow.getAttribute("data-delete-key-code")).toBe("null");

    fireEvent.keyDown(
      screen.getByRole("region", { name: "Work graph viewport" }),
      { key: "Escape" },
    );
    expect(clearGraphSelection).toHaveBeenCalledTimes(1);
  });

  it("moves the viewport for a primary touch drag that begins on the pane", () => {
    renderViewport();
    const pane = screen.getByTestId("mock-react-flow-pane");

    fireEvent.pointerDown(pane, {
      clientX: 20,
      clientY: 30,
      isPrimary: true,
      pointerId: 7,
      pointerType: "touch",
    });
    fireEvent.pointerMove(pane, {
      clientX: 70,
      clientY: 60,
      isPrimary: true,
      pointerId: 7,
      pointerType: "touch",
    });
    fireEvent.pointerUp(pane, {
      clientX: 70,
      clientY: 60,
      isPrimary: true,
      pointerId: 7,
      pointerType: "touch",
    });

    expect(setViewport).toHaveBeenCalledWith({ x: 50, y: 30, zoom: 1 });
  });

  it("dispatches batch delete from Delete and Backspace when graph selection is deletable", async () => {
    const deleteGraphSelection = vi.fn();

    renderViewport({
      canDeleteGraphSelection: true,
      deleteGraphSelection,
      editorMode: true,
    });

    const viewport = screen.getByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.keyDown(viewport, { key: "Delete" });
    fireEvent.keyDown(viewport, { key: "Backspace" });

    expect(deleteGraphSelection).toHaveBeenCalledTimes(2);
  });

  it("keeps the current-activity graph shell and React Flow canvas flat", () => {
    renderViewport();

    const graphFrame = screen.getByRole("region", {
      name: "Work graph viewport",
    });
    const reactFlow = screen.getByTestId("mock-react-flow");

    expect(graphFrame.className).toContain("shadow-none");
    expect(graphFrame.className).not.toContain("shadow-af-card");
    expect(graphFrame.className).not.toContain("shadow-af-panel");
    expect(reactFlow.className).toContain("shadow-none");
    expect(reactFlow.className).not.toContain("shadow-af-card");
    expect(reactFlow.className).not.toContain("shadow-af-panel");
  });
});

function renderViewport({
  activeTool = null,
  canDeleteGraphSelection = false,
  clearGraphSelection = vi.fn(),
  deleteGraphSelection = vi.fn(),
  edges = [],
  editorMode = false,
  handleGraphSelectionChange = vi.fn(),
  nodes = [],
  onEditorEdgeClick = vi.fn(),
  onEditorNodeClick = vi.fn(),
}: {
  activeTool?: "add" | "connect" | "delete" | null;
  canDeleteGraphSelection?: boolean;
  clearGraphSelection?: () => void;
  deleteGraphSelection?: () => void;
  edges?: Edge[];
  editorMode?: boolean;
  handleGraphSelectionChange?: (params: {
    edges: Edge[];
    nodes: Node[];
  }) => void;
  nodes?: Node[];
  onEditorEdgeClick?: (edgeId: string) => void;
  onEditorNodeClick?: (nodeId: string) => void;
} = {}) {
  const flowContainerRef = { current: null as HTMLElement | null };

  return render(
    <CurrentActivityGraphViewport
      addControls={{}}
      canDeleteGraphSelection={canDeleteGraphSelection}
      clearGraphSelection={clearGraphSelection}
      deleteGraphSelection={deleteGraphSelection}
      editorControls={{
        activeTool,
        canInteract: true,
        discardPendingChanges: vi.fn(),
        isEditing: editorMode,
        selectTool: vi.fn(),
        toggleMode: vi.fn(),
        unavailableClassifierWorkstationName: undefined,
      }}
      edges={edges}
      flowContainerRef={flowContainerRef}
      handleGraphSelectionChange={handleGraphSelectionChange}
      handleNodesChange={vi.fn()}
      hasPendingChanges={false}
      layoutControls={{
        canMoveLayout: false,
        canRedo: false,
        canUndo: false,
        initialFitViewKey: "full-graph",
        initialFitViewOptions: { padding: 0.18 },
        moveNode: vi.fn(),
        moveNodesByDelta: vi.fn(),
        redo: vi.fn(),
        reset: vi.fn(),
        undo: vi.fn(),
        updateViewport: vi.fn(),
      }}
      headingID="test-heading"
      imports={importController}
      nodeTypes={{}}
      nodes={nodes}
      onEditorEdgeClick={onEditorEdgeClick}
      onEditorNodeClick={onEditorNodeClick}
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
