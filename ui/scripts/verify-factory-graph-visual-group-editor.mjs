/**
 * Drives the real factory graph editor surface for visual group save/reload coverage.
 */

// biome-ignore lint/style/noExcessiveLinesPerFile: this browser workflow composes the full editor save, reload, and interaction scenario.
import {
  assertNodeSizeMatches,
  expectNoObsoleteNodeResizeActions,
  readNodeSize,
  resizeSelectedNode,
} from "./verify-factory-graph-node-size-browser-helpers.mjs";
import { expectVisualGroupDragWithMembers } from "./verify-factory-graph-visual-group-editor-group-drag.mjs";
import {
  expectDirectEdgeDrag,
  expectEditorGraphInteractions,
  expectGraphSurfaceBasics,
  expectPersistedEdgeWaypoint,
  expectRegionPointerThrough,
} from "./verify-factory-graph-visual-group-editor-interactions.mjs";
import {
  captureEvidence,
  selectGraphNodeWithMarquee,
} from "./verify-factory-graph-visual-group-editor-workflow-helpers.mjs";

const TARGET_WORKSTATION_LABEL = "Plan";
const TARGET_WORKSTATION_ID = "workstation:plan";
const GROUP_LABEL = "Planning lane";
const CUSTOM_GROUP_COLOR = "#a1b2c3";

export async function verifyFactoryGraphVisualGroupEditorWorkflow({
  page,
  storyUrl,
  waitForStoryRender,
}) {
  await page.goto(storyUrl, {
    timeout: 90_000,
    waitUntil: "networkidle",
  });
  await waitForStoryRender(page);
  await page.evaluate(() => {
    window.__resetVisualGroupEditorStory?.();
  });
  await page.reload({ waitUntil: "networkidle" });
  await waitForStoryRender(page);

  const { toolbar, viewport } = await enterVisualGroupEditor(page);
  const { edgeId, resizedNode } = await createAndEditVisualGroup(
    page,
    viewport,
  );
  const savedNodeSize = await saveVisualGroup(page, resizedNode, edgeId);

  await verifyVisualGroupAfterReload({
    currentUrl: page.url(),
    expectedEdgeId: edgeId,
    expectedNodeSize: savedNodeSize,
    page,
    toolbar,
    waitForStoryRender,
  });
}

async function enterVisualGroupEditor(page) {
  const viewport = page.getByRole("region", { name: "Work graph viewport" });
  await viewport.waitFor({ state: "visible" });

  const editModeButton = page.getByRole("button", { name: "Edit mode" });
  await editModeButton.waitFor({ state: "visible" });
  await editModeButton.click();
  await page.getByRole("button", { name: "Leave editor" }).waitFor({
    state: "visible",
  });

  const toolbar = page.getByRole("region", {
    name: "Factory graph editor tools",
  });
  await toolbar.waitFor({ state: "visible" });

  return { toolbar, viewport };
}

async function createAndEditVisualGroup(page, viewport) {
  const targetWorkstationNode = page
    .locator(".react-flow__node")
    .filter({
      has: page.getByRole("button", {
        exact: true,
        name: `Select ${TARGET_WORKSTATION_LABEL} workstation`,
      }),
    })
    .first();
  await targetWorkstationNode.waitFor({ state: "visible" });
  await selectGraphNodeWithMarquee(
    page,
    viewport,
    targetWorkstationNode,
    TARGET_WORKSTATION_LABEL,
  );
  // The marquee leaves React Flow's nodes-selection overlay stacked above the
  // node, which swallows pointer input aimed at the in-node resize grip.
  // Clear it and reselect the workstation directly, the way an operator grabs
  // the grip after clicking a node.
  await viewport.press("Escape");
  await page
    .locator(".react-flow__nodesselection-rect")
    .waitFor({ state: "detached" });
  await targetWorkstationNode
    .getByRole("button", {
      exact: true,
      name: `Select ${TARGET_WORKSTATION_LABEL} workstation`,
    })
    .click();
  await targetWorkstationNode
    .getByRole("button", {
      exact: true,
      name: `Select ${TARGET_WORKSTATION_LABEL} workstation`,
      pressed: true,
    })
    .waitFor({ state: "visible" });
  const resizedNode = await resizeSelectedNode(
    page,
    targetWorkstationNode,
    TARGET_WORKSTATION_ID,
  );

  await page.getByRole("button", { name: "Create group" }).click();
  await page
    .locator("[data-factory-visual-group-controls]")
    .waitFor({ state: "visible" });
  await expectVisualGroupControlsWithinViewport(page, viewport);

  const labelField = page.getByRole("textbox", { name: "Group label" });
  await labelField.fill(GROUP_LABEL);
  await expectLabelField(page, GROUP_LABEL);

  const membershipCheckbox = page.getByRole("checkbox", {
    name: `Include ${TARGET_WORKSTATION_LABEL} in this group`,
  });
  await expectChecked(membershipCheckbox, true);
  const secondaryMembershipCheckbox = page.getByRole("checkbox", {
    name: "Include Implement in this group",
  });
  const secondaryMembershipInputId =
    await secondaryMembershipCheckbox.getAttribute("id");
  if (!secondaryMembershipInputId) {
    throw new Error(
      "Expected the secondary membership Checkbox to have an id.",
    );
  }
  await page.locator(`label[for="${secondaryMembershipInputId}"]`).click();
  await expectChecked(secondaryMembershipCheckbox, true);
  await secondaryMembershipCheckbox.focus();
  await page.keyboard.press("Space");
  await expectChecked(secondaryMembershipCheckbox, false);
  await page.keyboard.press("Space");
  await expectChecked(secondaryMembershipCheckbox, true);
  const unrelatedMembershipCheckbox = page.getByRole("checkbox", {
    name: "Include Review in this group",
  });
  if (await unrelatedMembershipCheckbox.isChecked()) {
    await unrelatedMembershipCheckbox.uncheck();
  }
  await expectChecked(unrelatedMembershipCheckbox, false);

  const warningColorButton = page.getByRole("button", {
    exact: true,
    name: "Use warning group color",
  });
  await warningColorButton.click();
  await expectAttribute(warningColorButton, "aria-pressed", "true");

  const customColorPicker = page.getByLabel("Custom group color");
  await customColorPicker.fill(CUSTOM_GROUP_COLOR);
  await expectInputValue(customColorPicker, CUSTOM_GROUP_COLOR);
  await expectAttribute(warningColorButton, "aria-pressed", "false");

  const boundsBeforeResize = await readVisualGroupBounds(page);
  await resizeVisualGroup(page, "sw", { deltaX: -40, deltaY: 30 });
  const boundsAfterResize = await readVisualGroupBounds(page);
  expectBoundsChanged(boundsBeforeResize, boundsAfterResize, "resize");

  await page.getByRole("button", { name: "Fit to members" }).click();
  const boundsAfterFit = await readVisualGroupBounds(page);
  expectFiniteBounds(boundsAfterFit, "fit group");

  await expectVisualGroupDragWithMembers(page, {
    memberNodeIds: [TARGET_WORKSTATION_ID, "workstation:implement"],
    unrelatedNodeId: "workstation:review",
  });
  await resizeVisualGroup(page, "sw", { deltaX: -560, deltaY: 420 });

  const edgeId = await expectDirectEdgeDrag(
    page,
    page.locator("[data-factory-visual-group]").first(),
  );
  return { edgeId, resizedNode };
}

async function saveVisualGroup(page, resizedNode, edgeId) {
  await page.getByRole("button", { name: "Save changes" }).click();
  await page
    .getByRole("heading", { name: "Save factory graph changes?" })
    .waitFor({ state: "visible" });
  await page.getByRole("button", { name: "Save layout" }).click();
  await page.getByText("Topology saved").waitFor({ state: "visible" });

  const persistedFactory = await page.evaluate(() =>
    window.__getVisualGroupEditorPersistedFactory?.(),
  );
  const persistedGroup = persistedFactory?.layout?.groups?.find(
    (group) => group.label === GROUP_LABEL,
  );
  if (!persistedGroup) {
    throw new Error(`Saved factory did not contain the ${GROUP_LABEL} group.`);
  }
  if (persistedGroup.color !== CUSTOM_GROUP_COLOR) {
    throw new Error(
      `Expected the saved group color to be ${CUSTOM_GROUP_COLOR}, found ${persistedGroup.color}.`,
    );
  }
  if (!persistedGroup.nodeIds?.includes(TARGET_WORKSTATION_ID)) {
    throw new Error(
      `Expected the saved ${GROUP_LABEL} group to retain the selected ${TARGET_WORKSTATION_LABEL} member.`,
    );
  }
  expectFiniteBounds(persistedGroup.bounds, "saved group");
  const persistedNode = persistedFactory?.layout?.nodes?.find(
    (node) => node.id === TARGET_WORKSTATION_ID,
  )?.size;
  assertNodeSizeMatches(persistedNode, resizedNode, "saved");
  const persistedEdge = persistedFactory?.layout?.edges?.find(
    (edge) => edge.id === edgeId,
  );
  if (!persistedEdge?.waypoints?.length) {
    throw new Error(
      `Saved Factory layout did not retain the directly dragged waypoint for ${edgeId}.`,
    );
  }
  await captureEvidence(page, "visual-group-before-reload");
  return persistedNode;
}

async function verifyVisualGroupAfterReload({
  currentUrl,
  expectedEdgeId,
  expectedNodeSize,
  page,
  toolbar,
  waitForStoryRender,
}) {
  await page.reload({ waitUntil: "networkidle" });
  await waitForStoryRender(page);
  if (page.url() !== currentUrl) {
    throw new Error(
      "Reload navigated away from the visual group editor story.",
    );
  }

  const restoredRegion = page.getByRole("region", {
    exact: true,
    name: GROUP_LABEL,
  });
  await restoredRegion.waitFor({ state: "visible" });
  const restoredNode = page.locator(
    `.react-flow__node[data-id="${TARGET_WORKSTATION_ID}"]`,
  );
  await restoredNode.waitFor({ state: "visible" });
  assertNodeSizeMatches(
    await readNodeSize(restoredNode),
    expectedNodeSize,
    "reloaded read-only",
  );
  await expectNoObsoleteNodeResizeActions(page);
  await expectInlineGroupLabel(restoredRegion);
  await expectRegionPointerThrough(page, restoredRegion);
  const intakeObserverButton = page.getByRole("button", {
    exact: true,
    name: `Select ${TARGET_WORKSTATION_LABEL} workstation`,
  });
  await intakeObserverButton.click();
  await expectAttribute(intakeObserverButton, "aria-pressed", "true");
  await page
    .locator("[data-factory-visual-group-controls]")
    .waitFor({ state: "hidden" });
  await captureEvidence(page, "visual-group-after-reload-observer");

  await page.setViewportSize({ height: 720, width: 768 });
  await page.reload({ waitUntil: "networkidle" });
  await waitForStoryRender(page);
  const constrainedRegion = page.getByRole("region", {
    exact: true,
    name: GROUP_LABEL,
  });
  await constrainedRegion.waitFor({ state: "visible" });
  await expectInlineGroupLabel(constrainedRegion);
  await expectRegionPointerThrough(page, constrainedRegion);

  const editModeAfterReload = page.getByRole("button", { name: "Edit mode" });
  await page.setViewportSize({ height: 900, width: 1440 });
  await page.reload({ waitUntil: "networkidle" });
  await waitForStoryRender(page);
  await editModeAfterReload.waitFor({ state: "visible" });
  await editModeAfterReload.click();
  await toolbar.waitFor({ state: "visible" });
  await expectGraphSurfaceBasics(page);
  const reloadedNode = page.locator(
    `.react-flow__node[data-id="${TARGET_WORKSTATION_ID}"]`,
  );
  await reloadedNode
    .getByRole("button", {
      exact: true,
      name: `Select ${TARGET_WORKSTATION_LABEL} workstation`,
    })
    .click();
  await reloadedNode.locator(".factory-graph-node-resize-grip").waitFor({
    state: "visible",
  });
  assertNodeSizeMatches(
    await readNodeSize(reloadedNode),
    expectedNodeSize,
    "reloaded editor",
  );
  await expectNoObsoleteNodeResizeActions(page);
  await page
    .getByRole("button", {
      exact: true,
      name: `Visual group ${GROUP_LABEL}`,
    })
    .waitFor({ state: "visible" });
  const reloadedGroup = page.getByRole("button", {
    exact: true,
    name: `Visual group ${GROUP_LABEL}`,
  });
  await reloadedGroup.focus();
  await page.keyboard.press("Enter");
  await page
    .locator("[data-factory-visual-group-controls]")
    .waitFor({ state: "visible" });
  await expectLabelField(page, GROUP_LABEL);
  const membershipCheckboxAfterReload = page.getByRole("checkbox", {
    name: `Include ${TARGET_WORKSTATION_LABEL} in this group`,
  });
  await expectChecked(membershipCheckboxAfterReload, true);
  await expectChecked(
    page.getByRole("checkbox", { name: "Include Implement in this group" }),
    true,
  );
  const customColorAfterReload = page.getByLabel("Custom group color");
  await expectInputValue(customColorAfterReload, CUSTOM_GROUP_COLOR);
  const warningColorAfterReload = page.getByRole("button", {
    exact: true,
    name: "Use warning group color",
  });
  await expectAttribute(warningColorAfterReload, "aria-pressed", "false");
  await expectPersistedEdgeWaypoint(page, reloadedGroup, expectedEdgeId);
  await expectEditorGraphInteractions(page, { skipEdgeDrag: true });
  await captureEvidence(page, "visual-group-after-reload-editor");
}

async function expectAttribute(locator, name, expected) {
  const actual = await locator.getAttribute(name);
  if (actual !== expected) {
    throw new Error(
      `Expected ${name}=${expected} but found ${actual ?? "missing"}.`,
    );
  }
}

async function readVisualGroupBounds(page) {
  const group = page.locator("[data-factory-visual-group]").first();
  await group.waitFor({ state: "visible" });
  const bounds = await group.boundingBox();
  if (!bounds) {
    throw new Error("Could not measure the visual group bounds.");
  }

  return bounds;
}

async function expectVisualGroupControlsWithinViewport(page, viewport) {
  const controls = page.locator("[data-factory-visual-group-controls]");
  const controlsBox = await controls.boundingBox();
  const viewportBox = await viewport.boundingBox();
  const scrollMetrics = await controls.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }));

  if (!controlsBox || !viewportBox) {
    throw new Error(
      "Could not measure the visual group controls and graph viewport.",
    );
  }

  const controlsBottom = controlsBox.y + controlsBox.height;
  const viewportBottom = viewportBox.y + viewportBox.height;
  if (
    controlsBox.y < viewportBox.y ||
    controlsBottom > viewportBottom ||
    controlsBox.x < viewportBox.x ||
    controlsBox.x + controlsBox.width > viewportBox.x + viewportBox.width
  ) {
    throw new Error(
      `Visual group controls exceed the graph viewport: ${JSON.stringify({
        controlsBox,
        viewportBox,
      })}`,
    );
  }

  if (scrollMetrics.scrollHeight <= scrollMetrics.clientHeight) {
    throw new Error(
      `Expected the visual group controls to expose vertical scrolling for the overflowing membership panel: ${JSON.stringify(
        scrollMetrics,
      )}`,
    );
  }
}

function expectBoundsChanged(before, after, operation) {
  const changed = ["height", "width", "x", "y"].some(
    (key) => Math.abs(before[key] - after[key]) > 1,
  );
  if (!changed) {
    throw new Error(
      `Expected visual group bounds to change after ${operation}: ${JSON.stringify(
        {
          after,
          before,
        },
      )}`,
    );
  }
}

function expectFiniteBounds(bounds, description) {
  if (
    !bounds ||
    ![bounds.height, bounds.width, bounds.x, bounds.y].every((value) =>
      Number.isFinite(value),
    ) ||
    bounds.height <= 0 ||
    bounds.width <= 0
  ) {
    throw new Error(
      `Expected ${description} bounds to be finite and positive.`,
    );
  }
}

async function resizeVisualGroup(page, corner, { deltaX, deltaY }) {
  const handle = page
    .locator(`[data-factory-visual-group-resize="${corner}"]`)
    .first();
  await handle.waitFor({ state: "visible" });
  const box = await handle.boundingBox();
  if (!box) {
    throw new Error(`Could not measure visual group resize handle ${corner}.`);
  }

  const boundsBeforePointerResize = await readVisualGroupBounds(page);
  const startX = box.x + box.width / 2;
  const startY = box.y + box.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY);
  await page.mouse.up();
  await page.waitForTimeout(100);

  const boundsAfterPointerResize = await readVisualGroupBounds(page);
  if (!areBoundsEqual(boundsBeforePointerResize, boundsAfterPointerResize)) {
    return;
  }

  await handle.focus();
  await pressResizeKey(handle, deltaX, "ArrowRight", "ArrowLeft");
  await pressResizeKey(handle, deltaY, "ArrowDown", "ArrowUp");
  await page.waitForTimeout(100);
}

async function pressResizeKey(locator, delta, positiveKey, negativeKey) {
  const key = delta >= 0 ? positiveKey : negativeKey;
  const presses = Math.max(1, Math.ceil(Math.abs(delta) / 16));
  for (let index = 0; index < presses; index += 1) {
    await locator.press(key);
  }
}

function areBoundsEqual(left, right) {
  return ["height", "width", "x", "y"].every(
    (key) => Math.abs(left[key] - right[key]) <= 1,
  );
}

async function expectLabelField(page, value) {
  const labelField = page.getByRole("textbox", { name: "Group label" });
  const actual = await labelField.inputValue();
  if (actual !== value) {
    throw new Error(`Expected group label "${value}" but found "${actual}".`);
  }
}

async function expectInputValue(locator, expected) {
  const actual = await locator.inputValue();
  if (actual !== expected) {
    throw new Error(
      `Expected input value "${expected}" but found "${actual}".`,
    );
  }
}

async function expectInlineGroupLabel(region) {
  const label = region.locator("[data-factory-graph-group-region-label]");
  await label.waitFor({ state: "visible" });

  const [regionBounds, labelBounds, presentation] = await Promise.all([
    region.boundingBox(),
    label.boundingBox(),
    label.evaluate((element) => {
      const wrapper = element.parentElement;
      const style = getComputedStyle(element);
      const wrapperStyle = wrapper ? getComputedStyle(wrapper) : null;
      return {
        backgroundColor: style.backgroundColor,
        boxShadow: style.boxShadow,
        className: element.className,
        hasBorder:
          style.borderWidth !== "0px" || wrapperStyle?.borderWidth !== "0px",
        maxWidth: wrapperStyle?.maxWidth ?? "none",
        transform: style.transform,
      };
    }),
  ]);

  if (!regionBounds || !labelBounds) {
    throw new Error("Could not measure the rendered visual group label.");
  }

  if (
    labelBounds.x < regionBounds.x ||
    labelBounds.y < regionBounds.y ||
    labelBounds.x >= regionBounds.x + regionBounds.width ||
    labelBounds.y >= regionBounds.y + regionBounds.height
  ) {
    throw new Error(
      `Expected the group label to start inside the region's top-left border: ${JSON.stringify({ labelBounds, regionBounds })}`,
    );
  }

  if (
    presentation.hasBorder ||
    presentation.backgroundColor !== "rgba(0, 0, 0, 0)" ||
    presentation.boxShadow !== "none" ||
    presentation.transform !== "none" ||
    presentation.maxWidth === "none" ||
    /border|rounded|shadow|backdrop/.test(presentation.className)
  ) {
    throw new Error(
      `Expected an unboxed, bounded inline group label, found ${JSON.stringify(presentation)}.`,
    );
  }
}

async function expectChecked(locator, checked) {
  const actual = await locator.isChecked();
  if (actual !== checked) {
    throw new Error(
      `Expected checkbox checked=${checked} but found ${actual}.`,
    );
  }
}
