import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { createDefaultFactoryLayout } from "../../lib/layout/factory-graph-layout-operations";
import {
  addFactoryLayoutGroup,
  createFactoryLayoutGroup,
  defaultFactoryLayoutGroupBounds,
} from "../../lib/layout/visual-groups/factory-graph-layout-groups";
import { useFactoryGraphVisualGroupEditor } from "./factory-graph-visual-group-editor-hook";

function layoutWithGroup() {
  return addFactoryLayoutGroup(
    createDefaultFactoryLayout(),
    createFactoryLayoutGroup({
      bounds: defaultFactoryLayoutGroupBounds({ x: 0, y: 0 }),
      color: "info",
      id: "group-1",
      label: "Review",
      nodeIds: ["workstation:draft"],
      layout: createDefaultFactoryLayout(),
    }),
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: visual group hook scenarios stay grouped around one hook contract.
describe("useFactoryGraphVisualGroupEditor", () => {
  it("creates, selects, renames, styles, assigns, moves, resizes, and deletes groups", () => {
    const createVisualGroup = vi.fn(() => ({ id: "group-new" }));
    const renameVisualGroup = vi.fn();
    const setVisualGroupColor = vi.fn();
    const addNodeToVisualGroup = vi.fn();
    const removeNodeFromVisualGroup = vi.fn();
    const moveVisualGroupByDelta = vi.fn();
    const resizeVisualGroup = vi.fn();
    const deleteVisualGroup = vi.fn();

    const { result, rerender } = renderHook(
      ({ layout }) =>
        useFactoryGraphVisualGroupEditor({
          activeTool: null,
          addNodeToVisualGroup,
          canInteractWithEditor: true,
          canvasNodeOptions: [
            { id: "workstation:draft", label: "Draft" },
            { id: "worker:writer", label: "Writer" },
          ],
          createVisualGroup,
          deleteVisualGroup,
          editorMode: true,
          layout,
          locale: "en",
          moveVisualGroupByDelta,
          removeNodeFromVisualGroup,
          renameVisualGroup,
          resizeVisualGroup,
          resolveViewportCenter: () => ({ x: 120, y: 80 }),
          setVisualGroupColor,
        }),
      { initialProps: { layout: layoutWithGroup() } },
    );

    act(() => {
      result.current.handleCreateVisualGroup();
    });
    expect(createVisualGroup).toHaveBeenCalledWith({ x: 120, y: 80 });
    expect(result.current.selectedGroupId).toBe("group-new");

    rerender({ layout: layoutWithGroup() });
    act(() => {
      result.current.handleSelectVisualGroup("group-1");
    });
    expect(result.current.selectedGroupId).toBe("group-1");
    expect(result.current.visualGroupControls).not.toBeNull();

    act(() => {
      result.current.handleRenameSelectedGroup("Planning");
    });
    expect(renameVisualGroup).toHaveBeenCalledWith("group-1", "Planning");

    act(() => {
      result.current.handleSetSelectedGroupColor("success");
    });
    expect(setVisualGroupColor).toHaveBeenCalledWith("group-1", "success");

    act(() => {
      result.current.visualGroupControls?.onToggleNodeMembership(
        "worker:writer",
        true,
      );
    });
    expect(addNodeToVisualGroup).toHaveBeenCalledWith(
      "group-1",
      "worker:writer",
    );

    act(() => {
      result.current.visualGroupControls?.onToggleNodeMembership(
        "workstation:draft",
        false,
      );
    });
    expect(removeNodeFromVisualGroup).toHaveBeenCalledWith(
      "group-1",
      "workstation:draft",
    );

    act(() => {
      result.current.handleMoveVisualGroup(
        "group-1",
        { x: 10, y: 5 },
        new Map([["workstation:draft", { x: 40, y: 60 }]]),
      );
    });
    expect(moveVisualGroupByDelta).toHaveBeenCalledWith(
      "group-1",
      { x: 10, y: 5 },
      expect.any(Map),
    );

    act(() => {
      result.current.handleResizeVisualGroup("group-1", {
        height: 200,
        width: 300,
        x: 0,
        y: 0,
      });
    });
    expect(resizeVisualGroup).toHaveBeenCalledWith("group-1", {
      height: 200,
      width: 300,
      x: 0,
      y: 0,
    });

    act(() => {
      result.current.handleDeleteSelectedGroup();
    });
    expect(deleteVisualGroup).toHaveBeenCalledWith("group-1");
    expect(result.current.selectedGroupId).toBeNull();
  });

  it("ignores edits when delete/add tools are active or editor mode is off", () => {
    const createVisualGroup = vi.fn(() => ({ id: "group-new" }));

    const { result } = renderHook(() =>
      useFactoryGraphVisualGroupEditor({
        activeTool: "delete",
        addNodeToVisualGroup: vi.fn(),
        canInteractWithEditor: true,
        canvasNodeOptions: [],
        createVisualGroup,
        deleteVisualGroup: vi.fn(),
        editorMode: true,
        layout: layoutWithGroup(),
        moveVisualGroupByDelta: vi.fn(),
        removeNodeFromVisualGroup: vi.fn(),
        renameVisualGroup: vi.fn(),
        resizeVisualGroup: vi.fn(),
        resolveViewportCenter: () => ({ x: 0, y: 0 }),
        setVisualGroupColor: vi.fn(),
      }),
    );

    act(() => {
      result.current.handleCreateVisualGroup();
      result.current.handleSelectVisualGroup("group-1");
    });

    expect(createVisualGroup).not.toHaveBeenCalled();
    expect(result.current.selectedGroupId).toBeNull();
    expect(result.current.canEditVisualGroups).toBe(false);
  });

  it("toggles selection off when the same group is selected again", () => {
    const { result } = renderHook(() =>
      useFactoryGraphVisualGroupEditor({
        activeTool: null,
        addNodeToVisualGroup: vi.fn(),
        canInteractWithEditor: true,
        canvasNodeOptions: [],
        createVisualGroup: vi.fn(() => null),
        deleteVisualGroup: vi.fn(),
        editorMode: true,
        layout: layoutWithGroup(),
        moveVisualGroupByDelta: vi.fn(),
        removeNodeFromVisualGroup: vi.fn(),
        renameVisualGroup: vi.fn(),
        resizeVisualGroup: vi.fn(),
        resolveViewportCenter: () => ({ x: 0, y: 0 }),
        setVisualGroupColor: vi.fn(),
      }),
    );

    act(() => {
      result.current.handleSelectVisualGroup("group-1");
    });
    expect(result.current.selectedGroupId).toBe("group-1");

    act(() => {
      result.current.handleSelectVisualGroup("group-1");
    });
    expect(result.current.selectedGroupId).toBeNull();
  });

  it("clears selection and skips create when viewport center is unavailable", () => {
    const createVisualGroup = vi.fn(() => ({ id: "group-new" }));
    const { result } = renderHook(() =>
      useFactoryGraphVisualGroupEditor({
        activeTool: null,
        addNodeToVisualGroup: vi.fn(),
        canInteractWithEditor: true,
        canvasNodeOptions: [],
        createVisualGroup,
        deleteVisualGroup: vi.fn(),
        editorMode: true,
        layout: layoutWithGroup(),
        moveVisualGroupByDelta: vi.fn(),
        removeNodeFromVisualGroup: vi.fn(),
        renameVisualGroup: vi.fn(),
        resizeVisualGroup: vi.fn(),
        resolveViewportCenter: () => null,
        setVisualGroupColor: vi.fn(),
      }),
    );

    act(() => {
      result.current.handleSelectVisualGroup("group-1");
      result.current.clearSelectedVisualGroup();
      result.current.handleCreateVisualGroup();
    });

    expect(result.current.selectedGroupId).toBeNull();
    expect(createVisualGroup).not.toHaveBeenCalled();
  });

  it("hides selected group controls when editor mode is off", () => {
    const { result } = renderHook(() =>
      useFactoryGraphVisualGroupEditor({
        activeTool: null,
        addNodeToVisualGroup: vi.fn(),
        canInteractWithEditor: true,
        canvasNodeOptions: [],
        createVisualGroup: vi.fn(() => null),
        deleteVisualGroup: vi.fn(),
        editorMode: false,
        layout: layoutWithGroup(),
        moveVisualGroupByDelta: vi.fn(),
        removeNodeFromVisualGroup: vi.fn(),
        renameVisualGroup: vi.fn(),
        resizeVisualGroup: vi.fn(),
        resolveViewportCenter: () => ({ x: 0, y: 0 }),
        setVisualGroupColor: vi.fn(),
      }),
    );

    act(() => {
      result.current.handleSelectVisualGroup("group-1");
    });

    expect(result.current.visualGroupControls).toBeNull();
    expect(result.current.canEditVisualGroups).toBe(false);
  });

  it("surfaces stale members and invalid bounds on the selected group controls", () => {
    const layout = addFactoryLayoutGroup(createDefaultFactoryLayout(), {
      bounds: {
        height: Number.NaN,
        width: 200,
        x: 0,
        y: 0,
      },
      color: "info",
      id: "group-1",
      label: "Review",
      nodeIds: ["workstation:missing"],
    });

    const { result } = renderHook(() =>
      useFactoryGraphVisualGroupEditor({
        activeTool: null,
        addNodeToVisualGroup: vi.fn(),
        canInteractWithEditor: true,
        canvasNodeOptions: [],
        createVisualGroup: vi.fn(() => null),
        deleteVisualGroup: vi.fn(),
        editorMode: true,
        layout,
        locale: "en",
        moveVisualGroupByDelta: vi.fn(),
        removeNodeFromVisualGroup: vi.fn(),
        renameVisualGroup: vi.fn(),
        resizeVisualGroup: vi.fn(),
        resolveViewportCenter: () => ({ x: 0, y: 0 }),
        setVisualGroupColor: vi.fn(),
      }),
    );

    act(() => {
      result.current.handleSelectVisualGroup("group-1");
    });

    expect(result.current.visualGroupControls?.staleMemberNodeIds).toEqual([
      "workstation:missing",
    ]);
    expect(result.current.visualGroupControls?.boundsError).toMatch(
      /non-finite/i,
    );
  });
});
