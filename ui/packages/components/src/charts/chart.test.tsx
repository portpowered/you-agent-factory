// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import type { LegendPayload } from "recharts/types/component/DefaultLegendContent";

import { renderPackageComponent, screen } from "../testing/render";

import {
  ChartContainer,
  ChartLegendContent,
  type ChartConfig,
} from "./chart";

const sampleChartConfig: ChartConfig = {
  alpha: {
    color: "#3366cc",
    label: "Alpha series",
  },
  beta: {
    color: "#cc6633",
    label: "Beta series",
  },
};

const sampleChartData = [
  { alpha: 4, beta: 2, tick: 1 },
  { alpha: 6, beta: 3, tick: 2 },
  { alpha: 5, beta: 4, tick: 3 },
];

const sampleLegendPayload: LegendPayload[] = [
  {
    color: sampleChartConfig.alpha.color,
    dataKey: "alpha",
    type: "line",
    value: sampleChartConfig.alpha.label,
  },
  {
    color: sampleChartConfig.beta.color,
    dataKey: "beta",
    type: "line",
    value: sampleChartConfig.beta.label,
  },
];

describe("chart package exports", () => {
  it("renders a Recharts chart with accessible region, legend labels, and CSS color variables", () => {
    renderPackageComponent(
      <ChartContainer
        config={sampleChartConfig}
        footer={
          <ChartLegendContent
            payload={sampleLegendPayload}
          />
        }
        title="Sample throughput chart"
      >
        <LineChart data={sampleChartData}>
          <CartesianGrid vertical={false} />
          <XAxis dataKey="tick" />
          <YAxis />
          <Line dataKey="alpha" stroke="var(--color-alpha)" type="monotone" />
          <Line dataKey="beta" stroke="var(--color-beta)" type="monotone" />
        </LineChart>
      </ChartContainer>,
    );

    const chartRegion = screen.getByRole("img", { name: "Sample throughput chart" });
    expect(chartRegion).toBeInTheDocument();
    expect(chartRegion).toHaveAttribute("data-chart-container", "");
    expect(chartRegion).toHaveStyle({
      "--color-alpha": "#3366cc",
      "--color-beta": "#cc6633",
    });
    expect(screen.getByText("Alpha series")).toBeInTheDocument();
    expect(screen.getByText("Beta series")).toBeInTheDocument();
  });
});
