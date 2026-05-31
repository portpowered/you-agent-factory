import { render, screen } from "@testing-library/react";

import { EditableWorkerSaveHeaderAction } from "./worker-save-controls";

describe("EditableWorkerSaveHeaderAction", () => {
  it("uses warning styling when save is available and not submitting", () => {
    const { rerender } = render(
      <EditableWorkerSaveHeaderAction
        canSave
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    const saveButton = screen.getByRole("button", { name: "Save worker" });
    expect(saveButton.className).toContain("border-af-warning-border");
    expect(saveButton.className).toContain("bg-af-warning-surface");
    expect(saveButton.className).toContain("text-af-warning-text");

    rerender(
      <EditableWorkerSaveHeaderAction
        canSave={false}
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    expect(screen.getByRole("button", { name: "Save worker" }).className).not.toContain(
      "border-af-warning-border",
    );
  });

  it("does not use warning styling while save is submitting", () => {
    render(
      <EditableWorkerSaveHeaderAction
        canSave
        onClick={() => undefined}
        saveState={{ status: "submitting" }}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Saving worker..." }).className,
    ).not.toContain("border-af-warning-border");
  });
});
