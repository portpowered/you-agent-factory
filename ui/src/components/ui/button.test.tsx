import { render, screen } from "@testing-library/react";

import { Button } from "./button";

describe("Button", () => {
  it("uses semantic hover tokens for solid tones", () => {
    render(
      <>
        <Button>Primary action</Button>
        <Button tone="destructive">Delete</Button>
      </>,
    );

    const primaryButton = screen.getByRole("button", { name: "Primary action" });
    const destructiveButton = screen.getByRole("button", { name: "Delete" });

    expect(primaryButton.className.includes("hover:bg-af-accent-hover")).toBe(true);
    expect(primaryButton.className.includes("hover:border-af-accent-hover")).toBe(true);
    expect(primaryButton.className.includes("brightness-")).toBe(false);
    expect(destructiveButton.className.includes("hover:bg-af-danger-hover")).toBe(true);
    expect(destructiveButton.className.includes("hover:border-af-danger-hover")).toBe(true);
    expect(destructiveButton.className.includes("brightness-")).toBe(false);
  });
});
