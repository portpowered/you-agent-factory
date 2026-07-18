import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { FactoryTopologyReplay } from "./factory-topology-replay";
import {
  factoryTopologyReplayMessages,
  factoryTopologyReplayProjection,
} from "./factory-topology-replay-fixtures";

function ControlledTopologyStory() {
  const [selectedNodeId, setSelectedNodeId] = useState<string>();
  return (
    <div
      className="w-full max-w-7xl p-3 sm:p-5"
      data-story-shell="factory-topology-replay"
    >
      <FactoryTopologyReplay
        className="h-96 sm:h-[32rem]"
        formatNumber={(value) => new Intl.NumberFormat("de-DE").format(value)}
        messages={factoryTopologyReplayMessages}
        onSelectNode={setSelectedNodeId}
        projection={factoryTopologyReplayProjection}
        selectedNodeId={selectedNodeId}
      />
    </div>
  );
}

const meta = {
  title: "Visualizers/FactoryTopologyReplay",
  component: FactoryTopologyReplay,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof FactoryTopologyReplay>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Ready: Story = {
  args: {
    messages: factoryTopologyReplayMessages,
    projection: factoryTopologyReplayProjection,
  },
  render: () => <ControlledTopologyStory />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const review = await canvas.findByRole("button", { name: "Review" });
    await userEvent.click(review);
    await expect(canvas.getByText("Selected")).toBeVisible();
    await expect(canvas.getByText("1.234 Work")).toBeVisible();
  },
};
