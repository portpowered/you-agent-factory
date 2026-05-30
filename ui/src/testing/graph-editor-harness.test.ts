import { describe, expect, it, vi } from "vitest";

import {
  baseFactoryDefinitionDocument,
  createMockEditableFactoryGraph,
  createMockGraphEditorDraftState,
  wireMockEditableFactoryGraph,
  type MockEditableFactoryGraphHooks,
} from "./graph-editor-harness";

describe("graph-editor-harness", () => {
  it("createMockGraphEditorDraftState seeds empty draft state with factory fixtures", () => {
    const draftState = createMockGraphEditorDraftState();
    expect(draftState.baseDocument).toEqual(baseFactoryDefinitionDocument);
    expect(draftState.hasChanges).toBe(false);
    expect(draftState.validationErrors).toEqual([]);
  });

  it("createMockEditableFactoryGraph rejects graph edits without a loaded factory", () => {
    const draftState = createMockGraphEditorDraftState({
      baseDocument: null,
      latestDocument: null,
    });
    const graph = createMockEditableFactoryGraph({}, draftState);

    expect(graph.actions.addNode({ kind: "work-type", name: "story" })).toEqual({
      message: "Load the current factory before editing graph nodes.",
      ok: false,
      reason: "INVALID_FIELD",
    });
    expect(graph.saveState.canSave).toBe(false);
  });

  it("createMockEditableFactoryGraph saves pending definitions when changes are allowed", async () => {
    const saveFactoryDefinition = vi.fn().mockResolvedValue(undefined);
    const draftState = createMockGraphEditorDraftState({
      hasChanges: true,
      pendingFactoryDefinition: baseFactoryDefinitionDocument,
    });
    const graph = createMockEditableFactoryGraph(
      { saveFactoryDefinition },
      draftState,
    );

    await expect(graph.actions.save()).resolves.toBe(true);
    expect(saveFactoryDefinition).toHaveBeenCalledWith({
      baseVersion: baseFactoryDefinitionDocument.version,
      factoryDefinition: baseFactoryDefinitionDocument,
    });
    expect(draftState.replaceDraft).toHaveBeenCalled();
  });

  it("wireMockEditableFactoryGraph connects draft and editable graph hooks", () => {
    const hooks: MockEditableFactoryGraphHooks = {
      useEditableFactoryGraph: vi.fn(),
      useFactoryGraphDraftState: vi.fn(),
    };
    const draftState = wireMockEditableFactoryGraph(hooks);
    const options = { activeWorkCount: 0 };
    const graph = hooks.useEditableFactoryGraph(options);

    expect(hooks.useFactoryGraphDraftState).toHaveBeenCalledWith(options);
    expect(graph.draftState).toBe(draftState);
  });
});
