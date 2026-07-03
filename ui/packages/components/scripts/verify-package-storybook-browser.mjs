import { spawn } from "node:child_process";
import { readFileSync, readdirSync, rmSync } from "node:fs";
import net from "node:net";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const host = process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_HOST ?? "127.0.0.1";
const port = Number(
  process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_PORT ?? "3817",
);
const storiesToVerify = [
  {
    id: "primitives-packagetext--body",
    text: "Hello from the component package",
  },
  {
    id: "data-display-datatable--success",
    text: "Signal Router",
  },
  {
    id: "data-display-datatable--loading",
    text: "Loading product catalog",
  },
  {
    id: "data-display-datatable--empty",
    text: "No products match the current filters",
  },
  {
    id: "data-display-datatable--error-state",
    text: "Unable to load product catalog data",
  },
  {
    id: "data-display-datatable--dense",
    text: "Queue depth",
  },
  {
    id: "data-display-datatable--long-cell",
    text: "Provider session emitted a long diagnostic payload",
  },
  {
    id: "data-display-datatable--narrow-viewport",
    text: "Signal Router",
  },
];
const staticDir = path.join(packageRoot, "storybook-static");
const baseUrl = `http://${host}:${port}`;
const indexUrl = `${baseUrl}/index.json`;
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
    const assetBundleText = readStaticAssetBundleText();

    if (
      assetBundleText.includes("DashboardSessionProvider") ||
      assetBundleText.includes("@tanstack/react-query")
    ) {
      throw new Error(
        "Built package Storybook assets appear to include dashboard runtime providers.",
      );
    }

    for (const story of storiesToVerify) {
      const storyEntry = indexPayload.entries?.[story.id];

      if (!storyEntry) {
        throw new Error(
          `Expected package story ${story.id} in ${indexUrl}, found ${Object.keys(indexPayload.entries ?? {}).join(", ")}`,
        );
      }

      const storyIframeUrl = `${baseUrl}/iframe.html?id=${story.id}&viewMode=story`;
      const iframeResponse = await waitForHttpOk(storyIframeUrl);
      if (!iframeResponse.ok) {
        throw new Error(`Expected ${storyIframeUrl} to return HTTP 200.`);
      }

      if (!assetBundleText.includes(story.text)) {
        throw new Error(
          `Built package Storybook assets did not include story text for ${story.id}.`,
        );
      }

      console.log(
        `Verified package Storybook story ${story.id} at ${storyIframeUrl} without dashboard providers.`,
      );
    }
  } finally {
    cleanup();
    await delay(500);
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
