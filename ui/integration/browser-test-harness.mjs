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

/**
 * Poll until a durable checkpoint becomes true (API request captured, download
 * hook populated, dialog closed). Prefer this over asserting transient status
 * copy, animation frames, or heading visibility during teardown.
 */
export async function waitForDurableCheckpoint(
  label,
  conditionFn,
  timeoutMs = uiInteractionTimeoutMs,
  intervalMs = 100,
) {
  const deadline = Date.now() + timeoutMs;

  while (Date.now() < deadline) {
    if (await conditionFn()) {
      return;
    }
    await delay(intervalMs);
  }

  throw new Error(`Timed out waiting for durable checkpoint: ${label}`);
}

/**
 * Poll until a form action control is enabled. Use after filling required
 * fields instead of waiting for helper/status copy such as selected-file labels.
 */
export async function waitForDurableControlEnabled(
  locator,
  timeoutMs = uiInteractionTimeoutMs,
) {
  await waitForDurableCheckpoint(
    "control enabled",
    async () => await locator.isEnabled(),
    timeoutMs,
  );
}

/**
 * Wait for a Radix/dialog surface to close using dialog role state instead of
 * heading copy that may unmount asynchronously.
 */
export async function waitForDialogHidden(
  dialogLocator,
  timeoutMs = uiInteractionTimeoutMs,
) {
  await dialogLocator.waitFor({
    state: "hidden",
    timeout: timeoutMs,
  });
}

/**
 * Race a captured download hook against a dialog alert. Requires
 * `installCapturedDownloadHook(page)` before triggering export.
 */
export async function waitForCapturedDownloadOrDialogError(
  page,
  dialogLocator,
  timeoutMs = uiInteractionTimeoutMs,
) {
  const outcome = await Promise.race([
    page
      .waitForFunction(
        () => window.__agentFactoryCapturedDownloads.length > 0,
        null,
        { timeout: timeoutMs },
      )
      .then(() => "download"),
    dialogLocator
      .getByRole("alert")
      .waitFor({
        state: "visible",
        timeout: timeoutMs,
      })
      .then(() => "error"),
  ]);

  if (outcome === "error") {
    throw new Error(await dialogLocator.getByRole("alert").innerText());
  }
}

const workstationPromptEditorTimeoutMs = 60_000;

/** Playwright locator for Monaco's hidden input textarea (aria-label varies by a11y mode). */
export function getWorkstationPromptBodyTextarea(scope) {
  return scope.locator(".monaco-editor textarea.inputarea").first();
}

/** Fill a workstation prompt editor after Monaco has mounted in the browser. */
export async function fillWorkstationPromptBody(scope, text) {
  const monacoEditor = scope.locator(".monaco-editor").first();
  await monacoEditor.waitFor({
    state: "visible",
    timeout: workstationPromptEditorTimeoutMs,
  });

  const textarea = getWorkstationPromptBodyTextarea(scope);
  try {
    await textarea.waitFor({
      state: "visible",
      timeout: 5_000,
    });
    await textarea.fill(text);
    await textarea.press("Tab");
    return;
  } catch {
    await monacoEditor.click();
    await scope.page().keyboard.type(text);
  }
}

/** Check a shared styled checkbox whose native input is visually hidden. */
export async function checkStyledCheckbox(checkboxLocator) {
  await checkboxLocator.check({ force: true });
}

/** Select a Radix/shadcn combobox option by visible option label. */
export async function selectComboboxOption(combobox, optionName) {
  const page = combobox.page();
  await combobox.click();
  await page.getByRole("option", { name: optionName, exact: true }).click();
}

/** Open a labeled combobox within scope and choose an option by visible label. */
export async function selectLabeledComboboxOption(scope, label, optionName) {
  await selectComboboxOption(
    scope.getByRole("combobox", { name: label }),
    optionName,
  );
}

/** Fill the default model-worker operation contract in the add-worker dialog. */
export async function fillModelWorkerAddOperationDraft(
  scope,
  {
    inputSlotName = "text",
    operationName = "TTS",
    outputSlotName = "audio",
  } = {},
) {
  const addOperationButton = scope.getByRole("button", {
    name: "Add operation",
  });
  await addOperationButton.scrollIntoViewIfNeeded();
  await addOperationButton.click();

  const operationNameField = scope.locator(
    "#factory-graph-add-model-operation-name-0",
  );
  await operationNameField.scrollIntoViewIfNeeded();
  await operationNameField.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await operationNameField.fill(operationName);

  const inputSlotNameField = scope.locator(
    "#factory-graph-add-model-operation-input-slot-name-0",
  );
  await inputSlotNameField.scrollIntoViewIfNeeded();
  await inputSlotNameField.fill(inputSlotName);
  await checkStyledCheckbox(
    scope.locator(
      "#factory-graph-add-model-operation-input-slot-0-content-type-TEXT",
    ),
  );

  const outputSlotNameField = scope.locator(
    "#factory-graph-add-model-operation-output-slot-name-0",
  );
  await outputSlotNameField.scrollIntoViewIfNeeded();
  await outputSlotNameField.fill(outputSlotName);
  await checkStyledCheckbox(
    scope.locator(
      "#factory-graph-add-model-operation-output-slot-0-content-type-AUDIO",
    ),
  );
}

const modelProviderOptionLabels = {
  CLAUDE: "Claude",
  CODEX: "Codex",
  CURSOR: "Cursor",
  GEMINI: "Gemini",
  KIRO: "Kiro",
  OPENCODE: "OpenCode",
};

/** Map editable model-provider enum values to combobox option labels. */
export function modelProviderOptionLabel(value) {
  return modelProviderOptionLabels[value] ?? value;
}

const browserBuildCacheKey = "__agentFactoryBrowserIntegrationBuildComplete";
let browserArtifactSequence = 0;
let sharedBrowserPorts = null;
export const exportCoverImagePath = path.resolve(
  packageRoot,
  "..",
  "docs",
  "internal",
  "resources",
  "dashboard.png",
);
export const initialEditableFactoryDefinitionVersion = {
  logical: "1",
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

export function browserArtifactDirectory() {
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

async function findAvailablePort() {
  const probe = http.createServer();
  await new Promise((resolve, reject) => {
    probe.once("error", reject);
    probe.listen(0, previewHost, resolve);
  });

  const address = probe.address();
  if (!address || typeof address === "string") {
    throw new Error("Expected browser integration port probe to bind to TCP.");
  }

  await new Promise((resolve, reject) => {
    probe.close((error) => {
      if (error) {
        reject(error);
        return;
      }
      resolve();
    });
  });

  return address.port;
}

function configuredPort(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    return null;
  }

  const port = Number(value);
  if (!Number.isInteger(port) || port <= 0 || port > 65_535) {
    throw new Error(`Expected ${name} to be a valid TCP port, got "${value}".`);
  }
  return port;
}

async function browserPreviewPorts() {
  if (sharedBrowserPorts) {
    return sharedBrowserPorts;
  }

  const apiPort = configuredPort("AGENT_FACTORY_BROWSER_API_PORT");
  const previewPort = configuredPort("AGENT_FACTORY_BROWSER_PREVIEW_PORT");
  sharedBrowserPorts = {
    apiPort: apiPort ?? (await findAvailablePort()),
    previewPort: previewPort ?? (await findAvailablePort()),
  };
  return sharedBrowserPorts;
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

  throw new Error(
    `Unsupported local runtime command fallback: ${args.join(" ")}`,
  );
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

function replayEventLineWithFactoryVersion(line, version) {
  let event;
  try {
    event = JSON.parse(line);
  } catch {
    return line;
  }

  if (
    !event ||
    typeof event !== "object" ||
    !event.payload ||
    typeof event.payload !== "object" ||
    !event.payload.factory ||
    typeof event.payload.factory !== "object" ||
    event.payload.factory.version !== undefined
  ) {
    return line;
  }

  return JSON.stringify({
    ...event,
    payload: {
      ...event.payload,
      factory: {
        ...event.payload.factory,
        version: cloneVersion(version),
      },
    },
  });
}

function _buildSavedFactoryReplayLine(factory, version) {
  return JSON.stringify({
    context: {
      eventTime: version.physical,
      sequence: 1,
      tick: 1,
    },
    id: `saved-factory-${version.logical}`,
    payload: {
      factory: {
        ...factory,
        version: cloneVersion(version),
      },
    },
    type: "FACTORY_CHANGE",
  });
}

function buildSavedFactoryReplayLines(factory, version) {
  return [
    JSON.stringify({
      context: {
        eventTime: version.physical,
        sequence: 1,
        tick: 1,
      },
      id: `saved-structure-${version.logical}`,
      payload: {
        factory: {
          ...factory,
          version: cloneVersion(version),
        },
      },
      type: "INITIAL_STRUCTURE_REQUEST",
    }),
    JSON.stringify({
      context: {
        eventTime: version.physical,
        sequence: 2,
        tick: 2,
      },
      id: `saved-factory-state-${version.logical}`,
      payload: {
        previousState: "RUNNING",
        reason: "saved fixture ready",
        state: "FINISHED",
      },
      type: "FACTORY_STATE_RESPONSE",
    }),
  ];
}

function buildSessionMap(sessions, currentFactory, currentFactoryBySessionID) {
  const defaultSession =
    sessions.find((session) => session.id === defaultFactorySessionID) ??
    createDefaultSession(currentFactory);
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

  if (
    !sessionRegistry.sessions.some(
      (existingSession) => existingSession.id === session.id,
    )
  ) {
    sessionRegistry.sessions = [...sessionRegistry.sessions, session];
    return;
  }

  sessionRegistry.sessions = sessionRegistry.sessions.map((existingSession) =>
    existingSession.id === session.id ? session : existingSession,
  );
}

export async function startBrowserPreview() {
  const { apiPort, previewPort } = await browserPreviewPorts();
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

function isBenignBrowserError(error) {
  const message = (error ?? "").trim();
  if (message === "") {
    return true;
  }

  return (
    message.startsWith("Canceled: Canceled") &&
    (message.includes("setModel") || message.includes("dispose"))
  );
}

export function expectNoBrowserErrors(pageErrors, consoleErrors, expect) {
  expect(pageErrors.filter((error) => !isBenignBrowserError(error))).toEqual(
    [],
  );
  expect(consoleErrors.filter((error) => !isBenignBrowserError(error))).toEqual(
    [],
  );
}

export async function startFactoryApiServer({
  apiPort,
  currentFactory = null,
  currentFactoryBySessionID = {},
  eventLines = [],
  eventLinesBySessionID = {},
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
          const openedSessionState = sessionRegistry.state.get(
            result.session.id,
          );
          if (openedSessionState) {
            openedSessionState.eventLines =
              eventLinesBySessionID[result.session.id] ?? [];
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
        const parsedBody =
          requestBody.length === 0 ? null : JSON.parse(requestBody);
        const factory =
          parsedBody &&
          typeof parsedBody === "object" &&
          parsedBody.factoryDefinition &&
          typeof parsedBody.factoryDefinition === "object"
            ? parsedBody.factoryDefinition
            : parsedBody &&
                typeof parsedBody === "object" &&
                parsedBody.factory &&
                typeof parsedBody.factory === "object"
              ? parsedBody.factory
            : parsedBody;
        const normalizedFactory =
          factory && typeof factory === "object"
            ? {
                ...(sessionState.currentFactory?.name != null &&
                factory.name == null
                  ? { name: sessionState.currentFactory.name }
                  : null),
                ...factory,
              }
            : null;
        if (!normalizedFactory || normalizedFactory.name == null) {
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
            body: normalizedFactory,
            mode:
              parsedBody &&
              typeof parsedBody === "object" &&
              typeof parsedBody.mode === "string"
                ? parsedBody.mode
                : undefined,
            requestBody: parsedBody,
            sessionID,
          });
        }
        sessionState.currentFactory = normalizedFactory;
        bumpEditableFactoryDefinitionVersion(sessionID);
        sessionState.eventLines = buildSavedFactoryReplayLines(
          sessionState.currentFactory,
          sessionState.version,
        );
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

    if (request.url === "/factory-validations" && request.method === "POST") {
      response.writeHead(200, {
        "Access-Control-Allow-Origin": "*",
        "Content-Type": "application/json",
      });
      response.end(JSON.stringify({ targets: [] }));
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
          response.write(
            `data: ${replayEventLineWithFactoryVersion(
              line,
              sessionState.version,
            )}\n\n`,
          );
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
