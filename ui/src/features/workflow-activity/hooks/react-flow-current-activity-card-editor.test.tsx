// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction lint/style/noExcessiveLinesPerFile: focused hook coverage keeps session-reset and in-session save/leave branches on one harness seam.
import { act, renderHook } from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { createEmptyFactoryGraphDraft } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  readGraphDraftHasPendingChanges,
  useFactoryGraphTopologyEditorBridge,
} from "../state/factory-graph-topology-editor-bridge";
import { useCurrentActivityGraphState } from "./use-current-activity-graph-state";

const fixtureState = vi.hoisted(() => {
  const emptyDraft = () => ({
    additions: {
      docs: [],
      resources: [],
      workers: [],
      workStates: [],
      workTypes: [],
      workstations: [],
    },
    edgeChanges: {
      additions: [],
      removals: [],
    },
    removals: {
      docs: [],
      resources: [],
      workers: [],
      workStates: [],
      workTypes: [],
      workstations: [],
    },
  });
  const baseFactoryDefinition: CanonicalFactoryDefinition = {
    name: "Current Factory",
    resources: [],
    workers: [],
    workTypes: [
      {
        name: "story",
        states: [
          {
            name: "queued",
            type: "INITIAL",
          },
        ],
      },
    ],
    workstations: [
      {
        inputs: [],
        name: "review",
        outputs: [],
        type: "MODEL_WORKSTATION",
      },
    ],
  };

  return {
    baseFactoryDefinition,
    editableDocument: {
      ...baseFactoryDefinition,
      version: {
        logical: "4",
        physical: "2026-05-25T00:00:00Z",
      },
    },
    emptyDraft,
  };
});

const hookState = vi.hoisted(() => ({
  addEntityController: {
    reset: vi.fn(),
  },
  connectionController: {
    blockedRemovalReason: null,
    connectionNotice: null as string | null,
    handleConnectionAnchorClick: vi.fn(),
    handleEditorConnect: vi.fn(),
    pendingConnectionSource: null,
    setBlockedRemovalReason: vi.fn(),
    setConnectionNotice: vi.fn(),
  },
  currentFactoryQuery: {
    data: fixtureState.editableDocument,
    status: "success" as const,
  },
  draftState: {
    baseDocument: fixtureState.editableDocument,
    draft: fixtureState.emptyDraft(),
    hasChanges: false,
    latestDocument: fixtureState.editableDocument,
    pendingFactoryDefinition: fixtureState.baseFactoryDefinition,
    replaceDraft: vi.fn(),
    resetDraft: vi.fn(),
    validationErrors: [],
  },
  removalController: {
    handleCancelRemoval: vi.fn(),
    handleConfirmRemoval: vi.fn(),
    handleEditorEdgeDelete: vi.fn(),
    handleEditorNodeDelete: vi.fn(),
    pendingRemovalIntent: null,
    setPendingRemovalEdgeId: vi.fn(),
    setPendingRemovalNodeId: vi.fn(),
  },
  saveEditableDefinition: {
    error: null,
    isPending: false,
    reset: vi.fn(),
    saveAsync: vi.fn(async () => undefined),
  },
  unsupportedFromDefinition: undefined as string | undefined,
  documentSave: { status: "idle" as const },
  saveStateIsStale: false,
  layoutDirty: false,
  preferencesDirty: false,
  topologyDirty: false,
  layoutDraftState: {
    hasChanges: false,
    layoutDirty: false,
  },
}));

vi.mock(
  "../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  () => ({
    useCurrentFactoryDocument: vi.fn(() => hookState.currentFactoryQuery),
  }),
);

vi.mock(
  "../../current-factory-definition/hooks/useFactoryDocumentSave",
  () => ({
    useFactoryDocumentSave: () => hookState.saveEditableDefinition,
  }),
);

vi.mock("../../factory-graph-editor/hooks/use-editable-factory-graph", () => ({
  useEditableFactoryGraph: () => ({
    actions: {
      discard: hookState.draftState.resetDraft,
      resetLayout: vi.fn(),
      save: async () => {
        const factory =
          hookState.draftState.pendingFactoryDefinition ??
          hookState.draftState.latestDocument;
        if (!factory) {
          return false;
        }
        await hookState.saveEditableDefinition.saveAsync({
          baseVersion: hookState.draftState.latestDocument?.version,
          factory,
        });
        hookState.draftState.replaceDraft(createEmptyFactoryGraphDraft());
        return true;
      },
      updateLayoutViewport: vi.fn(),
    },
    documentSaveControls: {
      beginConfirmation: vi.fn(() => {
        hookState.documentSave = { status: "confirming" };
      }),
      cancelConfirmation: vi.fn(() => {
        hookState.documentSave = { status: "idle" };
      }),
      clearSaveFeedback: vi.fn(() => {
        hookState.documentSave = { status: "idle" };
      }),
    },
    draftState: hookState.draftState,
    layoutDraftState: hookState.layoutDraftState,
    pendingState: {
      dirtyState: {
        layoutDirty:
          hookState.layoutDraftState.layoutDirty || hookState.layoutDirty,
        preferencesDirty: hookState.preferencesDirty,
        topologyDirty:
          hookState.draftState.hasChanges || hookState.topologyDirty,
      },
      hasChanges:
        hookState.layoutDraftState.layoutDirty ||
        hookState.layoutDirty ||
        hookState.draftState.hasChanges ||
        hookState.topologyDirty,
      hasLayoutChanges:
        hookState.layoutDraftState.layoutDirty || hookState.layoutDirty,
      hasPortableDocumentChanges:
        hookState.layoutDraftState.layoutDirty ||
        hookState.layoutDirty ||
        hookState.draftState.hasChanges ||
        hookState.topologyDirty,
      hasPreferenceChanges: hookState.preferencesDirty,
      hasTopologyChanges:
        hookState.draftState.hasChanges || hookState.topologyDirty,
      layoutDirty:
        hookState.layoutDraftState.layoutDirty || hookState.layoutDirty,
      pendingFactoryDefinition: hookState.draftState.pendingFactoryDefinition,
      preferencesDirty: hookState.preferencesDirty,
      topologyDirty: hookState.draftState.hasChanges || hookState.topologyDirty,
    },
    saveMutation: {
      error: hookState.saveEditableDefinition.error,
      isPending: hookState.saveEditableDefinition.isPending,
      reset: hookState.saveEditableDefinition.reset,
    },
    saveState: {
      canSave:
        (hookState.layoutDraftState.layoutDirty ||
          hookState.layoutDirty ||
          hookState.draftState.hasChanges ||
          hookState.topologyDirty) &&
        hookState.draftState.latestDocument !== null &&
        hookState.draftState.validationErrors.length === 0 &&
        !hookState.saveStateIsStale,
      documentSave: hookState.saveStateIsStale
        ? {
            message:
              "The factory definition changed while you were editing. Refresh or discard your draft before saving.",
            status: "warning",
          }
        : hookState.documentSave,
      isStale: hookState.saveStateIsStale,
    },
  }),
}));

vi.mock(
  "../../factory-graph-editor/lib/editor/factory-graph-editor-additions",
  () => ({
    buildFactoryGraphAddEntityMenuActions: () => [],
  }),
);

vi.mock(
  "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-save-summary",
  () => ({
    buildFactoryGraphSaveSummary: () => ({
      additions: [],
      removals: [],
    }),
  }),
);

vi.mock("./use-current-activity-graph-add-controller", () => ({
  useFactoryGraphAddEntityController: () => hookState.addEntityController,
}));

vi.mock("./factory-graph-editor-availability", () => ({
  findClassifierGraphEditorUnsupportedWorkstationName: () =>
    hookState.unsupportedFromDefinition,
}));

vi.mock("./react-flow-current-activity-card-editor-connections", () => ({
  useFactoryGraphConnectionController: () => hookState.connectionController,
}));

vi.mock("./react-flow-current-activity-card-editor-removals", () => ({
  useFactoryGraphRemovalController: () => hookState.removalController,
}));

vi.mock(
  "../../factory-graph-editor/hooks/validation/use-factory-validation",
  () => ({
    useFactoryValidation: () => ({
      data: { targets: [] },
      isError: false,
      isFetching: false,
      isLoading: false,
      projection: {
        handleErrorsByAnchorId: new Map(),
        nodeErrorsByNodeId: new Map(),
      },
      targets: [],
    }),
  }),
);

vi.mock("./current-activity-graph-state-value", async () => {
  const actual = await vi.importActual("./current-activity-graph-state-value");

  return actual;
});

describe("useCurrentActivityGraphState", () => {
  beforeEach(() => {
    vi.mocked(useCurrentFactoryDocument).mockClear();
    useFactoryGraphTopologyEditorBridge.setState({
      graphDraftHasPendingChanges: false,
      handlers: null,
    });
    hookState.addEntityController.reset.mockReset();
    hookState.connectionController.handleConnectionAnchorClick.mockReset();
    hookState.connectionController.handleEditorConnect.mockReset();
    hookState.connectionController.setBlockedRemovalReason.mockReset();
    hookState.connectionController.setConnectionNotice.mockReset();
    hookState.currentFactoryQuery = {
      data: fixtureState.editableDocument,
      status: "success",
    };
    hookState.draftState = {
      baseDocument: fixtureState.editableDocument,
      draft: fixtureState.emptyDraft(),
      hasChanges: false,
      latestDocument: fixtureState.editableDocument,
      pendingFactoryDefinition: fixtureState.baseFactoryDefinition,
      replaceDraft: vi.fn(),
      resetDraft: vi.fn(),
      validationErrors: [],
    };
    hookState.removalController.handleConfirmRemoval.mockReset();
    hookState.removalController.handleEditorEdgeDelete.mockReset();
    hookState.removalController.handleEditorNodeDelete.mockReset();
    hookState.removalController.setPendingRemovalEdgeId.mockReset();
    hookState.removalController.setPendingRemovalNodeId.mockReset();
    hookState.saveEditableDefinition = {
      error: null,
      isPending: false,
      reset: vi.fn(),
      saveAsync: vi.fn(async () => undefined),
    };
    hookState.documentSave = { status: "idle" };
    hookState.unsupportedFromDefinition = undefined;
    hookState.saveStateIsStale = false;
  });

  it("uses the versioned event factory without calling the current factory document query", () => {
    hookState.currentFactoryQuery = {
      data: undefined,
      status: "pending",
    };
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.factory = {
      ...fixtureState.baseFactoryDefinition,
      version: {
        logical: "7",
        physical: "2026-06-09T00:00:00Z",
      },
    };

    const { result } = renderHook(() => useCurrentActivityGraphState(snapshot));

    expect(useCurrentFactoryDocument).not.toHaveBeenCalled();
    expect(result.current.status.isDefinitionLoading).toBe(false);
    expect(
      result.current.graphProjection.displayFactoryDefinition,
    ).toMatchObject({
      version: {
        logical: "7",
        physical: "2026-06-09T00:00:00Z",
      },
    });
  });

  it("does not call the current factory document query when entering editor mode without a versioned event factory", () => {
    hookState.currentFactoryQuery = {
      data: undefined,
      status: "pending",
    };
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.factory = fixtureState.baseFactoryDefinition;

    const { result } = renderHook(() => useCurrentActivityGraphState(snapshot));

    expect(useCurrentFactoryDocument).not.toHaveBeenCalled();

    act(() => {
      result.current.editorControls.toggleMode();
    });

    expect(useCurrentFactoryDocument).not.toHaveBeenCalled();
    expect(result.current.status.isDefinitionLoading).toBe(true);
  });

  it("enters and leaves editor mode while resetting transient state", () => {
    const { result } = renderHook(() =>
      useCurrentActivityGraphState(semanticWorkflowDashboardSnapshot),
    );

    expect(result.current.editorControls.isEditing).toBe(false);

    act(() => {
      result.current.editorControls.toggleMode();
    });

    expect(result.current.editorControls.isEditing).toBe(true);

    act(() => {
      result.current.editorControls.selectTool("connect");
    });

    act(() => {
      result.current.editorControls.toggleMode();
    });

    expect(result.current.editorControls.isEditing).toBe(false);
    expect(result.current.editorControls.activeTool).toBeNull();
    expect(hookState.addEntityController.reset).toHaveBeenCalledTimes(1);
    expect(hookState.saveEditableDefinition.reset).toHaveBeenCalledTimes(1);
    expect(
      hookState.connectionController.setConnectionNotice,
    ).toHaveBeenCalledWith(null);
    expect(
      hookState.removalController.setPendingRemovalEdgeId,
    ).toHaveBeenCalledWith(null);
    expect(
      hookState.removalController.setPendingRemovalNodeId,
    ).toHaveBeenCalledWith(null);
  });

  it("opens the leave confirmation instead of exiting when the draft has changes", () => {
    const { result, rerender } = renderHook(() =>
      useCurrentActivityGraphState(semanticWorkflowDashboardSnapshot),
    );

    act(() => {
      result.current.editorControls.toggleMode();
    });

    hookState.draftState.hasChanges = true;
    rerender();

    act(() => {
      result.current.editorControls.toggleMode();
    });

    expect(result.current.editorControls.isEditing).toBe(true);
    expect(result.current.leaveControls.isOpen).toBe(true);
    expect(hookState.addEntityController.reset).not.toHaveBeenCalled();
  });

  it("returns false without mutating when a draft cannot be saved", async () => {
    hookState.draftState.pendingFactoryDefinition = null;

    const { result } = renderHook(() =>
      useCurrentActivityGraphState(semanticWorkflowDashboardSnapshot),
    );

    let didSave = true;
    await act(async () => {
      didSave = await result.current.saveControls.confirmSave();
    });

    expect(didSave).toBe(false);
    expect(hookState.saveEditableDefinition.saveAsync).not.toHaveBeenCalled();
  });

  it("blocks saving while snapshot runtime reports in-flight dispatches", () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.runtime.in_flight_dispatch_count = 2;
    hookState.draftState.hasChanges = true;

    const { result } = renderHook(() => useCurrentActivityGraphState(snapshot));

    expect(result.current.status.hasActiveWork).toBe(true);
    expect(result.current.saveControls.canSave).toBe(false);
    expect(result.current.status.saveBlockedReason).toBe(
      "Topology save is unavailable while active work is still running in this factory.",
    );
  });

  it("blocks saving with stale-draft messaging when the document version drifts", () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.runtime.in_flight_dispatch_count = 0;
    hookState.draftState.hasChanges = true;
    hookState.saveStateIsStale = true;

    const { result } = renderHook(() => useCurrentActivityGraphState(snapshot));

    expect(result.current.status.isStaleDraft).toBe(true);
    expect(result.current.saveControls.canSave).toBe(false);
    expect(result.current.status.saveBlockedReason).toBe(
      "A newer factory topology arrived while this draft was open. Refresh or discard before saving.",
    );
  });

  it("closes the save confirmation when saving fails", async () => {
    hookState.draftState.hasChanges = true;
    hookState.saveEditableDefinition.saveAsync = vi.fn(async () => {
      throw new Error("Save failed");
    });
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.runtime.in_flight_dispatch_count = 0;

    const { result, rerender } = renderHook(() =>
      useCurrentActivityGraphState(snapshot),
    );

    act(() => {
      result.current.saveControls.requestConfirmation();
    });
    rerender();
    expect(result.current.saveControls.isConfirming).toBe(true);

    let didSave = true;
    await act(async () => {
      didSave = await result.current.saveControls.confirmSave();
    });

    expect(didSave).toBe(false);
    rerender();
    expect(result.current.saveControls.isConfirming).toBe(false);
  });

  it("resets editor chrome and topology bridge when factory document scope changes", () => {
    const { result, rerender } = renderHook(
      ({ factoryDocumentScopeKey }: { factoryDocumentScopeKey: string }) =>
        useCurrentActivityGraphState(
          semanticWorkflowDashboardSnapshot,
          "en",
          factoryDocumentScopeKey,
        ),
      {
        initialProps: { factoryDocumentScopeKey: "session-alpha" },
      },
    );

    act(() => {
      result.current.editorControls.toggleMode();
      result.current.editorControls.selectTool("connect");
      result.current.saveControls.requestConfirmation();
    });
    hookState.draftState.hasChanges = true;
    rerender({ factoryDocumentScopeKey: "session-alpha" });
    act(() => {
      result.current.editorControls.toggleMode();
    });
    expect(result.current.leaveControls.isOpen).toBe(true);
    expect(readGraphDraftHasPendingChanges()).toBe(true);
    useFactoryGraphTopologyEditorBridge.setState({
      handlers: {
        blockedRemovalReason: null,
        canInteractWithEditor: true,
        editorMode: true,
        requestNodeRemoval: vi.fn(),
      },
    });

    rerender({ factoryDocumentScopeKey: "session-beta" });

    expect(result.current.editorControls.isEditing).toBe(false);
    expect(result.current.editorControls.activeTool).toBeNull();
    expect(result.current.leaveControls.isOpen).toBe(false);
    expect(result.current.saveControls.isConfirming).toBe(false);
    expect(hookState.addEntityController.reset).toHaveBeenCalled();
    expect(hookState.saveEditableDefinition.reset).toHaveBeenCalled();
    expect(
      hookState.connectionController.setConnectionNotice,
    ).toHaveBeenCalledWith(null);
    expect(
      hookState.removalController.setPendingRemovalEdgeId,
    ).toHaveBeenCalledWith(null);
    expect(
      hookState.removalController.setPendingRemovalNodeId,
    ).toHaveBeenCalledWith(null);
    expect(useFactoryGraphTopologyEditorBridge.getState().handlers).toBeNull();
    expect(readGraphDraftHasPendingChanges()).toBe(false);
  });

  it("publishes graph draft pending state when draft dirtiness changes", () => {
    const { rerender } = renderHook(() =>
      useCurrentActivityGraphState(semanticWorkflowDashboardSnapshot),
    );

    expect(readGraphDraftHasPendingChanges()).toBe(false);

    hookState.draftState.hasChanges = true;
    rerender();

    expect(readGraphDraftHasPendingChanges()).toBe(true);

    hookState.draftState.hasChanges = false;
    rerender();

    expect(readGraphDraftHasPendingChanges()).toBe(false);
  });

  it("clears graph draft pending state on unmount", () => {
    hookState.draftState.hasChanges = true;
    const { unmount } = renderHook(() =>
      useCurrentActivityGraphState(semanticWorkflowDashboardSnapshot),
    );

    expect(readGraphDraftHasPendingChanges()).toBe(true);

    unmount();

    expect(readGraphDraftHasPendingChanges()).toBe(false);
  });

  it("saves the draft and leaves editor mode when asked to save before leaving", async () => {
    hookState.draftState.hasChanges = true;
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.runtime.in_flight_dispatch_count = 0;

    const { result } = renderHook(() => useCurrentActivityGraphState(snapshot));

    act(() => {
      result.current.editorControls.toggleMode();
      result.current.editorControls.selectTool("connect");
    });

    let didSave = false;
    await act(async () => {
      didSave = await result.current.saveControls.saveBeforeLeaving();
    });

    expect(didSave).toBe(true);
    expect(hookState.saveEditableDefinition.saveAsync).toHaveBeenCalledWith({
      baseVersion: fixtureState.editableDocument.version,
      factory: fixtureState.baseFactoryDefinition,
    });
    expect(hookState.draftState.replaceDraft).toHaveBeenCalledWith(
      createEmptyFactoryGraphDraft(),
    );
    expect(result.current.editorControls.isEditing).toBe(false);
    expect(result.current.editorControls.activeTool).toBeNull();
  });
});

const inSessionScopeKey = "session-in-session";

function renderEditorWithConstantScope(
  snapshot = semanticWorkflowDashboardSnapshot,
) {
  return renderHook(() =>
    useCurrentActivityGraphState(snapshot, "en", inSessionScopeKey),
  );
}

describe("useCurrentActivityGraphState in-session save and discard", () => {
  beforeEach(() => {
    useFactoryGraphTopologyEditorBridge.setState({
      graphDraftHasPendingChanges: false,
      handlers: null,
    });
    hookState.addEntityController.reset.mockReset();
    hookState.connectionController.setConnectionNotice.mockReset();
    hookState.currentFactoryQuery = {
      data: fixtureState.editableDocument,
      status: "success",
    };
    hookState.draftState = {
      baseDocument: fixtureState.editableDocument,
      draft: fixtureState.emptyDraft(),
      hasChanges: false,
      latestDocument: fixtureState.editableDocument,
      pendingFactoryDefinition: fixtureState.baseFactoryDefinition,
      replaceDraft: vi.fn(),
      resetDraft: vi.fn(),
      validationErrors: [],
    };
    hookState.saveEditableDefinition = {
      error: null,
      isPending: false,
      reset: vi.fn(),
      saveAsync: vi.fn(async () => undefined),
    };
    hookState.documentSave = { status: "idle" };
    hookState.saveStateIsStale = false;
  });

  it("keeps leave confirmation and editor mode when the draft is dirty within a constant scope", () => {
    const { result, rerender } = renderEditorWithConstantScope();

    act(() => {
      result.current.editorControls.toggleMode();
    });
    hookState.draftState.hasChanges = true;
    rerender();

    act(() => {
      result.current.editorControls.toggleMode();
    });

    expect(result.current.editorControls.isEditing).toBe(true);
    expect(result.current.leaveControls.isOpen).toBe(true);
    expect(hookState.addEntityController.reset).not.toHaveBeenCalled();
    expect(useFactoryGraphTopologyEditorBridge.getState().handlers).toBeNull();
  });

  it("discards pending changes without leaving editor mode within a constant scope", () => {
    hookState.draftState.hasChanges = true;
    const { result } = renderEditorWithConstantScope();

    act(() => {
      result.current.editorControls.toggleMode();
      result.current.editorControls.selectTool("connect");
      result.current.editorControls.discardPendingChanges();
    });

    expect(hookState.draftState.resetDraft).toHaveBeenCalledTimes(1);
    expect(hookState.addEntityController.reset).toHaveBeenCalledTimes(1);
    expect(result.current.editorControls.isEditing).toBe(true);
    expect(result.current.editorControls.activeTool).toBe("connect");
    expect(result.current.leaveControls.isOpen).toBe(false);
  });

  it("saves pending edits and clears the draft within a constant scope", async () => {
    hookState.draftState.hasChanges = true;
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.runtime.in_flight_dispatch_count = 0;

    const { result } = renderEditorWithConstantScope(snapshot);

    act(() => {
      result.current.editorControls.toggleMode();
    });

    let didSave = false;
    await act(async () => {
      didSave = await result.current.saveControls.confirmSave();
    });

    expect(didSave).toBe(true);
    expect(hookState.saveEditableDefinition.saveAsync).toHaveBeenCalledWith({
      baseVersion: fixtureState.editableDocument.version,
      factory: fixtureState.baseFactoryDefinition,
    });
    expect(hookState.draftState.replaceDraft).toHaveBeenCalledWith(
      createEmptyFactoryGraphDraft(),
    );
    expect(result.current.editorControls.isEditing).toBe(true);
  });

  it("blocks save when draft validation errors are present within a constant scope", async () => {
    hookState.draftState.hasChanges = true;
    hookState.draftState.validationErrors = [
      {
        code: "MISSING_REQUIRED_FIELD",
        field: "worker",
        message: "Workstation draft requires a worker assignment.",
        target: {
          id: "workstation:review",
          kind: "node",
        },
      },
    ];
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.runtime.in_flight_dispatch_count = 0;

    const { result } = renderEditorWithConstantScope(snapshot);

    let didSave = true;
    await act(async () => {
      didSave = await result.current.saveControls.confirmSave();
    });

    expect(didSave).toBe(false);
    expect(result.current.saveControls.canSave).toBe(false);
    expect(hookState.saveEditableDefinition.saveAsync).not.toHaveBeenCalled();
  });
});
