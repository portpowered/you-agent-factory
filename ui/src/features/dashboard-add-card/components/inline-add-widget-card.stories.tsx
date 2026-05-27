import { expect, userEvent, within } from "storybook/test";

import "../../../styles.css";
import { DEFAULT_DASHBOARD_LAYOUT } from "../../bento/hooks/dashboardLayoutSchema";
import { getDashboardWidgetPickerAvailability } from "../../bento/lib/dashboard-widget-picker";
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
  return <InlineAddWidgetCard pickerAvailability={pickerAvailability} />;
}

export const ResponsiveSelectorFlow = {
  render: () => (
    <div style={{ maxWidth: "320px", padding: "1rem" }}>
      <InlineAddWidgetCardStory />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Add widget" });
    const selector = within(card).getByRole("combobox", {
      name: "Browse widgets",
    });

    await expect(
      canvas.getByRole("heading", { name: "Add widget" }),
    ).toBeVisible();
    await expect(selector).toHaveValue("work-totals");
    await expect(
      canvas.getByRole("button", { name: "Move Add widget" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Add widget: Work totals" }),
    ).toBeVisible();

    await userEvent.selectOptions(selector, "terminal-work");

    await expect(
      canvas.getByRole("button", { name: "Add widget: Terminal work" }),
    ).toBeVisible();
    await expect(
      canvas.queryByText(
        "Review finished and failed work items in one compact list.",
      ),
    ).toBeNull();
  },
};
