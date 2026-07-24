import { fireEvent, render, screen } from "@testing-library/react";
import { ReactFlowProvider } from "@xyflow/react";
import { describe, expect, it, vi } from "vitest";

import { FactoryGraphEdgeWaypointLayer } from "./factory-graph-edge-waypoint-layer";

const EDGE_ID = "edge-1";

function enablePointerCapture(element: HTMLElement) {
  element.setPointerCapture = vi.fn();
  element.releasePointerCapture = vi.fn();
  element.hasPointerCapture = vi.fn(() => true);
}

function renderWaypointLayer(
  props: Partial<
    React.ComponentProps<typeof FactoryGraphEdgeWaypointLayer>
  > = {},
) {
  const onMoveWaypoint = vi.fn();
  const onRemoveWaypoint = vi.fn();

  render(
    <ReactFlowProvider>
      <FactoryGraphEdgeWaypointLayer
        ariaLabel={(index) => `Waypoint ${index + 1}`}
        edgeId={EDGE_ID}
        onMoveWaypoint={onMoveWaypoint}
        onRemoveWaypoint={onRemoveWaypoint}
        waypoints={[{ x: 100, y: 50 }]}
        {...props}
      />
    </ReactFlowProvider>,
  );

  return { onMoveWaypoint, onRemoveWaypoint };
}

describe("FactoryGraphEdgeWaypointLayer", () => {
  it("updates preview positions during drag without committing layout history", () => {
    const { onMoveWaypoint } = renderWaypointLayer();

    const handle = screen.getByRole("button", { name: "Waypoint 1" });
    enablePointerCapture(handle);
    fireEvent.pointerDown(handle, {
      clientX: 200,
      clientY: 100,
      pointerId: 1,
    });
    fireEvent.pointerMove(handle, {
      clientX: 260,
      clientY: 140,
      pointerId: 1,
    });
    fireEvent.pointerMove(handle, {
      clientX: 320,
      clientY: 180,
      pointerId: 1,
    });

    expect(onMoveWaypoint).not.toHaveBeenCalled();
    expect(handle.style.left).toBe("320px");
    expect(handle.style.top).toBe("180px");
  });

  it("commits a single move on pointer up after drag coalescing", () => {
    const { onMoveWaypoint } = renderWaypointLayer();

    const handle = screen.getByRole("button", { name: "Waypoint 1" });
    enablePointerCapture(handle);
    fireEvent.pointerDown(handle, {
      clientX: 200,
      clientY: 100,
      pointerId: 1,
    });
    fireEvent.pointerMove(handle, {
      clientX: 260,
      clientY: 140,
      pointerId: 1,
    });
    fireEvent.pointerUp(handle, {
      clientX: 300,
      clientY: 160,
      pointerId: 1,
    });

    expect(onMoveWaypoint).toHaveBeenCalledTimes(1);
    expect(onMoveWaypoint).toHaveBeenCalledWith(EDGE_ID, 0, {
      x: 300,
      y: 160,
    });
  });

  it("removes a focused waypoint with Delete", () => {
    const { onRemoveWaypoint } = renderWaypointLayer();

    const handle = screen.getByRole("button", { name: "Waypoint 1" });
    fireEvent.keyDown(handle, { key: "Delete" });

    expect(onRemoveWaypoint).toHaveBeenCalledWith(EDGE_ID, 0);
  });
});
