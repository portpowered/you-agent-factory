import { render, screen } from "@testing-library/react";
import { Position } from "@xyflow/react";
import { describe, expect, it, vi } from "vitest";

import {
  FACTORY_GRAPH_EDGE_TYPES,
  GraphNodeButton,
  GraphViewportSurface,
} from "../public";
import { ActivityGraphNodeShell } from "./graph-node-shell";

vi.mock("@xyflow/react", () => ({
  BaseEdge: ({ path }: { path: string }) => (
    <path data-testid="base-edge" d={path} />
  ),
  Handle: ({ id }: { id: string }) => <div data-testid={`handle-${id}`} />,
  Position: { Bottom: "bottom", Left: "left", Right: "right", Top: "top" },
  getBezierPath: () => ["M0,0 L10,10", 5, 5],
}));

describe("dashboard graph package migration", () => {
  it("renders migrated node shell handles and supports node button activation", () => {
    const onSelect = vi.fn();

    render(
      <ActivityGraphNodeShell
        handles={[
          {
            id: "workstation-on-continue-source",
            label: "Continue",
            side: "right",
            type: "source",
          },
        ]}
        nodeType="workstation"
      >
        <GraphNodeButton onClick={onSelect}>Select workstation</GraphNodeButton>
      </ActivityGraphNodeShell>,
    );

    expect(
      document.querySelector(
        '[data-node-handle-tone="continue"][data-node-handle-badge="workstation-on-continue-source"]',
      ),
    ).toBeTruthy();
    screen.getByRole("button", { name: "Select workstation" }).click();
    expect(onSelect).toHaveBeenCalledTimes(1);
  });

  it("renders migrated factory graph edges with dashboard edge classes", () => {
    const FactoryGraphEdge = FACTORY_GRAPH_EDGE_TYPES.factoryEditorEdge;

    const { container } = render(
      <FactoryGraphEdge
        data={{ label: "Continue" }}
        id="edge-1"
        selected={false}
        source="node-a"
        sourcePosition={Position.Right}
        sourceX={0}
        sourceY={0}
        target="node-b"
        targetPosition={Position.Left}
        targetX={100}
        targetY={0}
      />,
    );

    expect(container.querySelector(".agent-factory-editor-edge")).toBeTruthy();
    expect(
      container.querySelector(".agent-factory-editor-edge-label"),
    ).toBeTruthy();
  });

  it("renders migrated viewport surfaces with dashboard graph frame semantics", () => {
    render(
      <GraphViewportSurface aria-label="Factory graph">
        <p>Graph canvas</p>
      </GraphViewportSurface>,
    );

    const viewport = screen.getByLabelText("Factory graph");
    expect(viewport.getAttribute("data-dashboard-graph-frame")).toBe("true");
  });
});
