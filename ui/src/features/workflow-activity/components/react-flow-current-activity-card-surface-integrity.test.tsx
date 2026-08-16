// biome-ignore-all lint/style/noExcessiveLinesPerFile: the integrated renderer-boundary fixture keeps the real card and React Flow adapter together.
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";

import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { createDefaultFactoryLayout } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import { projectFactoryValidationTargets } from "../../factory-graph-editor/lib/projection/factory-validation-graph-projection";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";

const {
  reactFlowErrorCallbackCount,
  reactFlowErrorToReport,
  reactFlowRenderCount,
} = vi.hoisted(() => ({
  reactFlowErrorCallbackCount: { current: 0 },
  reactFlowErrorToReport: {
    current: { errorId: "008", message: "missing source handle" } as {
      errorId: string;
      message: string;
    } | null,
  },
  reactFlowRenderCount: { current: 0 },
}));

vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual("@xyflow/react");
  const { ReactFlowProvider } = actual;

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
      reactFlowRenderCount.current += 1;
      const error = reactFlowErrorToReport.current;
      if (error) {
        reactFlowErrorCallbackCount.current += 1;
        onError?.(error.errorId, error.message);
      }

      return (
        <ReactFlowProvider>
          <div data-testid="mock-react-flow">{children}</div>
        </ReactFlowProvider>
      );
    },
  };
});

afterEach(() => {
  cleanup();
  reactFlowErrorCallbackCount.current = 0;
  reactFlowErrorToReport.current = {
    errorId: "008",
    message: "missing source handle",
  };
  reactFlowRenderCount.current = 0;
});

it("contains an integrity React Flow onError beneath the card and retries only the graph", async () => {
  const viewModel = createSurfaceViewModel();

  render(
    <div>
      <CurrentActivityGraphSurface
        headingID="integrity-test-heading"
        locale="en"
        viewModel={viewModel as never}
        imports={createImportController()}
        selection={null}
        snapshot={semanticWorkflowDashboardSnapshot}
      />
      <aside data-testid="surrounding-dashboard">
        Dashboard remains mounted
      </aside>
    </div>,
  );

  expect(reactFlowErrorCallbackCount.current).toBeGreaterThanOrEqual(1);
  expect(
    screen.getByRole("heading", { name: "Graph rendering needs recovery" }),
  ).toBeTruthy();
  expect(
    screen.getByText(
      "This graph could not be rendered safely. The rest of the dashboard is still available.",
    ),
  ).toBeTruthy();
  expect(screen.getByTestId("surrounding-dashboard")).toBeTruthy();

  const failedRenderCount = reactFlowRenderCount.current;
  reactFlowErrorToReport.current = null;
  fireEvent.click(screen.getByRole("button", { name: "Retry graph" }));

  await waitFor(() => {
    expect(screen.getByTestId("mock-react-flow")).toBeTruthy();
  });
  expect(reactFlowRenderCount.current).toBeGreaterThan(failedRenderCount);
  expect(screen.getByTestId("surrounding-dashboard")).toBeTruthy();
  expect(
    screen.queryByRole("heading", { name: "Graph rendering needs recovery" }),
  ).toBeNull();
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: this fixture supplies the real surface's nested controller seams so the test crosses the card boundary.
function createSurfaceViewModel() {
  const noop = vi.fn();
  const canonicalLayout = createDefaultFactoryLayout();
  const emptyValidationProjection = projectFactoryValidationTargets([]);

  return {
    addControls: {
      actions: [],
      isMenuOpen: false,
      setMenuOpen: noop,
      startAction: noop,
      updatePlacementViewport: noop,
    },
    canDeleteGraphSelection: false,
    clearGraphSelection: noop,
    deleteGraphSelection: noop,
    edges: [],
    edgeWaypointControls: {
      handleEditorEdgeClick: noop,
      handleEditorEdgeDoubleClick: noop,
      handleMoveSelectedEdgeWaypoint: noop,
      handleRemoveSelectedEdgeWaypoint: noop,
      selectedEdgeWaypoints: [],
      selectedWaypointEdgeId: null,
      waypointAriaLabel: (index: number) => `Waypoint ${index + 1}`,
      waypointControls: null,
    },
    editorControls: {
      activeTool: null,
      canInteract: false,
      connect: noop,
      connectionNotice: null,
      discardPendingChanges: noop,
      isEditing: false,
      selectTool: noop,
      toggleMode: noop,
    },
    graphSelectionToolbarState: undefined,
    handleEdgesChange: noop,
    handleGraphSelectionChange: noop,
    handleGraphSelectionStart: noop,
    handleNodesChange: noop,
    layoutControls: {
      canMoveLayout: false,
      canRedo: false,
      canUndo: false,
      canonicalViewport: null,
      initialFitViewKey: "integrity-test",
      initialFitViewOptions: { padding: 0.18 },
      moveNode: noop,
      moveNodesByDelta: noop,
      redo: noop,
      reset: noop,
      undo: noop,
      updateViewport: noop,
    },
    nodes: [],
    removalControls: {
      blockedReason: null,
      deleteEdge: noop,
      deleteNode: noop,
    },
    saveControls: {
      canSave: false,
      requestConfirmation: noop,
    },
    status: {
      dirtyStateSummary: {
        layoutDirty: false,
        preferencesDirty: false,
        topologyDirty: false,
      },
      hasActiveWork: false,
      hasDocumentBackedLayoutDraft: true,
      hasLayoutChanges: false,
      hasSharedGraphChanges: false,
      hasTopologyChanges: false,
      isDefinitionLoading: false,
      isSaving: false,
      isStaleDraft: false,
      loadErrorMessage: undefined,
      preferencesDirty: false,
      saveBlockedReason: null,
      saveError: null,
    },
    validationControls: {
      draftErrors: [],
      factoryDefinition: semanticWorkflowDashboardSnapshot.factory,
      projection: emptyValidationProjection,
      targets: [],
    },
    visibilityControls: {
      hiddenNodeClasses: new Set(),
      isDirty: false,
      isMenuOpen: false,
      preset: "all",
      resetPreferences: noop,
      setMenuOpen: noop,
      setPreset: noop,
      toggleHiddenNodeClass: noop,
    },
    visualGroupControls: {
      canEditVisualGroups: false,
      clearSelectedVisualGroup: noop,
      groupAriaLabel: (group: { id: string; label?: string }) =>
        group.label ?? group.id,
      groupOutlineAriaLabel: (
        group: { id: string },
        edge: "top" | "right" | "bottom" | "left",
      ) => `${group.id} ${edge}`,
      groups: [],
      handleCreateVisualGroup: noop,
      handleMoveVisualGroup: noop,
      handleResizeVisualGroup: noop,
      handleSelectVisualGroup: noop,
      resizeHandleAriaLabel: (corner: string) => `Resize from ${corner}`,
      selectedGroupId: null,
      visualGroupControls: null,
    },
    graphState: {
      canonicalLayout,
      canonicalLayoutViewport: null,
      displayFactoryDefinition: semanticWorkflowDashboardSnapshot.factory,
      graphLayout: { edges: [], nodes: [] },
    },
  };
}

function createImportController() {
  return {
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
}
