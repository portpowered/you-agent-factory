import { spawn } from "node:child_process";
import net from "node:net";
import path from "node:path";
import process from "node:process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import {
  expectNoHorizontalOverflow,
  expectVisible,
  storyUrl,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";
import {
  DASHBOARD_FORM_FIELD_RESPONSIVE_VIEWPORTS,
  DASHBOARD_FORM_FIELD_STORY_CHECKS,
} from "./verify-dashboard-form-field-storybook-responsive.mjs";

const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const host = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const port = Number(process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "3829");
const staticDir = path.join(uiRoot, "storybook-static");
const baseUrl = `http://${host}:${port}`;
const STORY_RENDER_TIMEOUT_MS = 30_000;

function assertPortAvailable(hostName, portNumber) {
  return new Promise((resolve, reject) => {
    const server = net.createServer();

    server.once("error", (error) => {
      if (error?.code === "EADDRINUSE") {
        reject(
          new Error(
            `Port ${portNumber} on ${hostName} is already in use. Choose another AGENT_FACTORY_STORYBOOK_PORT.`,
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
    cwd: uiRoot,
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

async function verifyStory(browser, storyCheck, viewport) {
  const context = await browser.newContext({
    viewport: { height: viewport.height, width: viewport.width },
  });
  const page = await context.newPage();

  try {
    await page.goto(storyUrl(baseUrl, storyCheck.id), {
      timeout: STORY_RENDER_TIMEOUT_MS,
      waitUntil: "domcontentloaded",
    });
    await waitForStoryRender(page);

    await storyCheck.verify({
      expectNoHorizontalOverflow: (currentPage, label) =>
        expectNoHorizontalOverflow(currentPage, label),
      expectVisible: (locator, label) => expectVisible(locator, label),
      page,
      viewport,
    });
  } finally {
    await context.close();
  }
}

async function main() {
  await assertPortAvailable(host, port);

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

  const browser = await chromium.launch({ headless: true });

  try {
    await waitForHttpOk(`${baseUrl}/index.json`);

    for (const storyCheck of DASHBOARD_FORM_FIELD_STORY_CHECKS) {
      for (const viewport of DASHBOARD_FORM_FIELD_RESPONSIVE_VIEWPORTS) {
        await verifyStory(browser, storyCheck, viewport);
        console.log(
          `Verified ${storyCheck.id} at ${viewport.label} (${viewport.width}x${viewport.height}).`,
        );
      }
    }
  } finally {
    await browser.close();
    cleanup();
    await delay(500);
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
