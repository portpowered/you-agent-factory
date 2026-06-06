import { afterEach, describe, expect, it } from "vitest";

import {
  buildWorkstationGuardSelectorTheme,
  buildWorkstationPromptTheme,
} from "./monaco-theme";

describe("buildWorkstationPromptTheme", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("style");
  });

  it("derives the existing dark prompt theme colors from resolved tokens", () => {
    applyThemeTokens({
      "--color-af-foundation-accent": "#F5C76F",
      "--color-af-foundation-accent-strong": "#ECBF58",
      "--color-af-foundation-code-ink": "#5CCADD",
      "--color-af-foundation-danger-ink": "#FFB2B2",
      "--color-af-foundation-info": "#5CCADD",
      "--color-af-foundation-info-bright": "#7DD3FC",
      "--color-af-foundation-info-ink": "#B5EDF4",
      "--color-af-foundation-ink": "#F7F2E8",
      "--color-af-foundation-overlay": "#FFFFFF",
      "--color-af-foundation-success-ink": "#A7F0C4",
      "--color-af-foundation-surface": "#091117",
    });

    const theme = buildWorkstationPromptTheme();

    expect(theme.base).toBe("vs-dark");
    expect(theme.colors).toMatchObject({
      "editor.background": "#091117",
      "editor.foreground": "#F7F2E8",
      "editorCursor.foreground": "#F5C76F",
      "editorSuggestWidget.foreground": "#F7F2E8",
      "editorWidget.background": "#091117",
    });
    expect(theme.rules).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          foreground: "F5C76F",
          token: "delimiter.template",
        }),
        expect.objectContaining({
          foreground: "7DD3FC",
          token: "keyword.template",
        }),
        expect.objectContaining({
          foreground: "5CCADD",
          token: "variable.root",
        }),
      ]),
    );
  });

  it("switches to a light Monaco base and light-surface tokens for factory-light palettes", () => {
    applyThemeTokens({
      "--color-af-foundation-accent": "#F5C76F",
      "--color-af-foundation-accent-strong": "#C9972E",
      "--color-af-foundation-code-ink": "#0F4C5C",
      "--color-af-foundation-danger-ink": "#9F2F2F",
      "--color-af-foundation-info": "#2F8FAD",
      "--color-af-foundation-info-bright": "#4AA9C9",
      "--color-af-foundation-info-ink": "#1F6178",
      "--color-af-foundation-ink": "#1A2228",
      "--color-af-foundation-overlay": "#000000",
      "--color-af-foundation-success-ink": "#1F6F49",
      "--color-af-foundation-surface": "#FFFFFF",
    });

    const theme = buildWorkstationPromptTheme();

    expect(theme.base).toBe("vs");
    expect(theme.colors).toMatchObject({
      "editor.background": "#FFFFFF",
      "editor.foreground": "#1A2228",
      "editorCursor.foreground": "#F5C76F",
      "editorWidget.background": "#FFFFFF",
    });
    expect(theme.colors["editor.background"]).not.toBe("#091117");
    expect(theme.rules).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          foreground: "1F6F49",
          token: "string.template",
        }),
        expect.objectContaining({
          foreground: "0F4C5C",
          token: "variable.root",
        }),
      ]),
    );
  });
});

describe("buildWorkstationGuardSelectorTheme", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("style");
  });

  it("uses the same palette token source with guard-selector syntax rules", () => {
    applyThemeTokens({
      "--color-af-foundation-code-ink": "#0F4C5C",
      "--color-af-foundation-ink": "#1A2228",
      "--color-af-foundation-overlay": "#000000",
      "--color-af-foundation-success-ink": "#1F6F49",
      "--color-af-foundation-surface": "#FFFFFF",
    });

    const theme = buildWorkstationGuardSelectorTheme();

    expect(theme.base).toBe("vs");
    expect(theme.rules).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          foreground: "1A2228",
          token: "text",
        }),
        expect.objectContaining({
          foreground: "0F4C5C",
          token: "selector.field",
        }),
        expect.objectContaining({
          foreground: "1F6F49",
          token: "selector.tag",
        }),
      ]),
    );
  });
});

function applyThemeTokens(tokens: Record<string, string>) {
  for (const [name, value] of Object.entries(tokens)) {
    document.documentElement.style.setProperty(name, value);
  }
}
