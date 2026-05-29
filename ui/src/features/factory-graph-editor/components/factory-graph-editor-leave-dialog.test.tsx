import { fireEvent, render, screen, within } from "@testing-library/react";

import { FactoryGraphEditorLeaveDialog } from "./factory-graph-editor-leave-dialog";

describe("FactoryGraphEditorLeaveDialog", () => {
  it("opens with keep-editing, discard, and save actions", () => {
    const onCancel = vi.fn();
    const onDiscard = vi.fn();
    const onSave = vi.fn();

    render(
      <FactoryGraphEditorLeaveDialog
        canSave={true}
        isOpen={true}
        isSaving={false}
        onCancel={onCancel}
        onDiscard={onDiscard}
        onSave={onSave}
      />,
    );

    const dialog = screen.getByRole("dialog", {
      name: "Leave graph editor with unsaved changes?",
    });
    expect(
      within(dialog).getByText(
        "This graph editor session still has local topology changes.",
      ),
    ).toBeTruthy();
    expect(
      within(dialog).getByText(
        "Save to keep the pending factory topology, discard to revert to the latest server-backed graph, or keep editing.",
      ),
    ).toBeTruthy();

    fireEvent.click(
      within(dialog).getByRole("button", { name: "Keep editing" }),
    );
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Discard changes" }),
    );
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Save changes" }),
    );

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onDiscard).toHaveBeenCalledTimes(1);
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it("locks dismissal and mutation actions while saving", () => {
    const onCancel = vi.fn();
    const onDiscard = vi.fn();
    const onSave = vi.fn();

    render(
      <FactoryGraphEditorLeaveDialog
        canSave={true}
        isOpen={true}
        isSaving={true}
        onCancel={onCancel}
        onDiscard={onDiscard}
        onSave={onSave}
      />,
    );

    const dialog = screen.getByRole("dialog", {
      name: "Leave graph editor with unsaved changes?",
    });
    expect(
      within(dialog)
        .getByRole("button", { name: "Keep editing" })
        .getAttribute("disabled"),
    ).not.toBeNull();
    expect(
      within(dialog)
        .getByRole("button", { name: "Discard changes" })
        .getAttribute("disabled"),
    ).not.toBeNull();
    expect(
      within(dialog)
        .getByRole("button", { name: "Saving..." })
        .getAttribute("disabled"),
    ).not.toBeNull();

    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(onCancel).not.toHaveBeenCalled();
    expect(onDiscard).not.toHaveBeenCalled();
    expect(onSave).not.toHaveBeenCalled();
  });
});
