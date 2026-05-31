// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  expectNoBrowserErrors,
  fillWorkstationPromptBody,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";

const editableGraphFactoryDefinition = {
  metadata: {
    owner: "operations",
  },
  name: "Current Factory",
  resources: [
    {
      capacity: 2,
      name: "gpu",
    },
  ],
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
        {
          name: "done",
          type: "TERMINAL",
        },
      ],
    },
  ],
  workstations: [
    {
      body: "Draft the story.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "draft",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      resources: [
        {
          capacity: 2,
          name: "gpu",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

const editableGraphFactoryReplayLines = [
  JSON.stringify({
    context: {
      eventTime: "2026-05-19T15:00:00Z",
      sequence: 1,
      tick: 1,
    },
    id: "editable-graph-1",
    payload: {
      factory: {
        resources: [
          {
            capacity: 2,
            name: "gpu",
          },
        ],
        workTypes: [
          {
            name: "story",
            states: [
              {
                name: "queued",
                type: "INITIAL",
              },
              {
                name: "done",
                type: "TERMINAL",
              },
            ],
          },
        ],
        workers: [
          {
            model: "gpt-5",
            name: "writer",
            type: "MODEL_WORKER",
          },
        ],
        workstations: [
          {
            inputs: [
              {
                state: "queued",
                workType: "story",
              },
            ],
            name: "draft",
            outputs: [
              {
                state: "done",
                workType: "story",
              },
            ],
            resources: [
              {
                capacity: 2,
                name: "gpu",
              },
            ],
            worker: "writer",
          },
        ],
      },
    },
    type: "INITIAL_STRUCTURE_REQUEST",
  }),
  JSON.stringify({
    context: {
      eventTime: "2026-05-19T15:00:01Z",
      sequence: 2,
      tick: 2,
    },
    id: "editable-graph-2",
    payload: {
      previousState: "RUNNING",
      reason: "fixture ready",
      state: "FINISHED",
    },
    type: "FACTORY_STATE_RESPONSE",
  }),
];

const viewportCenterToleranceRatio = 0.35;

function distanceBetweenPoints(left, right) {
  const deltaX = left.x - right.x;
  const deltaY = left.y - right.y;
  return Math.hypot(deltaX, deltaY);
}

function boundingBoxesOverlap(left, right) {
  return (
    left.x < right.x + right.width &&
    left.x + left.width > right.x &&
    left.y < right.y + right.height &&
    left.y + left.height > right.y
  );
}

async function enterGraphEditor(page) {
  await page
    .getByRole("button", { name: "Enter factory graph editor" })
    .click();
  const toolbar = page.getByRole("region", {
    name: "Factory graph editor tools",
  });
  await toolbar.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  return toolbar;
}

async function graphViewport(page) {
  return page.getByRole("region", { name: "Work graph viewport" });
}

async function panGraphViewport(page, deltaX, deltaY) {
  const viewport = await graphViewport(page);
  const pane = viewport.locator(".react-flow__pane");
  await pane.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  const box = await pane.boundingBox();
  if (!box) {
    throw new Error("Expected graph pane bounding box.");
  }

  const startX = box.x + box.width * 0.2;
  const startY = box.y + box.height * 0.2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY, { steps: 12 });
  await page.mouse.up();
}

async function viewportScreenMetrics(page) {
  const viewport = await graphViewport(page);
  const box = await viewport.boundingBox();
  if (!box) {
    throw new Error("Expected graph viewport bounding box.");
  }

  return {
    box,
    center: {
      x: box.x + box.width / 2,
      y: box.y + box.height / 2,
    },
  };
}

function isNearViewportCenter(nodeCenter, viewportMetrics) {
  const maxDeltaX = viewportMetrics.box.width * viewportCenterToleranceRatio;
  const maxDeltaY = viewportMetrics.box.height * viewportCenterToleranceRatio;

  return (
    Math.abs(nodeCenter.x - viewportMetrics.center.x) <= maxDeltaX &&
    Math.abs(nodeCenter.y - viewportMetrics.center.y) <= maxDeltaY
  );
}

async function nodeScreenCenter(page, nodeTestId) {
  const node = page.getByTestId(nodeTestId);
  await node.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  const box = await node.boundingBox();
  if (!box) {
    throw new Error(`Expected bounding box for ${nodeTestId}.`);
  }

  return {
    x: box.x + box.width / 2,
    y: box.y + box.height / 2,
  };
}

async function nodeBoundingBox(page, nodeTestId) {
  const node = page.getByTestId(nodeTestId);
  await node.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  const box = await node.boundingBox();
  if (!box) {
    throw new Error(`Expected bounding box for ${nodeTestId}.`);
  }
  return box;
}

async function addWorker(
  page,
  toolbar,
  { model = "gpt-5", modelProvider = "CURSOR", name },
) {
  await toolbar.getByRole("button", { name: "Open add entity menu" }).click();
  await page
    .getByLabel("Add graph entity menu")
    .getByRole("button", { name: "Worker" })
    .click();

  const addDialog = page.getByRole("dialog", { name: "Add worker" });
  await addDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await addDialog.getByLabel("Identifier").fill(name);
  await addDialog
    .getByRole("combobox", { name: "Model provider" })
    .selectOption(modelProvider);
  await addDialog.getByRole("textbox", { name: "Model" }).fill(model);
  await addDialog.getByRole("button", { name: "Add entity" }).click();
}

async function addWorkstation(page, toolbar, { body, name }) {
  await toolbar.getByRole("button", { name: "Open add entity menu" }).click();
  await page
    .getByLabel("Add graph entity menu")
    .getByRole("button", { name: "Workstation" })
    .click();

  const addDialog = page.getByRole("dialog", { name: "Add workstation" });
  await addDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await addDialog.getByLabel("Identifier").fill(name);
  await fillWorkstationPromptBody(addDialog, body);
  await addDialog.getByRole("button", { name: "Add entity" }).click();
}

async function readNodeFlowPosition(page, nodeTestId) {
  return page.evaluate((testId) => {
    const element = document.querySelector(`[data-testid="${testId}"]`);
    const reactFlowNode =
      element?.classList.contains("react-flow__node") === true
        ? element
        : element?.closest(".react-flow__node");
    if (!reactFlowNode) {
      return null;
    }

    const transform =
      reactFlowNode.style.transform ||
      window.getComputedStyle(reactFlowNode).transform;
    if (!transform || transform === "none") {
      return null;
    }

    const translateMatch = /translate(?:3d)?\(([-\d.]+)px,\s*([-\d.]+)px/.exec(
      transform,
    );
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
  }, nodeTestId);
}

async function readNodeScreenPosition(page, nodeTestId) {
  const box = await page.getByTestId(nodeTestId).boundingBox();
  if (!box) {
    return null;
  }

  return { x: box.x, y: box.y };
}

async function addResource(page, toolbar, { capacity = "1", name }) {
  await toolbar.getByRole("button", { name: "Open add entity menu" }).click();
  await page
    .getByLabel("Add graph entity menu")
    .getByRole("button", { name: "Resource" })
    .click();

  const addDialog = page.getByRole("dialog", { name: "Add resource" });
  await addDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await addDialog.getByLabel("Identifier").fill(name);
  await addDialog.getByLabel("Capacity").fill(capacity);
  await addDialog.getByRole("button", { name: "Add entity" }).click();
}

async function dragNodeByOffset(page, nodeTestId, deltaX, deltaY) {
  const node = page.getByTestId(nodeTestId);
  await node.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  const viewport = await graphViewport(page);
  const nodeBox = await node.boundingBox();
  const viewportBox = await viewport.boundingBox();
  if (!nodeBox || !viewportBox) {
    throw new Error(`Expected bounding boxes for ${nodeTestId} drag.`);
  }

  await node.dragTo(viewport, {
    sourcePosition: { x: 20, y: 20 },
    targetPosition: {
      x: nodeBox.x - viewportBox.x + deltaX,
      y: nodeBox.y - viewportBox.y + deltaY,
    },
  });
}

async function saveGraphDraft(page, toolbar) {
  const saveChangesButton = toolbar.getByRole("button", {
    name: "Save changes",
  });
  await expect
    .poll(async () => await saveChangesButton.isEnabled(), {
      timeout: uiInteractionTimeoutMs,
    })
    .toBe(true);

  await saveChangesButton.click();
  const saveDialog = page.getByRole("dialog", {
    name: "Save factory graph changes?",
  });
  await saveDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await saveDialog.getByRole("button", { name: "Save topology" }).click();
  await page
    .getByText("Topology saved", { exact: true })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
}

describe.sequential("factory graph editor node placement browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    if (!preview) {
      return;
    }

    await Promise.race([
      preview.stop(),
      new Promise((resolve) => {
        setTimeout(resolve, 15_000);
      }),
    ]);
    preview = null;
  }, 120_000);

  afterEach(async () => {
    await new Promise((resolve) => setTimeout(resolve, 250));
  });

  it(
    "places a newly added worker near the visible viewport center after panning",
    async () => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: editableGraphFactoryDefinition,
        eventLines: editableGraphFactoryReplayLines,
      });
      const browserPage = await openBrowserPage();

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await server.replayCompleted;

        const toolbar = await enterGraphEditor(browserPage.page);
        await panGraphViewport(browserPage.page, 240, 180);

        const existingDraftCenter = await nodeScreenCenter(
          browserPage.page,
          "rf__node-workstation:draft",
        );
        await addWorker(browserPage.page, toolbar, { name: "assistant" });

        const newWorkerNode = browserPage.page.getByTestId(
          "rf__node-worker:assistant",
        );
        await newWorkerNode.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        const viewportMetrics = await viewportScreenMetrics(browserPage.page);
        const addedWorkerCenter = await nodeScreenCenter(
          browserPage.page,
          "rf__node-worker:assistant",
        );
        const distanceFromExistingDraft = distanceBetweenPoints(
          addedWorkerCenter,
          existingDraftCenter,
        );

        expect(isNearViewportCenter(addedWorkerCenter, viewportMetrics)).toBe(
          true,
        );
        expect(distanceFromExistingDraft).toBeGreaterThan(80);

        const flowPosition = await readNodeFlowPosition(
          browserPage.page,
          "rf__node-worker:assistant",
        );
        expect(flowPosition).not.toBeNull();
        expect(Number.isFinite(flowPosition.x)).toBe(true);
        expect(Number.isFinite(flowPosition.y)).toBe(true);

        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await server.stop();
        await browserPage.close();
      }
    },
    browserScenarioTimeoutMs,
  );

  it(
    "nudges a newly added workstation away from an existing viewport-center worker",
    async () => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: editableGraphFactoryDefinition,
        eventLines: editableGraphFactoryReplayLines,
      });
      const browserPage = await openBrowserPage();

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await server.replayCompleted;

        const toolbar = await enterGraphEditor(browserPage.page);
        await addWorker(browserPage.page, toolbar, { name: "center-anchor" });

        const anchorNode = browserPage.page.getByTestId(
          "rf__node-worker:center-anchor",
        );
        await anchorNode.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        await addWorkstation(browserPage.page, toolbar, {
          body: "Review the drafted story.",
          name: "review",
        });

        const workstationNode = browserPage.page.getByTestId(
          "rf__node-workstation:review",
        );
        await workstationNode.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        const anchorBox = await nodeBoundingBox(
          browserPage.page,
          "rf__node-worker:center-anchor",
        );
        const workstationBox = await nodeBoundingBox(
          browserPage.page,
          "rf__node-workstation:review",
        );

        expect(boundingBoxesOverlap(anchorBox, workstationBox)).toBe(false);

        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await server.stop();
        await browserPage.close();
      }
    },
    browserScenarioTimeoutMs,
  );

  it(
    "keeps a manually dragged node position after reload",
    async () => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: editableGraphFactoryDefinition,
        eventLines: editableGraphFactoryReplayLines,
      });
      const browserPage = await openBrowserPage();

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await server.replayCompleted;

        const toolbar = await enterGraphEditor(browserPage.page);
        await addResource(browserPage.page, toolbar, { name: "extra-gpu" });
        await browserPage.page
          .getByTestId("rf__node-resource:extra-gpu")
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        const initialFlowPosition = await readNodeFlowPosition(
          browserPage.page,
          "rf__node-resource:extra-gpu",
        );
        const initialScreenPosition = await readNodeScreenPosition(
          browserPage.page,
          "rf__node-resource:extra-gpu",
        );
        expect(initialFlowPosition ?? initialScreenPosition).not.toBeNull();

        await dragNodeByOffset(
          browserPage.page,
          "rf__node-resource:extra-gpu",
          160,
          120,
        );

        await expect
          .poll(
            async () => {
              const nextFlowPosition = await readNodeFlowPosition(
                browserPage.page,
                "rf__node-resource:extra-gpu",
              );
              const nextScreenPosition = await readNodeScreenPosition(
                browserPage.page,
                "rf__node-resource:extra-gpu",
              );
              if (!nextScreenPosition || !initialScreenPosition) {
                return null;
              }

              const movedOnScreen =
                Math.abs(nextScreenPosition.x - initialScreenPosition.x) > 8 ||
                Math.abs(nextScreenPosition.y - initialScreenPosition.y) > 8;
              const movedInFlow =
                initialFlowPosition &&
                nextFlowPosition &&
                (Math.abs(nextFlowPosition.x - initialFlowPosition.x) > 8 ||
                  Math.abs(nextFlowPosition.y - initialFlowPosition.y) > 8);

              return movedOnScreen || movedInFlow
                ? { flow: nextFlowPosition, screen: nextScreenPosition }
                : null;
            },
            { timeout: uiInteractionTimeoutMs },
          )
          .not.toBeNull();

        const draggedFlowPosition = await readNodeFlowPosition(
          browserPage.page,
          "rf__node-resource:extra-gpu",
        );

        await saveGraphDraft(browserPage.page, toolbar);

        await browserPage.page.reload({ waitUntil: "domcontentloaded" });
        await server.replayCompleted;
        await enterGraphEditor(browserPage.page);
        await browserPage.page
          .getByTestId("rf__node-resource:extra-gpu")
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        const reloadedFlowPosition = await readNodeFlowPosition(
          browserPage.page,
          "rf__node-resource:extra-gpu",
        );

        expect(reloadedFlowPosition).toEqual(draggedFlowPosition);
        expect(draggedFlowPosition).not.toBeNull();

        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await server.stop();
        await browserPage.close();
      }
    },
    browserScenarioTimeoutMs,
  );
});
