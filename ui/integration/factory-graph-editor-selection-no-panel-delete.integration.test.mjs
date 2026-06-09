// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  expectNoBrowserErrors,
  openBrowserPage,
  fillModelWorkerAddOperationDraft,
  modelProviderOptionLabel,
  selectLabeledComboboxOption,
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

const PANEL_TOPOLOGY_DELETE_BUTTON =
  /^Delete .+ (worker|work type|work state|workstation|resource)$/;

const topologySelectionButtons = [
  { entity: "worker", name: "Select writer worker" },
  { entity: "work type", name: "Select story work type" },
  { entity: "work state", name: "Select story:queued state" },
  { entity: "workstation", name: "Select draft workstation" },
  { entity: "resource", name: "Select gpu resource" },
];

function currentSelectionPanel(page) {
  return page.getByRole("article", { name: "Current selection" });
}

async function assertNoPanelTopologyDeleteInCurrentSelection(page) {
  const panel = currentSelectionPanel(page);
  await panel.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });

  expect(
    await panel.getByRole("heading", { name: "Factory graph" }).count(),
  ).toBe(0);

  for (const button of await panel.getByRole("button").all()) {
    const accessibleName = (await button.textContent())?.trim() ?? "";
    expect(accessibleName).not.toMatch(PANEL_TOPOLOGY_DELETE_BUTTON);
  }
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

async function addWorker(
  page,
  toolbar,
  { model = "gpt-5-mini", modelProvider = "CURSOR", name },
) {
  await toolbar.getByRole("button", { name: "Add" }).click();
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

describe.sequential("factory graph editor selection panel delete browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "shows current selection without panel topology delete for all five entity types",
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

        await enterGraphEditor(browserPage.page);

        for (const { name } of topologySelectionButtons) {
          const selectButton = browserPage.page.getByRole("button", {
            name,
          });
          await selectButton.waitFor({
            state: "visible",
            timeout: uiInteractionTimeoutMs,
          });
          await selectButton.click();
          await assertNoPanelTopologyDeleteInCurrentSelection(browserPage.page);
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

  it(
    "stages topology removal via graph delete tool and keeps worker configuration editable",
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

        await browserPage.page
          .getByRole("button", { name: "Select writer worker" })
          .click();
        const panel = currentSelectionPanel(browserPage.page);
        await panel
          .getByRole("heading", { name: "Worker configuration" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        const modelField = panel.getByRole("textbox", { name: "Model" });
        await modelField.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        expect(await modelField.isEditable()).toBe(true);
        await assertNoPanelTopologyDeleteInCurrentSelection(browserPage.page);

        await addWorker(browserPage.page, toolbar, { name: "spare" });
        await browserPage.page
          .getByTestId("rf__node-worker:spare")
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        const deleteToolButton = browserPage.page
          .getByRole("article", { name: "Factory graph" })
          .getByRole("button", { name: "Delete" });
        await deleteToolButton.scrollIntoViewIfNeeded();
        await deleteToolButton.click();
        expect(await deleteToolButton.getAttribute("aria-pressed")).toBe(
          "true",
        );

        await browserPage.page
          .getByRole("region", { name: "Work graph viewport" })
          .click({ position: { x: 8, y: 8 } });

        const spareGraphNode = browserPage.page.locator(
          '[data-id="worker:spare"]',
        );
        await spareGraphNode.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await spareGraphNode.click();

        await expect
          .poll(async () => await spareGraphNode.count(), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe(0);

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
