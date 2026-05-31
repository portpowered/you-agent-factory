import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import {
  assertChooseFileDragActiveNeutral,
  assertChooseFileShellNeutral,
} from "./choose-file-shell-assertions.mjs";
import {
  storyUrl,
  waitForDialog,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";

const dirname = path.dirname(fileURLToPath(import.meta.url));

export const CHOOSE_FILE_FIXTURE_PATH = path.resolve(
  dirname,
  "../../docs/internal/resources/dashboard.png",
);

export const EXPORT_CHOOSE_FILE_STORY_ID =
  "you-agent-factory-dashboard-export-factory-dialog--ready";

export const SUBMIT_WORK_CHOOSE_FILE_STORY_ID =
  "agent-factory-dashboard-submit-work-card--image-choose-file-verification";

export async function readElementClassName(locator, label) {
  await locator.waitFor({ state: "visible" });
  const className = await locator.evaluate((element) => element.className);
  if (typeof className !== "string" || className.length === 0) {
    throw new Error(`Could not read className for ${label}.`);
  }

  return className;
}

export async function verifyExportChooseFileChrome(_page, dialog) {
  const imageInput = dialog.getByLabel("Cover image");
  const idleClassName = await readElementClassName(
    imageInput,
    "export cover-image input",
  );
  assertChooseFileShellNeutral(idleClassName, "export cover-image input");

  await imageInput.setInputFiles(CHOOSE_FILE_FIXTURE_PATH);
  await dialog
    .getByText("Selected image: dashboard.png")
    .waitFor({ state: "visible" });

  const selectedClassName = await readElementClassName(
    imageInput,
    "export cover-image input after selection",
  );
  assertChooseFileShellNeutral(
    selectedClassName,
    "export cover-image input after selection",
  );
}

export async function verifySubmitWorkChooseFileChrome(page) {
  const card = page.getByRole("article", { name: "Submit work" });
  await card.waitFor({ state: "visible" });

  const dropzone = card.locator("label").filter({ hasText: "Image file" });
  const idleClassName = await readElementClassName(
    dropzone,
    "submit-work image dropzone",
  );
  assertChooseFileShellNeutral(idleClassName, "submit-work image dropzone");

  await dropzone.evaluate((label) => {
    const dataTransfer = new DataTransfer();
    dataTransfer.items.add(
      new File(["png"], "drag.png", { type: "image/png" }),
    );
    label.dispatchEvent(
      new DragEvent("dragenter", {
        bubbles: true,
        cancelable: true,
        dataTransfer,
      }),
    );
  });
  await card
    .getByText("Drop the image file to stage it.")
    .waitFor({ state: "visible" });

  const dragActiveClassName = await readElementClassName(
    dropzone,
    "submit-work image dropzone drag-active",
  );
  assertChooseFileDragActiveNeutral(
    dragActiveClassName,
    "submit-work image dropzone drag-active",
  );

  await dropzone.evaluate((label) => {
    const dataTransfer = new DataTransfer();
    label.dispatchEvent(
      new DragEvent("dragleave", {
        bubbles: true,
        cancelable: true,
        dataTransfer,
      }),
    );
  });

  const fileInput = card.locator('input[type="file"]');
  await fileInput.setInputFiles(CHOOSE_FILE_FIXTURE_PATH);
  await card
    .getByText("dashboard.png (image/png)")
    .waitFor({ state: "visible" });
  await card.getByText("Replace file").waitFor({ state: "visible" });

  const readyClassName = await readElementClassName(
    dropzone,
    "submit-work image dropzone after selection",
  );
  assertChooseFileShellNeutral(
    readyClassName,
    "submit-work image dropzone after selection",
  );
}

export async function verifyChooseFileStories({
  storybookUrl,
  browser = chromium,
} = {}) {
  const host = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
  const port = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
  const baseUrl = storybookUrl ?? `http://${host}:${port}`;
  const browserInstance = await browser.launch();

  try {
    const page = await browserInstance.newPage({
      viewport: { height: 900, width: 1440 },
    });

    await page.goto(storyUrl(baseUrl, EXPORT_CHOOSE_FILE_STORY_ID), {
      timeout: 90_000,
      waitUntil: "domcontentloaded",
    });
    await waitForStoryRender(page);
    const exportDialog = await waitForDialog(page, "Export factory");
    await verifyExportChooseFileChrome(page, exportDialog);

    await page.goto(storyUrl(baseUrl, SUBMIT_WORK_CHOOSE_FILE_STORY_ID), {
      timeout: 90_000,
      waitUntil: "domcontentloaded",
    });
    await waitForStoryRender(page);
    await verifySubmitWorkChooseFileChrome(page);
  } finally {
    await browserInstance.close();
  }
}

export async function main() {
  await verifyChooseFileStories();
  console.log("Unified choose-file browser verification passed.");
}

if (import.meta.main) {
  await main();
}
