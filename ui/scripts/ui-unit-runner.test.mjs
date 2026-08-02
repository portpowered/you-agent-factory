import { expect, test, vi } from "vitest";

import {
  buildUiUnitPhases,
  dashboardUnitVitestArgs,
  runUiUnit,
  uiUnitPhases,
} from "./ui-unit-runner.mjs";

test("runs the Bun lane before the existing Vitest dashboard-unit lane", () => {
  const spawn = vi.fn(() => ({ status: 0 }));

  expect(runUiUnit({ spawn })).toBe(0);
  expect(spawn).toHaveBeenCalledTimes(2);
  expect(spawn.mock.calls[0][0]).toBe("bun");
  expect(spawn.mock.calls[0][1]).toEqual(["run", "test:unit:bun"]);
  expect(spawn.mock.calls[1][0]).toBe("vitest");
  expect(spawn.mock.calls[1][1]).toEqual(dashboardUnitVitestArgs);
});

test("returns a Bun failure and does not hide it behind Vitest", () => {
  const spawn = vi.fn(() => ({ status: 17 }));

  expect(runUiUnit({ spawn })).toBe(17);
  expect(spawn).toHaveBeenCalledTimes(1);
});

test("returns a later Vitest failure after the Bun lane passes", () => {
  const spawn = vi
    .fn()
    .mockReturnValueOnce({ status: 0 })
    .mockReturnValueOnce({ status: 23 });

  expect(runUiUnit({ spawn })).toBe(23);
  expect(spawn).toHaveBeenCalledTimes(2);
});

test("keeps the aggregate phase definitions explicit and ordered", () => {
  expect(uiUnitPhases.map((phase) => phase.name)).toEqual([
    "Bun native unit",
    "Vitest dashboard unit",
  ]);
});

test("forwards focused Vitest arguments without changing Bun discovery", () => {
  const [bunPhase, vitestPhase] = buildUiUnitPhases([
    "src/lib/components-package-resolution.test.ts",
  ]);

  expect(bunPhase.args).toEqual(["run", "test:unit:bun"]);
  expect(vitestPhase.args).toEqual([
    ...dashboardUnitVitestArgs,
    "src/lib/components-package-resolution.test.ts",
  ]);
});
