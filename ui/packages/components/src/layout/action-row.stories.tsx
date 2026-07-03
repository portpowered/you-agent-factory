import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";

import { Text } from "../primitives/typography";
import { ActionRow } from "./action-row";

const LONG_STATUS =
  "Host-supplied status label that remains readable when the action row wraps on narrow screens";

const demoButtonClass =
  "inline-flex min-h-10 shrink-0 items-center rounded-full border border-outline px-4 text-sm";

function DemoButton({ children }: { children: ReactNode }) {
  return (
    <button className={demoButtonClass} type="button">
      {children}
    </button>
  );
}

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
        <DemoButton>Secondary</DemoButton>
        <DemoButton>Primary</DemoButton>
      </>
    ),
  },
};

export const ActionsOnly: Story = {
  args: {
    actions: <DemoButton>Save</DemoButton>,
  },
};

export const Dense: Story = {
  render: () => (
    <ActionRow
      actions={
        <>
          <DemoButton>Discard</DemoButton>
          <DemoButton>Save draft</DemoButton>
          <DemoButton>Publish</DemoButton>
        </>
      }
      statuses={<Text variant="dense">Dense metadata row status</Text>}
    />
  ),
};

export const LongLabel: Story = {
  render: () => (
    <div className="max-w-md">
      <ActionRow
        actions={
          <>
            <DemoButton>Discard changes</DemoButton>
            <DemoButton>Save draft</DemoButton>
          </>
        }
        statuses={<Text truncate>{LONG_STATUS}</Text>}
      />
    </div>
  ),
};

export const Wrapped: Story = {
  render: () => (
    <div className="max-w-xs">
      <ActionRow
        actions={
          <>
            <DemoButton>Discard</DemoButton>
            <DemoButton>Save draft</DemoButton>
            <DemoButton>Publish</DemoButton>
            <DemoButton>Archive</DemoButton>
          </>
        }
        statuses={<span>{LONG_STATUS}</span>}
      />
    </div>
  ),
};

export const Wide: Story = {
  parameters: {
    layout: "padded",
  },
  render: () => (
    <div className="w-full max-w-4xl">
      <ActionRow
        actions={
          <>
            <DemoButton>Secondary</DemoButton>
            <DemoButton>Primary</DemoButton>
          </>
        }
        statuses={<span>Ready for review</span>}
      />
    </div>
  ),
};

export const MobileActionRowWrapping: Story = {
  parameters: {
    layout: "padded",
    viewport: {
      defaultViewport: "mobile1",
    },
  },
  render: () => (
    <div className="w-full max-w-xs">
      <ActionRow
        actions={
          <>
            <DemoButton>Discard</DemoButton>
            <DemoButton>Save draft</DemoButton>
            <DemoButton>Publish</DemoButton>
          </>
        }
        statuses={<Text variant="dense">{LONG_STATUS}</Text>}
      />
    </div>
  ),
};

export const DesktopActionRowLayout: Story = {
  parameters: {
    layout: "padded",
    viewport: {
      defaultViewport: "desktop",
    },
  },
  render: () => (
    <div className="w-full max-w-4xl">
      <ActionRow
        actions={
          <>
            <DemoButton>Secondary</DemoButton>
            <DemoButton>Primary</DemoButton>
          </>
        }
        statuses={<span>Ready for review at wider dashboard widths</span>}
      />
    </div>
  ),
};
