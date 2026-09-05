import type { editor as MonacoEditorAPI } from "monaco-editor";

import type { ColorPaletteId } from "../../theme/color-palette";
import { getColorPaletteTokens } from "../../theme/color-palette-tokens";

type MonacoThemeTokens = {
  accent: string;
  accentStrong: string;
  code: string;
  dangerText: string;
  info: string;
  infoBright: string;
  infoInk: string;
  ink: string;
  onSurfaceVariant: string;
  overlay: string;
  outlineVariant: string;
  successText: string;
  surface: string;
};

export function buildWorkstationPromptTheme(
  paletteId: ColorPaletteId,
): MonacoEditorAPI.IStandaloneThemeData {
  const tokens = getMonacoThemeTokens(paletteId);
  const base = resolveMonacoBase(paletteId);
  const templateDelimiter = base === "vs" ? tokens.accentStrong : tokens.accent;
  const templateKeyword = base === "vs" ? tokens.info : tokens.infoBright;

  return {
    base,
    colors: buildSharedEditorColors(tokens, base),
    inherit: true,
    rules: [
      { foreground: toMonacoHex(tokens.ink), token: "text" },
      {
        fontStyle: "bold",
        foreground: toMonacoHex(templateDelimiter),
        token: "delimiter.template",
      },
      { foreground: toMonacoHex(templateKeyword), token: "keyword.template" },
      {
        foreground: toMonacoHex(tokens.infoInk),
        token: "keyword.function.template",
      },
      { foreground: toMonacoHex(tokens.ink), token: "identifier.template" },
      { foreground: toMonacoHex(tokens.successText), token: "string.template" },
      {
        foreground: toMonacoHex(tokens.accentStrong),
        token: "number.template",
      },
      { foreground: toMonacoHex(tokens.dangerText), token: "variable.local" },
      { foreground: toMonacoHex(tokens.code), token: "variable.root" },
    ],
  };
}

export function buildWorkstationGuardSelectorTheme(
  paletteId: ColorPaletteId,
): MonacoEditorAPI.IStandaloneThemeData {
  const tokens = getMonacoThemeTokens(paletteId);
  const base = resolveMonacoBase(paletteId);

  return {
    base,
    colors: buildSharedEditorColors(tokens, base),
    inherit: true,
    rules: [
      { foreground: toMonacoHex(tokens.ink), token: "text" },
      { foreground: toMonacoHex(tokens.code), token: "selector.field" },
      { foreground: toMonacoHex(tokens.successText), token: "selector.tag" },
    ],
  };
}

function getMonacoThemeTokens(paletteId: ColorPaletteId): MonacoThemeTokens {
  const tokens = getColorPaletteTokens(paletteId);

  return {
    accent: tokens.foundation.accent,
    accentStrong: tokens.foundation.accentStrong,
    code: tokens.foundation.codeInk,
    dangerText: tokens.foundation.dangerInk,
    info: tokens.foundation.info,
    infoBright: tokens.foundation.infoBright,
    infoInk: tokens.foundation.infoInk,
    ink: tokens.foundation.ink,
    onSurfaceVariant: tokens.semantic.foreground.muted,
    overlay: tokens.foundation.overlay,
    outlineVariant: tokens.semantic.outline.variant,
    successText: tokens.foundation.successInk,
    surface: tokens.foundation.surface,
  };
}

function buildSharedEditorColors(
  tokens: MonacoThemeTokens,
  base: MonacoEditorAPI.BuiltinTheme,
): MonacoEditorAPI.IColors {
  const selectionOpacity = base === "vs" ? 0.18 : 0.24;
  const inactiveSelectionOpacity = base === "vs" ? 0.12 : 0.16;
  const widgetSelectionOpacity = base === "vs" ? 0.12 : 0.18;

  return {
    "editor.background": tokens.surface,
    "editor.foreground": tokens.ink,
    "editorGutter.background": tokens.surface,
    "editorLineNumber.foreground": tokens.onSurfaceVariant,
    "editorLineNumber.activeForeground": tokens.ink,
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
    "scrollbarSlider.background": tokens.outlineVariant,
    "scrollbarSlider.hoverBackground": tokens.onSurfaceVariant,
    "scrollbarSlider.activeBackground": tokens.onSurfaceVariant,
  };
}

function resolveMonacoBase(
  paletteId: ColorPaletteId,
): MonacoEditorAPI.BuiltinTheme {
  return paletteId === "factory-light" ? "vs" : "vs-dark";
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

type RGBColor = {
  blue: number;
  green: number;
  red: number;
};

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
