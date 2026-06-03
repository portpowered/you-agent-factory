import { chooseFileShellClassName } from "./choose-file-shell";

describe("chooseFileShellClassName", () => {
  it("uses neutral dashed shell tokens without accent fill in idle state", () => {
    const className = chooseFileShellClassName();

    expect(className).toContain("border-dashed");
    expect(className).toContain("border-outline-variant");
    expect(className).toContain("bg-surface-container-low");
    expect(className).not.toContain("bg-primary-container");
    expect(className).not.toContain("border-primary");
  });

  it("uses neutral border and overlay emphasis when drag is active", () => {
    const className = chooseFileShellClassName({ dragActive: true });

    expect(className).toContain("border-outline-variant");
    expect(className).toContain("bg-af-overlay");
    expect(className).not.toContain("bg-primary-container");
    expect(className).not.toContain("border-primary");
  });
});
