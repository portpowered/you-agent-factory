import "@testing-library/jest-dom/vitest";
import { ReactFlow, ReactFlowProvider } from "@xyflow/react";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { FactoryGraphVisualGroupLayer } from "./factory-graph-visual-group-layer";

describe("FactoryGraphVisualGroupLayer", () => {
  it("renders groups behind interaction handles with labels and selection state", () => {
    const onSelectGroup = vi.fn();

    render(
      <ReactFlowProvider>
        <div style={{ height: 480, width: 640 }}>
          <ReactFlow nodes={[]} edges={[]} fitView>
            <FactoryGraphVisualGroupLayer
              groupAriaLabel={(group) => group.label ?? group.id}
              groups={[
                {
                  bounds: { height: 120, width: 200, x: 40, y: 60 },
                  color: "info",
                  id: "group-1",
                  label: "Review",
                  nodeIds: [],
                },
              ]}
              onSelectGroup={onSelectGroup}
              selectedGroupId="group-1"
            />
          </ReactFlow>
        </div>
      </ReactFlowProvider>,
    );

    expect(screen.getByText("Review")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Review" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });
});
