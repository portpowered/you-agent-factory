import { describe, expect, it } from "vitest";

import {
  diffBaseline,
  isExcludedSourceFile,
  parseBaselineEntries,
  scanSourceTextForHardcodedCopy,
  serializeFinding,
} from "./check-hardcoded-ui-copy";

describe("hardcoded UI copy check", () => {
  it("excludes test, story, fixture, generated, and catalog files", () => {
    expect(
      isExcludedSourceFile("src/features/header/dashboard-header.test.tsx"),
    ).toBe(true);
    expect(
      isExcludedSourceFile("src/features/header/dashboard-header.stories.tsx"),
    ).toBe(true);
    expect(isExcludedSourceFile("src/api/generated/openapi.ts")).toBe(true);
    expect(
      isExcludedSourceFile("src/components/dashboard/fixtures/runtime.ts"),
    ).toBe(true);
    expect(isExcludedSourceFile("src/testing/replay-fixtures.ts")).toBe(true);
    expect(
      isExcludedSourceFile("src/features/header/messages/header-controls.ts"),
    ).toBe(true);
    expect(
      isExcludedSourceFile("src/features/header/dashboard-header.tsx"),
    ).toBe(false);
  });

  it("finds hardcoded JSX text and accessibility attributes in production components", () => {
    const findings = scanSourceTextForHardcodedCopy(
      "src/features/example/example-card.tsx",
      [
        "export function ExampleCard() {",
        "  return (",
        '    <section aria-label="Example region">',
        "      <h2>Localized heading</h2>",
        '      <input placeholder={"Search factories"} />',
        "    </section>",
        "  );",
        "}",
      ].join("\n"),
    );

    expect(findings).toEqual([
      expect.objectContaining({
        file: "src/features/example/example-card.tsx",
        kind: "jsx-attribute",
        line: 3,
        text: "Example region",
      }),
      expect.objectContaining({
        file: "src/features/example/example-card.tsx",
        kind: "jsx-text",
        line: 4,
        text: "Localized heading",
      }),
      expect.objectContaining({
        file: "src/features/example/example-card.tsx",
        kind: "jsx-attribute",
        line: 5,
        text: "Search factories",
      }),
    ]);
  });

  it("tracks unexpected and stale baseline entries separately", () => {
    const findings = scanSourceTextForHardcodedCopy(
      "src/features/example/example-card.tsx",
      '<section aria-label="Example region">Localized heading</section>',
    );
    const baselineEntries = parseBaselineEntries(
      [
        "# comment",
        serializeFinding(findings[0]),
        "src/features/example/example-card.tsx|99|1|jsx-text|Retired copy",
      ].join("\n"),
    );

    const diff = diffBaseline(findings, baselineEntries);

    expect(diff.unexpectedFindings).toEqual([findings[1]]);
    expect(diff.staleEntries).toEqual([
      "src/features/example/example-card.tsx|99|1|jsx-text|Retired copy",
    ]);
  });
});
