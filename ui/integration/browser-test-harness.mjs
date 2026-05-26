// @vitest-environment node

import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import process from "node:process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

import { chromium } from "playwright";

const dirname = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(dirname, "..");
const replayFixtureDirectory = path.join(dirname, "fixtures");

export const previewHost = "127.0.0.1";
export const buildTimeoutMs = 600_000;
export const browserScenarioTimeoutMs = 240_000;
export const readyTimeoutMs = 90_000;
export const replayDelayMs = 25;
export const uiInteractionTimeoutMs = 10_000;
export const defaultFactorySessionID = "~default";
const sharedApiPort = 43117;
const sharedPreviewPort = 43118;
const browserBuildCacheKey = "__agentFactoryBrowserIntegrationBuildComplete";
let browserArtifactSequence = 0;
export const exportCoverImagePath = path.resolve(
  packageRoot,
  "..",
  "docs",
  "internal",
  "resources",
  "dashboard.png",
);
export const initialEditableFactoryDefinitionVersion = {
  logical: 1,
  physical: "2026-05-19T00:00:00Z",
};

const sessionFactoryPathPattern = /^\/factory-sessions\/([^/]+)\/factory$/;
const sessionEventsPathPattern = /^\/factory-sessions\/([^/]+)\/events$/;
const promptTemplateContractPathPattern =
  /^\/factory-sessions\/([^/]+)\/factory\/workstations\/[^/]+\/prompt-template-contract$/;
const promptTemplateValidationPathPattern =
  /^\/factory-sessions\/([^/]+)\/factory\/workstations\/[^/]+\/prompt-template-validation$/;

function buildReplayPromptTemplateContract() {
  return {
    availableVariables: [],
    inputCount: 0,
    unavailableAccessPatterns: [],
  };
}

function buildReplayPromptTemplateValidationResult() {
  return {
    diagnostics: [],
    valid: true,
  };
}

function createBunEnv(extraEnv = {}, options = {}) {
  const env = {
    ...process.env,
    ...extraEnv,
  };

  if (options.stripVitestEnv) {
    delete env.NODE_ENV;
    for (const key of Object.keys(env)) {
      if (key === "VITEST" || key.startsWith("VITEST_")) {
        delete env[key];
      }
    }
  }

  if (options.nodeEnv) {
    env.NODE_ENV = options.nodeEnv;
  }

  return env;
}

function bunCommand() {
  return process.platform === "win32" ? "bun.exe" : "bun";
}

function npmCommand() {
  return process.platform === "win32" ? "npm.cmd" : "npm";
}

function browserArtifactDirectory() {
  const configuredPath = process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR?.trim();
  if (!configuredPath) {
    return null;
  }
  return path.resolve(packageRoot, configuredPath);
}

function sanitizeArtifactLabel(value) {
  const normalized = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return normalized.length > 0 ? normalized : "browser-session";
}

function localPackageBinaryCommand(name) {
  const suffix = process.platform === "win32" ? ".cmd" : "";
  return path.join(packageRoot, "node_modules", ".bin", `${name}${suffix}`);
}

function hasBun() {
  const result = spawnSync(bunCommand(), ["--version"], {
    shell: false,
    stdio: "ignore",
  });
  return result.status === 0;
}

function resolveRuntimeCommand(args) {
  if (hasBun()) {
    return {
      args,
      command: bunCommand(),
    };
  }

  if (args[0] === "run") {
    return {
      args: ["run", ...args.slice(1)],
      command: npmCommand(),
    };
  }

  if (args[0] === "x") {
    const [_, binaryName, ...binaryArgs] = args;
    return {
      args: binaryArgs,
      command: localPackageBinaryCommand(binaryName),
    };
  }

  throw new Error(`Unsupported local runtime command fallback: ${args.join(" ")}`);
}

function spawnRuntime(args, extraEnv = {}, options = {}) {
  const runtime = resolveRuntimeCommand(args);
  return spawn(runtime.command, runtime.args, {
    cwd: packageRoot,
    env: createBunEnv(extraEnv, options),
    shell: false,
    stdio: "pipe",
  });
}

async function runRuntime(
  args,
  extraEnv = {},
  timeoutMs = buildTimeoutMs,
  options = {},
) {
  const child = spawnRuntime(args, extraEnv, options);
  let stdout = "";
  let stderr = "";

  child.stdout?.on("data", (chunk) => {
    stdout += chunk.toString();
  });
  child.stderr?.on("data", (chunk) => {
    stderr += chunk.toString();
  });

  const timeout = setTimeout(() => {
    child.kill("SIGTERM");
  }, timeoutMs);

  try {
    const [code, signal] = await once(child, "exit");
    if (code !== 0) {
      const runtime = resolveRuntimeCommand(args);
      throw new Error(
        [
          `${runtime.command} ${runtime.args.join(" ")} exited with ${code ?? "null"} / ${signal ?? "null"}.`,
          stdout.trim(),
          stderr.trim(),
        ]
          .filter((part) => part.length > 0)
          .join("\n"),
      );
    }
  } finally {
    clearTimeout(timeout);
  }
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null) {
    return;
  }

  if (process.platform === "win32") {
    const killer = spawn("taskkill", ["/pid", String(child.pid), "/t", "/f"], {
      shell: false,
      stdio: "ignore",
    });
    await once(killer, "exit");
    return;
  }

  child.kill("SIGTERM");
  await once(child, "exit");
}

async function waitForURL(url, timeoutMs = readyTimeoutMs) {
  const deadline = Date.now() + timeoutMs;

  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      if (response.ok) {
        return;
      }
    } catch {
      // Retry until the deadline while preview is starting.
    }

    await delay(250);
  }

  throw new Error(`Timed out waiting for ${url}.`);
}

function createDefaultSession(currentFactoryDefinition) {
  return {
    factoryDir: currentFactoryDefinition?.factoryDirectory ?? "/replay/factory",
    folderPath: currentFactoryDefinition?.sourceDirectory ?? "/replay/factory",
    id: defaultFactorySessionID,
    isDefault: true,
    project: currentFactoryDefinition?.name ?? "replay",
    target: {
      kind: "default",
    },
  };
}

function cloneVersion(version) {
  return {
    logical: version.logical,
    physical: version.physical,
  };
}

function buildSessionMap(sessions, currentFactory, currentFactoryBySessionID) {
  const defaultSession = sessions.find(
    (session) => session.id === defaultFactorySessionID,
  ) ?? createDefaultSession(currentFactory);
  const nextSessions = sessions.some(
    (session) => session.id === defaultFactorySessionID,
  )
    ? sessions
    : [defaultSession, ...sessions];

  const state = new Map();
  for (const session of nextSessions) {
    const sessionFactory =
      currentFactoryBySessionID[session.id] ??
      (session.id === defaultFactorySessionID ? currentFactory : null);
    state.set(session.id, {
      currentFactory: sessionFactory,
      eventLines: [],
      session,
      version: cloneVersion(initialEditableFactoryDefinitionVersion),
    });
  }

  return {
    sessions: nextSessions,
    state,
  };
}

function ensureSessionState(
  sessionRegistry,
  session,
  currentFactory,
  currentFactoryBySessionID,
) {
  const existingState = sessionRegistry.state.get(session.id);
  if (existingState) {
    existingState.session = session;
  } else {
    const sessionFactory =
      currentFactoryBySessionID[session.id] ??
      (session.id === defaultFactorySessionID ? currentFactory : null);
    sessionRegistry.state.set(session.id, {
      currentFactory: sessionFactory,
      eventLines: [],
      session,
      version: cloneVersion(initialEditableFactoryDefinitionVersion),
    });
  }

  if (!sessionRegistry.sessions.some((existingSession) => existingSession.id === session.id)) {
    sessionRegistry.sessions = [...sessionRegistry.sessions, session];
    return;
  }

  sessionRegistry.sessions = sessionRegistry.sessions.map((existingSession) =>
    existingSession.id === session.id ? session : existingSession,
  );
}

export async function startBrowserPreview() {
  const apiPort = sharedApiPort;
  const previewPort = sharedPreviewPort;
  const apiOrigin = `http://${previewHost}:${apiPort}`;
  const previewURL = `http://${previewHost}:${previewPort}/dashboard/ui/`;

  const globalBuildState = globalThis;
  if (!globalBuildState[browserBuildCacheKey]) {
    await runRuntime(
      ["run", "build"],
      {
        VITE_AGENT_FACTORY_API_ORIGIN: apiOrigin,
      },
      buildTimeoutMs,
      {
        nodeEnv: "production",
        stripVitestEnv: true,
      },
    );
    globalBuildState[browserBuildCacheKey] = true;
  }

  const previewProcess = spawnRuntime(
    [
      "x",
      "vite",
      "preview",
      "--host",
      previewHost,
      "--port",
      String(previewPort),
      "--strictPort",
    ],
    {
      AGENT_FACTORY_API_ORIGIN: apiOrigin,
    },
    {
      nodeEnv: "production",
      stripVitestEnv: true,
    },
  );

  await waitForURL(previewURL);

  return {
    apiOrigin,
    apiPort,
    previewPort,
    previewURL,
    stop: async () => {
      await stopProcess(previewProcess);
    },
  };
}

export async function loadReplayLines(fileName) {
  return (await readFile(path.join(replayFixtureDirectory, fileName), "utf8"))
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
}

export async function openBrowserPage(options = {}) {
  browserArtifactSequence += 1;
  const artifactDirectory = browserArtifactDirectory();
  const artifactLabel = sanitizeArtifactLabel(
    options.artifactLabel ??
      `browser-session-${String(browserArtifactSequence).padStart(2, "0")}`,
  );
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    acceptDownloads: options.acceptDownloads ?? false,
  });
  if (artifactDirectory) {
    await context.tracing.start({ screenshots: true, snapshots: true });
  }
  const page = await context.newPage();
  const pageErrors = [];
  const consoleErrors = [];

  page.on("pageerror", (error) => {
    pageErrors.push(error.stack ?? error.message);
  });
  page.on("console", (message) => {
    if (message.type() === "error") {
      consoleErrors.push(message.text());
    }
  });

  return {
    artifactDirectory,
    artifactLabel,
    browser,
    consoleErrors,
    context,
    page,
    pageErrors,
    close: async () => {
      const artifactWarnings = [];
      if (artifactDirectory) {
        await mkdir(artifactDirectory, { recursive: true });
        if (!page.isClosed()) {
          try {
            await page.screenshot({
              fullPage: true,
              path: path.join(artifactDirectory, `${artifactLabel}.png`),
            });
          } catch (error) {
            artifactWarnings.push(`screenshot: ${error.message}`);
          }
          try {
            await writeFile(
              path.join(artifactDirectory, `${artifactLabel}.html`),
              await page.content(),
              "utf8",
            );
          } catch (error) {
            artifactWarnings.push(`html: ${error.message}`);
          }
        }
        try {
          await context.tracing.stop({
            path: path.join(artifactDirectory, `${artifactLabel}.trace.zip`),
          });
        } catch (error) {
          artifactWarnings.push(`trace: ${error.message}`);
        }
        await writeFile(
          path.join(artifactDirectory, `${artifactLabel}.diagnostics.json`),
          JSON.stringify(
            {
              artifactLabel,
              consoleErrors,
              pageErrors,
              warnings: artifactWarnings,
              writtenAt: new Date().toISOString(),
            },
            null,
            2,
          ),
          "utf8",
        );
      }
      await page.close();
      await context.close();
      await browser.close();
    },
  };
}

export function expectNoBrowserErrors(pageErrors, consoleErrors, expect) {
  expect(pageErrors).toEqual([]);
  expect(consoleErrors).toEqual([]);
}

export async function startFactoryApiServer({
  activationResponseFactory = null,
  apiPort,
  currentFactory = null,
  currentFactoryBySessionID = {},
  eventLines = [],
  eventLinesBySessionID = {},
  onActivateFactory = null,
  onOpenFactorySession = null,
  onSaveCurrentFactory = null,
  pauseBeforeTick = null,
  sessions = [],
} = {}) {
  const requestedEventSessionIDs = [];
  let resolveReplayCompleted = () => {};
  const replayCompleted = new Promise((resolve) => {
    resolveReplayCompleted = resolve;
  });
  let resolveReplayPaused = () => {};
  const replayPaused =
    pauseBeforeTick === null
      ? Promise.resolve()
      : new Promise((resolve) => {
          resolveReplayPaused = resolve;
        });
  let pauseReleased = false;
  let resumeReplayStream = () => {};
  const releaseReplayStream = () => {
    if (pauseReleased) {
      return;
    }
    pauseReleased = true;
    resumeReplayStream();
  };
  const replayPauseReleased =
    pauseBeforeTick === null
      ? Promise.resolve()
      : new Promise((resolve) => {
          resumeReplayStream = resolve;
        });

  const sessionRegistry = buildSessionMap(
    sessions,
    currentFactory,
    currentFactoryBySessionID,
  );

  for (const [sessionID, definition] of Object.entries(eventLinesBySessionID)) {
    const sessionState = sessionRegistry.state.get(sessionID);
    if (sessionState) {
      sessionState.eventLines = definition;
    }
  }

  if (sessionRegistry.state.has(defaultFactorySessionID)) {
    sessionRegistry.state.get(defaultFactorySessionID).eventLines = eventLines;
  }

  function buildCurrentFactoryDocument(sessionID) {
    const sessionState = sessionRegistry.state.get(sessionID);
    return {
      ...sessionState.currentFactory,
      version: sessionState.version,
    };
  }

  function bumpEditableFactoryDefinitionVersion(sessionID) {
    const sessionState = sessionRegistry.state.get(sessionID);
    sessionState.version = {
      logical: sessionState.version.logical + 1,
      physical: new Date().toISOString(),
    };
  }

  const server = http.createServer((request, response) => {
    if (request.method === "OPTIONS") {
      response.writeHead(204, {
        "Access-Control-Allow-Headers": "Content-Type",
        "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
        "Access-Control-Allow-Origin": "*",
      });
      response.end();
      return;
    }

    if (request.url === "/favicon.ico") {
      response.writeHead(204, {
        "Access-Control-Allow-Origin": "*",
      });
      response.end();
      return;
    }

    if (request.url === "/factory-sessions" && request.method === "GET") {
      response.writeHead(200, {
        "Access-Control-Allow-Origin": "*",
        "Content-Type": "application/json",
      });
      response.end(
        JSON.stringify({
          sessions: sessionRegistry.sessions,
        }),
      );
      return;
    }

    if (request.url === "/factory-sessions" && request.method === "POST") {
      let requestBody = "";
      request.setEncoding("utf8");
      request.on("data", (chunk) => {
        requestBody += chunk;
      });
      request.on("end", async () => {
        const body = requestBody.length === 0 ? null : JSON.parse(requestBody);
        const result = onOpenFactorySession
          ? await onOpenFactorySession(body)
          : {
              session: null,
              targets: [],
            };

        if (result?.session) {
          ensureSessionState(
            sessionRegistry,
            result.session,
            currentFactory,
            currentFactoryBySessionID,
          );
          const openedSessionState = sessionRegistry.state.get(result.session.id);
          if (openedSessionState) {
            openedSessionState.eventLines = eventLinesBySessionID[result.session.id] ?? [];
          }
        }

        response.writeHead(200, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(JSON.stringify(result));
      });
      return;
    }

    const sessionFactoryMatch = request.url?.match(sessionFactoryPathPattern);
    if (sessionFactoryMatch && request.method === "GET") {
      const sessionID = decodeURIComponent(sessionFactoryMatch[1]);
      const sessionState = sessionRegistry.state.get(sessionID);
      if (!sessionState || sessionState.currentFactory === null) {
        response.writeHead(404, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(
          JSON.stringify({
            code: "NOT_FOUND",
            message: "The current factory definition is not available.",
          }),
        );
        return;
      }

      response.writeHead(200, {
        "Access-Control-Allow-Origin": "*",
        "Content-Type": "application/json",
      });
      response.end(JSON.stringify(buildCurrentFactoryDocument(sessionID)));
      return;
    }

    if (sessionFactoryMatch && request.method === "PUT") {
      const sessionID = decodeURIComponent(sessionFactoryMatch[1]);
      const sessionState = sessionRegistry.state.get(sessionID);
      if (!sessionState) {
        response.writeHead(404, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(
          JSON.stringify({
            code: "NOT_FOUND",
            message: "The selected factory session is not available.",
          }),
        );
        return;
      }

      let requestBody = "";
      request.setEncoding("utf8");
      request.on("data", (chunk) => {
        requestBody += chunk;
      });
      request.on("end", async () => {
        const body = requestBody.length === 0 ? null : JSON.parse(requestBody);
        if (!body || typeof body !== "object" || body.name == null) {
          response.writeHead(400, {
            "Access-Control-Allow-Origin": "*",
            "Content-Type": "application/json",
          });
          response.end(
            JSON.stringify({
              code: "BAD_REQUEST",
              message: "The current factory payload is required.",
            }),
          );
          return;
        }

        if (onSaveCurrentFactory) {
          await onSaveCurrentFactory({
            body,
            sessionID,
          });
        }
        sessionState.currentFactory = body;
        bumpEditableFactoryDefinitionVersion(sessionID);
        response.writeHead(200, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(JSON.stringify(buildCurrentFactoryDocument(sessionID)));
      });
      return;
    }

    if (
      request.url?.match(promptTemplateContractPathPattern) &&
      request.method === "GET"
    ) {
      response.writeHead(200, {
        "Access-Control-Allow-Origin": "*",
        "Content-Type": "application/json",
      });
      response.end(JSON.stringify(buildReplayPromptTemplateContract()));
      return;
    }

    if (
      request.url?.match(promptTemplateValidationPathPattern) &&
      request.method === "POST"
    ) {
      response.writeHead(200, {
        "Access-Control-Allow-Origin": "*",
        "Content-Type": "application/json",
      });
      response.end(JSON.stringify(buildReplayPromptTemplateValidationResult()));
      return;
    }

    if (request.url === "/factories" && request.method === "POST") {
      let requestBody = "";
      request.setEncoding("utf8");
      request.on("data", (chunk) => {
        requestBody += chunk;
      });
      request.on("end", async () => {
        const body = requestBody.length === 0 ? null : JSON.parse(requestBody);
        if (onActivateFactory) {
          await onActivateFactory(body);
        }

        const defaultSession =
          sessionRegistry.state.get(defaultFactorySessionID);
        defaultSession.currentFactory = activationResponseFactory ?? body;
        bumpEditableFactoryDefinitionVersion(defaultFactorySessionID);
        response.writeHead(200, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(JSON.stringify(defaultSession.currentFactory));
      });
      return;
    }

    const sessionEventsMatch = request.url?.match(sessionEventsPathPattern);
    if (sessionEventsMatch && request.method === "GET") {
      const sessionID = decodeURIComponent(sessionEventsMatch[1]);
      const sessionState = sessionRegistry.state.get(sessionID);
      requestedEventSessionIDs.push(sessionID);
      if (!sessionState) {
        response.writeHead(404, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "text/plain; charset=utf-8",
        });
        response.end("not found");
        return;
      }

      response.writeHead(200, {
        "Access-Control-Allow-Origin": "*",
        "Cache-Control": "no-cache, no-transform",
        Connection: "keep-alive",
        "Content-Type": "text/event-stream",
      });
      response.flushHeaders?.();

      let closed = false;
      let pauseReached = false;
      request.on("close", () => {
        closed = true;
      });

      void (async () => {
        for (const line of sessionState.eventLines) {
          if (closed) {
            return;
          }
          if (pauseBeforeTick !== null && !pauseReached) {
            const eventTick = JSON.parse(line).context?.tick;
            if (typeof eventTick === "number" && eventTick > pauseBeforeTick) {
              pauseReached = true;
              resolveReplayPaused();
              await replayPauseReleased;
              if (closed) {
                return;
              }
            }
          }
          response.write(`data: ${line}\n\n`);
          await delay(replayDelayMs);
        }
        if (pauseBeforeTick !== null && !pauseReached) {
          resolveReplayPaused();
        }
        if (!closed) {
          response.write(": replay-complete\n\n");
          resolveReplayCompleted();
        }
      })();
      return;
    }

    response.writeHead(404, {
      "Access-Control-Allow-Origin": "*",
      "Content-Type": "text/plain; charset=utf-8",
    });
    response.end("not found");
  });

  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(apiPort, previewHost, resolve);
  });

  return {
    releaseReplayStream,
    replayCompleted,
    replayPaused,
    requestedEventSessionIDs,
    stop: async () => {
      server.closeAllConnections?.();
      await new Promise((resolve, reject) => {
        server.close((error) => {
          if (error) {
            reject(error);
            return;
          }

          resolve();
        });
      });
    },
  };
}
