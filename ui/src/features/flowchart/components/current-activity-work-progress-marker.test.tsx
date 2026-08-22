import { render, screen } from "@testing-library/react";

import { FactoryGraphWorkProgressMarker } from "@you-agent-factory/factory-graph";

describe("FactoryGraphWorkProgressMarker", () => {
  it("renders an unboxed numeric marker with surface text styling", () => {
    render(
      <FactoryGraphWorkProgressMarker
        ariaLabel="11 active items"
        count={11}
        data-state-work-progress="numeric"
        kind="numeric"
      />,
    );

    const marker = screen.getByRole("status", { name: "11 active items" });

    expect(marker.textContent).toBe("11");
    expect(marker.className).toContain("min-h-6");
    expect(marker.className).toContain("text-base");
    expect(marker.className).toContain("text-on-surface");
    expect(marker.className).not.toContain("bg-success-container");
    expect(marker.className).not.toContain("border-af-success-border");
    expect(marker.getAttribute("data-state-work-progress")).toBe("numeric");
  });

  it("renders active dots with on-surface fill and explicit state semantics", () => {
    const { container } = render(
      <FactoryGraphWorkProgressMarker
        active
        ariaLabel="2 active items"
        data-workstation-work-progress="dots"
        dotCount={2}
        dotDataAttribute="data-workstation-work-progress-dot"
        kind="dots"
      />,
    );

    const marker = screen.getByRole("status", { name: "2 active items" });
    const dots = container.querySelectorAll(
      "[data-workstation-work-progress-dot]",
    );

    expect(marker.getAttribute("data-workstation-work-progress")).toBe("dots");
    expect(marker.getAttribute("data-work-progress-state")).toBe("active");
    expect(Array.from(dots).map((dot) => dot.textContent)).toEqual(["", ""]);
    expect(
      Array.from(dots).map((dot) =>
        dot.getAttribute("data-workstation-work-progress-dot"),
      ),
    ).toEqual(["0", "1"]);
    for (const dot of dots) {
      expect(dot.className).toContain("bg-on-surface");
      expect(dot.getAttribute("data-work-progress-dot-state")).toBe("active");
    }
  });

  it("renders idle dots with surface fill and a semantic border", () => {
    const { container } = render(
      <FactoryGraphWorkProgressMarker
        active={false}
        ariaLabel="2 queued items"
        dotCount={2}
        dotDataAttribute="data-state-work-progress-dot"
        kind="dots"
      />,
    );

    const marker = screen.getByRole("status", { name: "2 queued items" });
    const dots = container.querySelectorAll("[data-state-work-progress-dot]");

    expect(marker.getAttribute("data-work-progress-state")).toBe("idle");
    for (const dot of dots) {
      expect(dot.className).toContain("bg-surface");
      expect(dot.className).toContain("border-outline-variant");
      expect(dot.className).not.toContain("bg-on-surface");
      expect(dot.getAttribute("data-work-progress-dot-state")).toBe("idle");
    }
  });
});
