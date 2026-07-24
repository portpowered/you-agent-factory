export const OVERFLOW_TOLERANCE_PX = 4;
export const STORY_RENDER_TIMEOUT_MS = 30_000;

export function createStoryUrl(baseUrl) {
  return (storyId) => `${baseUrl}/iframe.html?id=${storyId}&viewMode=story`;
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

export async function expectVisibleLabelWithinViewport(
  page,
  labelText,
  viewport,
) {
  const label = page.getByText(labelText, { exact: true });
  await label.waitFor({ state: "visible" });

  const box = await label.boundingBox();
  if (!box) {
    throw new Error(`Could not measure label bounds for "${labelText}".`);
  }

  const exceedsViewport =
    box.x < -OVERFLOW_TOLERANCE_PX ||
    box.y < -OVERFLOW_TOLERANCE_PX ||
    box.x + box.width > viewport.width + OVERFLOW_TOLERANCE_PX ||
    box.y + box.height > viewport.height + OVERFLOW_TOLERANCE_PX;

  if (exceedsViewport) {
    throw new Error(
      `Label "${labelText}" exceeded the ${viewport.label} viewport (${viewport.width}x${viewport.height}).`,
    );
  }
}

export async function expectTextLikeFocusRingVisible(page, selector, label) {
  const hasFocusRing = await page.evaluate((elementSelector) => {
    const element = document.querySelector(elementSelector);
    if (!(element instanceof HTMLElement)) {
      return false;
    }

    element.focus();
    const styles = window.getComputedStyle(element);
    const outlineWidth = Number.parseFloat(styles.outlineWidth || "0");
    const boxShadow = styles.boxShadow;
    return (
      outlineWidth > 0 ||
      (boxShadow !== "none" && boxShadow.length > 0) ||
      element.matches(":focus-visible")
    );
  }, selector);

  if (!hasFocusRing) {
    throw new Error(`Expected a visible focus treatment on ${label}.`);
  }
}

export async function expectCheckboxFocusRingVisible(page, labelText, label) {
  const checkbox = page.getByRole("checkbox", { name: labelText });
  await checkbox.focus();

  const hasFocusRing = await checkbox.evaluate((input) => {
    if (!(input instanceof HTMLInputElement)) {
      return false;
    }

    const indicator = input.nextElementSibling;
    if (!(indicator instanceof HTMLElement)) {
      return input.matches(":focus-visible");
    }

    const styles = window.getComputedStyle(indicator);
    const boxShadow = styles.boxShadow;
    return (
      input.matches(":focus-visible") ||
      (boxShadow !== "none" && boxShadow.length > 0)
    );
  });

  if (!hasFocusRing) {
    throw new Error(`Expected a visible focus treatment on ${label}.`);
  }
}
