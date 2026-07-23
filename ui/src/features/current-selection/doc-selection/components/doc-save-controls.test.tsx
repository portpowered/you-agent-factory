import { fireEvent, render, screen } from "@testing-library/react";

import {
  EditableDocConfigurationHeaderActions,
  EditableDocSaveHeaderAction,
} from "./doc-save-controls";

describe("EditableDocConfigurationHeaderActions", () => {
  it("renders discard before save and wires both actions", () => {
    const onDiscard = vi.fn();
    const onSave = vi.fn();

    render(
      <EditableDocConfigurationHeaderActions
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
      "Save doc",
    ]);

    fireEvent.click(
      screen.getByRole("button", { name: "Discard local changes" }),
    );
    expect(onDiscard).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Save doc" }));
    expect(onSave).toHaveBeenCalledTimes(1);
  });
});

describe("EditableDocSaveHeaderAction", () => {
  it("uses warning styling when save is available and not submitting", () => {
    const { rerender } = render(
      <EditableDocSaveHeaderAction
        canSave
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    const saveButton = screen.getByRole("button", { name: "Save doc" });
    expect(saveButton.className).toContain("border-af-warning-border");

    rerender(
      <EditableDocSaveHeaderAction
        canSave={false}
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Save doc" }).className,
    ).not.toContain("border-af-warning-border");
  });

  it("shows the busy save label while submitting", () => {
    render(
      <EditableDocSaveHeaderAction
        canSave
        onClick={() => undefined}
        saveState={{ status: "submitting" }}
      />,
    );

    expect(screen.getByRole("button", { name: "Saving doc..." })).toBeTruthy();
  });
});
