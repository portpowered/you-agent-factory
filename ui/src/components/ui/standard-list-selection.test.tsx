import { fireEvent, render, screen } from "@testing-library/react";

import {
  StandardListSelection,
  StandardListSelectionItem,
  standardListSelectionRowClassName,
} from "./standard-list-selection";

/** Solid primary fill + selected shadow — not used for unselected accent rows (they use container + border-primary). */
const SOLID_ACCENT_SELECTED_TOKENS = [
  "bg-primary",
  "shadow-af-accent-selected",
] as const;
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

function expectNoSolidAccentSelectedTreatment(className: string) {
  const tokens = className.split(/\s+/);
  for (const token of SOLID_ACCENT_SELECTED_TOKENS) {
    expect(tokens).not.toContain(token);
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
  it("exposes row variant class generation without exporting raw class constants", () => {
    expect(
      standardListSelectionRowClassName({
        selected: false,
        tone: "success",
      }),
    ).toContain(STANDARD_LIST_SELECTION_ROW_SUCCESS_CLASS);
    expect(
      standardListSelectionRowClassName({
        selected: true,
        tone: "danger",
      }),
    ).toContain(STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS);
  });

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
    expect(row.className).toContain("af-body-text");
    expectNoSolidAccentSelectedTreatment(row.className);
  });

  it("allows row typography to be omitted for custom content", () => {
    render(
      <StandardListSelectionItem textRole="none">
        Custom row
      </StandardListSelectionItem>,
    );

    expect(
      screen.getByRole("button", { name: "Custom row" }).className,
    ).not.toContain("af-body-text");
  });

  it("uses neutral selected surfaces without accent fill when selected", () => {
    render(
      <StandardListSelectionItem selected tone="accent">
        Selected row
      </StandardListSelectionItem>,
    );

    const row = screen.getByRole("button", { name: "Selected row" });
    expect(row.className).toContain(STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS);
    expect(row.className).not.toContain(
      STANDARD_LIST_SELECTION_ROW_ACCENT_CLASS,
    );
    expect(row.getAttribute("aria-pressed")).toBe("true");
    expect(row.getAttribute("data-selected")).toBe("true");
    expectNoSolidAccentSelectedTreatment(row.className);
  });

  it("applies accent, success, and danger tone variants only while unselected", () => {
    const { rerender } = render(
      <StandardListSelectionItem tone="accent">
        Accent row
      </StandardListSelectionItem>,
    );

    let row = screen.getByRole("button", { name: "Accent row" });
    expect(row.className).toContain(STANDARD_LIST_SELECTION_ROW_ACCENT_CLASS);
    expect(row.className).toContain("border-primary");
    expect(row.className).toContain("bg-primary-container");
    expectNoSolidAccentSelectedTreatment(row.className);

    rerender(
      <StandardListSelectionItem tone="success">
        Success row
      </StandardListSelectionItem>,
    );
    row = screen.getByRole("button", { name: "Success row" });
    expect(row.className).toContain(STANDARD_LIST_SELECTION_ROW_SUCCESS_CLASS);
    expectNoSolidAccentSelectedTreatment(row.className);

    rerender(
      <StandardListSelectionItem tone="danger">
        Danger row
      </StandardListSelectionItem>,
    );
    row = screen.getByRole("button", { name: "Danger row" });
    expect(row.className).toContain(STANDARD_LIST_SELECTION_ROW_DANGER_CLASS);
    expectNoSolidAccentSelectedTreatment(row.className);
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
