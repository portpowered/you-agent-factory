const OVERFLOW_TOLERANCE_PX = 4;
const STORY_RENDER_TIMEOUT_MS = 30_000;

export const PACKAGE_SELECT_KEYBOARD_STORY_ID =
  "forms-packageselect--controlled-value";
export const PACKAGE_SELECT_FOCUS_STORY_ID = "forms-packageselect--focus";
export const PACKAGE_SELECT_STORY_LABEL = "Work type";

export const PACKAGE_SELECT_KEYBOARD_STORY_IDS = [
  PACKAGE_SELECT_KEYBOARD_STORY_ID,
  PACKAGE_SELECT_FOCUS_STORY_ID,
];

export const PACKAGE_SELECT_EMPTY_OPTIONS_STORY_ID =
  "forms-packageselect--empty-options";
export const PACKAGE_SELECT_LOADING_OPTIONS_STORY_ID =
  "forms-packageselect--loading-options";
export const PACKAGE_SELECT_ERROR_STATE_STORY_ID =
  "forms-packageselect--error-state";
export const PACKAGE_SELECT_LONG_LABEL_STORY_ID =
  "forms-packageselect--long-label";
export const PACKAGE_SELECT_LONG_LABEL_MOBILE_STORY_ID =
  "forms-packageselect--long-label-mobile-width";

export const PACKAGE_SELECT_EDGE_STATE_STORY_IDS = [
  PACKAGE_SELECT_EMPTY_OPTIONS_STORY_ID,
  PACKAGE_SELECT_LOADING_OPTIONS_STORY_ID,
  PACKAGE_SELECT_ERROR_STATE_STORY_ID,
  PACKAGE_SELECT_LONG_LABEL_STORY_ID,
  PACKAGE_SELECT_LONG_LABEL_MOBILE_STORY_ID,
];

export const PACKAGE_SELECT_RESPONSIVE_VIEWPORTS = [
  { height: 844, label: "mobile", width: 390 },
  { height: 900, label: "desktop", width: 1440 },
];

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

async function expectVisibleLabelWithinViewport(page, labelText, viewport) {
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

async function expectComboboxFocusRingVisible(page, labelText, label) {
  const trigger = page.getByRole("combobox", { name: labelText });
  await trigger.waitFor({
    state: "visible",
    timeout: STORY_RENDER_TIMEOUT_MS,
  });
  await trigger.focus();

  const hasFocusRing = await trigger.evaluate((element) => {
    if (!(element instanceof HTMLElement)) {
      return false;
    }

    const styles = window.getComputedStyle(element);
    const outlineWidth = Number.parseFloat(styles.outlineWidth || "0");
    const boxShadow = styles.boxShadow;
    return (
      outlineWidth > 0 ||
      (boxShadow !== "none" && boxShadow.length > 0) ||
      element.matches(":focus-visible")
    );
  });

  if (!hasFocusRing) {
    throw new Error(`Expected a visible focus treatment on ${label}.`);
  }
}

export async function verifyPackageSelectKeyboardStories({
  page,
  storyUrl,
  storyIds = PACKAGE_SELECT_KEYBOARD_STORY_IDS,
} = {}) {
  await page.setViewportSize({ height: 900, width: 1440 });

  for (const storyId of storyIds) {
    await page.goto(storyUrl(storyId), {
      timeout: 90_000,
      waitUntil: "networkidle",
    });
    await waitForStoryRender(page);

    if (storyId === PACKAGE_SELECT_KEYBOARD_STORY_ID) {
      const trigger = page.getByRole("combobox", {
        name: PACKAGE_SELECT_STORY_LABEL,
      });
      await trigger.waitFor({
        state: "visible",
        timeout: STORY_RENDER_TIMEOUT_MS,
      });
      await trigger.focus();
      await page.keyboard.press("ArrowDown");

      const listbox = page.getByRole("listbox");
      await listbox.waitFor({ state: "visible" });
      await page.getByRole("option", { name: "Story" }).waitFor({
        state: "visible",
      });

      await page.keyboard.press("Enter");
      await listbox.waitFor({ state: "hidden" });
      await trigger.waitFor({ state: "visible" });

      const selectedText = await trigger.textContent();
      if (!selectedText?.includes("Story")) {
        throw new Error(
          `Expected keyboard selection to update ${storyId}, got "${selectedText ?? ""}".`,
        );
      }

      const isFocused = await trigger.evaluate((element) => {
        return element === document.activeElement;
      });
      if (!isFocused) {
        throw new Error(
          `Expected focus to return to the select trigger after keyboard selection in ${storyId}.`,
        );
      }
      continue;
    }

    await expectComboboxFocusRingVisible(
      page,
      PACKAGE_SELECT_STORY_LABEL,
      storyId,
    );
  }
}

export async function verifyPackageSelectEdgeStateStories({
  page,
  storyUrl,
  storyIds = PACKAGE_SELECT_EDGE_STATE_STORY_IDS,
  viewports = PACKAGE_SELECT_RESPONSIVE_VIEWPORTS,
} = {}) {
  for (const viewport of viewports) {
    await page.setViewportSize({
      height: viewport.height,
      width: viewport.width,
    });

    for (const storyId of storyIds) {
      const useMobileViewportOnly =
        storyId === PACKAGE_SELECT_LONG_LABEL_MOBILE_STORY_ID &&
        viewport.label !== "mobile";
      const useDesktopViewportOnly =
        storyId === PACKAGE_SELECT_LONG_LABEL_STORY_ID &&
        viewport.label !== "desktop";
      if (useMobileViewportOnly || useDesktopViewportOnly) {
        continue;
      }

      await page.goto(storyUrl(storyId), {
        timeout: 90_000,
        waitUntil: "networkidle",
      });
      await waitForStoryRender(page);
      await expectNoHorizontalOverflow(page, `${storyId} (${viewport.label})`);
      await expectVisibleLabelWithinViewport(
        page,
        PACKAGE_SELECT_STORY_LABEL,
        viewport,
      );

      const trigger = page.getByRole("combobox", {
        name: PACKAGE_SELECT_STORY_LABEL,
      });
      await trigger.waitFor({
        state: "visible",
        timeout: STORY_RENDER_TIMEOUT_MS,
      });

      if (storyId === PACKAGE_SELECT_LOADING_OPTIONS_STORY_ID) {
        const loadingState = await trigger.evaluate((element) => ({
          ariaBusy: element.getAttribute("aria-busy"),
          disabled: element.hasAttribute("disabled"),
        }));
        if (loadingState.ariaBusy !== "true" || !loadingState.disabled) {
          throw new Error(
            `Expected ${storyId} to expose a disabled loading combobox.`,
          );
        }
        continue;
      }

      if (storyId === PACKAGE_SELECT_ERROR_STATE_STORY_ID) {
        const errorState = await trigger.evaluate((element) => ({
          ariaInvalid: element.getAttribute("aria-invalid"),
        }));
        if (errorState.ariaInvalid !== "true") {
          throw new Error(
            `Expected ${storyId} to expose aria-invalid on trigger.`,
          );
        }
        const alertText = await page.getByRole("alert").textContent();
        if (!alertText?.toLowerCase().includes("required")) {
          throw new Error(
            `Expected ${storyId} to render visible error alert text.`,
          );
        }
        continue;
      }

      if (storyId === PACKAGE_SELECT_EMPTY_OPTIONS_STORY_ID) {
        await trigger.click();
        const emptyOption = page.getByRole("option", {
          name: "No work types available",
        });
        await emptyOption.waitFor({ state: "visible" });
        const isDisabled = await emptyOption.evaluate((element) =>
          element.getAttribute("aria-disabled"),
        );
        if (isDisabled !== "true") {
          throw new Error(
            `Expected ${storyId} empty option to be aria-disabled.`,
          );
        }
        await page.keyboard.press("Escape");
        continue;
      }

      if (
        storyId === PACKAGE_SELECT_LONG_LABEL_STORY_ID ||
        storyId === PACKAGE_SELECT_LONG_LABEL_MOBILE_STORY_ID
      ) {
        const triggerBox = await trigger.boundingBox();
        if (!triggerBox) {
          throw new Error(`Could not measure trigger bounds for ${storyId}.`);
        }
        if (
          triggerBox.x + triggerBox.width >
          viewport.width + OVERFLOW_TOLERANCE_PX
        ) {
          throw new Error(
            `${storyId} trigger exceeded the ${viewport.label} viewport width.`,
          );
        }
      }
    }
  }
}
