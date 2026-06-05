import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { CurrentSelectionContentSection } from "./current-selection-content-section";

describe("CurrentSelectionContentSection", () => {
  it("renders an unbordered current-selection subsection with a dashboard h4", () => {
    render(
      <CurrentSelectionContentSection
        headingId="current-work-heading"
        title="Current work"
      >
        <p>Work list</p>
      </CurrentSelectionContentSection>,
    );

    const section = screen.getByRole("region", { name: "Current work" });
    const heading = screen.getByRole("heading", {
      level: 4,
      name: "Current work",
    });

    expect(section.getAttribute("aria-labelledby")).toBe(
      "current-work-heading",
    );
    expect(section.className).toContain("mt-4");
    expect(section.className).toContain("gap-2.5");
    expect(section.className).not.toContain("border-t");
    expect(heading.className).toContain("af-dashboard-section-heading");
  });

  it("supports an explicit region label when heading copy should not name the section", () => {
    render(
      <CurrentSelectionContentSection
        ariaLabel="Inference attempts"
        title="Attempts"
      >
        <p>No attempts</p>
      </CurrentSelectionContentSection>,
    );

    const section = screen.getByRole("region", { name: "Inference attempts" });

    expect(section.getAttribute("aria-label")).toBe("Inference attempts");
    expect(section.getAttribute("aria-labelledby")).toBeNull();
  });
});
