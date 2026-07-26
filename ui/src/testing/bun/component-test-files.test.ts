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

  it.each([
    ["vi.useFakeTimers();", "uses Vitest mocking or timer APIs"],
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
