import { render, screen } from "@testing-library/react";

import { AlertPanel } from "./alert-panel";

describe("AlertPanel", () => {
  it.each([
    ["danger", "bg-error-container"],
    ["info", "bg-info-container"],
    ["neutral", "bg-surface-container-low"],
    ["success", "bg-success-container"],
    ["warning", "bg-warning-container"],
  ] as const)("renders the %s tone", (tone, expectedClassName) => {
    render(<AlertPanel tone={tone}>Message</AlertPanel>);

    expect(screen.getByText("Message").className).toContain(expectedClassName);
  });

  it("renders the empty variant for prominent empty/error notices", () => {
    render(<AlertPanel variant="empty">No preview available</AlertPanel>);

    const panel = screen.getByText("No preview available");

    expect(panel.className).toContain("min-h-60");
    expect(panel.className).toContain("border-dashed");
  });

  it("supports compact empty notices", () => {
    render(
      <AlertPanel compact variant="empty">
        Inline warning
      </AlertPanel>,
    );

    expect(screen.getByText("Inline warning").className).toContain("min-h-0");
  });
});
