import { act, fireEvent, render, screen, within } from "@testing-library/react";
import {
  resetSelectionHistoryStore,
  useSelectionHistoryStore,
} from "../state/selectionHistoryStore";
import {
  CurrentSelectionHeaderActionProvider,
  SelectionDetailLayout,
} from "./current-selection-detail-layout";
import { CurrentSelectionLocaleProvider } from "./current-selection-locale";

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
    ).toBe("");
    expect(
      screen.getByRole("button", { name: "Redo selection" }).textContent,
    ).toBe("");
    expect(
      screen.getByRole("button", { name: "Undo selection" }).className,
    ).toContain("rounded-lg");
    expect(
      screen.getByRole("button", { name: "Undo selection" }).className,
    ).toContain("h-10");
    expect(
      screen.getByRole("button", { name: "Undo selection" }).className,
    ).toContain("w-10");
  });

  it("renders localized history control labels and accessible names from the requested locale", () => {
    render(
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <SelectionDetailLayout>
          <p>Body</p>
        </SelectionDetailLayout>
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByRole("article", { name: "当前选择" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "当前选择" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "撤销所选内容" }).textContent,
    ).toBe("");
    expect(
      screen.getByRole("button", { name: "重做所选内容" }).textContent,
    ).toBe("");
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
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <SelectionDetailLayout>
          <p>Body</p>
        </SelectionDetailLayout>
      </CurrentSelectionLocaleProvider>,
    );

    const undoButton = screen.getByRole("button", { name: "撤销所选内容" });
    const redoButton = screen.getByRole("button", { name: "重做所选内容" });

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
        .getByRole("button", { name: "重做所选内容" })
        .hasAttribute("disabled"),
    ).toBe(false);

    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "重做所选内容" }));
    });

    expect(useSelectionHistoryStore.getState().present.selection).toEqual({
      kind: "state-node",
      placeId: "story:complete",
    });
    expect(
      screen
        .getByRole("button", { name: "重做所选内容" })
        .hasAttribute("disabled"),
    ).toBe(true);
  });

  it("orders undo, redo, detail actions, then shared header actions", () => {
    render(
      <CurrentSelectionHeaderActionProvider
        headerAction={<button type="button">Remove card</button>}
      >
        <SelectionDetailLayout
          headerAction={<button type="button">Save changes</button>}
        >
          <p>Body</p>
        </SelectionDetailLayout>
      </CurrentSelectionHeaderActionProvider>,
    );

    const actionSection = screen
      .getByRole("button", { name: "Undo selection" })
      .closest("[data-dashboard-action-row-section='actions']");

    expect(actionSection).toBeTruthy();
    const buttons = within(actionSection as HTMLElement).getAllByRole("button");

    expect(
      buttons.map(
        (button) => button.getAttribute("aria-label") ?? button.textContent,
      ),
    ).toEqual([
      "Undo selection",
      "Redo selection",
      "Save changes",
      "Remove card",
    ]);
  });
});
