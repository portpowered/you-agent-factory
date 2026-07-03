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

export const PACKAGE_SELECT_KEYBOARD_STORY_ID =
  "forms-packageselect--controlled-value";
export const PACKAGE_SELECT_FOCUS_STORY_ID = "forms-packageselect--focus";
export const PACKAGE_SELECT_STORY_LABEL = "Work type";

export const PACKAGE_SELECT_KEYBOARD_STORY_IDS = [
  PACKAGE_SELECT_KEYBOARD_STORY_ID,
  PACKAGE_SELECT_FOCUS_STORY_ID,
];

export const PACKAGE_SELECT_EMPTY_OPTIONS_STORY_ID =
  "forms-packageselect--empty-options";
export const PACKAGE_SELECT_LOADING_OPTIONS_STORY_ID =
  "forms-packageselect--loading-options";
export const PACKAGE_SELECT_ERROR_STATE_STORY_ID =
  "forms-packageselect--error-state";
export const PACKAGE_SELECT_LONG_LABEL_STORY_ID =
  "forms-packageselect--long-label";
export const PACKAGE_SELECT_LONG_LABEL_MOBILE_STORY_ID =
  "forms-packageselect--long-label-mobile-width";

export const PACKAGE_SELECT_EDGE_STATE_STORY_IDS = [
  PACKAGE_SELECT_EMPTY_OPTIONS_STORY_ID,
  PACKAGE_SELECT_LOADING_OPTIONS_STORY_ID,
  PACKAGE_SELECT_ERROR_STATE_STORY_ID,
  PACKAGE_SELECT_LONG_LABEL_STORY_ID,
  PACKAGE_SELECT_LONG_LABEL_MOBILE_STORY_ID,
];

export const PACKAGE_SELECT_RESPONSIVE_VIEWPORTS = [
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

async function expectComboboxFocusRingVisible(page, labelText, label) {
  const trigger = page.getByRole("combobox", { name: labelText });
  await trigger.waitFor({
    state: "visible",
    timeout: STORY_RENDER_TIMEOUT_MS,
  });
  await trigger.focus();

  const hasFocusRing = await trigger.evaluate((element) => {
    if (!(element instanceof HTMLElement)) {
      return false;
    }

    const styles = window.getComputedStyle(element);
    const outlineWidth = Number.parseFloat(styles.outlineWidth || "0");
    const boxShadow = styles.boxShadow;
    return (
      outlineWidth > 0 ||
      (boxShadow !== "none" && boxShadow.length > 0) ||
      element.matches(":focus-visible")
    );
  });

  if (!hasFocusRing) {
    throw new Error(`Expected a visible focus treatment on ${label}.`);
  }
}

export async function verifyPackageSelectKeyboardStories({
  page,
  storyIds = PACKAGE_SELECT_KEYBOARD_STORY_IDS,
} = {}) {
  await page.setViewportSize({ height: 900, width: 1440 });

  for (const storyId of storyIds) {
    await page.goto(storyUrl(storyId), {
      timeout: 90_000,
      waitUntil: "networkidle",
    });
    await waitForStoryRender(page);

    if (storyId === PACKAGE_SELECT_KEYBOARD_STORY_ID) {
      const trigger = page.getByRole("combobox", {
        name: PACKAGE_SELECT_STORY_LABEL,
      });
      await trigger.waitFor({
        state: "visible",
        timeout: STORY_RENDER_TIMEOUT_MS,
      });
      await trigger.focus();
      await page.keyboard.press("ArrowDown");

      const listbox = page.getByRole("listbox");
      await listbox.waitFor({ state: "visible" });
      await page.getByRole("option", { name: "Story" }).waitFor({
        state: "visible",
      });

      await page.keyboard.press("Enter");
      await listbox.waitFor({ state: "hidden" });
      await trigger.waitFor({ state: "visible" });

      const selectedText = await trigger.textContent();
      if (!selectedText?.includes("Story")) {
        throw new Error(
          `Expected keyboard selection to update ${storyId}, got "${selectedText ?? ""}".`,
        );
      }

      const isFocused = await trigger.evaluate((element) => {
        return element === document.activeElement;
      });
      if (!isFocused) {
        throw new Error(
          `Expected focus to return to the select trigger after keyboard selection in ${storyId}.`,
        );
      }
      continue;
    }

    await expectComboboxFocusRingVisible(
      page,
      PACKAGE_SELECT_STORY_LABEL,
      storyId,
    );
  }
}

export async function verifyPackageSelectEdgeStateStories({
  page,
  storyIds = PACKAGE_SELECT_EDGE_STATE_STORY_IDS,
  viewports = PACKAGE_SELECT_RESPONSIVE_VIEWPORTS,
} = {}) {
  for (const viewport of viewports) {
    await page.setViewportSize({
      height: viewport.height,
      width: viewport.width,
    });

    for (const storyId of storyIds) {
      const useMobileViewportOnly =
        storyId === PACKAGE_SELECT_LONG_LABEL_MOBILE_STORY_ID &&
        viewport.label !== "mobile";
      const useDesktopViewportOnly =
        storyId === PACKAGE_SELECT_LONG_LABEL_STORY_ID &&
        viewport.label !== "desktop";
      if (useMobileViewportOnly || useDesktopViewportOnly) {
        continue;
      }

      await page.goto(storyUrl(storyId), {
        timeout: 90_000,
        waitUntil: "networkidle",
      });
      await waitForStoryRender(page);
      await expectNoHorizontalOverflow(
        page,
        `${storyId} (${viewport.label})`,
      );
      await expectVisibleLabelWithinViewport(
        page,
        PACKAGE_SELECT_STORY_LABEL,
        viewport,
      );

      const trigger = page.getByRole("combobox", {
        name: PACKAGE_SELECT_STORY_LABEL,
      });
      await trigger.waitFor({
        state: "visible",
        timeout: STORY_RENDER_TIMEOUT_MS,
      });

      if (storyId === PACKAGE_SELECT_LOADING_OPTIONS_STORY_ID) {
        const loadingState = await trigger.evaluate((element) => ({
          ariaBusy: element.getAttribute("aria-busy"),
          disabled: element.hasAttribute("disabled"),
        }));
        if (loadingState.ariaBusy !== "true" || !loadingState.disabled) {
          throw new Error(
            `Expected ${storyId} to expose a disabled loading combobox.`,
          );
        }
        continue;
      }

      if (storyId === PACKAGE_SELECT_ERROR_STATE_STORY_ID) {
        const errorState = await trigger.evaluate((element) => ({
          ariaInvalid: element.getAttribute("aria-invalid"),
        }));
        if (errorState.ariaInvalid !== "true") {
          throw new Error(`Expected ${storyId} to expose aria-invalid on trigger.`);
        }
        const alertText = await page.getByRole("alert").textContent();
        if (!alertText?.toLowerCase().includes("required")) {
          throw new Error(
            `Expected ${storyId} to render visible error alert text.`,
          );
        }
        continue;
      }

      if (storyId === PACKAGE_SELECT_EMPTY_OPTIONS_STORY_ID) {
        await trigger.click();
        const emptyOption = page.getByRole("option", {
          name: "No work types available",
        });
        await emptyOption.waitFor({ state: "visible" });
        const isDisabled = await emptyOption.evaluate((element) =>
          element.getAttribute("aria-disabled"),
        );
        if (isDisabled !== "true") {
          throw new Error(
            `Expected ${storyId} empty option to be aria-disabled.`,
          );
        }
        await page.keyboard.press("Escape");
        continue;
      }

      if (
        storyId === PACKAGE_SELECT_LONG_LABEL_STORY_ID ||
        storyId === PACKAGE_SELECT_LONG_LABEL_MOBILE_STORY_ID
      ) {
        const triggerBox = await trigger.boundingBox();
        if (!triggerBox) {
          throw new Error(`Could not measure trigger bounds for ${storyId}.`);
        }
        if (triggerBox.x + triggerBox.width > viewport.width + OVERFLOW_TOLERANCE_PX) {
          throw new Error(
            `${storyId} trigger exceeded the ${viewport.label} viewport width.`,
          );
        }
      }
    }
  }
}

async function verifyPackageStorybookBrowser() {
  const browser = await chromium.launch();
  const page = await browser.newPage();

  try {
    await verifyPackageSelectKeyboardStories({ page });
    await verifyPackageSelectEdgeStateStories({ page });
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
      ...PACKAGE_SELECT_KEYBOARD_STORY_IDS,
      ...PACKAGE_SELECT_EDGE_STATE_STORY_IDS,
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
