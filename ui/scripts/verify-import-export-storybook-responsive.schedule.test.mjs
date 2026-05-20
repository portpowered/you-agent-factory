import { describe, expect, test, vi } from "vitest";

import {
  runResponsiveStorybookChecks,
  storyChecks,
  verifyLocalizedExportDialog,
  verifyLocalizedImportDialog,
  viewportChecks,
} from "./verify-import-export-storybook-responsive.mjs";

describe("import/export responsive schedule", () => {
  test("keeps the import/export stories and viewport sizes in the default schedule", () => {
    const importExportStories = storyChecks.filter((storyCheck) =>
      storyCheck.label.includes("dialog"),
    );

    expect(importExportStories.map((storyCheck) => storyCheck.id)).toEqual([
      "infinite-you-dashboard-export-factory-dialog--ready",
      "infinite-you-dashboard-export-factory-dialog--localized-zh-cn",
      "infinite-you-dashboard-import-preview-dialog--ready",
      "infinite-you-dashboard-import-preview-dialog--localized-zh-cn",
    ]);
    expect(
      importExportStories.map((storyCheck) => storyCheck.dialogName),
    ).toEqual([
      "Export factory",
      "导出工厂",
      "Review factory import",
      "检查工厂导入",
    ]);
    expect(viewportChecks).toEqual([
      { height: 844, label: "mobile", width: 390 },
      { height: 1024, label: "tablet", width: 768 },
      { height: 900, label: "desktop", width: 1440 },
    ]);
  });

  test("runs every requested story at every requested viewport", async () => {
    const visitedUrls = [];
    const visitedViewports = [];
    const page = {
      close: vi.fn().mockResolvedValue(undefined),
      goto: vi.fn((url) => {
        visitedUrls.push(url);
        return Promise.resolve();
      }),
      setViewportSize: vi.fn((viewport) => {
        visitedViewports.push(viewport);
        return Promise.resolve();
      }),
      waitForFunction: vi.fn().mockResolvedValue(undefined),
      waitForSelector: vi.fn().mockResolvedValue(undefined),
    };
    const browser = {
      newPage: vi.fn().mockResolvedValue(page),
    };
    const checks = [
      { assertions: vi.fn().mockResolvedValue(undefined), id: "story-one" },
      { assertions: vi.fn().mockResolvedValue(undefined), id: "story-two" },
    ];
    const viewports = [
      { height: 111, label: "narrow", width: 222 },
      { height: 333, label: "wide", width: 444 },
    ];

    await runResponsiveStorybookChecks(browser, { checks, viewports });

    expect(visitedViewports).toEqual([
      { height: 111, width: 222 },
      { height: 111, width: 222 },
      { height: 333, width: 444 },
      { height: 333, width: 444 },
    ]);
    expect(visitedUrls).toEqual([
      "http://127.0.0.1:6008/iframe.html?id=story-one&viewMode=story",
      "http://127.0.0.1:6008/iframe.html?id=story-two&viewMode=story",
      "http://127.0.0.1:6008/iframe.html?id=story-one&viewMode=story",
      "http://127.0.0.1:6008/iframe.html?id=story-two&viewMode=story",
    ]);
    expect(checks[0].assertions).toHaveBeenCalledTimes(2);
    expect(checks[1].assertions).toHaveBeenCalledTimes(2);
    expect(page.close).toHaveBeenCalledTimes(4);
  });
});

describe("localized import/export assertions", () => {
  test("checks the expected localized export controls", async () => {
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
        if (options?.name === "取消") {
          return cancelButton;
        }
        return exportButton;
      }),
      getByText: vi.fn().mockReturnValue(helperCopy),
    };
    const page = {
      evaluate: vi
        .fn()
        .mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
    };

    await verifyLocalizedExportDialog(page, dialog, {
      height: 844,
      label: "mobile",
      width: 390,
    });

    expect(dialog.getByRole).toHaveBeenCalledWith("textbox", {
      name: "工厂名称",
    });
    expect(dialog.getByLabel).toHaveBeenCalledWith("封面图片");
    expect(dialog.getByRole).toHaveBeenCalledWith("button", { name: "取消" });
    expect(dialog.getByRole).toHaveBeenCalledWith("button", {
      name: "导出 PNG",
    });
    expect(dialog.getByText).toHaveBeenCalledWith(
      "确认导出不会更改当前仪表板状态",
    );
  });

  test("checks the expected localized import controls", async () => {
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
        if (options?.name === "取消导入") {
          return cancelButton;
        }
        if (options?.name === "启用工厂") {
          return activateButton;
        }
        return closeButton;
      }),
      getByText: vi.fn().mockReturnValue(fileName),
    };
    const page = {
      evaluate: vi
        .fn()
        .mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
    };

    await verifyLocalizedImportDialog(page, dialog, {
      height: 844,
      label: "mobile",
      width: 390,
    });

    expect(dialog.getByRole).toHaveBeenCalledWith("img", {
      name: "Dropped Factory 预览图",
    });
    expect(dialog.getByText).toHaveBeenCalledWith("factory-import.png");
    expect(dialog.getByRole).toHaveBeenCalledWith("button", {
      name: "取消导入",
    });
    expect(dialog.getByRole).toHaveBeenCalledWith("button", {
      name: "启用工厂",
    });
    expect(dialog.getByRole).toHaveBeenCalledWith("button", {
      name: "关闭导入预览",
    });
  });
});
