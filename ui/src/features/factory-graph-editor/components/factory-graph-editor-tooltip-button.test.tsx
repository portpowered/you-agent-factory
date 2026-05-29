import { JSDOM } from "jsdom";
import { render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { FactoryGraphEditorTooltipActionButton } from "./factory-graph-editor-tooltip-button";

if (typeof document === "undefined") {
  const { window } = new JSDOM("<!doctype html><html><body></body></html>", {
    url: "http://localhost/",
  });

  Object.assign(globalThis, {
    document: window.document,
    HTMLElement: window.HTMLElement,
    Node: window.Node,
    navigator: window.navigator,
    window,
  });
}

describe("FactoryGraphEditorTooltipActionButton", () => {
  it("shows and hides the tooltip on hover without changing the copy or semantics", async () => {
    const user = userEvent.setup({ document: globalThis.document });

    const view = render(
      <FactoryGraphEditorTooltipActionButton
        aria-label="Delete"
        tooltip="Remove nodes or edges from the draft"
      />,
    );

    const button = view.getByRole("button", { name: "Delete" });
    expect(button.getAttribute("aria-describedby")).toBeNull();
    expect(view.queryByRole("tooltip")).toBeNull();

    await user.hover(button);

    const tooltip = await view.findByRole("tooltip", {
      name: "Remove nodes or edges from the draft",
    });
    expect(button.getAttribute("aria-describedby")).toBe(tooltip.id);
    expect(tooltip.className).toContain("pointer-events-none");
    expect(tooltip.className).toContain("border-af-border-strong");
    expect(tooltip.className).toContain("bg-af-surface-raised");
    expect(tooltip.className).toContain("text-af-text");

    await user.unhover(button);
    expect(button.getAttribute("aria-describedby")).toBeNull();
    expect(view.queryByRole("tooltip")).toBeNull();
  });

  it("shows and hides the tooltip on keyboard focus", async () => {
    const user = userEvent.setup({ document: globalThis.document });

    const view = render(
      <FactoryGraphEditorTooltipActionButton
        aria-label="Connect"
        tooltip="Connect selected graph nodes"
      />,
    );

    await user.tab();

    const button = view.getByRole("button", { name: "Connect" });
    const tooltip = await view.findByRole("tooltip", {
      name: "Connect selected graph nodes",
    });
    expect(button.getAttribute("aria-describedby")).toBe(tooltip.id);

    await user.tab();
    expect(button.getAttribute("aria-describedby")).toBeNull();
    expect(view.queryByRole("tooltip")).toBeNull();
  });
});
