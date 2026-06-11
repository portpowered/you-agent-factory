import { describe, expect, it } from "vitest";

import {
  createMockEditableFactoryGraph,
  createMockGraphEditorDraftState,
} from "./graph-editor-harness";

describe("graph-editor-harness visual groups", () => {
  it("createMockEditableFactoryGraph wires visual group layout actions through graph actions", () => {
    const draftState = createMockGraphEditorDraftState();
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      draftState,
    );

    const createdGroup = graph.actions.createVisualGroup({ x: 120, y: 80 });
    expect(createdGroup?.id).toMatch(/^group-/);
    expect(graph.layoutDraftState.createVisualGroup).toHaveBeenCalledWith({
      x: 120,
      y: 80,
    });
    expect(graph.layoutDraftState.hasChanges).toBe(true);

    const groupId = createdGroup?.id ?? "group-1";
    graph.actions.renameVisualGroup(groupId, "Planning lane");
    graph.actions.setVisualGroupColor(groupId, "success");
    graph.actions.addNodeToVisualGroup(groupId, "workstation:draft");
    graph.actions.removeNodeFromVisualGroup(groupId, "workstation:draft");
    graph.actions.moveVisualGroupByDelta(
      groupId,
      { x: 12, y: 8 },
      new Map([["workstation:draft", { x: 40, y: 60 }]]),
    );
    graph.actions.resizeVisualGroup(groupId, {
      height: 220,
      width: 320,
      x: 10,
      y: 20,
    });
    graph.actions.deleteVisualGroup(groupId);

    expect(graph.layoutDraftState.renameVisualGroup).toHaveBeenCalledWith(
      groupId,
      "Planning lane",
    );
    expect(graph.layoutDraftState.setVisualGroupColor).toHaveBeenCalledWith(
      groupId,
      "success",
    );
    expect(graph.layoutDraftState.addNodeToVisualGroup).toHaveBeenCalledWith(
      groupId,
      "workstation:draft",
    );
    expect(graph.layoutDraftState.removeNodeFromVisualGroup).toHaveBeenCalledWith(
      groupId,
      "workstation:draft",
    );
    expect(graph.layoutDraftState.moveVisualGroupByDelta).toHaveBeenCalledWith(
      groupId,
      { x: 12, y: 8 },
      expect.any(Map),
    );
    expect(graph.layoutDraftState.resizeVisualGroup).toHaveBeenCalledWith(
      groupId,
      {
        height: 220,
        width: 320,
        x: 10,
        y: 20,
      },
    );
    expect(graph.layoutDraftState.deleteVisualGroup).toHaveBeenCalledWith(
      groupId,
    );
    expect(graph.layoutDraftState.layout.groups ?? []).toEqual([]);
  });
});
