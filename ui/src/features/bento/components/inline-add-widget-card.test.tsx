import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";

import { InlineAddWidgetCard } from "./inline-add-widget-card";

describe("InlineAddWidgetCard", () => {
  it("renders discoverable add-widget copy inside a dashboard grid card", () => {
    render(<InlineAddWidgetCard />);

    const card = screen.getByRole("article", { name: "Add widget" });

    expect(card.dataset.dashboardPanelShell).toBe("grid-card");
    expect(screen.getByText("Add a widget to this dashboard grid.")).toBeTruthy();
    expect(
      screen.getByText(
        "Browse available dashboard widgets without leaving this grid.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Move Add widget" }).getAttribute(
        "data-bento-drag-handle",
      ),
    ).toBe("true");
  });

  it("opens and dismisses the inline widget picker from the card action", () => {
    function TestHarness() {
      const [pickerOpen, setPickerOpen] = useState(false);

      return (
        <InlineAddWidgetCard
          onPickerOpenChange={setPickerOpen}
          pickerOpen={pickerOpen}
        />
      );
    }

    render(<TestHarness />);

    fireEvent.click(
      screen
        .getByText("Add a widget to this dashboard grid.")
        .closest("button") as HTMLButtonElement,
    );

    expect(
      screen.getByRole("dialog", { name: "Add dashboard widget" }),
    ).toBeTruthy();
    expect(screen.getByText("Workflow activity")).toBeTruthy();
    expect(screen.getByText("Provider session")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Close widget picker" }));

    expect(
      screen.queryByRole("dialog", { name: "Add dashboard widget" }),
    ).toBeNull();
  });

  it("localizes the visible add-widget card title", () => {
    render(<InlineAddWidgetCard locale="zh-CN" />);

    expect(screen.getByRole("article", { name: "添加小组件" })).toBeTruthy();
    expect(screen.getByText("将小组件添加到此仪表板网格。")).toBeTruthy();
  });
});
