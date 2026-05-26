import { fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";

import { DEFAULT_DASHBOARD_LAYOUT } from "../hooks/dashboardLayoutSchema";
import { getDashboardWidgetPickerAvailability } from "../lib/dashboard-widget-picker";
import { InlineAddWidgetCard } from "./inline-add-widget-card";

const pickerAvailability = getDashboardWidgetPickerAvailability(
  DEFAULT_DASHBOARD_LAYOUT,
);

function getCardActionButton() {
  return screen.getByRole("button", { name: "Add widget" });
}

function renderControlledCard(
  onSelectWidget?: (
    widgetType: (typeof pickerAvailability)[number]["widgetType"],
  ) => void,
) {
  function TestHarness() {
    const [pickerOpen, setPickerOpen] = useState(false);

    return (
      <InlineAddWidgetCard
        onPickerOpenChange={setPickerOpen}
        onSelectWidget={onSelectWidget}
        pickerAvailability={pickerAvailability}
        pickerOpen={pickerOpen}
      />
    );
  }

  render(<TestHarness />);
}

describe("InlineAddWidgetCard", () => {
  it("renders discoverable add-widget copy inside a dashboard grid card", () => {
    render(
      <InlineAddWidgetCard pickerAvailability={pickerAvailability} />,
    );

    const card = screen.getByRole("article", { name: "Add widget" });

    expect(card.dataset.dashboardPanelShell).toBe("grid-card");
    expect(within(card).getByText("Ready to add")).toBeTruthy();
    expect(within(card).getByText("Add another dashboard card from this inline slot.")).toBeTruthy();
    expect(within(card).getByText("Browse widgets")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Move Add widget" }).getAttribute(
        "data-bento-drag-handle",
      ),
    ).toBe("true");
  });

  it("opens and dismisses the inline widget picker from the card action", () => {
    renderControlledCard();

    fireEvent.click(getCardActionButton());

    expect(
      screen.getByRole("dialog", { name: "Add dashboard widget" }),
    ).toBeTruthy();
    expect(screen.getByText("Workflow activity")).toBeTruthy();
    expect(screen.getByText("Provider session")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Browse widgets: Workflow activity" }),
    ).toHaveProperty("disabled", false);
    expect(
      screen.getByRole("button", { name: "Browse widgets: Current selection" }),
    ).toHaveProperty("disabled", true);

    fireEvent.click(screen.getByRole("button", { name: "Close widget picker" }));

    expect(
      screen.queryByRole("dialog", { name: "Add dashboard widget" }),
    ).toBeNull();
    expect(screen.getByText("Ready to add")).toBeTruthy();
  });

  it("localizes the visible add-widget card title", () => {
    render(
      <InlineAddWidgetCard
        locale="zh-CN"
        pickerAvailability={pickerAvailability}
      />,
    );

    expect(screen.getByRole("article", { name: "添加小组件" })).toBeTruthy();
    expect(screen.getByText("从这个内联位置添加另一个仪表板卡片。")).toBeTruthy();
  });

  it("reports the selected widget type back to the dashboard seam", () => {
    const onSelectWidget = vi.fn();

    renderControlledCard(onSelectWidget);

    fireEvent.click(getCardActionButton());
    fireEvent.click(
      screen.getByRole("button", { name: "Browse widgets: Workflow activity" }),
    );

    expect(onSelectWidget).toHaveBeenCalledWith("work-graph");
  });

  it("shows an explicit unavailable state and disables opening the picker", () => {
    render(
      <InlineAddWidgetCard
        pickerAvailability={[
          {
            duplicateCapable: false,
            enabled: false,
            widgetType: "current-selection",
          },
        ]}
      />,
    );

    const actionButton = getCardActionButton();

    expect(screen.getByText("No additional widgets are available from this layout.")).toBeTruthy();
    expect(
      screen.getByText(
        "Remove a singleton widget or keep using duplicate-capable widgets to make room for a different card.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("No widgets available")).toBeTruthy();
    expect(actionButton).toHaveProperty("disabled", true);
    expect(
      screen.queryByRole("dialog", { name: "Add dashboard widget" }),
    ).toBeNull();
  });
});
