import {
  waitForDurableCheckpoint,
  waitForStableBoundingBox,
  waitForStableFactoryGraphViewport,
} from "../integration/browser-test-harness.mjs";
import {
  expectViewportTransformChange,
  findPointInsideRegion,
  readViewportTransform,
} from "./verify-factory-graph-visual-group-editor-interactions.mjs";

export async function expectVisualGroupDragWithMembers(
  page,
  { memberNodeIds, unrelatedNodeId, deltaX = 24, deltaY = 16 },
) {
  const group = page.locator("[data-factory-visual-group]").first();
  const members = memberNodeIds.map((nodeId) => graphNodeLocator(page, nodeId));
  const unrelated = graphNodeLocator(page, unrelatedNodeId);

  await waitForStableGroupGeometry(page, group, [...members, unrelated]);
  const before = await readGroupGeometry(group, members, unrelated);

  await dragVisualGroup(page, { deltaX, deltaY });
  await waitForStableGroupGeometry(page, group, [...members, unrelated]);
  const moved = await readGroupGeometry(group, members, unrelated);
  expectGroupAndMembersMoveTogether(before, moved, "normal zoom");

  const undoButton = page.getByRole("button", { name: "Undo" });
  await undoButton.waitFor({ state: "visible" });
  await waitForDurableCheckpoint("visual group undo enabled", () =>
    undoButton.isEnabled(),
  );
  await undoButton.click();
  await waitForStableGroupGeometry(page, group, [...members, unrelated]);
  expectGeometryRestored(
    before,
    await readGroupGeometry(group, members, unrelated),
    "undo",
  );

  const redoButton = page.getByRole("button", { name: "Redo" });
  await redoButton.waitFor({ state: "visible" });
  await waitForDurableCheckpoint("visual group redo enabled", () =>
    redoButton.isEnabled(),
  );
  await redoButton.click();
  await waitForStableGroupGeometry(page, group, [...members, unrelated]);
  expectGeometryRestored(
    moved,
    await readGroupGeometry(group, members, unrelated),
    "redo",
  );

  await zoomGraphAtRegion(page, group);
  await waitForStableGroupGeometry(page, group, [...members, unrelated]);
  const beforeZoomed = await readGroupGeometry(group, members, unrelated);
  await dragVisualGroup(page, { deltaX, deltaY });
  await waitForStableGroupGeometry(page, group, [...members, unrelated]);
  const movedZoomed = await readGroupGeometry(group, members, unrelated);
  expectGroupAndMembersMoveTogether(beforeZoomed, movedZoomed, "zoomed");
}

export async function dragVisualGroup(page, { deltaX, deltaY }) {
  const { box } = await findVisibleGroupDragSurface(page);

  const startX = box.x + box.width / 2;
  const startY = box.y + box.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY, { steps: 4 });
  await page.mouse.up();
}

async function findVisibleGroupDragSurface(page) {
  const candidates = [
    page.locator("[data-factory-visual-group-label]").first(),
    page.locator('[data-factory-visual-group-outline="bottom"]').first(),
    page.locator('[data-factory-visual-group-outline="left"]').first(),
    page.locator('[data-factory-visual-group-outline="right"]').first(),
    page.locator('[data-factory-visual-group-outline="top"]').first(),
  ];
  const viewport = await page.evaluate(() => ({
    height: window.innerHeight,
    width: window.innerWidth,
  }));

  for (const locator of candidates) {
    if (!(await locator.isVisible())) {
      continue;
    }
    const box = await locator.boundingBox();
    if (!box) {
      continue;
    }
    const centerX = box.x + box.width / 2;
    const centerY = box.y + box.height / 2;
    if (
      centerX >= 0 &&
      centerX <= viewport.width &&
      centerY >= 0 &&
      centerY <= viewport.height
    ) {
      const hitIsGroupSurface = await page.evaluate(
        ({ x, y }) => {
          const target = document.elementFromPoint(x, y);
          return Boolean(
            target?.closest(
              "[data-factory-visual-group-label], [data-factory-visual-group-outline]",
            ),
          );
        },
        { x: centerX, y: centerY },
      );
      if (hitIsGroupSurface) {
        return { box, locator };
      }
    }
  }

  throw new Error("Could not find a visible visual group drag surface.");
}

function graphNodeLocator(page, nodeId) {
  return page.locator(`.react-flow__node[data-id="${nodeId}"]`);
}

async function waitForStableGroupGeometry(page, group, nodes) {
  await waitForStableFactoryGraphViewport(page);
  await Promise.all([
    waitForStableBoundingBox(group),
    ...nodes.map((node) => waitForStableBoundingBox(node)),
  ]);
}

async function readGroupGeometry(group, members, unrelated) {
  const boxes = await Promise.all([
    group.boundingBox(),
    ...members.map((member) => member.boundingBox()),
    unrelated.boundingBox(),
  ]);
  if (boxes.some((box) => box === null)) {
    throw new Error("Could not measure the visual group drag geometry.");
  }

  return {
    group: boxes[0],
    members: boxes.slice(1, -1),
    unrelated: boxes.at(-1),
  };
}

function expectGroupAndMembersMoveTogether(before, after, transform) {
  const groupDelta = {
    x: after.group.x - before.group.x,
    y: after.group.y - before.group.y,
  };
  if (Math.hypot(groupDelta.x, groupDelta.y) < 2) {
    throw new Error(
      `Expected the visual group to move at ${transform} transform: ${JSON.stringify({ after, before })}`,
    );
  }

  for (const [index, member] of after.members.entries()) {
    const beforeMember = before.members[index];
    const memberDelta = {
      x: member.x - beforeMember.x,
      y: member.y - beforeMember.y,
    };
    if (
      Math.abs(memberDelta.x - groupDelta.x) > 2 ||
      Math.abs(memberDelta.y - groupDelta.y) > 2
    ) {
      throw new Error(
        `Expected member ${index} to follow the group at ${transform} transform: ${JSON.stringify({ groupDelta, memberDelta })}`,
      );
    }
  }

  if (
    Math.abs(after.unrelated.x - before.unrelated.x) > 2 ||
    Math.abs(after.unrelated.y - before.unrelated.y) > 2
  ) {
    throw new Error(
      `Expected the unrelated node to remain stable at ${transform} transform: ${JSON.stringify({ after: after.unrelated, before: before.unrelated })}`,
    );
  }
}

function expectGeometryRestored(expected, actual, operation) {
  const boxes = [expected.group, ...expected.members, expected.unrelated];
  const actualBoxes = [actual.group, ...actual.members, actual.unrelated];
  for (const [index, expectedBox] of boxes.entries()) {
    const actualBox = actualBoxes[index];
    if (
      Math.abs(actualBox.x - expectedBox.x) > 2 ||
      Math.abs(actualBox.y - expectedBox.y) > 2 ||
      Math.abs(actualBox.width - expectedBox.width) > 2 ||
      Math.abs(actualBox.height - expectedBox.height) > 2
    ) {
      throw new Error(
        `Expected ${operation} to restore graph geometry: ${JSON.stringify({ actual, expected })}`,
      );
    }
  }
}

async function zoomGraphAtRegion(page, region) {
  const point = await findPointInsideRegion(page, region, {
    requireCanvas: true,
  });
  const before = await readViewportTransform(page);
  await page.keyboard.down("Control");
  await page.mouse.move(point.x, point.y);
  await page.mouse.wheel(0, -240);
  await page.keyboard.up("Control");
  await expectViewportTransformChange(page, before, "zoom");
}
