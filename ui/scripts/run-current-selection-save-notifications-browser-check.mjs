import { chromium } from "playwright";
import { verifyCurrentSelectionSaveFlow } from "./verify-current-selection-storybook-responsive.mjs";
import {
  expectNoHorizontalOverflow,
  expectVisible,
  storyUrl,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";
import { ensureStorybookServer } from "./run-storybook-responsive-check.mjs";

const host = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const port = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const storybookUrl = `http://${host}:${port}`;
const storyId =
  "you-agent-factory-dashboard-bento-cards--current-selection-save-notifications";

const server = await ensureStorybookServer({ host, port: Number(port) });

try {
  const browser = await chromium.launch();
  const page = await browser.newPage({
    viewport: { height: 900, label: "desktop", width: 1440 },
  });
  await page.goto(storyUrl(storybookUrl, storyId), {
    timeout: 90_000,
    waitUntil: "networkidle",
  });
  await waitForStoryRender(page);
  await verifyCurrentSelectionSaveFlow({
    expectNoHorizontalOverflow,
    expectVisible,
    page,
    viewport: { height: 900, label: "desktop", width: 1440 },
  });
  console.log("Current selection save notification browser verification passed.");
  await browser.close();
} finally {
  await server.stop();
}
