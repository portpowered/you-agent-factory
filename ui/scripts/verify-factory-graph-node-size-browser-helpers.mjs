export async function resizeSelectedNode(page, node, nodeId) {
  const before = await readNodeSize(node);
  const resizeControl = node.locator(".factory-graph-node-resize-edge");
  await resizeControl.waitFor({ state: "visible" });
  await expectNoObsoleteNodeResizeActions(page);
  const controlBounds = await resizeControl.boundingBox();
  if (!controlBounds) {
    throw new Error(
      "Could not measure the selected node bottom-edge resize control.",
    );
  }

  const startX = controlBounds.x + controlBounds.width / 2;
  const startY = controlBounds.y + controlBounds.height / 2;
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + 72, startY + 44, { steps: 4 });
  await page.mouse.up();
  await page.waitForFunction(
    ({ id, width, height }) => {
      const node = document.querySelector(`.react-flow__node[data-id="${id}"]`);
      return Boolean(
        node &&
          Number.parseFloat(node.style.width) > width &&
          Number.parseFloat(node.style.height) > height,
      );
    },
    { id: nodeId, width: before.width, height: before.height },
  );

  const resized = await readNodeSize(node);
  if (resized.width <= before.width || resized.height <= before.height) {
    throw new Error(
      `Bottom-edge node resize did not increase both dimensions: before=${JSON.stringify(before)} after=${JSON.stringify(resized)}.`,
    );
  }

  const undoButton = page.getByRole("button", { name: "Undo" });
  await undoButton.click();
  await page.waitForFunction(
    ({ id, width, height }) => {
      const node = document.querySelector(`.react-flow__node[data-id="${id}"]`);
      return Boolean(
        node &&
          Math.abs(Number.parseFloat(node.style.width) - width) <= 1 &&
          Math.abs(Number.parseFloat(node.style.height) - height) <= 1,
      );
    },
    { id: nodeId, width: before.width, height: before.height },
  );
  assertNodeSizeMatches(await readNodeSize(node), before, "undo");

  await page.getByRole("button", { name: "Redo" }).click();
  await page.waitForFunction(
    ({ id, width, height }) => {
      const node = document.querySelector(`.react-flow__node[data-id="${id}"]`);
      return Boolean(
        node &&
          Math.abs(Number.parseFloat(node.style.width) - width) <= 1 &&
          Math.abs(Number.parseFloat(node.style.height) - height) <= 1,
      );
    },
    { id: nodeId, width: resized.width, height: resized.height },
  );
  assertNodeSizeMatches(await readNodeSize(node), resized, "redo");
  return resized;
}

export async function readNodeSize(node) {
  return node.evaluate((element) => ({
    height: Number.parseFloat(
      element.style.height || getComputedStyle(element).height,
    ),
    width: Number.parseFloat(
      element.style.width || getComputedStyle(element).width,
    ),
  }));
}

export function assertNodeSizeMatches(actual, expected, description) {
  if (
    !actual ||
    !expected ||
    Math.abs(actual.width - expected.width) > 1 ||
    Math.abs(actual.height - expected.height) > 1
  ) {
    throw new Error(
      `Node size differed after ${description}: expected=${JSON.stringify(expected)} actual=${JSON.stringify(actual)}.`,
    );
  }
}

export async function expectNoObsoleteNodeResizeActions(page) {
  if (
    (await page.getByRole("button", { name: "Fit to content" }).count()) > 0 ||
    (await page.getByRole("button", { name: "Reset size" }).count()) > 0
  ) {
    throw new Error(
      "Obsolete Fit to content or Reset size node actions rendered.",
    );
  }
}
