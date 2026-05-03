import { spawn } from "node:child_process";
import net from "node:net";
import process from "node:process";
import { setTimeout as delay } from "node:timers/promises";

export const HOST = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
export const PORT = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";

const READY_TIMEOUT_MS = 30000;
const POST_READY_SETTLE_MS = 1000;
const STORYBOOK_URL = `http://${HOST}:${PORT}`;
const STORYBOOK_INDEX_URL = `${STORYBOOK_URL}/index.json`;

function formatExit(code, signal) {
  if (code !== null) {
    return `code ${code}`;
  }

  return `signal ${signal ?? "unknown"}`;
}

function spawnBun(args, options = {}) {
  return spawn("bun", args, {
    cwd: process.cwd(),
    env: {
      ...process.env,
      AGENT_FACTORY_STORYBOOK_HOST: HOST,
      AGENT_FACTORY_STORYBOOK_PORT: PORT,
    },
    stdio: "inherit",
    shell: false,
    ...options,
  });
}

function runBun(args) {
  return new Promise((resolve, reject) => {
    const child = spawnBun(args);

    child.once("error", reject);
    child.once("exit", (code, signal) => {
      if (code === 0) {
        resolve();
        return;
      }

      reject(new Error(`bun ${args.join(" ")} exited with ${formatExit(code, signal)}.`));
    });
  });
}

export function createPortInUseError(host, port) {
  return new Error(
    `Port ${port} on ${host} is already in use. Stop the existing listener before running the dashboard Storybook interaction check.`,
  );
}

export function assertPortAvailable(host, port) {
  return new Promise((resolve, reject) => {
    const server = net.createServer();

    server.once("error", (error) => {
      if (error && typeof error === "object" && "code" in error && error.code === "EADDRINUSE") {
        reject(createPortInUseError(host, port));
        return;
      }

      reject(error);
    });

    server.listen(Number(port), host, () => {
      server.close((closeError) => {
        if (closeError) {
          reject(closeError);
          return;
        }

        resolve();
      });
    });
  });
}

export function createStorybookIndexTimeoutError(url = STORYBOOK_INDEX_URL, timeoutMs = READY_TIMEOUT_MS) {
  return new Error(
    `Timed out waiting for Storybook index at ${url} within ${timeoutMs}ms.`,
  );
}

export async function verifyStorybookIndex({
  fetchFn = fetch,
  url = STORYBOOK_INDEX_URL,
  maxAttempts = 10,
  retryDelayMs = 250,
  delayFn = delay,
} = {}) {
  for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
    try {
      const response = await fetchFn(url);

      if (!response.ok) {
        throw new Error(`Received ${response.status} from ${url}.`);
      }

      await response.json();
      return;
    } catch (error) {
      if (attempt === maxAttempts) {
        throw error;
      }

      await delayFn(retryDelayMs);
    }
  }
}

export async function waitForStableStorybookIndex({
  verifyIndex = () => verifyStorybookIndex(),
  delayFn = delay,
  settleMs = POST_READY_SETTLE_MS,
  nowFn = Date.now,
} = {}) {
  const deadline = nowFn() + settleMs;

  while (nowFn() < deadline) {
    await verifyIndex();

    const remainingMs = deadline - nowFn();
    if (remainingMs <= 0) {
      return;
    }

    await delayFn(Math.min(250, remainingMs));
  }
}

async function stopServer(child) {
  if (!child.pid || child.exitCode !== null) {
    return;
  }

  if (process.platform === "win32") {
    await new Promise((resolve, reject) => {
      const killer = spawn("taskkill", ["/pid", String(child.pid), "/t", "/f"], {
        stdio: "ignore",
        shell: false,
      });

      killer.once("error", reject);
      killer.once("exit", () => resolve());
    });
    return;
  }

  child.kill("SIGTERM");

  await new Promise((resolve) => {
    child.once("exit", () => resolve());
  });
}

export async function waitForStorybookReady({
  runWaitOn = () =>
    runBun([
      "x",
      "--no-install",
      "wait-on",
      "--timeout",
      String(READY_TIMEOUT_MS),
      STORYBOOK_INDEX_URL,
    ]),
  waitForStableIndex = () => waitForStableStorybookIndex(),
  serverExit,
} = {}) {
  await Promise.race([
    Promise.resolve()
      .then(runWaitOn)
      .catch(() => {
        throw createStorybookIndexTimeoutError();
      }),
    serverExit,
  ]);
  await waitForStableIndex();
}

export async function main() {
  await assertPortAvailable(HOST, PORT);

  const server = spawnBun([
    "x",
    "--no-install",
    "http-server",
    "storybook-static",
    "-p",
    PORT,
    "-a",
    HOST,
    "-s",
  ]);
  let shuttingDown = false;

  const serverExit = new Promise((_, reject) => {
    server.once("error", reject);
    server.once("exit", (code, signal) => {
      if (shuttingDown) {
        return;
      }

      reject(
        new Error(
          `Storybook static server exited before readiness or interaction tests completed (${formatExit(code, signal)}).`,
        ),
      );
    });
  });

  try {
    await waitForStorybookReady({ serverExit });
    await Promise.race([runBun(["run", "storybook:test-runner:ci"]), serverExit]);
    await Promise.race([runBun(["run", "storybook:responsive-check"]), serverExit]);
  } finally {
    shuttingDown = true;
    await stopServer(server);
  }
}

if (import.meta.main) {
  await main();
}
