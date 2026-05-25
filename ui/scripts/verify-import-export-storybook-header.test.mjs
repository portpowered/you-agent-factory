import { describe, expect, test, vi } from "vitest";

import { verifyDashboardHeader } from "./verify-import-export-storybook-responsive.mjs";
import { verifyDashboardSessionSwitching } from "./verify-dashboard-session-switching-storybook-responsive.mjs";
import { verifyDashboardSessionTabs } from "./verify-dashboard-session-tabs-storybook-responsive.mjs";

function createCurrentTickLocator() {
  return {
    boundingBox: vi
      .fn()
      .mockResolvedValue({ height: 20, width: 40, x: 380, y: 40 }),
    isVisible: vi.fn().mockResolvedValue(true),
  };
}

function createLocator(overrides = {}) {
  return {
    isVisible: vi.fn().mockResolvedValue(true),
    ...overrides,
  };
}

function createDesktopLocator(isDesktop, box, overrides = {}) {
  return createLocator({
    ...(isDesktop
      ? {
          boundingBox: vi.fn().mockResolvedValue(box),
        }
      : {}),
    ...overrides,
  });
}

function createHeaderFixture(isDesktop, headingWordmarkClassName) {
  return {
    heading: createDesktopLocator(
      isDesktop,
      { height: 20, width: 120, x: 10, y: 0 },
      {
        getByText: vi.fn().mockReturnValue(
          createLocator({
            getAttribute: vi.fn().mockResolvedValue(headingWordmarkClassName),
          }),
        ),
      },
    ),
    slider: createDesktopLocator(
      isDesktop,
      { height: 20, width: 200, x: 160, y: 0 },
      { focus: vi.fn().mockResolvedValue(undefined) },
    ),
    sessionTabs: createDesktopLocator(isDesktop, {
      height: 20,
      width: 340,
      x: 150,
      y: 0,
    }),
    languageButton: createDesktopLocator(isDesktop, {
      height: 20,
      width: 120,
      x: 660,
      y: 0,
    }),
    streamStatus: createDesktopLocator(isDesktop, {
      height: 20,
      width: 120,
      x: 380,
      y: 0,
    }),
    exportButton: createDesktopLocator(isDesktop, {
      height: 20,
      width: 120,
      x: 520,
      y: 0,
    }),
  };
}

function createRoleLookup({
  closeSelectedSessionButton,
  currentButton,
  exportButton,
  globalActions,
  heading,
  languageButton,
  openSessionButton,
  rootTab,
  sessionTabs,
  slider,
  streamStatus,
}) {
  return vi.fn((role, options) => {
    if (role === "heading") return heading;
    if (role === "slider") return slider;
    if (role === "navigation") return sessionTabs;
    if (role === "tab" && options == null) return { count: vi.fn().mockResolvedValue(3) };
    if (role === "tab" && options?.name === "root") return rootTab;
    if (role === "button" && options?.name === "Open another session") {
      return openSessionButton;
    }
    if (role === "button" && options?.name === "Close root session") {
      return closeSelectedSessionButton;
    }
    if (options?.name === "Change language") return languageButton;
    if (role === "status") return streamStatus;
    if (role === "group") return globalActions;
    if (options?.name === "Return to current tick") return currentButton;
    return exportButton;
  });
}

function createTextLookup(timelineStatus) {
  return vi.fn((text) => {
    if (text === "/workspace/root") {
      return createDesktopLocator(true, {
        height: 20,
        width: 120,
        x: 150,
        y: 0,
      });
    }
    return createDesktopLocator(true, {
      height: 20,
      width: 40,
      x: 380,
      y: 40,
    }, {
      first: vi.fn().mockReturnValue(timelineStatus),
    });
  });
}

function createPage({
  timelineStatus = createCurrentTickLocator(),
  headingWordmarkClassName = "truncate text-lg font-semibold",
  isDesktop = false,
  returnToCurrentVisible = false,
} = {}) {
  const { exportButton, heading, languageButton, sessionTabs, slider, streamStatus } =
    createHeaderFixture(isDesktop, headingWordmarkClassName);
  const rootTab = createLocator();
  const openSessionButton = createLocator();
  const closeSelectedSessionButton = createLocator();
  const currentButton = createLocator({
    isVisible: vi.fn().mockResolvedValue(returnToCurrentVisible),
  });
  const globalActions = createLocator();
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
    getByRole: createRoleLookup({
      closeSelectedSessionButton,
      currentButton,
      exportButton,
      globalActions,
      heading,
      languageButton,
      openSessionButton,
      rootTab,
      sessionTabs,
      slider,
      streamStatus,
    }),
    getByText: createTextLookup(timelineStatus),
  };

  return page;
}

describe("verifyDashboardHeader", () => {
  test("verifyDashboardHeader exercises keyboard and desktop ordering checks", async () => {
    const timelineStatus = createCurrentTickLocator();
    const page = createPage({ timelineStatus, isDesktop: true });

    await verifyDashboardHeader(page, null, {
      height: 900,
      label: "desktop",
      width: 1440,
    });

    expect(page.heading.getByText).toHaveBeenCalledWith("You Agent Factory");
    expect(page.slider.focus).not.toHaveBeenCalled();
    expect(page.keyboard.press).not.toHaveBeenCalled();
  });

  test("verifyDashboardHeader rejects a hidden sr-only heading wordmark", async () => {
    const page = createPage({ headingWordmarkClassName: "sr-only" });

    await expect(
      verifyDashboardHeader(page, null, {
        height: 844,
        label: "mobile",
        width: 390,
      }),
    ).rejects.toThrow(
      "Dashboard heading wordmark should remain visible instead of sr-only.",
    );
  });

  test("verifyDashboardHeader rejects the retired return-to-current button when it is still visible", async () => {
    const page = createPage({ returnToCurrentVisible: true });

    await expect(
      verifyDashboardHeader(page, null, {
        height: 844,
        label: "mobile",
        width: 390,
      }),
    ).rejects.toThrow(
      "Dashboard header still rendered the retired return-to-current button.",
    );
  });
});

describe("verifyDashboardSessionTabs", () => {
  test("verifyDashboardSessionTabs verifies the visible tab strip", async () => {
    const defaultTab = {
      getAttribute: vi.fn().mockResolvedValue("true"),
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const betaTab = {
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const closeRootButton = {
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const openButton = {
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const closeBetaButton = {
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const page = {
      evaluate: vi.fn().mockResolvedValue({ clientWidth: 768, scrollWidth: 768 }),
      getByRole: vi.fn((role, options) => {
        if (role === "navigation") {
          return { isVisible: vi.fn().mockResolvedValue(true) };
        }
        if (role === "tab" && options?.name === "root") {
          return defaultTab;
        }
        if (role === "tab" && options?.name === "beta") {
          return betaTab;
        }
        if (role === "button" && options?.name === "Close root session") {
          return closeRootButton;
        }
        if (role === "button" && options?.name === "Open another session") {
          return openButton;
        }
        if (role === "button" && options?.name === "Close beta session") {
          return closeBetaButton;
        }
        return { isVisible: vi.fn().mockResolvedValue(true) };
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
      },
      page,
      {
        height: 1024,
        label: "tablet",
        width: 768,
      },
    );

    expect(page.getByRole).toHaveBeenCalledWith("tab", { name: "root" });
    expect(page.getByRole).toHaveBeenCalledWith("tab", { name: "beta" });
    expect(page.getByRole).toHaveBeenCalledWith("button", {
      name: "Close root session",
    });
    expect(page.getByRole).toHaveBeenCalledWith("button", {
      name: "Open another session",
    });
    expect(page.getByRole).toHaveBeenCalledWith("button", {
      name: "Close beta session",
    });
  });
});

describe("verifyDashboardSessionSwitching", () => {
  test("verifies switching between multiple dashboard session tabs", async () => {
    const rootTab = {
      isVisible: vi.fn().mockResolvedValue(true),
      click: vi.fn().mockResolvedValue(undefined),
    };
    const betaTab = {
      isVisible: vi.fn().mockResolvedValue(true),
      click: vi.fn().mockResolvedValue(undefined),
      getAttribute: vi.fn().mockResolvedValue("true"),
    };
    const rootPanel = {
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const betaPanel = {
      isVisible: vi.fn().mockResolvedValue(true),
    };
    const activeStoryButton = {
      count: vi.fn().mockResolvedValue(0),
    };
    const page = {
      getByRole: vi.fn((role, options) => {
        if (role === "tab" && options?.name === "root") {
          return rootTab;
        }
        if (role === "tab" && options?.name === "beta") {
          return betaTab;
        }
        if (role === "tabpanel" && options?.name === "root") {
          return rootPanel;
        }
        if (role === "tabpanel" && options?.name === "beta") {
          return betaPanel;
        }
        if (role === "button" && String(options?.name) === "/Active Story/") {
          return activeStoryButton;
        }
        return createLocator();
      }),
    };
    const expectNoHorizontalOverflow = vi.fn().mockResolvedValue(undefined);

    await verifyDashboardSessionSwitching(
      {
        expectNoHorizontalOverflow,
        expectVisible: async (locator) => {
          if (!(await locator.isVisible())) {
            throw new Error("Locator was not visible.");
          }
        },
      },
      page,
      { label: "desktop" },
    );

    expect(rootTab.click).not.toHaveBeenCalled();
    expect(betaTab.click).toHaveBeenCalledTimes(1);
    expect(expectNoHorizontalOverflow).toHaveBeenCalledWith(
      page,
      "Dashboard session switching at desktop",
    );
  });
});
