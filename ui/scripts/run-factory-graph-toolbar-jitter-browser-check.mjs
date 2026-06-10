import { chromium } from "playwright";
import { ensureStorybookServer } from "./run-storybook-responsive-check.mjs";
import {
  storyUrl,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";

const host = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const port = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const storybookUrl = `http://${host}:${port}`;
const viewport = { height: 960, width: 1440 };

const GRAPH_VIEWPORT_DELTA_TOLERANCE_PX = 2;
const TOOLBAR_EXPANSION_MIN_DELTA_PX = 40;
const TOOLBAR_BUTTON_DELTA_TOLERANCE_PX = 4;
const SAMPLE_COUNT = 18;
const SAMPLE_INTERVAL_MS = 16;

const server = await ensureStorybookServer({ host, port: Number(port) });

try {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport });
  const storyId = await resolveStoryId({
    importPathSuffix:
      "/src/features/factory-graph-editor/components/controls/factory-graph-editor-controls.stories.tsx",
    storyName: "Toolbar Mode Transition",
    storybookUrl,
  });

  await page.goto(storyUrl(storybookUrl, storyId), {
    timeout: 90_000,
    waitUntil: "networkidle",
  });
  await waitForStoryRender(page);

  const toolbar = page.getByRole("region", {
    name: "Factory graph editor tools",
  });
  const probeSurface = page.getByLabel("Graph jitter probe surface");
  const editorControlsLane = page
    .locator("[data-toolbar-editor-controls-lane]")
    .first();
  const editorControlsRow = page
    .locator("[data-toolbar-editor-controls-row]")
    .first();
  const showButton = page.getByRole("button", { name: "Show or hide" }).first();
  const editModeButton = page
    .getByRole("button", { name: "Edit mode" })
    .first();
  const leaveEditorButton = page
    .getByRole("button", { name: "Leave editor" })
    .first();

  await toolbar.waitFor({ state: "visible" });
  await probeSurface.waitFor({ state: "visible" });
  await editorControlsLane.waitFor({ state: "attached" });
  await editorControlsRow.waitFor({ state: "attached" });
  await showButton.waitFor({ state: "visible" });
  await editModeButton.waitFor({ state: "visible" });

  const baseline = await readToolbarGeometry({
    editorControlsLane,
    editorControlsRow,
    modeButton: editModeButton,
    probeSurface,
    showButton,
    toolbar,
  });

  await editModeButton.click();
  await leaveEditorButton.waitFor({ state: "visible" });

  const expandSamples = await collectToolbarGeometrySamples({
    editorControlsLane,
    editorControlsRow,
    modeButton: leaveEditorButton,
    page,
    probeSurface,
    showButton,
    toolbar,
  });
  assertToolbarGeometryStable({
    baseline,
    expectedEditorControlsState: "expanded",
    label: "Entering edit mode",
    samples: expandSamples,
  });

  await leaveEditorButton.click();
  await editModeButton.waitFor({ state: "visible" });

  const collapseSamples = await collectToolbarGeometrySamples({
    editorControlsLane,
    editorControlsRow,
    modeButton: editModeButton,
    page,
    probeSurface,
    showButton,
    toolbar,
  });
  assertToolbarGeometryStable({
    baseline,
    expectedEditorControlsState: "collapsed",
    label: "Leaving edit mode",
    samples: collapseSamples,
  });

  console.log("Factory graph toolbar jitter browser verification passed.");
  await browser.close();
} finally {
  await server.stop();
}

async function resolveStoryId({ importPathSuffix, storyName, storybookUrl }) {
  const response = await fetch(`${storybookUrl}/index.json`);
  if (!response.ok) {
    throw new Error(`Could not load Storybook index: ${response.status}.`);
  }

  const index = await response.json();
  const entries = Object.values(index.entries ?? {});
  const storyEntry = entries.find(
    (entry) =>
      entry.importPath?.endsWith(importPathSuffix) && entry.name === storyName,
  );

  if (!storyEntry || typeof storyEntry.id !== "string") {
    throw new Error(`Could not resolve Storybook story id for "${storyName}".`);
  }

  return storyEntry.id;
}

async function collectToolbarGeometrySamples({
  editorControlsLane,
  editorControlsRow,
  modeButton,
  page,
  probeSurface,
  showButton,
  toolbar,
}) {
  const samples = [];

  for (let index = 0; index < SAMPLE_COUNT; index += 1) {
    await page.waitForTimeout(SAMPLE_INTERVAL_MS);
    samples.push(
      await readToolbarGeometry({
        editorControlsLane,
        editorControlsRow,
        modeButton,
        probeSurface,
        showButton,
        toolbar,
      }),
    );
  }

  return samples;
}

async function readToolbarGeometry({
  editorControlsLane,
  editorControlsRow,
  modeButton,
  probeSurface,
  showButton,
  toolbar,
}) {
  const [
    editorControlsLaneBox,
    editorControlsRowBox,
    modeButtonBox,
    probeBox,
    showButtonBox,
    toolbarBox,
  ] = await Promise.all([
    editorControlsLane.boundingBox(),
    editorControlsRow.boundingBox(),
    modeButton.boundingBox(),
    probeSurface.boundingBox(),
    showButton.boundingBox(),
    toolbar.boundingBox(),
  ]);

  if (
    !editorControlsLaneBox ||
    !editorControlsRowBox ||
    !modeButtonBox ||
    !probeBox ||
    !showButtonBox ||
    !toolbarBox
  ) {
    throw new Error("Could not measure toolbar jitter probe bounds.");
  }

  return {
    editorControlsLaneHeight: editorControlsLaneBox.height,
    editorControlsLaneWidth: editorControlsLaneBox.width,
    editorControlsRowHeight: editorControlsRowBox.height,
    editorControlsRowWidth: editorControlsRowBox.width,
    modeButtonHeight: modeButtonBox.height,
    modeButtonWidth: modeButtonBox.width,
    probeHeight: probeBox.height,
    probeTop: probeBox.y,
    showButtonHeight: showButtonBox.height,
    showButtonWidth: showButtonBox.width,
    toolbarHeight: toolbarBox.height,
    toolbarTop: toolbarBox.y,
  };
}

function assertToolbarGeometryStable({
  baseline,
  expectedEditorControlsState,
  label,
  samples,
}) {
  const finalSample = samples.at(-1);
  if (!finalSample) {
    throw new Error(`${label} did not collect toolbar geometry samples.`);
  }

  assertVerticalGeometryStable({ baseline, label, samples });
  assertEditorControlsWidthState({
    baseline,
    expectedEditorControlsState,
    finalSample,
    label,
  });
  assertToolbarButtonsStable({ baseline, label, samples });
}

function assertEditorControlsWidthState({
  baseline,
  expectedEditorControlsState,
  finalSample,
  label,
}) {
  if (
    expectedEditorControlsState === "expanded" &&
    finalSample.editorControlsLaneWidth - baseline.editorControlsLaneWidth <=
      TOOLBAR_EXPANSION_MIN_DELTA_PX
  ) {
    throw new Error(
      `${label} did not expand the editor controls lane horizontally.`,
    );
  }

  if (
    expectedEditorControlsState === "collapsed" &&
    Math.abs(
      finalSample.editorControlsLaneWidth - baseline.editorControlsLaneWidth,
    ) > TOOLBAR_BUTTON_DELTA_TOLERANCE_PX
  ) {
    throw new Error(
      `${label} did not return the editor controls lane to the collapsed width.`,
    );
  }

  if (
    expectedEditorControlsState === "expanded" &&
    finalSample.editorControlsRowWidth - baseline.editorControlsRowWidth <=
      TOOLBAR_EXPANSION_MIN_DELTA_PX
  ) {
    throw new Error(
      `${label} did not expand the editor controls row horizontally.`,
    );
  }

  if (
    expectedEditorControlsState === "collapsed" &&
    Math.abs(
      finalSample.editorControlsRowWidth - baseline.editorControlsRowWidth,
    ) > TOOLBAR_BUTTON_DELTA_TOLERANCE_PX
  ) {
    throw new Error(
      `${label} did not return the editor controls row to the collapsed width.`,
    );
  }
}

function assertVerticalGeometryStable({ baseline, label, samples }) {
  assertMaxDeltaWithin({
    baseline,
    key: "probeTop",
    label,
    limit: GRAPH_VIEWPORT_DELTA_TOLERANCE_PX,
    message: "moved the probe surface vertically",
    samples,
  });
  assertMaxDeltaWithin({
    baseline,
    key: "probeHeight",
    label,
    limit: GRAPH_VIEWPORT_DELTA_TOLERANCE_PX,
    message: "changed the probe surface height",
    samples,
  });
  assertMaxDeltaWithin({
    baseline,
    key: "editorControlsLaneHeight",
    label,
    limit: GRAPH_VIEWPORT_DELTA_TOLERANCE_PX,
    message: "changed the editor controls lane height",
    samples,
  });
  assertMaxDeltaWithin({
    baseline,
    key: "editorControlsRowHeight",
    label,
    limit: GRAPH_VIEWPORT_DELTA_TOLERANCE_PX,
    message: "changed the editor controls row height",
    samples,
  });
  assertMaxDeltaWithin({
    baseline,
    key: "toolbarTop",
    label,
    limit: GRAPH_VIEWPORT_DELTA_TOLERANCE_PX,
    message: "moved the toolbar vertically",
    samples,
  });
  assertMaxDeltaWithin({
    baseline,
    key: "toolbarHeight",
    label,
    limit: GRAPH_VIEWPORT_DELTA_TOLERANCE_PX,
    message: "changed the toolbar height",
    samples,
  });
}

function assertToolbarButtonsStable({ baseline, label, samples }) {
  assertMaxDeltaWithin({
    baseline,
    key: "showButtonHeight",
    label,
    limit: TOOLBAR_BUTTON_DELTA_TOLERANCE_PX,
    message: "changed the show button height",
    samples,
  });
  assertMaxDeltaWithin({
    baseline,
    key: "showButtonWidth",
    label,
    limit: TOOLBAR_BUTTON_DELTA_TOLERANCE_PX,
    message: "changed the show button width",
    samples,
  });
  assertMaxDeltaWithin({
    baseline,
    key: "modeButtonHeight",
    label,
    limit: TOOLBAR_BUTTON_DELTA_TOLERANCE_PX,
    message: "changed the mode button height",
    samples,
  });
  assertMaxDeltaWithin({
    baseline,
    key: "modeButtonWidth",
    label,
    limit: TOOLBAR_BUTTON_DELTA_TOLERANCE_PX,
    message: "changed the mode button width",
    samples,
  });
}

function assertMaxDeltaWithin({
  baseline,
  key,
  label,
  limit,
  message,
  samples,
}) {
  const maxDelta = Math.max(
    ...samples.map((sample) => Math.abs(sample[key] - baseline[key])),
  );

  if (maxDelta > limit) {
    throw new Error(`${label} ${message} by ${maxDelta.toFixed(2)}px.`);
  }
}
