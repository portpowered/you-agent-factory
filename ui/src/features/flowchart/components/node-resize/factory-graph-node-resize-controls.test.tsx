import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen } from "@testing-library/react";
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
      children?: React.ReactNode;
      className?: string;
      onResize?: (
        event: MouseEvent | TouchEvent,
        dimensions: { height: number; width: number },
      ) => void;
      onResizeEnd?: (
        event: MouseEvent | TouchEvent,
        dimensions: { height: number; width: number },
      ) => void;
      position: string;
      style?: Record<string, string>;
      variant?: string;
    }) => (
      <button
        className={props.className}
        data-testid={`resize-${props.position}`}
        data-variant={props.variant}
        onClick={() =>
          props.onResizeEnd?.(new MouseEvent("mouseup"), {
            height: 240,
            width: 280,
          })
        }
        onPointerMove={() =>
          props.onResize?.(new MouseEvent("mousemove"), {
            height: 210,
            width: 250,
          })
        }
        style={props.style}
        type="button"
      >
        {props.children}
      </button>
    ),
    useUpdateNodeInternals: () => updateNodeInternals,
  };
});

import {
  FACTORY_GRAPH_NODE_FAMILIES,
  FactoryGraphNodeResizeControls,
  type FactoryGraphNodeResizeControlsProps,
  factoryGraphNodeFamilyRole,
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

function grip(container: HTMLElement): HTMLElement | null {
  return container.querySelector("[data-factory-graph-node-resize-grip]");
}

describe("Factory graph node resize controls", () => {
  beforeEach(() => {
    updateNodeInternals.mockClear();
  });

  it("renders one bottom-right grip for a width-only family", async () => {
    const user = userEvent.setup();
    const onResizeEnd = vi.fn();
    const { container } = render(
      <FactoryGraphNodeResizeControls {...resizeProps({ onResizeEnd })} />,
    );

    const control = screen.getByTestId("resize-bottom-right");
    expect(container.querySelectorAll("[data-testid^='resize-']")).toHaveLength(
      1,
    );
    expect(control.getAttribute("data-variant")).toBe("handle");
    expect(container.textContent).toBe("");
    expect(
      container.querySelector("[data-factory-graph-node-resize-actions]"),
    ).toBeNull();

    await user.click(control);

    expect(onResizeEnd).toHaveBeenCalledWith({ height: 240, width: 280 });
    expect(updateNodeInternals).toHaveBeenCalledWith("worker:writer");
  });

  it.each(FACTORY_GRAPH_NODE_FAMILIES)(
    "uses the bottom-right corner for the %s family",
    (family) => {
      const { allowedAxes } = factoryGraphNodeFamilyRole(family);
      if (!allowedAxes.height && !allowedAxes.width) {
        throw new Error(`Expected ${family} to be resizable in this matrix.`);
      }

      const { container } = render(
        <FactoryGraphNodeResizeControls {...resizeProps({ allowedAxes })} />,
      );

      expect(
        container.querySelectorAll("[data-testid^='resize-']"),
      ).toHaveLength(1);
      expect(screen.getByTestId("resize-bottom-right")).toHaveAttribute(
        "data-variant",
        "handle",
      );
      expect(screen.queryByTestId("resize-right")).toBeNull();
      expect(screen.queryByTestId("resize-bottom")).toBeNull();
    },
  );

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

describe("Factory graph node resize grip appearance", () => {
  it("marks a both-axes family with a small bottom-right corner grip", () => {
    const { container } = render(
      <FactoryGraphNodeResizeControls
        {...resizeProps({
          allowedAxes: { height: true, width: true },
          nodeId: "workstation:review",
        })}
      />,
    );

    const control = screen.getByTestId("resize-bottom-right");
    expect(control).toHaveStyle({ height: "14px", width: "14px" });
    expect(grip(container)?.className).toContain("border-b-2");
    expect(grip(container)?.className).toContain("border-r-2");
  });

  it("tints the grip with the subtle neutral border token, never an accent", () => {
    const { container } = render(
      <FactoryGraphNodeResizeControls
        {...resizeProps({ allowedAxes: { height: true, width: true } })}
      />,
    );

    const gripClassName = grip(container)?.className ?? "";
    expect(gripClassName).toContain("border-af-text-subtle");
    expect(gripClassName).not.toContain("primary");
    expect(screen.getByTestId("resize-bottom-right")).not.toHaveStyle({
      borderBottomColor: "var(--color-primary)",
    });
  });

  it("stays out of sight until the node is hovered or focused", () => {
    render(
      <FactoryGraphNodeResizeControls
        {...resizeProps({ allowedAxes: { height: true, width: true } })}
      />,
    );

    const controlClassName =
      screen.getByTestId("resize-bottom-right").className ?? "";
    expect(controlClassName).toContain("opacity-0");
    expect(controlClassName).toContain(
      "group-hover/factory-graph-node:opacity-100",
    );
    expect(controlClassName).toContain(
      "group-focus-within/factory-graph-node:opacity-100",
    );
  });

  it("draws a single-axis grip as the same corner mark", () => {
    const { container } = render(
      <FactoryGraphNodeResizeControls {...resizeProps()} />,
    );

    const gripClassName = grip(container)?.className ?? "";
    expect(gripClassName).toContain("border-r-2");
    expect(gripClassName).toContain("border-b-2");
  });
});

describe("Factory graph node live resize", () => {
  beforeEach(() => {
    updateNodeInternals.mockClear();
  });

  it("reports dimensions continuously while the pointer drags", () => {
    const onResize = vi.fn();
    const onResizeEnd = vi.fn();
    render(
      <FactoryGraphNodeResizeControls
        {...resizeProps({
          allowedAxes: { height: true, width: true },
          onResize,
          onResizeEnd,
        })}
      />,
    );

    fireEvent.pointerMove(screen.getByTestId("resize-bottom-right"));

    expect(onResize).toHaveBeenCalledWith({ height: 210, width: 250 });
    expect(onResizeEnd).not.toHaveBeenCalled();
  });

  it("leaves node internals alone until the drag settles", () => {
    render(
      <FactoryGraphNodeResizeControls
        {...resizeProps({
          allowedAxes: { height: true, width: true },
          onResize: vi.fn(),
        })}
      />,
    );

    fireEvent.pointerMove(screen.getByTestId("resize-bottom-right"));

    expect(updateNodeInternals).not.toHaveBeenCalled();
  });
});
