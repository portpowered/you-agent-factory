import { spawn } from "node:child_process";
import { readdirSync, readFileSync, rmSync } from "node:fs";
import net from "node:net";
import path from "node:path";
import process from "node:process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import {
  PACKAGE_FORM_FOCUS_STORY_IDS,
  PACKAGE_FORM_MOBILE_STORY_IDS,
  verifyPackageFormFocusStories,
  verifyPackageFormMobileStories,
} from "./verify-package-form-storybook-browser.mjs";
import {
  PACKAGE_DATATABLE_STORY_IDS,
  verifyPackageDataTableStories,
} from "./verify-package-storybook-datatable.mjs";
import {
  hostResponsibilityCopy,
  overlayStoryIds,
  overlayStoryTexts,
  verifyPackageOverlayStories,
} from "./verify-package-storybook-overlays.mjs";
import {
  PACKAGE_SELECT_EDGE_STATE_STORY_IDS,
  PACKAGE_SELECT_KEYBOARD_STORY_IDS,
  verifyPackageSelectEdgeStateStories,
  verifyPackageSelectKeyboardStories,
} from "./verify-package-storybook-selects.mjs";

export {
  PACKAGE_FORM_FOCUS_STORY_IDS,
  PACKAGE_FORM_MOBILE_STORY_IDS,
} from "./verify-package-form-storybook-browser.mjs";
export {
  PACKAGE_SELECT_EDGE_STATE_STORY_IDS,
  PACKAGE_SELECT_FOCUS_STORY_ID,
  PACKAGE_SELECT_KEYBOARD_STORY_ID,
  PACKAGE_SELECT_KEYBOARD_STORY_IDS,
  PACKAGE_SELECT_STORY_LABEL,
} from "./verify-package-storybook-selects.mjs";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const host = process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_HOST ?? "127.0.0.1";
const port = Number(process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_PORT ?? "3817");
const staticDir = path.join(packageRoot, "storybook-static");
const baseUrl = `http://${host}:${port}`;
const indexUrl = `${baseUrl}/index.json`;

const PACKAGE_TEXT_STORY_ID = "primitives-packagetext--body";
const PACKAGE_TEXT = "Hello from the component package";

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
    .map((fileName) => readFileSync(path.join(assetsDir, fileName), "utf8"))
    .join("\n");
}

async function verifyPackageStorybookBrowser() {
  const browser = await chromium.launch();
  const page = await browser.newPage();

  try {
    await verifyPackageFormMobileStories({ page, storyUrl });
    await verifyPackageFormFocusStories({ page, storyUrl });
    await verifyPackageSelectKeyboardStories({ page, storyUrl });
    await verifyPackageSelectEdgeStateStories({ page, storyUrl });
    await verifyPackageOverlayStories({ page, baseUrl });
    await verifyPackageDataTableStories({ page, storyUrl });
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
      ...PACKAGE_SELECT_KEYBOARD_STORY_IDS,
      ...PACKAGE_SELECT_EDGE_STATE_STORY_IDS,
      ...overlayStoryIds,
      ...PACKAGE_DATATABLE_STORY_IDS,
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

    const assetBundleText = readStaticAssetBundleText();
    if (!assetBundleText.includes(PACKAGE_TEXT)) {
      throw new Error(
        `Built package Storybook assets did not include story text for ${PACKAGE_TEXT_STORY_ID}.`,
      );
    }

    for (const storyId of overlayStoryIds) {
      const storyText = overlayStoryTexts[storyId];
      if (!assetBundleText.includes(storyText)) {
        throw new Error(
          `Built package Storybook assets did not include story text for ${storyId}.`,
        );
      }

      console.log(
        `Verified package Storybook story ${storyId} at ${storyUrl(storyId)} without dashboard providers.`,
      );
    }

    if (!assetBundleText.includes(hostResponsibilityCopy)) {
      throw new Error(
        "Built package Storybook assets did not include overlay host responsibility docs copy.",
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
