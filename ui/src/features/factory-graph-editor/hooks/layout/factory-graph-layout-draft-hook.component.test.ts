// biome-ignore lint/style/noExcessiveLinesPerFile: layout draft hook scenarios stay grouped for undo coverage.
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { baseFactoryDefinition } from "../../lib/draft/factory-graph-draft.test-helpers";
import { factoryLayoutEdgeWaypoints } from "../../lib/layout/factory-graph-layout-edge-waypoints";
import {
  createDefaultFactoryLayout,
  factoryLayoutNodePosition,
  moveFactoryLayoutNode,
} from "../../lib/layout/factory-graph-layout-operations";
import {
  addFactoryLayoutGroup,
  addNodeToFactoryLayoutGroup,
  defaultFactoryLayoutGroupBounds,
  factoryLayoutGroupById,
} from "../../lib/layout/visual-groups/factory-graph-layout-groups";
import { useFactoryGraphLayoutDraftState } from "./factory-graph-layout-draft-hook";

const EDGE_ID = "workstation-output:workstation:draft->work-state:story:done";

describe("useFactoryGraphLayoutDraftState movement history", () => {
  it("records layout commands and supports undo and redo", () => {
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: baseFactoryDefinition,
        factoryDocumentScopeKey: "session-1",
      }),
    );

    act(() => {
      result.current.moveNode("workstation:draft", { x: 120, y: 160 });
    });

    expect(result.current.canUndoLayout).toBe(true);
    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 120,
      y: 160,
    });

    act(() => {
      result.current.undoLayout();
    });

    expect(result.current.canRedoLayout).toBe(true);
    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toBeUndefined();

    act(() => {
      result.current.redoLayout();
    });

    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 120,
      y: 160,
    });
  });

  it("clears history when discarding layout without recording a reset command", () => {
    const layoutDocument = {
      ...baseFactoryDefinition,
      layout: moveFactoryLayoutNode(
        baseFactoryDefinition.layout ?? {},
        "workstation:draft",
        {
          x: 40,
          y: 80,
        },
      ),
    };
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: layoutDocument,
        factoryDocumentScopeKey: "session-2",
      }),
    );

    act(() => {
      result.current.moveNode("workstation:draft", { x: 120, y: 160 });
      result.current.resetLayout({ recordHistory: false });
    });

    expect(result.current.canUndoLayout).toBe(false);
    expect(result.current.canRedoLayout).toBe(false);
  });
});

describe("useFactoryGraphLayoutDraftState viewport and multi-node moves", () => {
  it("records viewport updates with undo and redo", () => {
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: baseFactoryDefinition,
        factoryDocumentScopeKey: "session-viewport",
      }),
    );

    act(() => {
      result.current.updateViewport({ x: 40, y: 60, zoom: 1.5 });
    });

    expect(result.current.layout.viewport).toEqual({ x: 40, y: 60, zoom: 1.5 });
    expect(result.current.layoutDirty).toBe(true);

    act(() => {
      result.current.undoLayout();
    });

    expect(result.current.layout.viewport).toBeUndefined();
    expect(result.current.canRedoLayout).toBe(true);

    act(() => {
      result.current.redoLayout();
    });

    expect(result.current.layout.viewport).toEqual({ x: 40, y: 60, zoom: 1.5 });
  });

  it("moves multiple nodes by delta and supports undo", () => {
    const layoutDocument = {
      ...baseFactoryDefinition,
      layout: moveFactoryLayoutNode(
        baseFactoryDefinition.layout ?? {},
        "workstation:draft",
        {
          x: 100,
          y: 200,
        },
      ),
    };
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: layoutDocument,
        factoryDocumentScopeKey: "session-multi",
      }),
    );
    const resolvedPositions = new Map([
      ["workstation:draft", { x: 100, y: 200 }],
    ]);

    act(() => {
      result.current.moveNodesByDelta(
        ["workstation:draft"],
        { x: 20, y: -10 },
        resolvedPositions,
      );
    });

    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 120,
      y: 190,
    });

    act(() => {
      result.current.undoLayout();
    });

    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 100,
      y: 200,
    });
  });
});

describe("useFactoryGraphLayoutDraftState reset and adoption", () => {
  it("records reset layout in history when recordHistory is enabled", () => {
    const layoutDocument = {
      ...baseFactoryDefinition,
      layout: moveFactoryLayoutNode(
        baseFactoryDefinition.layout ?? {},
        "workstation:draft",
        {
          x: 40,
          y: 80,
        },
      ),
    };
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: layoutDocument,
        factoryDocumentScopeKey: "session-reset",
      }),
    );

    act(() => {
      result.current.moveNode("workstation:draft", { x: 120, y: 160 });
      result.current.resetLayout();
    });

    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 40,
      y: 80,
    });
    expect(result.current.canUndoLayout).toBe(true);

    act(() => {
      result.current.undoLayout();
    });

    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 120,
      y: 160,
    });
  });

  it("adopts saved layout and clears dirty state", () => {
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: baseFactoryDefinition,
        factoryDocumentScopeKey: "session-adopt",
      }),
    );
    const savedLayout = moveFactoryLayoutNode(
      baseFactoryDefinition.layout ?? {},
      "workstation:draft",
      {
        x: 300,
        y: 400,
      },
    );

    act(() => {
      result.current.moveNode("workstation:draft", { x: 10, y: 20 });
      result.current.adoptSavedLayout(savedLayout);
    });

    expect(result.current.layoutDirty).toBe(false);
    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 300,
      y: 400,
    });
    expect(result.current.canUndoLayout).toBe(false);
  });

  it("replaces layout and clears undo history", () => {
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: baseFactoryDefinition,
        factoryDocumentScopeKey: "session-replace",
      }),
    );
    const replacementLayout = moveFactoryLayoutNode(
      baseFactoryDefinition.layout ?? {},
      "workstation:draft",
      { x: 55, y: 66 },
    );

    act(() => {
      result.current.moveNode("workstation:draft", { x: 1, y: 2 });
      result.current.replaceLayout(replacementLayout);
    });

    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 55,
      y: 66,
    });
    expect(result.current.canUndoLayout).toBe(false);
  });
});

describe("useFactoryGraphLayoutDraftState edge waypoint edits", () => {
  it("marks layout dirty without topology changes when removing waypoints", () => {
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: baseFactoryDefinition,
        factoryDocumentScopeKey: "session-waypoint-remove",
      }),
    );

    act(() => {
      result.current.addEdgeWaypoint(EDGE_ID, { x: 10, y: 20 });
      result.current.addEdgeWaypoint(EDGE_ID, { x: 30, y: 40 });
    });

    expect(result.current.layoutDirty).toBe(true);
    expect(factoryLayoutEdgeWaypoints(result.current.layout, EDGE_ID)).toEqual([
      { x: 10, y: 20 },
      { x: 30, y: 40 },
    ]);

    act(() => {
      result.current.removeEdgeWaypoint(EDGE_ID, 0);
    });

    expect(result.current.layoutDirty).toBe(true);
    expect(factoryLayoutEdgeWaypoints(result.current.layout, EDGE_ID)).toEqual([
      { x: 30, y: 40 },
    ]);

    act(() => {
      result.current.removeEdgeWaypoint(EDGE_ID, 0);
    });

    expect(result.current.layoutDirty).toBe(false);
    expect(
      factoryLayoutEdgeWaypoints(result.current.layout, EDGE_ID),
    ).toBeUndefined();
  });

  it("records waypoint add, move, and remove commands with undo and redo", () => {
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: baseFactoryDefinition,
        factoryDocumentScopeKey: "session-waypoint-history",
      }),
    );

    act(() => {
      result.current.addEdgeWaypoint(EDGE_ID, { x: 10, y: 20 });
      result.current.addEdgeWaypoint(EDGE_ID, { x: 30, y: 40 });
      result.current.moveEdgeWaypoint(EDGE_ID, 1, { x: 35, y: 45 });
    });

    expect(result.current.layoutDirty).toBe(true);
    expect(factoryLayoutEdgeWaypoints(result.current.layout, EDGE_ID)).toEqual([
      { x: 10, y: 20 },
      { x: 35, y: 45 },
    ]);
    expect(result.current.canUndoLayout).toBe(true);

    act(() => {
      result.current.undoLayout();
    });

    expect(factoryLayoutEdgeWaypoints(result.current.layout, EDGE_ID)).toEqual([
      { x: 10, y: 20 },
      { x: 30, y: 40 },
    ]);
    expect(result.current.canRedoLayout).toBe(true);

    act(() => {
      result.current.undoLayout();
      result.current.undoLayout();
    });

    expect(
      factoryLayoutEdgeWaypoints(result.current.layout, EDGE_ID),
    ).toBeUndefined();
    expect(result.current.layoutDirty).toBe(false);

    act(() => {
      result.current.redoLayout();
      result.current.redoLayout();
      result.current.redoLayout();
    });

    expect(factoryLayoutEdgeWaypoints(result.current.layout, EDGE_ID)).toEqual([
      { x: 10, y: 20 },
      { x: 35, y: 45 },
    ]);
    expect(result.current.layoutDirty).toBe(true);

    act(() => {
      result.current.removeEdgeWaypoint(EDGE_ID, 0);
      result.current.undoLayout();
    });

    expect(factoryLayoutEdgeWaypoints(result.current.layout, EDGE_ID)).toEqual([
      { x: 10, y: 20 },
      { x: 35, y: 45 },
    ]);
  });

  it("adopts saved waypoint layout after reload without topology dirty state", () => {
    const savedLayoutDocument = {
      ...baseFactoryDefinition,
      layout: {
        schemaVersion: 1,
        edges: [
          {
            id: EDGE_ID,
            waypoints: [{ x: 180, y: 220 }],
          },
        ],
      },
    };
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: baseFactoryDefinition,
        factoryDocumentScopeKey: "session-waypoint-reload",
      }),
    );

    const savedLayout = savedLayoutDocument.layout;
    if (!savedLayout) {
      throw new Error("Expected saved layout fixture.");
    }

    act(() => {
      result.current.addEdgeWaypoint(EDGE_ID, { x: 10, y: 20 });
      result.current.adoptSavedLayout(savedLayout);
    });

    expect(result.current.layoutDirty).toBe(false);
    expect(factoryLayoutEdgeWaypoints(result.current.layout, EDGE_ID)).toEqual([
      { x: 180, y: 220 },
    ]);
  });
});

describe("useFactoryGraphLayoutDraftState scope and pruning", () => {
  it("prunes layout history for deleted node ids", () => {
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: baseFactoryDefinition,
        factoryDocumentScopeKey: "session-prune",
      }),
    );

    act(() => {
      result.current.moveNode("workstation:draft", { x: 80, y: 90 });
      result.current.pruneLayoutHistoryForNodeIds([]);
    });

    expect(result.current.canUndoLayout).toBe(false);
    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 80,
      y: 90,
    });
  });

  it("resets session state when the factory document scope changes", () => {
    const { result, rerender } = renderHook(
      ({ scopeKey }) =>
        useFactoryGraphLayoutDraftState({
          currentFactoryDocument: baseFactoryDefinition,
          factoryDocumentScopeKey: scopeKey,
        }),
      { initialProps: { scopeKey: "scope-a" } },
    );

    act(() => {
      result.current.moveNode("workstation:draft", { x: 12, y: 34 });
    });
    expect(result.current.layoutDirty).toBe(true);

    rerender({ scopeKey: "scope-b" });

    expect(result.current.layoutDirty).toBe(false);
    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toBeUndefined();
    expect(result.current.canUndoLayout).toBe(false);
  });
});

describe("useFactoryGraphLayoutDraftState visual group create rename style", () => {
  it("records group create, rename, and style changes with undo and redo", () => {
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: baseFactoryDefinition,
        factoryDocumentScopeKey: "session-group-style",
      }),
    );

    act(() => {
      result.current.createVisualGroup({ x: 200, y: 150 });
    });

    const createdGroup = factoryLayoutGroupById(
      result.current.layout,
      "group-1",
    );
    expect(createdGroup).toMatchObject({
      id: "group-1",
      label: "Group 1",
      nodeIds: [],
    });
    expect(result.current.layoutDirty).toBe(true);

    act(() => {
      result.current.renameVisualGroup("group-1", "Planning");
      result.current.setVisualGroupColor("group-1", "warning");
    });

    expect(
      factoryLayoutGroupById(result.current.layout, "group-1"),
    ).toMatchObject({
      color: "warning",
      label: "Planning",
    });

    act(() => {
      result.current.undoLayout();
      result.current.undoLayout();
    });

    expect(
      factoryLayoutGroupById(result.current.layout, "group-1"),
    ).toMatchObject({
      color: "primary",
      label: "Group 1",
    });

    act(() => {
      result.current.undoLayout();
    });

    expect(result.current.layout.groups ?? []).toEqual([]);

    act(() => {
      result.current.redoLayout();
    });

    expect(factoryLayoutGroupById(result.current.layout, "group-1")?.id).toBe(
      "group-1",
    );
  });
});

describe("useFactoryGraphLayoutDraftState visual group save reload", () => {
  it("adopts saved visual group layout after reload without topology dirty state", () => {
    const savedLayoutDocument = {
      ...baseFactoryDefinition,
      layout: addNodeToFactoryLayoutGroup(
        addFactoryLayoutGroup(createDefaultFactoryLayout(), {
          bounds: defaultFactoryLayoutGroupBounds({ x: 120, y: 80 }),
          color: "success",
          id: "group-1",
          label: "Review lane",
          locked: false,
          nodeIds: [],
          parentGroupId: null,
        }),
        "group-1",
        "workstation:draft",
      ),
    };
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: baseFactoryDefinition,
        factoryDocumentScopeKey: "session-group-reload",
      }),
    );

    const savedLayout = savedLayoutDocument.layout;
    if (!savedLayout) {
      throw new Error("Expected saved layout fixture.");
    }

    act(() => {
      result.current.createVisualGroup({ x: 0, y: 0 });
      result.current.adoptSavedLayout(savedLayout);
    });

    expect(result.current.layoutDirty).toBe(false);
    expect(factoryLayoutGroupById(result.current.layout, "group-1")).toEqual({
      bounds: defaultFactoryLayoutGroupBounds({ x: 120, y: 80 }),
      color: "success",
      id: "group-1",
      label: "Review lane",
      locked: false,
      nodeIds: ["workstation:draft"],
      parentGroupId: null,
    });
    expect(result.current.canUndoLayout).toBe(false);
  });
});

describe("useFactoryGraphLayoutDraftState visual group membership", () => {
  it("records group membership changes with undo and redo", () => {
    const layoutDocument = {
      ...baseFactoryDefinition,
      layout: addFactoryLayoutGroup(createDefaultFactoryLayout(), {
        bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
        id: "group-1",
        label: "Review",
        nodeIds: [],
      }),
    };
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: layoutDocument,
        factoryDocumentScopeKey: "session-group-membership",
      }),
    );

    act(() => {
      result.current.addNodeToVisualGroup("group-1", "workstation:draft");
    });

    expect(
      factoryLayoutGroupById(result.current.layout, "group-1")?.nodeIds,
    ).toEqual(["workstation:draft"]);
    expect(result.current.layoutDirty).toBe(true);
    expect(result.current.canUndoLayout).toBe(true);

    act(() => {
      result.current.undoLayout();
    });

    expect(
      factoryLayoutGroupById(result.current.layout, "group-1")?.nodeIds,
    ).toEqual([]);

    act(() => {
      result.current.redoLayout();
    });

    expect(
      factoryLayoutGroupById(result.current.layout, "group-1")?.nodeIds,
    ).toEqual(["workstation:draft"]);

    act(() => {
      result.current.removeNodeFromVisualGroup("group-1", "workstation:draft");
    });

    expect(
      factoryLayoutGroupById(result.current.layout, "group-1")?.nodeIds,
    ).toEqual([]);
  });
});

describe("useFactoryGraphLayoutDraftState visual group geometry", () => {
  it("records group move, resize, and delete with undo and redo", () => {
    const layoutDocument = {
      ...baseFactoryDefinition,
      layout: addNodeToFactoryLayoutGroup(
        moveFactoryLayoutNode(
          addFactoryLayoutGroup(createDefaultFactoryLayout(), {
            bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
            id: "group-1",
            label: "Review",
            nodeIds: [],
          }),
          "workstation:draft",
          { x: 40, y: 60 },
        ),
        "group-1",
        "workstation:draft",
      ),
    };
    const { result } = renderHook(() =>
      useFactoryGraphLayoutDraftState({
        currentFactoryDocument: layoutDocument,
        factoryDocumentScopeKey: "session-group-geometry",
      }),
    );

    act(() => {
      result.current.moveVisualGroupByDelta("group-1", { x: 10, y: 5 });
    });

    expect(
      factoryLayoutGroupById(result.current.layout, "group-1")?.bounds,
    ).toEqual({
      height: 320,
      width: 480,
      x: -230,
      y: -155,
    });
    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 50,
      y: 65,
    });

    act(() => {
      result.current.undoLayout();
    });

    expect(
      factoryLayoutGroupById(result.current.layout, "group-1")?.bounds,
    ).toEqual(defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }));

    act(() => {
      result.current.redoLayout();
      result.current.resizeVisualGroup("group-1", {
        height: 200,
        width: 300,
        x: 0,
        y: 0,
      });
    });

    expect(
      factoryLayoutGroupById(result.current.layout, "group-1")?.bounds,
    ).toEqual({
      height: 200,
      width: 300,
      x: 0,
      y: 0,
    });
    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 50,
      y: 65,
    });

    act(() => {
      result.current.deleteVisualGroup("group-1");
    });

    expect(result.current.layout.groups).toBeUndefined();
    expect(
      factoryLayoutNodePosition(result.current.layout, "workstation:draft"),
    ).toEqual({
      x: 50,
      y: 65,
    });

    act(() => {
      result.current.undoLayout();
    });

    expect(factoryLayoutGroupById(result.current.layout, "group-1")?.id).toBe(
      "group-1",
    );
  });
});
// Component lane: requires DOM APIs.
