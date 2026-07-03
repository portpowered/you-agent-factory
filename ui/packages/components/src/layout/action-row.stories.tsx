import type { Meta, StoryObj } from "@storybook/react-vite";

import { ActionRow } from "./action-row";

const meta = {
  title: "Layout/ActionRow",
  component: ActionRow,
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof ActionRow>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    statuses: <span>Ready</span>,
    actions: (
      <>
        <button type="button">Secondary</button>
        <button type="button">Primary</button>
      </>
    ),
  },
};

export const ActionsOnly: Story = {
  args: {
    actions: <button type="button">Save</button>,
  },
};
