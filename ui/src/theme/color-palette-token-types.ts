export interface ColorPaletteFoundationTokens {
  readonly accent: string;
  readonly accentInk: string;
  readonly accentStrong: string;
  readonly background: string;
  readonly backgroundMid: string;
  readonly backgroundStart: string;
  readonly canvas: string;
  readonly codeInk: string;
  readonly danger: string;
  readonly dangerBright: string;
  readonly dangerInk: string;
  readonly info: string;
  readonly infoBright: string;
  readonly infoInk: string;
  readonly infoStrong: string;
  readonly ink: string;
  readonly overlay: string;
  readonly secondaryAccent: string;
  readonly secondaryAccentInk: string;
  readonly shadow: string;
  readonly success: string;
  readonly successInk: string;
  readonly surface: string;
  readonly tertiaryAccent: string;
  readonly tertiaryAccentInk: string;
  readonly warning: string;
  readonly warningInk: string;
  readonly worker: string;
  readonly workerInk: string;
}

export interface ColorPaletteSemanticTokens {
  readonly accent: {
    readonly default: string;
    readonly ink: string;
    readonly strong: string;
    readonly surface: string;
  };
  readonly background: string;
  readonly code: string;
  readonly danger: {
    readonly bright: string;
    readonly default: string;
    readonly ink: string;
    readonly on: string;
    readonly surface: string;
  };
  readonly foreground: {
    readonly default: string;
    readonly disabled: string;
    readonly inverse: string;
    readonly muted: string;
    readonly subtle: string;
  };
  readonly info: {
    readonly bright: string;
    readonly default: string;
    readonly ink: string;
    readonly on: string;
    readonly strong: string;
    readonly surface: string;
  };
  readonly outline: {
    readonly default: string;
    readonly variant: string;
  };
  readonly overlay: {
    readonly default: string;
    readonly solid: string;
  };
  readonly secondary: {
    readonly default: string;
    readonly ink: string;
    readonly strong: string;
    readonly surface: string;
  };
  readonly success: {
    readonly default: string;
    readonly ink: string;
    readonly on: string;
    readonly surface: string;
  };
  readonly surface: {
    readonly container: string;
    readonly containerHigh: string;
    readonly containerHighest: string;
    readonly containerLow: string;
    readonly default: string;
    readonly muted: string;
    readonly raised: string;
  };
  readonly tertiary: {
    readonly default: string;
    readonly ink: string;
    readonly on: string;
    readonly surface: string;
  };
  readonly warning: {
    readonly default: string;
    readonly ink: string;
    readonly on: string;
    readonly surface: string;
  };
  readonly worker: {
    readonly default: string;
    readonly ink: string;
    readonly surface: string;
  };
}

export interface ColorPaletteTokens {
  readonly foundation: ColorPaletteFoundationTokens;
  readonly semantic: ColorPaletteSemanticTokens;
}
