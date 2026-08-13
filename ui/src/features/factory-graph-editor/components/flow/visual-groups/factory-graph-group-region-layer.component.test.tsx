// @component-test-runner vitest
import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";
import { ReactFlow, ReactFlowProvider } from "@xyflow/react";
import {
  type FactoryGraphGroupRegionInput,
  FactoryGraphGroupRegionLayer,
} from "@you-agent-factory/factory-graph";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";

const sampleGroup: FactoryGraphGroupRegionInput = {
  bounds: { height: 120, width: 200, x: 40, y: 60 },
  color: "info",
  id: "group-1",
  label: "Review",
};

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

function renderLayer(groups: readonly FactoryGraphGroupRegionInput[]) {
  return render(
    <ReactFlowProvider>
      <div style={{ height: 480, width: 640 }}>
        <ReactFlow
          defaultViewport={{ x: 0, y: 0, zoom: 1 }}
          edges={[]}
          nodes={[]}
          proOptions={{ hideAttribution: true }}
        >
          <FactoryGraphGroupRegionLayer groups={groups} />
        </ReactFlow>
      </div>
    </ReactFlowProvider>,
  );
}

describe("FactoryGraphGroupRegionLayer", () => {
  it("renders no layer for an empty group collection", () => {
    const { container } = renderLayer([]);

    expect(
      container.querySelector("[data-factory-graph-group-region-layer]"),
    ).not.toBeInTheDocument();
  });

  it("keeps the region and its interior pointer-transparent and read-only", () => {
    const { container } = renderLayer([sampleGroup]);
    const layer = container.querySelector(
      "[data-factory-graph-group-region-layer]",
    );
    const region = container.querySelector(
      '[data-factory-graph-group-region="group-1"]',
    );

    expect(layer).toHaveClass("pointer-events-none");
    expect(region).toHaveClass("pointer-events-none");
    expect(region).not.toContainElement(screen.queryByRole("button"));
    expect(screen.getByText("Review")).toBeVisible();
    expect(screen.getByRole("region", { name: "Review" })).toBeInTheDocument();
  });

  it("shows a safe neutral presentation for unsupported saved color values", () => {
    renderLayer([{ ...sampleGroup, color: "legacy-purple" }]);

    expect(screen.getByText("Review")).toBeVisible();
    expect(
      screen.getByRole("region", { name: "Review" }).getAttribute("style"),
    ).toContain("border-color: var(--color-outline-variant)");
  });
});
