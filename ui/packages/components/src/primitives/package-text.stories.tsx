import type { Meta, StoryObj } from "@storybook/react-vite";

import { PackageText } from "./package-text";

const meta = {
  title: "Primitives/PackageText",
  component: PackageText,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof PackageText>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Body: Story = {
  args: {
    children: "Hello from the component package",
  },
};

export const Title: Story = {
  args: {
    children: "Section heading",
    variant: "title",
  },
};
