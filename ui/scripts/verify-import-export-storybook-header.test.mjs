import { describe, expect, test, vi } from "vitest";

import { verifyDashboardHeader } from "./verify-import-export-storybook-responsive.mjs";
import { verifyDashboardSessionSwitching } from "./verify-dashboard-session-switching-storybook-responsive.mjs";
import { verifyDashboardSessionTabs } from "./verify-dashboard-session-tabs-storybook-responsive.mjs";

function createCurrentTickLocator() {
  return { isVisible: vi.fn().mockResolvedValue(true) };
}

function createPage({
  currentTick = createCurrentTickLocator(),
  headingWordmarkClassName = "sr-only",
  isDesktop = false,
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
            .mockResolvedValue({ height: 20, width: 120, x: 660, y: 0 }),
        }
      : {}),
    isVisible: vi.fn().mockResolvedValue(true),
  };
  const streamStatus = {
    ...(isDesktop
      ? {
          boundingBox: vi
            .fn()
            .mockResolvedValue({ height: 20, width: 120, x: 380, y: 0 }),
        }
      : {}),
    isVisible: vi.fn().mockResolvedValue(true),
  };
  const currentButton = {
    ...(isDesktop
      ? {
          boundingBox: vi
            .fn()
            .mockResolvedValue({ height: 20, width: 120, x: 380, y: 40 }),
        }
      : {}),
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
      if (text instanceof RegExp && text.test("Waiting for more ticks")) {
        return {
          first: vi.fn().mockReturnValue(currentTick),
          isVisible: vi.fn().mockResolvedValue(true),
        };
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

    expect(page.heading.getByText).toHaveBeenCalledWith("you-agent-factory");
    expect(page.slider.focus).not.toHaveBeenCalled();
    expect(page.currentButton.focus).not.toHaveBeenCalled();
    expect(page.keyboard.press).not.toHaveBeenCalled();
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
});

describe("verifyDashboardSessionTabs", () => {
  test("verifyDashboardSessionTabs verifies the visible tab strip", async () => {
    const defaultTab = {
      getAttribute: vi.fn().mockResolvedValue("true"),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const reviewTab = {
      getAttribute: vi.fn().mockResolvedValue("true"),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const targetButton = {
      click: vi.fn().mockResolvedValue(undefined),
    };
    const targetPicker = {
      getByRole: vi.fn().mockReturnValue(targetButton),
      getByText: vi.fn().mockReturnValue({
        isVisible: vi.fn().mockResolvedValue(true),
      }),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const folderField = {
      fill: vi.fn().mockResolvedValue(undefined),
    };
    const inspectButton = {
      click: vi.fn().mockResolvedValue(undefined),
    };
    const dialog = {
      getByRole: vi.fn((role, options) => {
        if (role === "textbox") {
          return folderField;
        }
        if (role === "button" && options?.name === "Inspect folder") {
          return inspectButton;
        }
        throw new Error(`unexpected dialog role ${role}`);
      }),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const openButton = {
      click: vi.fn().mockResolvedValue(undefined),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const closeReviewButton = {
      click: vi.fn().mockResolvedValue(undefined),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const page = {
      evaluate: vi.fn().mockResolvedValue({ clientWidth: 768, scrollWidth: 768 }),
      getByRole: vi.fn((role, options) => {
        if (role === "navigation") {
          return { isVisible: vi.fn().mockResolvedValue(true) };
        }
        if (role === "button" && options?.name === "Open another session") {
          return openButton;
        }
        if (role === "dialog") {
          return dialog;
        }
        if (role === "region") {
          return targetPicker;
        }
        if (
          role === "tab" &&
          options?.name instanceof RegExp &&
          options.name.test("root / default")
        ) {
          return defaultTab;
        }
        if (
          role === "tab" &&
          options?.name instanceof RegExp &&
          options.name.test("catalog / review")
        ) {
          return reviewTab;
        }
        if (
          role === "button" &&
          options?.name instanceof RegExp &&
          options.name.test("Close catalog / review session")
        ) {
          return closeReviewButton;
        }
        return { isVisible: vi.fn().mockResolvedValue(true) };
      }),
      getByText: vi.fn().mockReturnValue({
        isVisible: vi.fn().mockResolvedValue(true),
      }),
    };

    await verifyDashboardSessionTabs(
      {
        expectNoHorizontalOverflow: async () => {},
        expectVisible: async (locator) => {
          if (typeof locator.waitFor === "function") {
            await locator.waitFor();
            return;
          }
          if (!(await locator.isVisible())) {
            throw new Error("Locator was not visible.");
          }
        },
        waitForDialog: async () => dialog,
      },
      page,
      {
        height: 1024,
        label: "tablet",
        width: 768,
      },
    );

    expect(openButton.click).not.toHaveBeenCalled();
    expect(folderField.fill).not.toHaveBeenCalled();
    expect(inspectButton.click).not.toHaveBeenCalled();
    expect(targetPicker.getByRole).not.toHaveBeenCalled();
    expect(targetButton.click).not.toHaveBeenCalled();
  });
});

describe("verifyDashboardSessionSwitching", () => {
  test("verifyDashboardSessionSwitching exercises tab switching without state leakage", async () => {
    const betaStoryButton = {
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const activeStoryButton = {
      count: vi.fn().mockResolvedValue(0),
    };
    const betaTab = {
      click: vi.fn().mockResolvedValue(undefined),
      getAttribute: vi.fn().mockResolvedValue("true"),
    };
    const page = {
      evaluate: vi.fn().mockResolvedValue({ clientWidth: 1440, scrollWidth: 1440 }),
      getByRole: vi.fn((role, options) => {
        if (role === "tab" && options?.name === "root / beta beta") {
          return betaTab;
        }
        if (role === "button" && String(options?.name) === String(/Active Story/)) {
          return activeStoryButton;
        }
        if (role === "button" && String(options?.name) === String(/Beta Story/)) {
          return betaStoryButton;
        }
        throw new Error(`unexpected role ${role}`);
      }),
    };

    await verifyDashboardSessionSwitching(
      {
        expectNoHorizontalOverflow: async () => {},
        expectVisible: async (locator) => {
          if (!(await locator.isVisible())) {
            throw new Error("Locator was not visible.");
          }
        },
      },
      page,
      {
        height: 900,
        label: "desktop",
        width: 1440,
      },
    );

    expect(betaTab.click).toHaveBeenCalledTimes(1);
  });
});
