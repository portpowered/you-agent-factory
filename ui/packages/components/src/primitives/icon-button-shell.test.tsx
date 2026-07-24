// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { IconButtonShell } from "./icon-button-shell";

describe("IconButtonShell", () => {
  it("requires an accessible name via aria-label for icon-only actions", () => {
    renderPackageComponent(
      <IconButtonShell aria-label="Remove item">
        <span aria-hidden="true">x</span>
      </IconButtonShell>,
    );

    expect(
      screen.getByRole("button", { name: "Remove item" }),
    ).toBeInTheDocument();
  });

  it("provides a stable dashboard toolbar touch target", () => {
    renderPackageComponent(
      <IconButtonShell aria-label="Refresh jobs">
        <svg aria-hidden="true" viewBox="0 0 16 16" />
      </IconButtonShell>,
    );

    const button = screen.getByRole("button", { name: "Refresh jobs" });

    expect(button.className).toContain("h-10");
    expect(button.className).toContain("w-10");
    expect(button.className).toContain("rounded-lg");
  });

  it("supports destructive icon actions with dangerGhost tone", () => {
    renderPackageComponent(
      <IconButtonShell aria-label="Delete item" tone="dangerGhost">
        <span aria-hidden="true">x</span>
      </IconButtonShell>,
    );

    const button = screen.getByRole("button", { name: "Delete item" });

    expect(button.className).toContain("hover:border-af-danger-border");
    expect(button.className).toContain("hover:bg-error-container");
    expect(button.className).toContain("hover:text-on-error-container");
  });

  it("preserves focus-visible treatment for keyboard users", () => {
    renderPackageComponent(
      <IconButtonShell aria-label="Open settings">
        <span aria-hidden="true">⚙</span>
      </IconButtonShell>,
    );

    const button = screen.getByRole("button", { name: "Open settings" });

    expect(button.className).toContain("focus-visible:ring-2");
    expect(button.className).toContain("focus-visible:ring-af-focus-ring");
  });

  it("exposes loading busy state and prevents duplicate activation", () => {
    renderPackageComponent(
      <IconButtonShell aria-label="Export dashboard" loading>
        <svg aria-hidden="true" viewBox="0 0 16 16" />
      </IconButtonShell>,
    );

    const button = screen.getByRole("button", { name: "Export dashboard" });

    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toBeDisabled();
    expect(button.querySelector("svg.animate-spin")).toBeTruthy();
  });
});
