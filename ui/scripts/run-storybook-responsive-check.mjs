import {
  HOST,
  PORT,
  assertPortAvailable,
  spawnBun,
  stopServer,
  verifyStorybookIndex,
  waitForStorybookReady,
} from "./run-storybook-ci.mjs";
import { main as verifyResponsiveStories } from "./verify-import-export-storybook-responsive.mjs";

function formatExit(code, signal) {
  if (code !== null) {
    return `code ${code}`;
  }

  return `signal ${signal ?? "unknown"}`;
}

export function createServerExitPromise(server) {
  return new Promise((_, reject) => {
    server.once("error", reject);
    server.once("exit", (code, signal) => {
      reject(
        new Error(
          `Storybook static server exited before the responsive checks completed (${formatExit(code, signal)}).`,
        ),
      );
    });
  });
}

export async function ensureStorybookServer({
  assertAvailable = assertPortAvailable,
  host = HOST,
  port = PORT,
  spawnProcess = spawnBun,
  verifyIndex = verifyStorybookIndex,
  waitReady = waitForStorybookReady,
} = {}) {
  try {
    await verifyIndex({
      url: `http://${host}:${port}/index.json`,
    });

    return {
      startedServer: false,
      stop: async () => {},
    };
  } catch {
    await assertAvailable(host, port);
  }

  const server = spawnProcess([
    "x",
    "--no-install",
    "http-server",
    "storybook-static",
    "-p",
    port,
    "-a",
    host,
    "-s",
  ]);

  await waitReady({ serverExit: createServerExitPromise(server) });

  return {
    startedServer: true,
    stop: async (stopProcess = stopServer) => stopProcess(server),
  };
}

export async function main({
  ensureServer = ensureStorybookServer,
  stopProcess = stopServer,
  verifyResponsive = verifyResponsiveStories,
} = {}) {
  const server = await ensureServer();

  try {
    await verifyResponsive();
  } finally {
    await server.stop(stopProcess);
  }
}

if (import.meta.main) {
  await main();
}
