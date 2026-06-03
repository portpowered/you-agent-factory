import { render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { JSDOM } from "jsdom";

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
    expect(tooltip.className).toContain("border-outline-variant");
    expect(tooltip.className).toContain("bg-surface-container-high");
    expect(tooltip.className).toContain("text-on-surface");
    expect(tooltip.className).toContain("top-full");
    expect(tooltip.className).toContain("mt-2");

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

  it("positions the tooltip above the trigger when placement is above", async () => {
    const user = userEvent.setup({ document: globalThis.document });

    const view = render(
      <FactoryGraphEditorTooltipActionButton
        aria-label="Delete"
        placement="above"
        tooltip="Remove nodes or edges from the draft"
      />,
    );

    await user.hover(view.getByRole("button", { name: "Delete" }));

    const tooltip = await view.findByRole("tooltip", {
      name: "Remove nodes or edges from the draft",
    });
    expect(tooltip.className).toContain("bottom-full");
    expect(tooltip.className).toContain("mb-2");
    expect(tooltip.className).not.toContain("top-full");
  });
});
