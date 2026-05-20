import { describe, expect, test, vi } from "vitest";
import {
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  expectOrderedLeftEdges,
  expectVisible,
  verifyExportDialog,
  verifyImportDialog,
  verifyProviderSessionDetailSuccess,
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
    const goto = vi.fn().mockResolvedValue(undefined);
    const dialog = {
      waitFor: vi.fn().mockResolvedValue(undefined),
    };
    const getByRole = vi.fn((role, options) => ({
      ...(role === "dialog" && options?.name === "Export factory"
        ? dialog
        : { waitFor: vi.fn().mockResolvedValue(undefined) }),
    }));
    const page = {
      getByRole,
      goto,
      waitForFunction,
      waitForSelector,
    };
    const context = {
      close: vi.fn().mockResolvedValue(undefined),
      newPage: vi.fn().mockResolvedValue(page),
    };
    const browser = {
      newContext: vi.fn().mockResolvedValue(context),
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

    expect(browser.newContext).toHaveBeenCalledWith({
      viewport: { height: 844, width: 390 },
    });
    expect(context.newPage).toHaveBeenCalledTimes(1);
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
    expect(context.close).toHaveBeenCalledTimes(1);
  });

  test("closes the isolated context when story assertions fail", async () => {
    const page = {
      getByRole: vi.fn().mockReturnValue({
        waitFor: vi.fn().mockResolvedValue(undefined),
      }),
      goto: vi.fn().mockResolvedValue(undefined),
      waitForFunction: vi.fn().mockResolvedValue(undefined),
      waitForSelector: vi.fn().mockResolvedValue(undefined),
    };
    const context = {
      close: vi.fn().mockResolvedValue(undefined),
      newPage: vi.fn().mockResolvedValue(page),
    };
    const browser = {
      newContext: vi.fn().mockResolvedValue(context),
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

    expect(context.close).toHaveBeenCalledTimes(1);
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
});

describe("import story assertions", () => {
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

describe("provider-session story assertions", () => {
  test("verifyProviderSessionDetailSuccess checks the provider-session success panel", async () => {
    const selectedSessionHeading = { waitFor: vi.fn().mockResolvedValue(undefined) };
    const sourceHeading = { waitFor: vi.fn().mockResolvedValue(undefined) };
    const sourcePath = { waitFor: vi.fn().mockResolvedValue(undefined) };
    const tokenUsageHeading = { waitFor: vi.fn().mockResolvedValue(undefined) };
    const page = {
      evaluate: vi
        .fn()
        .mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
      getByRole: vi.fn((role, options) => {
        if (role !== "heading") {
          throw new Error(`unexpected page role lookup ${role} ${options?.name ?? ""}`);
        }
        if (options?.name === "Selected session details") {
          return selectedSessionHeading;
        }
        if (options?.name === "Source file") {
          return sourceHeading;
        }
        if (options?.name === "Token usage") {
          return tokenUsageHeading;
        }
        throw new Error(`unexpected heading lookup ${options?.name ?? ""}`);
      }),
      getByText: vi.fn((text) => {
        if (
          text ===
          "2026/05/20/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl"
        ) {
          return sourcePath;
        }
        throw new Error(`unexpected current selection text lookup ${text}`);
      }),
    };

    await verifyProviderSessionDetailSuccess(page, null, {
      height: 844,
      label: "mobile",
      width: 390,
    });

    expect(page.getByRole).toHaveBeenCalledWith("heading", {
      name: "Selected session details",
    });
    expect(page.getByRole).toHaveBeenCalledWith("heading", {
      name: "Source file",
    });
    expect(page.getByRole).toHaveBeenCalledWith("heading", {
      name: "Token usage",
    });
  });
});
