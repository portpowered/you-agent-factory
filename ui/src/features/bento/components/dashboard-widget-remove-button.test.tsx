import { fireEvent, render, screen } from "@testing-library/react";

import { DashboardWidgetRemoveButton } from "./dashboard-widget-remove-button";

describe("DashboardWidgetRemoveButton", () => {
  it("renders a shared dashboard control with a dashboard-specific label", () => {
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

    expect(button.className).toContain("h-10");
    expect(button.className).toContain("w-10");
    expect(button.className).toContain("focus-visible:ring-2");
    expect(button.className).toContain("border-af-border");
    expect(button.className).toContain("bg-af-surface-raised");

    fireEvent.click(button);

    expect(onClick).toHaveBeenCalledTimes(1);
  });
});
