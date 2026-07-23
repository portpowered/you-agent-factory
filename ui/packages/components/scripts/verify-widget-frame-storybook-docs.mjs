import { spawn } from "node:child_process";
import { readFileSync } from "node:fs";
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
const port = Number(process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_PORT ?? "3819");
const staticDir = path.join(packageRoot, "storybook-static");
const baseUrl = `http://${host}:${port}`;
const STORY_RENDER_TIMEOUT_MS = 30_000;

/** Story ids documented in docs/widget-frame-recipes.md */
const documentedStories = [
  {
    articleName: "Example widget",
    id: "recipes-widgetframe--success-content",
    label: "Success content",
  },
  {
    articleName: "Example widget",
    id: "recipes-widgetframe--empty-state",
    label: "Empty state",
  },
  {
    articleName: "Example widget",
    id: "recipes-widgetframe--loading-state",
    label: "Loading state",
  },
  {
    articleName: "Example widget",
    id: "recipes-widgetframe--error-state",
    label: "Error state",
  },
  {
    articleName: "Example widget",
    id: "recipes-widgetframe--success-state",
    label: "Success state",
  },
  {
    articleName: "Example widget",
    id: "recipes-widgetframe--collapsed-disclosure",
    label: "Collapsed disclosure",
  },
  {
    articleName: "Example widget",
    id: "recipes-widgetframe--expanded-disclosure",
    label: "Expanded disclosure",
  },
  {
    articleName: "Example widget with a longer heading label",
    id: "recipes-widgetframe--responsive-compact",
    label: "Responsive compact",
  },
  {
    articleName: "Example widget with a longer heading label",
    id: "recipes-widgetframe--responsive-medium",
    label: "Responsive medium",
  },
  {
    articleName: "Example widget with a longer heading label",
    id: "recipes-widgetframe--responsive-wide",
    label: "Responsive wide",
  },
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

async function verifyDocumentedStory(browser, story) {
  const context = await browser.newContext({
    viewport: { height: 900, width: 1280 },
  });
  const page = await context.newPage();

  try {
    await page.goto(storyUrl(story.id), {
      timeout: STORY_RENDER_TIMEOUT_MS,
      waitUntil: "domcontentloaded",
    });
    await waitForStoryRender(page);

    const frame = page.getByRole("article", { name: story.articleName });
    await frame.waitFor({ state: "visible", timeout: STORY_RENDER_TIMEOUT_MS });
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

  const indexPayload = JSON.parse(
    readFileSync(path.join(staticDir, "index.json"), "utf8"),
  );

  for (const story of documentedStories) {
    if (!indexPayload.entries?.[story.id]) {
      throw new Error(
        `Documented story ${story.id} (${story.label}) is missing from Storybook index.`,
      );
    }
  }

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

    for (const story of documentedStories) {
      await verifyDocumentedStory(browser, story);
      console.log(`Verified documented story ${story.id} (${story.label}).`);
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
