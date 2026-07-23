export const overlayStoryIds = [
  "overlays-dialog--default",
  "overlays-dialog--long-content",
  "overlays-dialog--controlled-open",
  "overlays-dialog--escape-close",
  "overlays-dialog--mobile-viewport",
  "overlays-popover--default",
  "overlays-popover--long-content",
  "overlays-popover--controlled-open",
  "overlays-popover--keyboard-open",
  "overlays-popover--viewport-placement",
  "overlays-collapsible--default",
  "overlays-collapsible--open",
  "overlays-collapsible--controlled",
  "overlays-collapsible--nested-content",
  "overlays-scrollarea--default",
  "overlays-scrollarea--horizontal-overflow",
  "overlays-scrollarea--mobile-width",
  "overlays-dialog--keyboard-focus",
  "overlays-popover--keyboard-focus",
  "overlays-collapsible--keyboard-focus",
  "overlays-scrollarea--keyboard-focus",
];

export const overlayStoryTexts = {
  "overlays-dialog--default": "Package dialog",
  "overlays-dialog--long-content":
    "Dialog long content paragraph twenty for overflow review.",
  "overlays-dialog--controlled-open":
    "Controlled dialog state is managed by the host application.",
  "overlays-dialog--escape-close":
    "Press Escape to close this dialog and return focus to the trigger.",
  "overlays-dialog--mobile-viewport":
    "Mobile dialog content remains reachable without horizontal page overflow.",
  "overlays-popover--default": "Popover content from the component package",
  "overlays-popover--long-content":
    "Popover long content paragraph twenty for overflow review.",
  "overlays-popover--controlled-open":
    "Controlled popover state is managed by the host application.",
  "overlays-popover--keyboard-open":
    "Popover opened from the keyboard trigger.",
  "overlays-popover--viewport-placement":
    "Popover placement stays within the viewport near screen edges.",
  "overlays-collapsible--default":
    "Collapsible content rendered from the package overlays category",
  "overlays-collapsible--open":
    "Collapsible open state is visible to assistive technology.",
  "overlays-collapsible--controlled":
    "Controlled collapsible state is managed by the host application.",
  "overlays-collapsible--nested-content":
    "Nested collapsible inner section for disclosure review.",
  "overlays-scrollarea--default": "Scrollable row 1",
  "overlays-scrollarea--horizontal-overflow":
    "Wide horizontal scroll item twenty for overflow review.",
  "overlays-scrollarea--mobile-width":
    "Mobile scroll area content remains reachable at narrow widths.",
  "overlays-dialog--keyboard-focus": "Package dialog",
  "overlays-popover--keyboard-focus":
    "Popover content from the component package",
  "overlays-collapsible--keyboard-focus":
    "Collapsible content rendered from the package overlays category",
  "overlays-scrollarea--keyboard-focus": "Scrollable field",
};

export const hostResponsibilityCopy =
  "Host apps supply accessible labels, trigger text, and overflow-safe content without dashboard providers.";

const overlayKeyboardStoryIds = [
  "overlays-dialog--keyboard-focus",
  "overlays-popover--keyboard-focus",
  "overlays-collapsible--keyboard-focus",
  "overlays-scrollarea--keyboard-focus",
];

const overlayViewportStoryChecks = [
  {
    storyId: "overlays-dialog--mobile-viewport",
    desktop: { width: 1280, height: 800 },
    mobile: { width: 320, height: 568 },
    triggerName: "Open mobile dialog",
    dialogName: "Mobile dialog",
  },
  {
    storyId: "overlays-popover--viewport-placement",
    desktop: { width: 1280, height: 800 },
    mobile: { width: 320, height: 568 },
    triggerName: "Edge popover trigger",
    popoverText:
      "Popover placement stays within the viewport near screen edges.",
  },
  {
    storyId: "overlays-scrollarea--mobile-width",
    desktop: { width: 1280, height: 800 },
    mobile: { width: 320, height: 568 },
    scrollAnchor:
      "Mobile scroll area content remains reachable at narrow widths.",
  },
];

async function assertNoHorizontalPageOverflow(page, storyId, viewportLabel) {
  const metrics = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));

  if (metrics.scrollWidth > metrics.clientWidth + 1) {
    throw new Error(
      `Expected ${storyId} at ${viewportLabel} to avoid horizontal page overflow (scrollWidth=${metrics.scrollWidth}, clientWidth=${metrics.clientWidth}).`,
    );
  }
}

async function verifyOverlayViewportStory(
  page,
  baseUrl,
  check,
  viewport,
  viewportLabel,
) {
  await page.setViewportSize(viewport);
  const iframeUrl = `${baseUrl}/iframe.html?id=${check.storyId}&viewMode=story`;
  await page.goto(iframeUrl, {
    waitUntil: "domcontentloaded",
    timeout: 30_000,
  });
  await page.waitForSelector("#storybook-root", { timeout: 10_000 });
  const storyRoot = page.locator("#storybook-root");

  if (check.triggerName && check.dialogName) {
    const trigger = storyRoot.getByRole("button", { name: check.triggerName });
    await trigger.click();
    const dialog = page.getByRole("dialog", { name: check.dialogName });
    await dialog.waitFor({ state: "visible", timeout: 10_000 });
    await assertNoHorizontalPageOverflow(page, check.storyId, viewportLabel);
    await page.keyboard.press("Escape");
    await dialog.waitFor({ state: "hidden", timeout: 10_000 });
    return;
  }

  if (check.triggerName && check.popoverText) {
    const trigger = storyRoot.getByRole("button", { name: check.triggerName });
    await trigger.click();
    await page.getByText(check.popoverText).waitFor({
      state: "visible",
      timeout: 10_000,
    });
    await assertNoHorizontalPageOverflow(page, check.storyId, viewportLabel);
    await page.keyboard.press("Escape");
    return;
  }

  if (check.scrollAnchor) {
    await storyRoot.getByText(check.scrollAnchor).waitFor({
      state: "visible",
      timeout: 10_000,
    });
    await assertNoHorizontalPageOverflow(page, check.storyId, viewportLabel);
    return;
  }

  throw new Error(`No viewport verification handler for ${check.storyId}.`);
}

async function verifyOverlayKeyboardStory(page, baseUrl, storyId) {
  const iframeUrl = `${baseUrl}/iframe.html?id=${storyId}&viewMode=story`;
  await page.goto(iframeUrl, {
    waitUntil: "domcontentloaded",
    timeout: 30_000,
  });
  await page.waitForSelector("#storybook-root", { timeout: 10_000 });
  const storyRoot = page.locator("#storybook-root");

  switch (storyId) {
    case "overlays-dialog--keyboard-focus": {
      const trigger = storyRoot.getByRole("button", { name: "Open dialog" });
      await trigger.click();
      const dialog = page.getByRole("dialog", { name: "Package dialog" });
      await dialog.waitFor({ state: "visible", timeout: 10_000 });
      await page.getByRole("button", { name: "Close" }).waitFor({
        state: "visible",
        timeout: 10_000,
      });
      await page.keyboard.press("Escape");
      await dialog.waitFor({ state: "hidden", timeout: 10_000 });
      await trigger.focus();
      break;
    }
    case "overlays-popover--keyboard-focus": {
      const trigger = storyRoot.getByRole("button", { name: "Open popover" });
      await trigger.click();
      await page
        .getByText("Popover content from the component package.")
        .waitFor({ state: "visible", timeout: 10_000 });
      await page.keyboard.press("Escape");
      await page
        .getByText("Popover content from the component package.")
        .waitFor({ state: "hidden", timeout: 10_000 });
      break;
    }
    case "overlays-collapsible--keyboard-focus": {
      const trigger = storyRoot.getByRole("button", { name: "Toggle details" });
      await trigger.focus();
      await page.keyboard.press("Enter");
      await storyRoot
        .getByText(
          "Collapsible content rendered from the package overlays category.",
        )
        .waitFor({ state: "visible", timeout: 10_000 });
      break;
    }
    case "overlays-scrollarea--keyboard-focus": {
      const field = storyRoot.getByRole("textbox", {
        name: "Scrollable field",
      });
      await field.waitFor({ state: "visible", timeout: 10_000 });
      await field.focus();
      const focused = await field.evaluate(
        (element) => element === document.activeElement,
      );
      if (!focused) {
        throw new Error(
          `Expected ScrollArea keyboard story field to receive focus for ${storyId}.`,
        );
      }
      break;
    }
    default:
      throw new Error(`No keyboard verification handler for ${storyId}.`);
  }
}

export async function verifyPackageOverlayStories({ page, baseUrl }) {
  for (const storyId of overlayKeyboardStoryIds) {
    await verifyOverlayKeyboardStory(page, baseUrl, storyId);
    console.log(`Verified package Storybook keyboard behavior for ${storyId}.`);
  }

  for (const check of overlayViewportStoryChecks) {
    for (const viewportLabel of ["desktop", "mobile"]) {
      const viewport = check[viewportLabel];
      await verifyOverlayViewportStory(
        page,
        baseUrl,
        check,
        viewport,
        viewportLabel,
      );
      console.log(
        `Verified package Storybook viewport behavior for ${check.storyId} at ${viewportLabel} (${viewport.width}x${viewport.height}).`,
      );
    }
  }
}
