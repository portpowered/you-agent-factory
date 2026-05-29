import process from "node:process";
import { chromium } from "playwright";
import { verifyDashboardShellConsolidation } from "./dashboard-shell-storybook-responsive.mjs";
import {
  OVERFLOW_TOLERANCE_PX,
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  expectVisible,
  storyUrl,
  waitForDialog,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";
import { verifyDashboardSessionTabs as verifyDashboardSessionTabsImpl } from "./verify-dashboard-session-tabs-storybook-responsive.mjs";
import { verifyBentoCardCatalogHeader as verifyBentoCardCatalogHeaderImpl } from "./verify-bento-card-catalog-storybook-header.mjs";
import { verifyBentoCardCatalogResponsive as verifyBentoCardCatalogResponsiveImpl } from "./verify-bento-card-catalog-storybook-responsive.mjs";
import {
  verifyEditorGraphParity as verifyEditorGraphParityImpl,
  verifyObserverGraphParity as verifyObserverGraphParityImpl,
} from "./verify-graph-parity-storybook-responsive.mjs";
import {
  createLocalizedExportDialogVerifier,
  createLocalizedImportDialogVerifier,
} from "./verify-import-export-storybook-localized.mjs";
import {
  verifyLocalizedSubmitWorkCard,
  verifyLocalizedTraceGrid,
  verifyLocalizedWorkflowActivity,
  verifyLocalizedWorkOutcomeChart,
} from "./verify-localized-widget-storybook-responsive.mjs";
import { verifyProviderSessionDetailSuccess as verifyProviderSessionDetailSuccessImpl } from "./verify-provider-session-storybook-responsive.mjs";
const STORYBOOK_HOST = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const STORYBOOK_PORT = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const STORYBOOK_URL = `http://${STORYBOOK_HOST}:${STORYBOOK_PORT}`;

export const viewportChecks = [
  { height: 844, label: "mobile", width: 390 },
  { height: 1024, label: "tablet", width: 768 },
  { height: 900, label: "desktop", width: 1440 },
];

export const storyChecks = [
  {
    assertions: verifyExportDialog,
    dialogName: "Export factory",
    id: "you-agent-factory-dashboard-export-factory-dialog--ready",
    label: "export dialog (en)",
  },
  {
    assertions: verifyLocalizedExportDialog,
    dialogName: "导出工厂",
    id: "you-agent-factory-dashboard-export-factory-dialog--localized-zh-cn",
    label: "export dialog (zh-CN)",
  },
  {
    assertions: verifyImportDialog,
    dialogName: "Review factory import",
    id: "you-agent-factory-dashboard-import-preview-dialog--ready",
    label: "import preview dialog (en)",
  },
  {
    assertions: verifyLocalizedImportDialog,
    dialogName: "检查工厂导入",
    id: "you-agent-factory-dashboard-import-preview-dialog--localized-zh-cn",
    label: "import preview dialog (zh-CN)",
  },
  {
    assertions: verifyDashboardHeader,
    id: "you-agent-factory-dashboard-dashboard-header--responsive-verification",
    label: "dashboard header",
  },
  {
    assertions: verifyDashboardSessionTabs,
    id: "you-agent-factory-dashboard-session-tabs--open-flow-verification",
    label: "dashboard session tabs",
  },
  {
    assertions: verifyDashboardShellConsolidation,
    id: "you-agent-factory-workflow-dashboard--header-action-buttons-verification",
    label: "dashboard shared shell",
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
      verifyProviderSessionDetailSuccessImpl({
        expectNoHorizontalOverflow,
        expectVisible,
        page,
        viewport,
      }),
    id: "you-agent-factory-current-selection-provider-session-detail-panel--timestamp-prefixed-session-success",
    label: "current selection provider-session success",
  },
  {
    assertions: verifyBentoCardCatalogResponsive,
    id: "you-agent-factory-dashboard-bento-cards--responsive-verification",
    label: "bento card catalog",
  },
  {
    assertions: verifyBentoCardCatalogHeader,
    id: "you-agent-factory-dashboard-bento-cards--header-consistency-verification",
    label: "bento card header catalog",
    viewports: [
      { height: 844, label: "mobile", width: 390 },
      { height: 900, label: "desktop", width: 1440 },
    ],
  },
  {
    assertions: verifyObserverGraphParity,
    id: "agent-factory-dashboard-react-flow-current-activity-card--semantic-workflow",
    label: "observer graph parity",
  },
  {
    assertions: verifyEditorGraphParity,
    id: "agent-factory-dashboard-factory-graph-editor-flow--worker-resource-density",
    label: "editor graph parity",
  },
];

export {
  expectDialogWithinViewport,
  expectNoHorizontalOverflow,
  expectVisible,
  waitForDialog,
  waitForStoryRegion,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";

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
  const heading = page.getByRole("heading", {
    level: 1,
    name: "U",
    exact: true,
  });
  const slider = page.getByRole("slider", { name: "Timeline tick" });
  const sessionTabs = page.getByRole("navigation", {
    name: "factory sessions",
  });
  const rootTab = page.getByRole("tab", { name: "root" });
  const allTabs = page.getByRole("tab");
  const languageButton = page.getByRole("button", {
    name: "Change language",
  });
  const sessionStreamToggle = page.getByRole("button", {
    name: /(Pause|Resume) .* updates/,
  });
  const exportButton = page.getByRole("button", { name: "Export PNG" });
  const globalActions = page.getByRole("group", {
    name: "Dashboard actions",
  });
  const timelineStatus = page.getByText(/^\d+\/\d+$/);

  await expectVisible(heading, "Dashboard heading");
  await expectVisible(sessionTabs, "Session tabs navigation");
  await expectVisible(rootTab, "Default session tab");
  await expectVisible(slider, "Timeline slider");
  await expectVisible(timelineStatus, "Timeline status");
  await expectVisible(languageButton, "Language menu button");
  await expectVisible(sessionStreamToggle, "Dashboard session stream toggle");
  await expectVisible(exportButton, "Export PNG button");
  await expectVisible(globalActions, "Global header actions");

  if ((await page.getByRole("status", { name: /Event stream/i }).count()) > 0) {
    throw new Error(
      "Dashboard header still rendered the retired event-stream status pill.",
    );
  }

  if ((await allTabs.count()) < 3) {
    throw new Error("Dashboard header did not render the expected multi-session tab strip.");
  }
  if ((await page.getByRole("status", { name: /Event stream/i }).count()) > 0) {
    throw new Error(
      "Dashboard header still rendered the retired event-stream status pill.",
    );
  }

  if (await page.getByRole("button", { name: "Return to current tick" }).isVisible()) {
    throw new Error(
      "Dashboard header still rendered the retired return-to-current button.",
    );
  }
  if (viewport.label === "desktop") {
    await expectOrderedLeftEdges(
      [heading, sessionTabs, languageButton],
      "Dashboard header desktop primary-row controls",
    );
    await expectOrderedLeftEdges(
      [slider, timelineStatus],
      "Dashboard header desktop timeline controls",
    );
  }
  await expectNoHorizontalOverflow(
    page,
    `Dashboard header at ${viewport.label}`,
  );
}
export async function verifyDashboardSessionTabs(page, _dialog, viewport) {
  return verifyDashboardSessionTabsImpl(
    { expectNoHorizontalOverflow, expectVisible, waitForDialog },
    page,
    viewport,
  );
}
export async function verifyBentoCardCatalogResponsive(page, _dialog, viewport) {
  return verifyBentoCardCatalogResponsiveImpl({
    expectNoHorizontalOverflow,
    expectVisible,
    page,
    viewport,
  });
}
export async function verifyBentoCardCatalogHeader(page, _dialog, viewport) {
  return verifyBentoCardCatalogHeaderImpl({
    expectNoHorizontalOverflow,
    expectVisible,
    page,
    viewport,
  });
}
export async function verifyObserverGraphParity(page, _dialog, viewport) {
  return verifyObserverGraphParityImpl(
    { expectNoHorizontalOverflow, expectVisible },
    page,
    viewport,
  );
}
export async function verifyEditorGraphParity(page, _dialog, viewport) {
  return verifyEditorGraphParityImpl(
    { expectNoHorizontalOverflow, expectVisible },
    page,
    viewport,
  );
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
    await page.goto(storyUrl(STORYBOOK_URL, storyCheck.id), {
      waitUntil: "domcontentloaded",
    });
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
  for (const storyCheck of checks) {
    for (const viewport of storyCheck.viewports ?? viewports) {
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
