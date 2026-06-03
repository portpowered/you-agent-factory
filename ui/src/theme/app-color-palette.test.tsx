import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  AppColorPaletteProvider,
  applyDocumentColorPalette,
  readStoredColorPalette,
  resolveInitialColorPalette,
  useAppColorPalette,
} from "./app-color-palette";
import { COLOR_PALETTE_STORAGE_KEY } from "./color-palette";

function PaletteProbe() {
  const { palette, setPalette, clearPaletteSelection } = useAppColorPalette();

  return (
    <div>
      <output data-testid="palette">{palette}</output>
      <button
        onClick={() => {
          setPalette("slate");
        }}
        type="button"
      >
        choose slate
      </button>
      <button
        onClick={() => {
          clearPaletteSelection();
        }}
        type="button"
      >
        clear palette
      </button>
    </div>
  );
}

describe("AppColorPaletteProvider", () => {
  afterEach(() => {
    window.sessionStorage.clear();
    applyDocumentColorPalette("factory-dark");
  });

  it("defaults to factory-dark and applies data-color-palette on the document root", () => {
    render(
      <AppColorPaletteProvider>
        <PaletteProbe />
      </AppColorPaletteProvider>,
    );

    expect(screen.getByTestId("palette").textContent).toBe("factory-dark");
    expect(document.documentElement.dataset.colorPalette).toBe("factory-dark");
  });

  it("persists palette selection in session storage for the current session", async () => {
    render(
      <AppColorPaletteProvider>
        <PaletteProbe />
      </AppColorPaletteProvider>,
    );

    screen.getByRole("button", { name: "choose slate" }).click();

    await waitFor(() => {
      expect(screen.getByTestId("palette").textContent).toBe("slate");
    });
    expect(document.documentElement.dataset.colorPalette).toBe("slate");
    expect(window.sessionStorage.getItem(COLOR_PALETTE_STORAGE_KEY)).toBe(
      "slate",
    );
  });

  it("restores a stored palette on mount", () => {
    window.sessionStorage.setItem(COLOR_PALETTE_STORAGE_KEY, "olive");

    render(
      <AppColorPaletteProvider>
        <PaletteProbe />
      </AppColorPaletteProvider>,
    );

    expect(screen.getByTestId("palette").textContent).toBe("olive");
    expect(document.documentElement.dataset.colorPalette).toBe("olive");
  });

  it("clears session storage when palette selection is reset", async () => {
    window.sessionStorage.setItem(COLOR_PALETTE_STORAGE_KEY, "slate");

    render(
      <AppColorPaletteProvider>
        <PaletteProbe />
      </AppColorPaletteProvider>,
    );

    screen.getByRole("button", { name: "clear palette" }).click();

    await waitFor(() => {
      expect(screen.getByTestId("palette").textContent).toBe("factory-dark");
    });
    expect(window.sessionStorage.getItem(COLOR_PALETTE_STORAGE_KEY)).toBeNull();
  });

  it("resolves invalid stored values to the default palette", () => {
    expect(resolveInitialColorPalette("unknown")).toBe("factory-dark");
    expect(readStoredColorPalette()).toBeNull();
  });
});
