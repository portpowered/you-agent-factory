import { chooseFileShellClassName } from "./choose-file-shell";

describe("chooseFileShellClassName", () => {
  it("uses neutral dashed shell tokens without accent fill in idle state", () => {
    const className = chooseFileShellClassName();

    expect(className).toContain("border-dashed");
    expect(className).toContain("border-af-border-strong");
    expect(className).toContain("bg-af-surface-subtle");
    expect(className).not.toContain("bg-af-accent-surface");
    expect(className).not.toContain("border-af-accent-border");
  });

  it("uses neutral border and overlay emphasis when drag is active", () => {
    const className = chooseFileShellClassName({ dragActive: true });

    expect(className).toContain("border-af-border-strong");
    expect(className).toContain("bg-af-overlay");
    expect(className).not.toContain("bg-af-accent-surface");
    expect(className).not.toContain("border-af-accent-border");
  });
});
