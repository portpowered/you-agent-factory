import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { CurrentActivityGraphHeaderActions } from "./react-flow-current-activity-card-editor-chrome";

describe("CurrentActivityGraphHeaderActions", () => {
  it("renders the status pill before the mode-toggle action", () => {
    const { container } = render(
      <CurrentActivityGraphHeaderActions
        editorMode={false}
        hasChanges={false}
        isDefinitionLoading={false}
        onToggle={() => {}}
      />,
    );

    const sections = container.querySelectorAll(
      "[data-dashboard-action-row-section]",
    );

    expect(sections).toHaveLength(2);
    expect(sections[0]?.getAttribute("data-dashboard-action-row-section")).toBe(
      "statuses",
    );
    expect(sections[1]?.getAttribute("data-dashboard-action-row-section")).toBe(
      "actions",
    );
    expect(within(sections[0] as HTMLElement).getByText("Observe mode")).toBeTruthy();
    expect(
      within(sections[1] as HTMLElement).getByRole("button", {
        name: "Enter factory graph editor",
      }),
    ).toBeTruthy();
  });

  it("preserves the tooltip and pressed state on the shared mode-toggle action", async () => {
    const user = userEvent.setup();

    render(
      <CurrentActivityGraphHeaderActions
        editorMode={true}
        hasChanges={true}
        isDefinitionLoading={false}
        onToggle={() => {}}
      />,
    );

    const toggle = screen.getByRole("button", {
      name: "Leave factory graph editor",
    });

    expect(toggle.getAttribute("aria-pressed")).toBe("true");
    await user.hover(toggle);
    expect(
      await screen.findByRole("tooltip", {
        name: "Leave factory graph editor",
      }),
    ).toBeTruthy();
    expect(screen.getByText("Unsaved graph changes")).toBeTruthy();
  });
});
