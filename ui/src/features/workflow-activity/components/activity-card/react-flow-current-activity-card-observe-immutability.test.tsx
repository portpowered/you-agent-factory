// @component-test-runner vitest: imports workspace graph packages that Bun resolves through declaration files.
import { fireEvent, render, screen } from "@testing-library/react";
import type {
  Connection,
  Edge,
  Node,
  NodeChange,
  ReactFlowInstance,
} from "@xyflow/react";
import { type ReactNode, useEffect, useRef } from "react";

import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import {
  createEditableFactoryGraphHookWrapper,
  setupEditableFactoryGraphSaveTestEnvironment,
} from "../../../../testing/editable-factory-graph-hook-test-helpers";
import { useFactoryGraphEdgeWaypointEditor } from "../../../factory-graph-editor/hooks/layout/factory-graph-edge-waypoint-editor-hook";
import { useFactoryGraphVisualGroupEditor } from "../../../factory-graph-editor/hooks/layout/factory-graph-visual-group-editor-hook";
import { useEditableFactoryGraph } from "../../../factory-graph-editor/hooks/use-editable-factory-graph";
import type { CurrentActivityImportController } from "../../hooks/current-activity-import-controller";
import { CurrentActivityGraphViewport } from "../react-flow-current-activity-card-viewport";

const OBSERVE_EDGE_ID =
  "workstation-output:workstation:document-only->work-state:story:done";
const OBSERVE_GROUP_ID = "group-1";
const OBSERVE_NODE_ID = "workstation:document-only";

type MockReactFlowProps = {
  children: ReactNode;
  defaultViewport?: { x: number; y: number; zoom: number };
  edges?: Edge[];
  fitView?: boolean;
  nodes?: Node[];
  nodesDraggable?: boolean;
  onConnect?: (connection: Connection) => void;
  onEdgeClick?: (_event: unknown, edge: Edge) => void;
  onEdgeDoubleClick?: (
    _event: { clientX: number; clientY: number },
    edge: Edge,
  ) => void;
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
  onNodesChange?: (changes: NodeChange[]) => void;
};

vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual("@xyflow/react");
  const { ReactFlowProvider } = actual;

  return {
    ...actual,
    Background: () => <div data-testid="graph-background" />,
    Controls: () => <div data-testid="graph-controls" />,
    ReactFlow: ({
      children,
      defaultViewport,
      edges,
      fitView,
      nodes,
      nodesDraggable,
      onConnect,
      onEdgeClick,
      onEdgeDoubleClick,
      onInit,
      onMoveEnd,
      onNodeDragStart,
      onNodeDragStop,
      onNodesChange,
    }: MockReactFlowProps) => {
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

      const firstNode = nodes?.[0];
      const firstEdge = edges?.[0];

      return (
        <ReactFlowProvider>
          <div
            data-default-viewport={JSON.stringify(defaultViewport ?? null)}
            data-fit-view={String(fitView ?? false)}
            data-node-position={JSON.stringify(firstNode?.position ?? null)}
            data-nodes-draggable={String(nodesDraggable ?? false)}
            data-testid="mock-react-flow"
          >
            <button
              onClick={() => onMoveEnd?.(null, { x: 12, y: 34, zoom: 1.25 })}
              type="button"
            >
              pan-viewport
            </button>
            <button
              onClick={() =>
                onConnect?.({
                  source: OBSERVE_NODE_ID,
                  sourceHandle: "output:done",
                  target: "work-state:story:done",
                  targetHandle: "input:done",
                })
              }
              type="button"
            >
              connect-nodes
            </button>
            <button
              onClick={() => firstEdge && onEdgeClick?.(null, firstEdge)}
              type="button"
            >
              select-edge
            </button>
            <button
              onClick={() =>
                firstEdge &&
                onEdgeDoubleClick?.({ clientX: 120, clientY: 160 }, firstEdge)
              }
              type="button"
            >
              double-click-edge
            </button>
            <button
              onClick={() => {
                if (!firstNode) {
                  return;
                }

                onNodeDragStart?.(null, firstNode, nodes ?? []);
                onNodeDragStop?.(
                  null,
                  {
                    ...firstNode,
                    position: {
                      x: firstNode.position.x + 16,
                      y: firstNode.position.y + 8,
                    },
                  },
                  nodes ?? [],
                );
              }}
              type="button"
            >
              drag-selected-nodes
            </button>
            <button
              onClick={() => {
                if (!firstNode) {
                  return;
                }

                onNodesChange?.([
                  {
                    id: firstNode.id,
                    position: {
                      x: firstNode.position.x + 24,
                      y: firstNode.position.y + 12,
                    },
                    type: "position",
                  },
                ]);
              }}
              type="button"
            >
              position-node
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

const observeFactoryDocument: CurrentFactoryDocument = {
  layout: {
    edges: [{ id: OBSERVE_EDGE_ID, waypoints: [{ x: 180, y: 220 }] }],
    groups: [
      {
        bounds: { height: 300, width: 420, x: 10, y: 20 },
        id: OBSERVE_GROUP_ID,
        label: "Baseline group",
        nodeIds: [OBSERVE_NODE_ID],
      },
    ],
    nodes: [{ id: OBSERVE_NODE_ID, position: { x: 40, y: 60 } }],
    schemaVersion: 1,
    viewport: { x: 4, y: 8, zoom: 1 },
  },
  name: "Observe Factory",
  version: { logical: "9", physical: "2026-05-31T01:00:00Z" },
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workers: [{ model: "gpt-5", name: "writer", type: "MODEL_WORKER" }],
  workstations: [
    {
      body: "Document plane baseline.",
      inputs: [{ state: "queued", workType: "story" }],
      name: "document-only",
      outputs: [{ state: "done", workType: "story" }],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

it("keeps the real document controller immutable across observe-only edit paths", () => {
  setupEditableFactoryGraphSaveTestEnvironment();
  const { EditableFactoryGraphHookWrapper } =
    createEditableFactoryGraphHookWrapper();

  render(<ObserveModeDocumentControllerHarness />, {
    wrapper: EditableFactoryGraphHookWrapper,
  });

  expect(
    screen.getByTestId("mock-react-flow").getAttribute("data-nodes-draggable"),
  ).toBe("false");
  const before = readObserveControllerState();

  for (const label of [
    "connect-nodes",
    "select-edge",
    "double-click-edge",
    "drag-selected-nodes",
    "position-node",
    "pan-viewport",
    "attempt-observe-edit-paths",
  ]) {
    fireEvent.click(screen.getByRole("button", { name: label }));
  }

  expect(readObserveControllerState()).toEqual(before);
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: the harness intentionally mounts the real document, projection, and interaction controllers together.
function ObserveModeDocumentControllerHarness() {
  const graph = useEditableFactoryGraph({
    currentFactoryDocument: observeFactoryDocument,
    factoryDocumentScopeKey: "observe-controller",
  });
  const flowContainerRef = useRef<HTMLElement | null>(null);
  const flowInstanceRef = useRef<ReactFlowInstance | null>(null);
  const projectedNodes = graph.projection.nodes.map((node) => ({
    id: node.id,
    position: node.position,
  }));
  const nodeGeometryById = new Map(
    graph.projection.nodes.map((node) => [
      node.id,
      {
        height: node.height ?? 120,
        position: node.position,
        width: node.width ?? 220,
      },
    ]),
  );
  const waypointEditor = useFactoryGraphEdgeWaypointEditor({
    activeTool: null,
    addEdgeWaypoint: graph.actions.addEdgeWaypoint,
    canInteractWithEditor: true,
    editorMode: false,
    handleEditorEdgeDelete: () => undefined,
    layout: graph.layoutDraftState.layout,
    locale: "en",
    moveEdgeWaypoint: graph.actions.moveEdgeWaypoint,
    nodes: projectedNodes,
    removeEdgeWaypoint: graph.actions.removeEdgeWaypoint,
  });
  const visualGroupEditor = useFactoryGraphVisualGroupEditor({
    activeTool: null,
    addNodeToVisualGroup: graph.actions.addNodeToVisualGroup,
    canInteractWithEditor: true,
    canvasNodeOptions: [{ id: OBSERVE_NODE_ID, label: "Document only" }],
    createVisualGroup: graph.actions.createVisualGroup,
    deleteVisualGroup: graph.actions.deleteVisualGroup,
    editorMode: false,
    fitVisualGroup: graph.actions.fitVisualGroup,
    layout: graph.layoutDraftState.layout,
    locale: "en",
    moveVisualGroupByDelta: graph.actions.moveVisualGroupByDelta,
    nodeGeometryById,
    removeNodeFromVisualGroup: graph.actions.removeNodeFromVisualGroup,
    renameVisualGroup: graph.actions.renameVisualGroup,
    resizeVisualGroup: graph.actions.resizeVisualGroup,
    resolveViewportCenter: () => ({ x: 200, y: 200 }),
    selectedNodeIds: [OBSERVE_NODE_ID],
    setVisualGroupColor: graph.actions.setVisualGroupColor,
  });

  return (
    <>
      <CurrentActivityGraphViewport
        addControls={{ updatePlacementViewport: () => undefined }}
        editorControls={{
          activeTool: null,
          canInteract: true,
          discardPendingChanges: () => undefined,
          isEditing: false,
          selectTool: () => undefined,
          toggleMode: () => undefined,
        }}
        edges={graph.projection.edges}
        flowContainerRef={flowContainerRef}
        flowInstanceRef={flowInstanceRef}
        handleEdgesChange={() => undefined}
        handleNodesChange={(changes) => {
          for (const change of changes) {
            if (change.type === "position" && change.position) {
              graph.actions.moveLayoutNode(change.id, change.position);
            }
          }
        }}
        hasPendingChanges={graph.pendingState.hasChanges}
        headingID="observe-controller-heading"
        imports={importController}
        layoutControls={{
          canMoveLayout: true,
          canRedo: graph.pendingState.canRedoLayout,
          canUndo: graph.pendingState.canUndoLayout,
          canonicalViewport: graph.layoutDraftState.layout.viewport ?? null,
          initialFitViewKey: "observe-controller",
          initialFitViewOptions: { padding: 0.18 },
          moveNode: graph.actions.moveLayoutNode,
          moveNodesByDelta: graph.actions.moveNodesByDelta,
          redo: graph.actions.redoLayout,
          reset: graph.actions.resetLayout,
          undo: graph.actions.undoLayout,
          updateViewport: graph.actions.updateLayoutViewport,
        }}
        nodeTypes={{}}
        nodes={graph.projection.nodes}
        onConnect={() => {
          graph.actions.connectNodes({
            sourceAnchorId: "output:done",
            sourceNodeId: OBSERVE_NODE_ID,
            targetAnchorId: "input:done",
            targetNodeId: "work-state:story:done",
          });
        }}
        onEditorEdgeClick={waypointEditor.handleEditorEdgeClick}
        onEditorEdgeDoubleClick={waypointEditor.handleEditorEdgeDoubleClick}
        onMoveEdgeWaypoint={waypointEditor.handleMoveSelectedEdgeWaypoint}
        onRemoveEdgeWaypoint={waypointEditor.handleRemoveSelectedEdgeWaypoint}
        onMoveVisualGroup={visualGroupEditor.handleMoveVisualGroup}
        onResizeVisualGroup={visualGroupEditor.handleResizeVisualGroup}
        onSelectVisualGroup={visualGroupEditor.handleSelectVisualGroup}
        saveControls={{
          canSave: graph.saveState.canSave,
          requestConfirmation: () => undefined,
        }}
        selectedEdgeWaypoints={waypointEditor.selectedEdgeWaypoints}
        selectedWaypointEdgeId={waypointEditor.selectedWaypointEdgeId}
        visualGroupAriaLabel={visualGroupEditor.groupAriaLabel}
        visualGroupCanEdit={visualGroupEditor.canEditVisualGroups}
        visualGroups={[]}
        waypointAriaLabel={waypointEditor.waypointAriaLabel}
        viewportMeasurement={DEFAULT_VIEWPORT_MEASUREMENT}
        visibilityControls={{
          hiddenNodeClasses: new Set(),
          isDirty: false,
          isMenuOpen: false,
          preset: "all",
          resetPreferences: () => undefined,
          setMenuOpen: () => undefined,
          setPreset: () => undefined,
          toggleHiddenNodeClass: () => undefined,
        }}
      />
      <button
        onClick={() => {
          waypointEditor.handleEditorEdgeClick(OBSERVE_EDGE_ID);
          waypointEditor.handleEditorEdgeDoubleClick(OBSERVE_EDGE_ID, {
            x: 240,
            y: 280,
          });
          waypointEditor.handleAddSelectedEdgeWaypoint({ x: 240, y: 280 });
          waypointEditor.handleMoveSelectedEdgeWaypoint(OBSERVE_EDGE_ID, 0, {
            x: 260,
            y: 300,
          });
          waypointEditor.handleRemoveSelectedEdgeWaypoint(OBSERVE_EDGE_ID, 0);
          visualGroupEditor.handleCreateVisualGroup();
          visualGroupEditor.handleSelectVisualGroup(OBSERVE_GROUP_ID);
          visualGroupEditor.handleFitSelectedGroup();
          visualGroupEditor.handleRenameSelectedGroup("Changed");
          visualGroupEditor.handleSetSelectedGroupColor("info");
          visualGroupEditor.visualGroupControls?.onToggleNodeMembership(
            OBSERVE_NODE_ID,
            false,
          );
          visualGroupEditor.handleMoveVisualGroup(
            OBSERVE_GROUP_ID,
            { x: 40, y: 50 },
            new Map([[OBSERVE_NODE_ID, { x: 40, y: 60 }]]),
          );
          visualGroupEditor.handleResizeVisualGroup(OBSERVE_GROUP_ID, {
            height: 500,
            width: 600,
            x: 20,
            y: 30,
          });
          visualGroupEditor.handleDeleteSelectedGroup();
        }}
        type="button"
      >
        attempt-observe-edit-paths
      </button>
      <output data-testid="observe-controller-state">
        {JSON.stringify({
          canonicalDocument: graph.pendingState.pendingFactoryDefinition,
          dirty: graph.pendingState.dirtyState,
          draft: graph.draftState.draft,
          hasChanges: graph.pendingState.hasChanges,
          history: {
            canRedo: graph.pendingState.canRedoLayout,
            canUndo: graph.pendingState.canUndoLayout,
          },
          layout: graph.layoutDraftState.layout,
          projection: {
            edges: graph.projection.edges.map((edge) => ({
              id: edge.id,
              waypoints: (edge.data as { waypoints?: unknown } | undefined)
                ?.waypoints,
            })),
            nodes: graph.projection.nodes.map((node) => ({
              id: node.id,
              position: node.position,
            })),
          },
          save: {
            canSave: graph.saveState.canSave,
            status: graph.saveState.documentSave.status,
          },
        })}
      </output>
    </>
  );
}

function readObserveControllerState() {
  return JSON.parse(
    screen.getByTestId("observe-controller-state").textContent ?? "null",
  );
}
