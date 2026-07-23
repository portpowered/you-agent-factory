import { chromium } from "playwright";
import { ensureStorybookServer } from "./run-storybook-responsive-check.mjs";
import {
  storyUrl,
  waitForStoryRender,
} from "./storybook-responsive-helpers.mjs";
import { verifyFactoryGraphVisualGroupEditorWorkflow } from "./verify-factory-graph-visual-group-editor.mjs";

const host = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const port = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const storybookUrl = `http://${host}:${port}`;
const storyId =
  "factory-graph-editor-visual-groups--editor-save-reload-workflow";

const server = await ensureStorybookServer({ host, port: Number(port) });

try {
  const browser = await chromium.launch();
  const page = await browser.newPage({
    viewport: { height: 900, width: 1440 },
  });

  await verifyFactoryGraphVisualGroupEditorWorkflow({
    page,
    storyUrl: storyUrl(storybookUrl, storyId),
    waitForStoryRender,
  });

  console.log("Visual group editor save/reload browser verification passed.");
  await browser.close();
} finally {
  await server.stop();
}
