import { describe, expect, test, vi } from "vitest";

import {
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  expectOrderedLeftEdges,
  expectVisible,
  verifyDashboardHeader,
  verifyExportDialog,
  verifyImportDialog,
  verifyStory,
  waitForDialog,
  waitForStoryRegion,
  waitForStoryRender,
} from "./verify-import-export-storybook-responsive.mjs";

describe("waitForStoryRender", () => {
  test("waits for the Storybook root selector and rendered children", async () => {
    const page = {
      waitForFunction: vi.fn().mockResolvedValue(undefined),
      waitForSelector: vi.fn().mockResolvedValue(undefined),
    };

    await waitForStoryRender(page);

    expect(page.waitForSelector).toHaveBeenCalledWith("#storybook-root", {
      state: "attached",
      timeout: 30000,
    });
    expect(page.waitForFunction).toHaveBeenCalledTimes(1);
    expect(page.waitForFunction.mock.calls[0]?.[1]).toEqual({
      timeout: 30000,
    });
  });

  test("treats portal-rendered story content as ready even when the root stays empty", async () => {
    const page = {
      waitForFunction: vi.fn().mockResolvedValue(undefined),
      waitForSelector: vi.fn().mockResolvedValue(undefined),
    };

    await waitForStoryRender(page);

    const readyCheck = page.waitForFunction.mock.calls[0]?.[0];
    if (typeof readyCheck !== "function") {
      throw new Error("expected Storybook readiness predicate");
    }

    document.body.innerHTML = `
      <div id="storybook-root"></div>
      <div role="dialog" aria-label="Export factory"></div>
    `;
    expect(readyCheck()).toBe(true);
  });
});

describe("verifyStory", () => {
  test("uses a fresh page per story and closes it after assertions", async () => {
    const waitForSelector = vi.fn().mockResolvedValue(undefined);
    const waitForFunction = vi.fn().mockResolvedValue(undefined);
    const setViewportSize = vi.fn().mockResolvedValue(undefined);
    const goto = vi.fn().mockResolvedValue(undefined);
    const close = vi.fn().mockResolvedValue(undefined);
    const dialog = {
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const getByRole = vi.fn((role, options) => ({
      ...(role === "dialog" && options?.name === "Export factory"
        ? dialog
        : { waitFor: vi.fn().mockResolvedValue(undefined) }),
    }));
    const page = {
      close,
      getByRole,
      goto,
      setViewportSize,
      waitForFunction,
      waitForSelector,
    };
    const browser = {
      newPage: vi.fn().mockResolvedValue(page),
    };
    const assertions = vi.fn().mockResolvedValue(undefined);

    await verifyStory(
      browser,
      {
        assertions,
        dialogName: "Export factory",
        id: "infinite-you-dashboard-export-factory-dialog--ready",
      },
      { height: 844, label: "mobile", width: 390 },
    );

    expect(browser.newPage).toHaveBeenCalledTimes(1);
    expect(setViewportSize).toHaveBeenCalledWith({ height: 844, width: 390 });
    expect(goto).toHaveBeenCalledWith(
      "http://127.0.0.1:6008/iframe.html?id=infinite-you-dashboard-export-factory-dialog--ready&viewMode=story",
      { waitUntil: "domcontentloaded" },
    );
    expect(getByRole).toHaveBeenCalledWith("dialog", {
      name: "Export factory",
    });
    expect(assertions).toHaveBeenCalledWith(page, dialog, {
      height: 844,
      label: "mobile",
      width: 390,
    });
    expect(close).toHaveBeenCalledTimes(1);
  });

  test("closes the page when story assertions fail", async () => {
    const page = {
      close: vi.fn().mockResolvedValue(undefined),
      getByRole: vi.fn().mockReturnValue({
        waitFor: vi.fn().mockResolvedValue(undefined),
      }),
      goto: vi.fn().mockResolvedValue(undefined),
      setViewportSize: vi.fn().mockResolvedValue(undefined),
      waitForFunction: vi.fn().mockResolvedValue(undefined),
      waitForSelector: vi.fn().mockResolvedValue(undefined),
    };
    const browser = {
      newPage: vi.fn().mockResolvedValue(page),
    };
    const failure = new Error("story failed");

    await expect(
      verifyStory(
        browser,
        {
          assertions: vi.fn().mockRejectedValue(failure),
          dialogName: "Export factory",
          id: "infinite-you-dashboard-export-factory-dialog--ready",
        },
        { height: 844, label: "mobile", width: 390 },
      ),
    ).rejects.toThrow("story failed");

    expect(page.close).toHaveBeenCalledTimes(1);
  });
});

describe("story locators", () => {
  test("waitForDialog waits for a named visible dialog", async () => {
    const dialog = {
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const page = {
      getByRole: vi.fn().mockReturnValue(dialog),
    };

    const result = await waitForDialog(page, "Export factory");

    expect(page.getByRole).toHaveBeenCalledWith("dialog", {
      name: "Export factory",
    });
    expect(dialog.waitFor).toHaveBeenCalledWith({ state: "visible" });
    expect(result).toBe(dialog);
  });

  test("waitForStoryRegion waits for a named visible region", async () => {
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
});

describe("viewport assertions", () => {
  test("expectNoHorizontalOverflow accepts matching widths", async () => {
    const page = {
      evaluate: vi.fn().mockResolvedValue({ clientWidth: 390, scrollWidth: 391 }),
    };

    await expect(
      expectNoHorizontalOverflow(page, "mobile export dialog"),
    ).resolves.toBeUndefined();
  });

  test("expectNoHorizontalOverflow rejects horizontal overflow beyond tolerance", async () => {
    const page = {
      evaluate: vi.fn().mockResolvedValue({ clientWidth: 390, scrollWidth: 393 }),
    };

    await expect(
      expectNoHorizontalOverflow(page, "mobile export dialog"),
    ).rejects.toThrow(
      "mobile export dialog overflowed horizontally: scrollWidth=393, clientWidth=390.",
    );
  });

  test("expectDialogWithinViewport accepts in-bounds dialogs", async () => {
    const dialog = {
      boundingBox: vi.fn().mockResolvedValue({
        height: 400,
        width: 360,
        x: 15,
        y: 30,
      }),
    };

    await expect(
      expectDialogWithinViewport(dialog, { height: 844, label: "mobile", width: 390 }, "Export"),
    ).resolves.toBeUndefined();
  });

  test("expectDialogWithinViewport rejects missing bounds", async () => {
    const dialog = {
      boundingBox: vi.fn().mockResolvedValue(null),
    };

    await expect(
      expectDialogWithinViewport(dialog, { height: 844, label: "mobile", width: 390 }, "Export"),
    ).rejects.toThrow("Could not measure Export dialog bounds.");
  });

  test("expectDialogWithinViewport rejects overflowing dialogs", async () => {
    const dialog = {
      boundingBox: vi.fn().mockResolvedValue({
        height: 850,
        width: 380,
        x: 8,
        y: 4,
      }),
    };

    await expect(
      expectDialogWithinViewport(dialog, { height: 844, label: "mobile", width: 390 }, "Export"),
    ).rejects.toThrow("Export dialog exceeded the mobile viewport (390x844).");
  });

  test("expectVisible accepts visible locators", async () => {
    await expect(
      expectVisible(
        { isVisible: vi.fn().mockResolvedValue(true) },
        "Export action button",
      ),
    ).resolves.toBeUndefined();
  });

  test("expectVisible rejects hidden locators", async () => {
    await expect(
      expectVisible(
        { isVisible: vi.fn().mockResolvedValue(false) },
        "Export action button",
      ),
    ).rejects.toThrow("Export action button was not visible.");
  });

  test("expectOrderedLeftEdges accepts left-to-right boxes", async () => {
    const locators = [
      { boundingBox: vi.fn().mockResolvedValue({ height: 10, width: 20, x: 10, y: 0 }) },
      { boundingBox: vi.fn().mockResolvedValue({ height: 10, width: 20, x: 31, y: 0 }) },
    ];

    await expect(
      expectOrderedLeftEdges(locators, "Header controls"),
    ).resolves.toBeUndefined();
  });

  test("expectOrderedLeftEdges rejects missing measurements", async () => {
    await expect(
      expectOrderedLeftEdges(
        [{ boundingBox: vi.fn().mockResolvedValue(null) }],
        "Header controls",
      ),
    ).rejects.toThrow("Could not measure Header controls.");
  });

  test("expectOrderedLeftEdges rejects overlapping order", async () => {
    const locators = [
      { boundingBox: vi.fn().mockResolvedValue({ height: 10, width: 40, x: 20, y: 0 }) },
      { boundingBox: vi.fn().mockResolvedValue({ height: 10, width: 30, x: 50, y: 0 }) },
    ];

    await expect(
      expectOrderedLeftEdges(locators, "Header controls"),
    ).rejects.toThrow("Header controls was not ordered left-to-right.");
  });
});

describe("story assertions", () => {
  test("verifyExportDialog checks the expected export controls", async () => {
    const textbox = { isVisible: vi.fn().mockResolvedValue(true) };
    const coverImage = { isVisible: vi.fn().mockResolvedValue(true) };
    const cancelButton = { isVisible: vi.fn().mockResolvedValue(true) };
    const exportButton = { isVisible: vi.fn().mockResolvedValue(true) };
    const helperCopy = { isVisible: vi.fn().mockResolvedValue(true) };
    const dialog = {
      boundingBox: vi.fn().mockResolvedValue({
        height: 500,
        width: 360,
        x: 12,
        y: 24,
      }),
      getByLabel: vi.fn().mockReturnValue(coverImage),
      getByRole: vi.fn((role, options) => {
        if (role === "textbox") {
          return textbox;
        }
        if (options?.name === "Cancel") {
          return cancelButton;
        }
        return exportButton;
      }),
      getByText: vi.fn().mockReturnValue(helperCopy),
    };
    const page = {
      evaluate: vi.fn().mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
    };

    await verifyExportDialog(page, dialog, {
      height: 844,
      label: "mobile",
      width: 390,
    });

    expect(dialog.getByRole).toHaveBeenCalledWith("textbox", {
      name: "Factory name",
    });
    expect(dialog.getByLabel).toHaveBeenCalledWith("Cover image");
    expect(dialog.getByRole).toHaveBeenCalledWith("button", { name: "Cancel" });
    expect(dialog.getByRole).toHaveBeenCalledWith("button", {
      name: "Export PNG",
    });
  });

  test("verifyImportDialog checks the expected import controls", async () => {
    const previewImage = { isVisible: vi.fn().mockResolvedValue(true) };
    const fileName = { isVisible: vi.fn().mockResolvedValue(true) };
    const cancelButton = { isVisible: vi.fn().mockResolvedValue(true) };
    const activateButton = { isVisible: vi.fn().mockResolvedValue(true) };
    const closeButton = { isVisible: vi.fn().mockResolvedValue(true) };
    const dialog = {
      boundingBox: vi.fn().mockResolvedValue({
        height: 500,
        width: 360,
        x: 12,
        y: 24,
      }),
      getByRole: vi.fn((role, options) => {
        if (role === "img") {
          return previewImage;
        }
        if (options?.name === "Cancel import") {
          return cancelButton;
        }
        if (options?.name === "Activate factory") {
          return activateButton;
        }
        return closeButton;
      }),
      getByText: vi.fn().mockReturnValue(fileName),
    };
    const page = {
      evaluate: vi.fn().mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
    };

    await verifyImportDialog(page, dialog, {
      height: 844,
      label: "mobile",
      width: 390,
    });

    expect(dialog.getByRole).toHaveBeenCalledWith("img", {
      name: "Dropped Factory preview",
    });
    expect(dialog.getByText).toHaveBeenCalledWith("factory-import.png");
    expect(dialog.getByRole).toHaveBeenCalledWith("button", {
      name: "Activate factory",
    });
  });
});

describe("verifyDashboardHeader", () => {
  test("verifyDashboardHeader exercises keyboard and desktop ordering checks", async () => {
    const heading = {
      boundingBox: vi.fn().mockResolvedValue({ height: 20, width: 120, x: 10, y: 0 }),
      getByText: vi.fn().mockReturnValue({
        getAttribute: vi.fn().mockResolvedValue("sr-only"),
        isVisible: vi.fn().mockResolvedValue(true),
      }),
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
    const toolbar = {
      getByRole: vi.fn((role, options) => {
        if (role === "heading") {
          return heading;
        }
        if (role === "slider") {
          return slider;
        }
        if (role === "status") {
          return streamStatus;
        }
        if (options?.name === "Return to current tick") {
          return currentButton;
        }
        return exportButton;
      }),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const page = {
      evaluate: vi.fn().mockResolvedValue({ clientWidth: 1440, scrollWidth: 1440 }),
      getByRole: vi.fn().mockReturnValue(toolbar),
      getByText: vi
        .fn()
        .mockReturnValueOnce(currentTick)
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

    expect(page.getByRole).toHaveBeenCalledWith("region", {
      name: "dashboard summary",
    });
    expect(slider.focus).toHaveBeenCalledTimes(1);
    expect(currentButton.focus).toHaveBeenCalledTimes(1);
    expect(page.keyboard.press).toHaveBeenNthCalledWith(1, "ArrowLeft");
    expect(page.keyboard.press).toHaveBeenNthCalledWith(2, "Enter");
  });

  test("verifyDashboardHeader rejects a non-sr-only heading wordmark", async () => {
    const toolbar = {
      getByRole: vi.fn((role, _options) => {
        if (role === "heading") {
          return {
            getByText: vi.fn().mockReturnValue({
              getAttribute: vi.fn().mockResolvedValue("text-visible"),
              isVisible: vi.fn().mockResolvedValue(true),
            }),
            isVisible: vi.fn().mockResolvedValue(true),
          };
        }
        if (role === "slider") {
          return { focus: vi.fn().mockResolvedValue(undefined), isVisible: vi.fn().mockResolvedValue(true) };
        }
        if (role === "status") {
          return { isVisible: vi.fn().mockResolvedValue(true) };
        }
        return { isVisible: vi.fn().mockResolvedValue(true) };
      }),
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const page = {
      evaluate: vi.fn().mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
      getByRole: vi.fn().mockReturnValue(toolbar),
      getByText: vi.fn().mockReturnValue({ isVisible: vi.fn().mockResolvedValue(true) }),
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
