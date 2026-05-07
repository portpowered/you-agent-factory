import { render, screen } from "@testing-library/react";

import { SelectionDetailLayout } from "./current-selection-detail-layout";
import { resetSelectionHistoryStore } from "./state/selectionHistoryStore";

describe("SelectionDetailLayout", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
  });

  afterEach(() => {
    resetSelectionHistoryStore();
  });

  it("uses the default English shell title when locale is omitted", () => {
    render(
      <SelectionDetailLayout>
        <p>Body</p>
      </SelectionDetailLayout>,
    );

    expect(
      screen.getByRole("article", { name: "Current selection" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Move Current selection" }),
    ).toBeTruthy();
  });

  it("renders the shell title from the requested locale", () => {
    render(
      <SelectionDetailLayout locale="ja">
        <p>Body</p>
      </SelectionDetailLayout>,
    );

    expect(screen.getByRole("article", { name: "現在の選択" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "現在の選択" })).toBeTruthy();
  });
});
