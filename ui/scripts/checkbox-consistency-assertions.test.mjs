import { describe, expect, test, vi } from "vitest";

import {
  assertCheckboxCheckedState,
  assertCheckboxDisabledState,
  assertCheckboxInvalidState,
  assertStyledCheckboxTreatment,
  readCheckboxIndicatorClassName,
} from "./checkbox-consistency-assertions.mjs";

function createCheckboxLocator({
  ariaInvalid,
  checked = false,
  className = "peer sr-only",
  disabled = false,
  indicatorClassName = "peer-checked:bg-primary border-outline peer-focus-visible:ring-af-focus-ring peer-disabled:bg-surface-container-low peer-aria-invalid:ring-af-danger-border",
  svgCount = 1,
} = {}) {
  const indicator = {
    className: indicatorClassName,
    evaluate: vi.fn(async (callback) =>
      callback({ className: indicatorClassName }),
    ),
    locator: vi.fn(() => ({
      count: vi.fn(async () => svgCount),
    })),
    waitFor: vi.fn().mockResolvedValue(undefined),
  };

  return {
    evaluate: vi.fn(async (callback) => callback({ className })),
    getAttribute: vi.fn(async (name) =>
      name === "aria-invalid" ? (ariaInvalid ?? null) : null,
    ),
    isChecked: vi.fn(async () => checked),
    isDisabled: vi.fn(async () => disabled),
    locator: vi.fn((selector) => {
      if (selector === "xpath=following-sibling::*[1]") {
        return indicator;
      }

      throw new Error(`Unexpected locator selector: ${selector}`);
    }),
  };
}

describe("checkbox-consistency-assertions", () => {
  test("assertStyledCheckboxTreatment accepts sr-only input and indicator markers", async () => {
    await expect(
      assertStyledCheckboxTreatment(createCheckboxLocator(), "test checkbox"),
    ).resolves.toBeUndefined();
  });

  test("assertStyledCheckboxTreatment rejects native-looking inputs", async () => {
    await expect(
      assertStyledCheckboxTreatment(
        createCheckboxLocator({ className: "h-4 w-4" }),
        "test checkbox",
      ),
    ).rejects.toThrow(/sr-only/);
  });

  test("readCheckboxIndicatorClassName returns the sibling indicator class", async () => {
    await expect(
      readCheckboxIndicatorClassName(createCheckboxLocator()),
    ).resolves.toContain("peer-checked:bg-primary");
  });

  test("assertCheckboxCheckedState compares checked state", async () => {
    await expect(
      assertCheckboxCheckedState(
        createCheckboxLocator({ checked: true }),
        true,
        "checked checkbox",
      ),
    ).resolves.toBeUndefined();

    await expect(
      assertCheckboxCheckedState(
        createCheckboxLocator({ checked: false }),
        true,
        "unchecked checkbox",
      ),
    ).rejects.toThrow(/expected checked=true/);
  });

  test("assertCheckboxDisabledState requires disabled input and styling", async () => {
    await expect(
      assertCheckboxDisabledState(
        createCheckboxLocator({ disabled: true }),
        "disabled checkbox",
      ),
    ).resolves.toBeUndefined();

    await expect(
      assertCheckboxDisabledState(
        createCheckboxLocator({ disabled: false }),
        "enabled checkbox",
      ),
    ).rejects.toThrow(/disabled/);
  });

  test("assertCheckboxInvalidState requires aria-invalid and styling", async () => {
    await expect(
      assertCheckboxInvalidState(
        createCheckboxLocator({ ariaInvalid: "true" }),
        "invalid checkbox",
      ),
    ).resolves.toBeUndefined();

    await expect(
      assertCheckboxInvalidState(
        createCheckboxLocator({ ariaInvalid: null }),
        "valid checkbox",
      ),
    ).rejects.toThrow(/aria-invalid/);
  });
});
