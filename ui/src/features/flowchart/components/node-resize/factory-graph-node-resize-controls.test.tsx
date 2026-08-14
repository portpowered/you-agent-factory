import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { updateNodeInternals } = vi.hoisted(() => ({
  updateNodeInternals: vi.fn(),
}));

vi.mock("@xyflow/react", () => ({
  NodeResizeControl: (props: {
    onResizeEnd?: (
      event: MouseEvent | TouchEvent,
      dimensions: { height: number; width: number },
    ) => void;
    position: string;
  }) => (
    <button
      data-testid={`resize-${props.position}`}
      onClick={() =>
        props.onResizeEnd?.(new MouseEvent("mouseup"), {
          height: 240,
          width: 280,
        })
      }
      type="button"
    />
  ),
  useUpdateNodeInternals: () => updateNodeInternals,
}));

import {
  FactoryGraphNodeResizeControls,
  type FactoryGraphNodeResizeControlsProps,
} from "@you-agent-factory/factory-graph";

function resizeProps(
  overrides: Partial<FactoryGraphNodeResizeControlsProps> = {},
): FactoryGraphNodeResizeControlsProps {
  return {
    allowedAxes: { height: false, width: true },
    bounds: {
      maximum: { height: 144, width: 360 },
      minimum: { height: 58, width: 156 },
    },
    fitDimensions: { height: 58, width: 260 },
    isVisible: true,
    labels: {
      fitToContent: "Fit to content",
      resetSize: "Reset size",
    },
    nodeId: "worker:writer",
    onFitToContent: vi.fn(),
    onResetSize: vi.fn(),
    onResizeEnd: vi.fn(),
    ...overrides,
  };
}

describe("Factory graph node resize controls", () => {
  beforeEach(() => {
    updateNodeInternals.mockClear();
  });

  it("renders only the allowed family axis and refreshes internals after pointer resize", async () => {
    const user = userEvent.setup();
    const onResizeEnd = vi.fn();
    const { container } = render(
      <FactoryGraphNodeResizeControls {...resizeProps({ onResizeEnd })} />,
    );

    expect(container.querySelectorAll("[data-testid^='resize-']")).toHaveLength(
      2,
    );
    expect(container.querySelector("[data-testid='resize-left']")).toBeTruthy();
    expect(
      container.querySelector("[data-testid='resize-right']"),
    ).toBeTruthy();
    expect(container.querySelector("[data-testid='resize-top']")).toBeNull();

    await user.click(screen.getByTestId("resize-right"));

    expect(onResizeEnd).toHaveBeenCalledWith({ height: 240, width: 280 });
    expect(updateNodeInternals).toHaveBeenCalledWith("worker:writer");
  });

  it("commits keyboard fit immediately and refreshes internals after its render", async () => {
    const user = userEvent.setup();
    const onFitToContent = vi.fn();
    let fitFrameCallback: FrameRequestCallback | undefined;
    const originalRequestAnimationFrame = globalThis.requestAnimationFrame;
    globalThis.requestAnimationFrame = ((callback: FrameRequestCallback) => {
      fitFrameCallback = callback;
      return 1;
    }) as typeof requestAnimationFrame;

    try {
      render(
        <FactoryGraphNodeResizeControls {...resizeProps({ onFitToContent })} />,
      );

      const fit = screen.getByRole("button", { name: "Fit to content" });
      expect(fit.className).toContain("focus-visible:ring-af-focus-ring");

      fit.focus();
      await user.keyboard("{Enter}");

      expect(onFitToContent).toHaveBeenCalledWith({ height: 58, width: 260 });
      expect(updateNodeInternals).not.toHaveBeenCalled();

      act(() => {
        fitFrameCallback?.(0);
      });

      expect(updateNodeInternals).toHaveBeenCalledTimes(1);
      expect(updateNodeInternals).toHaveBeenCalledWith("worker:writer");
    } finally {
      globalThis.requestAnimationFrame = originalRequestAnimationFrame;
    }
  });

  it("keeps Fit followed by Reset in activation order", async () => {
    const user = userEvent.setup();
    let authoredSize: { height: number; width: number } | undefined = {
      height: 100,
      width: 180,
    };
    let fitFrameCallback: FrameRequestCallback | undefined;
    const originalRequestAnimationFrame = globalThis.requestAnimationFrame;
    globalThis.requestAnimationFrame = ((callback: FrameRequestCallback) => {
      fitFrameCallback = callback;
      return 1;
    }) as typeof requestAnimationFrame;

    try {
      render(
        <FactoryGraphNodeResizeControls
          {...resizeProps({
            onFitToContent: (dimensions) => {
              authoredSize = dimensions;
            },
            onResetSize: () => {
              authoredSize = undefined;
            },
          })}
        />,
      );

      await user.click(screen.getByRole("button", { name: "Fit to content" }));
      expect(authoredSize).toEqual({ height: 58, width: 260 });

      await user.click(screen.getByRole("button", { name: "Reset size" }));
      expect(authoredSize).toBeUndefined();

      act(() => {
        fitFrameCallback?.(0);
      });

      expect(authoredSize).toBeUndefined();
    } finally {
      globalThis.requestAnimationFrame = originalRequestAnimationFrame;
    }
  });

  it("does not expose controls when the selected-node host is read-only", () => {
    const { container } = render(
      <FactoryGraphNodeResizeControls {...resizeProps({ isVisible: false })} />,
    );

    expect(
      container.querySelector("[data-factory-graph-node-resize-actions]"),
    ).toBeNull();
    expect(container.querySelectorAll("[data-testid^='resize-']")).toHaveLength(
      0,
    );
  });

  it("exposes both axes for a workstation-sized family", () => {
    const { container } = render(
      <FactoryGraphNodeResizeControls
        {...resizeProps({
          allowedAxes: { height: true, width: true },
          nodeId: "workstation:review",
        })}
      />,
    );

    expect(container.querySelectorAll("[data-testid^='resize-']")).toHaveLength(
      4,
    );
  });
});
