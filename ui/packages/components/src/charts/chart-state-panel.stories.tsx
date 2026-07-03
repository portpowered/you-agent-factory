import type { Meta, StoryObj } from "@storybook/react-vite";

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

export const Loading: Story = {};

export const Empty: Story = {
  args: {
    description: "No data points are available for this range.",
    status: "empty",
    title: "No chart data",
  },
};

export const ErrorState: Story = {
  args: {
    action: (
      <button type="button">
        Retry chart load
      </button>
    ),
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
