// @vitest-environment happy-dom

import { describe, expect, it, vi } from "vitest";

import { renderPackageComponent, screen, userEvent } from "../testing/render";
import { Button } from "./button";

describe("Button semantic variants", () => {
  it("uses semantic hover tokens for solid tones", () => {
    renderPackageComponent(
      <>
        <Button>Primary action</Button>
        <Button tone="destructive">Delete</Button>
        <Button tone="warning">Save changes</Button>
      </>,
    );

    const primaryButton = screen.getByRole("button", {
      name: "Primary action",
    });
    const destructiveButton = screen.getByRole("button", { name: "Delete" });
    const warningButton = screen.getByRole("button", {
      name: "Save changes",
    });

    expect(primaryButton.className).toContain("border-primary");
    expect(primaryButton.className).toContain("bg-primary");
    expect(primaryButton.className).toContain("hover:bg-on-primary-container");
    expect(primaryButton.className).toContain(
      "hover:border-on-primary-container",
    );
    expect(primaryButton.className).not.toContain("af-success");
    expect(primaryButton.className.includes("brightness-")).toBe(false);
    expect(destructiveButton.className).toContain("hover:bg-af-danger-hover");
    expect(destructiveButton.className).toContain(
      "hover:border-af-danger-hover",
    );
    expect(warningButton.className).toContain("bg-warning-container");
    expect(warningButton.className).toContain("border-af-warning-border");
    expect(warningButton.className).toContain("hover:bg-warning-container");
    expect(destructiveButton.className.includes("brightness-")).toBe(false);
  });

  it("renders secondary, outline, and ghost tones with semantic role classes", () => {
    renderPackageComponent(
      <>
        <Button tone="secondary">Secondary action</Button>
        <Button tone="outline">Outline action</Button>
        <Button tone="ghost">Ghost action</Button>
      </>,
    );

    const secondaryButton = screen.getByRole("button", {
      name: "Secondary action",
    });
    const outlineButton = screen.getByRole("button", {
      name: "Outline action",
    });
    const ghostButton = screen.getByRole("button", { name: "Ghost action" });

    expect(secondaryButton.className).toContain("bg-surface-container-low");
    expect(secondaryButton.className).toContain("text-primary");
    expect(outlineButton.className).toContain("border-outline");
    expect(outlineButton.className).toContain("bg-surface-container-high");
    expect(outlineButton.className).toContain("text-on-surface");
    expect(ghostButton.className).toContain("border-transparent");
    expect(ghostButton.className).toContain("bg-transparent");
    expect(ghostButton.className).toContain("text-on-surface-variant");
  });

  it("keeps destructive and warning variants visually distinguishable", () => {
    renderPackageComponent(
      <>
        <Button tone="destructive">Delete factory</Button>
        <Button tone="warning">Review warning</Button>
      </>,
    );

    const destructiveButton = screen.getByRole("button", {
      name: "Delete factory",
    });
    const warningButton = screen.getByRole("button", {
      name: "Review warning",
    });

    expect(destructiveButton.className).toContain("bg-error");
    expect(destructiveButton.className).toContain("text-on-error");
    expect(warningButton.className).toContain("bg-warning-container");
    expect(warningButton.className).toContain("text-on-warning-container");
    expect(destructiveButton.className).not.toContain("bg-warning-container");
    expect(warningButton.className).not.toContain("bg-error");
  });
});

describe("Button disabled and focus behavior", () => {
  it("applies disabled styling and prevents interaction semantics", () => {
    renderPackageComponent(
      <Button disabled tone="default">
        Disabled action
      </Button>,
    );

    const button = screen.getByRole("button", { name: "Disabled action" });

    expect(button).toBeDisabled();
    expect(button.className).toContain("disabled:pointer-events-none");
    expect(button.className).toContain("disabled:bg-surface-container-low");
    expect(button.className).toContain("disabled:text-on-surface-disabled");
  });

  it("exposes focus-visible ring treatment for keyboard users", () => {
    renderPackageComponent(<Button>Focusable action</Button>);

    const button = screen.getByRole("button", { name: "Focusable action" });

    expect(button.className).toContain("focus-visible:ring-2");
    expect(button.className).toContain("focus-visible:ring-af-focus-ring");
  });
});

describe("Button loading behavior", () => {
  it("exposes busy state, disables interaction, and preserves the accessible name while loading", () => {
    renderPackageComponent(
      <Button loading type="button">
        Save changes
      </Button>,
    );

    const button = screen.getByRole("button", { name: "Save changes" });

    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toBeDisabled();
    const spinner = button.querySelector("svg.animate-spin");
    expect(spinner).toBeTruthy();
    expect(
      spinner?.querySelector("circle.text-on-surface-disabled"),
    ).toBeTruthy();
    expect(
      button.querySelector("span.pointer-events-none.absolute.inset-0"),
    ).toBeTruthy();
  });

  it("supports icon-only loading buttons with an accessible label", () => {
    renderPackageComponent(
      <Button
        aria-label="Sync graph"
        loading
        size="icon"
        tone="outline"
        type="button"
      >
        <svg aria-hidden="true" viewBox="0 0 16 16" />
      </Button>,
    );

    const button = screen.getByRole("button", { name: "Sync graph" });

    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toBeDisabled();
    expect(button.className).toContain("h-11");
    expect(button.className).toContain("w-11");
    expect(button.querySelector("svg.animate-spin")).toBeTruthy();
  });

  it("prevents duplicate activation when loading with asChild projection", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();

    renderPackageComponent(
      <Button asChild loading>
        <button onClick={onClick} type="button">
          Save changes
        </button>
      </Button>,
    );

    const button = screen.getByRole("button", { name: "Save changes" });

    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toHaveAttribute("aria-disabled", "true");
    expect(button.className).toContain("pointer-events-none");

    await user.click(button);

    expect(onClick).not.toHaveBeenCalled();
  });
});

describe("Button asChild projection", () => {
  it("can project shared button styling onto child elements when structure requires it", () => {
    renderPackageComponent(
      <Button asChild size="sm" tone="outline">
        <span>Projected button label</span>
      </Button>,
    );

    const projectedLabel = screen.getByText("Projected button label");

    expect(projectedLabel.className).toContain("min-h-9");
    expect(projectedLabel.className).toContain("border-outline");
  });
});
