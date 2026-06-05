import { render, screen } from "@testing-library/react";

import { Button } from "./button";

describe("Button", () => {
  it("uses semantic hover tokens for solid tones", () => {
    render(
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

    expect(
      primaryButton.className.includes("hover:bg-on-primary-container"),
    ).toBe(true);
    expect(
      primaryButton.className.includes("hover:border-on-primary-container"),
    ).toBe(true);
    expect(primaryButton.className.includes("brightness-")).toBe(false);
    expect(
      destructiveButton.className.includes("hover:bg-af-danger-hover"),
    ).toBe(true);
    expect(
      destructiveButton.className.includes("hover:border-af-danger-hover"),
    ).toBe(true);
    expect(warningButton.className).toContain("bg-warning-container");
    expect(warningButton.className).toContain("hover:bg-warning-container");
    expect(destructiveButton.className.includes("brightness-")).toBe(false);
  });

  it("can project shared button styling onto child elements when structure requires it", () => {
    render(
      <Button asChild size="sm" tone="outline">
        <span>Projected button label</span>
      </Button>,
    );

    const projectedLabel = screen.getByText("Projected button label");

    expect(projectedLabel.className).toContain("min-h-9");
    expect(projectedLabel.className).toContain("border-outline");
  });

  it("supports compact pill controls for disclosure and filter affordances", () => {
    render(
      <Button size="pill" tone="outline">
        Toggle legend
      </Button>,
    );

    const button = screen.getByRole("button", { name: "Toggle legend" });

    expect(button.className).toContain("rounded-full");
    expect(button.className).toContain("min-h-9");
    expect(button.className).toContain("px-3");
  });
});
