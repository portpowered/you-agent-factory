import { afterEach, describe, expect, it } from "vitest";

import {
  buildWorkstationGuardSelectorTheme,
  buildWorkstationPromptTheme,
} from "./monaco-theme";

const PALETTE_FIXTURES = [
  {
    expectedBase: "vs-dark",
    id: "factory-dark",
    tokens: {
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
      "--color-af-foundation-surface": "#181F2B",
    },
  },
  {
    expectedBase: "vs",
    id: "factory-light",
    tokens: {
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
    },
  },
  {
    expectedBase: "vs-dark",
    id: "material-baseline",
    tokens: {
      "--color-af-foundation-accent": "#F5C76F",
      "--color-af-foundation-accent-strong": "#ECBF58",
      "--color-af-foundation-code-ink": "#C8F0FF",
      "--color-af-foundation-danger-ink": "#FFB8B8",
      "--color-af-foundation-info": "#67CBE0",
      "--color-af-foundation-info-bright": "#8ADCF0",
      "--color-af-foundation-info-ink": "#B8EDF8",
      "--color-af-foundation-ink": "#E6E0E9",
      "--color-af-foundation-overlay": "#FFFFFF",
      "--color-af-foundation-success-ink": "#A8F0C8",
      "--color-af-foundation-surface": "#1D1B20",
    },
  },
  {
    expectedBase: "vs-dark",
    id: "slate",
    tokens: {
      "--color-af-foundation-accent": "#F5C76F",
      "--color-af-foundation-accent-strong": "#ECBF58",
      "--color-af-foundation-code-ink": "#C7E7FF",
      "--color-af-foundation-danger-ink": "#FFB0B0",
      "--color-af-foundation-info": "#58B8D8",
      "--color-af-foundation-info-bright": "#7ECBF0",
      "--color-af-foundation-info-ink": "#B0E0F2",
      "--color-af-foundation-ink": "#E6EDF3",
      "--color-af-foundation-overlay": "#FFFFFF",
      "--color-af-foundation-success-ink": "#A4E8C4",
      "--color-af-foundation-surface": "#161B22",
    },
  },
  {
    expectedBase: "vs-dark",
    id: "olive",
    tokens: {
      "--color-af-foundation-accent": "#F5C76F",
      "--color-af-foundation-accent-strong": "#ECBF58",
      "--color-af-foundation-code-ink": "#D0F5E8",
      "--color-af-foundation-danger-ink": "#FFB0B0",
      "--color-af-foundation-info": "#58B0A0",
      "--color-af-foundation-info-bright": "#7ECFB8",
      "--color-af-foundation-info-ink": "#B0E8D8",
      "--color-af-foundation-ink": "#EEF2E4",
      "--color-af-foundation-overlay": "#FFFFFF",
      "--color-af-foundation-success-ink": "#A8E8BC",
      "--color-af-foundation-surface": "#1A1D15",
    },
  },
] as const;

describe("buildWorkstationPromptTheme", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("style");
  });

  it("derives the existing dark prompt theme colors from resolved tokens", () => {
    applyThemeTokens(PALETTE_FIXTURES[0].tokens);

    const theme = buildWorkstationPromptTheme();

    expect(theme.base).toBe("vs-dark");
    expect(theme.colors).toMatchObject({
      "editor.background": "#181F2B",
      "editor.foreground": "#F7F2E8",
      "editorCursor.foreground": "#F5C76F",
      "editorSuggestWidget.foreground": "#F7F2E8",
      "editorWidget.background": "#181F2B",
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

  it("uses stronger light-palette syntax colors for factory-light prompt tokens", () => {
    applyThemeTokens(PALETTE_FIXTURES[1].tokens);

    const theme = buildWorkstationPromptTheme();

    expect(theme.base).toBe("vs");
    expect(theme.colors).toMatchObject({
      "editor.background": "#FFFFFF",
      "editor.foreground": "#1A2228",
      "editorCursor.foreground": "#F5C76F",
      "editorWidget.background": "#FFFFFF",
    });
    expect(theme.colors["editor.background"]).not.toBe("#181F2B");
    expect(findRuleForeground(theme, "delimiter.template")).toBe("#C9972E");
    expect(findRuleForeground(theme, "keyword.template")).toBe("#2F8FAD");
    expect(findRuleForeground(theme, "string.template")).toBe("#1F6F49");
    expect(findRuleForeground(theme, "variable.root")).toBe("#0F4C5C");
  });

  it.each(PALETTE_FIXTURES)(
    "keeps prompt editor text and core syntax readable for %s",
    ({ expectedBase, id, tokens }) => {
      applyThemeTokens(tokens);

      const theme = buildWorkstationPromptTheme();
      const background =
        theme.colors["editor.background"] ??
        tokens["--color-af-foundation-surface"];

      expect(theme.base).toBe(expectedBase);
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
      expect(findRuleForeground(theme, "delimiter.template")).not.toBe(
        theme.colors["editor.foreground"],
      );
      expect(id).toBeTruthy();
    },
  );
});

describe("buildWorkstationGuardSelectorTheme", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("style");
  });

  it.each(PALETTE_FIXTURES)(
    "uses the same palette token source with readable guard-selector syntax for %s",
    ({ expectedBase, tokens }) => {
      applyThemeTokens(tokens);

      const theme = buildWorkstationGuardSelectorTheme();
      const background =
        theme.colors["editor.background"] ??
        tokens["--color-af-foundation-surface"];

      expect(theme.base).toBe(expectedBase);
      expect(findRuleForeground(theme, "text")).toBe(
        tokens["--color-af-foundation-ink"],
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
    },
  );
});

function applyThemeTokens(tokens: Record<string, string>) {
  for (const [name, value] of Object.entries(tokens)) {
    document.documentElement.style.setProperty(name, value);
  }
}

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
