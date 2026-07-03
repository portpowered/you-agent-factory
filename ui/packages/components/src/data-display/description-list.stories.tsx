import type { Meta, StoryObj } from "@storybook/react-vite";

import { Label, Text } from "../primitives/typography";
import { DescriptionList } from "./description-list";

const LONG_LABEL =
  "Extremely long host-supplied label that should remain readable without forcing horizontal page overflow";
const LONG_VALUE =
  "Host-supplied value with a very long identifier or message that should wrap within the description-list layout instead of clipping or overflowing the page";
const EMPTY_VALUE = "—";

const meta = {
  title: "Data Display/DescriptionList",
  component: DescriptionList,
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof DescriptionList>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <DescriptionList>
      <div>
        <Label as="dt">Status</Label>
        <Text as="dd">Active</Text>
      </div>
      <div>
        <Label as="dt">Owner</Label>
        <Text as="dd">Host application</Text>
      </div>
    </DescriptionList>
  ),
};

export const Compact: Story = {
  render: () => (
    <DescriptionList className="gap-1 [&_div]:grid-cols-[7rem_minmax(0,1fr)]">
      <div>
        <Label as="dt">Revision</Label>
        <Text as="dd" variant="dense">
          Dense metadata row value
        </Text>
      </div>
      <div>
        <Label as="dt">Updated</Label>
        <Text as="dd" variant="dense">
          2 minutes ago
        </Text>
      </div>
    </DescriptionList>
  ),
};

export const WideLayout: Story = {
  render: () => (
    <DescriptionList className="w-full max-w-4xl gap-3 md:grid-cols-2 [&_div]:grid-cols-[8.5rem_minmax(0,1fr)]">
      <div>
        <Label as="dt">Name</Label>
        <Text as="dd">Example resource</Text>
      </div>
      <div>
        <Label as="dt">Status</Label>
        <Text as="dd">Ready</Text>
      </div>
      <div>
        <Label as="dt">Owner</Label>
        <Text as="dd">Host application</Text>
      </div>
      <div>
        <Label as="dt">Region</Label>
        <Text as="dd">us-east-1</Text>
      </div>
    </DescriptionList>
  ),
};

export const LongLabel: Story = {
  render: () => (
    <div className="max-w-sm">
      <DescriptionList className="[&_div]:grid-cols-[8.5rem_minmax(0,1fr)]">
        <div>
          <Label as="dt" truncate>
            {LONG_LABEL}
          </Label>
          <Text as="dd">Ready</Text>
        </div>
      </DescriptionList>
    </div>
  ),
};

export const LongValue: Story = {
  render: () => (
    <div className="max-w-sm">
      <DescriptionList className="[&_div]:grid-cols-[8.5rem_minmax(0,1fr)]">
        <div>
          <Label as="dt">Message</Label>
          <Text as="dd" wrap>
            {LONG_VALUE}
          </Text>
        </div>
      </DescriptionList>
    </div>
  ),
};

export const EmptyValue: Story = {
  render: () => (
    <DescriptionList className="[&_div]:grid-cols-[8.5rem_minmax(0,1fr)]">
      <div>
        <Label as="dt">Trace ID</Label>
        <Text as="dd">{EMPTY_VALUE}</Text>
      </div>
      <div>
        <Label as="dt">Owner</Label>
        <Text as="dd">{EMPTY_VALUE}</Text>
      </div>
    </DescriptionList>
  ),
};

export const NarrowViewport: Story = {
  render: () => (
    <div className="w-full max-w-xs">
      <DescriptionList className="gap-2 [&_div]:grid-cols-[7rem_minmax(0,1fr)]">
        <div>
          <Label as="dt">Status</Label>
          <Text as="dd" wrap>
            {LONG_VALUE}
          </Text>
        </div>
        <div>
          <Label as="dt">Owner</Label>
          <Text as="dd" variant="dense">
            Dense metadata row
          </Text>
        </div>
      </DescriptionList>
    </div>
  ),
};

export const MobileDescriptionList: Story = {
  parameters: {
    layout: "padded",
    viewport: {
      defaultViewport: "mobile1",
    },
  },
  render: () => (
    <div className="w-full max-w-xs">
      <DescriptionList className="gap-2 [&_div]:grid-cols-[7rem_minmax(0,1fr)]">
        <div>
          <Label as="dt">Status</Label>
          <Text as="dd" wrap>
            {LONG_VALUE}
          </Text>
        </div>
        <div>
          <Label as="dt" truncate>
            {LONG_LABEL}
          </Label>
          <Text as="dd" variant="dense">
            Dense metadata row
          </Text>
        </div>
      </DescriptionList>
    </div>
  ),
};

export const DesktopDescriptionList: Story = {
  parameters: {
    layout: "padded",
    viewport: {
      defaultViewport: "desktop",
    },
  },
  render: () => (
    <div className="w-full max-w-2xl">
      <DescriptionList className="gap-3 md:grid-cols-2 [&_div]:grid-cols-[8.5rem_minmax(0,1fr)]">
        <div>
          <Label as="dt">Name</Label>
          <Text as="dd">Example resource</Text>
        </div>
        <div>
          <Label as="dt">Status</Label>
          <Text as="dd">Ready</Text>
        </div>
        <div>
          <Label as="dt">Message</Label>
          <Text as="dd" wrap>
            {LONG_VALUE}
          </Text>
        </div>
        <div>
          <Label as="dt">Owner</Label>
          <Text as="dd">Host application</Text>
        </div>
      </DescriptionList>
    </div>
  ),
};
