import { spawn } from "node:child_process";
import net from "node:net";
import path from "node:path";
import process from "node:process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

import { verifyPackageLayoutDisplayStories } from "./verify-package-layout-display-storybook-responsive.mjs";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const host = process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_HOST ?? "127.0.0.1";
const port = Number(process.env.AGENT_FACTORY_PACKAGE_STORYBOOK_PORT ?? "3817");
const staticDir = path.join(packageRoot, "storybook-static");
const storybookUrl = `http://${host}:${port}`;

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

async function ensureStorybookServer() {
  const indexUrl = `${storybookUrl}/index.json`;

  try {
    const response = await fetch(indexUrl, {
      signal: AbortSignal.timeout(10_000),
    });
    if (response.ok) {
      return {
        startedServer: false,
        stop: async () => {},
      };
    }
  } catch {
    // Build and serve below.
  }

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

  await waitForHttpOk(indexUrl);

  return {
    startedServer: true,
    stop: async () => {
      if (!serverExited && !server.killed) {
        server.kill("SIGTERM");
      }
      await delay(500);
    },
  };
}

const server = await ensureStorybookServer();

try {
  await verifyPackageLayoutDisplayStories({ storybookUrl });
  console.log(
    "Package typography/layout/display Storybook browser verification passed.",
  );
} finally {
  await server.stop();
}
