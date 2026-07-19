// @vitest-environment happy-dom

import { fireEvent } from "@testing-library/react";
import { Position, ReactFlowProvider } from "@xyflow/react";
import {
  GraphEdge,
  GraphNodeButton,
  type GraphNodeHandle,
  GraphNodeHandleBadge,
  GraphNodeShell,
} from "@you-agent-factory/components/graphs";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { renderPackageComponent, screen } from "../testing/render";

const sourceHandle: GraphNodeHandle = {
  id: "output-source",
  buttonAriaLabel: "Output connection",
  label: "Output",
  side: "right",
  tone: "output",
  type: "source",
};

const targetHandle: GraphNodeHandle = {
  id: "input-target",
  buttonAriaLabel: "Input connection",
  label: "Input",
  side: "left",
  tone: "input",
  type: "target",
};

function renderWithReactFlow(ui: ReactElement) {
  return renderPackageComponent(<ReactFlowProvider>{ui}</ReactFlowProvider>);
}

describe("graph edges and handles", () => {
  it("renders graph edge geometry from React Flow props with package-owned visual data", () => {
    renderWithReactFlow(
      <svg aria-hidden="true">
        <GraphEdge
          data={{ alwaysShowLabel: true, label: "connects" }}
          id="edge-bezier"
          interactionWidth={20}
          selected={false}
          sourcePosition={Position.Right}
          sourceX={40}
          sourceY={20}
          style={{ stroke: "var(--color-outline)" }}
          targetPosition={Position.Left}
          targetX={160}
          targetY={60}
        />
      </svg>,
    );

    const edge = document.querySelector('[data-edge-id="edge-bezier"]');
    expect(edge).toBeInTheDocument();
    expect(edge).toHaveAttribute("data-label-visible", "true");
    expect(edge?.querySelector("path")).toBeInTheDocument();
    expect(screen.getByText("connects")).toBeInTheDocument();
  });

  it("renders waypoint-routed graph edges when package edge data includes waypoints", () => {
    renderWithReactFlow(
      <svg aria-hidden="true">
        <GraphEdge
          data={{
            label: "routed",
            waypoints: [
              { x: 100, y: 20 },
              { x: 100, y: 80 },
            ],
          }}
          id="edge-waypoints"
          interactionWidth={20}
          selected
          sourcePosition={Position.Right}
          sourceX={0}
          sourceY={0}
          style={{ stroke: "var(--color-primary)" }}
          targetPosition={Position.Left}
          targetX={200}
          targetY={100}
        />
      </svg>,
    );

    const edge = document.querySelector('[data-edge-id="edge-waypoints"]');
    const path = edge?.querySelector("path");
    expect(path).toBeInTheDocument();
    expect(path?.getAttribute("d")).toContain("M 0 0");
    expect(path?.getAttribute("d")).toContain("200, 100");
    expect(edge).toHaveAttribute("data-label-visible", "true");
  });

  it("renders source and target handle badges with accessible labels", () => {
    renderWithReactFlow(
      <>
        <GraphNodeHandleBadge handle={targetHandle} />
        <GraphNodeHandleBadge handle={sourceHandle} />
      </>,
    );

    expect(screen.getByLabelText("Input connection")).toBeInTheDocument();
    expect(screen.getByLabelText("Output connection")).toBeInTheDocument();
    expect(screen.getAllByRole("img")).toHaveLength(2);
    expect(
      document.querySelector('[data-node-handle-badge="input-target"]'),
    ).toHaveAttribute("data-node-handle-tone", "input");
    expect(
      document.querySelector('[data-node-handle-badge="output-source"]'),
    ).toHaveAttribute("data-node-handle-tone", "output");
  });

  it("makes callback-backed handles keyboard-operable buttons", () => {
    const onButtonClick = vi.fn();
    renderWithReactFlow(
      <GraphNodeHandleBadge handle={{ ...sourceHandle, onButtonClick }} />,
    );

    const handle = screen.getByRole("button", { name: "Output connection" });
    fireEvent.keyDown(handle, { key: "Enter" });
    fireEvent.keyDown(handle, { key: " " });

    expect(onButtonClick).toHaveBeenCalledTimes(2);
  });

  it("keeps handle badge placement stable on selected node shells", () => {
    renderWithReactFlow(
      <GraphNodeShell
        handles={[targetHandle, sourceHandle]}
        nodeKind="example"
        state="selected"
        stateLabel="Selected node"
      >
        <GraphNodeButton graphState="selected">Selected node</GraphNodeButton>
      </GraphNodeShell>,
    );

    const shell = document.querySelector('[data-graph-node-state="selected"]');
    const leftRail = shell?.querySelector('[data-node-handle-rail="left"]');
    const rightRail = shell?.querySelector('[data-node-handle-rail="right"]');

    expect(
      leftRail?.querySelector('[data-node-handle-badge="input-target"]'),
    ).toBeInTheDocument();
    expect(
      rightRail?.querySelector('[data-node-handle-badge="output-source"]'),
    ).toBeInTheDocument();
    expect(shell).toHaveAttribute("aria-selected", "true");
    expect(screen.getByLabelText("Input connection")).toBeInTheDocument();
    expect(screen.getByLabelText("Output connection")).toBeInTheDocument();
  });
});
