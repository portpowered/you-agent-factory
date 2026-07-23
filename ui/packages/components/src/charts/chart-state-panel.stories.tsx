import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";

import { ChartStatePanel } from "./chart-state-panel";

const meta = {
  title: "Charts/ChartStatePanel",
  component: ChartStatePanel,
  parameters: {
    layout: "centered",
  },
  args: {
    description: "Waiting for chart data to load.",
    status: "loading",
    title: "Loading chart data",
  },
} satisfies Meta<typeof ChartStatePanel>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Loading: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const panel = await canvas.findByRole("status");

    await expect(panel).toHaveAttribute("data-chart-state", "loading");
    await expect(
      canvas.getByRole("heading", { name: "Loading chart data" }),
    ).toBeVisible();
    await expect(
      canvas.getByText("Waiting for chart data to load."),
    ).toBeVisible();
  },
};

export const Empty: Story = {
  args: {
    description: "No data points are available for this range.",
    status: "empty",
    title: "No chart data",
  },
};

export const ErrorState: Story = {
  args: {
    action: <button type="button">Retry chart load</button>,
    description: "The chart request failed. Try again later.",
    status: "error",
    title: "Unable to load chart",
  },
};

export const Success: Story = {
  args: {
    description: "The chart finished loading successfully.",
    status: "success",
    title: "Chart ready",
  },
};

export const EmbeddedEmpty: Story = {
  args: {
    description: "No data points are available for this range.",
    presentation: "embedded",
    status: "empty",
    title: "No chart data",
  },
  decorators: [
    (Story) => (
      <div className="w-64 rounded-2xl border border-outline bg-surface-container-low p-4">
        <Story />
      </div>
    ),
  ],
};

export const NarrowViewportError: Story = {
  args: {
    action: <button type="button">Retry chart load</button>,
    description: "The chart request failed. Try again later.",
    presentation: "embedded",
    status: "error",
    title: "Unable to load chart",
  },
  decorators: [
    (Story) => (
      <div
        className="w-64 overflow-hidden rounded-2xl border border-outline bg-surface-container-low p-4"
        data-story-shell="chart-state-panel"
      >
        <Story />
      </div>
    ),
  ],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const panel = await canvas.findByRole("alert");

    await expect(panel).toHaveAttribute("data-chart-state", "error");
    await expect(
      canvas.getByRole("button", { name: "Retry chart load" }),
    ).toBeVisible();

    const shell =
      canvasElement.querySelector<HTMLElement>("[data-story-shell]");
    expect(shell).not.toBeNull();
    expect((shell?.scrollWidth ?? 0) <= (shell?.clientWidth ?? 0) + 1).toBe(
      true,
    );
  },
};
