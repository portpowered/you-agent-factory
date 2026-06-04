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
    expect(
      within(sections[0] as HTMLElement).getByText("Observe"),
    ).toBeTruthy();
    expect(
      within(sections[1] as HTMLElement).getByRole("button", {
        name: "Edit mode",
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
      name: "Leave editor",
    });

    expect(toggle.getAttribute("aria-pressed")).toBe("true");
    expect(toggle.className).toContain("border-af-warning-border");
    expect(toggle.className).toContain("bg-warning-container");
    expect(toggle.className).toContain("text-on-warning-container");
    await user.hover(toggle);
    expect(
      await screen.findByRole("tooltip", {
        name: "Leave editor",
      }),
    ).toBeTruthy();
    expect(screen.getByText("Unsaved changes")).toBeTruthy();
  });

  it("applies warning styling on the compact header toggle when dirty", () => {
    render(
      <CurrentActivityGraphHeaderActions
        compact
        editorMode={true}
        hasChanges={true}
        isDefinitionLoading={false}
        onToggle={() => {}}
      />,
    );

    const toggle = screen.getByRole("button", {
      name: "Leave editor",
    });

    expect(toggle.className).toContain("border-af-warning-border");
    expect(toggle.className).toContain("bg-warning-container");
    expect(toggle.className).toContain("text-on-warning-container");
    expect(toggle.className).not.toContain("text-on-surface-variant");
  });

  it("keeps custom header actions in the actions section after the mode toggle", () => {
    const { container } = render(
      <CurrentActivityGraphHeaderActions
        editorMode={false}
        hasChanges={false}
        headerActions={<button type="button">Remove card</button>}
        isDefinitionLoading={false}
        onToggle={() => {}}
      />,
    );

    const sections = container.querySelectorAll(
      "[data-dashboard-action-row-section]",
    );
    const actionsSection = sections[1] as HTMLElement;
    const actionButtons = within(actionsSection).getAllByRole("button");

    expect(actionButtons).toHaveLength(2);
    expect(actionButtons[0]?.getAttribute("aria-label")).toBe("Edit mode");
    expect(actionButtons[1]?.textContent).toBe("Remove card");
  });
});
