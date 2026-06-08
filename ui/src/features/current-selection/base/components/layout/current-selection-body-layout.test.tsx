import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CurrentSelectionExpandableSection } from "../detail/current-selection-expandable-section";
import { CurrentSelectionBodyLayout } from "./current-selection-body-layout";

describe("CurrentSelectionBodyLayout", () => {
  it("renders the required primary title with workstation typography", () => {
    render(
      <CurrentSelectionBodyLayout title="Assembly workstation">
        <CurrentSelectionExpandableSection
          title="Summary"
          toggleLabel={(expanded) =>
            expanded ? "Collapse Summary" : "Expand Summary"
          }
        >
          <p>Summary details</p>
        </CurrentSelectionExpandableSection>
      </CurrentSelectionBodyLayout>,
    );

    expect(screen.getByText("Assembly workstation").className).toContain(
      "type-display-large",
    );
  });

  it("preserves expandable section heading and disclosure semantics", async () => {
    render(
      <CurrentSelectionBodyLayout title="Assembly workstation">
        <CurrentSelectionExpandableSection
          contentId="summary-content"
          defaultExpanded={false}
          headingId="summary-heading"
          title="Summary"
          toggleLabel={(expanded) =>
            expanded ? "Collapse Summary" : "Expand Summary"
          }
        >
          <p>Summary details</p>
        </CurrentSelectionExpandableSection>
        <CurrentSelectionExpandableSection
          contentId="history-content"
          defaultExpanded={false}
          headingId="history-heading"
          title="History"
          toggleLabel={(expanded) =>
            expanded ? "Collapse History" : "Expand History"
          }
        >
          <p>History details</p>
        </CurrentSelectionExpandableSection>
      </CurrentSelectionBodyLayout>,
    );

    const summarySection = screen
      .getByRole("heading", { level: 4, name: "Summary" })
      .closest("section");
    expect(summarySection).toBeTruthy();
    expect(
      screen
        .getByRole("heading", { level: 4, name: "History" })
        .closest("section"),
    ).toBeTruthy();

    const disclosure = within(summarySection as HTMLElement).getByRole(
      "button",
      {
        name: "Expand Summary",
      },
    );
    expect(disclosure.getAttribute("aria-controls")).toBe("summary-content");
    expect(disclosure.getAttribute("aria-expanded")).toBe("false");
    expect(
      screen
        .getByRole("button", { name: "Expand History" })
        .getAttribute("aria-controls"),
    ).toBe("history-content");
    expect(screen.queryByText("Summary details")).toBeNull();

    await userEvent.click(disclosure);

    expect(
      within(summarySection as HTMLElement)
        .getByRole("button", {
          name: "Collapse Summary",
        })
        .getAttribute("aria-expanded"),
    ).toBe("true");
    const summaryDetails = screen.getByText("Summary details");
    expect(summaryDetails).toBeTruthy();
    const summaryContent = document.getElementById("summary-content");
    expect(summaryContent?.className).toBe("grid");
  });
});
