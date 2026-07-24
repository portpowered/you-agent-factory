import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import net from "node:net";
import path from "node:path";
import process from "node:process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const uiRoot = path.resolve(packageRoot, "..", "..");
const require = createRequire(import.meta.url);
const { chromium } = require(path.join(uiRoot, "node_modules/playwright"));

const host = process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_HOST ?? "127.0.0.1";
const port = Number(process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_PORT ?? "3818");
const staticDir = path.join(packageRoot, "storybook-static");
const baseUrl = `http://${host}:${port}`;
const OVERFLOW_TOLERANCE_PX = 4;
const STORY_RENDER_TIMEOUT_MS = 30_000;

const responsiveStories = [
  {
    id: "recipes-widgetframe--responsive-compact",
    label: "Responsive compact widget frame",
  },
  {
    id: "recipes-widgetframe--responsive-wide",
    label: "Responsive wide widget frame",
  },
];

const viewports = [
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

async function waitForStoryRender(page) {
  await page.waitForSelector("#storybook-root", {
    state: "attached",
    timeout: STORY_RENDER_TIMEOUT_MS,
  });
  await page.waitForFunction(
    () => {
      const root = document.querySelector("#storybook-root");
      return root instanceof HTMLElement && root.childElementCount > 0;
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

async function verifyResponsiveStory(browser, story, viewport) {
  const context = await browser.newContext({
    viewport: { height: viewport.height, width: viewport.width },
  });
  const page = await context.newPage();

  try {
    await page.goto(storyUrl(story.id), {
      timeout: STORY_RENDER_TIMEOUT_MS,
      waitUntil: "domcontentloaded",
    });
    await waitForStoryRender(page);

    const frame = page.getByRole("article", {
      name: "Example widget with a longer heading label",
    });
    const refreshButton = page.getByRole("button", { name: "Refresh" });
    const expandButton = page.getByRole("button", { name: "Expand details" });

    await frame.waitFor({ state: "visible", timeout: STORY_RENDER_TIMEOUT_MS });
    await refreshButton.waitFor({
      state: "visible",
      timeout: STORY_RENDER_TIMEOUT_MS,
    });
    await expandButton.waitFor({
      state: "visible",
      timeout: STORY_RENDER_TIMEOUT_MS,
    });

    const shell = page.locator("[data-widget-frame-story-shell='true']");
    const shellMetrics = await shell.evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }));

    if (
      shellMetrics.scrollWidth >
      shellMetrics.clientWidth + OVERFLOW_TOLERANCE_PX
    ) {
      throw new Error(
        `${story.label} shell overflowed at ${viewport.label}: scrollWidth=${shellMetrics.scrollWidth}, clientWidth=${shellMetrics.clientWidth}.`,
      );
    }

    const [titleBox, refreshBox, expandBox] = await Promise.all([
      frame
        .getByRole("heading", {
          level: 3,
          name: "Example widget with a longer heading label",
        })
        .boundingBox(),
      refreshButton.boundingBox(),
      expandButton.boundingBox(),
    ]);

    if (!titleBox || !refreshBox || !expandBox) {
      throw new Error(
        `${story.label} controls were not measurable at ${viewport.label}.`,
      );
    }

    const controlsOverlap =
      titleBox.right > refreshBox.left - 1 &&
      titleBox.bottom > refreshBox.top + 1 &&
      titleBox.top < refreshBox.bottom - 1 &&
      titleBox.left < refreshBox.right - 1;

    if (controlsOverlap) {
      throw new Error(
        `${story.label} title overlapped the refresh action at ${viewport.label}.`,
      );
    }

    await expectNoHorizontalOverflow(
      page,
      `${story.label} at ${viewport.label}`,
    );
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

    for (const viewport of viewports) {
      for (const story of responsiveStories) {
        await verifyResponsiveStory(browser, story, viewport);
        console.log(
          `Verified ${story.id} at ${viewport.label} (${viewport.width}x${viewport.height}).`,
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
