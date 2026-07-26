import { describe, expect, test } from "vitest";

import {
  assertComponentTestDuration,
  defaultComponentTestMaxDurationMs,
  formatComponentDuration,
  getComponentTestMaxDurationMs,
} from "./component-test-duration";

describe("component test duration budget", () => {
  test("defaults to a 150 second total budget", () => {
    expect(getComponentTestMaxDurationMs({})).toBe(
      defaultComponentTestMaxDurationMs,
    );
  });

  test("accepts a positive environment override", () => {
    expect(
      getComponentTestMaxDurationMs({ UI_COMPONENT_MAX_DURATION_MS: "90000" }),
    ).toBe(90_000);
  });

  test("rejects invalid environment overrides", () => {
    expect(() =>
      getComponentTestMaxDurationMs({ UI_COMPONENT_MAX_DURATION_MS: "0" }),
    ).toThrow(/expected a positive number/);
  });

  test("fails only when total wall time exceeds the budget", () => {
    expect(() => assertComponentTestDuration(149_999, 150_000)).not.toThrow();
    expect(() => assertComponentTestDuration(150_001, 150_000)).toThrow(
      /exceeded the 150.00s wall-clock budget: 150.00s/,
    );
    expect(formatComponentDuration(27_125)).toBe("27.13s");
  });
});
