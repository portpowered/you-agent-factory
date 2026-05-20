import { describe, expect, test, vi } from "vitest";

import { verifyDashboardHeader } from "./verify-import-export-storybook-responsive.mjs";

function createCurrentTickLocator() {
  return { isVisible: vi.fn().mockResolvedValue(true) };
}

function createHiddenTickLabel(className = "sr-only") {
  return {
    first: vi.fn().mockReturnThis(),
    getAttribute: vi.fn().mockResolvedValue(className),
  };
}

function createPage({
  currentTick = createCurrentTickLocator(),
  headingWordmarkClassName = "sr-only",
  isDesktop = false,
  tickLabelClassName = "sr-only",
} = {}) {
  const heading = {
    ...(isDesktop
      ? {
          boundingBox: vi
            .fn()
            .mockResolvedValue({ height: 20, width: 120, x: 10, y: 0 }),
        }
      : {}),
    getByText: vi.fn().mockReturnValue({
      getAttribute: vi.fn().mockResolvedValue(headingWordmarkClassName),
      isVisible: vi.fn().mockResolvedValue(true),
    }),
    isVisible: vi.fn().mockResolvedValue(true),
  };
  const slider = {
    ...(isDesktop
      ? {
          boundingBox: vi
            .fn()
            .mockResolvedValue({ height: 20, width: 200, x: 160, y: 0 }),
        }
      : {}),
    focus: vi.fn().mockResolvedValue(undefined),
    isVisible: vi.fn().mockResolvedValue(true),
  };
  const languageButton = {
    ...(isDesktop
      ? {
          boundingBox: vi
            .fn()
            .mockResolvedValue({ height: 20, width: 120, x: 380, y: 0 }),
        }
      : {}),
    isVisible: vi.fn().mockResolvedValue(true),
  };
  const streamStatus = {
    ...(isDesktop
      ? {
          boundingBox: vi
            .fn()
            .mockResolvedValue({ height: 20, width: 180, x: 640, y: 0 }),
        }
      : {}),
    isVisible: vi.fn().mockResolvedValue(true),
  };
  const currentButton = {
    focus: vi.fn().mockResolvedValue(undefined),
    isVisible: vi.fn().mockResolvedValue(true),
  };
  const exportButton = {
    ...(isDesktop
      ? {
          boundingBox: vi
            .fn()
            .mockResolvedValue({ height: 20, width: 120, x: 520, y: 0 }),
        }
      : {}),
    isVisible: vi.fn().mockResolvedValue(true),
  };
  const page = {
    currentButton,
    heading,
    keyboard: {
      press: vi.fn().mockResolvedValue(undefined),
    },
    slider,
    evaluate: vi
      .fn()
      .mockResolvedValue({ clientWidth: isDesktop ? 1440 : 390, scrollWidth: isDesktop ? 1440 : 390 }),
    getByRole: vi.fn((role, options) => {
      if (role === "heading") return heading;
      if (role === "slider") return slider;
      if (options?.name === "Change language") return languageButton;
      if (role === "status") return streamStatus;
      if (options?.name === "Return to current tick") return currentButton;
      return exportButton;
    }),
    getByText: vi.fn((text) => {
      if (text === "Timeline tick") {
        return createHiddenTickLabel(tickLabelClassName);
      }
      return {
        first: vi.fn().mockReturnValue(currentTick),
        isVisible: vi.fn().mockResolvedValue(true),
      };
    }),
  };

  return page;
}

describe("verifyDashboardHeader", () => {
  test("verifyDashboardHeader exercises keyboard and desktop ordering checks", async () => {
    const currentTick = createCurrentTickLocator();
    const page = createPage({ currentTick, isDesktop: true });

    await verifyDashboardHeader(page, null, {
      height: 900,
      label: "desktop",
      width: 1440,
    });

    expect(page.heading.getByText).toHaveBeenCalledWith("Infinite You");
    expect(page.slider.focus).toHaveBeenCalledTimes(1);
    expect(page.currentButton.focus).toHaveBeenCalledTimes(1);
    expect(page.keyboard.press).toHaveBeenNthCalledWith(1, "ArrowLeft");
    expect(page.keyboard.press).toHaveBeenNthCalledWith(2, "Enter");
  });

  test("verifyDashboardHeader rejects a non-sr-only heading wordmark", async () => {
    const page = createPage({ headingWordmarkClassName: "text-visible" });

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

  test("verifyDashboardHeader rejects a visible timeline tick label", async () => {
    const page = createPage({ tickLabelClassName: "text-visible" });

    await expect(
      verifyDashboardHeader(page, null, {
        height: 844,
        label: "mobile",
        width: 390,
      }),
    ).rejects.toThrow(
      "Dashboard timeline tick label was not hidden with sr-only styling.",
    );
  });
});
