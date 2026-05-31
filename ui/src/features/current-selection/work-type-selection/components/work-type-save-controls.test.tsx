import { fireEvent, render, screen, within } from "@testing-library/react";

import {
  EditableWorkTypeSaveDialog,
  EditableWorkTypeSaveHeaderAction,
} from "./work-type-save-controls";

describe("EditableWorkTypeSaveHeaderAction", () => {
  it("uses warning styling when save is available and not submitting", () => {
    const { rerender } = render(
      <EditableWorkTypeSaveHeaderAction
        canSave
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    const saveButton = screen.getByRole("button", { name: "Save changes" });
    expect(saveButton.className).toContain("border-af-warning-border");

    rerender(
      <EditableWorkTypeSaveHeaderAction
        canSave={false}
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    expect(screen.getByRole("button", { name: "Save changes" }).className).not.toContain(
      "border-af-warning-border",
    );
  });
});

describe("EditableWorkTypeSaveDialog", () => {
  it("opens the overwrite confirmation and wires cancel and confirm actions", () => {
    const onCancel = vi.fn();
    const onConfirm = vi.fn();

    render(
      <EditableWorkTypeSaveDialog
        onCancel={onCancel}
        onConfirm={onConfirm}
        saveState={{ status: "confirming" }}
      />,
    );

    const dialog = screen.getByRole("dialog", {
      name: "Overwrite the running factory definition?",
    });
    expect(
      within(dialog).getByText(
        "Saving will overwrite the running factory definition with the work type name and CLI handling behavior in this draft.",
      ),
    ).toBeTruthy();

    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();

    fireEvent.click(
      within(dialog).getByRole("button", { name: "Overwrite factory" }),
    );
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});
