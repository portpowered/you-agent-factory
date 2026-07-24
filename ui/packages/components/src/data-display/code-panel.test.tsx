// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen, userEvent } from "../testing/render";
import { CodePanel, codePanelVariants } from "./code-panel";

const LONG_SINGLE_LINE =
  "const payload = 'abcdefghijklmnopqrstuvwxyz0123456789-'.repeat(24);";

const LONG_MULTI_LINE = Array.from(
  { length: 20 },
  (_, index) => `line ${index + 1}: repeated worker output context`,
).join("\n");

describe("CodePanel", () => {
  it("renders compact high-surface code content by default", () => {
    renderPackageComponent(<CodePanel>const value = 1;</CodePanel>);

    const codePanel = screen.getByText("const value = 1;");
    expect(codePanel.tagName).toBe("PRE");
    expect(codePanel.className).toContain("bg-surface-container-high");
    expect(codePanel.className).toContain("p-2");
    expect(codePanel.className).toContain("text-code-medium");
    expect(codePanel.className).toContain("overflow-x-auto");
    expect(codePanel.className).toContain("min-w-0");
    expect(codePanel.className).toContain("[overflow-wrap:anywhere]");
  });

  it("supports low-surface default-padding code panels", () => {
    renderPackageComponent(
      <CodePanel padding="default" surface="low">
        {'{ "status": "ready" }'}
      </CodePanel>,
    );

    const codePanel = screen.getByText('{ "status": "ready" }');
    expect(codePanel.className).toContain("bg-surface-container-low");
    expect(codePanel.className).toContain("p-3");
  });

  it("contains long single-line code without page-level overflow classes", () => {
    renderPackageComponent(
      <div className="grid w-64">
        <CodePanel>{LONG_SINGLE_LINE}</CodePanel>
      </div>,
    );

    const codePanel = screen.getByText(LONG_SINGLE_LINE, { exact: false });
    expect(codePanel.className).toContain("max-w-full");
    expect(codePanel.className).toContain("w-full");
    expect(codePanel.className).toContain("overflow-x-auto");
    expect(codePanel.className).toContain("[overflow-wrap:anywhere]");
  });

  it("scrolls long multi-line code inside a bounded panel", () => {
    renderPackageComponent(
      <CodePanel maxHeight="md">{LONG_MULTI_LINE}</CodePanel>,
    );

    const codePanel = screen.getByText(
      /line 1: repeated worker output context/,
    );
    expect(codePanel.tagName).toBe("PRE");
    expect(codePanel.className).toContain("max-h-72");
    expect(codePanel.className).toContain("overflow-y-auto");
    expect(codePanel).toHaveAttribute("tabindex", "0");
  });

  it("keeps header controls reachable when code content is long", async () => {
    const user = userEvent.setup();

    renderPackageComponent(
      <div className="grid w-64 gap-2">
        <div className="flex min-w-0 items-center justify-between gap-2">
          <span>Generated script</span>
          <button type="button">Copy</button>
        </div>
        <CodePanel maxHeight="sm">{LONG_MULTI_LINE}</CodePanel>
      </div>,
    );

    const copyButton = screen.getByRole("button", { name: "Copy" });
    await user.tab();
    expect(copyButton).toHaveFocus();

    const codePanel = screen.getByText(
      /line 1: repeated worker output context/,
    );
    await user.tab();
    expect(codePanel).toHaveFocus();
    expect(codePanel.className).toContain("focus-visible:outline");
  });

  it("exposes variant class generation for non-component consumers", () => {
    expect(
      codePanelVariants({
        className: "custom-code",
        maxHeight: "lg",
        padding: "default",
        surface: "low",
      }),
    ).toContain("custom-code");
    expect(
      codePanelVariants({
        maxHeight: "lg",
        padding: "default",
        surface: "low",
      }),
    ).toContain("max-h-96");
  });
});
