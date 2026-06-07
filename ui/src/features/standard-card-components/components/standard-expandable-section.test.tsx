import { render, screen, within } from "@testing-library/react";

import { StandardExpandableSection } from "./standard-expandable-section";

describe("StandardExpandableSection", () => {
  it("pins the section header and trigger to the top edge when expanded by default", () => {
    render(
      <StandardExpandableSection
        defaultExpanded
        heading="Completed work"
        supportingText="3 items"
        toggleLabel={({ expanded }) =>
          expanded ? "Collapse Completed work" : "Expand Completed work"
        }
      >
        <p>Expanded content</p>
      </StandardExpandableSection>,
    );

    const section = screen
      .getByRole("heading", { level: 5, name: "Completed work" })
      .closest("section");
    const trigger = screen.getByRole("button", {
      name: "Collapse Completed work",
    });
    const headerRow = trigger.parentElement;

    expect(section?.className).toContain("self-start");
    expect(section?.className).toContain("w-full");
    expect(headerRow?.className).toContain("grid-cols-[minmax(0,1fr)_auto]");
    expect(headerRow?.className).toContain("items-start");
    expect(trigger.className).toContain("self-start");
    expect(trigger.className).not.toContain("mt-0.5");
    expect(within(section as HTMLElement).getByText("Expanded content")).toBeTruthy();
  });
});
