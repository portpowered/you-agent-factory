import { describe, expect, test } from "vitest";

import {
  buildVitestComponentArgs,
  componentTestMaxWorkersEnvironmentVariable,
  getComponentTestMaxWorkers,
  minimumComponentTestWorkers,
} from "./component-test-workers";

describe("component test worker selection", () => {
  test("uses the detected logical CPU count when no override is set", () => {
    expect(getComponentTestMaxWorkers({}, 6)).toBe(6);
  });

  test("floors detected CPU counts below two workers", () => {
    expect(getComponentTestMaxWorkers({}, 1)).toBe(minimumComponentTestWorkers);
    expect(getComponentTestMaxWorkers({}, 0)).toBe(minimumComponentTestWorkers);
  });

  test("uses a valid environment override before CPU detection", () => {
    expect(
      getComponentTestMaxWorkers(
        { [componentTestMaxWorkersEnvironmentVariable]: "4" },
        32,
      ),
    ).toBe(4);
  });

  test.each(["", "not-a-worker-count", "0", "-1", "1.5"])(
    "rejects invalid environment override %j before Vitest starts",
    (override) => {
      expect(() =>
        getComponentTestMaxWorkers({
          [componentTestMaxWorkersEnvironmentVariable]: override,
        }),
      ).toThrow(/expected a positive integer/);
    },
  );

  test("keeps Vitest selection, retry, forwarded args, and files stable", () => {
    const args = buildVitestComponentArgs({
      componentTests: ["src/example.component.test.ts"],
      env: { [componentTestMaxWorkersEnvironmentVariable]: "4" },
      forwardedArgs: ["--reporter=verbose"],
    });

    expect(args).toEqual([
      "run",
      "--config",
      "vitest.lanes.config.ts",
      "--project=dashboard-component",
      "--maxWorkers=4",
      "--retry=0",
      "--reporter=verbose",
      "src/example.component.test.ts",
    ]);
    expect(args.filter((arg) => arg.startsWith("--maxWorkers"))).toHaveLength(
      1,
    );
  });
});
