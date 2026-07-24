// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { AlertPanel, AlertPanelText, AlertPanelTitle } from "./alert-panel";
import { ALERT_PANEL_SEMANTIC_CONFIG } from "./alert-panel-semantics";

describe("AlertPanel", () => {
  it.each([
    ["danger", "bg-error-container"],
    ["error", "bg-error-container"],
    ["info", "bg-info-container"],
    ["neutral", "bg-surface-container-low"],
    ["success", "bg-success-container"],
    ["warning", "bg-warning-container"],
  ] as const)("renders the %s tone", (tone, expectedClassName) => {
    renderPackageComponent(<AlertPanel tone={tone}>Message</AlertPanel>);

    expect(screen.getByText("Message").className).toContain(expectedClassName);
  });

  it.each([
    ["neutral", "status", undefined],
    ["info", "status", undefined],
    ["success", "status", undefined],
    ["warning", "alert", undefined],
    ["danger", "alert", undefined],
    ["error", "alert", "Error"],
    ["loading", "status", "Loading"],
    ["empty", "status", "Empty"],
  ] as const)(
    "renders the %s semantic variant with accessible structure",
    (semantic, expectedRole, expectedStatusLabel) => {
      renderPackageComponent(
        <AlertPanel semantic={semantic}>
          <AlertPanelTitle>Feedback title</AlertPanelTitle>
          <AlertPanelText>Feedback message</AlertPanelText>
        </AlertPanel>,
      );

      const panel = screen.getByRole(expectedRole);
      expect(panel).toHaveAttribute("data-af-feedback-variant", semantic);
      expect(screen.getByRole("heading", { level: 3 })).toHaveTextContent(
        "Feedback title",
      );
      expect(screen.getByText("Feedback message")).toBeTruthy();

      if (expectedStatusLabel) {
        expect(screen.getByText(expectedStatusLabel)).toBeTruthy();
      } else {
        expect(screen.queryByText("Error")).toBeNull();
        expect(screen.queryByText("Loading")).toBeNull();
        expect(screen.queryByText("Empty")).toBeNull();
      }
    },
  );

  it("marks loading semantic feedback as busy with default skeleton placeholders", () => {
    renderPackageComponent(<AlertPanel semantic="loading" />);

    const panel = screen.getByRole("status");
    expect(panel).toHaveAttribute("aria-busy", "true");
    expect(panel.querySelectorAll('[aria-hidden="true"]')).toHaveLength(2);
    expect(screen.getByText("Loading")).toBeTruthy();
  });

  it("renders the empty variant for prominent empty/error notices", () => {
    renderPackageComponent(
      <AlertPanel variant="empty">No preview available</AlertPanel>,
    );

    const panel = screen.getByText("No preview available");

    expect(panel.className).toContain("border-dashed");
    expect(panel.className).not.toContain("min-h-60");
  });

  it("supports compact empty notices", () => {
    renderPackageComponent(
      <AlertPanel compact variant="empty">
        Inline warning
      </AlertPanel>,
    );

    expect(screen.getByText("Inline warning").className).toContain("min-h-0");
  });

  it("renders alert copy that inherits the panel tone", () => {
    renderPackageComponent(
      <AlertPanel tone="danger">
        <AlertPanelText>Primary alert copy</AlertPanelText>
        <AlertPanelText as="span" variant="supporting">
          Supporting alert copy
        </AlertPanelText>
      </AlertPanel>,
    );

    const body = screen.getByText("Primary alert copy");
    const supporting = screen.getByText("Supporting alert copy");

    expect(body.className).toContain("text-body-medium");
    expect(body.className).toContain("!text-current");
    expect(supporting.tagName).toBe("SPAN");
    expect(supporting.className).toContain("text-body-small");
    expect(supporting.className).toContain("!text-current");
  });

  it("keeps consumer-provided role overrides for semantic variants", () => {
    renderPackageComponent(
      <AlertPanel role="note" semantic="error">
        <AlertPanelText>Custom role</AlertPanelText>
      </AlertPanel>,
    );

    expect(screen.getByRole("note")).toHaveTextContent("Custom role");
  });

  it("maps every semantic variant to token-backed tone and layout classes", () => {
    for (const [semantic, config] of Object.entries(
      ALERT_PANEL_SEMANTIC_CONFIG,
    )) {
      const { unmount } = renderPackageComponent(
        <AlertPanel
          data-testid={`panel-${semantic}`}
          semantic={semantic as never}
        >
          <AlertPanelText>{semantic}</AlertPanelText>
        </AlertPanel>,
      );

      const panel = screen.getByTestId(`panel-${semantic}`);
      expect(panel.className).toContain(
        config.variant === "empty" ? "border-dashed" : "grid",
      );
      unmount();
    }
  });
});
