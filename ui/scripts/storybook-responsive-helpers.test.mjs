import { describe, expect, test, vi } from "vitest";

import {
  expectNoHorizontalOverflow,
  expectVisible,
  OVERFLOW_TOLERANCE_PX,
  STORY_RENDER_TIMEOUT_MS,
  waitForStoryRegion,
} from "./storybook-responsive-helpers.mjs";

describe("storybook-responsive-helpers", () => {
  test("exposes the responsive check timeout and overflow tolerance", () => {
    expect(STORY_RENDER_TIMEOUT_MS).toBe(30000);
    expect(OVERFLOW_TOLERANCE_PX).toBe(1);
  });

  test("expectVisible waits for and accepts visible locators", async () => {
    const locator = {
      isVisible: vi.fn().mockResolvedValue(true),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };

    await expect(
      expectVisible(locator, "Export action button"),
    ).resolves.toBeUndefined();

    expect(locator.waitFor).toHaveBeenCalledWith({
      state: "visible",
      timeout: STORY_RENDER_TIMEOUT_MS,
    });
    expect(locator.isVisible).not.toHaveBeenCalled();
  });

  test("expectVisible rejects minimal hidden locators with the assertion label", async () => {
    const locator = {
      isVisible: vi.fn().mockResolvedValue(false),
    };

    await expect(
      expectVisible(locator, "Export action button"),
    ).rejects.toThrow("Export action button was not visible.");
  });

  test("expectVisible surfaces Playwright wait failures", async () => {
    const locator = {
      isVisible: vi.fn().mockResolvedValue(true),
      waitFor: vi.fn().mockRejectedValue(new Error("locator timed out")),
    };

    await expect(
      expectVisible(locator, "Export action button"),
    ).rejects.toThrow("locator timed out");
    expect(locator.isVisible).not.toHaveBeenCalled();
  });

  test("waitForStoryRegion resolves the named visible region", async () => {
    const region = {
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const page = {
      getByRole: vi.fn().mockReturnValue(region),
    };

    const result = await waitForStoryRegion(page, "dashboard summary");

    expect(page.getByRole).toHaveBeenCalledWith("region", {
      name: "dashboard summary",
    });
    expect(region.waitFor).toHaveBeenCalledWith({ state: "visible" });
    expect(result).toBe(region);
  });

  test("expectNoHorizontalOverflow accepts widths within tolerance", async () => {
    const page = {
      evaluate: vi
        .fn()
        .mockResolvedValue({ clientWidth: 390, scrollWidth: 391 }),
    };

    await expect(
      expectNoHorizontalOverflow(page, "mobile export dialog"),
    ).resolves.toBeUndefined();
  });

  test("expectNoHorizontalOverflow rejects widths beyond tolerance", async () => {
    const page = {
      evaluate: vi
        .fn()
        .mockResolvedValue({ clientWidth: 390, scrollWidth: 393 }),
    };

    await expect(
      expectNoHorizontalOverflow(page, "mobile export dialog"),
    ).rejects.toThrow(
      "mobile export dialog overflowed horizontally: scrollWidth=393, clientWidth=390.",
    );
  });
});
