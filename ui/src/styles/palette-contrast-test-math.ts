export type RgbColor = readonly [red: number, green: number, blue: number];

export interface ParsedCssColor {
  alpha: number;
  rgb: RgbColor;
}

export interface CssVariableReader {
  getPropertyValue(name: string): string;
}

function resolutionError(
  paletteId: string,
  tokenName: string,
  reason: string,
): Error {
  return new Error(
    `Unable to resolve palette=${paletteId} token=${tokenName}: ${reason}`,
  );
}

function resolveCssVariables(
  value: string,
  reader: CssVariableReader,
  paletteId: string,
  tokenName: string,
  variableStack: readonly string[] = [],
): string {
  const variableMatch = value.match(
    /var\(\s*(--[\w-]+)(?:\s*,\s*([^)]*))?\s*\)/,
  );
  if (!variableMatch?.[1]) {
    return value;
  }

  const variableName = variableMatch[1];
  if (variableStack.includes(variableName)) {
    throw resolutionError(
      paletteId,
      tokenName,
      `cyclic var() reference (${[...variableStack, variableName].join(" -> ")})`,
    );
  }

  const referencedValue = reader.getPropertyValue(variableName).trim();
  const fallbackValue = variableMatch[2]?.trim();
  const replacement = referencedValue || fallbackValue;
  if (!replacement) {
    throw resolutionError(
      paletteId,
      tokenName,
      `missing value for ${variableName}`,
    );
  }

  const resolvedReplacement = resolveCssVariables(
    replacement,
    reader,
    paletteId,
    tokenName,
    [...variableStack, variableName],
  );
  return resolveCssVariables(
    value.replace(variableMatch[0], resolvedReplacement),
    reader,
    paletteId,
    tokenName,
    variableStack,
  );
}

function parseAlpha(
  value: string,
  paletteId: string,
  tokenName: string,
): number {
  const numericValue = value.endsWith("%") ? value.slice(0, -1) : value;
  if (!numericValue.trim()) {
    throw resolutionError(
      paletteId,
      tokenName,
      `invalid alpha component ${value}`,
    );
  }
  const parsedValue = Number(numericValue);
  const alpha = value.endsWith("%") ? parsedValue / 100 : parsedValue;
  if (!Number.isFinite(alpha) || alpha < 0 || alpha > 1) {
    throw resolutionError(
      paletteId,
      tokenName,
      `invalid alpha component ${value}`,
    );
  }
  return alpha;
}

function parseChannel(
  value: string,
  paletteId: string,
  tokenName: string,
): number {
  const numericValue = value.endsWith("%") ? value.slice(0, -1) : value;
  if (!numericValue.trim()) {
    throw resolutionError(
      paletteId,
      tokenName,
      `invalid RGB channel component ${value}`,
    );
  }
  const parsedValue = Number(numericValue);
  const channel = value.endsWith("%") ? (parsedValue / 100) * 255 : parsedValue;
  if (!Number.isFinite(channel) || channel < 0 || channel > 255) {
    throw resolutionError(
      paletteId,
      tokenName,
      `invalid RGB channel component ${value}`,
    );
  }
  return channel;
}

function parseCssColor(
  paletteId: string,
  tokenName: string,
  rawValue: string,
): ParsedCssColor {
  const value = rawValue.trim().replace(/\s+/g, " ");
  if (!value) {
    throw resolutionError(paletteId, tokenName, "empty value");
  }

  const hexMatch = value.match(/^#([0-9a-f]+)$/i);
  if (hexMatch?.[1] && [3, 4, 6, 8].includes(hexMatch[1].length)) {
    const hex =
      hexMatch[1].length <= 4
        ? [...hexMatch[1]].map((digit) => `${digit}${digit}`).join("")
        : hexMatch[1];
    const channels = [
      Number.parseInt(hex.slice(0, 2), 16),
      Number.parseInt(hex.slice(2, 4), 16),
      Number.parseInt(hex.slice(4, 6), 16),
    ] as const;
    const alpha =
      hex.length === 8 ? Number.parseInt(hex.slice(6, 8), 16) / 255 : 1;
    return { alpha, rgb: channels };
  }

  const relativeRgbMatch = value.match(
    /^rgb\( from (.+?) r g b(?: \/ (.+?))? \)$/i,
  );
  if (relativeRgbMatch?.[1]) {
    const source = parseCssColor(paletteId, tokenName, relativeRgbMatch[1]);
    return {
      alpha:
        relativeRgbMatch[2] === undefined
          ? source.alpha
          : parseAlpha(relativeRgbMatch[2], paletteId, tokenName),
      rgb: source.rgb,
    };
  }

  const functionMatch = value.match(/^(rgba?)\((.*)\)$/i);
  if (functionMatch?.[1] && functionMatch[2] !== undefined) {
    const functionName = functionMatch[1].toLowerCase();
    const sections = functionMatch[2].split("/");
    if (sections.length > 2) {
      throw resolutionError(
        paletteId,
        tokenName,
        `unsupported color value ${rawValue}`,
      );
    }

    const components = sections[0]
      .replaceAll(",", " ")
      .trim()
      .split(/\s+/)
      .filter(Boolean);
    let alphaValue = sections[1]?.trim();
    if (
      alphaValue === undefined &&
      functionName === "rgba" &&
      components.length === 4
    ) {
      alphaValue = components.pop() ?? "";
    }
    if (components.length !== 3) {
      throw resolutionError(
        paletteId,
        tokenName,
        `unsupported color value ${rawValue}`,
      );
    }
    return {
      alpha:
        alphaValue === undefined
          ? 1
          : parseAlpha(alphaValue, paletteId, tokenName),
      rgb: components.map((component) =>
        parseChannel(component, paletteId, tokenName),
      ) as unknown as RgbColor,
    };
  }

  throw resolutionError(
    paletteId,
    tokenName,
    `unsupported or unparseable value ${rawValue}`,
  );
}

export function resolveCssColor(
  paletteId: string,
  tokenName: string,
  reader: CssVariableReader,
): ParsedCssColor {
  const rawValue = reader.getPropertyValue(tokenName).trim();
  if (!rawValue) {
    throw resolutionError(paletteId, tokenName, "missing value");
  }
  const resolvedValue = resolveCssVariables(
    rawValue,
    reader,
    paletteId,
    tokenName,
  );
  return parseCssColor(paletteId, tokenName, resolvedValue);
}

export function compositeOver(
  foreground: ParsedCssColor,
  background: RgbColor,
): RgbColor {
  if (foreground.alpha === 1) {
    return foreground.rgb;
  }
  return foreground.rgb.map(
    (channel, index) =>
      channel * foreground.alpha + background[index] * (1 - foreground.alpha),
  ) as unknown as RgbColor;
}

export function resolveFillRgb(
  fillToken: string,
  fill: ParsedCssColor,
  surfaceRgb: RgbColor,
): RgbColor {
  if (
    fillToken.endsWith("-container") ||
    fillToken.startsWith("--color-surface-container")
  ) {
    return compositeOver(fill, surfaceRgb);
  }
  return fill.alpha === 1 ? fill.rgb : compositeOver(fill, surfaceRgb);
}

/** Euclidean distance in sRGB byte space; chart status colors must be >= 24. */
export function rgbEuclideanDistance(
  first: RgbColor,
  second: RgbColor,
): number {
  return Math.hypot(
    first[0] - second[0],
    first[1] - second[1],
    first[2] - second[2],
  );
}

function relativeLuminance([red, green, blue]: RgbColor): number {
  const linearChannels = [red, green, blue].map((channel) => {
    const scaled = channel / 255;
    return scaled <= 0.03928
      ? scaled / 12.92
      : ((scaled + 0.055) / 1.055) ** 2.4;
  });
  return (
    0.2126 * linearChannels[0] +
    0.7152 * linearChannels[1] +
    0.0722 * linearChannels[2]
  );
}

export function contrastRatio(
  foreground: RgbColor,
  background: RgbColor,
): number {
  const foregroundLuminance = relativeLuminance(foreground);
  const backgroundLuminance = relativeLuminance(background);
  const lighter = Math.max(foregroundLuminance, backgroundLuminance);
  const darker = Math.min(foregroundLuminance, backgroundLuminance);
  return (lighter + 0.05) / (darker + 0.05);
}

export function stableRatio(ratio: number, precision: number): number {
  return Number(ratio.toFixed(precision));
}
