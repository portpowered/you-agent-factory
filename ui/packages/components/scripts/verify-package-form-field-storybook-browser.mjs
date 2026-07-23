import { spawn } from "node:child_process";
import { rmSync } from "node:fs";
import net from "node:net";
import path from "node:path";
import process from "node:process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import {
  expectNoHorizontalOverflow,
  expectTextLikeFocusRingVisible,
  expectVisibleLabelWithinViewport,
  waitForStoryRender,
} from "./verify-package-storybook-browser-helpers.mjs";

export const PACKAGE_FORM_FIELD_MOBILE_STORY_ID =
  "forms-formfield--mobile-width";
export const PACKAGE_FORM_FIELD_LONG_MESSAGE_MOBILE_STORY_ID =
  "forms-formfield--long-message-mobile-width";
export const PACKAGE_FORM_FIELD_FOCUS_STORY_ID = "forms-formfield--focus";
export const PACKAGE_FORM_FIELD_GROUPED_CONTROL_STORY_ID =
  "forms-formfield--grouped-control";

export const PACKAGE_FORM_FIELD_MOBILE_STORY_IDS = [
  PACKAGE_FORM_FIELD_MOBILE_STORY_ID,
  PACKAGE_FORM_FIELD_LONG_MESSAGE_MOBILE_STORY_ID,
];

export const PACKAGE_FORM_FIELD_FOCUS_STORY_IDS = [
  PACKAGE_FORM_FIELD_FOCUS_STORY_ID,
];

export const PACKAGE_FORM_FIELD_RESPONSIVE_VIEWPORTS = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

export const PACKAGE_FORM_FIELD_STORY_LABEL = "Display name";
export const PACKAGE_FORM_FIELD_LONG_LABEL =
  "Display name with a longer label that should wrap inside narrow layouts";
export const PACKAGE_FORM_FIELD_GROUP_LABEL = "Notification preferences";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const host = process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_HOST ?? "127.0.0.1";
const port = Number(process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_PORT ?? "3818");
const staticDir = path.join(packageRoot, "storybook-static");
const baseUrl = `http://${host}:${port}`;
const indexUrl = `${baseUrl}/index.json`;

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

function formFieldStoryLabel(storyId) {
  if (storyId.includes("long-message")) {
    return PACKAGE_FORM_FIELD_LONG_LABEL;
  }
  return PACKAGE_FORM_FIELD_STORY_LABEL;
}

export async function verifyPackageFormFieldMobileStories({
  page,
  storyIds = PACKAGE_FORM_FIELD_MOBILE_STORY_IDS,
  storyUrl: resolveStoryUrl = storyUrl,
  viewports = PACKAGE_FORM_FIELD_RESPONSIVE_VIEWPORTS,
} = {}) {
  for (const viewport of viewports) {
    await page.setViewportSize({
      height: viewport.height,
      width: viewport.width,
    });

    for (const storyId of storyIds) {
      await page.goto(resolveStoryUrl(storyId), {
        timeout: 90_000,
        waitUntil: "networkidle",
      });
      await waitForStoryRender(page);
      await expectNoHorizontalOverflow(page, `${storyId} (${viewport.label})`);
      await expectVisibleLabelWithinViewport(
        page,
        formFieldStoryLabel(storyId),
        viewport,
      );
    }
  }
}

export async function verifyPackageFormFieldFocusStories({
  page,
  storyIds = PACKAGE_FORM_FIELD_FOCUS_STORY_IDS,
  storyUrl: resolveStoryUrl = storyUrl,
} = {}) {
  await page.setViewportSize({ height: 900, width: 1440 });

  for (const storyId of storyIds) {
    await page.goto(resolveStoryUrl(storyId), {
      timeout: 90_000,
      waitUntil: "networkidle",
    });
    await waitForStoryRender(page);
    await expectTextLikeFocusRingVisible(page, 'input[type="text"]', storyId);
  }
}

export async function verifyPackageFormFieldGroupedControlStory({
  page,
  storyId = PACKAGE_FORM_FIELD_GROUPED_CONTROL_STORY_ID,
  storyUrl: resolveStoryUrl = storyUrl,
} = {}) {
  await page.setViewportSize({ height: 900, width: 1440 });
  await page.goto(resolveStoryUrl(storyId), {
    timeout: 90_000,
    waitUntil: "networkidle",
  });
  await waitForStoryRender(page);

  const group = page.getByRole("group", {
    name: PACKAGE_FORM_FIELD_GROUP_LABEL,
  });
  await group.waitFor({ state: "visible" });
  await page.getByLabel("Email").waitFor({ state: "visible" });
  await page.getByLabel("Text message").waitFor({ state: "visible" });
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

      reject(
        new Error(`build-storybook exited with code ${code ?? "unknown"}`),
      );
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

    for (const storyId of [
      ...PACKAGE_FORM_FIELD_MOBILE_STORY_IDS,
      ...PACKAGE_FORM_FIELD_FOCUS_STORY_IDS,
      PACKAGE_FORM_FIELD_GROUPED_CONTROL_STORY_ID,
    ]) {
      const entry = indexPayload.entries?.[storyId];
      if (!entry) {
        throw new Error(
          `Expected package story ${storyId} in ${indexUrl}, found ${Object.keys(indexPayload.entries ?? {}).join(", ")}`,
        );
      }

      const iframeResponse = await waitForHttpOk(storyUrl(storyId));
      if (!iframeResponse.ok) {
        throw new Error(`Expected ${storyUrl(storyId)} to return HTTP 200.`);
      }
    }

    const browser = await chromium.launch();
    const page = await browser.newPage();

    try {
      await verifyPackageFormFieldMobileStories({ page, storyUrl });
      await verifyPackageFormFieldFocusStories({ page, storyUrl });
      await verifyPackageFormFieldGroupedControlStory({ page, storyUrl });
    } finally {
      await browser.close();
    }

    console.log(
      `Verified FormField Storybook stories at ${baseUrl} without dashboard providers.`,
    );
  } finally {
    cleanup();
    await delay(500);
  }
}

if (import.meta.url === new URL(process.argv[1], "file:").href) {
  main().catch((error) => {
    console.error(error);
    process.exitCode = 1;
  });
}
