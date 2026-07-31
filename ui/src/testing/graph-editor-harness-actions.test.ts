import { describe, expect, it } from "vitest";

import {
  createHookTestGraphEditorDraftState,
  createMockEditableFactoryGraph,
  createMockGraphEditorDraftState,
} from "./graph-editor-harness";

describe("graph-editor-harness graph actions", () => {
  it("createMockEditableFactoryGraph connects and disconnects semantic edges", () => {
    const draftState = createHookTestGraphEditorDraftState();
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      draftState,
    );

    const connected = graph.actions.connectNodes({
      sourceAnchorId: "workstation-on-failure-source",
      sourceNodeId: "workstation:draft",
      targetAnchorId: "work-state-input-target",
      targetNodeId: "work-state:story:done",
    });

    expect(connected.ok).toBe(true);
    expect(draftState.replaceDraft).toHaveBeenCalled();

    const disconnected = graph.actions.disconnectEdge(
      "workstation-on-failure:workstation:draft->work-state:story:done",
    );

    expect(disconnected.ok).toBe(true);
    expect(draftState.replaceDraft).toHaveBeenCalledTimes(2);
  });

  it("createMockEditableFactoryGraph returns false from save when draft validation fails", async () => {
    const draftState = createHookTestGraphEditorDraftState({
      hasChanges: true,
    });
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      draftState,
    );

    expect(
      graph.actions.addNode({
        argsText: "",
        command: "",
        kind: "worker",
        model: "gpt-5-mini",
        modelProvider: "CODEX",
        name: "reviewer",
        operations: [
          {
            inputs: [{ contentTypes: ["TEXT"], name: "text", required: true }],
            name: "REVIEW",
            outputs: [{ contentTypes: ["TEXT"], name: "result" }],
          },
        ],
        workerType: "MODEL_WORKER",
      }).ok,
    ).toBe(true);
    expect(
      graph.actions.disconnectEdge(
        "worker-assignment:worker:writer->workstation:draft",
      ).ok,
    ).toBe(true);

    await expect(graph.actions.save()).resolves.toBe(false);
  });

  it("createHookTestGraphEditorDraftState resets draft through resetDraft", () => {
    const draftState = createHookTestGraphEditorDraftState({
      hasChanges: true,
    });

    draftState.resetDraft();

    expect(draftState.hasChanges).toBe(false);
  });

  it("createMockEditableFactoryGraph removes graph nodes when the factory is loaded", () => {
    const draftState = createHookTestGraphEditorDraftState();
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      draftState,
    );

    const removed = graph.actions.removeNode("resource:gpu");

    expect(removed.ok).toBe(true);
    expect(draftState.replaceDraft).toHaveBeenCalled();
  });

  it("createMockEditableFactoryGraph rejects unsupported field updates", () => {
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      createMockGraphEditorDraftState(),
    );

    expect(graph.actions.updateNodeField()).toEqual({
      message: "Field editing is not exercised by this component test.",
      ok: false,
      reason: "INVALID_FIELD",
    });
  });
});
