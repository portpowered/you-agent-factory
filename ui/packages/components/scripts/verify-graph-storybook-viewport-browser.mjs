import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import net from "node:net";
import path from "node:path";
import process from "node:process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const { chromium } = require("../../../node_modules/playwright");

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const host = process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_HOST ?? "127.0.0.1";
const port = Number(
  process.env.AGENT_FACTORY_PACKAGE_GRAPH_STORYBOOK_PORT ?? "3825",
);
const staticDir = path.join(packageRoot, "storybook-static");
const baseUrl = `http://${host}:${port}`;

const GRAPH_VIEWPORT_STORIES = [
  {
    id: "graphs-graphinteractiveexamples--interactive",
    label: "interactive",
    minHeight: 200,
    minWidth: 200,
  },
  {
    id: "graphs-graphinteractiveexamples--desktop-viewport",
    label: "desktop viewport",
    minHeight: 200,
    minWidth: 200,
  },
  {
    id: "graphs-graphinteractiveexamples--narrow-viewport",
    label: "narrow viewport",
    minHeight: 200,
    minWidth: 200,
  },
];

function assertPortAvailable(hostName, portNumber) {
  return new Promise((resolve, reject) => {
    const server = net.createServer();

    server.once("error", (error) => {
      if (error?.code === "EADDRINUSE") {
        reject(
          new Error(
            `Port ${portNumber} on ${hostName} is already in use. Choose another AGENT_FACTORY_PACKAGE_GRAPH_STORYBOOK_PORT.`,
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

async function verifyGraphViewportStory(page, story) {
  const iframeUrl = `${baseUrl}/iframe.html?id=${story.id}&viewMode=story`;
  await page.goto(iframeUrl, {
    timeout: 90_000,
    waitUntil: "networkidle",
  });

  const viewport = page.locator('[data-graph-viewport-surface="true"]').first();
  await viewport.waitFor({ state: "visible", timeout: 30_000 });

  const box = await viewport.boundingBox();
  if (!box || box.height < story.minHeight) {
    throw new Error(
      `Expected ${story.label} graph viewport height >= ${story.minHeight}px, got ${box?.height ?? 0}px (${iframeUrl}).`,
    );
  }
  if (!box || box.width < story.minWidth) {
    throw new Error(
      `Expected ${story.label} graph viewport width >= ${story.minWidth}px, got ${box?.width ?? 0}px (${iframeUrl}).`,
    );
  }

  await page.getByRole("button", { name: "Ready node" }).waitFor({
    state: "visible",
    timeout: 30_000,
  });
}

async function main() {
  await assertPortAvailable(host, port);

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

    const page = await browser.newPage();
    for (const story of GRAPH_VIEWPORT_STORIES) {
      await verifyGraphViewportStory(page, story);
      console.log(
        `Verified graph viewport sizing for ${story.label} at ${baseUrl}/iframe.html?id=${story.id}`,
      );
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
