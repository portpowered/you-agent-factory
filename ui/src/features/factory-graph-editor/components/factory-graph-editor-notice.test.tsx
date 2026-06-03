import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { FactoryGraphEditorNotice } from "./factory-graph-editor-notice";

describe("FactoryGraphEditorNotice", () => {
  it("renders a keyboard-reachable dismiss action for danger notices", async () => {
    const user = userEvent.setup();
    const onDismiss = vi.fn();

    render(
      <FactoryGraphEditorNotice
        dismissLabel="Dismiss"
        onDismiss={onDismiss}
        title="Topology save failed"
        tone="danger"
      >
        Save could not complete.
      </FactoryGraphEditorNotice>,
    );

    expect(screen.getByRole("alert")).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Topology save failed" }),
    ).toBeTruthy();

    const dismissButton = screen.getByRole("button", { name: "Dismiss" });
    expect(dismissButton.className).toContain("h-9");

    dismissButton.focus();
    await user.keyboard("{Enter}");
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
