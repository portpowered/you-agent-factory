// biome-ignore-all lint/style/noExcessiveLinesPerFile: surface coverage keeps related shared graph scenarios in one fixture file.
import { fireEvent, render, screen } from "@testing-library/react";

import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { createDefaultFactoryLayout } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import { projectFactoryValidationTargets } from "../../factory-graph-editor/lib/projection/factory-validation-graph-projection";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";

vi.mock("./react-flow-current-activity-card-viewport", () => ({
  CurrentActivityGraphViewport: ({
    addControls,
    editorControls,
    onConnect,
    onEditorEdgeClick,
    onEditorNodeClick,
    saveControls,
    saveDisabledReason,
    nodes,
  }: {
    addControls: {
      setMenuOpen: (open: boolean) => void;
      startAction: () => void;
    };
    editorControls: {
      discardPendingChanges: () => void;
      selectTool: (tool: string) => void;
    };
    onConnect: () => void;
    onEditorEdgeClick: () => void;
    onEditorNodeClick: () => void;
    saveControls: { requestConfirmation: () => void };
    saveDisabledReason: string | null;
    nodes: Array<{ id: string; selected?: boolean }>;
  }) => (
    <div
      data-disabled-reason={saveDisabledReason ?? ""}
      data-selected-node-ids={nodes
        .filter((node) => node.selected)
        .map((node) => node.id)
        .join(",")}
      data-testid="graph-viewport"
    >
      <button onClick={saveControls.requestConfirmation} type="button">
        Trigger save confirm
      </button>
      <button onClick={editorControls.discardPendingChanges} type="button">
        Trigger discard
      </button>
      <button onClick={addControls.startAction} type="button">
        Trigger add action
      </button>
      <button
        onClick={() => {
          addControls.setMenuOpen(true);
        }}
        type="button"
      >
        Trigger add menu
      </button>
      <button onClick={onConnect} type="button">
        Trigger connect
      </button>
      <button onClick={onEditorEdgeClick} type="button">
        Trigger edge delete
      </button>
      <button onClick={onEditorNodeClick} type="button">
        Trigger node delete
      </button>
      <button
        onClick={() => {
          editorControls.selectTool("connect");
        }}
        type="button"
      >
        Trigger tool select
      </button>
    </div>
  ),
}));

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: fixture mirrors the editor/view-model surface contract for component tests.
function createEditorStub(overrides: Record<string, unknown> = {}) {
  const base = {
    activeTool: "delete",
    addEdgeWaypoint: vi.fn(),
    moveEdgeWaypoint: vi.fn(),
    removeEdgeWaypoint: vi.fn(),
    addMenuActions: [],
    addMenuOpen: false,
    blockedRemovalReason:
      "Delete the pending route from the connected state instead.",
    canInteractWithEditor: true,
    canSaveDraft: true,
    connectionNotice: "Only workstation-to-work-state routes are supported.",
    currentFactoryDefinition: null,
    dirtyStateSummary: {
      layoutDirty: false,
      preferencesDirty: false,
      topologyDirty: true,
    },
    draftState: {
      hasChanges: true,
      pendingFactoryDefinition: null,
      source: "current-factory",
    },
    layoutDraftState: {
      canRedoLayout: false,
      canUndoLayout: false,
      hasChanges: false,
      layoutDirty: false,
    },
    moveLayoutNode: vi.fn(),
    editorMode: true,
    structuralValidation: {
      projection: {
        handleErrorsByNodeId: new Map(),
        nodeErrorsByNodeId: new Map(),
        workstationMessagesByNodeId: new Map(),
        workStateMessagesByNodeId: new Map(),
        workTypeMessagesByNodeId: new Map(),
      },
      targets: [],
    },
    handleAddEntityAction: vi.fn(),
    handleDiscardPendingChanges: vi.fn(),
    handleEditorModeToggle: vi.fn(),
    handleEditorConnect: vi.fn(),
    handleEditorEdgeDelete: vi.fn(),
    handleEditorNodeDelete: vi.fn(),
    hiddenNodeClasses: new Set(),
    hideShowMenuOpen: false,
    visibilityPreset: "all",
    hasActiveWork: true,
    isStaleDraft: true,
    saveBlockedReason: "Stop active work before saving this draft.",
    saveAttemptRevision: 0,
    saveEditableDefinition: {
      error: null,
      isPending: false,
    },
    requestSaveConfirmation: vi.fn(),
    setActiveTool: vi.fn(),
    setAddMenuOpen: vi.fn(),
    setHideShowMenuOpen: vi.fn(),
    setVisibilityPreset: vi.fn(),
    resetPreferences: vi.fn(),
    toggleHiddenNodeClass: vi.fn(),
  };
  const merged = {
    ...base,
    ...overrides,
  };
  const draftState = merged.draftState as {
    hasChanges?: boolean;
    pendingFactoryDefinition?: unknown;
    source?: string;
  };
  const layoutDraftState = merged.layoutDraftState as {
    canRedoLayout?: boolean;
    canUndoLayout?: boolean;
    layoutDirty?: boolean;
  };
  const hasTopologyChanges = draftState.hasChanges ?? false;
  const hasLayoutChanges = layoutDraftState.layoutDirty ?? false;
  const canonicalLayout = createDefaultFactoryLayout();
  const structuralValidation = merged.structuralValidation as {
    projection?: unknown;
    targets?: readonly unknown[];
  };
  const saveMutation = merged.saveEditableDefinition as {
    error?: unknown;
    isPending?: boolean;
  };

  return {
    ...merged,
    addControls: {
      actions: merged.addMenuActions,
      isMenuOpen: merged.addMenuOpen,
      setMenuOpen: merged.setAddMenuOpen,
      startAction: merged.handleAddEntityAction,
      ...((merged as { addControls?: object }).addControls ?? {}),
    },
    editorControls: {
      activeTool: merged.activeTool,
      canInteract: merged.canInteractWithEditor,
      connect: merged.handleEditorConnect,
      connectionNotice: merged.connectionNotice,
      discardPendingChanges: merged.handleDiscardPendingChanges,
      isEditing: merged.editorMode,
      selectTool: merged.setActiveTool,
      toggleMode: merged.handleEditorModeToggle,
      unavailableClassifierWorkstationName: (
        merged as { editorUnavailableClassifierWorkstationName?: string }
      ).editorUnavailableClassifierWorkstationName,
      ...((merged as { editorControls?: object }).editorControls ?? {}),
    },
    edgeWaypointControls: {
      handleEditorEdgeClick: merged.handleEditorEdgeDelete,
      handleEditorEdgeDoubleClick: vi.fn(),
      handleMoveSelectedEdgeWaypoint: vi.fn(),
      handleRemoveSelectedEdgeWaypoint: vi.fn(),
      selectedEdgeWaypoints: [],
      selectedWaypointEdgeId: null,
      waypointAriaLabel: vi.fn(),
      waypointControls: null,
      ...((merged as { edgeWaypointControls?: object }).edgeWaypointControls ??
        {}),
    },
    visualGroupControls: {
      canEditVisualGroups: true,
      clearSelectedVisualGroup: vi.fn(),
      groupAriaLabel: (group: { id: string; label?: string }) =>
        group.label ?? group.id,
      groups: [],
      handleCreateVisualGroup: vi.fn(),
      handleRenameSelectedGroup: vi.fn(),
      handleSelectVisualGroup: vi.fn(),
      handleSetSelectedGroupColor: vi.fn(),
      selectedGroup: undefined,
      selectedGroupId: null,
      visualGroupControls: null,
      ...((merged as { visualGroupControls?: object }).visualGroupControls ??
        {}),
    },
    graphState: {
      canonicalLayout,
      canonicalLayoutViewport: null,
      displayFactoryDefinition: semanticWorkflowDashboardSnapshot.factory,
      graphLayout: { edges: [], nodes: [] },
      ...((merged as { graphState?: object }).graphState ?? {}),
    },
    saveControls: {
      attemptRevision: merged.saveAttemptRevision,
      canSave: merged.canSaveDraft,
      feedback:
        (merged as { documentSave?: unknown }).documentSave ??
        ({ status: "idle" } as const),
      requestConfirmation: merged.requestSaveConfirmation,
      ...((merged as { saveControls?: object }).saveControls ?? {}),
    },
    layoutControls: {
      canMoveLayout: draftState.source === "current-factory",
      canRedo: layoutDraftState.canRedoLayout ?? false,
      canUndo: layoutDraftState.canUndoLayout ?? false,
      currentLayout: canonicalLayout,
      initialFitViewKey: "full-graph",
      initialFitViewOptions: { padding: 0.18 },
      ...((merged as { layoutControls?: object }).layoutControls ?? {}),
    },
    removalControls: {
      blockedReason: merged.blockedRemovalReason,
      cancel:
        (merged as { handleCancelRemoval?: unknown }).handleCancelRemoval ??
        vi.fn(),
      confirm:
        (merged as { handleConfirmRemoval?: unknown }).handleConfirmRemoval ??
        vi.fn(),
      deleteEdge: merged.handleEditorEdgeDelete,
      deleteNode: merged.handleEditorNodeDelete,
      pendingIntent:
        (merged as { pendingRemovalIntent?: unknown }).pendingRemovalIntent ??
        null,
      requestSelectionNodeRemoval:
        (merged as { handleSelectionNodeDelete?: unknown })
          .handleSelectionNodeDelete ?? vi.fn(),
      ...((merged as { removalControls?: object }).removalControls ?? {}),
    },
    visibilityControls: {
      hiddenNodeClasses: merged.hiddenNodeClasses,
      isDirty: false,
      isMenuOpen: merged.hideShowMenuOpen,
      preset: merged.visibilityPreset,
      resetPreferences: merged.resetPreferences,
      setMenuOpen: merged.setHideShowMenuOpen,
      setPreset: merged.setVisibilityPreset,
      toggleHiddenNodeClass: merged.toggleHiddenNodeClass,
      ...((merged as { visibilityControls?: object }).visibilityControls ?? {}),
    },
    status: {
      hasDocumentBackedLayoutDraft: draftState.source === "current-factory",
      hasActiveWork: merged.hasActiveWork,
      hasLayoutChanges,
      hasSharedGraphChanges: hasTopologyChanges || hasLayoutChanges,
      hasTopologyChanges,
      isDefinitionLoading: false,
      isStaleDraft: merged.isStaleDraft,
      isSaving: saveMutation.isPending ?? false,
      preferencesDirty: false,
      saveBlockedReason:
        (merged as { saveBlockedReason?: string | null }).saveBlockedReason ??
        null,
      saveError: saveMutation.error ?? null,
      ...((merged as { status?: object }).status ?? {}),
    },
    validationControls: {
      draftErrors: [],
      factoryDefinition:
        (merged as { validationFactoryDefinition?: unknown })
          .validationFactoryDefinition ??
        draftState.pendingFactoryDefinition ??
        merged.currentFactoryDefinition ??
        semanticWorkflowDashboardSnapshot.factory,
      projection: structuralValidation.projection,
      targets: structuralValidation.targets ?? [],
      ...((merged as { validationControls?: object }).validationControls ?? {}),
    },
  };
}

function createGraphStub() {
  return {
    canonicalLayoutViewport: null,
    edges: [],
    graphKey: "graph-key",
    handleNodesChange: vi.fn(),
    nodes: [],
  };
}

const importControllerStub = {
  dropState: { status: "idle" },
  onDragEnter: vi.fn(),
  onDragLeave: vi.fn(),
  onDragOver: vi.fn(),
  onDrop: vi.fn(),
} as never;

function createViewModelStub(overrides: Record<string, unknown> = {}) {
  return {
    ...createEditorStub(overrides),
    ...createGraphStub(),
    ...("nodes" in overrides ? { nodes: overrides.nodes } : {}),
  };
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: surface coverage keeps the shared editor chrome fixtures together.
describe("CurrentActivityGraphSurface", () => {
  it("uses the semantic graph viewport for observer states", () => {
    render(
      <CurrentActivityGraphSurface
        viewModel={createViewModelStub({ editorMode: false }) as never}
        imports={importControllerStub}
        selection={null}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByTestId("graph-viewport")).toBeTruthy();
  });

  it("keeps selected semantic graph nodes in the observer viewport", () => {
    const viewModel = createViewModelStub({
      editorMode: false,
      nodes: [{ id: "work-state:story:queued", selected: true }],
    });

    render(
      <CurrentActivityGraphSurface
        viewModel={viewModel as never}
        imports={importControllerStub}
        selection={{ kind: "state-node", placeId: "story:queued" }}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(
      screen.getByTestId("graph-viewport").getAttribute("data-selected-node-ids"),
    ).toBe("work-state:story:queued");
  });

  it("renders the explicit empty state when observer topology is unavailable", () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.factory = undefined;
    snapshot.topology.workstation_node_ids = [];

    render(
      <CurrentActivityGraphSurface
        viewModel={createViewModelStub({ editorMode: false }) as never}
        imports={importControllerStub}
        selection={null}
        snapshot={snapshot}
      />,
    );

    expect(screen.getByText("No workflow topology loaded")).toBeTruthy();
    expect(screen.queryByTestId("graph-viewport")).toBeNull();
  });

  it("hides editor alerts outside edit mode even when notice conditions exist", () => {
    render(
      <CurrentActivityGraphSurface
        viewModel={createViewModelStub({ editorMode: false }) as never}
        imports={importControllerStub}
        selection={null}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.queryByText("Editor alerts")).toBeNull();
    expect(screen.queryByText("Removal blocked")).toBeNull();
    expect(screen.queryByText("Connection blocked")).toBeNull();
    expect(screen.queryByText("Topology edits are blocked")).toBeNull();
    expect(
      screen.queryByText("A newer factory definition is available"),
    ).toBeNull();
    expect(screen.getByTestId("graph-viewport")).toBeTruthy();
  });

  it("renders shared-surface notices and forwards viewport editor actions", () => {
    const viewModel = createViewModelStub();

    render(
      <CurrentActivityGraphSurface
        viewModel={viewModel as never}
        imports={importControllerStub}
        selection={null}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByText("Removal blocked")).toBeTruthy();
    expect(
      screen.getByText(
        "Delete the pending route from the connected state instead.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Connection blocked")).toBeTruthy();
    expect(
      screen.getByText("Only workstation-to-work-state routes are supported."),
    ).toBeTruthy();
    expect(screen.getByText("Topology edits are blocked")).toBeTruthy();
    expect(
      screen.getByText(
        "Save is unavailable while active work is still running in the current factory.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("A newer factory definition is available"),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Refresh or discard the current draft before saving so you do not overwrite a newer topology version.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText("Topology save failed")).toBeNull();
    expect(screen.queryByText("Save failed")).toBeNull();
    expect(screen.queryByText("Topology saved")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger save confirm" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Trigger discard" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger add action" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger add menu" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger connect" }));
    fireEvent.click(
      screen.getByRole("button", { name: "Trigger edge delete" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Trigger node delete" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Trigger tool select" }),
    );

    expect(viewModel.requestSaveConfirmation).toHaveBeenCalledTimes(1);
    expect(viewModel.handleDiscardPendingChanges).toHaveBeenCalledTimes(1);
    expect(viewModel.handleAddEntityAction).toHaveBeenCalledTimes(1);
    expect(viewModel.setAddMenuOpen).toHaveBeenCalledWith(true);
    expect(viewModel.handleEditorConnect).toHaveBeenCalledTimes(1);
    expect(viewModel.removalControls.deleteEdge).toHaveBeenCalledTimes(1);
    expect(viewModel.removalControls.deleteNode).toHaveBeenCalledTimes(1);
    expect(viewModel.setActiveTool).toHaveBeenCalledWith("connect");
  });

  it("renders recoverable layout group warnings separately from blocking validation errors", () => {
    render(
      <CurrentActivityGraphSurface
        viewModel={
          createViewModelStub({
            blockedRemovalReason: null,
            connectionNotice: null,
            draftState: { hasChanges: true },
            hasActiveWork: false,
            isStaleDraft: false,
            structuralValidation: {
              projection: projectFactoryValidationTargets([]),
              targets: [
                {
                  code: "factory.layout.unknownGroupMemberReference",
                  message:
                    'Layout group "review-lane" references unknown graph node "workstation:missing".',
                  severity: "warning",
                  subject: {
                    id: "review-lane",
                    location: "REFERENCE",
                    type: "FACTORY",
                  },
                },
                {
                  code: "factory.workstation.missingFailureRoute",
                  message: 'Workstation "review" must define a failure route.',
                  severity: "error",
                  subject: {
                    id: "review",
                    location: "ON_FAILURE",
                    type: "WORKSTATION",
                  },
                },
              ],
            },
            validationControls: {
              draftErrors: [
                {
                  code: "MISSING_REQUIRED_FIELD",
                  field: "worker",
                  message:
                    'Workstation "review" must assign a worker before saving.',
                  target: {
                    id: "workstation:review",
                    kind: "node",
                  },
                },
              ],
            },
          }) as never
        }
        imports={importControllerStub}
        selection={null}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByText("Recoverable layout warning")).toBeTruthy();
    expect(
      screen.getByText(
        'Layout group "review-lane" references unknown graph node "workstation:missing".',
      ),
    ).toBeTruthy();
    expect(screen.getByText("Factory validation issue")).toBeTruthy();
    expect(
      screen.getByText(
        'Workstation "review" must assign a worker before saving.',
      ),
    ).toBeTruthy();
  });

  it("renders workstation validation messages in the failure notice when a marked workstation is selected", () => {
    const validationTargets = [
      {
        code: "factory.workstation.missingFailureRoute",
        message: 'Workstation "review" must define a failure route.',
        severity: "error" as const,
        subject: {
          id: "review",
          location: "ON_FAILURE" as const,
          type: "WORKSTATION" as const,
        },
      },
    ];
    const validationProjection =
      projectFactoryValidationTargets(validationTargets);

    render(
      <CurrentActivityGraphSurface
        viewModel={
          createViewModelStub({
            blockedRemovalReason: null,
            connectionNotice: null,
            currentFactoryDefinition: semanticWorkflowDashboardSnapshot.factory,
            draftState: { hasChanges: false },
            hasActiveWork: false,
            isStaleDraft: false,
            structuralValidation: {
              projection: validationProjection,
              targets: validationTargets,
            },
          }) as never
        }
        imports={importControllerStub}
        selection={{ kind: "node", nodeId: "review" }}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByText("Factory validation issue")).toBeTruthy();
    expect(
      screen.getByText('Workstation "review" must define a failure route.'),
    ).toBeTruthy();
  });

  it("renders draft validation errors in the failure notice without requiring a selection", () => {
    render(
      <CurrentActivityGraphSurface
        viewModel={
          createViewModelStub({
            blockedRemovalReason: null,
            connectionNotice: null,
            draftState: { hasChanges: true },
            hasActiveWork: false,
            isStaleDraft: false,
            validationControls: {
              draftErrors: [
                {
                  code: "MISSING_REQUIRED_FIELD",
                  field: "worker",
                  message:
                    'Workstation "review" must assign a worker before saving.',
                  target: {
                    id: "workstation:review",
                    kind: "node",
                  },
                },
              ],
            },
          }) as never
        }
        imports={importControllerStub}
        selection={null}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByText("Factory validation issue")).toBeTruthy();
    expect(
      screen.getByText(
        'Workstation "review" must assign a worker before saving.',
      ),
    ).toBeTruthy();
  });

  it("renders work type validation messages in the failure notice when a marked work type is selected", () => {
    const validationTargets = [
      {
        code: "factory.workType.missingCompletionState",
        message: 'work type "story" must declare a completion state.',
        severity: "error" as const,
        subject: {
          id: "story",
          location: "STATES" as const,
          type: "WORK_TYPE" as const,
        },
      },
    ];
    const validationProjection =
      projectFactoryValidationTargets(validationTargets);

    render(
      <CurrentActivityGraphSurface
        viewModel={
          createViewModelStub({
            blockedRemovalReason: null,
            connectionNotice: null,
            currentFactoryDefinition: semanticWorkflowDashboardSnapshot.factory,
            draftState: { hasChanges: false },
            hasActiveWork: false,
            isStaleDraft: false,
            structuralValidation: {
              projection: validationProjection,
              targets: validationTargets,
            },
          }) as never
        }
        imports={importControllerStub}
        selection={{ kind: "node", nodeId: "work-type:story" }}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByText("Factory validation issue")).toBeTruthy();
    expect(
      screen.getByText('work type "story" must declare a completion state.'),
    ).toBeTruthy();
  });

  it("renders work state validation messages in the failure notice when a marked work state is selected", () => {
    const validationTargets = [
      {
        code: "factory.workState.missingTerminalCompletionPath",
        message: 'work state "story:queued" has no terminal completion path.',
        severity: "error" as const,
        subject: {
          id: "story:queued",
          location: "TERMINAL" as const,
          type: "WORK_STATE" as const,
        },
      },
    ];
    const validationProjection =
      projectFactoryValidationTargets(validationTargets);

    render(
      <CurrentActivityGraphSurface
        viewModel={
          createViewModelStub({
            blockedRemovalReason: null,
            connectionNotice: null,
            currentFactoryDefinition: semanticWorkflowDashboardSnapshot.factory,
            draftState: { hasChanges: false },
            hasActiveWork: false,
            isStaleDraft: false,
            structuralValidation: {
              projection: validationProjection,
              targets: validationTargets,
            },
          }) as never
        }
        imports={importControllerStub}
        selection={{ kind: "state-node", placeId: "story:queued" }}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByText("Factory validation issue")).toBeTruthy();
    expect(
      screen.getByText(
        'work state "story:queued" has no terminal completion path.',
      ),
    ).toBeTruthy();
  });

  it("renders save rejection messages for referenced-entity removals in the save failure notice", () => {
    const saveError = new CurrentFactoryDefinitionError(
      "The factory definition is invalid.",
      {
        code: "INVALID_FACTORY_DEFINITION",
        status: 400,
        targets: [
          {
            code: "factory.route.danglingPlaceReference",
            message:
              'Workstation "process" references missing place "story:missing-state".',
            severity: "error",
            subject: {
              id: "process",
              location: "OUTPUTS",
              type: "WORKSTATION",
            },
          },
        ],
      },
    );

    render(
      <CurrentActivityGraphSurface
        viewModel={
          createViewModelStub({
            blockedRemovalReason: null,
            connectionNotice: null,
            currentFactoryDefinition: semanticWorkflowDashboardSnapshot.factory,
            draftState: { hasChanges: true },
            hasActiveWork: false,
            isStaleDraft: false,
            saveEditableDefinition: {
              error: saveError,
              isPending: false,
            },
            structuralValidation: {
              projection: projectFactoryValidationTargets([]),
              targets: [],
            },
          }) as never
        }
        imports={importControllerStub}
        selection={{ kind: "node", nodeId: "workstation:process" }}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByText("Topology save failed")).toBeTruthy();
    expect(screen.getByText("The factory definition is invalid.")).toBeTruthy();
    expect(screen.getByText("Factory validation issue")).toBeTruthy();
    expect(
      screen.getAllByText(
        'Workstation "process" references missing place "story:missing-state".',
      ).length,
    ).toBeGreaterThanOrEqual(1);
  });

  it("does not render save success inside the graph card", () => {
    render(
      <CurrentActivityGraphSurface
        viewModel={
          createViewModelStub({
            blockedRemovalReason: null,
            connectionNotice: null,
            draftState: { hasChanges: false },
            hasActiveWork: false,
            isStaleDraft: false,
            saveEditableDefinition: {
              error: null,
              isPending: false,
            },
          }) as never
        }
        imports={importControllerStub}
        selection={null}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.queryByText("Topology saved")).toBeNull();
    expect(
      screen.queryByText(
        "The draft has been cleared and the graph is waiting for the latest factory-change event refresh.",
      ),
    ).toBeNull();
  });
});
