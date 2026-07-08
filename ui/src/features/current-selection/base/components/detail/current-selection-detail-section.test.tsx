import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { CurrentSelectionDetailSection } from "./current-selection-detail-section";

describe("CurrentSelectionDetailSection", () => {
  it("renders a dashboard-styled section heading when title is provided", () => {
    render(
      <CurrentSelectionDetailSection
        headingId="request-details-heading"
        title="Request details"
      >
        <p>Request body</p>
      </CurrentSelectionDetailSection>,
    );

    const section = screen.getByRole("region", { name: "Request details" });
    const heading = screen.getByRole("heading", {
      level: 4,
      name: "Request details",
    });

    expect(section.getAttribute("aria-labelledby")).toBe(
      "request-details-heading",
    );
    expect(heading.className).toContain("af-section-heading");
  });

  it("keeps aria-label support for untitled detail sections", () => {
    render(
      <CurrentSelectionDetailSection ariaLabel="Untitled detail">
        <p>Body</p>
      </CurrentSelectionDetailSection>,
    );

    expect(
      screen.getByRole("region", { name: "Untitled detail" }),
    ).toBeTruthy();
  });
});
