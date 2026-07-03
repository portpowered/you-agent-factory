import type { Meta, StoryObj } from "@storybook/react-vite";

import { Label, Text } from "../primitives/typography";
import { DescriptionList } from "./description-list";

const meta = {
  title: "Data Display/DescriptionList",
  component: DescriptionList,
  parameters: {
    layout: "centered",
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
