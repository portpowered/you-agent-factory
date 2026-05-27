import { fireEvent, render, screen } from "@testing-library/react";

import { getTerminalWorkMessages } from "../messages/terminal-work";
import { TerminalWorkWidget } from "./terminal-work-widget";

describe("TerminalWorkWidget", () => {
  const originalDocumentLang = document.documentElement.lang;

  afterEach(() => {
    document.documentElement.lang = originalDocumentLang;
  });

  it("uses the browser locale at the terminal-work feature seam when no locale prop is provided", () => {
    document.documentElement.lang = "ja-JP";
    const messages = getTerminalWorkMessages("ja");

    render(
      <TerminalWorkWidget
        completedItems={[{ label: "Done Story", traceWorkID: "work-done-story" }]}
        failedItems={[]}
        onSelectItem={vi.fn()}
        selectedItem={null}
      />,
    );

    expect(screen.getByLabelText(messages.cardTitle)).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: messages.rowTitle("completed") }),
    ).toBeTruthy();
    expect(screen.getByText(messages.emptyState("failed"))).toBeTruthy();
  });

  it("renders zh-CN terminal-work copy when the widget locale is canonical Mandarin", () => {
    const messages = getTerminalWorkMessages("zh-CN");

    render(
      <TerminalWorkWidget
        completedItems={[{ label: "Done Story", traceWorkID: "work-done-story" }]}
        failedItems={[]}
        locale="zh-CN"
        onSelectItem={vi.fn()}
        selectedItem={null}
      />,
    );

    expect(screen.getByLabelText(messages.cardTitle)).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: messages.rowTitle("completed") }),
    ).toBeTruthy();
    expect(screen.getByText(messages.emptyState("failed"))).toBeTruthy();
  });

  it("stays prop-driven by reflecting the selected row and invoking the passed handler", () => {
    const onSelectItem = vi.fn();

    render(
      <TerminalWorkWidget
        completedItems={[{ label: "Done Story", traceWorkID: "work-done-story" }]}
        failedItems={[{ label: "Failed Story", traceWorkID: "work-failed-story" }]}
        onSelectItem={onSelectItem}
        selectedItem={{ label: "Done Story", status: "completed" }}
      />,
    );

    expect(
      screen
        .getByRole("button", { name: /Done Story/ })
        .getAttribute("data-selected"),
    ).toBe("true");
    expect(
      screen
        .getByRole("button", { name: /Failed Story/ })
        .getAttribute("data-selected"),
    ).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /Failed Story/ }));

    expect(onSelectItem).toHaveBeenCalledWith(
      "failed",
      expect.objectContaining({
        label: "Failed Story",
        traceWorkID: "work-failed-story",
      }),
    );
  });
});
