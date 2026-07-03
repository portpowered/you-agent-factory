import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const buttonDocsPath = path.join(packageRoot, "docs", "button.md");

describe("package button docs", () => {
  it("documents semantic variants, link differences, icon-only labels, and host responsibilities", () => {
    const docs = readFileSync(buttonDocsPath, "utf8");

    expect(docs).toContain("Button vs ButtonLink");
    expect(docs).toContain("Semantic button variants");
    expect(docs).toContain("destructive");
    expect(docs).toContain("warning");
    expect(docs).toContain("Loading buttons");
    expect(docs).toContain("Icon-only accessibility");
    expect(docs).toContain("aria-label");
    expect(docs).toContain("Host application responsibilities");
    expect(docs).toContain("Action copy");
    expect(docs).toContain("Click handlers");
    expect(docs).toContain("Routing");
    expect(docs).toContain("Domain workflows");
    expect(docs).toContain("Storybook examples");
    expect(docs).toContain("Semantic variants");
    expect(docs).toContain("Loading and icon only");
  });
});
