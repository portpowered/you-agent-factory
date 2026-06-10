import { fireEvent, render, screen } from "@testing-library/react";

import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { createDefaultFactoryLayout } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import { projectFactoryValidationTargets } from "../../factory-graph-editor/lib/projection/factory-validation-graph-projection";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";

vi.mock("./react-flow-current-activity-card-viewport", () => ({
  CurrentActivityGraphViewport: () => <div data-testid="graph-viewport" />,
}));

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: fixture mirrors the card view-model surface for this focused notice test.
function createViewModelStub(overrides: Record<string, unknown> = {}) {
  const canonicalLayout = createDefaultFactoryLayout();
  const base = {
    activeTool: null,
    addMenuActions: [],
    addMenuOpen: false,
    addEdgeWaypoint: vi.fn(),
    blockedRemovalReason: null,
    canInteractWithEditor: true,
    canSaveDraft: true,
    connectionNotice: null,
    currentFactoryDefinition: null,
    dirtyStateSummary: {
      layoutDirty: false,
      preferencesDirty: false,
      topologyDirty: true,
    },
    draftState: { hasChanges: true, pendingFactoryDefinition: null },
    edges: [],
    canonicalLayoutViewport: null,
    graphKey: "graph-key",
    graphState: {
      canonicalLayout,
      canonicalLayoutViewport: null,
      displayFactoryDefinition: semanticWorkflowDashboardSnapshot.factory,
      graphLayout: { edges: [], nodes: [] },
    },
    handleDiscardPendingChanges: vi.fn(),
    handleEditorConnect: vi.fn(),
    handleEditorEdgeDelete: vi.fn(),
    handleEditorModeToggle: vi.fn(),
    handleEditorNodeDelete: vi.fn(),
    handleNodesChange: vi.fn(),
    hiddenNodeClasses: new Set(),
    hideShowMenuOpen: false,
    layoutDraftState: {
      canRedoLayout: false,
      canUndoLayout: false,
      layoutDirty: false,
    },
    layoutControls: {
      canMoveLayout: true,
      canRedo: false,
      canUndo: false,
      currentLayout: canonicalLayout,
      initialFitViewKey: "full-graph",
      initialFitViewOptions: { padding: 0.18 },
    },
    moveEdgeWaypoint: vi.fn(),
    moveLayoutNode: vi.fn(),
    moveLayoutNodesByDelta: vi.fn(),
    nodes: [],
    removeEdgeWaypoint: vi.fn(),
    redoLayout: vi.fn(),
    resetLayout: vi.fn(),
    resetPreferences: vi.fn(),
    setActiveTool: vi.fn(),
    setAddMenuOpen: vi.fn(),
    setHideShowMenuOpen: vi.fn(),
    setIsConfirmingSave: vi.fn(),
    setVisibilityPreset: vi.fn(),
    toggleHiddenNodeClass: vi.fn(),
    undoLayout: vi.fn(),
    updateLayoutViewport: vi.fn(),
    visibilityPreset: "all",
    editorMode: true,
    structuralValidation: {
      projection: projectFactoryValidationTargets([]),
      targets: [],
    },
    hasActiveWork: false,
    isStaleDraft: false,
    saveAttemptRevision: 0,
    saveEditableDefinition: {
      error: null,
      isPending: false,
    },
    validationFactoryDefinition: semanticWorkflowDashboardSnapshot.factory,
    validationProjection: projectFactoryValidationTargets([]),
    validationTargets: [],
    ...overrides,
  };
  const saveMutation = base.saveEditableDefinition as {
    error?: unknown;
    isPending?: boolean;
  };

  return {
    ...base,
    editorControls: {
      activeTool: base.activeTool,
      canInteract: base.canInteractWithEditor,
      connectionNotice: base.connectionNotice,
      discardPendingChanges: base.handleDiscardPendingChanges,
      isEditing: base.editorMode,
      selectTool: base.setActiveTool,
      toggleMode: base.handleEditorModeToggle,
      unavailableClassifierWorkstationName: (
        base as { editorUnavailableClassifierWorkstationName?: string }
      ).editorUnavailableClassifierWorkstationName,
      ...((base as { editorControls?: object }).editorControls ?? {}),
    },
    edgeWaypointControls: {
      handleEditorEdgeClick: vi.fn(),
      handleEditorEdgeDoubleClick: vi.fn(),
      handleMoveSelectedEdgeWaypoint: vi.fn(),
      handleRemoveSelectedEdgeWaypoint: vi.fn(),
      selectedEdgeWaypoints: [],
      selectedWaypointEdgeId: null,
      waypointAriaLabel: vi.fn(),
      waypointControls: null,
      ...((base as { edgeWaypointControls?: object }).edgeWaypointControls ??
        {}),
    },
    addControls: {
      actions: base.addMenuActions,
      isMenuOpen: base.addMenuOpen,
      setMenuOpen: base.setAddMenuOpen,
      startAction:
        (base as { handleAddEntityAction?: unknown }).handleAddEntityAction ??
        vi.fn(),
      ...((base as { addControls?: object }).addControls ?? {}),
    },
    saveControls: {
      attemptRevision: base.saveAttemptRevision,
      canSave: base.canSaveDraft,
      feedback:
        (base as { documentSave?: unknown }).documentSave ??
        ({ status: "idle" } as const),
      requestConfirmation:
        (base as { requestSaveConfirmation?: unknown })
          .requestSaveConfirmation ?? vi.fn(),
      ...((base as { saveControls?: object }).saveControls ?? {}),
    },
    removalControls: {
      blockedReason: base.blockedRemovalReason,
      cancel:
        (base as { handleCancelRemoval?: unknown }).handleCancelRemoval ??
        vi.fn(),
      confirm:
        (base as { handleConfirmRemoval?: unknown }).handleConfirmRemoval ??
        vi.fn(),
      deleteEdge: base.handleEditorEdgeDelete,
      deleteNode: base.handleEditorNodeDelete,
      pendingIntent:
        (base as { pendingRemovalIntent?: unknown }).pendingRemovalIntent ??
        null,
      requestSelectionNodeRemoval:
        (base as { handleSelectionNodeDelete?: unknown })
          .handleSelectionNodeDelete ?? vi.fn(),
      ...((base as { removalControls?: object }).removalControls ?? {}),
    },
    visibilityControls: {
      hiddenNodeClasses: base.hiddenNodeClasses,
      isDirty: false,
      isMenuOpen: base.hideShowMenuOpen,
      preset: base.visibilityPreset,
      resetPreferences: base.resetPreferences,
      setMenuOpen: base.setHideShowMenuOpen,
      setPreset: base.setVisibilityPreset,
      toggleHiddenNodeClass: base.toggleHiddenNodeClass,
      ...((base as { visibilityControls?: object }).visibilityControls ?? {}),
    },
    validationControls: {
      draftErrors: [],
      factoryDefinition:
        (base as { validationFactoryDefinition?: unknown })
          .validationFactoryDefinition ??
        semanticWorkflowDashboardSnapshot.factory,
      projection:
        (base as { validationProjection?: unknown }).validationProjection ??
        projectFactoryValidationTargets([]),
      targets:
        (base as { validationTargets?: unknown[] }).validationTargets ?? [],
      ...((base as { validationControls?: object }).validationControls ?? {}),
    },
    status: {
      hasDocumentBackedLayoutDraft: true,
      hasActiveWork: base.hasActiveWork,
      hasLayoutChanges: false,
      hasSharedGraphChanges: true,
      hasTopologyChanges: true,
      isDefinitionLoading: false,
      isStaleDraft: base.isStaleDraft,
      isSaving: saveMutation.isPending ?? false,
      preferencesDirty: false,
      saveBlockedReason:
        (base as { saveBlockedReason?: string | null }).saveBlockedReason ??
        null,
      saveError: saveMutation.error ?? null,
      ...((base as { status?: object }).status ?? {}),
    },
  };
}

describe("CurrentActivityGraphSurface save failure notice", () => {
  it("collapses the floating save failure panel without dismissing the alert", () => {
    const saveError = new CurrentFactoryDefinitionError(
      "The factory definition is invalid.",
      {
        code: "INVALID_FACTORY_DEFINITION",
        status: 400,
        targets: [],
      },
    );

    render(
      <CurrentActivityGraphSurface
        viewModel={
          createViewModelStub({
            blockedRemovalReason: null,
            connectionNotice: null,
            hasActiveWork: false,
            isStaleDraft: false,
            saveAttemptRevision: 1,
            saveEditableDefinition: {
              error: saveError,
              isPending: false,
            },
          }) as never
        }
        imports={{} as never}
        selection={null}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.getByText("Editor alerts")).toBeTruthy();
    expect(screen.getByText("1 issue")).toBeTruthy();
    expect(screen.getByText("Topology save failed")).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "Collapse editor alerts" }),
    );
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.queryByText("Topology save failed")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Expand editor alerts" }),
    );
    expect(screen.getByText("Topology save failed")).toBeTruthy();
  });
});
