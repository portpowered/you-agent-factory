import {
  COLOR_PALETTE_IDS,
  type ColorPaletteId,
  type ColorPaletteOption,
} from "../../../theme/color-palette";

import {
  getHeaderControlsMessages,
  type HeaderControlsMessages,
} from "./header-controls";

function resolvePaletteOptionLabel(
  messages: HeaderControlsMessages,
  paletteId: ColorPaletteId,
): string {
  switch (paletteId) {
    case "factory-dark":
      return messages.paletteOptionFactoryDarkLabel;
    case "factory-light":
      return messages.paletteOptionFactoryLightLabel;
    case "material-baseline":
      return messages.paletteMaterialBaselineOptionLabel;
    case "slate":
      return messages.paletteOptionSlateLabel;
    case "olive":
      return messages.paletteOptionOliveLabel;
    default: {
      const exhaustive: never = paletteId;
      return exhaustive;
    }
  }
}

export function getColorPaletteOptions(
  locale?: string | null,
): readonly ColorPaletteOption[] {
  const messages = getHeaderControlsMessages(locale);

  return COLOR_PALETTE_IDS.map((id) => ({
    id,
    label: resolvePaletteOptionLabel(messages, id),
  }));
}
