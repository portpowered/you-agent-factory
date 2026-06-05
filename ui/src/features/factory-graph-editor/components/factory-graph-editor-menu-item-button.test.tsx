import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";

import { FactoryGraphEditorMenuItemButton } from "./factory-graph-editor-menu-item-button";

describe("FactoryGraphEditorMenuItemButton", () => {
  it("projects the graph editor menu item layout onto dashboard action buttons", () => {
    render(
      <FactoryGraphEditorMenuItemButton className="custom-menu-item">
        Add workstation
      </FactoryGraphEditorMenuItemButton>,
    );

    const button = screen.getByRole("button", { name: "Add workstation" });

    expect(button.className).toContain("w-full");
    expect(button.className).toContain("justify-start");
    expect(button.className).toContain("rounded-2xl");
    expect(button.className).toContain("border-transparent");
    expect(button.className).toContain("[&>span]:grid");
    expect(button.className).toContain("custom-menu-item");
  });

  it("keeps menu checkbox semantics available for visibility controls", () => {
    render(
      <FactoryGraphEditorMenuItemButton
        aria-checked={true}
        role="menuitemcheckbox"
      >
        Workflows
      </FactoryGraphEditorMenuItemButton>,
    );

    const button = screen.getByRole("menuitemcheckbox", { name: "Workflows" });

    expect(button).toHaveAttribute("aria-checked", "true");
    expect(button.className).toContain("text-left");
  });
});
