// @vitest-environment happy-dom

import { Position, ReactFlowProvider } from "@xyflow/react";
import {
  GraphEdge,
  GraphNodeButton,
  type GraphNodeHandle,
  GraphNodeShell,
  GraphViewportSurface,
} from "@you-agent-factory/components/graphs";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";
import { renderPackageComponent, screen } from "../testing/render";

const genericHandles: GraphNodeHandle[] = [
  {
    id: "input-target",
    label: "Input",
    side: "left",
    tone: "input",
    type: "target",
  },
  {
    id: "output-source",
    label: "Output",
    side: "right",
    tone: "output",
    type: "source",
  },
];

function renderWithReactFlow(ui: ReactElement) {
  return renderPackageComponent(<ReactFlowProvider>{ui}</ReactFlowProvider>);
}

describe("@you-agent-factory/components/graphs primitives", () => {
  it("renders graph node shell, button, edge, viewport surface, and handle badge", () => {
    renderWithReactFlow(
      <GraphViewportSurface aria-label="Graph viewport" className="h-64 w-96">
        <GraphNodeShell handles={genericHandles} nodeKind="example">
          <GraphNodeButton aria-label="Example node">
            Example node
          </GraphNodeButton>
        </GraphNodeShell>
        <svg aria-hidden="true" className="absolute inset-0">
          <GraphEdge
            data={{ label: "connects" }}
            id="edge-1"
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
        </svg>
      </GraphViewportSurface>,
    );

    expect(
      screen.getByRole("region", { name: "Graph viewport" }),
    ).toHaveAttribute("data-graph-viewport-surface", "true");
    expect(
      screen.getByRole("button", { name: "Example node" }),
    ).toBeInTheDocument();
    expect(
      document.querySelector('[data-node-handle-badge="input-target"]'),
    ).toBeInTheDocument();
    expect(
      document.querySelector('[data-edge-id="edge-1"]'),
    ).toBeInTheDocument();
    expect(
      document.querySelector('[data-graph-node-kind="example"]'),
    ).toBeInTheDocument();
  });
});
