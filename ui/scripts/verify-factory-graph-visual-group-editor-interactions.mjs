import {
  waitForDurableCheckpoint,
  waitForStableBoundingBox,
  waitForStableFactoryGraphNodePlacement,
  waitForStableFactoryGraphViewport,
} from "../integration/browser-test-harness.mjs";

const GROUP_LABEL = "Planning lane";
const STABLE_NODE_TEST_ID = "rf__node-workstation:plan";

export async function expectGraphSurfaceBasics(page) {
  await waitForStableGraphSurface(page);

  const nodes = page.locator(".react-flow__node");
  const edges = page.locator(".react-flow__edge");
  const controls = page.locator(".react-flow__controls-button");
  await waitForDurableCheckpoint("factory graph surface basics", {
    "multiple graph nodes": async () => (await nodes.count()) >= 2,
    "graph edge": async () => (await edges.count()) >= 1,
    "graph controls": async () => (await controls.count()) >= 3,
  });
}

export async function expectRegionPointerThrough(page, region) {
  const point = await findPointInsideRegion(page, region, {
    requireCanvas: false,
  });
  const hit = await readHitTarget(page, point);
  if (hit.isGroupRegion) {
    throw new Error(
      `The restored group region intercepted its interior at (${point.x}, ${point.y}).`,
    );
  }
  await page.mouse.click(point.x, point.y);
}

export async function expectEditorGraphInteractions(page, options = {}) {
  await waitForStableGraphSurface(page);
  const group = page.locator("[data-factory-visual-group]").first();
  await group.waitFor({ state: "visible" });

  const handle = await findElementPointInsideRegion(
    page,
    group,
    page.locator('.react-flow__handle.source[role="button"]'),
  );
  const handleDisabled = await handle.locator.getAttribute("aria-disabled");
  if (handleDisabled === "true") {
    throw new Error("The source handle inside the visual group was disabled.");
  }
  await handle.locator.focus();
  await handle.locator.press("Enter");
  await expectAttribute(handle.locator, "aria-pressed", "true");
  await page.locator('[data-factory-graph-editor-canvas="true"]').focus();
  await page.keyboard.press("Escape");

  await expectSelectionBoxInsideRegion(page, group);
  await expectPanInsideRegion(page, group);
  await expectZoomInsideRegion(page, group);

  await expectDirectEdgeDrag(page, group, options);
}

export async function expectDirectEdgeDrag(page, region, options = {}) {
  const { edgeId: expectedEdgeId } = options;
  const { edge, point } = await findEdgeInsideRegion(
    page,
    region,
    expectedEdgeId,
  );
  const edgeId = await readEdgeId(edge);
  if (!edgeId) {
    throw new Error("Could not identify the graph edge selected for dragging.");
  }

  const beforeWaypointCount = await page
    .locator("[data-factory-edge-waypoint]")
    .count();
  const finalPoint = { x: point.x + 36, y: point.y + 28 };
  await page.mouse.move(point.x, point.y);
  await page.mouse.down();
  await page.mouse.move(finalPoint.x, finalPoint.y, { steps: 5 });
  await page.mouse.up();

  const waypoints = page.locator("[data-factory-edge-waypoint]");
  await waitForDurableCheckpoint("direct edge drag waypoint", async () => {
    return (await waypoints.count()) > beforeWaypointCount;
  });
  await page.locator("[data-factory-edge-waypoint-controls]").waitFor({
    state: "visible",
  });

  const waypoint = waypoints.last();
  await waitForStableBoundingBox(waypoint);
  const waypointBounds = await waypoint.boundingBox();
  if (!waypointBounds) {
    throw new Error(
      "Could not measure the waypoint created by direct edge drag.",
    );
  }
  const waypointCenter = {
    x: waypointBounds.x + waypointBounds.width / 2,
    y: waypointBounds.y + waypointBounds.height / 2,
  };
  if (
    Math.hypot(
      waypointCenter.x - finalPoint.x,
      waypointCenter.y - finalPoint.y,
    ) > 18
  ) {
    throw new Error(
      `The rendered edge waypoint did not follow the pointer: ${JSON.stringify({ finalPoint, waypointCenter })}.`,
    );
  }

  const undoButton = page.getByRole("button", { name: "Undo" });
  await waitForDurableCheckpoint("direct edge drag undo enabled", () =>
    undoButton.isEnabled(),
  );
  await undoButton.click();
  await waitForDurableCheckpoint("direct edge drag undone", async () => {
    return (await waypoints.count()) === beforeWaypointCount;
  });

  const redoButton = page.getByRole("button", { name: "Redo" });
  await waitForDurableCheckpoint("direct edge drag redo enabled", () =>
    redoButton.isEnabled(),
  );
  await redoButton.click();
  await waitForDurableCheckpoint("direct edge drag redone", async () => {
    return (await waypoints.count()) > beforeWaypointCount;
  });

  return edgeId;
}

export async function expectPersistedEdgeWaypoint(page, region, edgeId) {
  const { point } = await findEdgeInsideRegion(page, region, edgeId);
  await page.mouse.click(point.x, point.y);
  await page
    .locator("[data-factory-edge-waypoint-controls]")
    .waitFor({ state: "visible" });
  await page.locator("[data-factory-edge-waypoint]").first().waitFor({
    state: "visible",
  });
}

async function waitForStableGraphSurface(page) {
  await page.getByTestId(STABLE_NODE_TEST_ID).waitFor({ state: "visible" });
  await waitForStableFactoryGraphNodePlacement(page, STABLE_NODE_TEST_ID);
  await waitForStableFactoryGraphViewport(page);
}

async function expectSelectionBoxInsideRegion(page, region) {
  const start = await findPointInsideRegion(page, region, {
    requireCanvas: true,
  });
  const selection = page.locator(".react-flow__selection");
  await page.mouse.move(start.x, start.y);
  await page.mouse.down();
  await page.mouse.move(start.x + 40, start.y + 40, { steps: 4 });
  await selection.waitFor({ state: "visible" });
  await page.mouse.up();
  await selection.waitFor({ state: "hidden" });
  await page.keyboard.press("Escape");
}

async function expectPanInsideRegion(page, region) {
  const start = await findPointInsideRegion(page, region, {
    requireCanvas: true,
  });
  const before = await readViewportTransform(page);
  await page.keyboard.down("Space");
  await page.mouse.move(start.x, start.y);
  await page.mouse.down();
  await page.mouse.move(start.x + 32, start.y + 24, { steps: 4 });
  await page.mouse.up();
  await page.keyboard.up("Space");
  await expectViewportTransformChange(page, before, "pan");
}

async function expectZoomInsideRegion(page, region) {
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

export async function findPointInsideRegion(page, region, { requireCanvas }) {
  await waitForStableFactoryGraphViewport(page);
  await waitForStableBoundingBox(region);
  const bounds = await region.boundingBox();
  if (!bounds) {
    throw new Error(
      "Could not measure the visual group region for hit testing.",
    );
  }

  const candidates = [
    [0.5, 0.5],
    [0.35, 0.5],
    [0.65, 0.5],
    [0.5, 0.35],
    [0.5, 0.65],
    [0.25, 0.75],
    [0.75, 0.25],
  ];
  const viewport = await page.evaluate(() => ({
    height: window.innerHeight,
    width: window.innerWidth,
  }));
  let canvasBounds = null;
  if (requireCanvas) {
    const canvas = page.locator(".react-flow__pane");
    await waitForStableBoundingBox(canvas);
    canvasBounds = await canvas.boundingBox();
  }
  const candidateBounds = canvasBounds
    ? {
        x: Math.max(bounds.x, canvasBounds.x),
        y: Math.max(bounds.y, canvasBounds.y),
        width:
          Math.min(
            bounds.x + bounds.width,
            canvasBounds.x + canvasBounds.width,
          ) - Math.max(bounds.x, canvasBounds.x),
        height:
          Math.min(
            bounds.y + bounds.height,
            canvasBounds.y + canvasBounds.height,
          ) - Math.max(bounds.y, canvasBounds.y),
      }
    : bounds;
  if (candidateBounds.width <= 0 || candidateBounds.height <= 0) {
    throw new Error(
      `Could not find a visible canvas portion inside ${GROUP_LABEL}.`,
    );
  }
  for (const [xRatio, yRatio] of candidates) {
    const point = {
      x: candidateBounds.x + candidateBounds.width * xRatio,
      y: candidateBounds.y + candidateBounds.height * yRatio,
    };
    const hit = await readHitTarget(page, point);
    if (
      !hit.isGroupRegion &&
      (!requireCanvas || (hit.isCanvas && !hit.isEdge && !hit.isNode)) &&
      point.x >= 0 &&
      point.y >= 0 &&
      point.x <= viewport.width &&
      point.y <= viewport.height
    ) {
      return point;
    }
  }

  throw new Error(
    `Could not find a ${requireCanvas ? "canvas" : "hit-testable"} point inside ${GROUP_LABEL}.`,
  );
}

async function findElementPointInsideRegion(page, region, candidates) {
  await waitForStableFactoryGraphViewport(page);
  await waitForStableBoundingBox(region);
  const bounds = await region.boundingBox();
  if (!bounds) {
    throw new Error("Could not measure the visual group region.");
  }

  const count = await candidates.count();
  for (let index = 0; index < count; index += 1) {
    const locator = candidates.nth(index);
    if (!(await locator.isVisible())) {
      continue;
    }
    const elementBounds = await locator.boundingBox();
    if (!elementBounds) {
      continue;
    }
    const point = {
      x: elementBounds.x + elementBounds.width / 2,
      y: elementBounds.y + elementBounds.height / 2,
    };
    if (
      point.x < bounds.x ||
      point.x > bounds.x + bounds.width ||
      point.y < bounds.y ||
      point.y > bounds.y + bounds.height
    ) {
      continue;
    }
    const hit = await readHitTarget(page, point);
    if (!hit.isGroupRegion) {
      return { locator, point };
    }
  }

  throw new Error(
    `Could not find an interactive graph element inside ${GROUP_LABEL}.`,
  );
}

async function findEdgeInsideRegion(page, region, expectedEdgeId) {
  await waitForStableFactoryGraphViewport(page);
  await waitForStableBoundingBox(region);
  const bounds = await region.boundingBox();
  if (!bounds) {
    throw new Error(
      "Could not measure the visual group region for edge testing.",
    );
  }

  const edges = page.locator(".react-flow__edge");
  const edgeCount = await edges.count();
  let matchingEdgePointOutsideRegion = null;
  for (let index = 0; index < edgeCount; index += 1) {
    const edge = edges.nth(index);
    if (expectedEdgeId && (await readEdgeId(edge)) !== expectedEdgeId) {
      continue;
    }
    const points = await edge
      .evaluate((element) => {
        const path = element.querySelector(".react-flow__edge-path");
        if (!(path instanceof SVGGeometryElement)) {
          return [];
        }
        const transform = path.getScreenCTM();
        if (!transform) {
          return [];
        }
        const length = path.getTotalLength();
        return [0.2, 0.4, 0.6, 0.8].map((ratio) => {
          const local = path.getPointAtLength(length * ratio);
          return {
            x: local.x * transform.a + local.y * transform.c + transform.e,
            y: local.x * transform.b + local.y * transform.d + transform.f,
          };
        });
      })
      .catch(() => []);
    for (const point of points) {
      const insideRegion =
        point.x >= bounds.x &&
        point.x <= bounds.x + bounds.width &&
        point.y >= bounds.y &&
        point.y <= bounds.y + bounds.height;
      if (!insideRegion && !expectedEdgeId) {
        continue;
      }
      const hit = await readHitTarget(page, point);
      if (hit.isEdge && !hit.isGroupRegion) {
        if (insideRegion) {
          return { edge, point };
        }
        if (expectedEdgeId && !matchingEdgePointOutsideRegion) {
          matchingEdgePointOutsideRegion = { edge, point };
        }
      }
    }
  }

  if (matchingEdgePointOutsideRegion) {
    return matchingEdgePointOutsideRegion;
  }

  throw new Error(
    `Could not find a hit-testable graph edge inside ${GROUP_LABEL}.`,
  );
}

async function readEdgeId(edge) {
  return (
    (await edge.getAttribute("data-id")) ??
    (await edge.getAttribute("data-edge-id"))
  );
}

async function readHitTarget(page, point) {
  return page.evaluate(({ x, y }) => {
    const target = document.elementFromPoint(x, y);
    return {
      isCanvas: Boolean(target?.closest(".react-flow__pane")),
      isEdge: Boolean(target?.closest(".react-flow__edge")),
      isNode: Boolean(target?.closest(".react-flow__node")),
      isGroupRegion: Boolean(
        target?.closest(
          "[data-factory-visual-group], [data-factory-graph-group-region]",
        ),
      ),
    };
  }, point);
}

export async function readViewportTransform(page) {
  return page.locator(".react-flow__viewport").evaluate((element) => {
    const transform = getComputedStyle(element).transform;
    const values = transform.match(/matrix\(([^)]+)\)/)?.[1];
    return values ? values.split(",").map(Number) : null;
  });
}

export async function expectViewportTransformChange(page, before, operation) {
  await page
    .waitForFunction(
      (previous) => {
        const element = document.querySelector(".react-flow__viewport");
        if (!element) {
          return false;
        }
        const values = getComputedStyle(element)
          .transform.match(/matrix\(([^)]+)\)/)?.[1]
          ?.split(",")
          .map(Number);
        return Boolean(
          values &&
            previous &&
            values.some(
              (value, index) => Math.abs(value - previous[index]) > 0.5,
            ),
        );
      },
      before,
      { timeout: 5_000 },
    )
    .catch((error) => {
      throw new Error(
        `Expected the graph ${operation} gesture to change the viewport.`,
        {
          cause: error,
        },
      );
    });
}

async function expectAttribute(locator, name, expected) {
  const actual = await locator.getAttribute(name);
  if (actual !== expected) {
    throw new Error(
      `Expected ${name}=${expected} but found ${actual ?? "missing"}.`,
    );
  }
}
