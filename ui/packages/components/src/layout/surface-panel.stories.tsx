import type { Meta, StoryObj } from "@storybook/react-vite";

import { SurfacePanel } from "./surface-panel";

const meta = {
  title: "Layout/SurfacePanel",
  component: SurfacePanel,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof SurfacePanel>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    children: "Surface panel content supplied by the host application",
  },
};

export const LowSurface: Story = {
  args: {
    children: "Low-surface panel",
    radius: "lg",
    surface: "low",
  },
};
