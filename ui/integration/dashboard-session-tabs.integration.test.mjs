// @vitest-environment node

import { describe, expect } from "vitest";

import {
  browserScenarioTimeoutMs,
  expectNoBrowserErrors,
  loadReplayLines,
  resolvedDefaultFactorySessionID,
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

const betaFactoryDefinition = {
  ...defaultFactoryDefinition,
  name: "Beta Browser Session Harness Factory",
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
          .getByRole("button", { name: "Open Factory" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByRole("button", { name: "Open Factory" })
          .click();

        const dialog = browserPage.page.getByRole("dialog", {
          name: "Open Factory",
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
        await dialog
          .getByRole("button", { name: "Open selected target" })
          .click();

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
        await expect
          .poll(() => readSessionTabStatusLabel(rootTab), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe("Factory event stream connected.");
        await expect
          .poll(() => readSessionTabStatusLabel(betaTab), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe("Loading factory events...");

        const sessionControls = browserPage.page.getByRole("article", {
          name: "Session controls",
        });
        await sessionControls.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await sessionControls
          .getByRole("status", { name: "Timeline mode: Live" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        const workTotalsCard = browserPage.page.locator(
          '[data-bento-card-id="work-totals"]',
        );
        const workTotalsState = workTotalsCard.getByRole("status");
        await workTotalsState.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await expect
          .poll(
            () =>
              workTotalsState.getAttribute("data-dashboard-card-content-state"),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("known-empty");
        await expect
          .poll(
            () =>
              workTotalsState.getAttribute(
                "data-dashboard-card-freshness-state",
              ),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("fresh");
        await expect
          .poll(
            () =>
              workTotalsState.getAttribute(
                "data-dashboard-card-temporal-state",
              ),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("live");

        const pauseButton = sessionControls.getByRole("button", {
          name: "Pause live dashboard updates for root",
        });
        await pauseButton.focus();
        await pauseButton.press("Enter");
        await sessionControls
          .getByRole("status", { name: "Live dashboard updates paused" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await sessionControls
          .getByRole("button", {
            name: "Resume live dashboard updates for root",
          })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        const timelineSlider = sessionControls.getByRole("slider", {
          name: "Timeline tick",
        });
        await timelineSlider.focus();
        await timelineSlider.press("ArrowLeft");
        await sessionControls
          .getByRole("status", { name: "Timeline mode: Historical" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await expect
          .poll(
            () =>
              workTotalsState.getAttribute(
                "data-dashboard-card-temporal-state",
              ),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("historical");
        expect(
          await sessionControls
            .getByText("Factory Session paused", { exact: true })
            .count(),
        ).toBe(0);

        await browserPage.page.setViewportSize({ width: 360, height: 800 });
        await sessionControls
          .getByRole("status", { name: "Timeline mode: Historical" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await workTotalsState.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await browserPage.page.setViewportSize({ width: 1280, height: 900 });

        const resumeButton = sessionControls.getByRole("button", {
          name: "Resume live dashboard updates for root",
        });
        await resumeButton.focus();
        await resumeButton.press("Enter");
        await sessionControls
          .getByRole("button", {
            name: "Pause live dashboard updates for root",
          })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await timelineSlider.press("ArrowRight");
        await sessionControls
          .getByRole("status", { name: "Timeline mode: Live" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await expect
          .poll(
            () =>
              workTotalsState.getAttribute(
                "data-dashboard-card-temporal-state",
              ),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("live");

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
        await expect
          .poll(() => readSessionTabStatusLabel(betaTab), {
            timeout: uiInteractionTimeoutMs,
          })
          .toMatch(/\S+/);
        await expect
          .poll(() => readSessionTabStatusLabel(rootTab), {
            timeout: uiInteractionTimeoutMs,
          })
          .toMatch(/\S+/);

        await browserPage.page.setViewportSize({ width: 360, height: 800 });
        await expect
          .poll(
            () =>
              browserPage.page
                .getByRole("navigation", { name: "factory sessions" })
                .getAttribute("class"),
            { timeout: uiInteractionTimeoutMs },
          )
          .toMatch(/overflow-x-auto/);
        await betaTab.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });

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

  it(
    "keeps retained card data visible as its live stream goes stale at desktop and narrow widths",
    async ({ expect, openBrowserPage, preview }) => {
      const replayLines = await loadReplayLines("event-stream-replay.jsonl");
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "dashboard-card-reconnecting",
      });
      let eventStreamAttempts = 0;

      try {
        await browserPage.page.route("**/events**", async (route) => {
          const acceptHeader = route.request().headers().accept ?? "";
          if (!acceptHeader.includes("text/event-stream")) {
            await route.continue();
            return;
          }

          eventStreamAttempts += 1;
          if (eventStreamAttempts === 1) {
            const body = replayLines
              .map((line) => `data: ${line}\n\n`)
              .join("");
            await route.fulfill({
              body,
              contentType: "text/event-stream",
              status: 200,
            });
            return;
          }

          await route.abort("failed");
        });

        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await browserPage.page
          .getByRole("heading", { level: 1, name: "U", exact: true })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        const workTotalsState = browserPage.page
          .locator('[data-bento-card-id="work-totals"]')
          .getByRole("status");
        await workTotalsState.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await expect
          .poll(
            () =>
              workTotalsState.getAttribute("data-dashboard-card-content-state"),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("populated");
        await expect
          .poll(
            () =>
              workTotalsState.getAttribute(
                "data-dashboard-card-freshness-state",
              ),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("stale");
        expect(eventStreamAttempts).toBeGreaterThanOrEqual(1);

        await browserPage.page.setViewportSize({ width: 360, height: 800 });
        await workTotalsState.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
        await expect
          .poll(
            () =>
              workTotalsState.getAttribute(
                "data-dashboard-card-freshness-state",
              ),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("stale");

        await browserPage.page.setViewportSize({ width: 1280, height: 900 });
        await workTotalsState.waitFor({
          state: "visible",
          timeout: uiInteractionTimeoutMs,
        });
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
    "keeps the selected beta session stable when alpha stream data arrives late",
    async ({ expect, openBrowserPage, preview }) => {
      const alphaLines = buildReplayLines(defaultFactoryDefinition);
      const betaLines = buildReplayLines(betaFactoryDefinition);
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: alphaLines,
        eventLinesBySessionID: {
          [betaSessionID]: betaLines,
        },
        currentFactoryBySessionID: {
          [betaSessionID]: betaFactoryDefinition,
        },
        sessions: [defaultSession, betaSession],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "dashboard-late-session-result",
      });
      const delayedAlphaRoutes = [];
      let alphaStreamAttempts = 0;

      try {
        await browserPage.page.route("**/events**", async (route) => {
          const acceptHeader = route.request().headers().accept ?? "";
          if (!acceptHeader.includes("text/event-stream")) {
            await route.continue();
            return;
          }

          const pathname = new URL(route.request().url()).pathname;
          const alphaEventsPath = `/factory-sessions/${resolvedDefaultFactorySessionID}/events`;
          if (pathname !== alphaEventsPath) {
            await route.continue();
            return;
          }

          alphaStreamAttempts += 1;
          if (alphaStreamAttempts === 1) {
            await route.fulfill({
              body: alphaLines
                .slice(0, 2)
                .map((line) => `data: ${line}\n\n`)
                .join(""),
              contentType: "text/event-stream",
              status: 200,
            });
            return;
          }

          delayedAlphaRoutes.push(route);
        });

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
          .poll(() => delayedAlphaRoutes.length, {
            timeout: uiInteractionTimeoutMs,
          })
          .toBeGreaterThan(0);

        await betaTab.click();
        await expect
          .poll(() => betaTab.getAttribute("aria-selected"), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe("true");
        await expect
          .poll(() => readSessionTabStatusLabel(betaTab), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe("Factory event stream connected.");

        const sessionControls = browserPage.page.getByRole("article", {
          name: "Session controls",
        });
        await sessionControls
          .getByRole("status", { name: "Timeline mode: Live" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        const workTotalsState = browserPage.page
          .locator('[data-bento-card-id="work-totals"]')
          .getByRole("status");
        await expect
          .poll(
            () =>
              workTotalsState.getAttribute("data-dashboard-card-content-state"),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("known-empty");
        const betaCardText = await workTotalsState.textContent();
        const betaCardFreshness = await workTotalsState.getAttribute(
          "data-dashboard-card-freshness-state",
        );

        const timelineSlider = sessionControls.getByRole("slider", {
          name: "Timeline tick",
        });
        await timelineSlider.focus();
        await timelineSlider.press("ArrowLeft");
        await sessionControls
          .getByRole("status", { name: "Timeline mode: Historical" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await timelineSlider.press("ArrowRight");
        await sessionControls
          .getByRole("status", { name: "Timeline mode: Live" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        const delayedAlphaRoute = delayedAlphaRoutes.at(-1);
        if (!delayedAlphaRoute) {
          throw new Error("Expected a delayed alpha event-stream request.");
        }
        await delayedAlphaRoute.fulfill({
          body: alphaLines
            .slice(2)
            .map((line) => `data: ${line}\n\n`)
            .join(""),
          contentType: "text/event-stream",
          status: 200,
        });

        await expect
          .poll(() => betaTab.getAttribute("aria-selected"), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe("true");
        await expect
          .poll(() => readSessionTabStatusLabel(betaTab), {
            timeout: uiInteractionTimeoutMs,
          })
          .toBe("Factory event stream connected.");
        await expect
          .poll(
            () =>
              workTotalsState.getAttribute("data-dashboard-card-content-state"),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe("known-empty");
        await expect
          .poll(
            () =>
              workTotalsState.getAttribute(
                "data-dashboard-card-freshness-state",
              ),
            { timeout: uiInteractionTimeoutMs },
          )
          .toBe(betaCardFreshness);
        expect(await workTotalsState.textContent()).toBe(betaCardText);
        await sessionControls
          .getByRole("status", { name: "Timeline mode: Live" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

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

async function readSessionTabStatusLabel(tabLocator) {
  return tabLocator.getByRole("img").getAttribute("aria-label");
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
