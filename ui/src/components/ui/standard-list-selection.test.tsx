import { fireEvent, render, screen } from "@testing-library/react";

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

describe("StandardListSelection", () => {
  it("renders a vertical list shell with optional selection announcement", () => {
    const { container } = render(
      <StandardListSelection selectionAnnouncement="Alpha target">
        <StandardListSelectionItem selected>Alpha</StandardListSelectionItem>
      </StandardListSelection>,
    );

    const list = container.firstElementChild;
    expect(list?.className).toContain("grid");
    expect(list?.className).toContain("gap-2");
    expect(screen.getByText("Alpha target").getAttribute("aria-live")).toBe(
      "polite",
    );
  });

  it("marks the list busy and disables rows when pending", () => {
    const onClick = vi.fn();

    const { container } = render(
      <StandardListSelection disabled>
        <StandardListSelectionItem onClick={onClick}>
          Alpha
        </StandardListSelectionItem>
      </StandardListSelection>,
    );

    const list = container.firstElementChild;
    expect(list?.getAttribute("aria-busy")).toBe("true");

    const row = screen.getByRole("button", { name: "Alpha" });
    expect(row.disabled).toBe(true);
    fireEvent.click(row);
    expect(onClick).not.toHaveBeenCalled();
  });
});

describe("StandardListSelectionItem", () => {
  it("uses neutral dark gray surfaces when unselected", () => {
    render(
      <StandardListSelectionItem tone="neutral">
        Neutral row
      </StandardListSelectionItem>,
    );

    const row = screen.getByRole("button", { name: "Neutral row" });
    expect(row.className).toContain(STANDARD_LIST_SELECTION_ROW_NEUTRAL_CLASS);
    expect(row.getAttribute("aria-pressed")).toBe("false");
    expect(row.getAttribute("data-selected")).toBe("false");
    expectNoAccentSelectedTreatment(row.className);
  });

  it("uses neutral selected surfaces without accent fill when selected", () => {
    render(
      <StandardListSelectionItem selected tone="info">
        Selected row
      </StandardListSelectionItem>,
    );

    const row = screen.getByRole("button", { name: "Selected row" });
    expect(row.className).toContain(STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS);
    expect(row.className).not.toContain(STANDARD_LIST_SELECTION_ROW_INFO_CLASS);
    expect(row.getAttribute("aria-pressed")).toBe("true");
    expect(row.getAttribute("data-selected")).toBe("true");
    expectNoAccentSelectedTreatment(row.className);
  });

  it("applies info and danger tone variants only while unselected", () => {
    const { rerender } = render(
      <StandardListSelectionItem tone="info">
        Info row
      </StandardListSelectionItem>,
    );

    let row = screen.getByRole("button", { name: "Info row" });
    expect(row.className).toContain(STANDARD_LIST_SELECTION_ROW_INFO_CLASS);
    expectNoAccentSelectedTreatment(row.className);

    rerender(
      <StandardListSelectionItem tone="danger">
        Danger row
      </StandardListSelectionItem>,
    );
    row = screen.getByRole("button", { name: "Danger row" });
    expect(row.className).toContain(STANDARD_LIST_SELECTION_ROW_DANGER_CLASS);
    expectNoAccentSelectedTreatment(row.className);
  });

  it("honors per-row disabled state over list pending", () => {
    render(
      <StandardListSelection>
        <StandardListSelectionItem disabled>
          Disabled row
        </StandardListSelectionItem>
      </StandardListSelection>,
    );

    expect(screen.getByRole("button", { name: "Disabled row" }).disabled).toBe(
      true,
    );
  });
});
