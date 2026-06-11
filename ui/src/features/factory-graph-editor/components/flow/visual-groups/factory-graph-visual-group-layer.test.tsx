import "@testing-library/jest-dom/vitest";
import { ReactFlow, ReactFlowProvider } from "@xyflow/react";
import { fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps, CSSProperties } from "react";
import { describe, expect, it, vi } from "vitest";

import type { FactoryLayoutGroup } from "../../../lib/layout/visual-groups/factory-graph-layout-groups";
import { FactoryGraphVisualGroupLayer } from "./factory-graph-visual-group-layer";

const sampleGroup: FactoryLayoutGroup = {
  bounds: { height: 120, width: 200, x: 40, y: 60 },
  color: "info",
  id: "group-1",
  label: "Review",
  nodeIds: [],
};

function enablePointerCapture(element: HTMLElement) {
  element.setPointerCapture = vi.fn();
  element.releasePointerCapture = vi.fn();
  element.hasPointerCapture = vi.fn(() => true);
}

function renderVisualGroupLayer(
  props: Partial<ComponentProps<typeof FactoryGraphVisualGroupLayer>> = {},
  wrapperStyle: CSSProperties = { height: 480, width: 640 },
) {
  const onSelectGroup = props.onSelectGroup ?? vi.fn();

  render(
    <ReactFlowProvider>
      <div style={wrapperStyle}>
        <ReactFlow
          defaultViewport={{ x: 0, y: 0, zoom: 1 }}
          edges={[]}
          fitView={wrapperStyle.position !== "relative"}
          nodes={[]}
        >
          <FactoryGraphVisualGroupLayer
            canEdit
            groupAriaLabel={(group) => group.label ?? group.id}
            groups={[sampleGroup]}
            onSelectGroup={onSelectGroup}
            resizeHandleAriaLabel={(corner) => `Resize ${corner}`}
            {...props}
          />
        </ReactFlow>
      </div>
    </ReactFlowProvider>,
  );

  return { onSelectGroup };
}

describe("FactoryGraphVisualGroupLayer", () => {
  it("renders groups behind interaction handles with labels and selection state", () => {
    renderVisualGroupLayer({
      onResizeGroup: vi.fn(),
      selectedGroupId: "group-1",
    });

    expect(screen.getByText("Review")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Review" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(
      screen.getByRole("button", { name: "Resize se" }),
    ).toBeInTheDocument();
  });

  it("selects a group when Enter or Space is pressed on the group body", () => {
    const { onSelectGroup } = renderVisualGroupLayer({ selectedGroupId: null });
    const groupBody = screen.getByRole("button", { name: "Review" });

    groupBody.focus();
    fireEvent.keyDown(groupBody, { key: "Enter" });
    expect(onSelectGroup).toHaveBeenCalledWith("group-1");

    onSelectGroup.mockClear();
    fireEvent.keyDown(groupBody, { key: " " });
    expect(onSelectGroup).toHaveBeenCalledWith("group-1");
  });

  it("commits group move and resize interactions on pointer up", () => {
    const onMoveGroup = vi.fn();
    const onResizeGroup = vi.fn();

    renderVisualGroupLayer(
      {
        onMoveGroup,
        onResizeGroup,
        selectedGroupId: "group-1",
      },
      { height: 480, position: "relative", width: 640 },
    );

    const groupBody = screen.getByRole("button", { name: "Review" });
    enablePointerCapture(groupBody);
    fireEvent.pointerDown(groupBody, {
      clientX: 100,
      clientY: 120,
      pointerId: 1,
    });
    fireEvent.pointerMove(groupBody, {
      clientX: 140,
      clientY: 150,
      pointerId: 1,
    });
    fireEvent.pointerUp(groupBody, {
      clientX: 140,
      clientY: 150,
      pointerId: 1,
    });

    expect(onMoveGroup).toHaveBeenCalledWith(
      "group-1",
      expect.objectContaining({ x: expect.any(Number), y: expect.any(Number) }),
      expect.any(Map),
    );

    const resizeHandle = screen.getByRole("button", { name: "Resize se" });
    enablePointerCapture(resizeHandle);
    fireEvent.pointerDown(resizeHandle, {
      clientX: 240,
      clientY: 180,
      pointerId: 2,
    });
    fireEvent.pointerMove(resizeHandle, {
      clientX: 280,
      clientY: 220,
      pointerId: 2,
    });
    fireEvent.pointerUp(resizeHandle, {
      clientX: 280,
      clientY: 220,
      pointerId: 2,
    });

    expect(onResizeGroup).toHaveBeenCalledWith(
      "group-1",
      expect.objectContaining({
        height: expect.any(Number),
        width: expect.any(Number),
        x: 40,
        y: 60,
      }),
    );
  });
});
