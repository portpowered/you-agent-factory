import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { getFactoryGraphEditorMessages } from "../../../factory-graph-editor/messages/editor";
import { CurrentActivityGraphHeaderActions } from "../react-flow-current-activity-card-editor-chrome";

describe("CurrentActivityGraphHeaderActions", () => {
  it("renders dirty summary text without mode toggle when showModeToggle is false", async () => {
    const locale = "en";
    const messages = getFactoryGraphEditorMessages(locale);
    const onToggle = vi.fn();

    render(
      <CurrentActivityGraphHeaderActions
        dirtyState={{
          layoutDirty: true,
          preferencesDirty: false,
          topologyDirty: false,
        }}
        editorMode
        hasChanges={false}
        isDefinitionLoading={false}
        locale={locale}
        onToggle={onToggle}
        showModeToggle={false}
      />,
    );

    expect(
      screen.getByText(
        messages.dirtyStateSummary({
          layoutDirty: true,
          preferencesDirty: false,
          topologyDirty: false,
        }),
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: messages.modeLeaveEditor }),
    ).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("renders mode toggle and status pill when showModeToggle is true", async () => {
    const locale = "en";
    const messages = getFactoryGraphEditorMessages(locale);
    const user = userEvent.setup();
    const onToggle = vi.fn();

    render(
      <CurrentActivityGraphHeaderActions
        editorMode={false}
        hasChanges={false}
        isDefinitionLoading={false}
        locale={locale}
        onToggle={onToggle}
        showModeToggle
      />,
    );

    await user.click(
      screen.getByRole("button", { name: messages.modeEnterEditor }),
    );
    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(screen.getByText(messages.modeObserve)).toBeTruthy();
  });
});
