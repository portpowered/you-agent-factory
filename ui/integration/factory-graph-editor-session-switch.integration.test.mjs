// @vitest-environment node

import { describe } from "vitest";

import {
  browserScenarioTimeoutMs,
  expectNoBrowserErrors,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";
import { isolatedMockBrowserTest as it } from "./mocked-browser-test-fixture.mjs";

const betaSessionID = "session-beta";

const defaultSession = {
  factoryDir: "/workspace/root",
  folderPath: "/workspace/root",
  id: "~default",
  isDefault: true,
  project: "root",
  target: {
    kind: "default",
  },
};

const betaSession = {
  factoryDir: "/workspace/root/beta",
  folderPath: "/workspace/root",
  id: betaSessionID,
  isDefault: false,
  project: "beta",
  target: {
    kind: "named",
    name: "beta",
  },
};

const alphaGraphFactoryDefinition = {
  name: "Alpha Graph Factory",
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
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

const betaGraphFactoryDefinition = {
  name: "Beta Graph Factory",
  workers: [
    {
      model: "gpt-5",
      name: "beta-writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "beta-story",
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
      body: "Beta drafting.",
      inputs: [
        {
          state: "queued",
          workType: "beta-story",
        },
      ],
      name: "beta-station",
      outputs: [
        {
          state: "done",
          workType: "beta-story",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "beta-writer",
    },
  ],
};

function buildEditableGraphReplayLines(workstationName, workTypeName) {
  return [
    JSON.stringify({
      context: {
        eventTime: "2026-05-19T15:00:00Z",
        sequence: 1,
        tick: 1,
      },
      id: `editable-graph-${workstationName}`,
      payload: {
        factory: {
          workTypes: [
            {
              name: workTypeName,
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
                  workType: workTypeName,
                },
              ],
              name: workstationName,
              outputs: [
                {
                  state: "done",
                  workType: workTypeName,
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
      id: `editable-graph-ready-${workstationName}`,
      payload: {
        previousState: "RUNNING",
        reason: "fixture ready",
        state: "FINISHED",
      },
      type: "FACTORY_STATE_RESPONSE",
    }),
  ];
}

const alphaGraphReplayLines = buildEditableGraphReplayLines("draft", "story");
const betaGraphReplayLines = buildEditableGraphReplayLines(
  "beta-station",
  "beta-story",
);

function factoryGraphCardScope(page) {
  return page.getByRole("article", { name: "Factory graph" });
}

async function enterGraphEditor(page) {
  const graphCard = factoryGraphCardScope(page);
  await graphCard.getByRole("button", { name: "Edit mode" }).click();
  await graphCard
    .getByRole("region", { name: "Factory graph editor tools" })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
}

async function addUnsavedWorkType(page, identifier, expect) {
  const graphCard = factoryGraphCardScope(page);
  const toolbar = graphCard.getByRole("region", {
    name: "Factory graph editor tools",
  });
  await toolbar.getByRole("button", { name: "Add" }).click();
  await page
    .getByLabel("Add graph entity menu")
    .getByRole("button", { name: "Work type" })
    .evaluate((button) => button.click());

  const addDialog = page.getByRole("dialog", {
    name: "Add work type",
  });
  await addDialog.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await addDialog.getByLabel("Identifier").fill(identifier);
  await addDialog.getByLabel("First state").fill("queued");
  await addDialog.getByRole("button", { name: "Add entity" }).click();

  const discardChangesButton = toolbar.getByRole("button", {
    name: "Discard changes",
  });
  await expect
    .poll(async () => await discardChangesButton.isEnabled(), {
      timeout: uiInteractionTimeoutMs,
    })
    .toBe(true);
}

async function expectEditorModeOff(page, expect) {
  const graphCard = factoryGraphCardScope(page);
  await graphCard
    .getByRole("button", { name: "Edit mode" })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  const toolbar = graphCard.getByRole("region", {
    name: "Factory graph editor tools",
  });
  await expect
    .poll(
      async () => {
        const addMenuCount = await toolbar
          .getByRole("button", { name: "Add" })
          .count();
        const unsavedCount = await graphCard
          .getByText("Unsaved changes")
          .count();
        const editorActiveCount = await graphCard
          .getByText("Editor mode active")
          .count();
        return addMenuCount + unsavedCount + editorActiveCount;
      },
      { timeout: uiInteractionTimeoutMs },
    )
    .toBe(0);
  await graphCard
    .getByRole("button", {
      name: "Edit mode",
    })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
}

async function runSessionSwitchClearsDirtyEditorScenario({
  expect,
  openBrowserPage,
  preview,
}) {
  const server = await startFactoryApiServer({
    apiPort: preview.apiPort,
    currentFactory: alphaGraphFactoryDefinition,
    currentFactoryBySessionID: {
      [betaSessionID]: betaGraphFactoryDefinition,
    },
    eventLines: alphaGraphReplayLines,
    eventLinesBySessionID: {
      [betaSessionID]: betaGraphReplayLines,
    },
    sessions: [defaultSession, betaSession],
  });
  const browserPage = await openBrowserPage();

  try {
    await browserPage.page.goto(preview.previewURL, {
      waitUntil: "domcontentloaded",
    });
    await server.replayCompleted;

    const rootTab = browserPage.page.getByRole("tab", { name: "root" });
    const betaTab = browserPage.page.getByRole("tab", { name: "beta" });
    await rootTab.waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
    await betaTab.waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });

    await enterGraphEditor(browserPage.page);
    await addUnsavedWorkType(browserPage.page, "essay", expect);
    await expect
      .poll(
        async () =>
          await browserPage.page
            .getByTestId("rf__node-work-type:essay")
            .count(),
        { timeout: uiInteractionTimeoutMs },
      )
      .toBe(1);

    await betaTab.click();
    await expect
      .poll(async () => betaTab.getAttribute("aria-selected"), {
        timeout: uiInteractionTimeoutMs,
      })
      .toBe("true");

    await expectEditorModeOff(browserPage.page, expect);
    expect(
      await browserPage.page.getByTestId("rf__node-work-type:essay").count(),
    ).toBe(0);
    await browserPage.page
      .getByTestId("rf__node-workstation:beta-station")
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    expect(
      await browserPage.page.getByTestId("rf__node-workstation:draft").count(),
    ).toBe(0);

    await rootTab.click();
    await expect
      .poll(async () => rootTab.getAttribute("aria-selected"), {
        timeout: uiInteractionTimeoutMs,
      })
      .toBe("true");

    await expectEditorModeOff(browserPage.page, expect);
    await browserPage.page
      .getByTestId("rf__node-workstation:draft")
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    expect(
      await browserPage.page.getByTestId("rf__node-work-type:essay").count(),
    ).toBe(0);

    expectNoBrowserErrors(
      browserPage.pageErrors,
      browserPage.consoleErrors,
      expect,
    );
  } finally {
    await server.stop();
    await browserPage.close();
  }
}

async function runLeaveEditorUnsavedPromptScenario({
  expect,
  openBrowserPage,
  preview,
}) {
  const server = await startFactoryApiServer({
    apiPort: preview.apiPort,
    currentFactory: alphaGraphFactoryDefinition,
    eventLines: alphaGraphReplayLines,
  });
  const browserPage = await openBrowserPage();

  try {
    await browserPage.page.goto(preview.previewURL, {
      waitUntil: "domcontentloaded",
    });
    await server.replayCompleted;

    await enterGraphEditor(browserPage.page);
    await addUnsavedWorkType(browserPage.page, "memo", expect);

    const graphCard = factoryGraphCardScope(browserPage.page);
    await graphCard.getByRole("button", { name: "Leave editor" }).click();

    const leaveDialog = browserPage.page.getByRole("dialog", {
      name: "Leave graph editor with unsaved changes?",
    });
    await leaveDialog.waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
    await leaveDialog.getByRole("button", { name: "Keep editing" }).click();
    await leaveDialog.waitFor({
      state: "hidden",
      timeout: uiInteractionTimeoutMs,
    });
    await graphCard
      .getByRole("region", { name: "Factory graph editor tools" })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

    await graphCard.getByRole("button", { name: "Leave editor" }).click();
    await leaveDialog.waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
    await leaveDialog.getByRole("button", { name: "Discard changes" }).click();
    await leaveDialog.waitFor({
      state: "hidden",
      timeout: uiInteractionTimeoutMs,
    });
    await expectEditorModeOff(browserPage.page, expect);

    expectNoBrowserErrors(
      browserPage.pageErrors,
      browserPage.consoleErrors,
      expect,
    );
  } finally {
    await server.stop();
    await browserPage.close();
  }
}

describe.concurrent("factory graph editor session switch browser integration", () => {
  it(
    "clears unsaved graph editor chrome and session A topology when switching to session B",
    async ({ expect, openBrowserPage, preview }) =>
      runSessionSwitchClearsDirtyEditorScenario({
        expect,
        openBrowserPage,
        preview,
      }),
    browserScenarioTimeoutMs,
  );

  it(
    "prompts before leaving graph editor with unsaved edits within one session",
    async ({ expect, openBrowserPage, preview }) =>
      runLeaveEditorUnsavedPromptScenario({
        expect,
        openBrowserPage,
        preview,
      }),
    browserScenarioTimeoutMs,
  );
});
