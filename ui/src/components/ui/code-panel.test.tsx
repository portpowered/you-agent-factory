import { render, screen } from "@testing-library/react";

import { CodePanel, codePanelVariants } from "./code-panel";

describe("CodePanel", () => {
  it("renders compact high-surface code content by default", () => {
    render(<CodePanel>const value = 1;</CodePanel>);

    const codePanel = screen.getByText("const value = 1;");
    expect(codePanel.tagName).toBe("PRE");
    expect(codePanel.className).toContain("bg-surface-container-high");
    expect(codePanel.className).toContain("p-2");
    expect(codePanel.className).toContain("af-dashboard-body-code");
  });

  it("supports low-surface default-padding code panels", () => {
    render(
      <CodePanel padding="default" surface="low">
        {'{ "status": "ready" }'}
      </CodePanel>,
    );

    const codePanel = screen.getByText('{ "status": "ready" }');
    expect(codePanel.className).toContain("bg-surface-container-low");
    expect(codePanel.className).toContain("p-3");
  });

  it("exposes variant class generation for non-component consumers", () => {
    expect(
      codePanelVariants({
        className: "custom-code",
        padding: "default",
        surface: "low",
      }),
    ).toContain("custom-code");
  });
});
