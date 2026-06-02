import { fireEvent, render, screen } from "@testing-library/react";
import {
  EditableResourceConfigurationHeaderActions,
  EditableResourceSaveHeaderAction,
} from "./resource-save-controls";

describe("EditableResourceConfigurationHeaderActions", () => {
  it("renders discard before save and wires both actions", () => {
    const onDiscard = vi.fn();
    const onSave = vi.fn();

    render(
      <EditableResourceConfigurationHeaderActions
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
      "Save resource",
    ]);

    fireEvent.click(
      screen.getByRole("button", { name: "Discard local changes" }),
    );
    expect(onDiscard).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Save resource" }));
    expect(onSave).toHaveBeenCalledTimes(1);
  });
});

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

    expect(
      screen
        .getByRole("button", { name: "Save resource" })
        .hasAttribute("disabled"),
    ).toBe(true);
  });

  it("shows busy label while submitting", () => {
    render(
      <EditableResourceSaveHeaderAction
        canSave={false}
        onClick={() => undefined}
        saveState={{ status: "submitting" }}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Saving resource..." }),
    ).toBeTruthy();
  });
});
