import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { selectComboboxOption } from "../../../testing/select-test-helpers";
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
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

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
    const header = card.querySelector("header");
    expect(header?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(header?.className).toContain("cursor-grab");
    expect(header?.className).toContain("bg-surface-container-high");
    expect(
      header?.querySelector("[data-bento-header-action-spacer='true']"),
    ).toBeTruthy();
    expect(card.querySelector("[class*='content-end']")).toBeNull();
    expect(
      within(card).queryByRole("button", { name: "Move Add widget" }),
    ).toBeNull();
  });

  it("renders available and unavailable widgets in the selector", async () => {
    const user = userEvent.setup();

    renderControlledCard();

    const selector = screen.getByRole("combobox", { name: "Browse widgets" });

    expect(selector).toHaveTextContent("Work totals");
    await user.click(selector);
    const listbox = await screen.findByRole("listbox");
    expect(
      within(listbox).getByRole("option", { name: "Workflow activity" }),
    ).not.toHaveAttribute("data-disabled");
    expect(
      within(listbox).getByRole("option", { name: "Current selection" }),
    ).toHaveAttribute("data-disabled");
  });

  it("supports keyboard selection and keeps the plus action focusable", async () => {
    const user = userEvent.setup();

    renderControlledCard();

    const selector = screen.getByRole("combobox", { name: "Browse widgets" });
    const actionButton = getCardActionButton();

    expect(actionButton.className).toContain("focus-visible:ring-2");

    await selectComboboxOption(user, selector, "Terminal work");

    expect(selector).toHaveTextContent("Terminal work");
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
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("reports the selected widget type back to the dashboard seam", () => {
    const onSelectWidget = vi.fn();

    renderControlledCard(onSelectWidget);

    fireEvent.click(getCardActionButton());

    expect(onSelectWidget).toHaveBeenCalledWith("work-totals");
  });

  it("reports the selected widget type after choosing a different card", async () => {
    const user = userEvent.setup();
    const onSelectWidget = vi.fn();

    renderControlledCard(onSelectWidget);

    await selectComboboxOption(
      user,
      screen.getByRole("combobox", { name: "Browse widgets" }),
      "Terminal work",
    );
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
    const selector = screen.getByRole("combobox", { name: "Browse widgets" });

    expect(selector).toHaveTextContent("No widgets available");
    expect(screen.queryByText(/Remove a singleton widget/)).toBeNull();
    expect(selector).toBeDisabled();
    expect(actionButton).toHaveProperty("disabled", true);
  });
});
