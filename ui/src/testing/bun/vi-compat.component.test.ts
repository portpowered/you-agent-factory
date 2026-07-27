import { describe, expect, it } from "bun:test";

import { bunVi } from "./vi-compat";

const TEST_GLOBAL = "__bunViWindowParityTest";

describe("bunVi global stubs", () => {
  it("mirrors stubs onto the Happy DOM window and restores both globals", () => {
    const globalDescriptor = Object.getOwnPropertyDescriptor(
      globalThis,
      TEST_GLOBAL,
    );
    const windowDescriptor = Object.getOwnPropertyDescriptor(
      window,
      TEST_GLOBAL,
    );

    try {
      bunVi.stubGlobal(TEST_GLOBAL, "stubbed");

      expect(Reflect.get(globalThis, TEST_GLOBAL)).toBe("stubbed");
      expect(Reflect.get(window, TEST_GLOBAL)).toBe("stubbed");
    } finally {
      bunVi.unstubAllGlobals();
    }

    expect(Object.getOwnPropertyDescriptor(globalThis, TEST_GLOBAL)).toEqual(
      globalDescriptor,
    );
    expect(Object.getOwnPropertyDescriptor(window, TEST_GLOBAL)).toEqual(
      windowDescriptor,
    );
  });
});
