import {
  COLOR_PALETTE_IDS,
  type ColorPaletteId,
} from "../src/theme/color-palette";
import type {
  ColorPaletteFoundationTokens,
  ColorPaletteSemanticTokens,
  ColorPaletteTokens,
} from "../src/theme/color-palette-token-types";

type CssDeclarations = Map<string, string>;

type ColorPaletteTokenSpec =
  | string
  | {
      readonly [key: string]: ColorPaletteTokenSpec;
    };

export type ColorPaletteTokenGraph = Readonly<
  Record<ColorPaletteId, ColorPaletteTokens>
>;

const FOUNDATION_TOKEN_SPEC = {
  accent: "--color-af-foundation-accent",
  accentInk: "--color-af-foundation-accent-ink",
  accentStrong: "--color-af-foundation-accent-strong",
  background: "--color-af-foundation-background",
  backgroundMid: "--color-af-foundation-background-mid",
  backgroundStart: "--color-af-foundation-background-start",
  canvas: "--color-af-foundation-canvas",
  codeInk: "--color-af-foundation-code-ink",
  danger: "--color-af-foundation-danger",
  dangerBright: "--color-af-foundation-danger-bright",
  dangerInk: "--color-af-foundation-danger-ink",
  info: "--color-af-foundation-info",
  infoBright: "--color-af-foundation-info-bright",
  infoInk: "--color-af-foundation-info-ink",
  infoStrong: "--color-af-foundation-info-strong",
  ink: "--color-af-foundation-ink",
  overlay: "--color-af-foundation-overlay",
  secondaryAccent: "--color-af-foundation-secondary-accent",
  secondaryAccentInk: "--color-af-foundation-secondary-accent-ink",
  shadow: "--color-af-shadow",
  success: "--color-af-foundation-success",
  successInk: "--color-af-foundation-success-ink",
  surface: "--color-af-foundation-surface",
  tertiaryAccent: "--color-af-foundation-tertiary-accent",
  tertiaryAccentInk: "--color-af-foundation-tertiary-accent-ink",
  warning: "--color-af-foundation-warning",
  warningInk: "--color-af-foundation-warning-ink",
  worker: "--color-af-foundation-worker",
  workerInk: "--color-af-foundation-worker-ink",
} as const satisfies ColorPaletteTokenSpec;

const SEMANTIC_TOKEN_SPEC = {
  accent: {
    default: "--color-primary",
    ink: "--color-on-primary",
    strong: "--color-on-primary-container",
    surface: "--color-primary-container",
  },
  background: "--color-background",
  code: "--color-code",
  danger: {
    bright: "--color-af-foundation-danger-bright",
    default: "--color-error",
    ink: "--color-on-error-container",
    on: "--color-on-error",
    surface: "--color-error-container",
  },
  foreground: {
    default: "--color-on-surface",
    disabled: "--color-on-surface-disabled",
    inverse: "--color-on-inverse",
    muted: "--color-on-surface-variant",
    subtle: "--color-on-surface-subtle",
  },
  info: {
    bright: "--color-af-foundation-info-bright",
    default: "--color-info",
    ink: "--color-on-info-container",
    on: "--color-on-info",
    strong: "--color-af-foundation-info-strong",
    surface: "--color-info-container",
  },
  outline: {
    default: "--color-outline",
    variant: "--color-outline-variant",
  },
  overlay: {
    default: "--color-af-overlay",
    solid: "--color-af-foundation-overlay",
  },
  secondary: {
    default: "--color-secondary",
    ink: "--color-on-secondary",
    strong: "--color-on-secondary-container",
    surface: "--color-secondary-container",
  },
  success: {
    default: "--color-success",
    ink: "--color-on-success-container",
    on: "--color-on-success",
    surface: "--color-success-container",
  },
  surface: {
    container: "--color-surface-container",
    containerHigh: "--color-surface-container-high",
    containerHighest: "--color-surface-container-highest",
    containerLow: "--color-surface-container-low",
    default: "--color-surface",
    muted: "--color-surface-container-low",
    raised: "--color-surface-container-high",
  },
  tertiary: {
    default: "--color-tertiary",
    ink: "--color-on-tertiary-container",
    on: "--color-on-tertiary",
    surface: "--color-tertiary-container",
  },
  warning: {
    default: "--color-warning",
    ink: "--color-on-warning-container",
    on: "--color-on-warning",
    surface: "--color-warning-container",
  },
  worker: {
    default: "--color-tertiary",
    ink: "--color-on-tertiary-container",
    surface: "--color-tertiary-container",
  },
} as const satisfies ColorPaletteTokenSpec;

interface RGBColor {
  readonly alpha: number;
  readonly blue: number;
  readonly green: number;
  readonly red: number;
}

export function generateColorPaletteTokens(
  compiledCss: string,
): ColorPaletteTokenGraph {
  const baseDeclarations = extractThemeDeclarations(compiledCss);
  const graph = {} as Record<ColorPaletteId, ColorPaletteTokens>;

  for (const paletteId of COLOR_PALETTE_IDS) {
    const paletteBlock = findPaletteBlock(compiledCss, paletteId);
    if (!paletteBlock) {
      throw new Error(
        `Color palette token drift: missing preset block for palette=${paletteId}`,
      );
    }

    const declarations = new Map(baseDeclarations);
    for (const [name, value] of parseDeclarations(paletteBlock)) {
      declarations.set(name, value);
    }

    graph[paletteId] = {
      foundation: resolveTokenSpec(
        FOUNDATION_TOKEN_SPEC,
        declarations,
        paletteId,
      ) as ColorPaletteFoundationTokens,
      semantic: resolveTokenSpec(
        SEMANTIC_TOKEN_SPEC,
        declarations,
        paletteId,
      ) as ColorPaletteSemanticTokens,
    };
  }

  return graph;
}

export function getPaletteTokenExportName(paletteId: ColorPaletteId): string {
  return `${paletteId.replace(/(^|-)([a-z])/g, (_, _separator, letter) =>
    letter.toUpperCase(),
  )}ColorPaletteTokens`;
}

export function renderColorPaletteTokens(): string {
  const imports = [...COLOR_PALETTE_IDS]
    .sort()
    .map((paletteId) => {
      const exportName = getPaletteTokenExportName(paletteId);
      return `import { ${exportName} } from "./generated/color-palette-tokens-${paletteId}";`;
    })
    .join("\n");
  const entries = COLOR_PALETTE_IDS.map(
    (paletteId) =>
      `  ${renderObjectKey(paletteId)}: ${getPaletteTokenExportName(paletteId)},`,
  ).join("\n");

  return `/**
 * Generated by scripts/generate-color-palette-tokens.ts from the compiled
 * dashboard CSS. Do not edit this file directly.
 */
import type { ColorPaletteId } from "./color-palette";
import type { ColorPaletteTokens } from "./color-palette-token-types";
${imports}

export type {
  ColorPaletteFoundationTokens,
  ColorPaletteSemanticTokens,
  ColorPaletteTokens,
} from "./color-palette-token-types";

export const COLOR_PALETTE_TOKENS = {
${entries}
} satisfies Readonly<Record<ColorPaletteId, ColorPaletteTokens>>;

export function getColorPaletteTokens(
  paletteId: ColorPaletteId,
): ColorPaletteTokens {
  return COLOR_PALETTE_TOKENS[paletteId];
}
`;
}

export function renderColorPaletteTokenModule(
  paletteId: ColorPaletteId,
  tokens: ColorPaletteTokens,
): string {
  return `/**
 * Generated by scripts/generate-color-palette-tokens.ts from the compiled
 * dashboard CSS. Do not edit this file directly.
 */
import type { ColorPaletteTokens } from "../color-palette-token-types";

export const ${getPaletteTokenExportName(paletteId)}: ColorPaletteTokens = ${renderObject(tokens, 0)};
`;
}

function renderObject(value: object, indent: number): string {
  const entries = Object.entries(value).map(([key, childValue]) => {
    const renderedValue =
      typeof childValue === "string"
        ? JSON.stringify(childValue)
        : renderObject(childValue as object, indent + 2);
    return `${" ".repeat(indent + 2)}${renderObjectKey(key)}: ${renderedValue},`;
  });
  return `{
${entries.join("\n")}
${" ".repeat(indent)}}`;
}

function renderObjectKey(key: string): string {
  return /^[A-Za-z_$][A-Za-z0-9_$]*$/.test(key) ? key : JSON.stringify(key);
}

function extractThemeDeclarations(compiledCss: string): CssDeclarations {
  const layerStart = compiledCss.search(/@layer\s+theme\s*\{/);
  if (layerStart < 0) {
    throw new Error(
      "Color palette token drift: compiled CSS has no theme layer",
    );
  }

  const layer = extractBlock(compiledCss, compiledCss.indexOf("{", layerStart));
  const rootStart = layer.search(/:root\s*,\s*:host\s*\{/);
  if (rootStart < 0) {
    throw new Error(
      "Color palette token drift: compiled CSS has no theme root block",
    );
  }

  return parseDeclarations(extractBlock(layer, layer.indexOf("{", rootStart)));
}

function findPaletteBlock(
  compiledCss: string,
  paletteId: ColorPaletteId,
): string | undefined {
  const escapedPaletteId = paletteId.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const selector = new RegExp(
    `(?:^|\\n)\\s*(?::root\\s*,\\s*)?\\[data-color-palette="${escapedPaletteId}"\\]\\s*\\{`,
    "g",
  );
  const match = selector.exec(compiledCss);
  if (!match || match.index === undefined) {
    return undefined;
  }

  const openBraceIndex = compiledCss.indexOf("{", match.index);
  return extractBlock(compiledCss, openBraceIndex);
}

function parseDeclarations(block: string): CssDeclarations {
  const declarations = new Map<string, string>();
  const declarationPattern = /(--[\w-]+)\s*:\s*([^;{}]+)\s*;/g;
  for (const match of block.matchAll(declarationPattern)) {
    const name = match[1];
    const value = match[2];
    if (name && value) {
      declarations.set(name, value.trim());
    }
  }
  return declarations;
}

function extractBlock(source: string, openBraceIndex: number): string {
  if (openBraceIndex < 0) {
    return "";
  }

  let depth = 0;
  for (let index = openBraceIndex; index < source.length; index += 1) {
    const character = source[index];
    if (character === "{") {
      depth += 1;
    } else if (character === "}") {
      depth -= 1;
      if (depth === 0) {
        return source.slice(openBraceIndex + 1, index);
      }
    }
  }

  throw new Error(
    "Color palette token drift: compiled CSS has an unclosed block",
  );
}

function resolveTokenSpec(
  spec: ColorPaletteTokenSpec,
  declarations: CssDeclarations,
  paletteId: ColorPaletteId,
): unknown {
  if (typeof spec === "string") {
    return formatColor(
      resolveColor(spec, declarations, paletteId, new Set<string>()),
    );
  }

  return Object.fromEntries(
    Object.entries(spec).map(([key, childSpec]) => [
      key,
      resolveTokenSpec(childSpec, declarations, paletteId),
    ]),
  );
}

function resolveColor(
  tokenName: string,
  declarations: CssDeclarations,
  paletteId: ColorPaletteId,
  resolving: Set<string>,
): RGBColor {
  if (resolving.has(tokenName)) {
    throw new Error(
      `Color palette token drift: palette=${paletteId} has a cycle at token=${tokenName}`,
    );
  }

  const rawValue = declarations.get(tokenName);
  if (!rawValue) {
    throw new Error(
      `Color palette token drift: palette=${paletteId} missing compiled CSS token ${tokenName}`,
    );
  }

  const value = rawValue.replace(/\s+/g, " ").trim();
  const nextResolving = new Set(resolving).add(tokenName);
  const variableMatch = value.match(/^var\(\s*(--[\w-]+)\s*\)$/i);
  if (variableMatch?.[1]) {
    return resolveColor(
      variableMatch[1],
      declarations,
      paletteId,
      nextResolving,
    );
  }

  const relativeColorMatch = value.match(
    /^rgb\(\s*from\s+var\(\s*(--[\w-]+)\s*\)\s+r\s+g\s+b\s*\/\s*([0-9.]+)\s*\)$/i,
  );
  if (relativeColorMatch?.[1] && relativeColorMatch[2]) {
    const base = resolveColor(
      relativeColorMatch[1],
      declarations,
      paletteId,
      nextResolving,
    );
    return {
      ...base,
      alpha: base.alpha * Number(relativeColorMatch[2]),
    };
  }

  const hexColor = parseHexColor(value);
  if (hexColor) {
    return hexColor;
  }

  const rgbColor = parseRgbColor(value);
  if (rgbColor) {
    return rgbColor;
  }

  throw new Error(
    `Color palette token drift: palette=${paletteId} token=${tokenName} has unsupported compiled value ${rawValue}`,
  );
}

function parseHexColor(value: string): RGBColor | undefined {
  const hex = value.slice(1);
  if (!/^#[\da-f]{3,8}$/i.test(value) || ![3, 6, 8].includes(hex.length)) {
    return undefined;
  }

  const expanded =
    hex.length === 3
      ? hex
          .split("")
          .map((channel) => `${channel}${channel}`)
          .join("")
      : hex;
  return {
    alpha:
      expanded.length === 8 ? Number.parseInt(expanded.slice(6), 16) / 255 : 1,
    blue: Number.parseInt(expanded.slice(4, 6), 16),
    green: Number.parseInt(expanded.slice(2, 4), 16),
    red: Number.parseInt(expanded.slice(0, 2), 16),
  };
}

function parseRgbColor(value: string): RGBColor | undefined {
  const match = value.match(
    /^rgba?\(\s*([\d.]+)\s*[, ]\s*([\d.]+)\s*[, ]\s*([\d.]+)(?:\s*[,/]\s*([\d.]+))?\s*\)$/i,
  );
  if (!match) {
    return undefined;
  }

  return {
    alpha: match[4] ? Number(match[4]) : 1,
    blue: Number(match[3]),
    green: Number(match[2]),
    red: Number(match[1]),
  };
}

function formatColor(color: RGBColor): string {
  const channels = [color.red, color.green, color.blue]
    .map((channel) => Math.round(channel).toString(16).padStart(2, "0"))
    .join("");
  const alpha = Math.round(Math.min(Math.max(color.alpha, 0), 1) * 255);
  return `#${channels}${alpha === 255 ? "" : alpha.toString(16).padStart(2, "0")}`;
}
