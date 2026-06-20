export const STYLED_SCROLLBAR_MARKER = "af-styled-scrollbar";

export const TEXTAREA_SCROLL_CONSTRAINT_MARKERS = [
  "min-h-28",
  "max-h-52",
  "overflow-y-auto",
  STYLED_SCROLLBAR_MARKER,
];

export async function readElementClassName(locator, label) {
  await locator.waitFor({ state: "visible" });
  const className = await locator.evaluate((element) => element.className);
  if (typeof className !== "string" || className.length === 0) {
    throw new Error(`Could not read className for ${label}.`);
  }

  return className;
}

export async function assertSubmitWorkTextareaScrollTreatment(
  textareaLocator,
  label,
) {
  const className = await readElementClassName(textareaLocator, label);
  for (const marker of TEXTAREA_SCROLL_CONSTRAINT_MARKERS) {
    if (!className.includes(marker)) {
      throw new Error(`${label}: textarea is missing ${marker} styling.`);
    }
  }
}

export async function assertSubmitWorkTextareaOverflows(textareaLocator, label) {
  const metrics = await textareaLocator.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }));

  if (metrics.scrollHeight <= metrics.clientHeight) {
    throw new Error(
      `${label}: expected overflowing textarea content (scrollHeight=${metrics.scrollHeight}, clientHeight=${metrics.clientHeight}).`,
    );
  }
}

export async function assertElementFocused(locator, label) {
  const focused = await locator.evaluate(
    (element) => element === document.activeElement,
  );
  if (!focused) {
    throw new Error(`${label} was not focused.`);
  }
}

export async function tabUntilFocused(page, locator, maxTabs, label) {
  for (let attempt = 0; attempt < maxTabs; attempt += 1) {
    if (await locator.evaluate((element) => element === document.activeElement)) {
      return;
    }

    await page.keyboard.press("Tab");
  }

  throw new Error(`Could not tab to ${label} within ${maxTabs} attempts.`);
}

export async function scrollOverflowingTextareaWithKeyboard(
  page,
  textareaLocator,
  viewport,
) {
  await page.keyboard.press("Control+Home");
  const scrollTopBefore = await textareaLocator.evaluate(
    (element) => element.scrollTop,
  );

  await page.keyboard.press("PageDown");
  let scrollTopAfter = await textareaLocator.evaluate(
    (element) => element.scrollTop,
  );

  if (scrollTopAfter <= scrollTopBefore) {
    for (let attempt = 0; attempt < 24; attempt += 1) {
      await page.keyboard.press("ArrowDown");
    }
    scrollTopAfter = await textareaLocator.evaluate(
      (element) => element.scrollTop,
    );
  }

  if (scrollTopAfter <= scrollTopBefore) {
    throw new Error(
      `Keyboard input did not scroll overflowing textarea content at ${viewport.label}.`,
    );
  }
}

export async function verifyKeyboardTextareaNavigation(page, card, viewport) {
  const submissionTextarea = card.getByRole("textbox", { name: "Text item 1" });
  const submitButton = card.getByRole("button", { name: "Submit work" });

  await card.click({ position: { x: 8, y: 8 } });
  await tabUntilFocused(
    page,
    submissionTextarea,
    8,
    `submission textarea (${viewport.label})`,
  );
  await assertElementFocused(
    submissionTextarea,
    `submission textarea (${viewport.label})`,
  );

  const selectionBefore = await submissionTextarea.evaluate(
    (element) => element.selectionStart,
  );
  await page.keyboard.press("ArrowDown");
  const selectionAfter = await submissionTextarea.evaluate(
    (element) => element.selectionStart,
  );
  if (selectionAfter <= selectionBefore) {
    throw new Error(
      `ArrowDown did not move within overflowing textarea content at ${viewport.label}.`,
    );
  }

  await scrollOverflowingTextareaWithKeyboard(page, submissionTextarea, viewport);

  let focusedSubmit = false;
  for (let attempt = 0; attempt < 6; attempt += 1) {
    await page.keyboard.press("Tab");
    focusedSubmit = await submitButton.evaluate(
      (element) => element === document.activeElement,
    );
    if (focusedSubmit) {
      break;
    }
  }

  if (!focusedSubmit) {
    throw new Error(
      `Could not tab from submission textarea to submit button at ${viewport.label}.`,
    );
  }
}
