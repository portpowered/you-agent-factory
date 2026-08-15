import { chromium } from "playwright";
import { expectNoBrowserErrors } from "../integration/browser-test-harness.mjs";
import { ensureStorybookServer } from "./run-storybook-responsive-check.mjs";
import {
  storyUrl,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";

const host = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const port = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const browserChannel = process.env.AGENT_FACTORY_GRAPH_PARITY_BROWSER_CHANNEL;
const storybookUrl = `http://${host}:${port}`;
const storyId =
  "factory-graph-editor-large-fixtures--large-factory-visual-parity-matrix";
const browserCheckTimeoutMs = 60_000;
const viewports = [
  { height: 844, label: "mobile", width: 390 },
  { height: 1024, label: "tablet", width: 768 },
  { height: 900, label: "desktop", width: 1440 },
];

const server = await ensureStorybookServer({ host, port: Number(port) });
const browser = await chromium.launch(
  browserChannel ? { channel: browserChannel } : undefined,
);

try {
  for (const viewport of viewports) {
    const page = await browser.newPage({ viewport });
    const pageErrors = [];
    const consoleErrors = [];
    page.on("pageerror", (error) => pageErrors.push(error.message));
    page.on("console", (message) => {
      if (message.type() === "error") {
        consoleErrors.push(message.text());
      }
    });
    page.setDefaultTimeout(browserCheckTimeoutMs);

    try {
      await page.emulateMedia({ reducedMotion: "reduce" });
      await page.goto(storyUrl(storybookUrl, storyId), {
        timeout: browserCheckTimeoutMs,
        waitUntil: "networkidle",
      });
      await waitForStoryRender(page);

      const viewportRegion = page.locator(
        '[data-large-parity-viewport="true"]',
      );
      await viewportRegion.waitFor({ state: "visible" });
      if (
        (await page.locator("[data-factory-graph-group-region]").count()) !== 3
      ) {
        throw new Error(
          `Large graph parity at ${viewport.label} did not render three read-only group regions.`,
        );
      }

      const authoredNode = page.locator(
        '.react-flow__node[data-id="workstation:ws-0"]',
      );
      await authoredNode.waitFor({ state: "attached" });
      const initialSize = await readNodeSize(authoredNode);
      const applyOverlayButton = page.getByRole("button", {
        name: "Apply live graph overlay",
      });
      const resetOverlayButton = page.getByRole("button", {
        name: "Reset live graph overlay",
      });
      await page
        .getByRole("button", {
          name: /^(?:Apply|Reset) live graph overlay$/,
        })
        .waitFor({ state: "visible" });
      if (await resetOverlayButton.isVisible()) {
        await resetOverlayButton.focus();
        await page.keyboard.press("Enter");
        await applyOverlayButton.waitFor({ state: "visible" });
      }
      await applyOverlayButton.focus();
      await page.keyboard.press("Enter");
      await resetOverlayButton.waitFor({ state: "visible" });

      if (
        (await page.locator('[data-state-work-progress="numeric"]').count()) !==
        2
      ) {
        throw new Error(
          `Large graph parity at ${viewport.label} did not use numeric Work mode for counts above three.`,
        );
      }
      if (
        (await page.locator("[data-state-work-progress-dot]").count()) !== 4
      ) {
        throw new Error(
          `Large graph parity at ${viewport.label} allocated more or fewer than four small Work markers for counts 1 and 3.`,
        );
      }
      if (
        (await page.locator('[data-graph-visual-active-flow="true"]').count()) <
        1
      ) {
        throw new Error(
          `Large graph parity at ${viewport.label} did not render a live active state.`,
        );
      }

      const reducedMotionAnimationNames = await page
        .locator('[data-graph-visual-active-flow="true"]')
        .evaluateAll((elements) =>
          elements.map(
            (element) => window.getComputedStyle(element).animationName,
          ),
        );
      if (reducedMotionAnimationNames.some((name) => name !== "none")) {
        throw new Error(
          `Reduced motion remained animated at ${viewport.label}: ${JSON.stringify(reducedMotionAnimationNames)}.`,
        );
      }

      const afterOverlaySize = await readNodeSize(authoredNode);
      if (
        afterOverlaySize.width !== initialSize.width ||
        afterOverlaySize.height !== initialSize.height
      ) {
        throw new Error(
          `Authored node size changed during the live overlay at ${viewport.label}: before=${JSON.stringify(initialSize)} after=${JSON.stringify(afterOverlaySize)}.`,
        );
      }

      const resetButton = page.getByRole("button", {
        name: "Reset live graph overlay",
      });
      await resetButton.focus();
      await page.keyboard.press("Enter");
      await page
        .getByRole("button", { name: "Apply live graph overlay" })
        .waitFor({ state: "visible" });

      assertNoBrowserErrors(pageErrors, consoleErrors, viewport.label);
    } finally {
      await page.close();
    }
  }

  console.log(
    "Factory graph large visual-parity browser verification passed at mobile, tablet, and desktop with strict browser-error checks.",
  );
} finally {
  await browser.close();
  await server.stop();
}

async function readNodeSize(node) {
  return node.evaluate((element) => ({
    height: element.style.height,
    width: element.style.width,
  }));
}

function assertNoBrowserErrors(pageErrors, consoleErrors, viewportLabel) {
  const strictExpect = (actual) => ({
    toEqual(expected) {
      if (JSON.stringify(actual) !== JSON.stringify(expected)) {
        throw new Error(
          `Browser errors at ${viewportLabel}: ${JSON.stringify(actual)}`,
        );
      }
    },
  });

  expectNoBrowserErrors(pageErrors, consoleErrors, strictExpect);
  const allErrors = [...pageErrors, ...consoleErrors];
  if (allErrors.some((error) => /(?:React Flow|error)\D*015/i.test(error))) {
    throw new Error(
      `React Flow error 015 occurred at ${viewportLabel}: ${JSON.stringify(allErrors)}`,
    );
  }
}
