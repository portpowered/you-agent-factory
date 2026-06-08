import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { createDefaultFactoryLayout } from "../../lib/layout/factory-graph-layout-operations";
import { useFactoryGraphEdgeWaypointEditor } from "./factory-graph-edge-waypoint-editor-hook";

const EDGE_ID =
  "workstation-output:workstation:review->work-state:story:done";

describe("useFactoryGraphEdgeWaypointEditor", () => {
  it("selects edges for waypoint editing and adds waypoints through layout actions", () => {
    const addEdgeWaypoint = vi.fn();
    const moveEdgeWaypoint = vi.fn();

    const { result } = renderHook(() =>
      useFactoryGraphEdgeWaypointEditor({
        activeTool: null,
        addEdgeWaypoint,
        canInteractWithEditor: true,
        editorMode: true,
        handleEditorEdgeDelete: vi.fn(),
        layout: createDefaultFactoryLayout(),
        locale: "en",
        moveEdgeWaypoint,
        nodes: [
          {
            id: "workstation:review",
            data: {},
            position: { x: 0, y: 0 },
            type: "factoryEntity",
          },
          {
            id: "work-state:story:done",
            data: {},
            position: { x: 200, y: 100 },
            type: "factoryEntity",
          },
        ],
      }),
    );

    act(() => {
      result.current.handleEditorEdgeClick(EDGE_ID);
    });

    expect(result.current.selectedWaypointEdgeId).toBe(EDGE_ID);
    expect(result.current.waypointControls).not.toBeNull();

    act(() => {
      result.current.handleAddSelectedEdgeWaypoint();
    });

    expect(addEdgeWaypoint).toHaveBeenCalledWith(EDGE_ID, { x: 100, y: 50 });
  });

  it("routes delete-tool edge clicks to edge deletion", () => {
    const handleEditorEdgeDelete = vi.fn();

    const { result } = renderHook(() =>
      useFactoryGraphEdgeWaypointEditor({
        activeTool: "delete",
        addEdgeWaypoint: vi.fn(),
        canInteractWithEditor: true,
        editorMode: true,
        handleEditorEdgeDelete,
        layout: createDefaultFactoryLayout(),
        locale: "en",
        moveEdgeWaypoint: vi.fn(),
        nodes: [],
      }),
    );

    act(() => {
      result.current.handleEditorEdgeClick(EDGE_ID);
    });

    expect(handleEditorEdgeDelete).toHaveBeenCalledWith(EDGE_ID);
    expect(result.current.selectedWaypointEdgeId).toBeNull();
  });
});
