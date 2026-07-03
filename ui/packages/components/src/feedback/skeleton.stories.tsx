import type { Meta, StoryObj } from "@storybook/react-vite";

import { Skeleton } from "./skeleton";

const meta = {
  title: "Feedback/Skeleton",
  component: Skeleton,
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof Skeleton>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Compact: Story = {
  render: () => (
    <div className="grid gap-2">
      <Skeleton className="h-4 w-24" />
      <Skeleton className="h-4 w-32" />
      <Skeleton className="h-4 w-16" />
    </div>
  ),
};

export const FullWidth: Story = {
  render: () => (
    <div className="w-full max-w-2xl">
      <Skeleton className="h-28 w-full" />
    </div>
  ),
};

export const PanelLoadingLayout: Story = {
  render: () => (
    <div
      aria-busy="true"
      aria-hidden="true"
      className="grid w-full max-w-xl gap-3"
    >
      <Skeleton className="h-4 w-32" />
      <Skeleton className="h-28 w-full" />
      <Skeleton className="h-4 w-full max-w-48" />
    </div>
  ),
};
