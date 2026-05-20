import process from "node:process";
import { chromium } from "playwright";
import { verifyDashboardShellConsolidation } from "./dashboard-shell-storybook-responsive.mjs";
import {
  createLocalizedExportDialogVerifier,
  createLocalizedImportDialogVerifier,
} from "./verify-import-export-storybook-localized.mjs";
import {
  verifyLocalizedCurrentSelection,
  verifyLocalizedSubmitWorkCard,
  verifyLocalizedTraceGrid,
  verifyLocalizedWorkflowActivity,
  verifyLocalizedWorkOutcomeChart,
} from "./verify-localized-widget-storybook-responsive.mjs";
import { verifyTraceFactoryGraphVisualConsistency } from "./graph-storybook-responsive.mjs";
import { verifyProviderSessionDetailSuccess as verifyProviderSessionDetailSuccessImpl } from "./verify-provider-session-storybook-responsive.mjs";
const STORYBOOK_HOST = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const STORYBOOK_PORT = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const STORYBOOK_URL = `http://${STORYBOOK_HOST}:${STORYBOOK_PORT}`;
const OVERFLOW_TOLERANCE_PX = 1;
const STORY_RENDER_TIMEOUT_MS = 30000;

export const viewportChecks = [
  { height: 844, label: "mobile", width: 390 },
  { height: 1024, label: "tablet", width: 768 },
  { height: 900, label: "desktop", width: 1440 },
];

export const storyChecks = [
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
    id: "infinite-you-workflow-dashboard--header-action-buttons-verification",
    label: "dashboard header",
  },
  {
    assertions: verifyDashboardShellConsolidation,
    id: "infinite-you-workflow-dashboard--header-action-buttons-verification",
    label: "dashboard shared shell",
  },
  {
    assertions: (page, _dialog, viewport) =>
      verifyTraceFactoryGraphVisualConsistency({
        expectNoHorizontalOverflow,
        expectVisible,
        page,
        viewport,
        waitForStoryRender,
      }),
    id: "agent-factory-dashboard-react-flow-current-activity-card--narrow-viewport",
    label: "trace/factory graph visual consistency",
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
  {
    assertions: (page, _dialog, viewport) =>
      verifyLocalizedCurrentSelection({
        expectNoHorizontalOverflow,
        expectVisible,
        page,
        viewport,
      }),
    id: "infinite-you-workflow-dashboard--locale-propagation-verification",
    label: "current selection widget (zh-CN)",
  },
  {
    assertions: verifyProviderSessionDetailSuccess,
    id: "infinite-you-current-selection-provider-session-detail-panel--timestamp-prefixed-session-success",
    label: "current selection provider-session success",
  },
  {
    assertions: verifyCurrentSelectionPromptHint,
    id: "infinite-you-workflow-dashboard--current-selection-prompt-hint-verification",
    label: "current selection prompt hinting",
  },
];

function storyUrl(storyId) {
  return `${STORYBOOK_URL}/iframe.html?id=${storyId}&viewMode=story`;
}

export async function waitForStoryRender(page) {
  await page.waitForSelector("#storybook-root", {
    state: "attached",
    timeout: STORY_RENDER_TIMEOUT_MS,
  });
  await page.waitForFunction(
    () => {
      const root = document.querySelector("#storybook-root");
      if (!(root instanceof HTMLElement)) {
        return false;
      }
      if (root.childElementCount > 0) {
        return true;
      }
      return Array.from(document.body.children).some((child) => {
        if (!(child instanceof HTMLElement)) {
          return false;
        }
        if (
          child.id === "storybook-root" ||
          child.id === "storybook-docs" ||
          child.tagName === "SCRIPT" ||
          child.tagName === "STYLE"
        ) {
          return false;
        }

        return true;
      });
    },
    { timeout: STORY_RENDER_TIMEOUT_MS },
  );
}

export async function waitForDialog(page, dialogName) {
  const dialog = page.getByRole("dialog", { name: dialogName });
  await dialog.waitFor({ state: "visible" });
  return dialog;
}

export async function waitForStoryRegion(page, regionName) {
  const region = page.getByRole("region", { name: regionName });
  await region.waitFor({ state: "visible" });
  return region;
}

export async function expectNoHorizontalOverflow(page, label) {
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

export async function expectDialogWithinViewport(dialog, viewport, label) {
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

export async function expectVisible(locator, label) {
  if (typeof locator.waitFor === "function") {
    await locator.waitFor({
      state: "visible",
      timeout: STORY_RENDER_TIMEOUT_MS,
    });
    return;
  }

  if (!(await locator.isVisible())) {
    throw new Error(`${label} was not visible.`);
  }
}

export async function verifyExportDialog(page, dialog, viewport) {
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

export async function verifyImportDialog(page, dialog, viewport) {
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
const verifyLocalizedExportDialogImpl = createLocalizedExportDialogVerifier({
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  expectVisible,
});
const verifyLocalizedImportDialogImpl = createLocalizedImportDialogVerifier({
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  expectVisible,
});
export async function verifyLocalizedExportDialog(page, dialog, viewport) {
  return verifyLocalizedExportDialogImpl(page, dialog, viewport);
}
export async function verifyLocalizedImportDialog(page, dialog, viewport) {
  return verifyLocalizedImportDialogImpl(page, dialog, viewport);
}

export async function expectOrderedLeftEdges(locators, label) {
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

export async function verifyDashboardHeader(page, _dialog, viewport) {
  const heading = page.getByRole("heading", { name: "Infinite You" });
  const hiddenWordmark = heading.getByText("Infinite You");
  const slider = page.getByRole("slider", { name: "Timeline tick" });
  const languageButton = page.getByRole("button", {
    name: "Change language",
  });
  const streamStatus = page.getByRole("status", {
    name: /Infinite You event stream (connecting|live)/,
  });
  const currentTick = page.getByText(/\d+\/\d+/).first();
  const currentButton = page.getByRole("button", {
    name: "Return to current tick",
  });
  const exportButton = page.getByRole("button", { name: "Export PNG" });

  await expectVisible(heading, "Dashboard heading");
  await expectVisible(hiddenWordmark, "Accessible Infinite You wordmark");
  await expectVisible(slider, "Timeline slider");
  await expectVisible(languageButton, "Language menu button");
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
    page.getByText(/\d+\/\d+/),
    "Keyboard-updated timeline tick text",
  );

  await currentButton.focus();
  await page.keyboard.press("Enter");
  await expectVisible(currentTick, "Restored current timeline tick text");

  if (viewport.label === "desktop") {
    await expectOrderedLeftEdges(
      [heading, slider, languageButton, exportButton, streamStatus],
      "Dashboard header desktop controls",
    );
  }

  await expectNoHorizontalOverflow(
    page,
    `Dashboard header at ${viewport.label}`,
  );
}


export async function verifyCurrentSelectionPromptHint(page, _dialog, viewport) {
  const currentSelection = page.getByRole("article", {
    name: "Current selection",
  });
  await currentSelection.waitFor({ state: "visible" });
  const helpButton = currentSelection.getByRole("button", {
    name: "Close prompt variable help",
  });
  const promptField = currentSelection.getByRole("textbox", { name: "Prompt" });
  const saveButton = currentSelection.getByRole("button", {
    name: "Save changes",
  });

  await expectVisible(helpButton, "Prompt variable help toggle");
  await expectVisible(
    currentSelection.getByText("This workstation exposes 1 authored input."),
    "Prompt help input-count summary",
  );
  await expectVisible(
    currentSelection.getByText("Available variables"),
    "Prompt help available variables heading",
  );
  await expectVisible(
    currentSelection.getByText("{{ .WorkID }}"),
    "Prompt help example snippet",
  );

  await promptField.focus();
  await promptField.fill("Use {{ (index .Inputs 1).Payload }}.");
  await expectVisible(
    currentSelection.getByRole("heading", { name: "Prompt diagnostics" }),
    "Prompt diagnostics summary",
  );
  await expectVisible(
    currentSelection.getByText(".Inputs[1]", { exact: true }),
    "Prompt diagnostics variable path",
  );
  await expectVisible(
    currentSelection.locator("mark").filter({ hasText: "(index .Inputs 1)" }),
    "Prompt squiggle overlay",
  );

  const saveButtonDisabled = await saveButton.isDisabled();
  if (!saveButtonDisabled) {
    throw new Error("Save changes should stay disabled while diagnostics remain.");
  }

  await expectNoHorizontalOverflow(
    page,
    `Current selection prompt hinting at ${viewport.label}`,
  );
}

export async function verifyProviderSessionDetailSuccess(
  page,
  _dialog,
  viewport,
) {
  return verifyProviderSessionDetailSuccessImpl({
    expectNoHorizontalOverflow,
    expectVisible,
    page,
    viewport,
  });
}

export async function verifyStory(browser, storyCheck, viewport) {
  const context = await browser.newContext({
    viewport: {
      height: viewport.height,
      width: viewport.width,
    },
  });
  const page = await context.newPage();

  try {
    await page.goto(storyUrl(storyCheck.id), { waitUntil: "domcontentloaded" });
    await waitForStoryRender(page);
    const dialog = storyCheck.dialogName
      ? await waitForDialog(page, storyCheck.dialogName)
      : null;

    await storyCheck.assertions(page, dialog, viewport);
  } finally {
    await context.close();
  }
}

export async function runResponsiveStorybookChecks(
  browser,
  { checks = storyChecks, viewports = viewportChecks } = {},
) {
  for (const viewport of viewports) {
    for (const storyCheck of checks) {
      await verifyStory(browser, storyCheck, viewport);
    }
  }
}

export async function main() {
  const browser = await chromium.launch({ headless: true });

  try {
    await runResponsiveStorybookChecks(browser);
  } finally {
    await browser.close();
  }
}

if (import.meta.main) {
  await main();
}
