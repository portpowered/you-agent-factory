import { render, screen } from "@testing-library/react";

import { getTerminalWorkMessages } from "./messages";
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
});
