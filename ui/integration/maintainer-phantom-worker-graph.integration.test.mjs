// @vitest-environment node

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

const SYSTEM_TIME_WORK_TYPE_ID = "__system_time";
const SYSTEM_TIME_EXPIRY_TRANSITION_ID = "__system_time:expire";

const maintainerRuntimeShapedFactory = {
  name: "maintainer-runtime-factory",
  resources: [{ capacity: 10, name: "executor-slot" }],
  workers: [{ name: "processor" }, { name: "workspace-setup" }],
  workTypes: [
    {
      name: "task",
      states: [
        { name: "init", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
    {
      name: SYSTEM_TIME_WORK_TYPE_ID,
      states: [{ name: "pending", type: "PROCESSING" }],
    },
  ],
  workstations: [
    {
      inputs: [{ state: "init", workType: "task" }],
      name: "process",
      outputs: [{ state: "done", workType: "task" }],
      resources: [{ capacity: 1, name: "executor-slot" }],
      worker: "processor",
    },
    {
      inputs: [{ state: "init", workType: "task" }],
      name: "setup-workspace",
      outputs: [{ state: "done", workType: "task" }],
      worker: "workspace-setup",
    },
    {
      inputs: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
      name: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
      outputs: [],
      worker: "",
    },
  ],
};

const maintainerRuntimeShapedEventLines = [
  JSON.stringify({
    context: {
      eventTime: "2026-05-31T20:00:00Z",
      sequence: 1,
      tick: 1,
    },
    id: "maintainer-phantom-worker-smoke-1",
    payload: {
      factory: maintainerRuntimeShapedFactory,
    },
    type: "INITIAL_STRUCTURE_REQUEST",
  }),
  JSON.stringify({
    context: {
      eventTime: "2026-05-31T20:00:01Z",
      sequence: 2,
      tick: 2,
    },
    id: "maintainer-phantom-worker-smoke-2",
    payload: {
      previousState: "RUNNING",
      reason: "maintainer phantom worker smoke complete",
      state: "FINISHED",
    },
    type: "FACTORY_STATE_RESPONSE",
  }),
];

describe.sequential("maintainer phantom worker graph browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  }, buildTimeoutMs);

  it(
    "renders defined worker labels without a bare worker: node and supports worker selection",
    async () => {
      const replayServer = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: maintainerRuntimeShapedFactory,
        eventLines: maintainerRuntimeShapedEventLines,
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "maintainer-phantom-worker-graph",
      });

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await replayServer.replayCompleted;

        const graphViewport = browserPage.page.getByRole("region", {
          name: "Work graph viewport",
        });
        await graphViewport.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        const processorWorkerButton = graphViewport.getByRole("button", {
          name: "Select processor worker",
        });
        const workspaceSetupWorkerButton = graphViewport.getByRole("button", {
          name: "Select workspace-setup worker",
        });

        await processorWorkerButton.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await workspaceSetupWorkerButton.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

        const workerSelectionLabels = await graphViewport
          .getByRole("button", { name: / worker$/ })
          .evaluateAll((elements) =>
            elements
              .map(
                (element) => element.getAttribute("aria-label")?.trim() ?? "",
              )
              .filter((label) => label.length > 0),
          );
        expect(workerSelectionLabels).toEqual(
          expect.arrayContaining([
            "Select processor worker",
            "Select workspace-setup worker",
          ]),
        );
        expect(workerSelectionLabels).not.toContain("Select  worker");

        await processorWorkerButton.scrollIntoViewIfNeeded();
        await processorWorkerButton.click();
        await expect
          .poll(
            async () => processorWorkerButton.getAttribute("aria-pressed"),
            {
              timeout: uiInteractionTimeoutMs,
            },
          )
          .toBe("true");

        await workspaceSetupWorkerButton.scrollIntoViewIfNeeded();
        await workspaceSetupWorkerButton.click();
        await expect
          .poll(
            async () =>
              workspaceSetupWorkerButton.getAttribute("aria-pressed"),
            {
              timeout: uiInteractionTimeoutMs,
            },
          )
          .toBe("true");
        await expect
          .poll(
            async () => processorWorkerButton.getAttribute("aria-pressed"),
            {
              timeout: uiInteractionTimeoutMs,
            },
          )
          .toBeNull();

        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await replayServer.stop();
        await browserPage.close();
      }
    },
    browserScenarioTimeoutMs,
  );
});
