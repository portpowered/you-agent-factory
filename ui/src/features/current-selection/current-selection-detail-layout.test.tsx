import { act, fireEvent, render, screen } from "@testing-library/react";

import { CurrentSelectionLocaleProvider } from "./current-selection-locale";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import {
  resetSelectionHistoryStore,
  useSelectionHistoryStore,
} from "./state/selectionHistoryStore";

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
    expect(
      screen.getByRole("button", { name: "Undo selection" }).textContent,
    ).toBe("Undo");
    expect(
      screen.getByRole("button", { name: "Redo selection" }).textContent,
    ).toBe("Redo");
  });

  it("renders localized history control labels and accessible names from the requested locale", () => {
    render(
      <CurrentSelectionLocaleProvider locale="ja">
        <SelectionDetailLayout>
          <p>Body</p>
        </SelectionDetailLayout>
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByRole("article", { name: "現在の選択" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "現在の選択" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "選択を元に戻す" }).textContent,
    ).toBe("元に戻す");
    expect(
      screen.getByRole("button", { name: "選択をやり直す" }).textContent,
    ).toBe("やり直す");
  });

  it("keeps undo and redo history behavior unchanged apart from the localized copy source", () => {
    const store = useSelectionHistoryStore.getState();

    act(() => {
      store.commitSelectionState({
        selection: { kind: "node", nodeId: "review" },
        terminalWorkDetail: null,
      });
      store.commitSelectionState({
        selection: { kind: "state-node", placeId: "story:complete" },
        terminalWorkDetail: null,
      });
    });

    render(
      <CurrentSelectionLocaleProvider locale="ja">
        <SelectionDetailLayout>
          <p>Body</p>
        </SelectionDetailLayout>
      </CurrentSelectionLocaleProvider>,
    );

    const undoButton = screen.getByRole("button", { name: "選択を元に戻す" });
    const redoButton = screen.getByRole("button", { name: "選択をやり直す" });

    expect(undoButton.hasAttribute("disabled")).toBe(false);
    expect(redoButton.hasAttribute("disabled")).toBe(true);

    act(() => {
      fireEvent.click(undoButton);
    });

    expect(useSelectionHistoryStore.getState().present.selection).toEqual({
      kind: "node",
      nodeId: "review",
    });
    expect(
      screen
        .getByRole("button", { name: "選択をやり直す" })
        .hasAttribute("disabled"),
    ).toBe(false);

    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "選択をやり直す" }));
    });

    expect(useSelectionHistoryStore.getState().present.selection).toEqual({
      kind: "state-node",
      placeId: "story:complete",
    });
    expect(
      screen
        .getByRole("button", { name: "選択をやり直す" })
        .hasAttribute("disabled"),
    ).toBe(true);
  });
});
