import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { updateNodeInternals } = vi.hoisted(() => ({
  updateNodeInternals: vi.fn(),
}));

vi.mock("@xyflow/react", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@xyflow/react")>();

  return {
    ...actual,
    NodeResizeControl: (props: {
      onResizeEnd?: (
        event: MouseEvent | TouchEvent,
        dimensions: { height: number; width: number },
      ) => void;
      position: string;
      style?: Record<string, string>;
      variant?: string;
    }) => (
      <button
        data-testid={`resize-${props.position}`}
        data-variant={props.variant}
        onClick={() =>
          props.onResizeEnd?.(new MouseEvent("mouseup"), {
            height: 240,
            width: 280,
          })
        }
        style={props.style}
        type="button"
      />
    ),
    useUpdateNodeInternals: () => updateNodeInternals,
  };
});

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
    isVisible: true,
    nodeId: "worker:writer",
    onResizeEnd: vi.fn(),
    ...overrides,
  };
}

describe("Factory graph node resize controls", () => {
  beforeEach(() => {
    updateNodeInternals.mockClear();
  });

  it("renders one bottom-edge line for a width-only family", async () => {
    const user = userEvent.setup();
    const onResizeEnd = vi.fn();
    const { container } = render(
      <FactoryGraphNodeResizeControls {...resizeProps({ onResizeEnd })} />,
    );

    const control = screen.getByTestId("resize-right");
    expect(container.querySelectorAll("[data-testid^='resize-']")).toHaveLength(
      1,
    );
    expect(control.getAttribute("data-variant")).toBe("line");
    expect(control).toHaveStyle({
      height: "10px",
      left: "0px",
      top: "100%",
      width: "100%",
    });
    expect(container.textContent).toBe("");
    expect(
      container.querySelector("[data-factory-graph-node-resize-actions]"),
    ).toBeNull();

    await user.click(control);

    expect(onResizeEnd).toHaveBeenCalledWith({ height: 240, width: 280 });
    expect(updateNodeInternals).toHaveBeenCalledWith("worker:writer");
  });

  it("uses the bottom-right edge for a family that allows both axes", () => {
    const { container } = render(
      <FactoryGraphNodeResizeControls
        {...resizeProps({
          allowedAxes: { height: true, width: true },
          nodeId: "workstation:review",
        })}
      />,
    );

    expect(container.querySelectorAll("[data-testid^='resize-']")).toHaveLength(
      1,
    );
    expect(screen.getByTestId("resize-bottom-right")).toHaveAttribute(
      "data-variant",
      "line",
    );
  });

  it("uses the bottom edge for a height-only family", () => {
    render(
      <FactoryGraphNodeResizeControls
        {...resizeProps({ allowedAxes: { height: true, width: false } })}
      />,
    );

    expect(screen.getByTestId("resize-bottom")).toHaveAttribute(
      "data-variant",
      "line",
    );
    expect(screen.queryByTestId("resize-right")).toBeNull();
  });

  it("does not expose controls when the selected-node host is read-only", () => {
    const { container } = render(
      <FactoryGraphNodeResizeControls {...resizeProps({ isVisible: false })} />,
    );

    expect(container.querySelectorAll("[data-testid^='resize-']")).toHaveLength(
      0,
    );
  });

  it("does not render a control when the family has no allowed resize axis", () => {
    const { container } = render(
      <FactoryGraphNodeResizeControls
        {...resizeProps({ allowedAxes: { height: false, width: false } })}
      />,
    );

    expect(container.querySelectorAll("[data-testid^='resize-']")).toHaveLength(
      0,
    );
  });
});
