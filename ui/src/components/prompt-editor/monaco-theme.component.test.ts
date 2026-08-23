import { describe, expect, it } from "vitest";

import { COLOR_PALETTE_IDS } from "../../theme/color-palette";
import { getColorPaletteTokens } from "../../theme/color-palette-tokens";
import {
  buildWorkstationGuardSelectorTheme,
  buildWorkstationPromptTheme,
} from "./monaco-theme";

describe("buildWorkstationPromptTheme", () => {
  it("derives the dark prompt theme from the generated factory-dark tokens", () => {
    const paletteId = "factory-dark";
    const tokens = getColorPaletteTokens(paletteId);
    const theme = buildWorkstationPromptTheme(paletteId);

    expect(theme.base).toBe("vs-dark");
    expect(theme.colors).toMatchObject({
      "editor.background": tokens.foundation.surface,
      "editor.foreground": tokens.foundation.ink,
      "editorGutter.background": tokens.foundation.surface,
      "editorLineNumber.foreground": tokens.semantic.foreground.muted,
      "editorLineNumber.activeForeground": tokens.foundation.ink,
      "editorCursor.foreground": tokens.foundation.accent,
      "editorSuggestWidget.foreground": tokens.foundation.ink,
      "editorWidget.background": tokens.foundation.surface,
    });
    expect(theme.rules).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          foreground: toMonacoHex(tokens.foundation.accent),
          token: "delimiter.template",
        }),
        expect.objectContaining({
          foreground: toMonacoHex(tokens.foundation.infoBright),
          token: "keyword.template",
        }),
        expect.objectContaining({
          foreground: toMonacoHex(tokens.foundation.codeInk),
          token: "variable.root",
        }),
      ]),
    );
  });

  it("uses the light Monaco base and stronger light syntax colors", () => {
    const paletteId = "factory-light";
    const tokens = getColorPaletteTokens(paletteId);
    const theme = buildWorkstationPromptTheme(paletteId);

    expect(theme.base).toBe("vs");
    expect(theme.colors).toMatchObject({
      "editor.background": tokens.foundation.surface,
      "editor.foreground": tokens.foundation.ink,
      "editorCursor.foreground": tokens.foundation.accent,
      "editorWidget.background": tokens.foundation.surface,
    });
    expect(findRuleForeground(theme, "delimiter.template")).toBe(
      `#${toMonacoHex(tokens.foundation.accentStrong)}`,
    );
    expect(findRuleForeground(theme, "keyword.template")).toBe(
      `#${toMonacoHex(tokens.foundation.info)}`,
    );
    expect(findRuleForeground(theme, "string.template")).toBe(
      `#${toMonacoHex(tokens.foundation.successInk)}`,
    );
    expect(findRuleForeground(theme, "variable.root")).toBe(
      `#${toMonacoHex(tokens.foundation.codeInk)}`,
    );
  });

  it.each(COLOR_PALETTE_IDS)(
    "keeps prompt editor text and core syntax readable for %s",
    (paletteId) => {
      const tokens = getColorPaletteTokens(paletteId);
      const theme = buildWorkstationPromptTheme(paletteId);
      const background = theme.colors["editor.background"];

      expect(theme.base).toBe(paletteId === "factory-light" ? "vs" : "vs-dark");
      expect(background).toBe(tokens.foundation.surface);
      expect(theme.colors["editorGutter.background"]).toBe(
        tokens.foundation.surface,
      );
      expect(theme.colors["editorLineNumber.foreground"]).toBe(
        tokens.semantic.foreground.muted,
      );
      expect(theme.colors["editorLineNumber.activeForeground"]).toBe(
        tokens.foundation.ink,
      );
      expect(
        readContrastRatio(theme.colors["editor.foreground"] ?? "", background),
      ).toBeGreaterThanOrEqual(4.5);
      expect(
        readContrastRatio(
          findRuleForeground(theme, "keyword.template"),
          background,
        ),
      ).toBeGreaterThanOrEqual(3);
      expect(
        readContrastRatio(
          findRuleForeground(theme, "string.template"),
          background,
        ),
      ).toBeGreaterThanOrEqual(3);
      expect(
        readContrastRatio(
          findRuleForeground(theme, "variable.root"),
          background,
        ),
      ).toBeGreaterThanOrEqual(3);
      expect(theme.colors["scrollbarSlider.background"]).toBe(
        tokens.semantic.outline.variant,
      );
      expect(theme.colors["scrollbarSlider.hoverBackground"]).toBe(
        tokens.semantic.foreground.muted,
      );
      expect(theme.colors["scrollbarSlider.activeBackground"]).toBe(
        tokens.semantic.foreground.muted,
      );
    },
  );
});

describe("buildWorkstationGuardSelectorTheme", () => {
  it.each(COLOR_PALETTE_IDS)(
    "uses generated palette tokens with readable guard-selector syntax for %s",
    (paletteId) => {
      const tokens = getColorPaletteTokens(paletteId);
      const theme = buildWorkstationGuardSelectorTheme(paletteId);
      const background = theme.colors["editor.background"];

      expect(theme.base).toBe(paletteId === "factory-light" ? "vs" : "vs-dark");
      expect(theme.colors["editorGutter.background"]).toBe(
        tokens.foundation.surface,
      );
      expect(theme.colors["editorLineNumber.foreground"]).toBe(
        tokens.semantic.foreground.muted,
      );
      expect(findRuleForeground(theme, "text")).toBe(
        `#${toMonacoHex(tokens.foundation.ink)}`,
      );
      expect(
        readContrastRatio(findRuleForeground(theme, "text"), background),
      ).toBeGreaterThanOrEqual(4.5);
      expect(
        readContrastRatio(
          findRuleForeground(theme, "selector.field"),
          background,
        ),
      ).toBeGreaterThanOrEqual(3);
      expect(
        readContrastRatio(
          findRuleForeground(theme, "selector.tag"),
          background,
        ),
      ).toBeGreaterThanOrEqual(3);
      expect(theme.colors["scrollbarSlider.background"]).toBe(
        tokens.semantic.outline.variant,
      );
      expect(theme.colors["scrollbarSlider.hoverBackground"]).toBe(
        tokens.semantic.foreground.muted,
      );
      expect(theme.colors["scrollbarSlider.activeBackground"]).toBe(
        tokens.semantic.foreground.muted,
      );
    },
  );
});

function findRuleForeground(
  theme: ReturnType<typeof buildWorkstationPromptTheme>,
  token: string,
): string {
  const foreground = theme.rules.find(
    (rule) => rule.token === token,
  )?.foreground;
  if (!foreground) {
    throw new Error(`expected Monaco rule foreground for token ${token}`);
  }

  return `#${foreground}`;
}

function toMonacoHex(color: string): string {
  return color.replace(/^#/, "").slice(0, 6).toUpperCase();
}

function readContrastRatio(foreground: string, background: string): number {
  const foregroundLuminance = readRelativeLuminance(foreground);
  const backgroundLuminance = readRelativeLuminance(background);
  const lighter = Math.max(foregroundLuminance, backgroundLuminance);
  const darker = Math.min(foregroundLuminance, backgroundLuminance);

  return (lighter + 0.05) / (darker + 0.05);
}

function readRelativeLuminance(color: string): number {
  const normalized = color.replace(/^#/, "");
  const rgb = [
    Number.parseInt(normalized.slice(0, 2), 16),
    Number.parseInt(normalized.slice(2, 4), 16),
    Number.parseInt(normalized.slice(4, 6), 16),
  ];

  const [red, green, blue] = rgb.map((channel) => {
    const normalizedChannel = channel / 255;
    if (normalizedChannel <= 0.03928) {
      return normalizedChannel / 12.92;
    }

    return ((normalizedChannel + 0.055) / 1.055) ** 2.4;
  });

  return 0.2126 * red + 0.7152 * green + 0.0722 * blue;
}
