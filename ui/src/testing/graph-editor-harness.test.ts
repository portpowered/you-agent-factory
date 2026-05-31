import { describe, expect, it, vi } from "vitest";

import {
  baseFactoryDefinitionDocument,
  createHookTestGraphEditorDraftState,
  createMockEditableFactoryGraph,
  createMockGraphEditorDraftState,
  draftWorkstationFactoryDocument,
  mockEditableFactoryGraph,
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
    const draftState = createMockGraphEditorDraftState({
      hasChanges: true,
      pendingFactoryDefinition: baseFactoryDefinitionDocument,
    });
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      draftState,
    );

    await expect(graph.actions.save()).resolves.toBe(true);
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

  it("createMockEditableFactoryGraph rejects graph removal without a matching node", () => {
    const graph = createMockEditableFactoryGraph(
      {},
      createMockGraphEditorDraftState(),
    );

    expect(graph.actions.removeNode("missing-node")).toMatchObject({
      ok: false,
      reason: "NODE_NOT_FOUND",
    });
  });

  it("createMockEditableFactoryGraph blocks save when active work is in flight", async () => {
    const draftState = createMockGraphEditorDraftState({
      hasChanges: true,
      pendingFactoryDefinition: baseFactoryDefinitionDocument,
    });
    const graph = createMockEditableFactoryGraph(
      { activeWorkCount: 2, factoryDocumentScopeKey: "session-default" },
      draftState,
    );

    await expect(graph.actions.save()).resolves.toBe(false);
    expect(graph.saveState.canSave).toBe(false);
  });

  it("mockEditableFactoryGraph alias matches createMockEditableFactoryGraph", () => {
    expect(mockEditableFactoryGraph).toBe(createMockEditableFactoryGraph);
  });

  it("createHookTestGraphEditorDraftState seeds draft workstation topology", () => {
    const draftState = createHookTestGraphEditorDraftState();
    expect(draftState.baseDocument).toEqual(draftWorkstationFactoryDocument);
    expect(draftState.graph.nodes.map((node) => node.id)).toContain(
      "workstation:draft",
    );
  });

  it("createHookTestGraphEditorDraftState mutates draft on replaceDraft", () => {
    const draftState = createHookTestGraphEditorDraftState();
    const nextDraft = createMockGraphEditorDraftState().draft;

    draftState.replaceDraft(nextDraft);

    expect(draftState.draft).toBe(nextDraft);
    expect(draftState.hasChanges).toBe(true);
  });

  it("createMockEditableFactoryGraph marks save state stale when versions diverge", () => {
    const draftState = createMockGraphEditorDraftState({
      baseDocument: baseFactoryDefinitionDocument,
      hasChanges: true,
      latestDocument: {
        ...baseFactoryDefinitionDocument,
        version: {
          logical: "9",
          physical: "2026-05-18T15:32:00.001Z",
        },
      },
    });
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      draftState,
    );

    expect(graph.saveState.isStale).toBe(true);
    expect(graph.saveState.canSave).toBe(false);
  });
});
