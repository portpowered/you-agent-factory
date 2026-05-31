import { renderHook } from "@testing-library/react";

import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { useGraphEditorControllers } from "./use-graph-editor-controllers";

const fixtureState = vi.hoisted(() => ({
  addEntityController: {
    reset: vi.fn(),
  },
  connectionController: {
    blockedRemovalReason: null as string | null,
    connectionNotice: null as string | null,
    handleConnectionAnchorClick: vi.fn(),
    handleEditorConnect: vi.fn(),
    pendingConnectionSource: null,
    setBlockedRemovalReason: vi.fn(),
    setConnectionNotice: vi.fn(),
  },
  removalController: {
    blockedRemovalReason: null as string | null,
    handleConfirmRemoval: vi.fn(),
    handleEditorEdgeDelete: vi.fn(),
    handleEditorNodeDelete: vi.fn(),
    pendingRemovalIntent: null,
    setBlockedRemovalReason: vi.fn(),
    setPendingRemovalEdgeId: vi.fn(),
    setPendingRemovalNodeId: vi.fn(),
  },
}));

vi.mock("../components/react-flow-current-activity-card-editor-chrome", () => ({
  useFactoryGraphAddEntityController: () => fixtureState.addEntityController,
}));

vi.mock("./react-flow-current-activity-card-editor-connections", () => ({
  useFactoryGraphConnectionController: () => fixtureState.connectionController,
}));

vi.mock("./react-flow-current-activity-card-editor-removals", () => ({
  useFactoryGraphRemovalController: () => fixtureState.removalController,
}));

function buildEditableGraph(): EditableFactoryGraphViewModel {
  return {
    actions: {
      addNode: vi.fn(),
      connectNodes: vi.fn(),
      discard: vi.fn(),
      disconnectEdge: vi.fn(),
      removeNode: vi.fn(),
      save: vi.fn(async () => true),
      updateNodeField: vi.fn(),
    },
    draftState: {
      baseDocument: null,
      draft: {
        additions: {
          resources: [],
          workers: [],
          workStates: [],
          workTypes: [],
          workstations: [],
        },
        edgeChanges: { additions: [], removals: [] },
        removals: {
          resources: [],
          workers: [],
          workStates: [],
          workTypes: [],
          workstations: [],
        },
      },
      hasChanges: false,
      latestDocument: null,
      pendingFactoryDefinition: null,
      replaceDraft: vi.fn(),
      resetDraft: vi.fn(),
      validationErrors: [],
    },
    documentSaveControls: {
      beginConfirmation: vi.fn(),
      cancelConfirmation: vi.fn(),
      clearSaveFeedback: vi.fn(),
    },
    saveState: {
      canSave: false,
      documentSave: { status: "idle" },
      isStale: false,
    },
  };
}

describe("useGraphEditorControllers", () => {
  it("composes add-entity, connection, and removal controller surfaces", () => {
    const { result } = renderHook(() =>
      useGraphEditorControllers({
        activeTool: "connect",
        canInteractWithEditor: true,
        currentFactoryDefinition: null,
        draftState: buildEditableGraph().draftState,
        editableGraph: buildEditableGraph(),
        locale: "en",
        saveEditableDefinition: {
          error: null,
          isPending: false,
          reset: vi.fn(),
          saveAsync: vi.fn(),
        } as never,
        setActiveTool: vi.fn(),
      }),
    );

    expect(result.current.addEntityController).toBe(
      fixtureState.addEntityController,
    );
    expect(result.current.handleConnectionAnchorClick).toBe(
      fixtureState.connectionController.handleConnectionAnchorClick,
    );
    expect(result.current.handleEditorConnect).toBe(
      fixtureState.connectionController.handleEditorConnect,
    );
    expect(result.current.handleEditorEdgeDelete).toBe(
      fixtureState.removalController.handleEditorEdgeDelete,
    );
    expect(result.current.handleEditorNodeDelete).toBe(
      fixtureState.removalController.handleEditorNodeDelete,
    );
    expect(result.current.setConnectionNotice).toBe(
      fixtureState.connectionController.setConnectionNotice,
    );
    expect(result.current.setPendingRemovalEdgeId).toBe(
      fixtureState.removalController.setPendingRemovalEdgeId,
    );
  });
});
