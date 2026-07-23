import type { Meta, StoryObj } from "@storybook/react-vite";

import { Code, Heading, Label, Text } from "./typography";

const LONG_LABEL =
  "Extremely long host-supplied label that should truncate instead of forcing horizontal overflow";

const meta = {
  title: "Primitives/Typography",
  component: Text,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof Text>;

export default meta;

type Story = StoryObj<typeof meta>;

export const BodyText: Story = {
  args: {
    children: "Body copy supplied by the host application",
  },
};

export const SupportingText: Story = {
  args: {
    children: "Supporting metadata",
    variant: "supporting",
  },
};

export const MutedText: Story = {
  args: {
    children: "Muted secondary metadata",
    variant: "muted",
  },
};

export const CaptionText: Story = {
  args: {
    children: "Caption supplied by the host application",
    variant: "caption",
  },
};

export const DenseText: Story = {
  args: {
    children: "Dense metadata row copy",
    variant: "dense",
  },
};

export const PageHeading: Story = {
  render: () => <Heading level="page">Page title</Heading>,
};

export const SectionHeading: Story = {
  render: () => <Heading level="section">Section title</Heading>,
};

export const FieldLabel: Story = {
  render: () => <Label>Field label</Label>,
};

export const InlineCode: Story = {
  render: () => <Code size="supporting">example-id</Code>,
};

export const LongTruncatedText: Story = {
  render: () => (
    <div className="w-48">
      <Text truncate>{LONG_LABEL}</Text>
    </div>
  ),
};

export const LongWrappingText: Story = {
  render: () => (
    <div className="w-48">
      <Text wrap>{LONG_LABEL}</Text>
    </div>
  ),
};

export const TypographyRoleShowcase: Story = {
  render: () => (
    <div className="grid max-w-md gap-3">
      <Heading level="page">Page heading</Heading>
      <Heading level="section">Section heading</Heading>
      <Text>Body text supplied by the host application</Text>
      <Text variant="supporting">Supporting metadata</Text>
      <Text variant="muted">Muted secondary metadata</Text>
      <Text variant="caption">Caption copy</Text>
      <Text variant="dense">Dense metadata row</Text>
      <Label>Field label</Label>
      <Code size="supporting">example-id</Code>
    </div>
  ),
};

export const MobileTypographyRoles: Story = {
  parameters: {
    layout: "padded",
    viewport: {
      defaultViewport: "mobile1",
    },
  },
  render: () => (
    <div className="grid w-full min-w-0 max-w-xs gap-3">
      <Heading level="section">Section heading</Heading>
      <Text variant="dense">
        Dense metadata remains readable on narrow screens
      </Text>
      <div className="min-w-0 w-full">
        <Text truncate>{LONG_LABEL}</Text>
      </div>
    </div>
  ),
};

export const DesktopTypographyRoles: Story = {
  parameters: {
    layout: "padded",
    viewport: {
      defaultViewport: "desktop",
    },
  },
  render: () => (
    <div className="grid w-full min-w-0 max-w-2xl gap-3">
      <Heading level="page">Page heading</Heading>
      <Text>Body text remains readable at wider dashboard widths</Text>
      <div className="min-w-0 w-full max-w-96">
        <Text truncate>{LONG_LABEL}</Text>
      </div>
    </div>
  ),
};
