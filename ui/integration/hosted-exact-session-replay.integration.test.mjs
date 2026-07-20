// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  expectNoBrowserErrors,
  openBrowserPage,
  resolvedDefaultFactorySessionID,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";

const betaSessionID = "hosted-session-beta";
const defaultSession = {
  factoryDir: "/workspace/root",
  folderPath: "/workspace/root",
  id: "~default",
  isDefault: true,
  project: "root",
  target: { kind: "default" },
};
const betaSession = {
  factoryDir: "/workspace/root/beta",
  folderPath: "/workspace/root",
  id: betaSessionID,
  isDefault: false,
  project: "beta",
  target: { kind: "named", name: "beta" },
};

function factoryDefinition(prefix, stateName) {
  return {
    name: `${prefix} hosted factory`,
    workers: [],
    workTypes: [
      {
        name: `${prefix}-story`,
        states: [{ name: stateName, type: "INITIAL" }],
      },
    ],
    workstations: [],
  };
}

function replayLines(prefix) {
  return [
    [1, 1, `${prefix}-initial`],
    [2, 2, `${prefix}-same-tick-head`],
    [2, 3, `${prefix}-same-tick-tail`],
  ].map(([tick, sequence, stateName]) =>
    JSON.stringify({
      context: {
        eventTime: `2026-07-20T12:00:0${sequence}.000Z`,
        sequence,
        tick,
      },
      id: `${prefix}-topology-${sequence}`,
      payload: { factory: factoryDefinition(prefix, stateName) },
      type: sequence === 1 ? "INITIAL_STRUCTURE_REQUEST" : "FACTORY_CHANGE",
    }),
  );
}

async function waitForExactTopology(page, expectedState, rejectedState) {
  await page.getByText(expectedState, { exact: true }).waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  expect(await page.getByText(rejectedState, { exact: true }).count()).toBe(0);
  await page
    .getByRole("slider", { name: "Select Factory replay tick" })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
}

async function exerciseExactSessionReplay(preview, viewport, refreshSelection) {
  const rootLines = replayLines("root");
  const betaLines = replayLines("beta");
  const server = await startFactoryApiServer({
    apiPort: preview.apiPort,
    currentFactory: factoryDefinition("root", "root-current"),
    currentFactoryBySessionID: {
      [betaSessionID]: factoryDefinition("beta", "beta-current"),
    },
    eventLines: rootLines,
    eventLinesBySessionID: { [betaSessionID]: betaLines },
    sessions: [defaultSession, betaSession],
  });
  const browserPage = await openBrowserPage();
  const { page } = browserPage;

  try {
    await page.setViewportSize(viewport);
    await page.goto(preview.previewURL, { waitUntil: "domcontentloaded" });
    await waitForExactTopology(
      page,
      "root-same-tick-tail",
      "root-same-tick-head",
    );
    await expect
      .poll(
        () =>
          server.requestedEventSessionIDs.includes(
            resolvedDefaultFactorySessionID,
          ),
        { timeout: uiInteractionTimeoutMs },
      )
      .toBe(true);

    await page.getByRole("tab", { name: "beta" }).click();
    await waitForExactTopology(
      page,
      "beta-same-tick-tail",
      "root-same-tick-tail",
    );
    await expect
      .poll(() => server.requestedEventSessionIDs.includes(betaSessionID), {
        timeout: uiInteractionTimeoutMs,
      })
      .toBe(true);

    if (refreshSelection) {
      await page.reload({ waitUntil: "domcontentloaded" });
      await waitForExactTopology(
        page,
        "root-same-tick-tail",
        "beta-same-tick-tail",
      );
    } else {
      await page.getByRole("tab", { name: "root" }).click();
      await waitForExactTopology(
        page,
        "root-same-tick-tail",
        "beta-same-tick-tail",
      );
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
}

describe.sequential("hosted exact-session topology replay", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "renders only the selected hosted session in canonical same-tick order at desktop width",
    async () => {
      await exerciseExactSessionReplay(
        preview,
        { height: 900, width: 1440 },
        false,
      );
    },
    browserScenarioTimeoutMs,
  );

  it(
    "renders only the selected hosted session at narrow width and refreshes from the authoritative default",
    async () => {
      await exerciseExactSessionReplay(
        preview,
        { height: 844, width: 390 },
        true,
      );
    },
    browserScenarioTimeoutMs,
  );
});
