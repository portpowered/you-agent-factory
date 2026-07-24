import { describe, expect, it, vi } from "vitest";

import { useFactoryDocumentSave } from "../features/current-factory-definition/hooks/useFactoryDocumentSave";
import { mockFactoryDocumentSave } from "./factory-document-save-mocks";
import {
  baseFactoryDefinitionDocument,
  createMockEditableFactoryGraph,
  createMockGraphEditorDraftState,
} from "./graph-editor-harness";

vi.mock(
  "../features/current-factory-definition/hooks/useFactoryDocumentSave",
  () => ({
    useFactoryDocumentSave: vi.fn(),
  }),
);

describe("graph-editor-harness layout and mutation helpers", () => {
  it("createMockEditableFactoryGraph saves layout-only edits and resets layout on discard", async () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
    vi.mocked(useFactoryDocumentSave).mockReturnValue(saveMutation);

    const draftState = createMockGraphEditorDraftState({
      pendingFactoryDefinition: baseFactoryDefinitionDocument,
    });
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      draftState,
    );

    graph.actions.moveLayoutNode("worker:writer", { x: 120, y: 240 });
    expect(graph.layoutDraftState.hasChanges).toBe(true);

    await expect(graph.actions.save()).resolves.toBe(true);
    expect(saveMutation.saveAsync).toHaveBeenCalled();
    expect(graph.layoutDraftState.adoptSavedLayout).toHaveBeenCalled();

    graph.actions.moveLayoutNode("worker:writer", { x: 40, y: 80 });
    graph.actions.discard();
    expect(graph.layoutDraftState.resetLayout).toHaveBeenCalled();
    expect(draftState.resetDraft).toHaveBeenCalled();
  });

  it("createMockEditableFactoryGraph saves visual group layout edits", async () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
    vi.mocked(useFactoryDocumentSave).mockReturnValue(saveMutation);

    const draftState = createMockGraphEditorDraftState({
      pendingFactoryDefinition: baseFactoryDefinitionDocument,
    });
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      draftState,
    );

    graph.actions.createVisualGroup({ x: 80, y: 120 });
    expect(graph.layoutDraftState.hasChanges).toBe(true);

    await expect(graph.actions.save()).resolves.toBe(true);
    expect(saveMutation.saveAsync).toHaveBeenCalled();
    expect(graph.layoutDraftState.adoptSavedLayout).toHaveBeenCalled();
  });

  it("createMockEditableFactoryGraph wires document save controls and graph mutations", () => {
    const draftState = createMockGraphEditorDraftState();
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      draftState,
    );

    graph.documentSaveControls.beginConfirmation();
    expect(draftState.documentSave).toEqual({ status: "confirming" });

    graph.documentSaveControls.cancelConfirmation();
    expect(draftState.documentSave).toEqual({ status: "idle" });

    graph.documentSaveControls.clearSaveFeedback();
    expect(draftState.documentSave).toEqual({ status: "idle" });

    expect(
      graph.actions.updateLayoutViewport({ x: 1, y: 2, zoom: 0.5 }),
    ).toBeUndefined();
    expect(graph.layoutDraftState.updateViewport).toHaveBeenCalledWith({
      x: 1,
      y: 2,
      zoom: 0.5,
    });
  });

  it("createMockEditableFactoryGraph rejects connect and disconnect without a loaded factory", () => {
    const draftState = createMockGraphEditorDraftState({
      baseDocument: null,
      latestDocument: null,
    });
    const graph = createMockEditableFactoryGraph({}, draftState);

    expect(
      graph.actions.connectNodes({
        sourceId: "worker:writer",
        targetId: "workstation:review",
      }),
    ).toEqual({
      message: "Load the current factory before connecting graph nodes.",
      ok: false,
      reason: "INVALID_CONNECTION",
    });
    expect(graph.actions.disconnectEdge("missing-edge")).toEqual({
      message: "Load the current factory before disconnecting graph edges.",
      ok: false,
      reason: "UNKNOWN_EDGE",
    });
  });
});
