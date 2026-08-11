// @vitest-environment node

import { rm } from "node:fs/promises";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  expectNoBrowserErrors,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";
import {
  exportFactoryPngFromDashboard,
  installCapturedDownloadHook,
  openImportDialogForDroppedPng,
  waitForDashboardReady,
} from "./factory-name-preservation-browser-helpers.mjs";

const nonDefaultSessionID = "session-review";

const defaultFactoryDefinition = {
  name: "Browser Session Harness Factory",
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

const reviewSessionFactoryDefinition = {
  metadata: {
    owner: "operations",
  },
  name: "Review Session Import Factory",
  resources: [
    {
      capacity: 2,
      name: "gpu",
    },
  ],
  workers: [
    {
      model: "gpt-5",
      name: "review-writer",
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
      body: "Review the story.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "review",
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
      worker: "review-writer",
    },
  ],
};

function renameReplayWorkstation(lines, workstationName) {
  return lines.map((line) => {
    const event = JSON.parse(line);
    const payloadFactory = event.payload?.factory;
    if (Array.isArray(payloadFactory?.workstations)) {
      payloadFactory.workstations = payloadFactory.workstations.map(
        (workstation) => ({
          ...workstation,
          name: workstationName,
        }),
      );
    }

    const payloadWorkstation = event.payload?.workstation;
    if (payloadWorkstation && typeof payloadWorkstation === "object") {
      event.payload.workstation = {
        ...payloadWorkstation,
        name: workstationName,
      };
    }

    return JSON.stringify(event);
  });
}

function buildReplayLines(factoryDefinition) {
  return [
    JSON.stringify({
      context: {
        eventTime: "2026-05-19T15:00:00Z",
        sequence: 1,
        tick: 1,
      },
      id: `factory-import-${factoryDefinition.name}`,
      payload: {
        factory: factoryDefinition,
      },
      type: "INITIAL_STRUCTURE_REQUEST",
    }),
    JSON.stringify({
      context: {
        eventTime: "2026-05-19T15:00:01Z",
        sequence: 2,
        tick: 2,
      },
      id: `factory-import-ready-${factoryDefinition.name}`,
      payload: {
        previousState: "RUNNING",
        reason: "fixture ready",
        state: "FINISHED",
      },
      type: "FACTORY_STATE_RESPONSE",
    }),
  ];
}

async function openReviewSessionTab(page) {
  await page
    .getByRole("button", { name: "Open Factory" })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  await page.getByRole("button", { name: "Open Factory" }).click();

  const dialog = page.getByRole("dialog", {
    name: "Open Factory",
  });
  await dialog.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  await dialog.getByLabel("Factory folder").fill("/workspace/project");
  await dialog.getByRole("button", { name: "Start Factory" }).click();
  await dialog
    .getByRole("button", { name: "Review factory" })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  await dialog.getByRole("button", { name: "Review factory" }).click();
  await dialog.getByRole("button", { name: "Open selected target" }).click();
  await dialog.waitFor({
    state: "hidden",
    timeout: uiInteractionTimeoutMs,
  });

  const activeReviewTab = page.getByRole("tab", {
    name: "review",
    selected: true,
  });
  await activeReviewTab.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
}

function createImportActivationTracking() {
  return {
    sessionFactoryPutRequests: [],
  };
}

async function startReviewSessionImportServer(preview, tracking) {
  const sessionReplayLines = renameReplayWorkstation(
    buildReplayLines(reviewSessionFactoryDefinition),
    "Session Review",
  );
  return startFactoryApiServer({
    apiPort: preview.apiPort,
    currentFactory: defaultFactoryDefinition,
    eventLines: buildReplayLines(defaultFactoryDefinition),
    eventLinesBySessionID: {
      [nonDefaultSessionID]: sessionReplayLines,
    },
    currentFactoryBySessionID: {
      [nonDefaultSessionID]: reviewSessionFactoryDefinition,
    },
    onSaveCurrentFactory: async (request) => {
      tracking.sessionFactoryPutRequests.push(request);
    },
    onOpenFactorySession: async (body) => {
      if (!body?.target) {
        return {
          targets: [
            {
              factoryDir: "/workspace/project/review",
              folderPath: "/workspace/project",
              label: "Review factory",
              project: "review",
              ref: {
                kind: "named",
                name: "review",
              },
            },
          ],
        };
      }

      return {
        session: {
          factoryDir: "/workspace/project/review",
          folderPath: "/workspace/project",
          id: nonDefaultSessionID,
          isDefault: false,
          project: "review",
          target: {
            kind: "named",
            name: "review",
          },
        },
      };
    },
  });
}

async function assertReviewSessionEventStream(page, server) {
  await openReviewSessionTab(page);
  await expect
    .poll(
      async () => server.requestedEventSessionIDs.includes(nonDefaultSessionID),
      { timeout: uiInteractionTimeoutMs },
    )
    .toBe(true);
  await page
    .getByRole("button", { name: "Select Session Review" })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
}

async function importFactoryPngAndActivate(page, options) {
  const { download, downloadPath, exportName, sessionFactoryPutRequests } =
    options;
  const importDialog = await openImportDialogForDroppedPng(
    page,
    downloadPath,
    download,
  );
  await importDialog
    .getByRole("img", { name: `${exportName} preview` })
    .waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
  expect(await importDialog.textContent()).toContain(exportName);
  expect(await importDialog.textContent()).toContain(download.filename);

  const activateButton = importDialog.getByRole("button", {
    name: "Confirm import",
  });
  await expect
    .poll(async () => await activateButton.isEnabled(), {
      timeout: uiInteractionTimeoutMs,
    })
    .toBe(true);
  await activateButton.click();
  await expect
    .poll(async () => sessionFactoryPutRequests.length, {
      timeout: uiInteractionTimeoutMs,
    })
    .toBe(1);
  await importDialog.waitFor({
    state: "hidden",
    timeout: uiInteractionTimeoutMs,
  });
}

function assertSessionScopedImportActivation(tracking) {
  expect(tracking.sessionFactoryPutRequests).toHaveLength(1);
  expect(tracking.sessionFactoryPutRequests[0]?.sessionID).toBe(
    nonDefaultSessionID,
  );
  if (tracking.sessionFactoryPutRequests[0]?.mode !== undefined) {
    expect(tracking.sessionFactoryPutRequests[0]?.mode).toBe("REPLACE_CURRENT");
  }
  expect(tracking.sessionFactoryPutRequests[0]?.body).toMatchObject({
    ...reviewSessionFactoryDefinition,
    name: reviewSessionFactoryDefinition.name,
    version: {
      logical: "2",
      physical: "2026-05-19T00:00:00.001Z",
    },
  });
}

async function runSecondSessionImportScenario(preview) {
  const tracking = createImportActivationTracking();
  const server = await startReviewSessionImportServer(preview, tracking);
  const browserPage = await openBrowserPage({
    acceptDownloads: true,
    artifactLabel: "factory-import-second-session",
  });
  let downloadDirectory = null;

  await installCapturedDownloadHook(browserPage.page);

  try {
    await waitForDashboardReady(browserPage.page, preview.previewURL, server);
    await assertReviewSessionEventStream(browserPage.page, server);

    const exportName = "Second Tab Import Roundtrip";
    const exportResult = await exportFactoryPngFromDashboard(
      browserPage.page,
      exportName,
    );
    downloadDirectory = exportResult.downloadDirectory;

    await importFactoryPngAndActivate(browserPage.page, {
      download: exportResult.download,
      downloadPath: exportResult.downloadPath,
      exportName,
      sessionFactoryPutRequests: tracking.sessionFactoryPutRequests,
    });
    assertSessionScopedImportActivation(tracking);
    expectNoBrowserErrors(
      browserPage.pageErrors,
      browserPage.consoleErrors,
      expect,
    );
  } finally {
    if (downloadDirectory) {
      await rm(downloadDirectory, { force: true, recursive: true });
    }
    await server.stop();
    await browserPage.close();
  }
}

describe.sequential("factory import second session browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "activates imported factory through PUT on the selected non-default session tab without POST /factories",
    async () => {
      await runSecondSessionImportScenario(preview);
    },
    browserScenarioTimeoutMs,
  );
});
