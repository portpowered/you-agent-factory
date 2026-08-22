// @vitest-environment node

import { describe } from "vitest";

import {
  browserScenarioTimeoutMs,
  expectNoBrowserErrors,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";
import { isolatedMockBrowserTest as it } from "./mocked-browser-test-fixture.mjs";

const currentFactory = {
  metadata: {
    owner: "operations",
  },
  name: "Native Scrollbar Browser Factory",
  resources: [
    {
      capacity: 2,
      name: "gpu",
    },
  ],
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
        {
          name: "done",
          type: "TERMINAL",
        },
      ],
    },
  ],
  workstations: [
    {
      body: "Draft the story.",
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      name: "draft",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      resources: [
        {
          capacity: 2,
          name: "gpu",
        },
      ],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

const replayLines = [
  JSON.stringify({
    context: {
      eventTime: "2026-05-19T15:00:00Z",
      sequence: 1,
      tick: 1,
    },
    id: "native-scrollbar-1",
    payload: {
      factory: currentFactory,
    },
    type: "INITIAL_STRUCTURE_REQUEST",
  }),
  JSON.stringify({
    context: {
      eventTime: "2026-05-19T15:00:01Z",
      sequence: 2,
      tick: 2,
    },
    id: "native-scrollbar-2",
    payload: {
      previousState: "RUNNING",
      reason: "fixture ready",
      state: "FINISHED",
    },
    type: "FACTORY_STATE_RESPONSE",
  }),
];

async function installNativeOverflowFixture(page) {
  await page.evaluate(() => {
    const fixture = document.createElement("main");
    fixture.dataset.nativeScrollbarFixture = "true";
    fixture.innerHTML = `
      <section data-native-scroll-vertical tabindex="0">
        <div data-native-scroll-content></div>
      </section>
      <section data-native-scroll-horizontal tabindex="0">
        <div data-native-scroll-content></div>
      </section>
    `;
    fixture.style.cssText =
      "display:flex;gap:16px;position:fixed;right:8px;bottom:8px;z-index:2147483647;";

    const vertical = fixture.querySelector("[data-native-scroll-vertical]");
    const horizontal = fixture.querySelector("[data-native-scroll-horizontal]");
    const verticalContent = vertical?.querySelector(
      "[data-native-scroll-content]",
    );
    const horizontalContent = horizontal?.querySelector(
      "[data-native-scroll-content]",
    );
    if (
      !(vertical instanceof HTMLElement) ||
      !(horizontal instanceof HTMLElement) ||
      !(verticalContent instanceof HTMLElement) ||
      !(horizontalContent instanceof HTMLElement)
    ) {
      throw new Error("Native scrollbar fixture failed to mount.");
    }

    vertical.style.cssText +=
      "height:96px;width:128px;overflow:auto;background:var(--color-surface);";
    horizontal.style.cssText +=
      "height:96px;width:128px;overflow:auto;background:var(--color-surface);";
    verticalContent.style.cssText =
      "height:640px;width:96px;background:var(--color-surface-container);";
    horizontalContent.style.cssText =
      "height:96px;width:640px;background:var(--color-surface-container);";
    document.body.append(fixture);
  });

  await page
    .locator("[data-native-scroll-vertical]")
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
}

async function readNativeScrollbarProjection(page) {
  return page.evaluate(() => {
    const vertical = document.querySelector("[data-native-scroll-vertical]");
    if (!(vertical instanceof HTMLElement)) {
      throw new Error("Native vertical scrollport is missing.");
    }

    const elementStyle = getComputedStyle(vertical);
    const scrollbarStyle = getComputedStyle(vertical, "::-webkit-scrollbar");
    const trackStyle = getComputedStyle(vertical, "::-webkit-scrollbar-track");
    const thumbStyle = getComputedStyle(vertical, "::-webkit-scrollbar-thumb");
    const tokenProbe = document.createElement("span");
    tokenProbe.style.backgroundColor = "var(--color-outline-variant)";
    vertical.append(tokenProbe);
    const resolvedToken = getComputedStyle(tokenProbe).backgroundColor;
    tokenProbe.remove();

    return {
      clientHeight: vertical.clientHeight,
      scrollHeight: vertical.scrollHeight,
      scrollbarColor: elementStyle.getPropertyValue("scrollbar-color"),
      scrollbarWidth: elementStyle.getPropertyValue("scrollbar-width"),
      thumbBackgroundColor: thumbStyle.backgroundColor,
      thumbBorderRadius: thumbStyle.borderRadius,
      thumbWidth: scrollbarStyle.width,
      trackBackgroundColor: trackStyle.backgroundColor,
      trackHeight: scrollbarStyle.height,
      token: resolvedToken,
    };
  });
}

async function prepareRadixScrollbarFixture(page) {
  await page.evaluate(() => {
    const viewport = Array.from(
      document.querySelectorAll("[data-radix-scroll-area-viewport]"),
    ).find((candidate) => {
      return candidate instanceof HTMLElement && candidate.clientHeight > 0;
    });
    if (!(viewport instanceof HTMLElement)) {
      throw new Error("A visible Radix ScrollArea viewport is missing.");
    }

    const content = viewport.firstElementChild;
    if (!(content instanceof HTMLElement)) {
      throw new Error("The Radix ScrollArea viewport has no content wrapper.");
    }
    const marker = document.createElement("div");
    marker.dataset.radixScrollbarFixtureContent = "true";
    marker.style.cssText = "height:1200px;width:1200px;";
    content.append(marker);

    viewport.dataset.radixScrollbarFixtureViewport = "true";
    viewport.scrollTop = 32;
  });

  // Hover-revealed ScrollAreas (type="hover", e.g. the agent bento cards)
  // only mount their Radix scrollbar while the pointer is over the scroll
  // area, so hover the fixture viewport before reading the projection.
  await page
    .locator('[data-radix-scrollbar-fixture-viewport="true"]')
    .hover({ timeout: uiInteractionTimeoutMs });
  await page.waitForFunction(
    () => {
      const viewport = document.querySelector(
        '[data-radix-scrollbar-fixture-viewport="true"]',
      );
      const root = viewport?.parentElement;
      return (
        (root?.querySelectorAll('[data-orientation="vertical"]').length ?? 0) >
        0
      );
    },
    undefined,
    { timeout: uiInteractionTimeoutMs },
  );
}

async function readRadixScrollbarProjection(page) {
  await prepareRadixScrollbarFixture(page);
  return page.evaluate(() => {
    const viewport = document.querySelector(
      '[data-radix-scrollbar-fixture-viewport="true"]',
    );
    if (!(viewport instanceof HTMLElement)) {
      throw new Error("The Radix scrollbar fixture viewport is missing.");
    }

    const root = viewport.parentElement;
    const viewportStyle = getComputedStyle(viewport);
    const nativeScrollbarStyle = getComputedStyle(
      viewport,
      "::-webkit-scrollbar",
    );

    return {
      nativeScrollbarDisplay: nativeScrollbarStyle.display,
      rootVerticalTrackCount:
        root?.querySelectorAll('[data-orientation="vertical"]').length ?? 0,
      scrollHeight: viewport.scrollHeight,
      scrollTop: viewport.scrollTop,
      scrollbarWidth: viewportStyle.getPropertyValue("scrollbar-width"),
    };
  });
}

async function readForcedColorsProjection(page) {
  await page.emulateMedia({ forcedColors: "active" });
  try {
    return await page.evaluate(() => {
      const vertical = document.querySelector("[data-native-scroll-vertical]");
      if (!(vertical instanceof HTMLElement)) {
        throw new Error("Native vertical scrollport is missing.");
      }
      const style = getComputedStyle(vertical);
      return {
        forcedColorsActive: matchMedia("(forced-colors: active)").matches,
        forcedColorAdjust: style.getPropertyValue("forced-color-adjust"),
        scrollbarColor: style.getPropertyValue("scrollbar-color"),
        scrollbarWidth: style.getPropertyValue("scrollbar-width"),
      };
    });
  } finally {
    await page.emulateMedia({ forcedColors: "none" });
  }
}

async function applyTwoHundredPercentPageScale(page) {
  const client = await page.context().newCDPSession(page);
  await client.send("Emulation.setPageScaleFactor", { pageScaleFactor: 2 });
}

async function assertNativeScrollbarProjection(page, expect) {
  const nativeProjection = await readNativeScrollbarProjection(page);
  expect(nativeProjection.scrollHeight).toBeGreaterThan(
    nativeProjection.clientHeight,
  );
  expect(nativeProjection.scrollbarWidth).toBe("thin");
  expect(nativeProjection.scrollbarColor).toContain(nativeProjection.token);
  expect(nativeProjection.thumbWidth).toBe("10px");
  expect(nativeProjection.trackHeight).toBe("10px");
  expect(nativeProjection.thumbBorderRadius).toBe("999px");
  expect(nativeProjection.thumbBackgroundColor).toBe(nativeProjection.token);
  expect(nativeProjection.trackBackgroundColor).toBe("rgba(0, 0, 0, 0)");
}

async function assertNativeScrolling(page, expect) {
  const vertical = page.locator("[data-native-scroll-vertical]");
  const verticalBox = await vertical.boundingBox();
  if (!verticalBox) {
    throw new Error("Native vertical scrollport is not measurable.");
  }
  await page.mouse.move(
    verticalBox.x + verticalBox.width / 2,
    verticalBox.y + verticalBox.height / 2,
  );
  await page.mouse.wheel(0, 160);
  await page.waitForFunction(() => {
    const element = document.querySelector("[data-native-scroll-vertical]");
    return element instanceof HTMLElement && element.scrollTop > 0;
  });
  expect(
    await vertical.evaluate((element) => element.scrollTop),
  ).toBeGreaterThan(0);
  await vertical.focus();
  await page.keyboard.press("PageDown");
  await page.waitForFunction(() => {
    const element = document.querySelector("[data-native-scroll-vertical]");
    return element instanceof HTMLElement && element.scrollTop > 0;
  });
  expect(
    await vertical.evaluate((element) => element.scrollTop),
  ).toBeGreaterThan(0);

  const horizontal = page.locator("[data-native-scroll-horizontal]");
  expect(
    await horizontal.evaluate((element) => element.scrollWidth),
  ).toBeGreaterThan(
    await horizontal.evaluate((element) => element.clientWidth),
  );
  await horizontal.evaluate((element) => {
    element.scrollLeft = 32;
  });
  expect(
    await horizontal.evaluate((element) => element.scrollLeft),
  ).toBeGreaterThan(0);
  return vertical;
}

async function assertRadixProjection(page, expect) {
  const radixProjection = await readRadixScrollbarProjection(page);
  expect(radixProjection.scrollHeight).toBeGreaterThan(0);
  expect(radixProjection.scrollTop).toBeGreaterThan(0);
  expect(radixProjection.scrollbarWidth).toBe("none");
  expect(radixProjection.nativeScrollbarDisplay).toBe("none");
  expect(radixProjection.rootVerticalTrackCount).toBe(1);
}

async function assertForcedColorsProjection(page, expect) {
  const forcedColorsProjection = await readForcedColorsProjection(page);
  expect(forcedColorsProjection.forcedColorsActive).toBe(true);
  expect(forcedColorsProjection.forcedColorAdjust).toBe("auto");
  expect(forcedColorsProjection.scrollbarColor).toBe("auto");
  expect(forcedColorsProjection.scrollbarWidth).toBe("auto");
}

async function assertTwoHundredPercentPageScale(page, vertical) {
  await applyTwoHundredPercentPageScale(page);
  if ((await page.evaluate(() => visualViewport.scale)) !== 2) {
    throw new Error("The browser did not apply the requested 200% page scale.");
  }
  if (
    (await vertical.evaluate((element) => element.scrollHeight)) <=
    (await vertical.evaluate((element) => element.clientHeight))
  ) {
    throw new Error("Native overflow disappeared at 200% page scale.");
  }
  await vertical.evaluate((element) => {
    element.scrollTop = 0;
  });
  await page.keyboard.press("PageDown");
  await page.waitForFunction(() => {
    const element = document.querySelector("[data-native-scroll-vertical]");
    return element instanceof HTMLElement && element.scrollTop > 0;
  });
}

describe.concurrent("dashboard scrollbar browser contract", () => {
  it(
    "styles real native overflow, preserves Radix ownership, restores forced colors, and remains usable at 200% page scale",
    async ({ expect, openBrowserPage, preview }) => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory,
        eventLines: replayLines,
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "native-scrollbar-contract",
      });

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await browserPage.page.waitForFunction(() => {
          return document.querySelectorAll('link[rel="stylesheet"]').length > 0;
        });
        await installNativeOverflowFixture(browserPage.page);

        await assertNativeScrollbarProjection(browserPage.page, expect);
        const vertical = await assertNativeScrolling(browserPage.page, expect);
        await assertRadixProjection(browserPage.page, expect);
        await assertForcedColorsProjection(browserPage.page, expect);
        await assertTwoHundredPercentPageScale(browserPage.page, vertical);
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await server.stop();
      }
    },
    browserScenarioTimeoutMs,
  );
});
