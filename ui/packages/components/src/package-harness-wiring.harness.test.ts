import { execSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../../..",
);

describe("component package harness wiring", () => {
  it("exposes labeled make targets for the verification harness", () => {
    const output = execSync("make -n ui-components-verify", {
      cwd: repoRoot,
      encoding: "utf8",
    });

    expect(output).toContain("ui-components-typecheck");
    expect(output).toContain("ui-components-test");
    expect(output).toContain("ui-components-storybook");
    expect(output).toContain("ui-components-boundary");
    expect(output).toContain("ui-components-dependency-direction");
  });
});
