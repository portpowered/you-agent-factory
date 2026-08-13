import { chromium } from "playwright";

import { HOST, PORT } from "./run-storybook-ci.mjs";
import { ensureStorybookServer } from "./run-storybook-responsive-check.mjs";
import {
  storyUrl,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";

const baseUrl = `http://${HOST}:${PORT}`;
const storyID =
  "you-agent-factory-components-prompt-editor--monaco-scrollbar-projection";
const visibleEditorKinds = ["workstation-prompt", "factory-doc-text"];

function editorLocator(page, editorKind) {
  return page.locator(`[data-monaco-editor="${editorKind}"]`).first();
}

async function readEditorProjection(page, editorKind) {
  return page.evaluate((kind) => {
    const shell = document.querySelector(`[data-monaco-editor="${kind}"]`);
    const editor = shell?.querySelector(".monaco-editor");
    const vertical = editor?.querySelector(".scrollbar.vertical");
    const slider = vertical?.querySelector(".slider");
    const overflowingDescendant = Array.from(
      editor?.querySelectorAll("*") ?? [],
    ).find(
      (candidate) =>
        candidate instanceof HTMLElement &&
        candidate.scrollHeight > candidate.clientHeight,
    );
    const sliderStyle = slider ? getComputedStyle(slider) : null;
    const verticalBox = vertical?.getBoundingClientRect();
    const sliderBox = slider?.getBoundingClientRect();

    return {
      hasEditor: editor != null,
      hasSlider: slider != null,
      overflowingDescendant: overflowingDescendant
        ? {
            clientHeight: overflowingDescendant.clientHeight,
            scrollHeight: overflowingDescendant.scrollHeight,
            scrollTop: overflowingDescendant.scrollTop,
          }
        : null,
      slider: slider
        ? {
            backgroundColor: sliderStyle?.backgroundColor ?? "",
            className: slider.className,
            height: sliderBox?.height ?? 0,
            left: slider.style.left,
            top: slider.style.top,
            transform: sliderStyle?.transform ?? "",
            width: sliderBox?.width ?? 0,
          }
        : null,
      verticalScrollbar: verticalBox
        ? { height: verticalBox.height, width: verticalBox.width }
        : null,
      visualViewportScale: window.visualViewport?.scale ?? 1,
    };
  }, editorKind);
}

async function hoverEditorScrollbar(page, editorKind) {
  const vertical = editorLocator(page, editorKind)
    .locator(".monaco-editor .scrollbar.vertical")
    .first();
  const slider = vertical.locator(".slider").first();
  await vertical.hover({ force: true });
  const box = (await slider.boundingBox()) ?? (await vertical.boundingBox());
  if (!box) {
    throw new Error(`The ${editorKind} vertical scrollbar is not measurable.`);
  }

  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.waitForTimeout(50);
}

async function assertEditorOverflow(page, editorKind) {
  await editorLocator(page, editorKind)
    .locator(".monaco-editor")
    .waitFor({ state: "visible" });
  await page.waitForFunction((kind) => {
    const editor = document.querySelector(
      `[data-monaco-editor="${kind}"] .monaco-editor`,
    );
    return Array.from(editor?.querySelectorAll("*") ?? []).some(
      (candidate) =>
        candidate instanceof HTMLElement &&
        candidate.scrollHeight > candidate.clientHeight,
    );
  }, editorKind);

  const before = await readEditorProjection(page, editorKind);
  if (!before.hasEditor || !before.hasSlider || !before.overflowingDescendant) {
    throw new Error(`The ${editorKind} editor did not expose real overflow.`);
  }
  if (
    before.overflowingDescendant.scrollHeight <=
    before.overflowingDescendant.clientHeight
  ) {
    throw new Error(`The ${editorKind} editor is not vertically overflowing.`);
  }

  await hoverEditorScrollbar(page, editorKind);
  const hovered = await readEditorProjection(page, editorKind);
  if (before.slider?.backgroundColor === hovered.slider?.backgroundColor) {
    throw new Error(
      `The ${editorKind} slider did not change on hover (${before.slider?.backgroundColor} -> ${hovered.slider?.backgroundColor}; ${before.slider?.className} -> ${hovered.slider?.className}).`,
    );
  }
  if (hovered.verticalScrollbar?.width !== 8) {
    throw new Error(
      `Expected the ${editorKind} vertical scrollbar allocation to remain 8 CSS px, received ${hovered.verticalScrollbar?.width}.`,
    );
  }

  const slider = editorLocator(page, editorKind)
    .locator(".monaco-editor .scrollbar.vertical .slider")
    .first();
  const sliderBox = await slider.boundingBox();
  if (!sliderBox) {
    throw new Error(`The ${editorKind} slider is not measurable.`);
  }
  await page.mouse.move(
    sliderBox.x + sliderBox.width / 2,
    sliderBox.y + sliderBox.height / 2,
  );
  await page.mouse.down();
  const active = await readEditorProjection(page, editorKind);
  await page.mouse.up();

  if (active.slider?.backgroundColor !== hovered.slider?.backgroundColor) {
    throw new Error(
      `The ${editorKind} active slider role diverged from hover.`,
    );
  }

  await editorLocator(page, editorKind).locator(".native-edit-context").focus();
  await page.keyboard.press("PageDown");
  await page.waitForTimeout(100);
  const afterKeyboard = await readEditorProjection(page, editorKind);
  if (
    afterKeyboard.slider?.top === before.slider?.top &&
    afterKeyboard.slider?.transform === before.slider?.transform
  ) {
    throw new Error(`The ${editorKind} editor did not move on PageDown.`);
  }
}

async function main() {
  const server = await ensureStorybookServer();
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { height: 900, width: 1280 },
  });
  const page = await context.newPage();

  try {
    await page.goto(storyUrl(baseUrl, storyID), {
      timeout: 60_000,
      waitUntil: "domcontentloaded",
    });
    await waitForStoryRender(page);

    for (const editorKind of visibleEditorKinds) {
      await assertEditorOverflow(page, editorKind);
    }

    const guard = await readEditorProjection(
      page,
      "workstation-guard-selector",
    );
    if (guard.slider?.width !== 0 || guard.verticalScrollbar?.width !== 0) {
      throw new Error("The guard-selector editor exposed a visible scrollbar.");
    }

    const client = await context.newCDPSession(page);
    await client.send("Emulation.setPageScaleFactor", { pageScaleFactor: 2 });
    await page.waitForFunction(() => window.visualViewport?.scale === 2);

    for (const editorKind of visibleEditorKinds) {
      const scaled = await readEditorProjection(page, editorKind);
      if (scaled.visualViewportScale !== 2) {
        throw new Error(
          `The ${editorKind} editor did not reach 200% page scale.`,
        );
      }
      if (
        scaled.verticalScrollbar?.width !== 8 ||
        scaled.overflowingDescendant == null ||
        scaled.overflowingDescendant.scrollHeight <=
          scaled.overflowingDescendant.clientHeight
      ) {
        throw new Error(
          `The ${editorKind} editor lost its 8 CSS-pixel scrollbar or overflow at 200% page scale.`,
        );
      }
      await hoverEditorScrollbar(page, editorKind);
    }

    const scaledGuard = await readEditorProjection(
      page,
      "workstation-guard-selector",
    );
    if (
      scaledGuard.slider?.width !== 0 ||
      scaledGuard.verticalScrollbar?.width !== 0
    ) {
      throw new Error(
        "The guard-selector editor exposed a scrollbar at 200% page scale.",
      );
    }

    console.log(
      "Verified prompt/document Monaco scrollbar roles, scrolling, 8px dimensions, guard suppression, and 200% page scale.",
    );
  } finally {
    await context.close();
    await browser.close();
    await server.stop();
  }
}

await main();
