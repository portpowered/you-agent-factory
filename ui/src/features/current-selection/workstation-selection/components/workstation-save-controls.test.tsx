import { fireEvent, render, screen } from "@testing-library/react";

import {
  EditableWorkstationConfigurationHeaderActions,
  EditableWorkstationSaveHeaderAction,
} from "./workstation-save-controls";

describe("EditableWorkstationConfigurationHeaderActions", () => {
  it("renders discard before save and wires both actions", () => {
    const onDiscard = vi.fn();
    const onSave = vi.fn();

    render(
      <EditableWorkstationConfigurationHeaderActions
        canDiscard
        canSave
        onDiscard={onDiscard}
        onSave={onSave}
        saveState={{ status: "idle" }}
      />,
    );

    const buttons = screen.getAllByRole("button");
    expect(buttons.map((button) => button.getAttribute("aria-label"))).toEqual([
      "Discard local changes",
      "Save changes",
    ]);

    fireEvent.click(
      screen.getByRole("button", { name: "Discard local changes" }),
    );
    expect(onDiscard).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    expect(onSave).toHaveBeenCalledTimes(1);
  });
});

describe("EditableWorkstationSaveHeaderAction", () => {
  it("uses stronger warning hover styling when save is available and not submitting", () => {
    const { rerender } = render(
      <EditableWorkstationSaveHeaderAction
        canSave
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    const saveButton = screen.getByRole("button", { name: "Save changes" });
    expect(saveButton.className).toContain("border-af-warning-border");
    expect(saveButton.className).toContain("bg-warning-container");
    expect(saveButton.className).toContain("text-on-warning-container");
    expect(saveButton.className).toContain("hover:border-af-warning");
    expect(saveButton.className).toContain("hover:bg-warning");
    expect(saveButton.className).toContain("hover:text-on-warning");

    rerender(
      <EditableWorkstationSaveHeaderAction
        canSave={false}
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Save changes" }).className,
    ).not.toContain("border-af-warning-border");
  });

  it("does not use active warning styling while save is submitting", () => {
    render(
      <EditableWorkstationSaveHeaderAction
        canSave
        onClick={() => undefined}
        saveState={{ status: "submitting" }}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Saving..." }).className,
    ).not.toContain("border-af-warning-border");
  });
});
