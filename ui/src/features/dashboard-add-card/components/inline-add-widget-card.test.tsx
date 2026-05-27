import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { DEFAULT_DASHBOARD_LAYOUT } from "../../bento/hooks/dashboardLayoutSchema";
import { getDashboardWidgetPickerAvailability } from "../../bento/lib/dashboard-widget-picker";
import { InlineAddWidgetCard } from "./inline-add-widget-card";

const pickerAvailability = getDashboardWidgetPickerAvailability(
  DEFAULT_DASHBOARD_LAYOUT,
);

function getCardActionButton() {
  return screen.getByRole("button", { name: "Add widget: Work totals" });
}

function renderControlledCard(
  onSelectWidget?: (
    widgetType: (typeof pickerAvailability)[number]["widgetType"],
  ) => void,
) {
  render(
    <InlineAddWidgetCard
      onSelectWidget={onSelectWidget}
      pickerAvailability={pickerAvailability}
    />,
  );
}

describe("InlineAddWidgetCard content", () => {
  it("renders discoverable add-widget copy inside a dashboard grid card", () => {
    render(<InlineAddWidgetCard pickerAvailability={pickerAvailability} />);

    const card = screen.getByRole("article", { name: "Add widget" });

    expect(card.dataset.dashboardPanelShell).toBe("grid-card");
    expect(
      within(card).getByRole("heading", { name: "Add widget" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("combobox", { name: "Browse widgets" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("button", {
        name: "Add widget: Work totals",
      }),
    ).toBeTruthy();
    expect(
      within(card).queryByText("Duplicate allowed: Work totals"),
    ).toBeNull();
    expect(
      within(card).queryByText(
        "Track headline counts for dispatched, completed, and failed work.",
      ),
    ).toBeNull();
    expect(
      screen
        .getByRole("button", { name: "Move Add widget" })
        .getAttribute("data-bento-drag-handle"),
    ).toBe("true");
  });

  it("renders available and unavailable widgets in the selector", () => {
    renderControlledCard();

    const selector = screen.getByRole("combobox", { name: "Browse widgets" });

    expect(selector).toHaveProperty("value", "work-totals");
    expect(
      screen.getByRole("option", { name: "Workflow activity" }),
    ).toHaveProperty("disabled", false);
    expect(
      screen.getByRole("option", { name: "Current selection" }),
    ).toHaveProperty("disabled", true);
  });

  it("supports keyboard selection and keeps the plus action focusable", async () => {
    const user = userEvent.setup();

    renderControlledCard();

    const selector = screen.getByRole("combobox", { name: "Browse widgets" });
    const actionButton = getCardActionButton();

    expect(actionButton.className).toContain("focus-visible:ring-2");

    await user.selectOptions(selector, "terminal-work");

    expect(selector).toHaveProperty("value", "terminal-work");
    expect(
      screen.getByRole("button", { name: "Add widget: Terminal work" }),
    ).toBeTruthy();
    expect(
      screen.queryByText(
        "Review finished and failed work items in one compact list.",
      ),
    ).toBeNull();

    actionButton.focus();
    expect(document.activeElement).toBe(actionButton);
  });

  it("localizes the visible add-widget card title", () => {
    render(
      <InlineAddWidgetCard
        locale="zh-CN"
        pickerAvailability={pickerAvailability}
      />,
    );

    expect(screen.getByRole("article", { name: "添加小组件" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "添加小组件" })).toBeTruthy();
  });
});

describe("InlineAddWidgetCard interactions", () => {
  it("reports the selected widget type back to the dashboard seam", () => {
    const onSelectWidget = vi.fn();

    renderControlledCard(onSelectWidget);

    fireEvent.click(getCardActionButton());

    expect(onSelectWidget).toHaveBeenCalledWith("work-totals");
  });

  it("reports the selected widget type after choosing a different card", () => {
    const onSelectWidget = vi.fn();

    renderControlledCard(onSelectWidget);

    fireEvent.change(screen.getByRole("combobox", { name: "Browse widgets" }), {
      target: { value: "terminal-work" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Add widget: Terminal work" }),
    );

    expect(onSelectWidget).toHaveBeenCalledWith("terminal-work");
  });

  it("shows an explicit unavailable state and disables adding a widget", () => {
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

    const actionButton = screen.getByRole("button", { name: "Add widget" });

    expect(
      screen.getByRole("option", { name: "No widgets available" }),
    ).toBeTruthy();
    expect(screen.queryByText(/Remove a singleton widget/)).toBeNull();
    expect(
      screen.getByRole("combobox", { name: "Browse widgets" }),
    ).toHaveProperty("disabled", true);
    expect(actionButton).toHaveProperty("disabled", true);
  });
});
