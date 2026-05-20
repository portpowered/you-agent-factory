import { describe, expect, test, vi } from "vitest";

import { verifyDashboardHeader } from "./verify-import-export-storybook-responsive.mjs";

describe("verifyDashboardHeader", () => {
  test("verifyDashboardHeader exercises keyboard and desktop ordering checks", async () => {
    const hiddenWordmark = {
      getAttribute: vi.fn().mockResolvedValue("sr-only"),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const heading = {
      boundingBox: vi.fn().mockResolvedValue({ height: 20, width: 120, x: 10, y: 0 }),
      getByText: vi.fn().mockReturnValue(hiddenWordmark),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const slider = {
      boundingBox: vi.fn().mockResolvedValue({ height: 20, width: 200, x: 160, y: 0 }),
      focus: vi.fn().mockResolvedValue(undefined),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const streamStatus = {
      boundingBox: vi.fn().mockResolvedValue({ height: 20, width: 180, x: 380, y: 0 }),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const currentButton = {
      focus: vi.fn().mockResolvedValue(undefined),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const exportButton = {
      boundingBox: vi.fn().mockResolvedValue({ height: 20, width: 120, x: 580, y: 0 }),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const currentTick = { isVisible: vi.fn().mockResolvedValue(true) };
    const page = {
      evaluate: vi.fn().mockResolvedValue({ clientWidth: 1440, scrollWidth: 1440 }),
      getByRole: vi.fn((role, options) => {
        if (role === "heading") return heading;
        if (role === "slider") return slider;
        if (role === "status") return streamStatus;
        if (options?.name === "Return to current tick") return currentButton;
        return exportButton;
      }),
      getByText: vi
        .fn()
        .mockReturnValueOnce({ first: vi.fn().mockReturnValue(currentTick) })
        .mockReturnValueOnce({ isVisible: vi.fn().mockResolvedValue(true) })
        .mockReturnValueOnce(currentTick),
      keyboard: {
        press: vi.fn().mockResolvedValue(undefined),
      },
    };

    await verifyDashboardHeader(page, null, {
      height: 900,
      label: "desktop",
      width: 1440,
    });

    expect(heading.getByText).toHaveBeenCalledWith("Infinite You");
    expect(slider.focus).toHaveBeenCalledTimes(1);
    expect(currentButton.focus).toHaveBeenCalledTimes(1);
    expect(page.keyboard.press).toHaveBeenNthCalledWith(1, "ArrowLeft");
    expect(page.keyboard.press).toHaveBeenNthCalledWith(2, "Enter");
  });

  test("verifyDashboardHeader rejects a non-sr-only heading wordmark", async () => {
    const heading = {
      getByText: vi.fn().mockReturnValue({
        getAttribute: vi.fn().mockResolvedValue("text-visible"),
        isVisible: vi.fn().mockResolvedValue(true),
      }),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const page = {
      evaluate: vi.fn().mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
      getByRole: vi.fn((role, options) => {
        if (role === "heading") return heading;
        if (role === "slider") {
          return {
            focus: vi.fn().mockResolvedValue(undefined),
            isVisible: vi.fn().mockResolvedValue(true),
          };
        }
        if (role === "status") return { isVisible: vi.fn().mockResolvedValue(true) };
        if (options?.name === "Return to current tick") {
          return {
            focus: vi.fn().mockResolvedValue(undefined),
            isVisible: vi.fn().mockResolvedValue(true),
          };
        }
        return { isVisible: vi.fn().mockResolvedValue(true) };
      }),
      getByText: vi
        .fn()
        .mockReturnValueOnce({
          first: vi.fn().mockReturnValue({
            isVisible: vi.fn().mockResolvedValue(true),
          }),
        })
        .mockReturnValue({ isVisible: vi.fn().mockResolvedValue(true) }),
      keyboard: {
        press: vi.fn().mockResolvedValue(undefined),
      },
    };

    await expect(
      verifyDashboardHeader(page, null, {
        height: 844,
        label: "mobile",
        width: 390,
      }),
    ).rejects.toThrow(
      "Dashboard heading wordmark was not hidden with sr-only styling.",
    );
  });
});
