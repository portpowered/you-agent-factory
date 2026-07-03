import { spawn } from "node:child_process";
import { readFileSync, readdirSync, rmSync } from "node:fs";
import net from "node:net";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const host = process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_HOST ?? "127.0.0.1";
const port = Number(
  process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_PORT ?? "3817",
);
const staticDir = path.join(packageRoot, "storybook-static");
const baseUrl = `http://${host}:${port}`;
const indexUrl = `${baseUrl}/index.json`;
const OVERFLOW_TOLERANCE_PX = 4;
const STORY_RENDER_TIMEOUT_MS = 30_000;

const PACKAGE_TEXT_STORY_ID = "primitives-packagetext--body";
const PACKAGE_TEXT = "Hello from the component package";

export const PACKAGE_INPUT_MOBILE_STORY_ID = "forms-packageinput--mobile-width";
export const PACKAGE_TEXTAREA_MOBILE_STORY_ID =
  "forms-packagetextarea--mobile-width";
export const PACKAGE_CHECKBOX_MOBILE_STORY_ID =
  "forms-packagecheckbox--mobile-width";
export const PACKAGE_FILE_INPUT_MOBILE_STORY_ID =
  "forms-packagefileinput--mobile-width";

export const PACKAGE_INPUT_FOCUS_STORY_ID = "forms-packageinput--focus";
export const PACKAGE_TEXTAREA_FOCUS_STORY_ID = "forms-packagetextarea--focus";
export const PACKAGE_CHECKBOX_FOCUS_STORY_ID = "forms-packagecheckbox--focus";
export const PACKAGE_FILE_INPUT_FOCUS_STORY_ID = "forms-packagefileinput--focus";

export const PACKAGE_FORM_MOBILE_STORY_IDS = [
  PACKAGE_INPUT_MOBILE_STORY_ID,
  PACKAGE_TEXTAREA_MOBILE_STORY_ID,
  PACKAGE_CHECKBOX_MOBILE_STORY_ID,
  PACKAGE_FILE_INPUT_MOBILE_STORY_ID,
];

export const PACKAGE_FORM_FOCUS_STORY_IDS = [
  PACKAGE_INPUT_FOCUS_STORY_ID,
  PACKAGE_TEXTAREA_FOCUS_STORY_ID,
  PACKAGE_CHECKBOX_FOCUS_STORY_ID,
  PACKAGE_FILE_INPUT_FOCUS_STORY_ID,
];

export const PACKAGE_FORM_RESPONSIVE_VIEWPORTS = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

function storyUrl(storyId) {
  return `${baseUrl}/iframe.html?id=${storyId}&viewMode=story`;
}

function assertPortAvailable(hostName, portNumber) {
  return new Promise((resolve, reject) => {
    const server = net.createServer();

    server.once("error", (error) => {
      if (error?.code === "EADDRINUSE") {
        reject(
          new Error(
            `Port ${portNumber} on ${hostName} is already in use. Choose another AGENT_FACTORY_PACKAGE_STORYBOOK_PORT.`,
          ),
        );
        return;
      }

      reject(error);
    });

    server.listen(portNumber, hostName, () => {
      server.close(() => resolve());
    });
  });
}

function spawnCommand(command, args, options = {}) {
  return spawn(command, args, {
    cwd: packageRoot,
    stdio: "inherit",
    shell: false,
    ...options,
  });
}

async function waitForHttpOk(url, timeoutMs = 30_000) {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    try {
      const response = await fetch(url, {
        signal: AbortSignal.timeout(10_000),
      });
      if (response.ok) {
        return response;
      }
    } catch {
      // Storybook static server may still be starting.
    }

    await delay(250);
  }

  throw new Error(`Timed out waiting for ${url}`);
}

function readStaticAssetBundleText() {
  const assetsDir = path.join(staticDir, "assets");
  return readdirSync(assetsDir)
    .filter((fileName) => fileName.endsWith(".js"))
    .map((fileName) =>
      readFileSync(path.join(assetsDir, fileName), "utf8"),
    )
    .join("\n");
}

async function waitForStoryRender(page) {
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

async function expectVisibleLabelWithinViewport(page, labelText, viewport) {
  const label = page.getByText(labelText, { exact: true });
  await label.waitFor({ state: "visible" });

  const box = await label.boundingBox();
  if (!box) {
    throw new Error(`Could not measure label bounds for "${labelText}".`);
  }

  const exceedsViewport =
    box.x < -OVERFLOW_TOLERANCE_PX ||
    box.y < -OVERFLOW_TOLERANCE_PX ||
    box.x + box.width > viewport.width + OVERFLOW_TOLERANCE_PX ||
    box.y + box.height > viewport.height + OVERFLOW_TOLERANCE_PX;

  if (exceedsViewport) {
    throw new Error(
      `Label "${labelText}" exceeded the ${viewport.label} viewport (${viewport.width}x${viewport.height}).`,
    );
  }
}

async function expectTextLikeFocusRingVisible(page, selector, label) {
  const hasFocusRing = await page.evaluate((elementSelector) => {
    const element = document.querySelector(elementSelector);
    if (!(element instanceof HTMLElement)) {
      return false;
    }

    element.focus();
    const styles = window.getComputedStyle(element);
    const outlineWidth = Number.parseFloat(styles.outlineWidth || "0");
    const boxShadow = styles.boxShadow;
    return (
      outlineWidth > 0 ||
      (boxShadow !== "none" && boxShadow.length > 0) ||
      element.matches(":focus-visible")
    );
  }, selector);

  if (!hasFocusRing) {
    throw new Error(`Expected a visible focus treatment on ${label}.`);
  }
}

async function expectCheckboxFocusRingVisible(page, labelText, label) {
  const checkbox = page.getByRole("checkbox", { name: labelText });
  await checkbox.focus();

  const hasFocusRing = await checkbox.evaluate((input) => {
    if (!(input instanceof HTMLInputElement)) {
      return false;
    }

    const indicator = input.nextElementSibling;
    if (!(indicator instanceof HTMLElement)) {
      return input.matches(":focus-visible");
    }

    const styles = window.getComputedStyle(indicator);
    const boxShadow = styles.boxShadow;
    return (
      input.matches(":focus-visible") ||
      (boxShadow !== "none" && boxShadow.length > 0)
    );
  });

  if (!hasFocusRing) {
    throw new Error(`Expected a visible focus treatment on ${label}.`);
  }
}

export async function verifyPackageFormMobileStories({
  page,
  storyIds = PACKAGE_FORM_MOBILE_STORY_IDS,
  viewports = PACKAGE_FORM_RESPONSIVE_VIEWPORTS,
} = {}) {
  for (const viewport of viewports) {
    await page.setViewportSize({
      height: viewport.height,
      width: viewport.width,
    });

    for (const storyId of storyIds) {
      await page.goto(storyUrl(storyId), {
        timeout: 90_000,
        waitUntil: "networkidle",
      });
      await waitForStoryRender(page);
      await expectNoHorizontalOverflow(
        page,
        `${storyId} (${viewport.label})`,
      );

      const labelText = storyId.includes("textarea")
        ? "Factory notes"
        : storyId.includes("checkbox")
          ? "Enable cron trigger"
          : storyId.includes("fileinput")
            ? "Factory cover image"
            : "Factory name";
      await expectVisibleLabelWithinViewport(page, labelText, viewport);
    }
  }
}

export async function verifyPackageFormFocusStories({
  page,
  storyIds = PACKAGE_FORM_FOCUS_STORY_IDS,
} = {}) {
  await page.setViewportSize({ height: 900, width: 1440 });

  for (const storyId of storyIds) {
    await page.goto(storyUrl(storyId), {
      timeout: 90_000,
      waitUntil: "networkidle",
    });
    await waitForStoryRender(page);

    if (storyId.includes("checkbox")) {
      await expectCheckboxFocusRingVisible(
        page,
        "Enable cron trigger",
        storyId,
      );
      continue;
    }

    const selector = storyId.includes("fileinput")
      ? 'input[type="file"]'
      : storyId.includes("textarea")
        ? "textarea"
        : 'input[type="text"]';
    await expectTextLikeFocusRingVisible(page, selector, storyId);
  }
}

async function verifyPackageStorybookBrowser() {
  const browser = await chromium.launch();
  const page = await browser.newPage();

  try {
    await verifyPackageFormMobileStories({ page });
    await verifyPackageFormFocusStories({ page });
  } finally {
    await browser.close();
  }
}

async function main() {
  await assertPortAvailable(host, port);

  rmSync(staticDir, { force: true, recursive: true });

  await new Promise((resolve, reject) => {
    const build = spawnCommand("bun", ["run", "build-storybook"]);
    build.once("error", reject);
    build.once("exit", (code) => {
      if (code === 0) {
        resolve();
        return;
      }

      reject(new Error(`build-storybook exited with code ${code ?? "unknown"}`));
    });
  });

  const server = spawnCommand("bunx", [
    "http-server",
    staticDir,
    "-p",
    String(port),
    "-a",
    host,
    "-s",
  ]);

  let serverExited = false;
  server.once("exit", () => {
    serverExited = true;
  });

  const cleanup = () => {
    if (!serverExited && !server.killed) {
      server.kill("SIGTERM");
    }
  };

  process.once("exit", cleanup);
  process.once("SIGINT", () => {
    cleanup();
    process.exit(1);
  });
  process.once("SIGTERM", () => {
    cleanup();
    process.exit(1);
  });

  try {
    const indexResponse = await waitForHttpOk(indexUrl);
    const indexPayload = await indexResponse.json();
    const storyEntry = indexPayload.entries?.[PACKAGE_TEXT_STORY_ID];

    if (!storyEntry) {
      throw new Error(
        `Expected package story ${PACKAGE_TEXT_STORY_ID} in ${indexUrl}, found ${Object.keys(indexPayload.entries ?? {}).join(", ")}`,
      );
    }

    for (const storyId of [
      PACKAGE_TEXT_STORY_ID,
      ...PACKAGE_FORM_MOBILE_STORY_IDS,
      ...PACKAGE_FORM_FOCUS_STORY_IDS,
    ]) {
      const iframeResponse = await waitForHttpOk(storyUrl(storyId));
      if (!iframeResponse.ok) {
        throw new Error(`Expected ${storyUrl(storyId)} to return HTTP 200.`);
      }
    }

    const assetBundleText = readStaticAssetBundleText();
    if (!assetBundleText.includes(PACKAGE_TEXT)) {
      throw new Error(
        `Built package Storybook assets did not include story text for ${PACKAGE_TEXT_STORY_ID}.`,
      );
    }

    if (
      assetBundleText.includes("DashboardSessionProvider") ||
      assetBundleText.includes("@tanstack/react-query")
    ) {
      throw new Error(
        "Built package Storybook assets appear to include dashboard runtime providers.",
      );
    }

    await verifyPackageStorybookBrowser();

    console.log(
      `Verified package Storybook stories at ${baseUrl} without dashboard providers.`,
    );
  } finally {
    cleanup();
    await delay(500);
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
