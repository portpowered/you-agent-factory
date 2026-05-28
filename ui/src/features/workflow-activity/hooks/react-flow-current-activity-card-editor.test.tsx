// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: focused hook coverage keeps the mergeability-only save/leave branches in one seam.
import { act, renderHook } from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import { createEmptyFactoryGraphDraft } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";

const fixtureState = vi.hoisted(() => {
  const emptyDraft = () => ({
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
    handleConfirmRemoval: vi.fn(),
    handleEditorEdgeDelete: vi.fn(),
    handleEditorNodeDelete: vi.fn(),
    pendingRemovalIntent: null,
    setPendingRemovalEdgeId: vi.fn(),
    setPendingRemovalNodeId: vi.fn(),
  },
  saveEditableDefinition: {
    error: null,
    mutateAsync: vi.fn(async () => undefined),
    reset: vi.fn(),
    status: "idle" as const,
  },
  unsupportedFromDefinition: undefined as string | undefined,
}));

vi.mock("../../current-factory-definition/public", () => ({
  useCurrentFactoryDocument: () => hookState.currentFactoryQuery,
  useSaveCurrentFactory: () => hookState.saveEditableDefinition,
}));

vi.mock("../../factory-graph-editor/public", () => ({
  createEmptyFactoryGraphDraft,
  useEditableFactoryGraph: () => ({
    actions: {
      discard: hookState.draftState.resetDraft,
      save: async () => {
        if (!hookState.draftState.pendingFactoryDefinition) {
          return false;
        }
        await hookState.saveEditableDefinition.mutateAsync({
          baseVersion: hookState.draftState.latestDocument?.version,
          factoryDefinition: hookState.draftState.pendingFactoryDefinition,
        });
        hookState.draftState.replaceDraft(createEmptyFactoryGraphDraft());
        return true;
      },
    },
    draftState: hookState.draftState,
    saveState: {
      canSave:
        hookState.draftState.hasChanges &&
        hookState.draftState.pendingFactoryDefinition !== null &&
        hookState.draftState.latestDocument !== null,
      isStale: false,
    },
  }),
}));

vi.mock(
  "../../factory-graph-editor/lib/factory-graph-editor-additions",
  () => ({
    buildFactoryGraphAddEntityMenuActions: () => [],
  }),
);

vi.mock(
  "../../factory-graph-editor/lib/factory-graph-editor-save-summary",
  () => ({
    buildFactoryGraphSaveSummary: () => ({
      additions: [],
      removals: [],
    }),
  }),
);

vi.mock("../components/react-flow-current-activity-card-editor-chrome", () => ({
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

vi.mock("./react-flow-current-activity-card-editor-value", () => ({
  buildCurrentActivityGraphEditorValue: (value: Record<string, unknown>) =>
    value,
}));

describe("useCurrentActivityGraphEditor", () => {
  beforeEach(() => {
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
      mutateAsync: vi.fn(async () => undefined),
      reset: vi.fn(),
      status: "idle",
    };
    hookState.unsupportedFromDefinition = undefined;
  });

  it("enters and leaves editor mode while resetting transient state", () => {
    const { result } = renderHook(() =>
      useCurrentActivityGraphEditor(semanticWorkflowDashboardSnapshot),
    );

    expect(result.current.editorMode).toBe(false);

    act(() => {
      result.current.handleEditorModeToggle();
    });

    expect(result.current.editorMode).toBe(true);

    act(() => {
      result.current.setActiveTool("connect");
    });

    act(() => {
      result.current.handleEditorModeToggle();
    });

    expect(result.current.editorMode).toBe(false);
    expect(result.current.activeTool).toBeNull();
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
    const { result } = renderHook(() =>
      useCurrentActivityGraphEditor(semanticWorkflowDashboardSnapshot),
    );

    act(() => {
      result.current.handleEditorModeToggle();
    });

    hookState.draftState.hasChanges = true;

    act(() => {
      result.current.handleEditorModeToggle();
    });

    expect(result.current.editorMode).toBe(true);
    expect(result.current.isConfirmingLeaveEditor).toBe(true);
    expect(hookState.addEntityController.reset).not.toHaveBeenCalled();
  });

  it("returns false without mutating when a draft cannot be saved", async () => {
    hookState.draftState.pendingFactoryDefinition = null;

    const { result } = renderHook(() =>
      useCurrentActivityGraphEditor(semanticWorkflowDashboardSnapshot),
    );

    let didSave = true;
    await act(async () => {
      didSave = await result.current.handleSaveDraft();
    });

    expect(didSave).toBe(false);
    expect(hookState.saveEditableDefinition.mutateAsync).not.toHaveBeenCalled();
  });

  it("closes the save confirmation when saving fails", async () => {
    hookState.draftState.hasChanges = true;
    hookState.saveEditableDefinition.mutateAsync = vi.fn(async () => {
      throw new Error("Save failed");
    });
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.runtime.in_flight_dispatch_count = 0;

    const { result } = renderHook(() =>
      useCurrentActivityGraphEditor(snapshot),
    );

    act(() => {
      result.current.setIsConfirmingSave(true);
    });

    let didSave = true;
    await act(async () => {
      didSave = await result.current.handleSaveDraft();
    });

    expect(didSave).toBe(false);
    expect(result.current.isConfirmingSave).toBe(false);
  });

  it("saves the draft and leaves editor mode when asked to save before leaving", async () => {
    hookState.draftState.hasChanges = true;
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.runtime.in_flight_dispatch_count = 0;

    const { result } = renderHook(() =>
      useCurrentActivityGraphEditor(snapshot),
    );

    act(() => {
      result.current.handleEditorModeToggle();
      result.current.setActiveTool("connect");
      result.current.setIsConfirmingLeaveEditor(true);
      result.current.setIsConfirmingSave(true);
    });

    let didSave = false;
    await act(async () => {
      didSave = await result.current.handleSaveBeforeLeavingEditor();
    });

    expect(didSave).toBe(true);
    expect(hookState.saveEditableDefinition.mutateAsync).toHaveBeenCalledWith({
      baseVersion: fixtureState.editableDocument.version,
      factoryDefinition: fixtureState.baseFactoryDefinition,
    });
    expect(hookState.draftState.replaceDraft).toHaveBeenCalledWith(
      createEmptyFactoryGraphDraft(),
    );
    expect(result.current.editorMode).toBe(false);
    expect(result.current.activeTool).toBeNull();
  });
});
