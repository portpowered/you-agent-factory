import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { EditableConfigurationSaveRow } from "./editable-configuration-save-row";

describe("EditableConfigurationSaveRow", () => {
  it("disables the labeled Save button when canSave is false", () => {
    render(
      <EditableConfigurationSaveRow
        busyLabel="Saving worker..."
        canSave={false}
        isSaving={false}
        onSave={() => undefined}
        saveLabel="Save worker"
      />,
    );

    expect(screen.getByRole("button", { name: "Save worker" })).toBeDisabled();
  });

  it("enables Save with warning styling when canSave is true and not saving", () => {
    render(
      <EditableConfigurationSaveRow
        busyLabel="Saving changes..."
        canSave
        isSaving={false}
        onSave={() => undefined}
        saveLabel="Save changes"
      />,
    );

    const saveButton = screen.getByRole("button", { name: "Save changes" });
    expect(saveButton).toBeEnabled();
    expect(saveButton.className).toContain("border-af-warning-border");
    expect(saveButton.className).toContain("bg-warning-container");
    expect(saveButton.className).toContain("text-on-warning-container");
  });

  it("shows busy label text and aria-busy while saving", () => {
    render(
      <EditableConfigurationSaveRow
        busyLabel="Saving worker..."
        canSave
        isSaving
        onSave={() => undefined}
        saveLabel="Save worker"
      />,
    );

    const saveButton = screen.getByRole("button", { name: "Saving worker..." });
    expect(saveButton).toHaveAttribute("aria-busy", "true");
    expect(saveButton).toBeDisabled();
    expect(saveButton.className).not.toContain("border-af-warning-border");
  });

  it("renders an optional reset slot before the Save button", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const onReset = vi.fn();

    render(
      <EditableConfigurationSaveRow
        busyLabel="Saving worker..."
        canSave
        isSaving={false}
        onSave={onSave}
        resetSlot={
          <button onClick={onReset} type="button">
            Reset to latest
          </button>
        }
        saveLabel="Save worker"
      />,
    );

    await user.click(screen.getByRole("button", { name: "Reset to latest" }));
    await user.click(screen.getByRole("button", { name: "Save worker" }));

    expect(onReset).toHaveBeenCalledTimes(1);
    expect(onSave).toHaveBeenCalledTimes(1);
  });
});
