import { fireEvent, render, screen } from "@testing-library/react";

import { DashboardWidgetRemoveButton } from "./dashboard-widget-remove-button";

describe("DashboardWidgetRemoveButton", () => {
  it("renders a compact close-style remove control with a dashboard-specific label", () => {
    const onClick = vi.fn();
    render(
      <DashboardWidgetRemoveButton
        onClick={onClick}
        widgetTitle="Work totals"
      />,
    );

    const button = screen.getByRole("button", {
      name: "Remove Work totals widget from dashboard",
    });

    expect(button.className).toContain("size-8");
    expect(button.className).toContain("focus-visible:ring-2");

    fireEvent.click(button);

    expect(onClick).toHaveBeenCalledTimes(1);
  });
});
