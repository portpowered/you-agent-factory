import { fireEvent, render, screen } from "@testing-library/react";

import {
  EditableWorkStateConfigurationHeaderActions,
  EditableWorkStateSaveHeaderAction,
} from "./work-state-save-controls";

describe("EditableWorkStateConfigurationHeaderActions", () => {
  it("renders discard before save and wires both actions", () => {
    const onDiscard = vi.fn();
    const onSave = vi.fn();

    render(
      <EditableWorkStateConfigurationHeaderActions
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
      "Save work state",
    ]);

    fireEvent.click(
      screen.getByRole("button", { name: "Discard local changes" }),
    );
    expect(onDiscard).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Save work state" }));
    expect(onSave).toHaveBeenCalledTimes(1);
  });
});

describe("EditableWorkStateSaveHeaderAction", () => {
  it("uses warning styling when save is available and not submitting", () => {
    const { rerender } = render(
      <EditableWorkStateSaveHeaderAction
        canSave
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    const saveButton = screen.getByRole("button", { name: "Save work state" });
    expect(saveButton.className).toContain("border-af-warning-border");
    expect(saveButton.className).toContain("bg-af-warning-surface");
    expect(saveButton.className).toContain("text-af-warning-text");

    rerender(
      <EditableWorkStateSaveHeaderAction
        canSave={false}
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Save work state" }).className,
    ).not.toContain("border-af-warning-border");
  });

  it("does not use warning styling while save is submitting", () => {
    render(
      <EditableWorkStateSaveHeaderAction
        canSave
        onClick={() => undefined}
        saveState={{ status: "submitting" }}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Saving work state..." }).className,
    ).not.toContain("border-af-warning-border");
  });
});
