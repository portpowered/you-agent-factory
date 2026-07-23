import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  SCROLL_AREA_HORIZONTAL_ANCHOR,
  SCROLL_AREA_MOBILE_ANCHOR,
} from "./overlay-story-copy";
import { overlayStoryDocs } from "./overlay-story-docs";
import { verifyScrollAreaKeyboardFocus } from "./overlay-storybook-play";
import { ScrollArea, ScrollBar } from "./scroll-area";

const meta = {
  title: "Overlays/ScrollArea",
  component: ScrollArea,
  parameters: {
    layout: "centered",
    docs: overlayStoryDocs,
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

export const HorizontalOverflow: Story = {
  render: () => (
    <ScrollArea className="w-72 rounded-2xl border border-outline p-3">
      <div className="flex w-max gap-4 pb-2 text-body-medium text-on-surface">
        {Array.from({ length: 20 }, (_, index) => {
          const label = `Wide horizontal scroll item ${index + 1}`;
          return (
            <div
              className="min-w-40 rounded-lg border border-outline px-3 py-2"
              key={label}
            >
              {label}
            </div>
          );
        })}
        <div className="min-w-40 rounded-lg border border-outline px-3 py-2">
          {SCROLL_AREA_HORIZONTAL_ANCHOR}
        </div>
      </div>
      <ScrollBar orientation="horizontal" />
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

export const MobileWidth: Story = {
  parameters: {
    viewport: {
      defaultViewport: "mobile1",
    },
  },
  render: () => (
    <ScrollArea className="h-40 w-full max-w-xs rounded-2xl border border-outline p-3">
      <div className="space-y-3 text-body-medium text-on-surface">
        <p>{SCROLL_AREA_MOBILE_ANCHOR}</p>
        {[
          "Mobile scroll row 1",
          "Mobile scroll row 2",
          "Mobile scroll row 3",
          "Mobile scroll row 4",
          "Mobile scroll row 5",
          "Mobile scroll row 6",
        ].map((row) => (
          <p key={row}>{row}</p>
        ))}
      </div>
    </ScrollArea>
  ),
};
