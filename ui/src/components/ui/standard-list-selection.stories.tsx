import { expect, within } from "storybook/test";

import {
  STANDARD_LIST_SELECTION_ROW_DANGER_CLASS,
  STANDARD_LIST_SELECTION_ROW_INFO_CLASS,
  STANDARD_LIST_SELECTION_ROW_NEUTRAL_CLASS,
  STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS,
  StandardListSelection,
  StandardListSelectionItem,
} from "./standard-list-selection";

const ACCENT_SELECTED_TOKENS = [
  "bg-af-accent",
  "bg-af-accent-surface",
  "border-af-accent",
  "shadow-af-accent-selected",
] as const;

function expectNoAccentSelectedTreatment(className: string) {
  for (const token of ACCENT_SELECTED_TOKENS) {
    expect(className).not.toContain(token);
  }
}

function StandardListSelectionShowcase() {
  return (
    <div className="grid max-w-md gap-6 p-6">
      <section
        aria-labelledby="standard-list-selection-states"
        className="grid gap-3"
      >
        <h2
          className="text-sm font-semibold text-af-text"
          id="standard-list-selection-states"
        >
          Standard list selection states
        </h2>
        <StandardListSelection selectionAnnouncement="Alpha target selected">
          <StandardListSelectionItem>
            Neutral unselected
          </StandardListSelectionItem>
          <StandardListSelectionItem selected>
            Neutral selected
          </StandardListSelectionItem>
          <StandardListSelectionItem disabled>
            Disabled row
          </StandardListSelectionItem>
        </StandardListSelection>
      </section>

      <section
        aria-labelledby="standard-list-selection-tones"
        className="grid gap-3"
      >
        <h2
          className="text-sm font-semibold text-af-text"
          id="standard-list-selection-tones"
        >
          Outcome tone variants
        </h2>
        <StandardListSelection>
          <StandardListSelectionItem tone="info">
            Info unselected
          </StandardListSelectionItem>
          <StandardListSelectionItem selected tone="info">
            Info selected
          </StandardListSelectionItem>
          <StandardListSelectionItem tone="danger">
            Danger unselected
          </StandardListSelectionItem>
          <StandardListSelectionItem selected tone="danger">
            Danger selected
          </StandardListSelectionItem>
        </StandardListSelection>
      </section>

      <section
        aria-labelledby="standard-list-selection-pending"
        className="grid gap-3"
      >
        <h2
          className="text-sm font-semibold text-af-text"
          id="standard-list-selection-pending"
        >
          Pending list
        </h2>
        <StandardListSelection disabled>
          <StandardListSelectionItem selected>
            Pending selected row
          </StandardListSelectionItem>
        </StandardListSelection>
      </section>
    </div>
  );
}

export default {
  title: "Agent Factory/UI/Standard List Selection",
  tags: ["test"],
};

export const SharedStandardListSelection = {
  render: () => <StandardListSelectionShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    const neutralUnselected = canvas.getByRole("button", {
      name: "Neutral unselected",
    });
    expect(neutralUnselected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_NEUTRAL_CLASS,
    );
    await expect(neutralUnselected).toHaveAttribute("aria-pressed", "false");
    await expect(neutralUnselected).toHaveAttribute("data-selected", "false");
    expectNoAccentSelectedTreatment(neutralUnselected.className);

    const neutralSelected = canvas.getByRole("button", {
      name: "Neutral selected",
    });
    expect(neutralSelected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS,
    );
    await expect(neutralSelected).toHaveAttribute("aria-pressed", "true");
    await expect(neutralSelected).toHaveAttribute("data-selected", "true");
    expectNoAccentSelectedTreatment(neutralSelected.className);

    await expect(
      canvas.getByRole("button", { name: "Disabled row" }),
    ).toBeDisabled();

    const infoUnselected = canvas.getByRole("button", {
      name: "Info unselected",
    });
    expect(infoUnselected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_INFO_CLASS,
    );
    expectNoAccentSelectedTreatment(infoUnselected.className);

    const infoSelected = canvas.getByRole("button", { name: "Info selected" });
    expect(infoSelected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS,
    );
    expect(infoSelected.className).not.toContain(
      STANDARD_LIST_SELECTION_ROW_INFO_CLASS,
    );
    await expect(infoSelected).toHaveAttribute("aria-pressed", "true");
    expectNoAccentSelectedTreatment(infoSelected.className);

    const dangerUnselected = canvas.getByRole("button", {
      name: "Danger unselected",
    });
    expect(dangerUnselected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_DANGER_CLASS,
    );
    expectNoAccentSelectedTreatment(dangerUnselected.className);

    const dangerSelected = canvas.getByRole("button", {
      name: "Danger selected",
    });
    expect(dangerSelected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS,
    );
    expectNoAccentSelectedTreatment(dangerSelected.className);

    const pendingList = canvas.getByRole("button", {
      name: "Pending selected row",
    });
    await expect(pendingList).toBeDisabled();
    await expect(pendingList).toHaveAttribute("aria-pressed", "true");
    expectNoAccentSelectedTreatment(pendingList.className);

    await expect(canvas.getByText("Alpha target selected")).toBeVisible();
  },
};
