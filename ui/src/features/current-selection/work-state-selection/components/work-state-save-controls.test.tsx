import { render, screen } from "@testing-library/react";

import { EditableWorkStateSaveHeaderAction } from "./work-state-save-controls";

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
