import type { Meta, StoryObj } from "@storybook/react-vite";

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "./collapsible";

const meta = {
  title: "Overlays/Collapsible",
  component: Collapsible,
  parameters: {
    layout: "centered",
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
