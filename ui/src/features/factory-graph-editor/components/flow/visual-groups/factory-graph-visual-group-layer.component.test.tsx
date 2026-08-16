// biome-ignore-all lint/style/noExcessiveLinesPerFile: visual-group layer interaction cases share one React Flow rendering harness.
import "../../../../../testing/vitest-dom-capabilities.setup";
import "@testing-library/jest-dom/vitest";

import { fireEvent, render, screen } from "@testing-library/react";
import { ReactFlow, ReactFlowProvider } from "@xyflow/react";
import type { ComponentProps, CSSProperties } from "react";
import { describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";

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
  const setPointerCapture = vi.fn();
  const releasePointerCapture = vi.fn();
  const hasPointerCapture = vi.fn(() => true);
  Object.defineProperty(element, "setPointerCapture", {
    configurable: true,
    value: setPointerCapture,
  });
  Object.defineProperty(element, "releasePointerCapture", {
    configurable: true,
    value: releasePointerCapture,
  });
  Object.defineProperty(element, "hasPointerCapture", {
    configurable: true,
    value: hasPointerCapture,
  });

  return { hasPointerCapture, releasePointerCapture, setPointerCapture };
}

function renderVisualGroupLayer(
  props: Partial<ComponentProps<typeof FactoryGraphVisualGroupLayer>> = {},
  wrapperStyle: CSSProperties = { height: 480, width: 640 },
  nodes: ComponentProps<typeof ReactFlow>["nodes"] = [],
) {
  const onSelectGroup = props.onSelectGroup ?? vi.fn();

  render(
    <ReactFlowProvider>
      <div style={wrapperStyle}>
        <ReactFlow
          defaultViewport={{ x: 0, y: 0, zoom: 1 }}
          edges={[]}
          fitView={wrapperStyle.position !== "relative"}
          defaultNodes={nodes}
        >
          <FactoryGraphVisualGroupLayer
            canEdit
            groupAriaLabel={(group) => group.label ?? group.id}
            groupOutlineAriaLabel={(group, edge) =>
              `${group.label ?? group.id} outline ${edge}`
            }
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

  it("ignores non-action keyboard events on the group label affordance", () => {
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

    expect(screen.getByTitle("Review")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Review" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(
      screen.getByRole("button", { name: "Resize se" }),
    ).toBeInTheDocument();
  });

  it("keeps the decorative interior transparent and targets only label or outline affordances", () => {
    renderVisualGroupLayer();

    const body = document.querySelector('[data-factory-visual-group-body=""]');
    const label = screen.getByRole("button", { name: "Review" });
    const outline = screen.getByRole("button", {
      name: "Review outline top",
    });

    expect(body).toHaveClass("pointer-events-none");
    expect(body).not.toHaveAttribute("role");
    expect(label).toHaveClass("pointer-events-auto");
    expect(outline).toHaveClass("pointer-events-auto");
  });

  it("selects a group when Enter or Space is pressed on the group label affordance", () => {
    const { onSelectGroup } = renderVisualGroupLayer({ selectedGroupId: null });
    const groupBody = screen.getByRole("button", { name: "Review" });

    groupBody.focus();
    fireEvent.keyDown(groupBody, { key: "Enter" });
    expect(onSelectGroup).toHaveBeenCalledWith("group-1");

    onSelectGroup.mockClear();
    fireEvent.keyDown(groupBody, { key: " " });
    expect(onSelectGroup).toHaveBeenCalledWith("group-1");
  });

  it("moves from the label with keyboard arrows and resizes from a focused handle", () => {
    const onMoveGroup = vi.fn();
    const onResizeGroup = vi.fn();

    renderVisualGroupLayer({
      onMoveGroup,
      onResizeGroup,
      selectedGroupId: "group-1",
    });

    fireEvent.keyDown(screen.getByRole("button", { name: "Review" }), {
      key: "ArrowRight",
    });
    expect(onMoveGroup).toHaveBeenCalledWith(
      "group-1",
      { x: 16, y: 0 },
      expect.any(Map),
    );

    fireEvent.keyDown(screen.getByRole("button", { name: "Resize se" }), {
      key: "ArrowDown",
    });
    expect(onResizeGroup).toHaveBeenCalledWith("group-1", {
      height: 136,
      width: 200,
      x: 40,
      y: 60,
    });
  });

  it("moves from the outline with keyboard arrows and pointer dragging", () => {
    const onMoveGroup = vi.fn();

    renderVisualGroupLayer({
      onMoveGroup,
      selectedGroupId: "group-1",
    });

    const outline = screen.getByRole("button", {
      name: "Review outline right",
    });
    fireEvent.keyDown(outline, { key: "ArrowLeft" });
    expect(onMoveGroup).toHaveBeenCalledWith(
      "group-1",
      { x: -16, y: 0 },
      expect.any(Map),
    );

    onMoveGroup.mockClear();
    enablePointerCapture(outline);
    fireEvent.pointerDown(outline, {
      clientX: 240,
      clientY: 120,
      pointerId: 4,
    });
    fireEvent.pointerMove(outline, {
      clientX: 280,
      clientY: 150,
      pointerId: 4,
    });
    fireEvent.pointerUp(outline, {
      clientX: 280,
      clientY: 150,
      pointerId: 4,
    });

    expect(onMoveGroup).toHaveBeenCalledWith(
      "group-1",
      expect.objectContaining({ x: expect.any(Number), y: expect.any(Number) }),
      expect.any(Map),
    );
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
              groupOutlineAriaLabel={(group, edge) =>
                `${group.label ?? group.id} outline ${edge}`
              }
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
              groupOutlineAriaLabel={(group, edge) =>
                `${group.label ?? group.id} outline ${edge}`
              }
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

  it("leaves Escape untouched when no group gesture is active", () => {
    const onMoveGroup = vi.fn();
    const onResizeGroup = vi.fn();

    renderVisualGroupLayer({ onMoveGroup, onResizeGroup });

    const escapeEvent = new KeyboardEvent("keydown", {
      cancelable: true,
      key: "Escape",
    });
    window.dispatchEvent(escapeEvent);

    expect(escapeEvent.defaultPrevented).toBe(false);
    expect(onMoveGroup).not.toHaveBeenCalled();
    expect(onResizeGroup).not.toHaveBeenCalled();
  });

  it.each(["Escape", "pointercancel"] as const)(
    "cancels a moving group through %s and restores its exact preview geometry",
    (cancelInput) => {
      const restoreBrowserShims = installDashboardBrowserTestShims();
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
        [
          {
            data: { factoryGraphNodeId: "workstation:draft" },
            id: "workstation:draft",
            position: { x: 40, y: 60 },
          },
        ],
      );

      const groupBody = screen.getByRole("button", { name: "Review" });
      groupBody.focus();
      const pointerCapture = enablePointerCapture(groupBody);
      fireEvent.pointerDown(groupBody, {
        clientX: 100,
        clientY: 120,
        pointerId: 7,
      });
      fireEvent.pointerMove(groupBody, {
        clientX: 140,
        clientY: 150,
        pointerId: 7,
      });

      const group = document.querySelector(
        '[data-factory-visual-group="group-1"]',
      );
      const memberNode = screen.getByTestId("rf__node-workstation:draft");
      expect(group).not.toHaveStyle({ left: "40px", top: "60px" });
      expect(memberNode).not.toHaveStyle({
        transform: "translate(40px,60px)",
      });

      if (cancelInput === "Escape") {
        fireEvent.keyDown(groupBody, { key: "Escape" });
      } else {
        fireEvent.pointerCancel(groupBody, { pointerId: 7 });
      }

      expect(onMoveGroup).not.toHaveBeenCalled();
      expect(group).toHaveStyle({
        height: "120px",
        left: "40px",
        top: "60px",
        width: "200px",
      });
      expect(memberNode).toHaveStyle({ transform: "translate(40px,60px)" });
      expect(document.activeElement).toBe(groupBody);
      expect(pointerCapture.releasePointerCapture).toHaveBeenCalledWith(7);
      restoreBrowserShims();
    },
  );

  it.each(["Escape", "pointercancel"] as const)(
    "cancels a resizing group through %s and restores its exact preview bounds",
    (cancelInput) => {
      const onResizeGroup = vi.fn();

      renderVisualGroupLayer(
        {
          onResizeGroup,
          selectedGroupId: "group-1",
        },
        { height: 480, position: "relative", width: 640 },
      );

      const resizeHandle = screen.getByRole("button", { name: "Resize se" });
      resizeHandle.focus();
      const pointerCapture = enablePointerCapture(resizeHandle);
      fireEvent.pointerDown(resizeHandle, {
        clientX: 240,
        clientY: 180,
        pointerId: 8,
      });
      fireEvent.pointerMove(resizeHandle, {
        clientX: 280,
        clientY: 220,
        pointerId: 8,
      });

      const group = document.querySelector(
        '[data-factory-visual-group="group-1"]',
      );
      expect(group).not.toHaveStyle({ height: "120px", width: "200px" });

      if (cancelInput === "Escape") {
        fireEvent.keyDown(resizeHandle, { key: "Escape" });
      } else {
        fireEvent.pointerCancel(resizeHandle, { pointerId: 8 });
      }

      expect(onResizeGroup).not.toHaveBeenCalled();
      expect(group).toHaveStyle({
        height: "120px",
        left: "40px",
        top: "60px",
        width: "200px",
      });
      expect(document.activeElement).toBe(resizeHandle);
      expect(pointerCapture.releasePointerCapture).toHaveBeenCalledWith(8);
    },
  );

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
              groupOutlineAriaLabel={(group, edge) =>
                `${group.label ?? group.id} outline ${edge}`
              }
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

  it("previews one delta for every member while leaving unrelated nodes stable", () => {
    const restoreBrowserShims = installDashboardBrowserTestShims();
    const onMoveGroup = vi.fn();

    render(
      <ReactFlowProvider>
        <div style={{ height: 480, position: "relative", width: 640 }}>
          <ReactFlow
            defaultViewport={{ x: 0, y: 0, zoom: 1 }}
            edges={[]}
            defaultNodes={[
              {
                data: { factoryGraphNodeId: "workstation:draft" },
                id: "workstation:draft",
                position: { x: 40, y: 60 },
              },
              {
                data: { factoryGraphNodeId: "worker:writer" },
                id: "worker:writer",
                position: { x: 160, y: 90 },
              },
              {
                data: { factoryGraphNodeId: "workstation:unrelated" },
                id: "workstation:unrelated",
                position: { x: 420, y: 300 },
              },
            ]}
          >
            <FactoryGraphVisualGroupLayer
              canEdit
              groupAriaLabel={(group) => group.label ?? group.id}
              groupOutlineAriaLabel={(group, edge) =>
                `${group.label ?? group.id} outline ${edge}`
              }
              groups={[
                {
                  ...sampleGroup,
                  nodeIds: ["workstation:draft", "worker:writer"],
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
      pointerId: 9,
    });
    fireEvent.pointerMove(groupBody, {
      clientX: 140,
      clientY: 150,
      pointerId: 9,
    });

    expect(screen.getByTestId("rf__node-workstation:draft")).not.toHaveStyle({
      transform: "translate(40px,60px)",
    });
    expect(screen.getByTestId("rf__node-worker:writer")).not.toHaveStyle({
      transform: "translate(160px,90px)",
    });
    expect(screen.getByTestId("rf__node-workstation:unrelated")).toHaveStyle({
      transform: "translate(420px,300px)",
    });

    fireEvent.pointerUp(groupBody, {
      clientX: 140,
      clientY: 150,
      pointerId: 9,
    });

    expect(onMoveGroup).toHaveBeenCalledWith(
      "group-1",
      expect.objectContaining({ x: expect.any(Number), y: expect.any(Number) }),
      expect.any(Map),
    );
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
