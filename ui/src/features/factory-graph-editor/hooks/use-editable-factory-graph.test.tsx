import { act, renderHook, waitFor } from "@testing-library/react";
import {
  createEditableFactoryGraphHookWrapper,
  renderEditableFactoryGraphHook,
  setupEditableFactoryGraphSaveTestEnvironment,
} from "../../../testing/editable-factory-graph-hook-test-helpers";
import { mockFactoryDocumentSave } from "../../../testing/factory-document-save-mocks";
import {
  baseFactoryDefinition,
  currentFactoryDocument,
} from "../lib/factory-graph-draft.test-helpers";
import {
  createHookTestGraphEditorDraftState,
  type MockGraphEditorDraftState,
} from "../../../testing/graph-editor-harness";
import { createEmptyFactoryGraphDraft } from "../lib/factory-graph-draft-types";
import { useEditableFactoryGraph } from "./use-editable-factory-graph";

const hookState = vi.hoisted(() => ({
  draftState: {} as MockGraphEditorDraftState,
}));

vi.mock("./factory-graph-draft-hook", () => ({
  useFactoryGraphDraftState: () => hookState.draftState,
}));

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: the view-model contract is clearer when its state and action scenarios stay together.
describe("useEditableFactoryGraph", () => {
  beforeEach(() => {
    hookState.draftState = createHookTestGraphEditorDraftState();
    setupEditableFactoryGraphSaveTestEnvironment();
  });

  it("exposes pending, projection, validation, and blocked operation state", () => {
    const { result, rerender } = renderEditableFactoryGraphHook({
      currentFactoryDocument: currentFactoryDocument,
    });

    expect(result.current.pendingState.hasChanges).toBe(false);
    expect(result.current.projection.nodes.map((node) => node.id)).toContain(
      "workstation:draft",
    );
    expect(
      result.current.projection.nodes.find(
        (node) => node.id === "work-state:story:queued",
      )?.data.workStateType,
    ).toBe("INITIAL");
    expect(
      result.current.projection.nodes.find(
        (node) => node.id === "work-state:story:done",
      )?.data.workStateType,
    ).toBe("TERMINAL");
    expect(result.current.validationState.isValid).toBe(true);

    act(() => {
      result.current.actions.addNode({
        argsText: "",
        command: "",
        kind: "worker",
        model: "gpt-5-mini",
        modelProvider: "CURSOR",
        name: "reviewer",
        workerType: "MODEL_WORKER",
      });
    });
    rerender();

    expect(result.current.pendingState.hasChanges).toBe(true);
    expect(
      result.current.graphState?.graph.nodes.map((node) => node.id),
    ).toContain("worker:reviewer");

    act(() => {
      result.current.actions.addNode({
        argsText: "",
        command: "",
        kind: "worker",
        model: "gpt-5-mini",
        modelProvider: "CURSOR",
        name: "writer",
        workerType: "MODEL_WORKER",
      });
    });

    expect(result.current.blockedOperation).toMatchObject({
      ok: false,
      reason: "DUPLICATE_IDENTIFIER",
    });
    expect(result.current.pendingState.hasChanges).toBe(true);
  });

  it("reports stale save state when the current factory changes during a draft", () => {
    hookState.draftState.hasChanges = true;
    hookState.draftState.latestDocument = {
      ...currentFactoryDocument,
      version: {
        logical: "6",
        physical: "2026-05-18T16:00:00Z",
      },
    };

    const { result } = renderEditableFactoryGraphHook({
      currentFactoryDocument: hookState.draftState.latestDocument ?? undefined,
    });

    expect(result.current.saveState.isStale).toBe(true);
    expect(result.current.saveState.canSave).toBe(false);
    expect(result.current.saveState.documentSave).toEqual({
      message:
        "The factory definition changed while you were editing. Refresh or discard your draft before saving.",
      status: "warning",
    });
  });

  it("drives save confirmation from scoped document save state", () => {
    setupEditableFactoryGraphSaveTestEnvironment(
      mockFactoryDocumentSave({ mode: "success" }),
    );
    hookState.draftState.hasChanges = true;

    const { result } = renderEditableFactoryGraphHook({
      currentFactoryDocument: currentFactoryDocument,
      factoryDocumentScopeKey: "session-graph",
    });

    act(() => {
      result.current.documentSaveControls.beginConfirmation();
    });

    expect(result.current.saveState.documentSave).toEqual({
      status: "confirming",
    });

    act(() => {
      result.current.documentSaveControls.cancelConfirmation();
    });

    expect(result.current.saveState.documentSave).toEqual({
      status: "idle",
    });
  });

  it("delegates save to scoped factory document save", async () => {
    const saveMutation = setupEditableFactoryGraphSaveTestEnvironment(
      mockFactoryDocumentSave({ mode: "success" }),
    );
    hookState.draftState.hasChanges = true;
    hookState.draftState.draft = {
      ...createEmptyFactoryGraphDraft(),
      additions: {
        ...createEmptyFactoryGraphDraft().additions,
        resources: [
          {
            capacity: 1,
            name: "review-slot",
          },
        ],
      },
    };

    const { result } = renderEditableFactoryGraphHook({
      currentFactoryDocument: currentFactoryDocument,
      factoryDocumentScopeKey: "session-graph",
    });

    let didSave = false;
    await act(async () => {
      didSave = await result.current.actions.save();
    });

    expect(didSave).toBe(true);
    expect(saveMutation.saveAsync).toHaveBeenCalledWith({
      baseVersion: currentFactoryDocument.version,
      factory: expect.objectContaining({
        resources: expect.arrayContaining([
          expect.objectContaining({ name: "review-slot" }),
        ]),
      }),
    });
    expect(hookState.draftState.replaceDraft).toHaveBeenCalledWith(
      createEmptyFactoryGraphDraft(),
    );
    await waitFor(() => {
      expect(result.current.saveState.documentSave).toEqual({
        status: "success",
      });
    });
  });

  it("does not save when scopeKey is missing", async () => {
    const saveMutation = setupEditableFactoryGraphSaveTestEnvironment(
      mockFactoryDocumentSave({ mode: "success" }),
    );
    hookState.draftState.hasChanges = true;

    const { result } = renderHook(
      () =>
        useEditableFactoryGraph({
          currentFactoryDocument: currentFactoryDocument,
          factoryDocumentScopeKey: null,
        }),
      {
        wrapper:
          createEditableFactoryGraphHookWrapper()
            .EditableFactoryGraphHookWrapper,
      },
    );

    let didSave = true;
    await act(async () => {
      didSave = await result.current.actions.save();
    });

    expect(didSave).toBe(false);
    expect(saveMutation.saveAsync).not.toHaveBeenCalled();
  });

  it("keeps the draft and exposes scoped save errors when persist fails", async () => {
    setupEditableFactoryGraphSaveTestEnvironment(
      mockFactoryDocumentSave({
        mode: "error",
        rejectedError: new Error("API unavailable"),
      }),
    );
    hookState.draftState.hasChanges = true;
    hookState.draftState.draft = {
      ...createEmptyFactoryGraphDraft(),
      additions: {
        ...createEmptyFactoryGraphDraft().additions,
        workers: [
          {
            model: "gpt-5-mini",
            name: "reviewer",
          },
        ],
      },
    };

    const { result } = renderEditableFactoryGraphHook({
      currentFactoryDocument: {
        ...baseFactoryDefinition,
        version: currentFactoryDocument.version,
      },
    });

    let didSave = true;
    await act(async () => {
      didSave = await result.current.actions.save();
    });

    expect(didSave).toBe(false);
    expect(hookState.draftState.hasChanges).toBe(true);
    await waitFor(() => {
      expect(result.current.saveState.documentSave).toEqual({
        errorMessage: "API unavailable",
        status: "error",
      });
    });
  });
});
