import { expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  buildBunComponentTestCommand,
  formatBunFailureReport,
  runBunComponentTests,
} from "./bun-component-test-runner";

test("names multiple assertion files and setup errors from the CI Bun command", () => {
  const directory = mkdtempSync(join(tmpdir(), "bun-component-runner-test-"));
  const firstFile = join(directory, "first.test.ts");
  const secondFile = join(directory, "second.test.ts");
  const setupFailureFile = join(directory, "setup-failure.test.ts");

  writeFileSync(
    firstFile,
    [
      'import { expect, test } from "bun:test";',
      "",
      'test("reports the first full test name", () => {',
      '  expect({ received: "first" }).toEqual({ expected: "expected-first" });',
      "});",
    ].join("\n"),
  );
  writeFileSync(
    secondFile,
    [
      'import { expect, test } from "bun:test";',
      "",
      'test("reports the second full test name", () => {',
      '  expect({ received: "second" }).toEqual({ expected: "expected-second" });',
      "});",
    ].join("\n"),
  );
  writeFileSync(
    setupFailureFile,
    'throw new Error("setup failure remains file-owned");\n',
  );

  try {
    expect(buildBunComponentTestCommand([firstFile, secondFile])).toEqual([
      process.execPath,
      "test",
      "--preload",
      "./src/testing/bun/component.setup.ts",
      "--reporter=dots",
      "--timeout=10000",
      firstFile,
      secondFile,
    ]);

    const result = runBunComponentTests([
      firstFile,
      secondFile,
      setupFailureFile,
    ]);
    const report = formatBunFailureReport(`${result.stdout}\n${result.stderr}`);

    expect(result.exitCode).toBe(1);
    expect(
      report
        .split("\n")
        .some(
          (line) =>
            line.startsWith("- file: ") && line.endsWith("first.test.ts"),
        ),
    ).toBe(true);
    expect(report).toContain(
      "full test name: reports the first full test name",
    );
    expect(report).toContain("expect(received).toEqual(expected)");
    expect(report).toContain("expected-first");
    expect(
      report
        .split("\n")
        .some(
          (line) =>
            line.startsWith("- file: ") && line.endsWith("second.test.ts"),
        ),
    ).toBe(true);
    expect(report).toContain(
      "full test name: reports the second full test name",
    );
    expect(report).toContain("expected-second");
    expect(
      report
        .split("\n")
        .some(
          (line) =>
            line.startsWith("- file: ") &&
            line.endsWith("setup-failure.test.ts"),
        ),
    ).toBe(true);
    expect(report).toContain("setup/runtime error:");
    expect(report).toContain("setup failure remains file-owned");
    expect(report).not.toContain(
      "full test name: setup failure remains file-owned",
    );
  } finally {
    rmSync(directory, { force: true, recursive: true });
  }
});
