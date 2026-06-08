// @vitest-environment node

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  openBrowserPage,
  startBrowserPreview,
} from "./browser-test-harness.mjs";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const paletteIds = [
  "factory-dark",
  "factory-light",
  "material-baseline",
  "slate",
  "olive",
];
const accentInkRgb = "rgb(26, 34, 40)";
const factoryLightAccentRgb = "rgb(245, 199, 111)";
const transparentBackgrounds = new Set([
  "",
  "rgba(0, 0, 0, 0)",
  "transparent",
]);

function normalizeColor(color) {
  return color.replace(/\s+/g, " ").trim().toLowerCase();
}

function menuItemMarkup() {
  return `
    <button
      type="button"
      role="menuitemradio"
      aria-checked="true"
      class="inline-flex min-h-0 w-full items-center justify-start rounded-xl border px-3 py-2 text-sm font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-af-focus-ring focus-visible:ring-offset-0 disabled:pointer-events-none disabled:border-outline disabled:bg-surface-container-low disabled:text-on-surface-disabled border-primary bg-primary-container text-on-primary"
    >
      <span class="grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-2 text-left">
        Factory Light
      </span>
    </button>
  `;
}

describe.sequential("header palette menu selected-state browser integration", () => {
  let preview = null;
  let compiledCss = "";

  beforeAll(async () => {
    preview = await startBrowserPreview();
    compiledCss = readFileSync(
      path.join(packageRoot, "dist", "assets", "index.css"),
      "utf8",
    );
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  }, buildTimeoutMs);

  it.each(paletteIds)(
    "renders selected palette menu items with accent-ink foreground for palette %s",
    async (paletteId) => {
      const browserPage = await openBrowserPage({
        artifactLabel: `header-palette-menu-${paletteId}`,
      });

      try {
        await browserPage.page.setContent(
          `<!DOCTYPE html>
            <html data-color-palette="${paletteId}">
              <head><style>${compiledCss}</style></head>
              <body>${menuItemMarkup()}</body>
            </html>`,
        );

        const computed = await browserPage.page.evaluate(() => {
          const element = document.querySelector('[role="menuitemradio"]');
          const styles = getComputedStyle(element);
          return {
            backgroundColor: styles.backgroundColor,
            color: styles.color,
          };
        });

        expect(normalizeColor(computed.color)).toBe(
          normalizeColor(accentInkRgb),
        );
        expect(
          transparentBackgrounds.has(normalizeColor(computed.backgroundColor)),
        ).toBe(false);

        if (paletteId === "factory-light") {
          expect(normalizeColor(computed.color)).not.toBe(
            normalizeColor(factoryLightAccentRgb),
          );
        }
      } finally {
        await browserPage.close();
      }
    },
    browserScenarioTimeoutMs,
  );
});
