export const OVERFLOW_TOLERANCE_PX = 1;
export const STORY_RENDER_TIMEOUT_MS = 30000;

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

export async function expectVisible(locator, label) {
  if (typeof locator.waitFor === "function") {
    await locator.waitFor({
      state: "visible",
      timeout: STORY_RENDER_TIMEOUT_MS,
    });
  }

  if (!(await locator.isVisible())) {
    throw new Error(`${label} was not visible.`);
  }
}
