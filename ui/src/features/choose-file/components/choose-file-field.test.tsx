import { render, screen } from "@testing-library/react";

import { ChooseFileField } from "./choose-file-field";

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
    expect(shell.className).toContain("border-af-border-strong");
    expect(shell.className).toContain("bg-af-surface-subtle");
    expect(shell.className).not.toContain("bg-af-accent-surface");
    expect(shell.className).not.toContain("border-af-accent-border");
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
    expect(shell.className).toContain("bg-af-surface-subtle");
    expect(shell.className).not.toContain("bg-af-accent-surface");
    expect(shell.className).not.toContain("border-af-accent-border");

    rerender(
      <ChooseFileField
        control={<div data-testid="choose-file-shell">Drop files here</div>}
        dragActive
      />,
    );

    const activeShell = screen.getByTestId("choose-file-shell");
    expect(activeShell.className).toContain("border-af-border-strong");
    expect(activeShell.className).toContain("bg-af-overlay");
    expect(activeShell.className).not.toContain("bg-af-accent-surface");
    expect(activeShell.className).not.toContain("border-af-accent-border");
  });
});
