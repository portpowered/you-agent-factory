export const OVERFLOW_TOLERANCE_PX = 1;
export const STORY_RENDER_TIMEOUT_MS = 30000;

export function storyUrl(storybookUrl, storyId) {
  return `${storybookUrl}/iframe.html?id=${storyId}&viewMode=story`;
}

export async function waitForStoryRender(page) {
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

export async function waitForDialog(page, dialogName) {
  const dialog = page.getByRole("dialog", { name: dialogName });
  await dialog.waitFor({ state: "visible" });
  return dialog;
}

export async function waitForStoryRegion(page, regionName) {
  const region = page.getByRole("region", { name: regionName });
  await region.waitFor({ state: "visible" });
  return region;
}

export async function expectNoHorizontalOverflow(page, label) {
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

export async function expectDialogWithinViewport(dialog, viewport, label) {
  const box = await dialog.boundingBox();
  if (!box) {
    throw new Error(`Could not measure ${label} dialog bounds.`);
  }
  const exceedsViewport =
    box.x < -OVERFLOW_TOLERANCE_PX ||
    box.y < -OVERFLOW_TOLERANCE_PX ||
    box.x + box.width > viewport.width + OVERFLOW_TOLERANCE_PX ||
    box.y + box.height > viewport.height + OVERFLOW_TOLERANCE_PX;

  if (exceedsViewport) {
    throw new Error(
      `${label} dialog exceeded the ${viewport.label} viewport (${viewport.width}x${viewport.height}).`,
    );
  }
}

export async function expectVisible(locator, label) {
  if (typeof locator.waitFor === "function") {
    await locator.waitFor({
      state: "visible",
      timeout: STORY_RENDER_TIMEOUT_MS,
    });
    return;
  }

  if (!(await locator.isVisible())) {
    throw new Error(`${label} was not visible.`);
  }
}
