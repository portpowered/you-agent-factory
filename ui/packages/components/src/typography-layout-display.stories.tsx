import type { Meta, StoryObj } from "@storybook/react-vite";

import {
  ActionRow,
  DescriptionList,
  Heading,
  Label,
  SurfacePanel,
  Text,
} from "@you-agent-factory/components";

const meta = {
  title: "Showcase/TypographyLayoutDisplay",
  parameters: {
    layout: "padded",
  },
} satisfies Meta;

export default meta;

type Story = StoryObj<typeof meta>;

export const RepresentativeExample: Story = {
  render: () => (
    <SurfacePanel className="grid max-w-md gap-3" radius="lg">
      <Heading level="section">Resource details</Heading>
      <DescriptionList>
        <div>
          <Label as="dt">Name</Label>
          <Text as="dd">Example resource</Text>
        </div>
        <div>
          <Label as="dt">Status</Label>
          <Text as="dd">Ready</Text>
        </div>
      </DescriptionList>
      <ActionRow
        actions={<button type="button">Open</button>}
        statuses={<span>Last updated by host</span>}
      />
    </SurfacePanel>
  ),
};
