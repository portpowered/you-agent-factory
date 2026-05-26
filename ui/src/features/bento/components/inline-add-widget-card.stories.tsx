import { expect, userEvent, within } from "storybook/test";
import { useState } from "react";

import "../../../styles.css";
import { DEFAULT_DASHBOARD_LAYOUT } from "../hooks/dashboardLayoutSchema";
import { getDashboardWidgetPickerAvailability } from "../lib/dashboard-widget-picker";
import { InlineAddWidgetCard } from "./inline-add-widget-card";

const pickerAvailability = getDashboardWidgetPickerAvailability(
  DEFAULT_DASHBOARD_LAYOUT,
);

export default {
  title: "you-agent-factory/Dashboard/Inline Add Widget Card",
  component: InlineAddWidgetCard,
  tags: ["test"],
};

function InlineAddWidgetCardStory() {
  const [pickerOpen, setPickerOpen] = useState(false);

  return (
    <InlineAddWidgetCard
      onPickerOpenChange={setPickerOpen}
      pickerAvailability={pickerAvailability}
      pickerOpen={pickerOpen}
    />
  );
}

export const ResponsivePickerFlow = {
  render: () => (
    <div style={{ maxWidth: "320px", padding: "1rem" }}>
      <InlineAddWidgetCardStory />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    const card = await canvas.findByRole("article", { name: "Add widget" });
    const action = within(card).getByRole("button", { name: "Add widget" });

    await expect(canvas.getByText("Ready to add")).toBeVisible();
    await expect(
      canvas.getByText("Add another dashboard card from this inline slot."),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Move Add widget" }),
    ).toBeVisible();

    await userEvent.click(action);

    await expect(
      await page.findByRole("dialog", { name: "Add dashboard widget" }),
    ).toBeVisible();
    await expect(
      await page.findByRole("button", {
        name: "Browse widgets: Workflow activity",
      }),
    ).toBeVisible();

    await userEvent.click(
      await page.findByRole("button", { name: "Close widget picker" }),
    );

    await expect(
      page.queryByRole("dialog", { name: "Add dashboard widget" }),
    ).toBeNull();
  },
};
