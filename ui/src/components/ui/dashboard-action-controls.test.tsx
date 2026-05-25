import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";

import { DashboardActionButton } from "./dashboard-action-button";
import { DashboardStatusPill } from "./dashboard-status-pill";

describe("DashboardActionButton", () => {
  it("supports icon-only, text-only, and icon-plus-text usage", () => {
    render(
      <>
        <DashboardActionButton aria-label="Export" iconOnly>
          <svg aria-hidden="true" viewBox="0 0 16 16" />
        </DashboardActionButton>
        <DashboardActionButton>Save</DashboardActionButton>
        <DashboardActionButton>
          <svg aria-hidden="true" viewBox="0 0 16 16" />
          <span>Discard</span>
        </DashboardActionButton>
      </>,
    );

    expect(screen.getByRole("button", { name: "Export" }).textContent).toBe("");
    expect(screen.getByRole("button", { name: "Save" }).textContent).toBe("Save");
    expect(screen.getByRole("button", { name: "Discard" }).textContent).toBe(
      "Discard",
    );
  });

  it("preserves compact footprint and accessible busy state while executing", () => {
    render(
      <DashboardActionButton executing type="button">
        Save changes
      </DashboardActionButton>,
    );

    const button = screen.getByRole("button", { name: "Save changes" });
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toBeDisabled();
    expect(button.className).toContain("min-h-10");
    expect(button.querySelector(".animate-spin")).toBeTruthy();
  });

  it("keeps disabled and pressed semantics available for migrated surfaces", () => {
    render(
      <>
        <DashboardActionButton aria-pressed={true} tone="secondary" type="button">
          Active
        </DashboardActionButton>
        <DashboardActionButton disabled type="button">
          Disabled
        </DashboardActionButton>
      </>,
    );

    expect(screen.getByRole("button", { name: "Active" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "Disabled" })).toBeDisabled();
  });
});

describe("DashboardStatusPill", () => {
  it("renders a non-interactive status label with semantic tones", () => {
    render(
      <>
        <DashboardStatusPill tone="neutral">Observe mode</DashboardStatusPill>
        <DashboardStatusPill role="status" tone="warning">
          Draft changes pending
        </DashboardStatusPill>
      </>,
    );

    expect(screen.getByText("Observe mode").tagName).toBe("SPAN");
    expect(screen.queryByRole("button", { name: "Observe mode" })).toBeNull();
    expect(screen.getByText("Draft changes pending").className).toContain(
      "border-af-warning-border",
    );
    expect(screen.getByRole("status").textContent).toBe("Draft changes pending");
  });
});
