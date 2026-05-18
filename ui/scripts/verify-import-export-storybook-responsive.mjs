import process from "node:process";
import { chromium } from "playwright";
import {
  verifyLocalizedSubmitWorkCard,
  verifyLocalizedTraceGrid,
  verifyLocalizedWorkflowActivity,
  verifyLocalizedWorkOutcomeChart,
} from "./verify-localized-widget-storybook-responsive.mjs";

const STORYBOOK_HOST = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const STORYBOOK_PORT = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const STORYBOOK_URL = `http://${STORYBOOK_HOST}:${STORYBOOK_PORT}`;
const OVERFLOW_TOLERANCE_PX = 1;

const viewportChecks = [
  { height: 844, label: "mobile", width: 390 },
  { height: 1024, label: "tablet", width: 768 },
  { height: 900, label: "desktop", width: 1440 },
];

const storyChecks = [
  {
    assertions: verifyExportDialog,
    dialogName: "Export factory",
    id: "infinite-you-dashboard-export-factory-dialog--ready",
    label: "export dialog (en)",
  },
  {
    assertions: verifyLocalizedExportDialog,
    dialogName: "导出工厂",
    id: "infinite-you-dashboard-export-factory-dialog--localized-zh-cn",
    label: "export dialog (zh-CN)",
  },
  {
    assertions: verifyImportDialog,
    dialogName: "Review factory import",
    id: "infinite-you-dashboard-import-preview-dialog--ready",
    label: "import preview dialog (en)",
  },
  {
    assertions: verifyLocalizedImportDialog,
    dialogName: "检查工厂导入",
    id: "infinite-you-dashboard-import-preview-dialog--localized-zh-cn",
    label: "import preview dialog (zh-CN)",
  },
  {
    assertions: verifyDashboardHeader,
    id: "infinite-you-workflow-dashboard--header-timeline-alignment-verification",
    label: "dashboard header",
  },
  {
    assertions: (page, _dialog, viewport) =>
      verifyLocalizedSubmitWorkCard({
        expectNoHorizontalOverflow,
        expectVisible,
        page,
        viewport,
      }),
    id: "agent-factory-dashboard-submit-work-card--localized-zh-cn",
    label: "submit work widget (zh-CN)",
  },
  {
    assertions: (page, _dialog, viewport) =>
      verifyLocalizedTraceGrid({
        expectNoHorizontalOverflow,
        expectVisible,
        page,
        viewport,
      }),
    id: "agent-factory-dashboard-trace-grid-bento-card--localized-zh-cn",
    label: "trace drilldown widget (zh-CN)",
  },
  {
    assertions: (page, _dialog, viewport) =>
      verifyLocalizedWorkOutcomeChart({
        expectNoHorizontalOverflow,
        expectVisible,
        page,
        viewport,
      }),
    id: "agent-factory-dashboard-work-outcome-chart-card--localized-zh-cn",
    label: "work outcome widget (zh-CN)",
  },
  {
    assertions: (page, _dialog, viewport) =>
      verifyLocalizedWorkflowActivity({
        expectNoHorizontalOverflow,
        expectVisible,
        page,
        viewport,
      }),
    id: "agent-factory-dashboard-react-flow-current-activity-card--localized-zh-cn",
    label: "workflow activity widget (zh-CN)",
  },
];

function storyUrl(storyId) {
  return `${STORYBOOK_URL}/iframe.html?id=${storyId}&viewMode=story`;
}

async function waitForDialog(page, dialogName) {
  const dialog = page.getByRole("dialog", { name: dialogName });
  await dialog.waitFor({ state: "visible" });
  return dialog;
}

async function waitForStoryRegion(page, regionName) {
  const region = page.getByRole("region", { name: regionName });
  await region.waitFor({ state: "visible" });
  return region;
}

async function expectNoHorizontalOverflow(page, label) {
  const metrics = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));

  if (metrics.scrollWidth > metrics.clientWidth + OVERFLOW_TOLERANCE_PX) {
    throw new Error(
      `${label} overflowed horizontally: scrollWidth=${metrics.scrollWidth}, clientWidth=${metrics.clientWidth}.`,
    );
  }
}

async function expectDialogWithinViewport(dialog, viewport, label) {
  const box = await dialog.boundingBox();

  if (!box) {
    throw new Error(`Could not measure ${label} dialog bounds.`);
  }

  const exceedsViewport =
    box.x < -OVERFLOW_TOLERANCE_PX ||
    box.y < -OVERFLOW_TOLERANCE_PX ||
    box.x + box.width > viewport.width + OVERFLOW_TOLERANCE_PX ||
    box.y + box.height > viewport.height + OVERFLOW_TOLERANCE_PX;

  if (exceedsViewport) {
    throw new Error(
      `${label} dialog exceeded the ${viewport.label} viewport (${viewport.width}x${viewport.height}).`,
    );
  }
}

async function expectVisible(locator, label) {
  await locator.scrollIntoViewIfNeeded();
  if (!(await locator.isVisible())) {
    throw new Error(`${label} was not visible.`);
  }
}

async function verifyExportDialog(page, dialog, viewport) {
  await expectVisible(
    dialog.getByRole("textbox", { name: "Factory name" }),
    "Factory name input",
  );
  await expectVisible(dialog.getByLabel("Cover image"), "Cover image input");
  await expectVisible(
    dialog.getByRole("button", { name: "Cancel" }),
    "Export cancel button",
  );
  await expectVisible(
    dialog.getByRole("button", { name: "Export PNG" }),
    "Export action button",
  );
  await expectVisible(
    dialog.getByText(
      "Confirming export keeps the current dashboard state unchanged",
    ),
    "Export helper copy",
  );
  await expectDialogWithinViewport(dialog, viewport, "Export");
  await expectNoHorizontalOverflow(page, `Export dialog at ${viewport.label}`);
}

async function verifyImportDialog(page, dialog, viewport) {
  await expectVisible(
    dialog.getByRole("img", { name: "Dropped Factory preview" }),
    "Import preview image",
  );
  await expectVisible(
    dialog.getByText("factory-import.png"),
    "Dropped file name",
  );
  await expectVisible(
    dialog.getByRole("button", { name: "Cancel import" }),
    "Import cancel button",
  );
  await expectVisible(
    dialog.getByRole("button", { name: "Activate factory" }),
    "Import activate button",
  );
  await expectVisible(
    dialog.getByRole("button", { name: "Close import preview" }),
    "Import close button",
  );
  await expectDialogWithinViewport(dialog, viewport, "Import preview");
  await expectNoHorizontalOverflow(
    page,
    `Import preview dialog at ${viewport.label}`,
  );
}

async function verifyLocalizedExportDialog(page, dialog, viewport) {
  await expectVisible(
    dialog.getByRole("textbox", { name: "工厂名称" }),
    "Localized factory name input",
  );
  await expectVisible(
    dialog.getByLabel("封面图片"),
    "Localized cover image input",
  );
  await expectVisible(
    dialog.getByRole("button", { name: "取消" }),
    "Localized export cancel button",
  );
  await expectVisible(
    dialog.getByRole("button", { name: "导出 PNG" }),
    "Localized export action button",
  );
  await expectVisible(
    dialog.getByText("确认导出不会更改当前仪表板状态"),
    "Localized export helper copy",
  );
  await expectDialogWithinViewport(dialog, viewport, "Localized export");
  await expectNoHorizontalOverflow(
    page,
    `Localized export dialog at ${viewport.label}`,
  );
}

async function verifyLocalizedImportDialog(page, dialog, viewport) {
  await expectVisible(
    dialog.getByRole("img", { name: "Dropped Factory 预览图" }),
    "Localized import preview image",
  );
  await expectVisible(
    dialog.getByText("factory-import.png"),
    "Localized dropped file name",
  );
  await expectVisible(
    dialog.getByRole("button", { name: "取消导入" }),
    "Localized import cancel button",
  );
  await expectVisible(
    dialog.getByRole("button", { name: "启用工厂" }),
    "Localized import activate button",
  );
  await expectVisible(
    dialog.getByRole("button", { name: "关闭导入预览" }),
    "Localized import close button",
  );
  await expectDialogWithinViewport(
    dialog,
    viewport,
    "Localized import preview",
  );
  await expectNoHorizontalOverflow(
    page,
    `Localized import preview dialog at ${viewport.label}`,
  );
}

async function expectOrderedLeftEdges(locators, label) {
  let previousRight = null;

  for (const locator of locators) {
    const box = await locator.boundingBox();
    if (!box) {
      throw new Error(`Could not measure ${label}.`);
    }

    if (
      previousRight !== null &&
      box.x < previousRight - OVERFLOW_TOLERANCE_PX
    ) {
      throw new Error(`${label} was not ordered left-to-right.`);
    }

    previousRight = box.x + box.width;
  }
}

async function verifyDashboardHeader(page, _dialog, viewport) {
  const toolbar = await waitForStoryRegion(page, "dashboard summary");
  const heading = toolbar.getByRole("heading", { name: "Infinite You" });
  const hiddenWordmark = heading.getByText("Infinite You");
  const switcher = page.locator("#dashboard-language-switcher");
  const slider = toolbar.getByRole("slider", { name: "Timeline tick" });
  const streamStatus = toolbar.getByRole("status", {
    name: /Infinite You event stream (connecting|live)/,
  });
  const currentTick = page.getByText("Tick 5 of 5");
  const currentButton = toolbar.getByRole("button", {
    name: "Return to current tick",
  });
  const exportButton = toolbar.getByRole("button", { name: "Export PNG" });

  await expectVisible(heading, "Dashboard heading");
  await expectVisible(hiddenWordmark, "Accessible Infinite You wordmark");
  await expectVisible(switcher, "Dashboard language switcher");
  await expectVisible(
    toolbar.getByText("Language"),
    "Dashboard language label",
  );
  await expectVisible(slider, "Timeline slider");
  await expectVisible(streamStatus, "Dashboard stream status");
  await expectVisible(currentTick, "Current timeline tick text");
  await expectVisible(currentButton, "Return-to-current button");
  await expectVisible(exportButton, "Export PNG button");

  const hiddenWordmarkClass = await hiddenWordmark.getAttribute("class");
  if (!hiddenWordmarkClass?.includes("sr-only")) {
    throw new Error(
      "Dashboard heading wordmark was not hidden with sr-only styling.",
    );
  }

  await slider.focus();
  await page.keyboard.press("ArrowLeft");
  await expectVisible(
    page.getByText("Tick 4 of 5"),
    "Keyboard-updated timeline tick text",
  );

  await currentButton.focus();
  await page.keyboard.press("Enter");
  await expectVisible(currentTick, "Restored current timeline tick text");

  if (viewport.label === "desktop") {
    await expectOrderedLeftEdges(
      [heading, switcher, slider, streamStatus, exportButton],
      "Dashboard header desktop controls",
    );
  }

  await page.selectOption("#dashboard-language-switcher", "zh-CN");
  const localizedToolbar = await waitForStoryRegion(page, "仪表板概览");
  await expectVisible(
    page.locator("#dashboard-language-switcher"),
    "Localized dashboard language switcher",
  );
  await expectVisible(
    localizedToolbar.getByText("语言"),
    "Localized dashboard language label",
  );
  await expectVisible(
    localizedToolbar.getByRole("button", { name: "导出 PNG" }),
    "Localized export trigger button",
  );
  await expectVisible(
    page.getByText("第 5 个刻度，共 5 个"),
    "Localized current timeline tick text",
  );

  await expectNoHorizontalOverflow(
    page,
    `Dashboard header at ${viewport.label}`,
  );
}

async function verifyStory(page, storyCheck, viewport) {
  await page.setViewportSize({
    height: viewport.height,
    width: viewport.width,
  });
  await page.goto(storyUrl(storyCheck.id), { waitUntil: "networkidle" });
  const dialog = storyCheck.dialogName
    ? await waitForDialog(page, storyCheck.dialogName)
    : null;

  await storyCheck.assertions(page, dialog, viewport);
}

async function main() {
  const browser = await chromium.launch({ headless: true });

  try {
    const page = await browser.newPage();

    for (const viewport of viewportChecks) {
      for (const storyCheck of storyChecks) {
        await verifyStory(page, storyCheck, viewport);
      }
    }
  } finally {
    await browser.close();
  }
}

await main();
