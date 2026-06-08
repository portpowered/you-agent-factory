import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { FactoryGraphEditorToolbar } from "./controls/factory-graph-editor-controls";
import {
  ConnectIcon,
  RedoIcon,
  ResetLayoutIcon,
  SaveIcon,
  TrashIcon,
  UndoIcon,
} from "./factory-graph-editor-toolbar-icons";

describe("factory graph editor toolbar icons", () => {
  it("renders icon svg elements for toolbar actions", () => {
    const { container } = render(
      <>
        <ConnectIcon />
        <TrashIcon />
        <UndoIcon />
        <RedoIcon />
        <ResetLayoutIcon />
        <SaveIcon />
      </>,
    );

    const icons = container.querySelectorAll("svg[aria-hidden='true']");
    expect(icons.length).toBe(6);
  });

  it("invokes layout history toolbar actions when enabled", async () => {
    const user = userEvent.setup();
    const onUndoLayout = vi.fn();
    const onRedoLayout = vi.fn();
    const onResetLayout = vi.fn();

    render(
      <div className="relative min-h-48">
        <FactoryGraphEditorToolbar
          activeTool={null}
          canInteract={true}
          canRedoLayout={true}
          canUndoLayout={true}
          hasPendingChanges={false}
          onRedoLayout={onRedoLayout}
          onResetLayout={onResetLayout}
          onSelectTool={() => {}}
          onUndoLayout={onUndoLayout}
          visible={true}
        />
      </div>,
    );

    await user.click(screen.getByRole("button", { name: "Undo" }));
    await user.click(screen.getByRole("button", { name: "Redo" }));
    await user.click(screen.getByRole("button", { name: "Reset layout" }));

    expect(onUndoLayout).toHaveBeenCalledTimes(1);
    expect(onRedoLayout).toHaveBeenCalledTimes(1);
    expect(onResetLayout).toHaveBeenCalledTimes(1);
  });
});
