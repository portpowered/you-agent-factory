import type { ReactNode } from "react";
import { createContext, useContext, useEffect, useMemo, useState } from "react";

import {
  COLOR_PALETTE_STORAGE_KEY,
  type ColorPaletteId,
  DEFAULT_COLOR_PALETTE,
  resolveColorPaletteId,
} from "./color-palette";

interface AppColorPaletteContextValue {
  clearPaletteSelection: () => void;
  palette: ColorPaletteId;
  setPalette: (palette: string | null | undefined) => void;
}

export interface AppColorPaletteProviderProps {
  children: ReactNode;
  initialPalette?: string | null;
}

const AppColorPaletteContext =
  createContext<AppColorPaletteContextValue | null>(null);

export function AppColorPaletteProvider({
  children,
  initialPalette,
}: AppColorPaletteProviderProps) {
  const [selectedPalette, setSelectedPalette] = useState<
    string | null | undefined
  >(() => initialPalette ?? readStoredColorPalette());
  const palette = useMemo(
    () => resolveColorPaletteId(selectedPalette),
    [selectedPalette],
  );

  useEffect(() => {
    applyDocumentColorPalette(palette);
  }, [palette]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }

    if (selectedPalette === undefined || selectedPalette === null) {
      window.sessionStorage.removeItem(COLOR_PALETTE_STORAGE_KEY);
      return;
    }

    window.sessionStorage.setItem(COLOR_PALETTE_STORAGE_KEY, palette);
  }, [palette, selectedPalette]);

  const value = useMemo<AppColorPaletteContextValue>(
    () => ({
      clearPaletteSelection: () => {
        setSelectedPalette(null);
      },
      palette,
      setPalette: (nextPalette) => {
        setSelectedPalette(nextPalette);
      },
    }),
    [palette],
  );

  return (
    <AppColorPaletteContext.Provider value={value}>
      {children}
    </AppColorPaletteContext.Provider>
  );
}

export function useAppColorPalette(
  paletteOverride?: string | null,
): AppColorPaletteContextValue {
  const context = useContext(AppColorPaletteContext);

  if (paletteOverride !== undefined && paletteOverride !== null) {
    const palette = resolveColorPaletteId(paletteOverride);
    return {
      clearPaletteSelection: () => {},
      palette,
      setPalette: () => {},
    };
  }

  if (context) {
    return context;
  }

  return {
    clearPaletteSelection: () => {},
    palette: resolveColorPaletteId(readStoredColorPalette()),
    setPalette: () => {},
  };
}

export function applyDocumentColorPalette(palette: ColorPaletteId): void {
  if (typeof document === "undefined") {
    return;
  }

  document.documentElement.dataset.colorPalette = palette;
}

export function readStoredColorPalette(): string | null {
  if (typeof window === "undefined") {
    return null;
  }

  return window.sessionStorage.getItem(COLOR_PALETTE_STORAGE_KEY);
}

export function resolveInitialColorPalette(
  initialPalette?: string | null,
): ColorPaletteId {
  return resolveColorPaletteId(initialPalette ?? readStoredColorPalette());
}

export { DEFAULT_COLOR_PALETTE };
