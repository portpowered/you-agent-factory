import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "./collapsible";
import {
  COLLAPSIBLE_CONTROLLED_ANCHOR,
  COLLAPSIBLE_NESTED_ANCHOR,
  COLLAPSIBLE_OPEN_ANCHOR,
} from "./overlay-story-copy";
import { overlayStoryDocs } from "./overlay-story-docs";
import { verifyCollapsibleKeyboardFocus } from "./overlay-storybook-play";

const meta = {
  title: "Overlays/Collapsible",
  component: Collapsible,
  parameters: {
    layout: "centered",
    docs: overlayStoryDocs,
  },
} satisfies Meta<typeof Collapsible>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <Collapsible className="w-72 rounded-2xl border border-outline p-3">
      <CollapsibleTrigger className="w-full text-left text-on-surface">
        Toggle details
      </CollapsibleTrigger>
      <CollapsibleContent className="pt-3 text-body-medium text-on-surface-variant">
        Collapsible content rendered from the package overlays category.
      </CollapsibleContent>
    </Collapsible>
  ),
};

export const Open: Story = {
  render: () => (
    <Collapsible
      className="w-72 rounded-2xl border border-outline p-3"
      defaultOpen
    >
      <CollapsibleTrigger className="w-full text-left text-on-surface">
        Open collapsible section
      </CollapsibleTrigger>
      <CollapsibleContent className="pt-3 text-body-medium text-on-surface-variant">
        {COLLAPSIBLE_OPEN_ANCHOR}
      </CollapsibleContent>
    </Collapsible>
  ),
};

export const Controlled: Story = {
  render: function ControlledStory() {
    const [open, setOpen] = useState(false);

    return (
      <div className="space-y-3">
        <button
          className="rounded-lg border border-outline px-4 py-2 text-on-surface"
          onClick={() => setOpen((current) => !current)}
          type="button"
        >
          Toggle controlled collapsible
        </button>
        <Collapsible
          className="w-72 rounded-2xl border border-outline p-3"
          onOpenChange={setOpen}
          open={open}
        >
          <CollapsibleTrigger className="w-full text-left text-on-surface">
            Controlled collapsible trigger
          </CollapsibleTrigger>
          <CollapsibleContent className="pt-3 text-body-medium text-on-surface-variant">
            {COLLAPSIBLE_CONTROLLED_ANCHOR}
          </CollapsibleContent>
        </Collapsible>
      </div>
    );
  },
};

export const NestedContent: Story = {
  render: () => (
    <Collapsible className="w-80 rounded-2xl border border-outline p-3">
      <CollapsibleTrigger className="w-full text-left text-on-surface">
        Parent collapsible section
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-3 pt-3 text-body-medium text-on-surface-variant">
        <p>Outer collapsible content for nested disclosure review.</p>
        <Collapsible className="rounded-xl border border-outline p-3">
          <CollapsibleTrigger className="w-full text-left text-on-surface">
            Nested collapsible trigger
          </CollapsibleTrigger>
          <CollapsibleContent className="pt-3 text-on-surface-variant">
            {COLLAPSIBLE_NESTED_ANCHOR}
          </CollapsibleContent>
        </Collapsible>
      </CollapsibleContent>
    </Collapsible>
  ),
};

export const KeyboardFocus: Story = {
  ...Default,
  play: verifyCollapsibleKeyboardFocus,
};
