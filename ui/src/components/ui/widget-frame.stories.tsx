import { expect, within } from "storybook/test";

import { NoSelectionDetailCard } from "../../features/current-selection/no-selection-detail-card";
import "../../styles.css";
import { DashboardWidgetFrame } from "./widget-frame";

export default {
  title: "Agent Factory/Widget Frame",
  component: DashboardWidgetFrame,
};

export const CurrentSelectionEmptyState = {
  render: () => (
    <div style={{ maxWidth: "720px", padding: "1rem" }}>
      <NoSelectionDetailCard />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(await canvas.findByRole("article", { name: "Current selection" })).toBeVisible();
    await expect(await canvas.findByRole("button", { name: "Undo selection" })).toBeDisabled();
    await expect(await canvas.findByRole("button", { name: "Redo selection" })).toBeDisabled();
    await expect(
      await canvas.findByText(
        "Select a workstation, work item, or state node to inspect live details.",
      ),
    ).toBeVisible();
  },
};

