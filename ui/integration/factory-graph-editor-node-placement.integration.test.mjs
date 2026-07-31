// @vitest-environment node

import { describe } from "vitest";

import {
  browserScenarioTimeoutMs,
  expectNoBrowserErrors,
  fillModelWorkerAddOperationDraft,
  fillWorkstationPromptBody,
  modelProviderOptionLabel,
  resolvedDefaultFactorySessionID,
  selectLabeledComboboxOption,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";
import { waitForDashboardReady } from "./factory-name-preservation-browser-helpers.mjs";
import { isolatedMockBrowserTest as it } from "./mocked-browser-test-fixture.mjs";

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

const flowPositionTolerancePx = 8;
const persistedFlowPositionTolerancePx = 80;

function flowPositionsMatchWithinTolerance(
  left,
  right,
  tolerance = flowPositionTolerancePx,
) {
  if (!left || !right) {
    return false;
  }

  return (
    Math.abs(left.x - right.x) <= tolerance &&
    Math.abs(left.y - right.y) <= tolerance
  );
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
  await page.getByRole("button", { name: "Edit mode" }).click();
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

function isWithinViewportBounds(nodeCenter, viewportMetrics) {
  return (
    nodeCenter.x >= viewportMetrics.box.x &&
    nodeCenter.x <= viewportMetrics.box.x + viewportMetrics.box.width &&
    nodeCenter.y >= viewportMetrics.box.y &&
    nodeCenter.y <= viewportMetrics.box.y + viewportMetrics.box.height
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
  { model = "gpt-5", modelProvider = "CODEX", name },
) {
  await toolbar.getByRole("button", { name: "Add" }).click();
  await page
    .getByLabel("Add graph entity menu")
    .getByRole("button", { name: "Worker" })
    .evaluate((button) => button.click());

  const addDialog = page.getByRole("dialog", { name: "Add worker" });
  await addDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await addDialog.getByLabel("Identifier").fill(name);
  await selectLabeledComboboxOption(
    addDialog,
    "Model provider",
    modelProviderOptionLabel(modelProvider),
  );
  await addDialog.getByRole("textbox", { name: "Model" }).fill(model);
  await fillModelWorkerAddOperationDraft(addDialog);
  await addDialog.getByRole("button", { name: "Add entity" }).click();
  await addDialog.waitFor({
    state: "hidden",
    timeout: uiInteractionTimeoutMs,
  });
}

async function addWorkstation(page, toolbar, { body, name }) {
  await toolbar.getByRole("button", { name: "Add" }).click();
  await page
    .getByLabel("Add graph entity menu")
    .getByRole("button", { name: "Workstation" })
    .evaluate((button) => button.click());

  const addDialog = page.getByRole("dialog", { name: "Add workstation" });
  await addDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await addDialog.getByLabel("Identifier").fill(name);
  await fillWorkstationPromptBody(addDialog, body);
  await addDialog.getByRole("button", { name: "Add entity" }).click();
  await addDialog.waitFor({
    state: "hidden",
    timeout: uiInteractionTimeoutMs,
  });
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

function savedLayoutNodePosition(factory, nodeId) {
  return (
    factory?.layout?.nodes?.find((node) => node.id === nodeId)?.position ?? null
  );
}

async function readNodeScreenPosition(page, nodeTestId) {
  const box = await page.getByTestId(nodeTestId).boundingBox();
  if (!box) {
    return null;
  }

  return { x: box.x, y: box.y };
}

async function addResource(page, toolbar, { capacity = "1", name }) {
  await toolbar.getByRole("button", { name: "Add" }).click();
  await page
    .getByLabel("Add graph entity menu")
    .getByRole("button", { name: "Resource" })
    .evaluate((button) => button.click());

  const addDialog = page.getByRole("dialog", { name: "Add resource" });
  await addDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await addDialog.getByLabel("Identifier").fill(name);
  await addDialog.getByLabel("Capacity").fill(capacity);
  await addDialog.getByRole("button", { name: "Add entity" }).click();
  await addDialog.waitFor({
    state: "hidden",
    timeout: uiInteractionTimeoutMs,
  });
}

async function dragNodeByOffset(page, nodeTestId, deltaX, deltaY) {
  const node = page.getByTestId(nodeTestId);
  await node.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  const initialFlowPosition = await readNodeFlowPosition(page, nodeTestId);
  const nodeBox = await node.boundingBox();
  if (!nodeBox) {
    throw new Error(`Expected bounding boxes for ${nodeTestId} drag.`);
  }

  const startX = nodeBox.x + nodeBox.width / 2;
  const startY = nodeBox.y + nodeBox.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY, { steps: 16 });
  await page.mouse.up();

  const mouseFlowPosition = await readNodeFlowPosition(page, nodeTestId);
  if (
    initialFlowPosition &&
    mouseFlowPosition &&
    (Math.abs(mouseFlowPosition.x - initialFlowPosition.x) > 8 ||
      Math.abs(mouseFlowPosition.y - initialFlowPosition.y) > 8)
  ) {
    return;
  }

  await node.focus();
  for (let step = 0; step < Math.ceil(Math.abs(deltaX) / 10); step += 1) {
    await page.keyboard.press(deltaX > 0 ? "ArrowRight" : "ArrowLeft");
  }
  for (let step = 0; step < Math.ceil(Math.abs(deltaY) / 10); step += 1) {
    await page.keyboard.press(deltaY > 0 ? "ArrowDown" : "ArrowUp");
  }
}

async function saveGraphDraft(page, toolbar, expect) {
  const saveChangesButton = toolbar.getByRole("button", {
    name: "Save changes",
  });
  await expect
    .poll(async () => await saveChangesButton.isEnabled(), {
      timeout: uiInteractionTimeoutMs,
    })
    .toBe(true);

  await saveChangesButton.focus();
  await saveChangesButton.press("Enter");
  const saveDialog = page.getByRole("dialog", {
    name: "Save factory graph changes?",
  });
  await saveDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  const confirmButton = saveDialog
    .getByRole("button", { name: /Save (topology|changes)/ })
    .first();
  await confirmButton.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  const saveResponsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === "PUT" &&
      response
        .url()
        .includes(
          `/factory-sessions/${resolvedDefaultFactorySessionID}/factory`,
        ),
    { timeout: uiInteractionTimeoutMs },
  );
  await confirmButton.click();
  const saveResponse = await saveResponsePromise;
  if (!saveResponse.ok()) {
    throw new Error(
      `Expected graph save response to succeed, received ${saveResponse.status()}: ${await saveResponse.text()} | request=${saveResponse.request().postData() ?? "<empty>"}`,
    );
  }
}

describe.concurrent("factory graph editor node placement browser integration", () => {
  it(
    "places a newly added workstation near the visible viewport center after panning",
    async ({ expect, openBrowserPage, preview }) => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: editableGraphFactoryDefinition,
        eventLines: editableGraphFactoryReplayLines,
      });
      const browserPage = await openBrowserPage();

      try {
        await waitForDashboardReady(
          browserPage.page,
          preview.previewURL,
          server,
        );

        const toolbar = await enterGraphEditor(browserPage.page);
        await panGraphViewport(browserPage.page, -220, 160);

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

        const viewportMetrics = await viewportScreenMetrics(browserPage.page);
        const addedWorkstationCenter = await nodeScreenCenter(
          browserPage.page,
          "rf__node-workstation:review",
        );

        expect(
          isWithinViewportBounds(addedWorkstationCenter, viewportMetrics),
        ).toBe(true);

        const flowPosition = await readNodeFlowPosition(
          browserPage.page,
          "rf__node-workstation:review",
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
    async ({ expect, openBrowserPage, preview }) => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: editableGraphFactoryDefinition,
        eventLines: editableGraphFactoryReplayLines,
      });
      const browserPage = await openBrowserPage();

      try {
        await waitForDashboardReady(
          browserPage.page,
          preview.previewURL,
          server,
        );

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
    "persists a newly added workstation in the saved factory payload with its flow position",
    async ({ expect, openBrowserPage, preview }) => {
      const saveRequests = [];
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: editableGraphFactoryDefinition,
        eventLines: editableGraphFactoryReplayLines,
        onSaveCurrentFactory: async (value) => {
          saveRequests.push(value);
        },
      });
      const browserPage = await openBrowserPage();

      try {
        await waitForDashboardReady(
          browserPage.page,
          preview.previewURL,
          server,
        );

        const toolbar = await enterGraphEditor(browserPage.page);
        await panGraphViewport(browserPage.page, -180, 140);

        await addWorkstation(browserPage.page, toolbar, {
          body: "Review the drafted story.",
          name: "review",
        });

        const workstationTestId = "rf__node-workstation:review";
        await browserPage.page
          .getByTestId(workstationTestId)
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        const positionBeforeSave = await readNodeFlowPosition(
          browserPage.page,
          workstationTestId,
        );
        expect(positionBeforeSave).not.toBeNull();

        await saveGraphDraft(browserPage.page, toolbar, expect);
        await expect
          .poll(() => saveRequests.length, {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(1);
        expect(saveRequests[0]?.body?.workstations).toEqual(
          expect.arrayContaining([
            expect.objectContaining({
              body: "Review the drafted story.",
              name: "review",
              type: "INFERENCE_RUN",
            }),
          ]),
        );
        const persistedPosition = savedLayoutNodePosition(
          saveRequests[0]?.body,
          "workstation:review",
        );
        expect(
          flowPositionsMatchWithinTolerance(
            positionBeforeSave,
            persistedPosition,
            persistedFlowPositionTolerancePx,
          ),
        ).toBe(true);

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
    "persists a manually dragged node position in the saved shared layout",
    async ({ expect, openBrowserPage, preview }) => {
      const saveRequests = [];
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: editableGraphFactoryDefinition,
        eventLines: editableGraphFactoryReplayLines,
        onSaveCurrentFactory: async (value) => {
          saveRequests.push(value);
        },
      });
      const browserPage = await openBrowserPage();

      try {
        await waitForDashboardReady(
          browserPage.page,
          preview.previewURL,
          server,
        );

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

        await saveGraphDraft(browserPage.page, toolbar, expect);
        await expect
          .poll(() => saveRequests.length, {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(1);
        expect(saveRequests[0]?.body?.resources).toEqual(
          expect.arrayContaining([
            expect.objectContaining({
              capacity: 1,
              name: "extra-gpu",
            }),
          ]),
        );
        const persistedPosition = savedLayoutNodePosition(
          saveRequests[0]?.body,
          "resource:extra-gpu",
        );

        expect(draggedFlowPosition).not.toBeNull();
        expect(
          flowPositionsMatchWithinTolerance(
            draggedFlowPosition,
            persistedPosition,
            persistedFlowPositionTolerancePx,
          ),
        ).toBe(true);
        const dragChangedFlowPosition =
          initialFlowPosition &&
          draggedFlowPosition &&
          !flowPositionsMatchWithinTolerance(
            initialFlowPosition,
            draggedFlowPosition,
            8,
          );
        if (dragChangedFlowPosition) {
          expect(
            flowPositionsMatchWithinTolerance(
              initialFlowPosition,
              persistedPosition,
              40,
            ),
          ).toBe(false);
        }

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
