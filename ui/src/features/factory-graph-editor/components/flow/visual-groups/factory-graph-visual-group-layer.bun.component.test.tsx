// biome-ignore-all lint/style/noExcessiveLinesPerFile: visual-group layer interaction cases share one React Flow rendering harness.
import "../../../../../testing/vitest-dom-capabilities.setup";

import { fireEvent, render, screen } from "@testing-library/react";
import { ReactFlow, ReactFlowProvider } from "@xyflow/react";
import { describe, expect, it, mock } from "bun:test";
import type { ComponentProps, CSSProperties } from "react";

import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";

import type { FactoryLayoutGroup } from "../../../lib/layout/visual-groups/factory-graph-layout-groups";
import { FactoryGraphVisualGroupLayer } from "./factory-graph-visual-group-layer";

const vi = { fn: mock };

const sampleGroup: FactoryLayoutGroup = {
  bounds: { height: 120, width: 200, x: 40, y: 60 },
  color: "info",
  id: "group-1",
  label: "Review",
  nodeIds: [],
};

function enablePointerCapture(element: HTMLElement) {
  Object.defineProperty(element, "setPointerCapture", {
    configurable: true,
    value: vi.fn(),
  });
  Object.defineProperty(element, "releasePointerCapture", {
    configurable: true,
    value: vi.fn(),
  });
  Object.defineProperty(element, "hasPointerCapture", {
    configurable: true,
    value: vi.fn(() => true),
  });
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: pointer drag scenarios share one layer fixture.
describe("FactoryGraphVisualGroupLayer", () => {
  it("falls back to the group id when the label is blank", () => {
    renderVisualGroupLayer({
      groups: [
        {
          ...sampleGroup,
          label: "   ",
        },
      ],
    });

    expect(screen.getByText("group-1")).toBeInTheDocument();
  });

  it("ignores non-action keyboard events on the group body", () => {
    const { onSelectGroup } = renderVisualGroupLayer();
    const groupBody = screen.getByRole("button", { name: "Review" });

    groupBody.focus();
    fireEvent.keyDown(groupBody, { key: "Tab" });

    expect(onSelectGroup).not.toHaveBeenCalled();
  });

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

  it("renders nothing when there are no groups", () => {
    const { container } = render(
      <ReactFlowProvider>
        <div style={{ height: 480, width: 640 }}>
          <ReactFlow
            defaultViewport={{ x: 0, y: 0, zoom: 1 }}
            edges={[]}
            nodes={[]}
          >
            <FactoryGraphVisualGroupLayer
              canEdit
              groupAriaLabel={(group) => group.label ?? group.id}
              groups={[]}
              onSelectGroup={vi.fn()}
              resizeHandleAriaLabel={(corner) => `Resize ${corner}`}
              selectedGroupId={null}
            />
          </ReactFlow>
        </div>
      </ReactFlowProvider>,
    );

    expect(
      container.querySelector("[data-factory-visual-group-layer]"),
    ).not.toBeInTheDocument();
  });

  it("selects a group on click when the pointer does not move enough to drag", () => {
    const { onSelectGroup } = renderVisualGroupLayer({ onMoveGroup: vi.fn() });
    const groupBody = screen.getByRole("button", { name: "Review" });

    enablePointerCapture(groupBody);
    fireEvent.pointerDown(groupBody, {
      clientX: 100,
      clientY: 120,
      pointerId: 1,
    });
    fireEvent.pointerUp(groupBody, {
      clientX: 101,
      clientY: 121,
      pointerId: 1,
    });

    expect(onSelectGroup).toHaveBeenCalledWith("group-1");
  });

  it("does not move member nodes when click-selecting a group with slight pointer jitter", () => {
    const restoreBrowserShims = installDashboardBrowserTestShims();
    const onMoveGroup = vi.fn();
    const onSelectGroup = vi.fn();

    render(
      <ReactFlowProvider>
        <div style={{ height: 480, position: "relative", width: 640 }}>
          <ReactFlow
            defaultViewport={{ x: 0, y: 0, zoom: 1 }}
            edges={[]}
            nodes={[
              {
                data: { factoryGraphNodeId: "workstation:draft" },
                id: "workstation:draft",
                position: { x: 40, y: 60 },
              },
            ]}
          >
            <FactoryGraphVisualGroupLayer
              canEdit
              groupAriaLabel={(group) => group.label ?? group.id}
              groups={[
                {
                  ...sampleGroup,
                  nodeIds: ["workstation:draft"],
                },
              ]}
              onMoveGroup={onMoveGroup}
              onSelectGroup={onSelectGroup}
              resizeHandleAriaLabel={(corner) => `Resize ${corner}`}
              selectedGroupId={null}
            />
          </ReactFlow>
        </div>
      </ReactFlowProvider>,
    );

    const groupBody = screen.getByRole("button", { name: "Review" });
    enablePointerCapture(groupBody);
    fireEvent.pointerDown(groupBody, {
      clientX: 100,
      clientY: 120,
      pointerId: 1,
    });
    fireEvent.pointerMove(groupBody, {
      clientX: 102,
      clientY: 122,
      pointerId: 1,
    });
    fireEvent.pointerUp(groupBody, {
      clientX: 102,
      clientY: 122,
      pointerId: 1,
    });

    expect(onSelectGroup).toHaveBeenCalledWith("group-1");
    expect(onMoveGroup).not.toHaveBeenCalled();
    expect(screen.getByTestId("rf__node-workstation:draft")).toHaveStyle({
      transform: "translate(40px,60px)",
    });
    restoreBrowserShims();
  });

  it("hides resize handles and removes keyboard focus when editing is disabled", () => {
    renderVisualGroupLayer({
      canEdit: false,
      onResizeGroup: vi.fn(),
      selectedGroupId: "group-1",
    });

    expect(
      screen.queryByRole("button", { name: "Resize se" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Review" })).toHaveAttribute(
      "tabindex",
      "-1",
    );
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

  it.each(["nw", "ne", "sw"] as const)(
    "commits resize interactions from the %s corner",
    (corner) => {
      const onResizeGroup = vi.fn();

      renderVisualGroupLayer(
        {
          onResizeGroup,
          selectedGroupId: "group-1",
        },
        { height: 480, position: "relative", width: 640 },
      );

      const resizeHandle = screen.getByRole("button", {
        name: `Resize ${corner}`,
      });
      enablePointerCapture(resizeHandle);
      fireEvent.pointerDown(resizeHandle, {
        clientX: 240,
        clientY: 180,
        pointerId: 3,
      });
      fireEvent.pointerMove(resizeHandle, {
        clientX: 280,
        clientY: 220,
        pointerId: 3,
      });
      fireEvent.pointerUp(resizeHandle, {
        clientX: 280,
        clientY: 220,
        pointerId: 3,
      });

      expect(onResizeGroup).toHaveBeenCalledWith(
        "group-1",
        expect.objectContaining({
          height: expect.any(Number),
          width: expect.any(Number),
          x: expect.any(Number),
          y: expect.any(Number),
        }),
      );
    },
  );

  it("ignores pointer handlers when editing or callbacks are unavailable", () => {
    const onMoveGroup = vi.fn();
    const onResizeGroup = vi.fn();

    renderVisualGroupLayer({
      canEdit: false,
      onMoveGroup,
      onResizeGroup,
      selectedGroupId: "group-1",
    });

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

    expect(onMoveGroup).not.toHaveBeenCalled();
    expect(onResizeGroup).not.toHaveBeenCalled();
  });

  it("ignores pointer events from a different pointer id during drag", () => {
    const onMoveGroup = vi.fn();

    renderVisualGroupLayer(
      {
        onMoveGroup,
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
      pointerId: 2,
    });
    fireEvent.pointerUp(groupBody, {
      clientX: 140,
      clientY: 150,
      pointerId: 2,
    });

    expect(onMoveGroup).not.toHaveBeenCalled();
  });

  it("captures member node start positions when dragging a group with canvas nodes", () => {
    const restoreBrowserShims = installDashboardBrowserTestShims();
    const onMoveGroup = vi.fn();

    render(
      <ReactFlowProvider>
        <div style={{ height: 480, position: "relative", width: 640 }}>
          <ReactFlow
            defaultViewport={{ x: 0, y: 0, zoom: 1 }}
            edges={[]}
            nodes={[
              {
                data: { factoryGraphNodeId: "workstation:draft" },
                id: "workstation:draft",
                position: { x: 40, y: 60 },
              },
            ]}
          >
            <FactoryGraphVisualGroupLayer
              canEdit
              groupAriaLabel={(group) => group.label ?? group.id}
              groups={[
                {
                  ...sampleGroup,
                  nodeIds: ["workstation:draft"],
                },
              ]}
              onMoveGroup={onMoveGroup}
              onSelectGroup={vi.fn()}
              resizeHandleAriaLabel={(corner) => `Resize ${corner}`}
              selectedGroupId="group-1"
            />
          </ReactFlow>
        </div>
      </ReactFlowProvider>,
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

    const [, , startMemberPositions] = onMoveGroup.mock.calls[0] ?? [];
    expect(onMoveGroup).toHaveBeenCalledWith(
      "group-1",
      expect.objectContaining({ x: expect.any(Number), y: expect.any(Number) }),
      expect.any(Map),
    );
    expect(startMemberPositions?.get("workstation:draft")).toEqual({
      x: 40,
      y: 60,
    });
    restoreBrowserShims();
  });

  it("commits group moves that include saved member node ids", () => {
    const onMoveGroup = vi.fn();

    renderVisualGroupLayer(
      {
        groups: [
          {
            ...sampleGroup,
            nodeIds: ["workstation:draft"],
          },
        ],
        onMoveGroup,
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
  });
});
