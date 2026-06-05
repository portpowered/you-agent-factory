import { render, screen } from "@testing-library/react";

import { ChooseFileField } from "./choose-file-field";
import { ChooseFileNativeInput } from "./choose-file-native-input";

describe("ChooseFileField", () => {
  it("renders export-style field grouping with neutral dashed shell on the control", () => {
    render(
      <ChooseFileField
        control={
          <label htmlFor="choose-file-test" id="choose-file-shell">
            Choose a file
          </label>
        }
        description={<p>Supported formats: PNG</p>}
        label={<span>Cover image</span>}
      />,
    );

    const fieldGroup = screen.getByText("Cover image").parentElement;
    const shell = screen.getByText("Choose a file");
    expect(fieldGroup?.className).toContain("space-y-2");
    expect(shell.className).toContain("border-dashed");
    expect(shell.className).toContain("border-outline-variant");
    expect(shell.className).toContain("bg-surface-container-low");
    expect(shell.className).not.toContain("bg-primary-container");
    expect(shell.className).not.toContain("border-primary");
    expect(screen.getByText("Cover image")).toBeTruthy();
    expect(screen.getByText("Supported formats: PNG")).toBeTruthy();
  });

  it("uses neutral border and surface emphasis when drag is active", () => {
    const { rerender } = render(
      <ChooseFileField
        control={<div data-testid="choose-file-shell">Drop files here</div>}
        dragActive={false}
      />,
    );

    const shell = screen.getByTestId("choose-file-shell");
    expect(shell.className).toContain("bg-surface-container-low");
    expect(shell.className).not.toContain("bg-primary-container");
    expect(shell.className).not.toContain("border-primary");

    rerender(
      <ChooseFileField
        control={<div data-testid="choose-file-shell">Drop files here</div>}
        dragActive
      />,
    );

    const activeShell = screen.getByTestId("choose-file-shell");
    expect(activeShell.className).toContain("border-outline-variant");
    expect(activeShell.className).toContain("bg-af-overlay");
    expect(activeShell.className).not.toContain("bg-primary-container");
    expect(activeShell.className).not.toContain("border-primary");
  });

  it("composes native file input chrome inside the shared dashed shell", () => {
    render(
      <ChooseFileField
        control={
          <ChooseFileNativeInput
            aria-label="Factory cover image"
            className="custom-native-input"
          />
        }
        label={<span>Cover image</span>}
      />,
    );

    const input = screen.getByLabelText("Factory cover image");
    expect(input.getAttribute("type")).toBe("file");
    expect(input.className).toContain("border-dashed");
    expect(input.className).toContain("bg-surface-container-low");
    expect(input.className).toContain("file:bg-surface-container-high");
    expect(input.className).toContain("custom-native-input");
  });
});
