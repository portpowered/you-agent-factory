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
    expect(screen.getByText("Save could not complete.").className).toContain(
      "!text-current",
    );
    expect(screen.getByText("Save could not complete.").className).toContain(
      "af-dashboard-body-text",
    );

    const dismissButton = screen.getByRole("button", { name: "Dismiss" });
    expect(dismissButton.className).toContain("h-9");

    dismissButton.focus();
    await user.keyboard("{Enter}");
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("uses shared warning alert styling for warning notices", () => {
    render(
      <FactoryGraphEditorNotice title="Unsaved changes" tone="warning">
        Review the draft before leaving.
      </FactoryGraphEditorNotice>,
    );

    expect(screen.getByRole("status").className).toContain(
      "bg-warning-container",
    );
    expect(
      screen.getByText("Review the draft before leaving.").className,
    ).toContain("!text-current");
  });
});
