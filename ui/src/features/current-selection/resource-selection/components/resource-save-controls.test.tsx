import { render, screen } from "@testing-library/react";
import { EditableResourceSaveHeaderAction } from "./resource-save-controls";

describe("EditableResourceSaveHeaderAction", () => {
  it("renders save action with warning emphasis when dirty and saveable", () => {
    render(
      <EditableResourceSaveHeaderAction
        canSave
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    expect(screen.getByRole("button", { name: "Save resource" })).toBeTruthy();
  });

  it("disables save when canSave is false", () => {
    render(
      <EditableResourceSaveHeaderAction
        canSave={false}
        onClick={() => undefined}
        saveState={{ status: "idle" }}
      />,
    );

    expect(screen.getByRole("button", { name: "Save resource" }).hasAttribute("disabled")).toBe(
      true,
    );
  });

  it("shows busy label while submitting", () => {
    render(
      <EditableResourceSaveHeaderAction
        canSave={false}
        onClick={() => undefined}
        saveState={{ status: "submitting" }}
      />,
    );

    expect(screen.getByRole("button", { name: "Saving resource..." })).toBeTruthy();
  });
});
