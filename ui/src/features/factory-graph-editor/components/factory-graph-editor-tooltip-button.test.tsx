import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { FactoryGraphEditorTooltipActionButton } from "./factory-graph-editor-tooltip-button";

describe("FactoryGraphEditorTooltipActionButton", () => {
  it("shows and hides the tooltip on hover without changing the copy or semantics", async () => {
    const user = userEvent.setup();

    render(
      <FactoryGraphEditorTooltipActionButton
        aria-label="Delete"
        tooltip="Remove nodes or edges from the draft"
      />,
    );

    const button = screen.getByRole("button", { name: "Delete" });
    expect(button.getAttribute("aria-describedby")).toBeNull();
    expect(screen.queryByRole("tooltip")).toBeNull();

    await user.hover(button);

    const tooltip = await screen.findByRole("tooltip", {
      name: "Remove nodes or edges from the draft",
    });
    expect(button.getAttribute("aria-describedby")).toBe(tooltip.id);
    expect(tooltip.className).toContain("pointer-events-none");
    expect(tooltip.className).toContain("border-af-border-strong");
    expect(tooltip.className).toContain("bg-af-surface-raised");
    expect(tooltip.className).toContain("text-af-text");

    await user.unhover(button);
    expect(button.getAttribute("aria-describedby")).toBeNull();
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("shows and hides the tooltip on keyboard focus", async () => {
    const user = userEvent.setup();

    render(
      <FactoryGraphEditorTooltipActionButton
        aria-label="Connect"
        tooltip="Connect selected graph nodes"
      />,
    );

    await user.tab();

    const button = screen.getByRole("button", { name: "Connect" });
    const tooltip = await screen.findByRole("tooltip", {
      name: "Connect selected graph nodes",
    });
    expect(button.getAttribute("aria-describedby")).toBe(tooltip.id);

    await user.tab();
    expect(button.getAttribute("aria-describedby")).toBeNull();
    expect(screen.queryByRole("tooltip")).toBeNull();
  });
});
