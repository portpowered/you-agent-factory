import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "./dialog";
import {
  createLongParagraphs,
  DIALOG_CONTROLLED_ANCHOR,
  DIALOG_ESCAPE_ANCHOR,
  DIALOG_LONG_CONTENT_ANCHOR,
  DIALOG_MOBILE_ANCHOR,
} from "./overlay-story-copy";
import { overlayStoryDocs } from "./overlay-story-docs";
import {
  verifyDialogEscapeClose,
  verifyDialogKeyboardFocus,
} from "./overlay-storybook-play";

const meta = {
  title: "Overlays/Dialog",
  component: Dialog,
  parameters: {
    layout: "centered",
    docs: overlayStoryDocs,
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
          Host apps supply labels, ids, and children without dashboard
          providers.
        </p>
      </DialogContent>
    </Dialog>
  ),
};

export const LongContent: Story = {
  render: () => (
    <Dialog>
      <DialogTrigger className="rounded-lg border border-outline px-4 py-2 text-on-surface">
        Open long dialog
      </DialogTrigger>
      <DialogContent aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>Long dialog content</DialogTitle>
          <DialogDescription>
            Review vertical overflow inside the dialog shell.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3 text-body-medium text-on-surface">
          {createLongParagraphs("Dialog long content", 20).map((paragraph) => (
            <p key={paragraph}>{paragraph}</p>
          ))}
          <p>{DIALOG_LONG_CONTENT_ANCHOR}</p>
        </div>
      </DialogContent>
    </Dialog>
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
          Open controlled dialog
        </button>
        <Dialog onOpenChange={setOpen} open={open}>
          <DialogContent aria-describedby={undefined}>
            <DialogHeader>
              <DialogTitle>Controlled dialog</DialogTitle>
              <DialogDescription>{DIALOG_CONTROLLED_ANCHOR}</DialogDescription>
            </DialogHeader>
          </DialogContent>
        </Dialog>
      </div>
    );
  },
};

export const EscapeClose: Story = {
  render: () => (
    <Dialog>
      <DialogTrigger className="rounded-lg border border-outline px-4 py-2 text-on-surface">
        Open dialog for Escape
      </DialogTrigger>
      <DialogContent aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>Escape close dialog</DialogTitle>
          <DialogDescription>{DIALOG_ESCAPE_ANCHOR}</DialogDescription>
        </DialogHeader>
      </DialogContent>
    </Dialog>
  ),
  play: verifyDialogEscapeClose,
};

export const MobileViewport: Story = {
  parameters: {
    viewport: {
      defaultViewport: "mobile1",
    },
  },
  render: () => (
    <Dialog>
      <DialogTrigger className="rounded-lg border border-outline px-4 py-2 text-on-surface">
        Open mobile dialog
      </DialogTrigger>
      <DialogContent aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>Mobile dialog</DialogTitle>
          <DialogDescription>{DIALOG_MOBILE_ANCHOR}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3 text-body-medium text-on-surface">
          {createLongParagraphs("Mobile dialog content", 8).map((paragraph) => (
            <p key={paragraph}>{paragraph}</p>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  ),
};

export const KeyboardFocus: Story = {
  ...Default,
  play: verifyDialogKeyboardFocus,
};
