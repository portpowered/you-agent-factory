// @vitest-environment node

import { describe, expect } from "vitest";

import {
  browserScenarioTimeoutMs,
  expectNoBrowserErrors,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";
import { isolatedMockBrowserTest as it } from "./mocked-browser-test-fixture.mjs";

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

const openedFactoryDefinition = {
  name: "Opened Review Factory",
  workers: [
    {
      model: "gpt-5",
      name: "review-writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "review-story",
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
          workType: "review-story",
        },
      ],
      name: "session-review",
      outputs: [
        {
          state: "done",
          workType: "review-story",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "review-writer",
    },
  ],
};

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
      id: `session-tabs-${factoryDefinition.name}`,
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
      id: `session-tabs-ready-${factoryDefinition.name}`,
      payload: {
        previousState: "RUNNING",
        reason: "fixture ready",
        state: "FINISHED",
      },
      type: "FACTORY_STATE_RESPONSE",
    }),
  ];
}

describe.concurrent("dashboard session tabs browser integration", () => {
  it(
    "opens an existing factory from folder inspection into a new active session tab",
    async ({ expect, openBrowserPage, preview }) => {
      const openSessionRequests = [];
      const sessionReplayLines = renameReplayWorkstation(
        buildReplayLines(openedFactoryDefinition),
        "Session Review",
      );
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: buildReplayLines(defaultFactoryDefinition),
        eventLinesBySessionID: {
          "session-review": sessionReplayLines,
        },
        currentFactoryBySessionID: {
          "session-review": openedFactoryDefinition,
        },
        onOpenFactorySession: async (body) => {
          openSessionRequests.push(body);
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
              id: "session-review",
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
      const browserPage = await openBrowserPage();

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await browserPage.page
          .getByRole("heading", { level: 1, name: "U", exact: true })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByRole("button", { name: "Open another session" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByRole("button", { name: "Open another session" })
          .click();

        const dialog = browserPage.page.getByRole("dialog", {
          name: "Factory Session",
        });
        await dialog.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await dialog.getByLabel("Factory folder").fill("/workspace/project");
        await dialog.getByRole("button", { name: "Start Factory" }).click();

        await dialog
          .getByRole("button", { name: "Review factory" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await dialog.getByRole("button", { name: "Review factory" }).click();

        await dialog.waitFor({
          state: "hidden",
          timeout: uiInteractionTimeoutMs,
        });
        await expect
          .poll(
            async () =>
              server.requestedEventSessionIDs.includes("session-review"),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe(true);

        const activeReviewTab = browserPage.page.getByRole("tab", {
          name: "review",
          selected: true,
        });
        await activeReviewTab.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await browserPage.page
          .getByRole("button", { name: "Select Session Review" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        expect(openSessionRequests).toEqual([
          {
            folderPath: "/workspace/project",
            validateOnly: true,
          },
          {
            folderPath: "/workspace/project",
            target: {
              kind: "named",
              name: "review",
            },
          },
        ]);
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
    "shows outline active session tab styling and moves selection across preloaded tabs",
    async ({ expect, openBrowserPage, preview }) => {
      const replayLines = buildReplayLines(defaultFactoryDefinition);
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: replayLines,
        eventLinesBySessionID: {
          [betaSessionID]: replayLines,
        },
        currentFactoryBySessionID: {
          [betaSessionID]: defaultFactoryDefinition,
        },
        sessions: [defaultSession, betaSession],
      });
      const browserPage = await openBrowserPage();

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await browserPage.page
          .getByRole("heading", { level: 1, name: "U", exact: true })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

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
        await expect
          .poll(async () => rootTab.getAttribute("aria-selected"), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe("true");

        expectSubtleActiveSessionTabShell(
          await readSessionTabShellClassName(rootTab),
        );
        expectMutedInactiveSessionTabShell(
          await readSessionTabShellClassName(betaTab),
        );

        await betaTab.click();
        await expect
          .poll(async () => betaTab.getAttribute("aria-selected"), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe("true");
        await expect
          .poll(async () => rootTab.getAttribute("aria-selected"), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe("false");

        expectSubtleActiveSessionTabShell(
          await readSessionTabShellClassName(betaTab),
        );
        expectMutedInactiveSessionTabShell(
          await readSessionTabShellClassName(rootTab),
        );

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

async function readSessionTabShellClassName(tabLocator) {
  return tabLocator.evaluate((tab) => {
    const shell = tab.parentElement;
    if (!shell) {
      throw new Error("Expected session tab shell wrapper");
    }
    return shell.className;
  });
}

function expectSubtleActiveSessionTabShell(className) {
  expect(className).toContain("bg-surface-container-low");
  expect(className).not.toContain("border-outline-variant");
  expect(className).not.toContain("bg-surface-container-high");
  expect(className).not.toContain("bg-primary");
  expect(className).not.toContain("bg-primary-container");
}

function expectMutedInactiveSessionTabShell(className) {
  expect(className).toContain("text-on-surface-variant");
  expect(className).not.toContain("bg-surface-container-low");
  expect(className).not.toContain("bg-surface-container-high");
  expect(className).not.toContain("hover:border-outline");
}
