// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { compile } from "@tailwindcss/node";
import { render, screen } from "@testing-library/react";
import { beforeAll, describe, expect, it } from "vitest";

import { applyDocumentColorPalette } from "../../theme/app-color-palette";
import { Button } from "./button";
import { OVERLAY_HOVER_VERIFICATION_PALETTES } from "./color-role-overlay-hover-surfaces";
import { Table, TableBody, TableCell, TableRow } from "./table";

const stylesDir = path.dirname(fileURLToPath(import.meta.url));
const uiRoot = path.resolve(stylesDir, "../../..");
const stylesSourcePath = path.join(uiRoot, "src", "styles.css");

function injectCompiledRootRules(compiledCss: string): void {
  const rootBlocks = compiledCss.match(/:root[^{]*\{[^}]*\}/g) ?? [];
  const paletteBlocks =
    compiledCss.match(/\[data-color-palette="[^"]+"\][^{]*\{[^}]*\}/g) ?? [];
  const style = document.createElement("style");
  style.textContent = [...rootBlocks, ...paletteBlocks].join("\n");
  document.head.appendChild(style);
}

function readCssVariable(name: string): string {
  return getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
}

describe("shared primitive overlay hover color roles (behavior)", () => {
  beforeAll(async () => {
    const source = readFileSync(stylesSourcePath, "utf8");
    const compiled = await compile(source, {
      base: path.dirname(stylesSourcePath),
      from: stylesSourcePath,
      onDependency: () => {},
    });
    injectCompiledRootRules(compiled.build([]));
  });

  it.each(OVERLAY_HOVER_VERIFICATION_PALETTES)(
    "applies palette %s before reading surface-container-low",
    (paletteId) => {
      applyDocumentColorPalette(paletteId);
      expect(document.documentElement.dataset.colorPalette).toBe(paletteId);
      expect(readCssVariable("--color-surface-container-low")).toBeTruthy();
    },
  );

  it("keeps distinct surface-container-low values across factory-dark and factory-light", () => {
    applyDocumentColorPalette("factory-dark");
    const darkLow = readCssVariable("--color-surface-container-low");

    applyDocumentColorPalette("factory-light");
    const lightLow = readCssVariable("--color-surface-container-low");

    expect(darkLow).toBeTruthy();
    expect(lightLow).toBeTruthy();
    expect(darkLow).not.toBe(lightLow);
  });

  it("renders migrated controls with surface-container hover and selected class hooks", () => {
    applyDocumentColorPalette("factory-dark");

    render(
      <>
        <Button data-testid="ghost" tone="ghost">
          Ghost
        </Button>
        <Button data-testid="outline" tone="outline">
          Outline
        </Button>
        <Button data-testid="secondary" tone="secondary">
          Secondary
        </Button>
        <Table>
          <TableBody>
            <TableRow data-testid="hover-row">
              <TableCell>Hover</TableCell>
            </TableRow>
            <TableRow data-state="selected" data-testid="selected-row">
              <TableCell>Selected</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </>,
    );

    expect(screen.getByTestId("ghost").className).toContain(
      "hover:bg-surface-container-low",
    );
    expect(screen.getByTestId("outline").className).toContain(
      "hover:bg-surface-container-highest",
    );
    expect(screen.getByTestId("secondary").className).toContain(
      "hover:bg-surface-container",
    );
    expect(screen.getByTestId("hover-row").className).toContain(
      "hover:bg-surface-container",
    );
    expect(screen.getByTestId("selected-row").className).toContain(
      "data-[state=selected]:bg-surface-container-low",
    );
  });
});
