import { useMemo } from "react";

import { Background, ReactFlow, ReactFlowProvider } from "@xyflow/react";
import * as d3 from "d3";
import GridLayout from "react-grid-layout";
import { render, screen } from "@testing-library/react";

import "@xyflow/react/dist/style.css";
import "react-grid-layout/css/styles.css";
import "react-resizable/css/styles.css";

const chartValues = [2, 5, 3];

class TestResizeObserver {
  public disconnect(): void {}

  public observe(): void {}

  public unobserve(): void {}
}

function DependencySmokeDashboard() {
  const chartPath = useMemo(() => {
    const xScale = d3.scaleLinear().domain([0, chartValues.length - 1]).range([0, 120]);
    const yScale = d3.scaleLinear().domain([0, 5]).range([40, 0]);

    return d3
      .line<number>()
      .x((_value, index) => xScale(index))
      .y((value) => yScale(value))(chartValues);
  }, []);

  return (
    <GridLayout
      className="dependency-smoke-grid"
      gridConfig={{
        cols: 2,
        containerPadding: [0, 0],
        margin: [8, 8],
        maxRows: 2,
        rowHeight: 80,
      }}
      layout={[
        { h: 2, i: "chart", w: 1, x: 0, y: 0 },
        { h: 2, i: "flow", w: 1, x: 1, y: 0 },
      ]}
      width={640}
    >
      <article key="chart" aria-label="D3 dependency smoke">
        <svg aria-label="D3 chart smoke" role="img" viewBox="0 0 120 40">
          <path d={chartPath ?? ""} fill="none" stroke="currentColor" />
        </svg>
      </article>
      <article key="flow" aria-label="React Flow dependency smoke" style={{ height: 160 }}>
        <ReactFlowProvider>
          <ReactFlow
            edges={[{ id: "source-target", source: "source", target: "target" }]}
            fitView
            nodes={[
              {
                data: { label: "Source" },
                id: "source",
                position: { x: 0, y: 0 },
              },
              {
                data: { label: "Target" },
                id: "target",
                position: { x: 160, y: 0 },
              },
            ]}
          >
            <Background />
          </ReactFlow>
        </ReactFlowProvider>
      </article>
    </GridLayout>
  );
}

describe("dashboard visualization dependencies", () => {
  beforeEach(() => {
    vi.stubGlobal("ResizeObserver", TestResizeObserver);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("imports and renders the selected bento, chart, and graph libraries", () => {
    render(<DependencySmokeDashboard />);

    expect(screen.getByLabelText("D3 dependency smoke")).toBeTruthy();
    expect(screen.getByRole("img", { name: "D3 chart smoke" })).toBeTruthy();
    expect(screen.getByLabelText("React Flow dependency smoke")).toBeTruthy();
    expect(screen.getByText("Source")).toBeTruthy();
    expect(screen.getByText("Target")).toBeTruthy();
  });
});
