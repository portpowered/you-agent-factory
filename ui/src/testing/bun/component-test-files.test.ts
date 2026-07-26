import { describe, expect, it } from "vitest";

import { classifyComponentTestSource } from "../../../scripts/component-test-files";

describe("classifyComponentTestSource", () => {
  it("assigns ordinary browserless TSX tests to Bun", () => {
    expect(
      classifyComponentTestSource(
        "src/features/example/example-card.test.tsx",
        `import { render } from "${"@testing-library"}/react";`,
      ),
    ).toMatchObject({ runner: "bun" });
  });

  it("assigns native vi helpers to Bun", () => {
    expect(
      classifyComponentTestSource(
        "src/features/example/example-card.test.tsx",
        "const callback = vi.fn(); vi.mocked(callback);",
      ),
    ).toMatchObject({ runner: "bun" });
  });

  it("assigns extended helpers to Bun when the test imports the Bun vi shim", () => {
    expect(
      classifyComponentTestSource(
        "src/features/example/example-card.test.tsx",
        "import { bunVi as vi } from '../../../testing/bun/vi-compat'; vi.stubGlobal('fetch', vi.fn()); vi.unstubAllGlobals(); vi.spyOn(console, 'warn'); vi.clearAllMocks(); vi.restoreAllMocks();",
      ),
    ).toMatchObject({ runner: "bun" });
  });

  it.each([
    ["vi.useFakeTimers();", "uses unsupported Vitest APIs: useFakeTimers"],
    ["vi.mock('./dependency');", "uses unsupported Vitest APIs: mock"],
    [
      "vi.stubGlobal('fetch', vi.fn()); vi.unstubAllGlobals();",
      "uses unsupported Vitest APIs: stubGlobal, unstubAllGlobals",
    ],
  ])("keeps %s in Vitest", (source, reason) => {
    expect(
      classifyComponentTestSource(
        "src/features/example/example-card.test.tsx",
        source,
      ),
    ).toMatchObject({ reason, runner: "vitest" });
  });

  it("honors an execution-proven compatibility marker", () => {
    expect(
      classifyComponentTestSource(
        "src/features/example/example-card.test.tsx",
        "// @component-test-runner vitest: requires jsdom geometry",
      ),
    ).toMatchObject({
      reason: "explicitly documented Vitest compatibility exception",
      runner: "vitest",
    });
  });

  it("keeps explicitly migrated Bun files native", () => {
    expect(
      classifyComponentTestSource(
        "src/features/example/example-card.isolated.bun.component.test.tsx",
        'import { describe } from "bun:test";',
      ),
    ).toMatchObject({ runner: "bun" });
  });
});
