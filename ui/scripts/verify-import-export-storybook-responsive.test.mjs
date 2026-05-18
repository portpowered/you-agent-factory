import { describe, expect, test, vi } from "vitest";

import {
  verifyStory,
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
});
