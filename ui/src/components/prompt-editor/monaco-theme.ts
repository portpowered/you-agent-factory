// biome-ignore lint/style/noExcessiveLinesPerFile: Monaco theme derivation keeps palette token fallback precedence in one module.
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
  surface: "#181f2b",
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
  const templateDelimiter = base === "vs" ? tokens.accentStrong : tokens.accent;
  const templateKeyword = base === "vs" ? tokens.info : tokens.infoBright;

  return {
    base,
    colors: buildSharedEditorColors(tokens),
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: Palette token resolution keeps stylesheet, computed-style, and probe fallback precedence in one place.
function readMonacoThemeTokens(root: Element | null): MonacoThemeTokens {
  if (typeof window === "undefined" || !root) {
    return { ...FALLBACK_THEME_TOKENS };
  }

  const styles = window.getComputedStyle(root);
  const stylesheetTokens = readMonacoThemeStylesheetTokens(root);
  const probeTokens = readMonacoThemeProbeTokens(root);

  return {
    accent: readCssColor(
      styles,
      ["--color-primary", "--color-af-accent", "--color-af-foundation-accent"],
      pickUsableThemeColor(
        stylesheetTokens.accent,
        probeTokens.accent,
        FALLBACK_THEME_TOKENS.accent,
      ),
    ),
    accentStrong: readCssColor(
      styles,
      [
        "--color-on-primary-container",
        "--color-af-accent-hover",
        "--color-af-foundation-accent-strong",
      ],
      pickUsableThemeColor(
        stylesheetTokens.accentStrong,
        probeTokens.accentStrong,
        FALLBACK_THEME_TOKENS.accentStrong,
      ),
    ),
    code: readCssColor(
      styles,
      ["--color-code", "--color-af-code-ink", "--color-af-foundation-code-ink"],
      pickUsableThemeColor(
        stylesheetTokens.code,
        probeTokens.code,
        FALLBACK_THEME_TOKENS.code,
      ),
    ),
    dangerText: readCssColor(
      styles,
      [
        "--color-on-error-container",
        "--color-af-danger-text",
        "--color-af-foundation-danger-ink",
      ],
      pickUsableThemeColor(
        stylesheetTokens.dangerText,
        probeTokens.dangerText,
        FALLBACK_THEME_TOKENS.dangerText,
      ),
    ),
    info: readCssColor(
      styles,
      ["--color-info", "--color-af-info", "--color-af-foundation-info"],
      pickUsableThemeColor(
        stylesheetTokens.info,
        probeTokens.info,
        FALLBACK_THEME_TOKENS.info,
      ),
    ),
    infoBright: readCssColor(
      styles,
      ["--color-af-foundation-info-bright", "--color-info", "--color-af-info"],
      pickUsableThemeColor(
        stylesheetTokens.infoBright,
        probeTokens.infoBright,
        FALLBACK_THEME_TOKENS.infoBright,
      ),
    ),
    infoInk: readCssColor(
      styles,
      [
        "--color-on-info-container",
        "--color-af-info-text",
        "--color-af-foundation-info-ink",
      ],
      pickUsableThemeColor(
        stylesheetTokens.infoInk,
        probeTokens.infoInk,
        FALLBACK_THEME_TOKENS.infoInk,
      ),
    ),
    ink: readCssColor(
      styles,
      ["--color-on-surface", "--color-af-text", "--color-af-foundation-ink"],
      pickUsableThemeColor(
        stylesheetTokens.ink,
        probeTokens.ink,
        FALLBACK_THEME_TOKENS.ink,
      ),
    ),
    overlay: readCssColor(
      styles,
      ["--color-af-foundation-overlay"],
      pickUsableThemeColor(
        stylesheetTokens.overlay,
        probeTokens.overlay,
        FALLBACK_THEME_TOKENS.overlay,
      ),
    ),
    successText: readCssColor(
      styles,
      [
        "--color-on-success-container",
        "--color-af-success-text",
        "--color-af-foundation-success-ink",
      ],
      pickUsableThemeColor(
        stylesheetTokens.successText,
        probeTokens.successText,
        FALLBACK_THEME_TOKENS.successText,
      ),
    ),
    surface: readCssColor(
      styles,
      [
        "--color-surface",
        "--color-af-surface",
        "--color-af-foundation-surface",
      ],
      pickUsableThemeColor(
        stylesheetTokens.surface,
        probeTokens.surface,
        FALLBACK_THEME_TOKENS.surface,
      ),
    ),
  };
}

function readMonacoThemeStylesheetTokens(
  root: Element,
): Partial<MonacoThemeTokens> {
  const document = root.ownerDocument;
  const paletteID =
    document?.documentElement.getAttribute("data-color-palette") ??
    "factory-dark";
  if (!document) {
    return {};
  }

  const foundationVars = new Map<string, string>();
  const selectorsToMatch = new Set([
    ":root",
    `[data-color-palette=${paletteID}]`,
    `[data-color-palette="${paletteID}"]`,
  ]);

  for (const sheet of Array.from(document.styleSheets)) {
    let rules: CSSRuleList | undefined;
    try {
      rules = sheet.cssRules;
    } catch {
      continue;
    }

    for (const rule of Array.from(rules)) {
      if (!(rule instanceof CSSStyleRule)) {
        continue;
      }

      const selectorText = rule.selectorText ?? "";
      if (
        ![...selectorsToMatch].some((selector) =>
          selectorText.includes(selector),
        )
      ) {
        continue;
      }

      for (const name of [
        "--color-af-foundation-accent",
        "--color-af-foundation-accent-strong",
        "--color-af-foundation-code-ink",
        "--color-af-foundation-danger-ink",
        "--color-af-foundation-info",
        "--color-af-foundation-info-bright",
        "--color-af-foundation-info-ink",
        "--color-af-foundation-ink",
        "--color-af-foundation-overlay",
        "--color-af-foundation-success-ink",
        "--color-af-foundation-surface",
      ]) {
        const value = rule.style.getPropertyValue(name).trim();
        if (isUsableMonacoThemeColor(value)) {
          foundationVars.set(name, value);
        }
      }
    }
  }

  return {
    accent: foundationVars.get("--color-af-foundation-accent"),
    accentStrong: foundationVars.get("--color-af-foundation-accent-strong"),
    code: foundationVars.get("--color-af-foundation-code-ink"),
    dangerText: foundationVars.get("--color-af-foundation-danger-ink"),
    info: foundationVars.get("--color-af-foundation-info"),
    infoBright: foundationVars.get("--color-af-foundation-info-bright"),
    infoInk: foundationVars.get("--color-af-foundation-info-ink"),
    ink: foundationVars.get("--color-af-foundation-ink"),
    overlay: foundationVars.get("--color-af-foundation-overlay"),
    successText: foundationVars.get("--color-af-foundation-success-ink"),
    surface: foundationVars.get("--color-af-foundation-surface"),
  };
}

function readMonacoThemeProbeTokens(root: Element): Partial<MonacoThemeTokens> {
  const document = root.ownerDocument;
  const container = document?.body;
  if (!document || !container) {
    return {};
  }

  const probe = document.createElement("div");
  probe.setAttribute("aria-hidden", "true");
  probe.className =
    "pointer-events-none fixed left-0 top-0 grid gap-0 opacity-0";
  probe.innerHTML = `
    <div data-monaco-probe="surface" class="bg-surface text-on-surface border border-outline"></div>
    <div data-monaco-probe="accent" class="bg-primary text-on-primary-container"></div>
    <div data-monaco-probe="info" class="bg-info text-on-info-container"></div>
    <div data-monaco-probe="success" class="bg-success-container text-on-success-container"></div>
    <div data-monaco-probe="danger" class="bg-error-container text-on-error-container"></div>
    <div data-monaco-probe="code" class="text-code"></div>
  `;
  container.appendChild(probe);

  const read = (name: string) =>
    probe.querySelector<HTMLElement>(`[data-monaco-probe="${name}"]`);
  const surfaceElement = read("surface");
  const accentElement = read("accent");
  const infoElement = read("info");
  const successElement = read("success");
  const dangerElement = read("danger");
  const codeElement = read("code");
  const surface = surfaceElement
    ? window.getComputedStyle(surfaceElement).backgroundColor
    : undefined;
  const accent = accentElement
    ? window.getComputedStyle(accentElement).backgroundColor
    : undefined;
  const accentStrong = accentElement
    ? window.getComputedStyle(accentElement).color
    : undefined;
  const code = codeElement
    ? window.getComputedStyle(codeElement).color
    : undefined;
  const dangerText = dangerElement
    ? window.getComputedStyle(dangerElement).color
    : undefined;
  const info = infoElement
    ? window.getComputedStyle(infoElement).backgroundColor
    : undefined;
  const infoInk = infoElement
    ? window.getComputedStyle(infoElement).color
    : undefined;
  const successText = successElement
    ? window.getComputedStyle(successElement).color
    : undefined;

  probe.remove();

  return {
    accent,
    accentStrong,
    code,
    dangerText,
    info,
    infoBright: infoInk,
    infoInk,
    ink: surfaceElement
      ? window.getComputedStyle(surfaceElement).color
      : undefined,
    overlay:
      surface && parseColor(surface)
        ? relativeLuminance(parseColor(surface) as RGBColor) >= 0.5
          ? "#000000"
          : "#FFFFFF"
        : undefined,
    successText,
    surface,
  };
}

function readCssColor(
  styles: CSSStyleDeclaration,
  names: readonly string[],
  fallback: string,
): string {
  for (const name of names) {
    const value = styles.getPropertyValue(name).trim();
    if (isUsableMonacoThemeColor(value)) {
      return value;
    }
  }

  return fallback;
}

function isUsableMonacoThemeColor(color: string) {
  const normalized = color.trim().toLowerCase();
  if (normalized.length === 0 || normalized === "transparent") {
    return false;
  }

  const slashAlphaMatch = normalized.match(
    /^rgba?\(\s*[0-9.]+\s+[0-9.]+\s+[0-9.]+\s*\/\s*([0-9.]+)\s*\)$/i,
  );
  if (slashAlphaMatch && Number.parseFloat(slashAlphaMatch[1]) <= 0) {
    return false;
  }

  const commaAlphaMatch = normalized.match(
    /^rgba\(\s*[0-9.]+\s*,\s*[0-9.]+\s*,\s*[0-9.]+\s*,\s*([0-9.]+)\s*\)$/i,
  );
  if (commaAlphaMatch && Number.parseFloat(commaAlphaMatch[1]) <= 0) {
    return false;
  }

  return normalized.startsWith("#") || parseColor(normalized) !== null;
}

function pickUsableThemeColor(...candidates: Array<string | undefined>) {
  for (const candidate of candidates) {
    if (candidate && isUsableMonacoThemeColor(candidate)) {
      return candidate;
    }
  }

  return candidates[candidates.length - 1] ?? FALLBACK_THEME_TOKENS.surface;
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
