import { fireEvent, render, screen } from "@testing-library/react";

import { ExpandablePanelIcon } from "./expandable-panel-icon";
import { ExpandablePanelTrigger } from "./expandable-panel-trigger";

describe("ExpandablePanelIcon", () => {
  it("rotates the chevron when expanded", () => {
    const { container, rerender } = render(
      <ExpandablePanelIcon expanded={false} />,
    );

    const collapsedGlyph = container.querySelector("svg");
    expect(collapsedGlyph?.getAttribute("class")?.includes("rotate-0")).toBe(
      true,
    );
    expect(collapsedGlyph?.getAttribute("class")?.includes("rotate-180")).toBe(
      false,
    );

    rerender(<ExpandablePanelIcon expanded />);

    const expandedGlyph = container.querySelector("svg");
    expect(expandedGlyph?.getAttribute("class")?.includes("rotate-180")).toBe(
      true,
    );
    expect(expandedGlyph?.getAttribute("aria-hidden")).toBe("true");
  });
});

describe("ExpandablePanelTrigger", () => {
  it("exposes disclosure semantics and icon when collapsed", () => {
    render(
      <ExpandablePanelTrigger
        aria-label="Expand section"
        controlsID="panel-content"
        expanded={false}
        type="button"
      />,
    );

    const trigger = screen.getByRole("button", { name: "Expand section" });

    expect(trigger.getAttribute("aria-controls")).toBe("panel-content");
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(trigger.querySelector("svg")).not.toBeNull();
    expect(
      trigger
        .querySelector("svg")
        ?.getAttribute("class")
        ?.includes("rotate-180"),
    ).toBe(false);
  });

  it("exposes disclosure semantics and rotated icon when expanded", () => {
    render(
      <ExpandablePanelTrigger
        aria-label="Collapse section"
        controlsID="panel-content"
        expanded
        type="button"
      />,
    );

    const trigger = screen.getByRole("button", { name: "Collapse section" });

    expect(trigger.getAttribute("aria-controls")).toBe("panel-content");
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(
      trigger
        .querySelector("svg")
        ?.getAttribute("class")
        ?.includes("rotate-180"),
    ).toBe(true);
  });

  it("renders visible label children alongside the icon", () => {
    render(
      <ExpandablePanelTrigger
        controlsID="panel-content"
        expanded={false}
        type="button"
      >
        Expand
      </ExpandablePanelTrigger>,
    );

    expect(
      screen.getByRole("button", { name: "Expand" }).textContent,
    ).toContain("Expand");
  });

  it("invokes onToggle with the next expanded state", () => {
    const onToggle = vi.fn();

    render(
      <ExpandablePanelTrigger
        aria-label="Toggle panel"
        controlsID="panel-content"
        expanded={false}
        onToggle={onToggle}
        type="button"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Toggle panel" }));

    expect(onToggle).toHaveBeenCalledWith(true);
  });

  it("maps section and compact variants to dashboard toggle classes", () => {
    const { rerender } = render(
      <ExpandablePanelTrigger
        aria-label="Section toggle"
        controlsID="panel-content"
        expanded={false}
        variant="section"
      />,
    );

    const sectionTrigger = screen.getByRole("button", {
      name: "Section toggle",
    });
    expect(sectionTrigger.className.includes("px-2.5")).toBe(true);
    expect(sectionTrigger.className.includes("py-2")).toBe(true);

    rerender(
      <ExpandablePanelTrigger
        aria-label="Compact toggle"
        controlsID="panel-content"
        expanded={false}
        variant="compact"
      />,
    );

    const compactTrigger = screen.getByRole("button", {
      name: "Compact toggle",
    });
    expect(compactTrigger.className.includes("min-h-9")).toBe(true);
    expect(compactTrigger.className.includes("py-1.5")).toBe(true);
  });
});
