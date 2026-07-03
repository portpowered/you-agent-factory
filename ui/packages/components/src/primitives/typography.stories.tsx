import type { Meta, StoryObj } from "@storybook/react-vite";

import { Code, Heading, Label, Text } from "./typography";

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
