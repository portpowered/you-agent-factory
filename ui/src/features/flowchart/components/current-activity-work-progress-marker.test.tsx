import { render, screen } from "@testing-library/react";

import { CurrentActivityWorkProgressMarker } from "./current-activity-work-progress-marker";

describe("CurrentActivityWorkProgressMarker", () => {
  it("renders a numeric success marker with shared graph progress styling", () => {
    render(
      <CurrentActivityWorkProgressMarker
        ariaLabel="11 active items"
        className="min-h-5"
        count={11}
        data-state-work-progress="numeric"
        kind="numeric"
      />,
    );

    const marker = screen.getByRole("status", { name: "11 active items" });

    expect(marker.textContent).toBe("11");
    expect(marker.className).toContain("bg-success-container");
    expect(marker.className).toContain("border-af-success-border");
    expect(marker.getAttribute("data-state-work-progress")).toBe("numeric");
  });

  it("renders indexed success dots and an optional overflow suffix", () => {
    const { container } = render(
      <CurrentActivityWorkProgressMarker
        ariaLabel="6 active items"
        data-workstation-work-progress="dots"
        dotCount={2}
        dotDataAttribute="data-workstation-work-progress-dot"
        kind="dots"
        suffix={<span>+2</span>}
      />,
    );

    const marker = screen.getByRole("status", { name: "6 active items" });
    const dots = container.querySelectorAll(
      "[data-workstation-work-progress-dot]",
    );

    expect(marker.getAttribute("data-workstation-work-progress")).toBe("dots");
    expect(Array.from(dots).map((dot) => dot.textContent)).toEqual(["", ""]);
    expect(
      Array.from(dots).map((dot) =>
        dot.getAttribute("data-workstation-work-progress-dot"),
      ),
    ).toEqual(["0", "1"]);
    expect(marker.textContent).toBe("+2");
  });
});
