// @vitest-environment node

import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { mkdir, readFile, stat, writeFile } from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import process from "node:process";
import readline from "node:readline";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

import { chromium } from "playwright";

import { runSharedBrowserBuild } from "./browser-build-lock.mjs";

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
export const resolvedDefaultFactorySessionID =
  "019e0000-0000-7000-8000-000000000042";
export const timelineCheckpointDBVersion = 3;
export const timelineCheckpointSchemaVersion = 4;

export function emptyMaterializedWorkOutcomeState(cursor) {
  return {
    accumulator: {
      activeDispatchesByID: {},
      appliedEventCount: 0,
      completedAcceptedCount: 0,
      completedDispatchCount: 0,
      failedWorkItemsByID: {},
      initialPlaceIDs: [],
      workItemsByID: {},
    },
    counts: {
      completed: 0,
      dispatched: 0,
      failed: 0,
      inFlight: 0,
      queued: 0,
    },
    cursor: {
      eventID: cursor.afterEventId,
      eventTime: cursor.eventTime ?? "1970-01-01T00:00:00Z",
      sequence: cursor.afterSequence,
      tick: cursor.selectedTick,
    },
    failedByWorkType: {},
    failedWorkLabels: [],
    samples: [],
    version: 1,
  };
}

function isDefaultFactorySessionSelector(sessionID) {
  return (
    sessionID === defaultFactorySessionID ||
    sessionID === resolvedDefaultFactorySessionID
  );
}

function resolveRegistrySessionID(sessionID) {
  return isDefaultFactorySessionSelector(sessionID)
    ? defaultFactorySessionID
    : sessionID;
}

function resolvedFactorySessionIDForSession(session) {
  return session.isDefault || session.id === defaultFactorySessionID
    ? resolvedDefaultFactorySessionID
    : session.id;
}

function logicalSessionKeyIDForSession(session) {
  const targetKind = session.target?.kind ?? "default";
  const targetName = session.target?.name;
  const nameSuffix =
    typeof targetName === "string" && targetName.length > 0
      ? `::${targetName}`
      : "::";
  return `${session.folderPath}::${targetKind}${nameSuffix}`;
}

function buildStreamIdentityForSession(session, streamGenerationID) {
  return {
    backendScopeID: `${session.folderPath}::browser-integration`,
    factorySessionID: resolvedFactorySessionIDForSession(session),
    logicalSessionKeyID: logicalSessionKeyIDForSession(session),
    streamGenerationID,
  };
}

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

/** Wait for the dashboard session sync-preflight handshake to succeed. */
export async function waitForDashboardSyncPreflight(
  page,
  timeoutMs = readyTimeoutMs,
) {
  await page.waitForResponse(
    (response) => {
      try {
        return (
          sessionSyncPreflightPathPattern.test(
            new URL(response.url()).pathname,
          ) && response.ok()
        );
      } catch {
        return false;
      }
    },
    { timeout: timeoutMs },
  );
}

/**
 * Poll until the dashboard inline widget picker is mounted. Prefer this over
 * heading-only readiness checkpoints because the header can render during
 * sync-preflight recovery or empty-session shells before the bento mounts.
 */
export async function waitForDashboardWidgetPicker(
  page,
  timeoutMs = readyTimeoutMs,
) {
  await waitForDurableCheckpoint(
    "dashboard widget picker",
    async () =>
      page
        .getByRole("combobox", { name: "Browse widgets" })
        .isVisible()
        .catch(() => false),
    timeoutMs,
  );
  return page.getByRole("combobox", { name: "Browse widgets" });
}

/** Open the dashboard and wait for sync-preflight plus the widget picker. */
export async function gotoDashboardAndWaitForWidgetPicker(
  page,
  url,
  timeoutMs = readyTimeoutMs,
) {
  const syncPreflightResponse = waitForDashboardSyncPreflight(page, timeoutMs);
  await page.goto(url, { waitUntil: "domcontentloaded" });
  await syncPreflightResponse;
  await waitForDashboardWidgetPicker(page, timeoutMs);
}

/**
 * Seed a timeline checkpoint for the next dashboard load without letting an
 * initial shell visit clobber the fixture via debounced checkpoint persistence.
 */
export async function openDashboardWithSeededCheckpoint(
  page,
  previewURL,
  seedCheckpoint,
) {
  await page.goto(previewURL, { waitUntil: "domcontentloaded" });
  await delay(800);
  await seedCheckpoint();
  await page.reload({ waitUntil: "domcontentloaded" });
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
const browserProcessStateKey = "__agentFactoryBrowserIntegrationBrowserState";
const browserPreviewStateKey = "__agentFactoryBrowserIntegrationPreviewState";
const repoProcessGroupKey = Symbol("repoProcessGroup");
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
const sessionSyncPreflightPathPattern =
  /^\/factory-sessions\/([^/]+)\/sync-preflight(?:\?.*)?$/;
const sessionEventsPathPattern =
  /^\/factory-sessions\/([^/]+)\/events(?:\?.*)?$/;
const factorySessionReadPathPattern = /^\/factory-sessions\/([^/]+)$/;
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
  const root = path.resolve(packageRoot, configuredPath);
  if (process.env.AGENT_FACTORY_BROWSER_ARTIFACT_WORKER_ISOLATION !== "true") {
    return root;
  }
  const workerID =
    process.env.VITEST_POOL_ID ??
    process.env.VITEST_WORKER_ID ??
    String(process.pid);
  return path.join(root, `worker-${sanitizeArtifactLabel(workerID)}`);
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

async function browserDistReady() {
  try {
    await stat(path.join(packageRoot, "dist", "index.html"));
    await stat(path.join(packageRoot, "dist", "assets", "index.js"));
    await stat(path.join(packageRoot, "dist", "assets", "index.css"));
    const bundle = await readFile(
      path.join(packageRoot, "dist", "assets", "index.js"),
      "utf8",
    );
    // Rebuild when the preview bundle predates session sync-preflight bootstrap.
    return bundle.includes("/sync-preflight");
  } catch {
    return false;
  }
}

export async function findAvailablePort() {
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

async function assertPortAvailable(host, port) {
  const probe = http.createServer();
  let listening = false;
  try {
    await new Promise((resolve, reject) => {
      probe.once("error", reject);
      probe.listen(port, host, () => {
        listening = true;
        resolve();
      });
    });
  } finally {
    if (listening) {
      await new Promise((resolve, reject) => {
        probe.close((error) => {
          if (error) {
            reject(error);
            return;
          }
          resolve();
        });
      });
    }
  }
}

/**
 * Poll until a TCP port is free so sequential browser integration suites can
 * reuse the shared preview API port after a real-backend harness stops.
 */
export async function waitForPortAvailable(
  host = previewHost,
  port,
  timeoutMs = 15_000,
  intervalMs = 250,
) {
  const deadline = Date.now() + timeoutMs;
  let lastError = null;

  while (Date.now() < deadline) {
    try {
      await assertPortAvailable(host, port);
      return;
    } catch (error) {
      lastError = error;
      if (
        error &&
        typeof error === "object" &&
        "code" in error &&
        error.code !== "EADDRINUSE"
      ) {
        throw error;
      }
      await delay(intervalMs);
    }
  }

  const suffix =
    lastError instanceof Error
      ? `: ${lastError.message}`
      : lastError
        ? `: ${lastError}`
        : ".";
  throw new Error(
    `Timed out waiting for ${host}:${port} to become available${suffix}`,
  );
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

async function isolatedBrowserPreviewPorts() {
  const apiPort = await findAvailablePort();
  let previewPort = await findAvailablePort();
  while (previewPort === apiPort) {
    previewPort = await findAvailablePort();
  }
  return { apiPort, previewPort };
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
  const child = spawn(runtime.command, runtime.args, {
    cwd: packageRoot,
    detached: process.platform !== "win32",
    env: createBunEnv(extraEnv, options),
    shell: false,
    stdio: "pipe",
  });
  child[repoProcessGroupKey] = process.platform !== "win32";
  return child;
}

function spawnRepoProcess(command, args, options = {}) {
  const child = spawn(command, args, {
    cwd: path.resolve(packageRoot, ".."),
    detached: process.platform !== "win32",
    env: createBunEnv(options.extraEnv),
    shell: false,
    stdio: ["ignore", "pipe", "pipe"],
  });
  child[repoProcessGroupKey] = process.platform !== "win32";
  return child;
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
  if (!child || child.exitCode !== null || child.signalCode !== null) {
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

  const exited = once(child, "exit");
  if (child[repoProcessGroupKey]) {
    // Commands such as `go run` launch the compiled program as a child process.
    // Terminate the process group so that child cannot retain the shared API
    // port after the launcher exits.
    try {
      process.kill(-child.pid, "SIGTERM");
    } catch (error) {
      if (error?.code !== "ESRCH") {
        throw error;
      }
    }
  } else {
    child.kill("SIGTERM");
  }
  await exited;
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

function buildSessionSyncPreflightResponse(
  request,
  sessionState,
  requestedSessionId,
) {
  const requestURL = new URL(request.url ?? "/", `http://${previewHost}`);
  const afterEventID = requestURL.searchParams.get("after_event_id");
  const afterSequenceValue = requestURL.searchParams.get("after_sequence");
  const afterSequence =
    afterSequenceValue != null ? Number(afterSequenceValue) : null;
  const reconnectCursorProvided =
    typeof afterEventID === "string" || afterSequenceValue != null;
  const reconnectCursorValid =
    typeof afterEventID === "string" &&
    afterEventID.length > 0 &&
    typeof afterSequence === "number" &&
    Number.isFinite(afterSequence);
  if (!sessionState) {
    return {
      checkpointReusable: false,
      reasonCode: "session_not_found",
      reconnectCursor: {
        provided: reconnectCursorProvided,
        validForStreamGeneration: false,
      },
      requestedSessionId,
    };
  }

  const reconnectCursor = {
    provided: reconnectCursorProvided,
    validForStreamGeneration: reconnectCursorValid,
  };
  if (reconnectCursorValid && typeof afterEventID === "string") {
    reconnectCursor.afterEventId = afterEventID;
  }
  if (
    reconnectCursorValid &&
    typeof afterSequence === "number" &&
    Number.isFinite(afterSequence)
  ) {
    reconnectCursor.afterSequence = afterSequence;
  }

  return {
    backendScopeId: `${sessionState.session.folderPath}::browser-integration`,
    checkpointReusable: reconnectCursorValid,
    factorySessionId: resolvedFactorySessionIDForSession(sessionState.session),
    logicalSessionKeyId: logicalSessionKeyIDForSession(sessionState.session),
    reasonCode: "ok",
    reconnectCursor,
    requestedSessionId,
    streamGenerationId: sessionState.version.physical,
  };
}

async function createBrowserPreview(ports = null) {
  const { apiPort, previewPort } = ports ?? (await browserPreviewPorts());
  const apiOrigin = `http://${previewHost}:${apiPort}`;
  const previewURL = `http://${previewHost}:${previewPort}/dashboard/ui/`;
  const sourceMapBuild =
    process.env.AGENT_FACTORY_PROFILE_SOURCEMAPS === "true" ||
    process.env.AGENT_FACTORY_PROFILE_SOURCEMAPS === "1";
  const buildCacheKey = sourceMapBuild
    ? `${browserBuildCacheKey}:sourcemaps`
    : browserBuildCacheKey;

  await runSharedBrowserBuild({
    build: () =>
      runRuntime(
        ["run", "build"],
        {
          AGENT_FACTORY_PROFILE_SOURCEMAPS: sourceMapBuild ? "true" : "false",
        },
        buildTimeoutMs,
        {
          nodeEnv: "production",
          stripVitestEnv: true,
        },
      ),
    buildCacheKey,
    ready: browserDistReady,
  });

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

export async function startIsolatedBrowserPreview() {
  const preview = await startBrowserPreview();
  const apiPort = await findAvailablePort();
  return {
    ...preview,
    apiOrigin: `http://${previewHost}:${apiPort}`,
    apiPort,
    stop: async () => {},
  };
}

export async function startDedicatedBrowserPreview() {
  return createBrowserPreview(await isolatedBrowserPreviewPorts());
}

function browserPreviewState() {
  const globalState = globalThis;
  if (!globalState[browserPreviewStateKey]) {
    globalState[browserPreviewStateKey] = {
      cleanupRegistered: false,
      preview: null,
      previewPromise: null,
    };
  }
  return globalState[browserPreviewStateKey];
}

function browserProcessState() {
  const globalState = globalThis;
  if (!globalState[browserProcessStateKey]) {
    globalState[browserProcessStateKey] = {
      browser: null,
      browserPromise: null,
      cleanupRegistered: false,
    };
  }
  return globalState[browserProcessStateKey];
}

export async function startBrowserPreview() {
  const state = browserPreviewState();
  if (!state.previewPromise) {
    state.previewPromise = createBrowserPreview()
      .then((preview) => {
        state.preview = preview;
        if (!state.cleanupRegistered) {
          state.cleanupRegistered = true;
          process.once("exit", () => {
            if (state.preview) {
              state.preview.stop().catch(() => {});
            }
          });
        }
        return preview;
      })
      .catch((error) => {
        state.previewPromise = null;
        throw error;
      });
  }

  const preview = await state.previewPromise;
  return {
    ...preview,
    stop: stopBrowserPreview,
  };
}

export async function stopBrowserPreview() {
  const state = browserPreviewState();
  if (state.preview) {
    await state.preview.stop();
  }
  state.preview = null;
  state.previewPromise = null;
  sharedBrowserPorts = null;
}

export async function loadReplayLines(fileName) {
  return (await readFile(path.join(replayFixtureDirectory, fileName), "utf8"))
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
}

function recordBrowserDiagnostic(
  diagnostics,
  value,
  { characterLimit, entryLimit },
) {
  diagnostics.push(
    characterLimit === null ? value : String(value).slice(0, characterLimit),
  );
  if (entryLimit !== null) {
    diagnostics.splice(0, Math.max(0, diagnostics.length - entryLimit));
  }
}

export function installBrowserErrorCapture(
  page,
  { characterLimit = null, entryLimit = null } = {},
) {
  const pageErrors = [];
  const consoleErrors = [];
  const diagnosticPolicy = { characterLimit, entryLimit };

  page.on("pageerror", (error) => {
    recordBrowserDiagnostic(
      pageErrors,
      error.stack ?? error.message,
      diagnosticPolicy,
    );
  });
  page.on("console", (message) => {
    if (message.type() === "error") {
      recordBrowserDiagnostic(consoleErrors, message.text(), diagnosticPolicy);
    }
  });

  return { consoleErrors, pageErrors };
}

async function captureFullBrowserArtifacts({
  artifactDirectory,
  artifactLabel,
  consoleErrors,
  context,
  page,
  pageErrors,
}) {
  const warnings = [];
  if (!page.isClosed()) {
    try {
      await page.screenshot({
        fullPage: true,
        path: path.join(artifactDirectory, `${artifactLabel}.png`),
      });
    } catch (error) {
      warnings.push(`screenshot: ${error.message}`);
    }
    try {
      await writeFile(
        path.join(artifactDirectory, `${artifactLabel}.html`),
        await page.content(),
        "utf8",
      );
    } catch (error) {
      warnings.push(`html: ${error.message}`);
    }
  }
  try {
    await context.tracing.stop({
      path: path.join(artifactDirectory, `${artifactLabel}.trace.zip`),
    });
  } catch (error) {
    warnings.push(`trace: ${error.message}`);
  }
  await writeFile(
    path.join(artifactDirectory, `${artifactLabel}.diagnostics.json`),
    JSON.stringify(
      {
        artifactLabel,
        consoleErrors,
        pageErrors,
        warnings,
        writtenAt: new Date().toISOString(),
      },
      null,
      2,
    ),
    "utf8",
  );
}

async function captureBoundedBrowserArtifacts({
  artifactDirectory,
  artifactLabel,
  consoleErrors,
  pageErrors,
}) {
  await writeFile(
    path.join(artifactDirectory, `${artifactLabel}.diagnostics.json`),
    JSON.stringify(
      {
        artifactLabel,
        consoleErrorCount: consoleErrors.length,
        pageErrorCount: pageErrors.length,
        writtenAt: new Date().toISOString(),
      },
      null,
      2,
    ),
    "utf8",
  );
}

async function closeBrowserPageResources({
  artifactDirectory,
  artifactLabel,
  boundedArtifacts,
  consoleErrors,
  context,
  page,
  pageErrors,
}) {
  const failures = [];
  try {
    if (artifactDirectory) {
      await mkdir(artifactDirectory, { recursive: true });
      const capture = boundedArtifacts
        ? captureBoundedBrowserArtifacts
        : captureFullBrowserArtifacts;
      await capture({
        artifactDirectory,
        artifactLabel,
        consoleErrors,
        context,
        page,
        pageErrors,
      });
    }
  } catch (error) {
    failures.push(error);
  }
  for (const close of [
    () => (page.isClosed() ? undefined : page.close()),
    () => context.close(),
  ]) {
    try {
      await close();
    } catch (error) {
      failures.push(error);
    }
  }
  if (failures.length > 0) {
    throw new AggregateError(
      failures,
      "failed to capture artifacts or close browser page",
    );
  }
}

export async function openBrowserPage(options = {}) {
  browserArtifactSequence += 1;
  const artifactDirectory = browserArtifactDirectory();
  const boundedArtifacts = options.artifactMode === "bounded";
  const diagnosticLimit = boundedArtifacts
    ? (options.diagnosticLimit ?? 16)
    : null;
  const diagnosticCharacterLimit = boundedArtifacts
    ? (options.diagnosticCharacterLimit ?? 512)
    : null;
  const artifactLabel = sanitizeArtifactLabel(
    options.artifactLabel ??
      `browser-session-${String(browserArtifactSequence).padStart(2, "0")}`,
  );
  const state = browserProcessState();
  if (!state.browserPromise) {
    state.browserPromise = chromium
      .launch({ headless: true })
      .then((browser) => {
        state.browser = browser;
        if (!state.cleanupRegistered) {
          state.cleanupRegistered = true;
          process.once("exit", () => {
            if (state.browser) {
              state.browser.close().catch(() => {});
            }
          });
        }
        return browser;
      });
  }
  const browser = await state.browserPromise;
  const context = await browser.newContext({
    acceptDownloads: options.acceptDownloads ?? false,
  });
  if (options.apiOrigin) {
    await context.addInitScript((apiOrigin) => {
      globalThis.__agentFactoryBrowserTestAPIOrigin = apiOrigin;
    }, options.apiOrigin);
  }
  let page;
  try {
    if (artifactDirectory && !boundedArtifacts) {
      await context.tracing.start({ screenshots: true, snapshots: true });
    }
    page = await context.newPage();
  } catch (error) {
    await context.close().catch(() => {});
    throw error;
  }
  const { consoleErrors, pageErrors } = installBrowserErrorCapture(page, {
    characterLimit: diagnosticCharacterLimit,
    entryLimit: diagnosticLimit,
  });

  return {
    artifactDirectory,
    artifactLabel,
    browser,
    consoleErrors,
    context,
    page,
    pageErrors,
    close: () =>
      closeBrowserPageResources({
        artifactDirectory,
        artifactLabel,
        boundedArtifacts,
        consoleErrors,
        context,
        page,
        pageErrors,
      }),
  };
}

export async function startRealBackendBrowserHarness({
  apiPort,
  factoryDir = path.resolve(packageRoot, "..", "factory"),
  requestID = "req-browser-runtime-001",
  startMode = "sync",
  workflowFixture,
  workflowName,
} = {}) {
  if (!workflowFixture || !workflowName) {
    throw new Error(
      "startRealBackendBrowserHarness requires workflowFixture and workflowName.",
    );
  }

  const child = spawnRepoProcess(
    "go",
    [
      "run",
      "./tests/functional/internal/support/cmd/browser_api_harness",
      "--api-port",
      String(apiPort),
      "--factory-dir",
      factoryDir,
      "--request-id",
      requestID,
      "--start-mode",
      startMode,
      "--workflow-fixture",
      workflowFixture,
      "--workflow-name",
      workflowName,
    ],
    {
      extraEnv: {
        CGO_ENABLED: process.env.CGO_ENABLED ?? "0",
      },
    },
  );

  let stderr = "";
  child.stderr?.on("data", (chunk) => {
    stderr += chunk.toString();
  });

  const lineReader = readline.createInterface({
    input: child.stdout,
  });
  const ready = new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(
        new Error(
          `Timed out waiting for real backend browser harness readiness.\n${stderr.trim()}`,
        ),
      );
    }, readyTimeoutMs);

    function rejectWithProcessExit(code, signal) {
      clearTimeout(timeout);
      reject(
        new Error(
          `Real backend browser harness exited before readiness: code=${code ?? "null"} signal=${signal ?? "null"}\n${stderr.trim()}`,
        ),
      );
    }

    child.once("exit", rejectWithProcessExit);
    lineReader.once("line", (line) => {
      clearTimeout(timeout);
      child.off("exit", rejectWithProcessExit);
      try {
        resolve(JSON.parse(line));
      } catch (error) {
        reject(
          new Error(
            `Failed to parse real backend browser harness ready payload: ${line}\n${error.message}\n${stderr.trim()}`,
          ),
        );
      }
    });
  });

  try {
    const payload = await ready;
    return {
      apiOrigin: payload.apiOrigin,
      sessionID: payload.sessionId,
      stop: async () => {
        lineReader.close();
        await stopProcess(child);
      },
    };
  } catch (error) {
    lineReader.close();
    await stopProcess(child);
    throw error;
  }
}

function isBenignBrowserError(error) {
  const message = (error ?? "").trim();
  if (message === "") {
    return true;
  }

  if (message.startsWith("Failed to load resource:")) {
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

  function sessionStateForRequest(sessionID) {
    return sessionRegistry.state.get(resolveRegistrySessionID(sessionID));
  }

  function buildCurrentFactoryDocument(sessionID) {
    const sessionState = sessionStateForRequest(sessionID);
    return {
      ...sessionState.currentFactory,
      version: sessionState.version,
    };
  }

  function buildFactorySessionDocument(sessionID) {
    const sessionState = sessionStateForRequest(sessionID);
    if (!sessionState) {
      return null;
    }

    const lifecycleTimestamp = sessionState.version.physical;
    const factoryState =
      sessionState.eventLines.length > 0 ? "FINISHED" : "IDLE";

    return {
      factoryDir: sessionState.session.factoryDir,
      folderPath: sessionState.session.folderPath,
      id: resolvedFactorySessionIDForSession(sessionState.session),
      isDefault: sessionState.session.isDefault,
      project: sessionState.session.project,
      runtime: {
        lifecycle: {
          startedAt: lifecycleTimestamp,
          updatedAt: lifecycleTimestamp,
        },
        orchestratorKind: "PETRI",
        progress: {
          categories: {
            failed: 0,
            initial: 0,
            processing: 0,
            terminal: 0,
          },
          factoryState,
          inFlightCount: 0,
          totalTokens: 0,
        },
        status: "IDLE",
        streamIdentity: buildStreamIdentityForSession(
          sessionState.session,
          lifecycleTimestamp,
        ),
        usage: {
          resources: [],
        },
      },
      target: sessionState.session.target,
    };
  }

  function bumpEditableFactoryDefinitionVersion(sessionID) {
    const sessionState = sessionStateForRequest(sessionID);
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
          sessions: sessionRegistry.sessions.map((session) =>
            buildFactorySessionDocument(session.id),
          ),
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

    const factorySessionReadMatch =
      request.method === "GET"
        ? request.url?.match(factorySessionReadPathPattern)
        : null;
    if (factorySessionReadMatch) {
      const sessionID = decodeURIComponent(factorySessionReadMatch[1]);
      const sessionDocument = buildFactorySessionDocument(sessionID);
      if (!sessionDocument) {
        response.writeHead(404, {
          "Access-Control-Allow-Origin": "*",
          "Content-Type": "application/json",
        });
        response.end(
          JSON.stringify({
            code: "FACTORY_SESSION_NOT_FOUND",
            message: `Factory session ${sessionID} was not found.`,
          }),
        );
        return;
      }

      response.writeHead(200, {
        "Access-Control-Allow-Origin": "*",
        "Content-Type": "application/json",
      });
      response.end(JSON.stringify(sessionDocument));
      return;
    }

    const sessionFactoryMatch = request.url?.match(sessionFactoryPathPattern);
    if (sessionFactoryMatch && request.method === "GET") {
      const sessionID = decodeURIComponent(sessionFactoryMatch[1]);
      const sessionState = sessionStateForRequest(sessionID);
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

    const sessionSyncPreflightMatch = request.url?.match(
      sessionSyncPreflightPathPattern,
    );
    if (sessionSyncPreflightMatch && request.method === "GET") {
      const sessionID = decodeURIComponent(sessionSyncPreflightMatch[1]);
      const sessionState = sessionStateForRequest(sessionID);

      response.writeHead(200, {
        "Access-Control-Allow-Origin": "*",
        "Content-Type": "application/json",
      });
      response.end(
        JSON.stringify(
          buildSessionSyncPreflightResponse(request, sessionState, sessionID),
        ),
      );
      return;
    }

    if (sessionFactoryMatch && request.method === "PUT") {
      const sessionID = decodeURIComponent(sessionFactoryMatch[1]);
      const sessionState = sessionStateForRequest(sessionID);
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
      const sessionState = sessionStateForRequest(sessionID);
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

  await waitForPortAvailable(previewHost, apiPort);

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
