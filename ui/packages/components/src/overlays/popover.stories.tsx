import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
  createLongParagraphs,
  POPOVER_CONTROLLED_ANCHOR,
  POPOVER_KEYBOARD_OPEN_ANCHOR,
  POPOVER_LONG_CONTENT_ANCHOR,
  POPOVER_VIEWPORT_PLACEMENT_ANCHOR,
} from "./overlay-story-copy";
import { overlayStoryDocs } from "./overlay-story-docs";
import {
  verifyPopoverKeyboardFocus,
  verifyPopoverKeyboardOpen,
} from "./overlay-storybook-play";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";

const meta = {
  title: "Overlays/Popover",
  component: Popover,
  parameters: {
    layout: "centered",
    docs: overlayStoryDocs,
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

export const LongContent: Story = {
  render: () => (
    <Popover>
      <PopoverTrigger className="rounded-lg border border-outline px-4 py-2 text-on-surface">
        Open long popover
      </PopoverTrigger>
      <PopoverContent className="max-h-64 overflow-y-auto">
        <div className="space-y-3 text-body-medium text-on-surface">
          {createLongParagraphs("Popover long content", 20).map((paragraph) => (
            <p key={paragraph}>{paragraph}</p>
          ))}
          <p>{POPOVER_LONG_CONTENT_ANCHOR}</p>
        </div>
      </PopoverContent>
    </Popover>
  ),
};

export const ControlledOpen: Story = {
  render: function ControlledOpenStory() {
    const [open, setOpen] = useState(false);

    return (
      <div className="space-y-3">
        <button
          className="rounded-lg border border-outline px-4 py-2 text-on-surface"
          onClick={() => setOpen(true)}
          type="button"
        >
          Open controlled popover
        </button>
        <Popover onOpenChange={setOpen} open={open}>
          <PopoverTrigger className="rounded-lg border border-outline px-4 py-2 text-on-surface">
            Popover anchor
          </PopoverTrigger>
          <PopoverContent>
            <p className="text-body-medium text-on-surface">
              {POPOVER_CONTROLLED_ANCHOR}
            </p>
          </PopoverContent>
        </Popover>
      </div>
    );
  },
};

export const KeyboardOpen: Story = {
  render: () => (
    <Popover>
      <PopoverTrigger className="rounded-lg border border-outline px-4 py-2 text-on-surface">
        Open popover with keyboard
      </PopoverTrigger>
      <PopoverContent>
        <p className="text-body-medium text-on-surface">
          {POPOVER_KEYBOARD_OPEN_ANCHOR}
        </p>
      </PopoverContent>
    </Popover>
  ),
  play: verifyPopoverKeyboardOpen,
};

export const ViewportPlacement: Story = {
  parameters: {
    layout: "fullscreen",
    viewport: {
      defaultViewport: "mobile1",
    },
  },
  render: () => (
    <div className="flex h-[32rem] w-full items-end justify-end p-4">
      <Popover>
        <PopoverTrigger className="rounded-lg border border-outline px-4 py-2 text-on-surface">
          Edge popover trigger
        </PopoverTrigger>
        <PopoverContent align="end" collisionPadding={16} side="top">
          <p className="max-w-[16rem] text-body-medium text-on-surface">
            {POPOVER_VIEWPORT_PLACEMENT_ANCHOR}
          </p>
        </PopoverContent>
      </Popover>
    </div>
  ),
};

export const KeyboardFocus: Story = {
  ...Default,
  play: verifyPopoverKeyboardFocus,
};
