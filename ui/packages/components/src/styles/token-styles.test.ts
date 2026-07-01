import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const packageSrcDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const stylesEntrypointPath = path.join(packageSrcDir, "styles.css");
const packageStylesDir = path.join(packageSrcDir, "styles");

function readTokenCss(fileName: string): string {
  return readFileSync(path.join(packageStylesDir, fileName), "utf8");
}

describe("youagentfactory/components token styles entrypoint", () => {
  const stylesEntrypointSource = readFileSync(stylesEntrypointPath, "utf8");

  it("imports shared token layers from the package styles directory", () => {
    expect(stylesEntrypointSource).toContain(
      '@import "./styles/color-palette-presets.css";',
    );
    expect(stylesEntrypointSource).toContain(
      '@import "./styles/color-role-tokens.css";',
    );
    expect(stylesEntrypointSource).toContain(
      '@import "./styles/text-color-role-tokens.css";',
    );
    expect(stylesEntrypointSource).toContain(
      '@import "./styles/typography-role-tokens.css";',
    );
    expect(stylesEntrypointSource).toContain(
      '@import "./styles/layout-role-tokens.css";',
    );
  });

  it("exposes role, palette, typography, and layout tokens for consumers", () => {
    const palettePresets = readTokenCss("color-palette-presets.css");
    const roleTokens = readTokenCss("color-role-tokens.css");
    const typographyTokens = readTokenCss("typography-role-tokens.css");
    const layoutTokens = readTokenCss("layout-role-tokens.css");

    expect(palettePresets).toContain("--color-af-foundation-accent:");
    expect(roleTokens).toContain("--color-af-text: var(--color-on-surface);");
    expect(typographyTokens).toContain("--text-title-large:");
    expect(layoutTokens).toContain("--spacing-layout-inset-dialog:");
    expect(palettePresets).toContain('[data-color-palette="slate"]');
  });
});
