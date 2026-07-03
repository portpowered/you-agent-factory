import type { Meta, StoryObj } from "@storybook/react-vite";

import { ScrollArea } from "./scroll-area";
import { verifyScrollAreaKeyboardFocus } from "./overlay-storybook-play";

const meta = {
  title: "Overlays/ScrollArea",
  component: ScrollArea,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof ScrollArea>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <ScrollArea className="h-32 w-72 rounded-2xl border border-outline p-3">
      <div className="space-y-3 text-body-medium text-on-surface">
        {[
          "Scrollable row 1",
          "Scrollable row 2",
          "Scrollable row 3",
          "Scrollable row 4",
          "Scrollable row 5",
          "Scrollable row 6",
          "Scrollable row 7",
          "Scrollable row 8",
        ].map((row) => (
          <p key={row}>{row}</p>
        ))}
      </div>
    </ScrollArea>
  ),
};

export const KeyboardFocus: Story = {
  render: () => (
    <ScrollArea className="h-32 w-72 rounded-2xl border border-outline p-3">
      <div className="space-y-3 text-body-medium text-on-surface">
        <input
          aria-label="Scrollable field"
          className="w-full rounded-lg border border-outline bg-transparent px-3 py-2 text-on-surface"
        />
        {[
          "Scrollable row 1",
          "Scrollable row 2",
          "Scrollable row 3",
          "Scrollable row 4",
        ].map((row) => (
          <p key={row}>{row}</p>
        ))}
      </div>
    </ScrollArea>
  ),
  play: verifyScrollAreaKeyboardFocus,
};
