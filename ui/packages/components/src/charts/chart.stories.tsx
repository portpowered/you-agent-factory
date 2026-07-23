import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { useState } from "react";
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import { expect, userEvent, within } from "storybook/test";

import { ChartContainer, ChartLegendContent } from "./chart";
import {
  sampleChartConfig,
  sampleChartData,
  sampleChartTitle,
  sampleLegendPayload,
} from "./chart-story-fixtures";

function ChartStoryShell({
  children,
  maxWidth = "640px",
}: {
  children: ReactNode;
  maxWidth?: string;
}) {
  return (
    <div
      data-story-shell="chart"
      style={{ maxWidth, padding: "1rem", width: "100%" }}
    >
      {children}
    </div>
  );
}

function SampleLineChart() {
  return (
    <LineChart data={sampleChartData}>
      <CartesianGrid vertical={false} />
      <XAxis dataKey="tick" />
      <YAxis />
      <Line dataKey="alpha" stroke="var(--color-alpha)" type="monotone" />
      <Line dataKey="beta" stroke="var(--color-beta)" type="monotone" />
    </LineChart>
  );
}

function InteractiveLegendChart() {
  const [hiddenSeries, setHiddenSeries] = useState<ReadonlySet<string>>(
    () => new Set(["beta"]),
  );

  const handleToggleSeries = (key: string) => {
    setHiddenSeries((current) => {
      const next = new Set(current);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  return (
    <ChartContainer
      config={sampleChartConfig}
      footer={
        <ChartLegendContent
          getToggleLabel={(label, hidden) =>
            hidden ? `Show ${label}` : `Hide ${label}`
          }
          hiddenSeries={hiddenSeries}
          onToggleSeries={handleToggleSeries}
          payload={sampleLegendPayload}
        />
      }
      title={sampleChartTitle}
    >
      <SampleLineChart />
    </ChartContainer>
  );
}

function expectNoOverflowInStoryShell(canvasElement: HTMLElement): void {
  const shell = canvasElement.querySelector<HTMLElement>("[data-story-shell]");

  expect(shell).not.toBeNull();
  expect((shell?.scrollWidth ?? 0) <= (shell?.clientWidth ?? 0) + 1).toBe(true);

  const chart = shell?.querySelector<HTMLElement>("[data-chart-container]");
  expect(chart).not.toBeNull();
  expect((chart?.scrollWidth ?? 0) <= (shell?.clientWidth ?? 0) + 1).toBe(true);
}

const meta = {
  title: "Charts/Chart",
  parameters: {
    layout: "centered",
  },
} satisfies Meta;

export default meta;

type Story = StoryObj<typeof meta>;

export const Populated: Story = {
  render: () => (
    <ChartStoryShell>
      <ChartContainer
        config={sampleChartConfig}
        footer={<ChartLegendContent payload={sampleLegendPayload} />}
        title={sampleChartTitle}
      >
        <SampleLineChart />
      </ChartContainer>
    </ChartStoryShell>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const chart = await canvas.findByRole("img", { name: sampleChartTitle });

    await expect(chart).toBeVisible();
    await expect(canvas.getByText("Alpha series")).toBeVisible();
    await expect(canvas.getByText("Beta series")).toBeVisible();
  },
};

export const InteractiveLegend: Story = {
  render: () => (
    <ChartStoryShell>
      <InteractiveLegendChart />
    </ChartStoryShell>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const hiddenBetaButton = await canvas.findByRole("button", {
      name: "Show Beta series",
    });

    await expect(hiddenBetaButton).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(hiddenBetaButton);
    await expect(hiddenBetaButton).toHaveAttribute("aria-pressed", "true");

    hiddenBetaButton.focus();
    await userEvent.keyboard("{Enter}");
    await expect(hiddenBetaButton).toHaveAttribute("aria-pressed", "false");
  },
};

export const NarrowViewport: Story = {
  render: () => (
    <ChartStoryShell maxWidth="320px">
      <ChartContainer
        config={sampleChartConfig}
        footer={<ChartLegendContent payload={sampleLegendPayload} />}
        title={sampleChartTitle}
      >
        <SampleLineChart />
      </ChartContainer>
    </ChartStoryShell>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const chart = await canvas.findByRole("img", { name: sampleChartTitle });

    await expect(chart).toBeVisible();
    await expect(canvas.getByText("Alpha series")).toBeVisible();
    await expect(canvas.getByText("Beta series")).toBeVisible();
    expectNoOverflowInStoryShell(canvasElement);
  },
};
