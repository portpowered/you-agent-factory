import process from "node:process";
import { chromium } from "playwright";

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
    id: "agent-factory-dashboard-export-factory-dialog--ready",
    label: "export dialog",
  },
  {
    assertions: verifyImportDialog,
    dialogName: "Review factory import",
    id: "agent-factory-dashboard-import-preview-dialog--ready",
    label: "import preview dialog",
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
  if (!(await locator.isVisible())) {
    throw new Error(`${label} was not visible.`);
  }
}

async function verifyExportDialog(page, dialog, viewport) {
  await expectVisible(dialog.getByRole("textbox", { name: "Factory name" }), "Factory name input");
  await expectVisible(dialog.getByLabel("Cover image"), "Cover image input");
  await expectVisible(dialog.getByRole("button", { name: "Cancel" }), "Export cancel button");
  await expectVisible(dialog.getByRole("button", { name: "Export PNG" }), "Export action button");
  await expectVisible(
    dialog.getByText("Confirming export keeps the current dashboard state unchanged"),
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
  await expectVisible(dialog.getByText("factory-import.png"), "Dropped file name");
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
  await expectNoHorizontalOverflow(page, `Import preview dialog at ${viewport.label}`);
}

async function verifyStory(page, storyCheck, viewport) {
  await page.setViewportSize({ height: viewport.height, width: viewport.width });
  await page.goto(storyUrl(storyCheck.id), { waitUntil: "networkidle" });
  const dialog = await waitForDialog(page, storyCheck.dialogName);

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
