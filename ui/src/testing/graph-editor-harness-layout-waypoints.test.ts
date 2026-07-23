import { describe, expect, it } from "vitest";

import {
  buildMockGraphSavePayload,
  createHookTestGraphEditorDraftState,
  createMockEditableFactoryGraph,
  createMockGraphEditorDraftState,
} from "./graph-editor-harness";

describe("graph-editor-harness layout waypoint helpers", () => {
  it("createMockEditableFactoryGraph mutates edge waypoint layout through harness actions", () => {
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      createMockGraphEditorDraftState(),
    );
    const edgeId =
      "workstation-output:workstation:draft->work-state:story:done";

    graph.actions.addEdgeWaypoint(edgeId, { x: 100, y: 50 }, 0);
    expect(graph.layoutDraftState.layoutDirty).toBe(true);
    expect(
      graph.layoutDraftState.layout.edges?.find((edge) => edge.id === edgeId)
        ?.waypoints,
    ).toEqual([{ x: 100, y: 50 }]);

    graph.actions.moveEdgeWaypoint(edgeId, 0, { x: 110, y: 60 });
    expect(
      graph.layoutDraftState.layout.edges?.find((edge) => edge.id === edgeId)
        ?.waypoints,
    ).toEqual([{ x: 110, y: 60 }]);

    graph.actions.removeEdgeWaypoint(edgeId, 0);
    expect(
      graph.layoutDraftState.layout.edges?.find((edge) => edge.id === edgeId)
        ?.waypoints,
    ).toBeUndefined();
  });

  it("createMockLayoutDraftState mutates edge waypoints directly", () => {
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      createMockGraphEditorDraftState(),
    );
    const { layoutDraftState } = graph;
    const edgeId =
      "workstation-output:workstation:draft->work-state:story:done";

    layoutDraftState.addEdgeWaypoint(edgeId, { x: 10, y: 20 });
    layoutDraftState.moveEdgeWaypoint(edgeId, 0, { x: 30, y: 40 });
    layoutDraftState.removeEdgeWaypoint(edgeId, 0);

    expect(layoutDraftState.layoutDirty).toBe(true);
    expect(layoutDraftState.layout.edges).toBeUndefined();
  });

  it("buildMockGraphSavePayload throws when pending edits cannot be applied", () => {
    const draftState = createHookTestGraphEditorDraftState({
      hasChanges: true,
    });
    const graph = createMockEditableFactoryGraph(
      { factoryDocumentScopeKey: "session-default" },
      draftState,
    );

    expect(() =>
      buildMockGraphSavePayload({
        draft: draftState.draft,
        latestDocument: draftState.latestDocument,
      }),
    ).not.toThrow();

    graph.actions.disconnectEdge(
      "worker-assignment:worker:writer->workstation:draft",
    );

    expect(() =>
      buildMockGraphSavePayload({
        draft: draftState.draft,
        latestDocument: draftState.latestDocument,
      }),
    ).toThrow("Expected mock graph draft to produce a save payload.");
  });
});
