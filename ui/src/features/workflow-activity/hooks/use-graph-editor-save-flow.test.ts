import { act, renderHook } from "@testing-library/react";

import { createEmptyFactoryGraphDraft } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { useGraphEditorSaveFlow } from "./use-graph-editor-save-flow";

const fixtureState = vi.hoisted(() => {
  const emptyDraft = {
    additions: {
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
      resources: [],
      workers: [],
      workStates: [],
      workTypes: [],
      workstations: [],
    },
  };
  const baseDocument = {
    name: "Current Factory",
    resources: [],
    version: {
      logical: "4",
      physical: "2026-05-25T00:00:00Z",
    },
    workers: [],
    workTypes: [],
    workstations: [],
  };

  return {
    addEntityController: {
      reset: vi.fn(),
    },
    baseDocument,
    documentSave: { status: "idle" as const },
    documentSaveControls: {
      beginConfirmation: vi.fn(),
      cancelConfirmation: vi.fn(),
      clearSaveFeedback: vi.fn(),
    },
    emptyDraft,
    draftState: {
      baseDocument,
      draft: emptyDraft,
      hasChanges: false,
      latestDocument: baseDocument,
      pendingFactoryDefinition: baseDocument,
      replaceDraft: vi.fn(),
      resetDraft: vi.fn(),
      validationErrors: [],
    },
    saveEditableDefinition: {
      error: null,
      isPending: false,
      reset: vi.fn(),
      saveAsync: vi.fn(async () => undefined),
    },
    saveStateIsStale: false,
    transientControllerReset: {
      setBlockedRemovalReason: vi.fn(),
      setConnectionNotice: vi.fn(),
      setPendingRemovalEdgeId: vi.fn(),
      setPendingRemovalNodeId: vi.fn(),
    },
  };
});

function buildEditableGraph(): EditableFactoryGraphViewModel {
  return {
    actions: {
      discard: fixtureState.draftState.resetDraft,
      save: async () => {
        if (!fixtureState.draftState.pendingFactoryDefinition) {
          return false;
        }
        await fixtureState.saveEditableDefinition.saveAsync({
          baseVersion: fixtureState.draftState.latestDocument?.version,
          factory: fixtureState.draftState.pendingFactoryDefinition,
        });
        fixtureState.draftState.replaceDraft(createEmptyFactoryGraphDraft());
        return true;
      },
    },
    documentSaveControls: fixtureState.documentSaveControls,
    draftState: fixtureState.draftState,
    saveMutation: fixtureState.saveEditableDefinition,
    saveState: {
      canSave:
        fixtureState.draftState.hasChanges &&
        fixtureState.draftState.pendingFactoryDefinition !== null &&
        fixtureState.draftState.latestDocument !== null &&
        !fixtureState.saveStateIsStale,
      documentSave: fixtureState.documentSave,
      isStale: fixtureState.saveStateIsStale,
    },
  } as EditableFactoryGraphViewModel;
}

function renderSaveFlow(activeWorkCount = 0) {
  const setActiveTool = vi.fn();
  const setEditorMode = vi.fn();

  return {
    setActiveTool,
    setEditorMode,
    ...renderHook(() =>
      useGraphEditorSaveFlow({
        activeWorkCount,
        addEntityController: fixtureState.addEntityController,
        documentSave: fixtureState.documentSave,
        documentSaveControls: fixtureState.documentSaveControls,
        draftState: fixtureState.draftState,
        editableGraph: buildEditableGraph(),
        locale: "en",
        saveEditableDefinition: fixtureState.saveEditableDefinition,
        setActiveTool,
        setEditorMode,
        transientControllerReset: fixtureState.transientControllerReset,
      }),
    ),
  };
}

function resetSaveFlowFixture() {
  fixtureState.addEntityController.reset.mockReset();
  fixtureState.documentSave = { status: "idle" };
  fixtureState.documentSaveControls.beginConfirmation.mockReset();
  fixtureState.documentSaveControls.cancelConfirmation.mockReset();
  fixtureState.documentSaveControls.clearSaveFeedback.mockReset();
  fixtureState.saveStateIsStale = false;
  fixtureState.draftState = {
    baseDocument: fixtureState.baseDocument,
    draft: fixtureState.emptyDraft,
    hasChanges: false,
    latestDocument: fixtureState.baseDocument,
    pendingFactoryDefinition: fixtureState.baseDocument,
    replaceDraft: vi.fn(),
    resetDraft: vi.fn(),
    validationErrors: [],
  };
  fixtureState.saveEditableDefinition = {
    error: null,
    isPending: false,
    reset: vi.fn(),
    saveAsync: vi.fn(async () => undefined),
  };
  fixtureState.transientControllerReset.setBlockedRemovalReason.mockReset();
  fixtureState.transientControllerReset.setConnectionNotice.mockReset();
  fixtureState.transientControllerReset.setPendingRemovalEdgeId.mockReset();
  fixtureState.transientControllerReset.setPendingRemovalNodeId.mockReset();
}

describe("useGraphEditorSaveFlow", () => {
  beforeEach(resetSaveFlowFixture);

  it("blocks saving while active work is in flight", () => {
    fixtureState.draftState.hasChanges = true;

    const { result } = renderSaveFlow(2);

    expect(result.current.hasActiveWork).toBe(true);
    expect(result.current.canSaveDraft).toBe(false);
    expect(result.current.saveBlockedReason).toBe(
      "Topology save is unavailable while active work is still running in this factory.",
    );
  });

  it("blocks saving with stale-draft messaging when the document version drifts", () => {
    fixtureState.draftState.hasChanges = true;
    fixtureState.saveStateIsStale = true;

    const { result } = renderSaveFlow(0);

    expect(result.current.isStaleDraft).toBe(true);
    expect(result.current.canSaveDraft).toBe(false);
    expect(result.current.saveBlockedReason).toBe(
      "A newer factory topology arrived while this draft was open. Refresh or discard before saving.",
    );
  });

  it("opens save confirmation through scoped beginConfirmation", () => {
    const { result } = renderSaveFlow(0);

    act(() => {
      result.current.setIsConfirmingSave(true);
    });

    expect(
      fixtureState.documentSaveControls.beginConfirmation,
    ).toHaveBeenCalledTimes(1);
  });

  it("reflects scoped confirming status for isConfirmingSave", () => {
    fixtureState.documentSave = { status: "confirming" };

    const { result } = renderSaveFlow(0);

    expect(result.current.isConfirmingSave).toBe(true);
  });

  it("discards pending changes without leaving editor mode", () => {
    fixtureState.draftState.hasChanges = true;
    const { result, setActiveTool, setEditorMode } = renderSaveFlow(0);

    act(() => {
      result.current.handleDiscardPendingChanges();
    });

    expect(fixtureState.draftState.resetDraft).toHaveBeenCalledTimes(1);
    expect(fixtureState.addEntityController.reset).toHaveBeenCalledTimes(1);
    expect(fixtureState.documentSaveControls.clearSaveFeedback).toHaveBeenCalledTimes(
      1,
    );
    expect(setEditorMode).not.toHaveBeenCalled();
    expect(setActiveTool).not.toHaveBeenCalled();
    expect(result.current.isConfirmingLeaveEditor).toBe(false);
  });

  it("clears scoped save feedback when saving fails", async () => {
    fixtureState.draftState.hasChanges = true;
    fixtureState.saveEditableDefinition.saveAsync = vi.fn(async () => {
      throw new Error("Save failed");
    });

    const { result } = renderSaveFlow(0);

    let didSave = true;
    await act(async () => {
      didSave = await result.current.handleSaveDraft();
    });

    expect(didSave).toBe(false);
    expect(fixtureState.documentSaveControls.clearSaveFeedback).toHaveBeenCalled();
  });

  it("discards pending changes and leaves editor mode from the leave dialog path", () => {
    fixtureState.draftState.hasChanges = true;
    const { result, setActiveTool, setEditorMode } = renderSaveFlow(0);

    act(() => {
      result.current.handleDiscardEditorChanges();
    });

    expect(fixtureState.draftState.resetDraft).toHaveBeenCalledTimes(1);
    expect(setEditorMode).toHaveBeenCalledWith(false);
    expect(setActiveTool).toHaveBeenCalledWith(null);
    expect(result.current.isConfirmingLeaveEditor).toBe(false);
  });

  it("saves and leaves editor mode when asked to save before leaving", async () => {
    fixtureState.draftState.hasChanges = true;
    const { result, setActiveTool, setEditorMode } = renderSaveFlow(0);

    act(() => {
      result.current.setIsConfirmingLeaveEditor(true);
    });

    let didSave = false;
    await act(async () => {
      didSave = await result.current.handleSaveBeforeLeavingEditor();
    });

    expect(didSave).toBe(true);
    expect(fixtureState.saveEditableDefinition.saveAsync).toHaveBeenCalledWith({
      baseVersion: fixtureState.baseDocument.version,
      factory: fixtureState.baseDocument,
    });
    expect(setEditorMode).toHaveBeenCalledWith(false);
    expect(setActiveTool).toHaveBeenCalledWith(null);
    expect(result.current.isConfirmingLeaveEditor).toBe(false);
  });
});

describe("useGraphEditorSaveFlow saveAttemptRevision", () => {
  beforeEach(resetSaveFlowFixture);

  it("starts at zero", () => {
    const { result } = renderSaveFlow(0);

    expect(result.current.saveAttemptRevision).toBe(0);
  });

  it("increments when each save starts", async () => {
    fixtureState.draftState.hasChanges = true;
    const { result } = renderSaveFlow(0);

    await act(async () => {
      await result.current.handleSaveDraft();
    });
    expect(result.current.saveAttemptRevision).toBe(1);

    fixtureState.draftState.hasChanges = true;
    await act(async () => {
      await result.current.handleSaveBeforeLeavingEditor();
    });
    expect(result.current.saveAttemptRevision).toBe(2);
  });

  it("does not increment on rerender alone", () => {
    fixtureState.draftState.hasChanges = true;
    const { result, rerender } = renderSaveFlow(0);

    expect(result.current.saveAttemptRevision).toBe(0);

    rerender();

    expect(result.current.saveAttemptRevision).toBe(0);
  });

  it("does not increment when save is blocked", async () => {
    fixtureState.draftState.hasChanges = true;
    const { result } = renderSaveFlow(2);

    await act(async () => {
      await result.current.handleSaveDraft();
    });

    expect(result.current.saveAttemptRevision).toBe(0);
  });
});
