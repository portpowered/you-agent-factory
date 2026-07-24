const OVERFLOW_TOLERANCE_PX = 4;
const STORY_RENDER_TIMEOUT_MS = 30_000;

export const PACKAGE_DATATABLE_STORY_IDS = [
  "data-display-datatable--success",
  "data-display-datatable--loading",
  "data-display-datatable--empty",
  "data-display-datatable--error-state",
  "data-display-datatable--dense",
  "data-display-datatable--long-cell",
  "data-display-datatable--narrow-viewport",
];

const PACKAGE_DATATABLE_NARROW_VIEWPORT = { height: 844, width: 390 };

async function waitForStoryRender(page) {
  await page.waitForSelector("#storybook-root", {
    state: "attached",
    timeout: STORY_RENDER_TIMEOUT_MS,
  });
  await page.waitForFunction(
    () => {
      const root = document.querySelector("#storybook-root");
      if (!(root instanceof HTMLElement)) {
        return false;
      }
      if (root.childElementCount > 0) {
        return true;
      }
      return Array.from(document.body.children).some((child) => {
        if (!(child instanceof HTMLElement)) {
          return false;
        }
        if (
          child.id === "storybook-root" ||
          child.id === "storybook-docs" ||
          child.tagName === "SCRIPT" ||
          child.tagName === "STYLE"
        ) {
          return false;
        }

        return true;
      });
    },
    { timeout: STORY_RENDER_TIMEOUT_MS },
  );
}

async function expectNoHorizontalOverflow(page, label) {
  const metrics = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));

  if (metrics.scrollWidth > metrics.clientWidth + OVERFLOW_TOLERANCE_PX) {
    throw new Error(
      `${label} overflowed horizontally: scrollWidth=${metrics.scrollWidth}, clientWidth=${metrics.clientWidth}.`,
    );
  }
}

async function expectVisibleTextWithinViewport(page, text, viewport) {
  const node = page.getByText(text, { exact: false });
  await node.first().waitFor({ state: "visible" });

  const box = await node.first().boundingBox();
  if (!box) {
    throw new Error(`Could not measure text bounds for "${text}".`);
  }

  const exceedsViewport =
    box.x < -OVERFLOW_TOLERANCE_PX ||
    box.y < -OVERFLOW_TOLERANCE_PX ||
    box.x + box.width > viewport.width + OVERFLOW_TOLERANCE_PX ||
    box.y + box.height > viewport.height + OVERFLOW_TOLERANCE_PX;

  if (exceedsViewport) {
    throw new Error(
      `Text "${text}" exceeded the ${viewport.label} viewport (${viewport.width}x${viewport.height}).`,
    );
  }
}

async function verifySuccessStory(page, storyId) {
  const table = page.getByRole("table", { name: "Product catalog" });
  await table.waitFor({ state: "visible", timeout: STORY_RENDER_TIMEOUT_MS });
  await page.getByRole("columnheader", { name: "Product" }).waitFor({
    state: "visible",
  });
  await page.getByRole("cell", { name: "Signal Router" }).waitFor({
    state: "visible",
  });
  await page.getByRole("cell", { name: "Review Queue" }).waitFor({
    state: "visible",
  });
  console.log(`Verified DataTable success DOM for ${storyId}.`);
}

async function verifyLoadingStory(page, storyId) {
  const status = page.getByRole("status");
  await status.waitFor({ state: "visible", timeout: STORY_RENDER_TIMEOUT_MS });
  await page.getByText("Loading product catalog", { exact: true }).waitFor({
    state: "visible",
  });

  const loadingState = await status.evaluate((element) => ({
    ariaBusy: element.getAttribute("aria-busy"),
  }));
  if (loadingState.ariaBusy !== "true") {
    throw new Error(`Expected ${storyId} loading status to expose aria-busy.`);
  }

  const staleRowCount = await page
    .getByRole("cell", { name: "Signal Router" })
    .count();
  if (staleRowCount > 0) {
    throw new Error(
      `Expected ${storyId} to hide stale row data while loading.`,
    );
  }

  console.log(`Verified DataTable loading DOM for ${storyId}.`);
}

async function verifyEmptyStory(page, storyId) {
  const status = page.getByRole("status");
  await status.waitFor({ state: "visible", timeout: STORY_RENDER_TIMEOUT_MS });
  await page
    .getByText("No products match the current filters", { exact: true })
    .waitFor({ state: "visible" });

  const staleRowCount = await page
    .getByRole("cell", { name: "Signal Router" })
    .count();
  if (staleRowCount > 0) {
    throw new Error(`Expected ${storyId} to hide row data in empty state.`);
  }

  console.log(`Verified DataTable empty DOM for ${storyId}.`);
}

async function verifyErrorStory(page, storyId) {
  const alert = page.getByRole("alert");
  await alert.waitFor({ state: "visible", timeout: STORY_RENDER_TIMEOUT_MS });
  await page
    .getByText("Unable to load product catalog data", { exact: true })
    .waitFor({ state: "visible" });

  const staleRowCount = await page
    .getByRole("cell", { name: "Signal Router" })
    .count();
  if (staleRowCount > 0) {
    throw new Error(`Expected ${storyId} to hide row data in error state.`);
  }

  console.log(`Verified DataTable error DOM for ${storyId}.`);
}

async function verifyDenseStory(page, storyId) {
  await page.getByRole("cell", { name: "Queue depth" }).waitFor({
    state: "visible",
    timeout: STORY_RENDER_TIMEOUT_MS,
  });
  await page.getByRole("button", { name: "Inspect" }).waitFor({
    state: "visible",
  });

  const denseContainer = page.locator('[data-size="dense"]');
  await denseContainer.waitFor({ state: "visible" });
  if ((await denseContainer.count()) < 1) {
    throw new Error(`Expected ${storyId} to render a dense table container.`);
  }

  console.log(`Verified DataTable dense DOM for ${storyId}.`);
}

async function verifyLongCellStory(page, storyId) {
  const longCopy =
    "Provider session emitted a long diagnostic payload describing retry policy, guard evaluation order, and downstream workstation routing without forcing the surrounding page to scroll horizontally.";
  await page.getByText(longCopy, { exact: false }).waitFor({
    state: "visible",
    timeout: STORY_RENDER_TIMEOUT_MS,
  });
  await expectNoHorizontalOverflow(page, storyId);

  console.log(`Verified DataTable long-cell DOM for ${storyId}.`);
}

async function verifyNarrowViewportStory(page, storyId) {
  const viewport = {
    ...PACKAGE_DATATABLE_NARROW_VIEWPORT,
    label: "mobile",
  };
  await page.setViewportSize({
    height: viewport.height,
    width: viewport.width,
  });
  await page.getByRole("cell", { name: "Signal Router" }).waitFor({
    state: "visible",
    timeout: STORY_RENDER_TIMEOUT_MS,
  });
  await expectNoHorizontalOverflow(page, `${storyId} (${viewport.label})`);
  await expectVisibleTextWithinViewport(page, "Signal Router", viewport);

  console.log(`Verified DataTable narrow viewport DOM for ${storyId}.`);
}

export async function verifyPackageDataTableStories({
  page,
  storyUrl,
  storyIds = PACKAGE_DATATABLE_STORY_IDS,
} = {}) {
  await page.setViewportSize({ height: 900, width: 1440 });

  for (const storyId of storyIds) {
    await page.goto(storyUrl(storyId), {
      timeout: 90_000,
      waitUntil: "networkidle",
    });
    await waitForStoryRender(page);

    if (storyId === "data-display-datatable--success") {
      await verifySuccessStory(page, storyId);
      continue;
    }
    if (storyId === "data-display-datatable--loading") {
      await verifyLoadingStory(page, storyId);
      continue;
    }
    if (storyId === "data-display-datatable--empty") {
      await verifyEmptyStory(page, storyId);
      continue;
    }
    if (storyId === "data-display-datatable--error-state") {
      await verifyErrorStory(page, storyId);
      continue;
    }
    if (storyId === "data-display-datatable--dense") {
      await verifyDenseStory(page, storyId);
      continue;
    }
    if (storyId === "data-display-datatable--long-cell") {
      await verifyLongCellStory(page, storyId);
      continue;
    }
    if (storyId === "data-display-datatable--narrow-viewport") {
      await verifyNarrowViewportStory(page, storyId);
    }
  }
}
