import type { Meta, StoryObj } from "@storybook/react-vite";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "./dialog";

const meta = {
  title: "Overlays/Dialog",
  component: Dialog,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof Dialog>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <Dialog>
      <DialogTrigger className="rounded-lg border border-outline px-4 py-2 text-on-surface">
        Open dialog
      </DialogTrigger>
      <DialogContent aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>Package dialog</DialogTitle>
          <DialogDescription>
            Dialog content rendered from @you-agent-factory/components.
          </DialogDescription>
        </DialogHeader>
        <p className="text-body-medium text-on-surface">
          Host apps supply labels, ids, and children without dashboard providers.
        </p>
      </DialogContent>
    </Dialog>
  ),
};
