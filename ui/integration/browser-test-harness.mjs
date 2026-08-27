// @vitest-environment node

import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { existsSync, readdirSync } from "node:fs";
import {
  mkdir,
  mkdtemp,
  readFile,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import http from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";
import { performance } from "node:perf_hooks";
import process from "node:process";
import readline from "node:readline";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

import { chromium } from "playwright";

import { runSharedBrowserBuild } from "./browser-build-lock.mjs";

const dirname = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(dirname, "..");
const replayFixtureDirectory = path.join(dirname, "fixtures");

const realBackendHarnessDiagnosticPrefix = "[real-backend-browser-harness]";
const realBackendHarnessProcessStartedMarker =
  "[browser-api-harness] phase=process-started";
const realBackendHarnessDiagnosticLimit = 8_192;
const realBackendHarnessDiagnosticTruncationMarker =
  "[earlier diagnostics truncated]";

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

function formatDurableCheckpointThrownOutcome(error) {
  if (error instanceof Error) {
    return `${error.name}: ${error.message}`;
  }

  return String(error);
}

function normalizeDurableCheckpointConditions(conditionFn) {
  if (typeof conditionFn === "function") {
    return null;
  }

  if (
    conditionFn === null ||
    typeof conditionFn !== "object" ||
    Array.isArray(conditionFn)
  ) {
    throw new TypeError(
      "Durable checkpoint conditions must be a function or a named condition object",
    );
  }

  const namedConditions = Object.entries(conditionFn);
  if (namedConditions.length === 0) {
    throw new TypeError("Durable checkpoint named conditions cannot be empty");
  }

  for (const [name, condition] of namedConditions) {
    if (typeof condition !== "function") {
      throw new TypeError(
        `Durable checkpoint condition must be a function: ${name}`,
      );
    }
  }

  return namedConditions;
}

async function evaluateNamedDurableCheckpointConditions(namedConditions) {
  return Promise.all(
    namedConditions.map(async ([name, condition]) => {
      try {
        return {
          name,
          ready: Boolean(await condition()),
          outcome: "returned false",
        };
      } catch (error) {
        return {
          error: formatDurableCheckpointThrownOutcome(error),
          name,
          outcome: "threw",
          ready: false,
        };
      }
    }),
  );
}

function formatNamedDurableCheckpointTimeout(label, outcomes) {
  const unsatisfiedConditions = outcomes
    .filter(({ ready }) => !ready)
    .map(({ error, name, outcome }) => {
      if (outcome === "threw") {
        return `${name} (threw: ${error})`;
      }

      return `${name} (returned false)`;
    });

  return `Timed out waiting for durable checkpoint: ${label}; unsatisfied conditions: ${unsatisfiedConditions.join(", ")}`;
}

/**
 * Poll until a durable checkpoint becomes true (API request captured, download
 * hook populated, dialog closed). Prefer this over asserting transient status
 * copy, animation frames, or heading visibility during teardown.
 *
 * Existing callers can pass one synchronous or asynchronous boolean function.
 * Named checkpoints can pass an object whose keys describe synchronous or
 * asynchronous sub-conditions. Named sub-condition failures are retained for
 * the next poll and included in the timeout diagnostic.
 */
export async function waitForDurableCheckpoint(
  label,
  conditionFn,
  timeoutMs = uiInteractionTimeoutMs,
  intervalMs = 100,
) {
  const namedConditions = normalizeDurableCheckpointConditions(conditionFn);
  let lastNamedOutcomes = null;
  const deadline = Date.now() + timeoutMs;

  while (Date.now() < deadline) {
    if (namedConditions === null) {
      if (await conditionFn()) {
        return;
      }
    } else {
      lastNamedOutcomes =
        await evaluateNamedDurableCheckpointConditions(namedConditions);
      if (lastNamedOutcomes.every(({ ready }) => ready)) {
        return;
      }
    }
    await delay(intervalMs);
  }

  if (namedConditions !== null) {
    throw new Error(
      formatNamedDurableCheckpointTimeout(label, lastNamedOutcomes ?? []),
    );
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

function boundingBoxesEqual(left, right) {
  return (
    left !== null &&
    right !== null &&
    left.x === right.x &&
    left.y === right.y &&
    left.width === right.width &&
    left.height === right.height
  );
}

/** Wait until a browser element reports the same geometry on two observations. */
export async function waitForStableBoundingBox(
  locator,
  timeoutMs = uiInteractionTimeoutMs,
  intervalMs = 100,
) {
  let previousBox = null;
  let stableBox = null;

  await waitForDurableCheckpoint(
    "stable bounding box",
    async () => {
      const nextBox = await locator.boundingBox().catch(() => null);
      const stable = boundingBoxesEqual(previousBox, nextBox);
      previousBox = nextBox;

      if (!stable) {
        return false;
      }

      stableBox = nextBox;
      return true;
    },
    timeoutMs,
    intervalMs,
  );

  return stableBox;
}

/** Wait until React Flow has committed the same viewport transform twice. */
export async function waitForStableFactoryGraphViewport(
  page,
  timeoutMs = uiInteractionTimeoutMs,
  intervalMs = 100,
) {
  const flowViewport = page.locator(
    "[data-current-activity-flow] .react-flow__viewport",
  );
  let previousTransform = null;
  let stableTransform = null;

  await waitForDurableCheckpoint(
    "factory graph viewport settlement",
    async () => {
      const nextTransform = await flowViewport
        .evaluate((element) => window.getComputedStyle(element).transform)
        .catch(() => null);
      const stable =
        nextTransform !== null && nextTransform === previousTransform;
      previousTransform = nextTransform;

      if (!stable) {
        return false;
      }

      stableTransform = nextTransform;
      return true;
    },
    timeoutMs,
    intervalMs,
  );

  return stableTransform;
}

/** Read the React Flow position encoded by a graph node's DOM transform. */
export async function readFactoryGraphNodeFlowPosition(nodeLocator) {
  return nodeLocator
    .evaluate((element) => {
      const flowNode =
        element.classList.contains("react-flow__node") === true
          ? element
          : element.closest(".react-flow__node");
      if (!flowNode) {
        return null;
      }

      const transform =
        flowNode.style.transform || window.getComputedStyle(flowNode).transform;
      if (!transform || transform === "none") {
        return null;
      }

      const translateMatch =
        /translate(?:3d)?\(([-\d.]+)px,\s*([-\d.]+)px/.exec(transform);
      if (translateMatch) {
        return {
          x: Number(translateMatch[1]),
          y: Number(translateMatch[2]),
        };
      }

      const matrixMatch = /matrix\(([^)]+)\)/.exec(transform);
      if (matrixMatch) {
        const values = matrixMatch[1]
          .split(",")
          .map((value) => Number.parseFloat(value.trim()));
        if (values.length >= 6) {
          return { x: values[4], y: values[5] };
        }
      }

      return null;
    })
    .catch(() => null);
}

export function flowPositionDistance(left, right) {
  if (!left || !right) {
    return null;
  }

  return Math.max(Math.abs(left.x - right.x), Math.abs(left.y - right.y));
}

async function draggableFactoryGraphNodeStartPoint(nodeLocator, nodeLabel) {
  const point = await nodeLocator.evaluate((element) => {
    const flowNode =
      element.classList.contains("react-flow__node") === true
        ? element
        : element.closest(".react-flow__node");
    if (!flowNode) {
      return null;
    }

    const flowNodeBox = flowNode.getBoundingClientRect();
    const elementBox = element.getBoundingClientRect();
    const candidates = [
      { x: flowNodeBox.right - 4, y: flowNodeBox.top + flowNodeBox.height / 2 },
      { x: flowNodeBox.left + 4, y: flowNodeBox.top + flowNodeBox.height / 2 },
      {
        x: flowNodeBox.left + flowNodeBox.width / 2,
        y: flowNodeBox.top + 4,
      },
      {
        x: flowNodeBox.left + flowNodeBox.width / 2,
        y: flowNodeBox.bottom - 4,
      },
      { x: elementBox.right - 4, y: elementBox.top + elementBox.height / 2 },
    ];

    for (const candidate of candidates) {
      const hit = document.elementFromPoint(candidate.x, candidate.y);
      if (hit && flowNode.contains(hit) && !hit.closest(".nodrag")) {
        return candidate;
      }
    }

    return null;
  });

  if (!point) {
    throw new Error(
      `Expected a draggable surface inside ${nodeLabel}; all candidate points were nodrag controls.`,
    );
  }

  return point;
}

const defaultDragAttemptCount = 2;
const defaultDragSettleDelayMs = 50;

async function dragFactoryGraphNodeAttempt(
  page,
  nodeLocator,
  deltaX,
  deltaY,
  nodeLabel,
  steps,
  settleDelayMs,
) {
  const startPoint = await draggableFactoryGraphNodeStartPoint(
    nodeLocator,
    nodeLabel,
  );
  let pointerGestureStarted = false;
  let midDragFlowPosition = null;

  try {
    await page.waitForTimeout(settleDelayMs);
    await page.mouse.move(startPoint.x, startPoint.y);
    pointerGestureStarted = true;
    await page.mouse.down();
    await page.waitForTimeout(settleDelayMs);
    await page.mouse.move(startPoint.x + deltaX, startPoint.y + deltaY, {
      steps,
    });
    await page.waitForTimeout(settleDelayMs);
    midDragFlowPosition = await readFactoryGraphNodeFlowPosition(nodeLocator);
  } finally {
    if (pointerGestureStarted) {
      await page.mouse.up().catch(() => {});
    }
  }

  await page.waitForTimeout(settleDelayMs);
  const postMouseUpFlowPosition =
    await readFactoryGraphNodeFlowPosition(nodeLocator);

  return { midDragFlowPosition, postMouseUpFlowPosition };
}

/**
 * Drag a graph node and verify that React Flow moved it during and after the
 * pointer gesture. The tolerance is caller-provided because browser checks
 * intentionally use different movement contracts. A failed displacement gets
 * one bounded settle/retry before the measured failure is reported.
 */
export async function dragNodeByOffset(
  page,
  nodeLocator,
  deltaX,
  deltaY,
  {
    displacementTolerancePx,
    maxAttempts = defaultDragAttemptCount,
    nodeLabel = "graph node",
    settleDelayMs = defaultDragSettleDelayMs,
    steps = 16,
  } = {},
) {
  if (!Number.isFinite(displacementTolerancePx)) {
    throw new TypeError(
      `A finite displacement tolerance is required to drag ${nodeLabel}.`,
    );
  }
  if (!Number.isInteger(maxAttempts) || maxAttempts < 1) {
    throw new TypeError(
      `A positive integer attempt count is required to drag ${nodeLabel}.`,
    );
  }
  if (!Number.isFinite(settleDelayMs) || settleDelayMs < 0) {
    throw new TypeError(
      `A non-negative settle delay is required to drag ${nodeLabel}.`,
    );
  }

  await nodeLocator.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await page.waitForTimeout(settleDelayMs);
  const initialFlowPosition =
    await readFactoryGraphNodeFlowPosition(nodeLocator);

  let latestObservation = {
    midDragDistancePx: null,
    midDragFlowPosition: null,
    postMouseUpDistancePx: null,
    postMouseUpFlowPosition: null,
  };
  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    await nodeLocator.waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
    const { midDragFlowPosition, postMouseUpFlowPosition } =
      await dragFactoryGraphNodeAttempt(
        page,
        nodeLocator,
        deltaX,
        deltaY,
        nodeLabel,
        steps,
        settleDelayMs,
      );
    const midDragDistancePx = flowPositionDistance(
      initialFlowPosition,
      midDragFlowPosition,
    );
    const postMouseUpDistancePx = flowPositionDistance(
      initialFlowPosition,
      postMouseUpFlowPosition,
    );
    latestObservation = {
      midDragDistancePx,
      midDragFlowPosition,
      postMouseUpDistancePx,
      postMouseUpFlowPosition,
    };

    if (
      midDragDistancePx !== null &&
      midDragDistancePx > displacementTolerancePx &&
      postMouseUpDistancePx !== null &&
      postMouseUpDistancePx > displacementTolerancePx
    ) {
      return {
        initialFlowPosition,
        ...latestObservation,
      };
    }
  }

  throw new Error(
    `Mouse drag did not produce the required flow displacement: ${JSON.stringify(
      {
        attempts: maxAttempts,
        initialFlowPosition,
        ...latestObservation,
      },
    )}`,
  );
}

function graphNodePlacementSamplesEqual(left, right) {
  if (!left || !right) {
    return false;
  }

  return (
    left.nodeTransform === right.nodeTransform &&
    left.viewportTransform === right.viewportTransform &&
    left.node.x === right.node.x &&
    left.node.y === right.node.y &&
    left.node.width === right.node.width &&
    left.node.height === right.node.height &&
    left.viewport.x === right.viewport.x &&
    left.viewport.y === right.viewport.y &&
    left.viewport.width === right.viewport.width &&
    left.viewport.height === right.viewport.height
  );
}

const settledGraphNodePlacementSampleCount = 3;

function formatGraphNodePlacementNumber(value) {
  return Number.isFinite(value) ? value.toFixed(2) : String(value);
}

function graphNodePlacementViewportViolation(sample) {
  if (!sample) {
    return null;
  }

  const nodeCenter = {
    x: sample.node.x + sample.node.width / 2,
    y: sample.node.y + sample.node.height / 2,
  };
  const viewport = {
    bottom: sample.viewport.y + sample.viewport.height,
    left: sample.viewport.x,
    right: sample.viewport.x + sample.viewport.width,
    top: sample.viewport.y,
  };
  const violations = [];

  if (nodeCenter.x < viewport.left) {
    violations.push({
      axis: "x",
      boundary: "left edge",
      direction: "left of",
      overshootPx: viewport.left - nodeCenter.x,
    });
  }
  if (nodeCenter.x > viewport.right) {
    violations.push({
      axis: "x",
      boundary: "right edge",
      direction: "right of",
      overshootPx: nodeCenter.x - viewport.right,
    });
  }
  if (nodeCenter.y < viewport.top) {
    violations.push({
      axis: "y",
      boundary: "top edge",
      direction: "above",
      overshootPx: viewport.top - nodeCenter.y,
    });
  }
  if (nodeCenter.y > viewport.bottom) {
    violations.push({
      axis: "y",
      boundary: "bottom edge",
      direction: "below",
      overshootPx: nodeCenter.y - viewport.bottom,
    });
  }

  if (violations.length === 0) {
    return null;
  }

  return {
    nodeCenter,
    viewport,
    violations,
  };
}

function graphNodePlacementIsWithinViewport(sample) {
  return graphNodePlacementViewportViolation(sample) === null;
}

function formatGraphNodePlacementViewportViolation(
  nodeTestId,
  pollCount,
  stableSampleCount,
  violation,
) {
  const outOfRangeAxes = violation.violations
    .map(
      ({ axis, boundary, direction, overshootPx }) =>
        `${axis} ${direction} ${boundary} by ${formatGraphNodePlacementNumber(overshootPx)}px`,
    )
    .join("; ");

  return [
    `Settled graph node placement cannot satisfy viewport visibility for ${nodeTestId}`,
    `after ${stableSampleCount} consecutive byte-identical samples at poll ${pollCount}`,
    `node center=(${formatGraphNodePlacementNumber(violation.nodeCenter.x)}, ${formatGraphNodePlacementNumber(violation.nodeCenter.y)})`,
    `viewport edges={left=${formatGraphNodePlacementNumber(violation.viewport.left)}, right=${formatGraphNodePlacementNumber(violation.viewport.right)}, top=${formatGraphNodePlacementNumber(violation.viewport.top)}, bottom=${formatGraphNodePlacementNumber(violation.viewport.bottom)}}`,
    `out-of-range axes: ${outOfRangeAxes}`,
  ].join("; ");
}

/**
 * Wait until a graph node's DOM geometry and both React Flow transforms settle.
 * Viewport visibility is required by default, but callers that are proving a
 * saved flow position can opt into geometry-only settlement when camera
 * framing is an independent shared-layout concern.
 */
export async function waitForStableFactoryGraphNodePlacement(
  page,
  nodeTestId,
  timeoutMs = uiInteractionTimeoutMs,
  intervalMs = 100,
  onObservation,
  { requireViewportVisibility = true } = {},
) {
  let previousSample = null;
  let stableSample = null;
  let pollCount = 0;
  let stableSampleCount = 0;

  await waitForDurableCheckpoint(
    `factory graph node placement: ${nodeTestId}`,
    async () => {
      pollCount += 1;
      const nextSample = await page
        .evaluate((testId) => {
          const target = [...document.querySelectorAll("[data-testid]")].find(
            (element) => element.getAttribute("data-testid") === testId,
          );
          const flowNode = target?.closest(".react-flow__node");
          const graphSurface = target?.closest("[data-current-activity-flow]");
          const flowViewport = flowNode
            ?.closest(".react-flow")
            ?.querySelector(".react-flow__viewport");

          if (!target || !flowNode || !graphSurface || !flowViewport) {
            return null;
          }

          const nodeBox = target.getBoundingClientRect();
          const viewportBox = graphSurface.getBoundingClientRect();
          return {
            node: {
              height: nodeBox.height,
              width: nodeBox.width,
              x: nodeBox.x,
              y: nodeBox.y,
            },
            nodeTransform: window.getComputedStyle(flowNode).transform,
            viewport: {
              height: viewportBox.height,
              width: viewportBox.width,
              x: viewportBox.x,
              y: viewportBox.y,
            },
            viewportTransform: window.getComputedStyle(flowViewport).transform,
          };
        }, nodeTestId)
        .catch(() => null);
      const stable = graphNodePlacementSamplesEqual(previousSample, nextSample);
      stableSampleCount = stable ? stableSampleCount + 1 : nextSample ? 1 : 0;
      const viewportViolation = graphNodePlacementViewportViolation(nextSample);
      const withinViewport = Boolean(
        nextSample && graphNodePlacementIsWithinViewport(nextSample),
      );
      const terminalDiagnostic =
        requireViewportVisibility &&
        stableSampleCount >= settledGraphNodePlacementSampleCount &&
        viewportViolation
          ? formatGraphNodePlacementViewportViolation(
              nodeTestId,
              pollCount,
              stableSampleCount,
              viewportViolation,
            )
          : null;
      onObservation?.({
        nextSample,
        pollCount,
        stable,
        stableSampleCount,
        terminalDiagnostic,
        viewportViolation,
        withinViewport,
      });
      previousSample = nextSample;

      if (terminalDiagnostic) {
        throw new Error(terminalDiagnostic);
      }

      if (
        !stable ||
        !nextSample ||
        (requireViewportVisibility && !withinViewport)
      ) {
        return false;
      }

      stableSample = nextSample;
      return true;
    },
    timeoutMs,
    intervalMs,
  );

  return stableSample;
}

/** Wait until React Flow has measured every graph node for selection gestures. */
export async function waitForFactoryGraphSelectionReady(
  page,
  timeoutMs = uiInteractionTimeoutMs,
) {
  const readinessMarker = page.locator(
    '[data-factory-graph-selection-ready="true"]',
  );

  await waitForDurableCheckpoint(
    "factory graph selection readiness",
    async () => (await readinessMarker.count()) > 0,
    timeoutMs,
  );
}

/** Wait for the graph selection projection and its enabled batch-delete action. */
export async function waitForFactoryGraphSelectionDeleteButton(
  toolbar,
  timeoutMs = uiInteractionTimeoutMs,
) {
  const selectedGraphSelection = toolbar.locator(
    '[data-toolbar-graph-selection="single"], [data-toolbar-graph-selection="multi"]',
  );
  const batchDeleteButton = toolbar.getByRole("button", {
    name: /^Delete (?:\d+ )?selected graph items?$/,
  });

  await waitForDurableCheckpoint(
    "factory graph selection delete control",
    {
      "selection projection": async () =>
        await selectedGraphSelection.isVisible(),
      "delete control visibility": async () =>
        await batchDeleteButton.isVisible(),
      "delete control enabled": async () => await batchDeleteButton.isEnabled(),
    },
    timeoutMs,
  );

  return batchDeleteButton;
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

function writeRealBackendHarnessDiagnostic(writer, message) {
  const line = message.startsWith(realBackendHarnessDiagnosticPrefix)
    ? `${message}\n`
    : `${realBackendHarnessDiagnosticPrefix} ${message}\n`;
  if (typeof writer === "function") {
    writer(line);
    return;
  }
  writer?.write?.(line);
}

function formatRealBackendHarnessDuration(elapsedMs) {
  return Number.isFinite(elapsedMs)
    ? `${elapsedMs.toFixed(1)}ms`
    : "unavailable";
}

function cacheDirectoryReuseState(cachePath, name) {
  if (typeof cachePath !== "string" || cachePath.length === 0) {
    return "not-overridden";
  }

  try {
    if (!existsSync(cachePath)) {
      return "missing";
    }

    const entries = readdirSync(cachePath);
    if (name === "GOPATH") {
      return "available";
    }
    if (name === "GOMODCACHE") {
      const moduleEntries = entries.filter((entry) => entry !== "cache");
      if (moduleEntries.length === 0) {
        return "empty";
      }
      if (moduleEntries.length === 1 && moduleEntries[0] === "golang.org") {
        const goModuleEntries = readdirSync(path.join(cachePath, "golang.org"));
        if (
          goModuleEntries.length === 1 &&
          goModuleEntries[0].startsWith("toolchain@")
        ) {
          return "toolchain-only";
        }
      }
      return "populated";
    }
    return entries.length > 0 ? "populated" : "empty";
  } catch {
    return "unreadable";
  }
}

function formatGoCacheReuse(cacheReuse) {
  return ["GOCACHE", "GOMODCACHE", "GOPATH"]
    .map((name) => `${name}:${cacheReuse?.[name] ?? "unavailable"}`)
    .join(",");
}

export function createRealBackendHarnessStartupTiming(
  startedAt = performance.now(),
) {
  return {
    applicationStartupMs: null,
    cacheResolutionMs: null,
    cacheReuse: null,
    cacheReuseBeforeResolution: null,
    firstReadyPayloadMs: null,
    harnessCompilationSetupMs: null,
    processLaunchMs: null,
    processSpawnedAt: null,
    processStartedAt: null,
    startedAt,
    totalStartupMs: null,
  };
}

function publicRealBackendHarnessStartupTiming(timing) {
  return {
    applicationStartupMs: timing.applicationStartupMs,
    cacheResolutionMs: timing.cacheResolutionMs,
    cacheReuse: timing.cacheReuse,
    cacheReuseBeforeResolution: timing.cacheReuseBeforeResolution,
    firstReadyPayloadMs: timing.firstReadyPayloadMs,
    harnessCompilationSetupMs: timing.harnessCompilationSetupMs,
    processLaunchMs: timing.processLaunchMs,
    totalStartupMs: timing.totalStartupMs,
  };
}

export function formatRealBackendHarnessStartupTiming(timing) {
  return [
    `${realBackendHarnessDiagnosticPrefix} timing`,
    `cache-resolution=${formatRealBackendHarnessDuration(timing?.cacheResolutionMs)}`,
    `harness-compilation-setup=${formatRealBackendHarnessDuration(timing?.harnessCompilationSetupMs)}`,
    `process-launch=${formatRealBackendHarnessDuration(timing?.processLaunchMs)}`,
    `application-startup=${formatRealBackendHarnessDuration(timing?.applicationStartupMs)}`,
    `first-ready-payload=${formatRealBackendHarnessDuration(timing?.firstReadyPayloadMs)}`,
    `total=${formatRealBackendHarnessDuration(timing?.totalStartupMs)}`,
    `cache-reuse-before=${formatGoCacheReuse(timing?.cacheReuseBeforeResolution)}`,
    `cache-reuse-after=${formatGoCacheReuse(timing?.cacheReuse)}`,
  ].join(" ");
}

function sanitizeRealBackendHarnessDiagnostic(text, secrets = []) {
  let safeText = String(text ?? "");
  for (const secret of secrets) {
    if (typeof secret === "string" && secret.length > 0) {
      safeText = safeText.split(secret).join("<redacted>");
    }
  }

  safeText = safeText
    .replace(
      /(?:[A-Za-z]:[\\/]|\/(?:Users|home|runner|tmp|private|var|workspace|workspaces)\/)[^\s"'`]+/g,
      "<path>",
    )
    .replace(
      /\b(authorization|api[-_]?key|password|secret|token)\s*[:=]\s*\S+/gi,
      "$1=<redacted>",
    )
    .replace(
      /\b(payload|body|request|response|content|prompt|output)\s*[:=]\s*.+$/gi,
      "$1=<redacted>",
    );

  if (safeText.length <= realBackendHarnessDiagnosticLimit) {
    return safeText;
  }

  const suffixLimit =
    realBackendHarnessDiagnosticLimit -
    realBackendHarnessDiagnosticTruncationMarker.length -
    1;
  return `${realBackendHarnessDiagnosticTruncationMarker}\n${safeText.slice(-suffixLimit)}`;
}

function appendBoundedRealBackendHarnessDiagnostic(previous, next) {
  const combined = `${previous}${next}`;
  if (combined.length <= realBackendHarnessDiagnosticLimit) {
    return combined;
  }

  const suffixLimit =
    realBackendHarnessDiagnosticLimit -
    realBackendHarnessDiagnosticTruncationMarker.length -
    1;
  return `${realBackendHarnessDiagnosticTruncationMarker}\n${combined.slice(-suffixLimit)}`;
}

function startupPhaseFromDiagnosticLine(line) {
  const normalizedLine = line.trim();
  if (normalizedLine === realBackendHarnessProcessStartedMarker) {
    return "process-started";
  }

  const match = /^\[browser-api-harness\] phase=([a-z-]+)$/.exec(
    normalizedLine,
  );
  return match?.[1] ?? null;
}

function currentRealBackendHarnessStartupPhase(timing) {
  if (timing?.firstReadyPayloadMs !== null) {
    return "ready-payload";
  }
  if (timing?.processStartedAt === null) {
    return "harness-compilation/setup";
  }
  return "application-startup";
}

function recordRealBackendHarnessStartupPhase(timing, phase, now, writer) {
  if (phase !== "process-started" || timing.processStartedAt !== null) {
    return;
  }

  timing.processStartedAt = now;
  timing.harnessCompilationSetupMs = Math.max(
    0,
    now - (timing.processSpawnedAt ?? timing.startedAt),
  );
  writeRealBackendHarnessDiagnostic(
    writer,
    `phase=harness-process-started elapsed=${formatRealBackendHarnessDuration(timing.harnessCompilationSetupMs)}`,
  );
}

function observeRealBackendHarnessStderr({
  child,
  diagnosticWriter,
  now,
  secrets,
  timing,
}) {
  let captured = "";
  let pendingLine = "";

  const observeLine = (line) => {
    captured = appendBoundedRealBackendHarnessDiagnostic(captured, `${line}\n`);
    const safeLine = sanitizeRealBackendHarnessDiagnostic(line, secrets).trim();
    if (safeLine.length > 0) {
      writeRealBackendHarnessDiagnostic(
        diagnosticWriter,
        `child-stderr ${safeLine}`,
      );
    }

    const phase = startupPhaseFromDiagnosticLine(line);
    if (phase) {
      recordRealBackendHarnessStartupPhase(
        timing,
        phase,
        now(),
        diagnosticWriter,
      );
    }
  };

  const observeChunk = (chunk) => {
    const text = chunk.toString();
    pendingLine += text;
    const lines = pendingLine.split(/\r?\n/);
    pendingLine = lines.pop() ?? "";
    for (const line of lines) {
      observeLine(line);
    }
  };

  child.stderr?.on("data", observeChunk);
  child.stderr?.on("end", () => {
    if (pendingLine.length > 0) {
      observeLine(pendingLine);
      pendingLine = "";
    }
  });

  return () => {
    if (pendingLine.length > 0) {
      observeLine(pendingLine);
      pendingLine = "";
    }
    return captured;
  };
}

function annotateRealBackendHarnessReadinessError(
  error,
  timing,
  getCapturedStderr,
  secrets,
) {
  const safeErrorMessage = sanitizeRealBackendHarnessDiagnostic(
    error.message,
    secrets,
  ).trim();
  const capturedStderr = sanitizeRealBackendHarnessDiagnostic(
    getCapturedStderr(),
    secrets,
  ).trim();
  return new Error(
    [
      safeErrorMessage,
      `phase=${currentRealBackendHarnessStartupPhase(timing)}`,
      formatRealBackendHarnessStartupTiming(timing),
      capturedStderr.length > 0 ? `captured-stderr=${capturedStderr}` : null,
    ]
      .filter(Boolean)
      .join("\n"),
  );
}

export function waitForRealBackendHarnessReadiness({
  child,
  diagnosticWriter = process.stderr,
  getCapturedStderr = () => "",
  lineReader,
  now = () => performance.now(),
  secrets = [],
  timeoutMs = readyTimeoutMs,
  timing,
} = {}) {
  if (!child || !lineReader || !timing) {
    throw new TypeError(
      "waitForRealBackendHarnessReadiness requires child, lineReader, and timing.",
    );
  }

  return new Promise((resolve, reject) => {
    let settled = false;
    const timeout = setTimeout(() => {
      settleFailure(
        new Error(
          "Timed out waiting for real backend browser harness readiness.",
        ),
      );
    }, timeoutMs);

    function cleanupListeners() {
      clearTimeout(timeout);
      child.off("exit", rejectWithProcessExit);
      child.off("error", rejectWithProcessError);
      lineReader.off("line", resolveReadyLine);
    }

    function settleFailure(error) {
      if (settled) {
        return;
      }
      settled = true;
      cleanupListeners();
      reject(
        annotateRealBackendHarnessReadinessError(
          error,
          timing,
          getCapturedStderr,
          secrets,
        ),
      );
    }

    function rejectWithProcessExit(code, signal) {
      settleFailure(
        new Error(
          `Real backend browser harness exited before readiness: code=${code ?? "null"} signal=${signal ?? "null"}`,
        ),
      );
    }

    function rejectWithProcessError(error) {
      settleFailure(
        new Error(`Real backend browser harness process error: ${error}`),
      );
    }

    function resolveReadyLine(line) {
      let payload;
      try {
        payload = JSON.parse(line);
      } catch (error) {
        settleFailure(
          new Error(
            `Failed to parse real backend browser harness ready payload (${error.message}); received stdout line length=${line.length}`,
          ),
        );
        return;
      }

      if (
        !payload ||
        typeof payload !== "object" ||
        typeof payload.apiOrigin !== "string" ||
        typeof payload.sessionId !== "string"
      ) {
        settleFailure(
          new Error(
            "Real backend browser harness ready payload did not contain apiOrigin and sessionId strings.",
          ),
        );
        return;
      }

      const readyAt = now();
      timing.firstReadyPayloadMs = Math.max(
        0,
        readyAt - (timing.processSpawnedAt ?? timing.startedAt),
      );
      timing.applicationStartupMs =
        timing.processStartedAt === null
          ? null
          : Math.max(0, readyAt - timing.processStartedAt);
      timing.totalStartupMs = Math.max(0, readyAt - timing.startedAt);
      writeRealBackendHarnessDiagnostic(
        diagnosticWriter,
        formatRealBackendHarnessStartupTiming(timing),
      );
      if (settled) {
        return;
      }
      settled = true;
      cleanupListeners();
      resolve(payload);
    }

    child.once("exit", rejectWithProcessExit);
    child.once("error", rejectWithProcessError);
    lineReader.once("line", resolveReadyLine);
  });
}

function resolveGoCacheEnvironment({
  diagnosticWriter = process.stderr,
  now = () => performance.now(),
} = {}) {
  const startedAt = now();
  writeRealBackendHarnessDiagnostic(
    diagnosticWriter,
    "phase=cache-resolution started",
  );
  const names = ["GOCACHE", "GOMODCACHE", "GOPATH"];
  const cacheReuseBeforeResolution = Object.fromEntries(
    names.map((name) => [
      name,
      cacheDirectoryReuseState(process.env[name], name),
    ]),
  );
  const result = spawnSync("go", ["env", "-json", ...names], {
    encoding: "utf8",
    shell: false,
  });
  if (result.status !== 0) {
    throw new Error(
      `Failed to resolve Go cache paths for real backend browser harness: ${sanitizeRealBackendHarnessDiagnostic(result.stderr)}`,
    );
  }
  const values = JSON.parse(result.stdout);
  const environment = Object.fromEntries(
    names
      .filter((name) => typeof values[name] === "string" && values[name] !== "")
      .map((name) => [name, values[name]]),
  );
  const cacheReuse = Object.fromEntries(
    names.map((name) => [
      name,
      cacheDirectoryReuseState(environment[name], name),
    ]),
  );
  const elapsedMs = Math.max(0, now() - startedAt);
  writeRealBackendHarnessDiagnostic(
    diagnosticWriter,
    `phase=cache-resolution complete elapsed=${formatRealBackendHarnessDuration(elapsedMs)} cache-reuse-before=${formatGoCacheReuse(cacheReuseBeforeResolution)} cache-reuse-after=${formatGoCacheReuse(cacheReuse)}`,
  );
  return {
    cacheReuse,
    cacheReuseBeforeResolution,
    elapsedMs,
    environment,
  };
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
  diagnosticWriter = process.stderr,
  factoryDir = path.resolve(packageRoot, "..", "factory"),
  now = () => performance.now(),
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

  const timing = createRealBackendHarnessStartupTiming(now());
  const goCache = resolveGoCacheEnvironment({ diagnosticWriter, now });
  timing.cacheResolutionMs = goCache.elapsedMs;
  timing.cacheReuse = goCache.cacheReuse;
  timing.cacheReuseBeforeResolution = goCache.cacheReuseBeforeResolution;
  writeRealBackendHarnessDiagnostic(
    diagnosticWriter,
    "phase=temporary-home-setup started",
  );
  const temporaryHomeStartedAt = now();
  const customerHome = await mkdtemp(
    path.join(tmpdir(), "you-browser-backend-"),
  );
  writeRealBackendHarnessDiagnostic(
    diagnosticWriter,
    `phase=temporary-home-setup complete elapsed=${formatRealBackendHarnessDuration(Math.max(0, now() - temporaryHomeStartedAt))}`,
  );
  let child;
  try {
    writeRealBackendHarnessDiagnostic(
      diagnosticWriter,
      "phase=process-launch started",
    );
    const processLaunchStartedAt = now();
    child = spawnRepoProcess(
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
          ...goCache.environment,
          HOME: customerHome,
          USERPROFILE: customerHome,
        },
      },
    );
    child.once("spawn", () => {
      timing.processSpawnedAt = now();
      timing.processLaunchMs = Math.max(
        0,
        timing.processSpawnedAt - processLaunchStartedAt,
      );
      writeRealBackendHarnessDiagnostic(
        diagnosticWriter,
        `phase=process-launch complete elapsed=${formatRealBackendHarnessDuration(timing.processLaunchMs)}`,
      );
    });
  } catch (error) {
    await rm(customerHome, { force: true, recursive: true });
    throw error;
  }

  const readCapturedStderr = observeRealBackendHarnessStderr({
    child,
    diagnosticWriter,
    now,
    secrets: [
      customerHome,
      factoryDir,
      requestID,
      workflowFixture,
      workflowName,
      ...Object.values(goCache.environment),
    ],
    timing,
  });

  const lineReader = readline.createInterface({
    input: child.stdout,
  });
  async function stopHarness() {
    lineReader.close();
    try {
      await stopProcess(child);
    } finally {
      await rm(customerHome, { force: true, recursive: true });
    }
  }
  const ready = waitForRealBackendHarnessReadiness({
    child,
    diagnosticWriter,
    getCapturedStderr: readCapturedStderr,
    lineReader,
    now,
    secrets: [
      customerHome,
      factoryDir,
      requestID,
      workflowFixture,
      workflowName,
      ...Object.values(goCache.environment),
    ],
    timeoutMs: readyTimeoutMs,
    timing,
  });

  try {
    const payload = await ready;
    return {
      apiOrigin: payload.apiOrigin,
      sessionID: payload.sessionId,
      startupTimings: publicRealBackendHarnessStartupTiming(timing),
      stop: stopHarness,
    };
  } catch (error) {
    await stopHarness();
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

  let stopPromise = null;
  return {
    releaseReplayStream,
    replayCompleted,
    replayPaused,
    requestedEventSessionIDs,
    stop: async () => {
      if (!stopPromise) {
        stopPromise = new Promise((resolve, reject) => {
          server.close((error) => {
            if (error) {
              reject(error);
              return;
            }

            resolve();
          });
          // Stop accepting reconnects before terminating the dashboard's
          // long-lived event-stream sockets.
          server.closeAllConnections?.();
          server.closeIdleConnections?.();
        });
      }
      await stopPromise;
      await waitForPortAvailable(previewHost, apiPort);
    },
  };
}
