import { fireEvent, render, screen, within } from "@testing-library/react";

import { EditableWorkstationSaveDialog } from "./workstation-save-controls";

describe("EditableWorkstationSaveDialog", () => {
  it("opens the overwrite confirmation and wires cancel and confirm actions", () => {
    const onCancel = vi.fn();
    const onConfirm = vi.fn();

    render(
      <EditableWorkstationSaveDialog
        onCancel={onCancel}
        onConfirm={onConfirm}
        overwriteFieldNames={[]}
        saveState={{ status: "confirming" }}
      />,
    );

    const dialog = screen.getByRole("dialog", {
      name: "Overwrite the running factory definition?",
    });
    expect(
      within(dialog).getByText(
        "Saving will overwrite the running factory definition with the kind, worker, and prompt values in this workstation draft.",
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

  it("keeps the save dialog locked while submitting", () => {
    const onCancel = vi.fn();
    const onConfirm = vi.fn();

    render(
      <EditableWorkstationSaveDialog
        onCancel={onCancel}
        onConfirm={onConfirm}
        overwriteFieldNames={["prompt", "worker"]}
        saveState={{ status: "submitting" }}
      />,
    );

    const dialog = screen.getByRole("dialog", {
      name: "Overwrite the running factory definition?",
    });
    expect(
      within(dialog).getByText(
        "Saving will overwrite newer server values for prompt, worker with the draft currently shown in the editor.",
      ),
    ).toBeTruthy();

    const cancelButton = within(dialog).getByRole("button", { name: "Cancel" });
    const submitButton = within(dialog).getByRole("button", {
      name: "Saving...",
    });
    expect(cancelButton.getAttribute("disabled")).not.toBeNull();
    expect(submitButton.getAttribute("disabled")).not.toBeNull();
    expect(submitButton.getAttribute("aria-busy")).toBe("true");

    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(onCancel).not.toHaveBeenCalled();
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
