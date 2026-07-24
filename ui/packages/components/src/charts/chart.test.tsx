// @vitest-environment happy-dom

import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import type { LegendPayload } from "recharts/types/component/DefaultLegendContent";
import { describe, expect, it, vi } from "vitest";

import { renderPackageComponent, screen, userEvent } from "../testing/render";

import { type ChartConfig, ChartContainer, ChartLegendContent } from "./chart";

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
        footer={<ChartLegendContent payload={sampleLegendPayload} />}
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

    const chartRegion = screen.getByRole("img", {
      name: "Sample throughput chart",
    });
    expect(chartRegion).toBeInTheDocument();
    expect(chartRegion).toHaveAttribute("data-chart-container", "");
    expect(chartRegion).toHaveStyle({
      "--color-alpha": "#3366cc",
      "--color-beta": "#cc6633",
    });
    expect(screen.getByText("Alpha series")).toBeInTheDocument();
    expect(screen.getByText("Beta series")).toBeInTheDocument();
  });

  it("accepts caller-owned hidden series state, toggle callbacks, and presentation props", async () => {
    const hiddenSeries = new Set(["beta"]);
    const onToggleSeries = vi.fn();
    const user = userEvent.setup();

    renderPackageComponent(
      <ChartContainer
        config={sampleChartConfig}
        footer={
          <ChartLegendContent
            getToggleLabel={(label, hidden) =>
              hidden ? `Show ${label}` : `Hide ${label}`
            }
            hiddenSeries={hiddenSeries}
            onToggleSeries={onToggleSeries}
            payload={sampleLegendPayload}
          />
        }
        presentation="embedded"
        title="Caller-owned legend chart"
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

    const chartRegion = screen.getByRole("img", {
      name: "Caller-owned legend chart",
    });
    expect(chartRegion).toHaveAttribute("data-chart-presentation", "embedded");

    const hiddenBetaButton = screen.getByRole("button", {
      name: "Show Beta series",
    });
    expect(hiddenBetaButton).toHaveAttribute("aria-pressed", "false");
    expect(hiddenBetaButton).toHaveAttribute(
      "data-chart-legend-series-hidden",
      "true",
    );
    expect(hiddenBetaButton).toHaveClass("text-on-surface-disabled");
    expect(hiddenBetaButton.className).not.toContain("text-af-text-disabled");

    const visibleAlphaButton = screen.getByRole("button", {
      name: "Hide Alpha series",
    });
    expect(visibleAlphaButton).toHaveAttribute("aria-pressed", "true");
    expect(visibleAlphaButton).not.toHaveClass("text-on-surface-disabled");

    await user.click(hiddenBetaButton);
    expect(onToggleSeries).toHaveBeenCalledWith("beta");
  });
});
