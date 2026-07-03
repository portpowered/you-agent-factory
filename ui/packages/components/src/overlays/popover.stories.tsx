import type { Meta, StoryObj } from "@storybook/react-vite";

import { Popover, PopoverContent, PopoverTrigger } from "./popover";
import { verifyPopoverKeyboardFocus } from "./overlay-storybook-play";

const meta = {
  title: "Overlays/Popover",
  component: Popover,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof Popover>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <Popover>
      <PopoverTrigger className="rounded-lg border border-outline px-4 py-2 text-on-surface">
        Open popover
      </PopoverTrigger>
      <PopoverContent>
        <p className="text-body-medium text-on-surface">
          Popover content from the component package.
        </p>
      </PopoverContent>
    </Popover>
  ),
};

export const KeyboardFocus: Story = {
  ...Default,
  play: verifyPopoverKeyboardFocus,
};
