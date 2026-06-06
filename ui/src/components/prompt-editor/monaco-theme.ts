import type { editor as MonacoEditorAPI } from "monaco-editor";

const FALLBACK_THEME_TOKENS = {
  accent: "#F5C76F",
  accentStrong: "#ECBF58",
  code: "#D5FBFF",
  dangerText: "#FFB2B2",
  info: "#5CCADD",
  infoBright: "#7DD3FC",
  infoInk: "#B5EDF4",
  ink: "#F7F2E8",
  overlay: "#FFFFFF",
  successText: "#A7F0C4",
  surface: "#091117",
} as const;

type RGBColor = {
  blue: number;
  green: number;
  red: number;
};

type MonacoThemeTokens = {
  accent: string;
  accentStrong: string;
  code: string;
  dangerText: string;
  info: string;
  infoBright: string;
  infoInk: string;
  ink: string;
  overlay: string;
  successText: string;
  surface: string;
};

export function buildWorkstationPromptTheme(
  root: Element | null = document.documentElement,
): MonacoEditorAPI.IStandaloneThemeData {
  const tokens = readMonacoThemeTokens(root);
  const base = resolveMonacoBase(tokens.surface);

  return {
    base,
    colors: buildSharedEditorColors(tokens),
    inherit: true,
    rules: [
      { foreground: toMonacoHex(tokens.ink), token: "text" },
      {
        fontStyle: "bold",
        foreground: toMonacoHex(tokens.accent),
        token: "delimiter.template",
      },
      { foreground: toMonacoHex(tokens.infoBright), token: "keyword.template" },
      {
        foreground: toMonacoHex(tokens.infoInk),
        token: "keyword.function.template",
      },
      { foreground: toMonacoHex(tokens.ink), token: "identifier.template" },
      { foreground: toMonacoHex(tokens.successText), token: "string.template" },
      { foreground: toMonacoHex(tokens.accentStrong), token: "number.template" },
      { foreground: toMonacoHex(tokens.dangerText), token: "variable.local" },
      { foreground: toMonacoHex(tokens.code), token: "variable.root" },
    ],
  };
}

export function buildWorkstationGuardSelectorTheme(
  root: Element | null = document.documentElement,
): MonacoEditorAPI.IStandaloneThemeData {
  const tokens = readMonacoThemeTokens(root);
  const base = resolveMonacoBase(tokens.surface);

  return {
    base,
    colors: buildSharedEditorColors(tokens),
    inherit: true,
    rules: [
      { foreground: toMonacoHex(tokens.ink), token: "text" },
      { foreground: toMonacoHex(tokens.code), token: "selector.field" },
      { foreground: toMonacoHex(tokens.successText), token: "selector.tag" },
    ],
  };
}

function buildSharedEditorColors(
  tokens: MonacoThemeTokens,
): MonacoEditorAPI.IColors {
  const base = resolveMonacoBase(tokens.surface);
  const selectionOpacity = base === "vs" ? 0.18 : 0.24;
  const inactiveSelectionOpacity = base === "vs" ? 0.12 : 0.16;
  const widgetSelectionOpacity = base === "vs" ? 0.12 : 0.18;

  return {
    "editor.background": tokens.surface,
    "editor.foreground": tokens.ink,
    "editor.lineHighlightBackground": withAlpha(tokens.overlay, 0.04),
    "editor.selectionBackground": withAlpha(tokens.info, selectionOpacity),
    "editor.inactiveSelectionBackground": withAlpha(
      tokens.info,
      inactiveSelectionOpacity,
    ),
    "editorCursor.foreground": tokens.accent,
    "editorWhitespace.foreground": withAlpha(tokens.overlay, 0.14),
    "editorIndentGuide.background1": withAlpha(tokens.overlay, 0.08),
    "editorIndentGuide.activeBackground1": withAlpha(tokens.overlay, 0.18),
    "editorWidget.background": tokens.surface,
    "editorWidget.border": withAlpha(tokens.overlay, 0.12),
    "editorSuggestWidget.background": tokens.surface,
    "editorSuggestWidget.foreground": tokens.ink,
    "editorSuggestWidget.selectedBackground": withAlpha(
      tokens.info,
      widgetSelectionOpacity,
    ),
    "editorSuggestWidget.highlightForeground": tokens.accent,
    "editorHoverWidget.background": tokens.surface,
    "editorHoverWidget.border": withAlpha(tokens.overlay, 0.12),
  };
}

function readMonacoThemeTokens(root: Element | null): MonacoThemeTokens {
  if (typeof window === "undefined" || !root) {
    return { ...FALLBACK_THEME_TOKENS };
  }

  const styles = window.getComputedStyle(root);

  return {
    accent: readCssColor(styles, [
      "--color-primary",
      "--color-af-accent",
      "--color-af-foundation-accent",
    ], FALLBACK_THEME_TOKENS.accent),
    accentStrong: readCssColor(styles, [
      "--color-on-primary-container",
      "--color-af-accent-hover",
      "--color-af-foundation-accent-strong",
    ], FALLBACK_THEME_TOKENS.accentStrong),
    code: readCssColor(styles, [
      "--color-code",
      "--color-af-code-ink",
      "--color-af-foundation-code-ink",
    ], FALLBACK_THEME_TOKENS.code),
    dangerText: readCssColor(styles, [
      "--color-on-error-container",
      "--color-af-danger-text",
      "--color-af-foundation-danger-ink",
    ], FALLBACK_THEME_TOKENS.dangerText),
    info: readCssColor(styles, [
      "--color-info",
      "--color-af-info",
      "--color-af-foundation-info",
    ], FALLBACK_THEME_TOKENS.info),
    infoBright: readCssColor(styles, [
      "--color-af-foundation-info-bright",
      "--color-info",
      "--color-af-info",
    ], FALLBACK_THEME_TOKENS.infoBright),
    infoInk: readCssColor(styles, [
      "--color-on-info-container",
      "--color-af-info-text",
      "--color-af-foundation-info-ink",
    ], FALLBACK_THEME_TOKENS.infoInk),
    ink: readCssColor(styles, [
      "--color-on-surface",
      "--color-af-text",
      "--color-af-foundation-ink",
    ], FALLBACK_THEME_TOKENS.ink),
    overlay: readCssColor(styles, [
      "--color-af-foundation-overlay",
    ], FALLBACK_THEME_TOKENS.overlay),
    successText: readCssColor(styles, [
      "--color-on-success-container",
      "--color-af-success-text",
      "--color-af-foundation-success-ink",
    ], FALLBACK_THEME_TOKENS.successText),
    surface: readCssColor(styles, [
      "--color-surface",
      "--color-af-surface",
      "--color-af-foundation-surface",
    ], FALLBACK_THEME_TOKENS.surface),
  };
}

function readCssColor(
  styles: CSSStyleDeclaration,
  names: readonly string[],
  fallback: string,
): string {
  for (const name of names) {
    const value = styles.getPropertyValue(name).trim();
    if (value.length > 0) {
      return value;
    }
  }

  return fallback;
}

function resolveMonacoBase(
  backgroundColor: string,
): MonacoEditorAPI.BuiltinTheme {
  const rgb = parseColor(backgroundColor);
  if (!rgb) {
    return "vs-dark";
  }

  return relativeLuminance(rgb) >= 0.5 ? "vs" : "vs-dark";
}

function relativeLuminance({ blue, green, red }: RGBColor) {
  const [r, g, b] = [red, green, blue].map((channel) => {
    const normalized = channel / 255;
    if (normalized <= 0.03928) {
      return normalized / 12.92;
    }

    return ((normalized + 0.055) / 1.055) ** 2.4;
  });

  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function withAlpha(color: string, alpha: number) {
  const rgb = parseColor(color);
  if (!rgb) {
    return color;
  }

  return `#${toMonacoHex(color)}${toAlphaHex(alpha)}`;
}

function toMonacoHex(color: string) {
  const rgb = parseColor(color);
  if (!rgb) {
    return color.replace(/^#/, "").toUpperCase();
  }

  return [rgb.red, rgb.green, rgb.blue]
    .map((channel) => channel.toString(16).padStart(2, "0").toUpperCase())
    .join("");
}

function parseColor(color: string): RGBColor | null {
  const normalized = color.trim();
  if (normalized.startsWith("#")) {
    return parseHexColor(normalized);
  }

  const rgbMatch = normalized.match(
    /^rgba?\(\s*([0-9.]+)\s+([0-9.]+)\s+([0-9.]+)(?:\s*\/\s*[0-9.]+)?\s*\)$/i,
  );
  if (rgbMatch) {
    return {
      blue: Number.parseFloat(rgbMatch[3]),
      green: Number.parseFloat(rgbMatch[2]),
      red: Number.parseFloat(rgbMatch[1]),
    };
  }

  const commaSeparatedRgbMatch = normalized.match(
    /^rgba?\(\s*([0-9.]+)\s*,\s*([0-9.]+)\s*,\s*([0-9.]+)(?:\s*,\s*[0-9.]+)?\s*\)$/i,
  );
  if (commaSeparatedRgbMatch) {
    return {
      blue: Number.parseFloat(commaSeparatedRgbMatch[3]),
      green: Number.parseFloat(commaSeparatedRgbMatch[2]),
      red: Number.parseFloat(commaSeparatedRgbMatch[1]),
    };
  }

  return null;
}

function parseHexColor(color: string): RGBColor | null {
  const hex = color.slice(1);
  if (hex.length === 3) {
    return {
      blue: Number.parseInt(hex[2] + hex[2], 16),
      green: Number.parseInt(hex[1] + hex[1], 16),
      red: Number.parseInt(hex[0] + hex[0], 16),
    };
  }

  if (hex.length === 6 || hex.length === 8) {
    return {
      blue: Number.parseInt(hex.slice(4, 6), 16),
      green: Number.parseInt(hex.slice(2, 4), 16),
      red: Number.parseInt(hex.slice(0, 2), 16),
    };
  }

  return null;
}

function toAlphaHex(alpha: number) {
  return Math.round(Math.min(Math.max(alpha, 0), 1) * 255)
    .toString(16)
    .padStart(2, "0")
    .toUpperCase();
}
