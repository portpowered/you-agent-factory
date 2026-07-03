import { render, screen } from "@testing-library/react";

import { AlertPanel, AlertPanelText } from "./alert-panel";

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

    expect(panel.className).toContain("border-dashed");
    expect(panel.className).not.toContain("min-h-60");
  });

  it("supports compact empty notices", () => {
    render(
      <AlertPanel compact variant="empty">
        Inline warning
      </AlertPanel>,
    );

    expect(screen.getByText("Inline warning").className).toContain("min-h-0");
  });

  it("renders alert copy that inherits the panel tone", () => {
    render(
      <AlertPanel tone="danger">
        <AlertPanelText>Primary alert copy</AlertPanelText>
        <AlertPanelText as="span" variant="supporting">
          Supporting alert copy
        </AlertPanelText>
      </AlertPanel>,
    );

    const body = screen.getByText("Primary alert copy");
    const supporting = screen.getByText("Supporting alert copy");

    expect(body.className).toContain("af-body-text");
    expect(body.className).toContain("!text-current");
    expect(supporting.tagName).toBe("SPAN");
    expect(supporting.className).toContain("af-supporting-text");
    expect(supporting.className).toContain("!text-current");
  });
});
