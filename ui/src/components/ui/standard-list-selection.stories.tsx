import { expect, within } from "storybook/test";

import {
  StandardListSelection,
  StandardListSelectionItem,
} from "./standard-list-selection";

const STANDARD_LIST_SELECTION_ROW_ACCENT_CLASS =
  "border-primary bg-primary-container text-on-primary factory-light:text-on-primary-container";
const STANDARD_LIST_SELECTION_ROW_DANGER_CLASS =
  "border-af-danger-border bg-error-container text-on-error";
const STANDARD_LIST_SELECTION_ROW_NEUTRAL_CLASS =
  "border-outline bg-surface-container-high text-on-surface hover:border-outline-variant hover:bg-af-overlay";
const STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS =
  "border-outline-variant bg-surface-container-low text-on-surface";
const STANDARD_LIST_SELECTION_ROW_SUCCESS_CLASS =
  "border-af-success-border bg-success-container text-on-success-container";

const SOLID_ACCENT_SELECTED_TOKENS = [
  "bg-primary",
  "border-primary",
  "shadow-af-accent-selected",
] as const;

function expectNoSolidAccentSelectedTreatment(className: string) {
  const tokens = className.split(/\s+/);
  for (const token of SOLID_ACCENT_SELECTED_TOKENS) {
    expect(tokens).not.toContain(token);
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
          Accent and semantic row tones
        </h2>
        <StandardListSelection>
          <StandardListSelectionItem tone="accent">
            Accent unselected
          </StandardListSelectionItem>
          <StandardListSelectionItem selected tone="accent">
            Accent selected
          </StandardListSelectionItem>
          <StandardListSelectionItem tone="success">
            Success unselected
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
    expectNoSolidAccentSelectedTreatment(neutralUnselected.className);

    const neutralSelected = canvas.getByRole("button", {
      name: "Neutral selected",
    });
    expect(neutralSelected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS,
    );
    await expect(neutralSelected).toHaveAttribute("aria-pressed", "true");
    await expect(neutralSelected).toHaveAttribute("data-selected", "true");
    expectNoSolidAccentSelectedTreatment(neutralSelected.className);

    await expect(
      canvas.getByRole("button", { name: "Disabled row" }),
    ).toBeDisabled();

    const accentUnselected = canvas.getByRole("button", {
      name: "Accent unselected",
    });
    expect(accentUnselected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_ACCENT_CLASS,
    );
    expectNoSolidAccentSelectedTreatment(accentUnselected.className);

    const accentSelected = canvas.getByRole("button", {
      name: "Accent selected",
    });
    expect(accentSelected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS,
    );
    expect(accentSelected.className).not.toContain(
      STANDARD_LIST_SELECTION_ROW_ACCENT_CLASS,
    );
    await expect(accentSelected).toHaveAttribute("aria-pressed", "true");
    expectNoSolidAccentSelectedTreatment(accentSelected.className);

    const successUnselected = canvas.getByRole("button", {
      name: "Success unselected",
    });
    expect(successUnselected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_SUCCESS_CLASS,
    );
    expectNoSolidAccentSelectedTreatment(successUnselected.className);

    const dangerUnselected = canvas.getByRole("button", {
      name: "Danger unselected",
    });
    expect(dangerUnselected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_DANGER_CLASS,
    );
    expectNoSolidAccentSelectedTreatment(dangerUnselected.className);

    const dangerSelected = canvas.getByRole("button", {
      name: "Danger selected",
    });
    expect(dangerSelected.className).toContain(
      STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS,
    );
    expectNoSolidAccentSelectedTreatment(dangerSelected.className);

    const pendingList = canvas.getByRole("button", {
      name: "Pending selected row",
    });
    await expect(pendingList).toBeDisabled();
    await expect(pendingList).toHaveAttribute("aria-pressed", "true");
    expectNoSolidAccentSelectedTreatment(pendingList.className);

    await expect(canvas.getByText("Alpha target selected")).toBeVisible();
  },
};
