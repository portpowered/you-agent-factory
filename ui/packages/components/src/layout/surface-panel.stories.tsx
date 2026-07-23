import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";

import { Heading, Label, Text } from "../primitives/typography";
import { ActionRow } from "./action-row";
import { SurfacePanel } from "./surface-panel";

const LONG_LABEL =
  "Host-supplied panel label that should remain readable without forcing horizontal overflow";

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
  title: "Layout/SurfacePanel",
  component: SurfacePanel,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof SurfacePanel>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    children: "Surface panel content supplied by the host application",
  },
};

export const LowSurface: Story = {
  args: {
    children: "Low-surface panel",
    radius: "lg",
    surface: "low",
  },
};

export const Dense: Story = {
  render: () => (
    <SurfacePanel className="grid gap-2" padding="compact" radius="lg">
      <Heading level="section">Compact panel</Heading>
      <Text variant="dense">
        Dense panel body copy supplied by the host application
      </Text>
    </SurfacePanel>
  ),
};

export const LongLabel: Story = {
  render: () => (
    <SurfacePanel className="grid max-w-md gap-3" radius="lg">
      <Heading level="section" truncate>
        {LONG_LABEL}
      </Heading>
      <Text>Supporting panel content remains inside the bordered surface.</Text>
    </SurfacePanel>
  ),
};

export const StructuredPanel: Story = {
  render: () => (
    <SurfacePanel className="grid max-w-md gap-3" radius="lg">
      <header className="grid gap-1">
        <Label>Section label</Label>
        <Heading level="section">Panel heading</Heading>
      </header>
      <Text>Panel body content supplied by the host application.</Text>
      <footer>
        <ActionRow
          actions={
            <>
              <DemoButton>Cancel</DemoButton>
              <DemoButton>Save</DemoButton>
            </>
          }
        />
      </footer>
    </SurfacePanel>
  ),
};

export const Wide: Story = {
  parameters: {
    layout: "padded",
  },
  render: () => (
    <SurfacePanel className="grid w-full max-w-4xl gap-4" radius="2xl">
      <Heading level="section">Wide surface panel</Heading>
      <Text>
        Surface panels preserve border, spacing, radius, and content structure
        at wider dashboard widths.
      </Text>
      <ActionRow
        actions={<DemoButton>Open details</DemoButton>}
        statuses={<span>Ready</span>}
      />
    </SurfacePanel>
  ),
};

export const MobileSurfacePanelLayout: Story = {
  parameters: {
    layout: "padded",
    viewport: {
      defaultViewport: "mobile1",
    },
  },
  render: () => (
    <SurfacePanel
      className="grid w-full min-w-0 max-w-xs gap-3"
      padding="compact"
      radius="lg"
    >
      <Heading level="section">Mobile panel</Heading>
      <Text variant="dense">
        Dense panel content remains readable on narrow screens.
      </Text>
      <ActionRow
        className="min-w-0 w-full"
        actions={
          <>
            <DemoButton>Cancel</DemoButton>
            <DemoButton>Save</DemoButton>
          </>
        }
        statuses={<Text truncate>{LONG_LABEL}</Text>}
      />
    </SurfacePanel>
  ),
};

export const DesktopSurfacePanelLayout: Story = {
  parameters: {
    layout: "padded",
    viewport: {
      defaultViewport: "desktop",
    },
  },
  render: () => (
    <SurfacePanel className="grid w-full max-w-2xl gap-4" radius="2xl">
      <Heading level="section">Desktop panel</Heading>
      <Text>
        Surface panel structure remains readable at wider dashboard viewport
        sizes.
      </Text>
      <ActionRow
        actions={<DemoButton>Primary action</DemoButton>}
        statuses={<span>Ready for review</span>}
      />
    </SurfacePanel>
  ),
};
