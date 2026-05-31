// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  buildTimeoutMs,
  browserScenarioTimeoutMs,
  expectNoBrowserErrors,
  loadReplayLines,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";

const defaultFactoryDefinition = {
  name: "Browser Session Harness Factory",
};

const openedFactoryDefinition = {
  name: "Opened Review Factory",
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

describe.sequential("dashboard session tabs browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "opens an existing factory from folder inspection into a new active session tab",
    async () => {
      const openSessionRequests = [];
      const sessionReplayLines = renameReplayWorkstation(
        await loadReplayLines("graph-state-smoke-replay.jsonl"),
        "Session Review",
      );
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: await loadReplayLines("graph-state-smoke-replay.jsonl"),
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
          .getByRole("button", { name: "Open another session" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByRole("button", { name: "Open another session" })
          .click();

        const dialog = browserPage.page.getByRole("dialog", {
          name: "Factory Session",
        });
        await dialog.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await dialog.getByLabel("Factory folder").fill("/workspace/project");
        await dialog.getByRole("button", { name: "Start Factory" }).click();

        await dialog
          .getByRole("button", { name: "Review factory" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await dialog
          .getByRole("button", { name: "Review factory" })
          .click();

        await dialog.waitFor({
          state: "hidden",
          timeout: uiInteractionTimeoutMs,
        });
        await expect
          .poll(
            async () => server.requestedEventSessionIDs.includes("session-review"),
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
    async () => {
      const replayLines = await loadReplayLines("graph-state-smoke-replay.jsonl");
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

        expectOutlineActiveSessionTabShell(
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

        expectOutlineActiveSessionTabShell(
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

function expectOutlineActiveSessionTabShell(className) {
  expect(className).toContain("border-af-border-strong");
  expect(className).toContain("bg-af-surface-raised");
  expect(className).not.toContain("bg-af-accent");
  expect(className).not.toContain("bg-af-accent-surface");
}

function expectMutedInactiveSessionTabShell(className) {
  expect(className).toContain("text-af-text-muted");
}
