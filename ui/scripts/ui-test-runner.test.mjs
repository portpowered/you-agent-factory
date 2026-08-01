import { expect, test, vi } from "vitest";

import { runUiTest, uiTestPhases } from "./ui-test-runner.mjs";

test("stops the standard frontend test command when aggregate units fail", () => {
  const spawn = vi.fn(() => ({ status: 19 }));

  expect(runUiTest({ spawn })).toBe(19);
  expect(spawn).toHaveBeenCalledTimes(1);
  expect(spawn.mock.calls[0][1]).toEqual(["run", "test:unit"]);
});

test("continues from aggregate units to component and browser stages", () => {
  const spawn = vi.fn(() => ({ status: 0 }));

  expect(runUiTest({ spawn })).toBe(0);
  expect(spawn).toHaveBeenCalledTimes(3);
  expect(spawn.mock.calls.map((call) => call[1])).toEqual([
    ["run", "test:unit"],
    ["run", "test:component"],
    ["run", "test:integration"],
  ]);
});

test("preserves the standard frontend stage order", () => {
  expect(uiTestPhases.map((phase) => phase.name)).toEqual([
    "Unit tests",
    "Component tests",
    "Browser integration tests",
  ]);
});
