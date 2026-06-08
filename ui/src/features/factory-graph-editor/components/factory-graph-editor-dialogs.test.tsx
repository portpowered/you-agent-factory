import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import {
  FactoryGraphEditorActionPopover,
  FactoryGraphEditorConfirmationDialog,
} from "./factory-graph-editor-dialogs";

describe("FactoryGraphEditorActionPopover", () => {
  it("renders popover content when open", async () => {
    const user = userEvent.setup();

    function Harness() {
      const [open, setOpen] = useState(false);
      return (
        <FactoryGraphEditorActionPopover
          description="Add a graph entity."
          onOpenChange={setOpen}
          open={open}
          title="Add entity"
          trigger={<button type="button">Open menu</button>}
        >
          <button type="button">Workstation</button>
        </FactoryGraphEditorActionPopover>
      );
    }

    render(<Harness />);
    await user.click(screen.getByRole("button", { name: "Open menu" }));

    expect(screen.getByText("Add entity")).toBeTruthy();
    expect(screen.getByText("Add a graph entity.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Workstation" })).toBeTruthy();
  });
});

describe("FactoryGraphEditorConfirmationDialog", () => {
  it("blocks dismiss actions while busy", () => {
    const onCancel = vi.fn();

    render(
      <FactoryGraphEditorConfirmationDialog
        cancelLabel="Cancel"
        confirmLabel="Confirm"
        description="Busy dialog"
        isBusy
        isOpen
        onCancel={onCancel}
        onConfirm={() => {}}
        title="Confirm action"
      />,
    );

    const dialog = screen.getByRole("dialog", { name: "Confirm action" });
    fireEvent.keyDown(dialog, { key: "Escape" });
    fireEvent.pointerDown(document.body);

    expect(onCancel).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Cancel" })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("calls onCancel when the dialog closes while idle", async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();

    render(
      <FactoryGraphEditorConfirmationDialog
        cancelLabel="Cancel"
        confirmLabel="Confirm"
        description="Idle dialog"
        isOpen
        onCancel={onCancel}
        onConfirm={() => {}}
        title="Confirm action"
      />,
    );

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});
