import { ensureStorybookServer } from "./run-storybook-responsive-check.mjs";
import { verifySubmitWorkTextareaScrollableStories } from "./verify-submit-work-textarea-scrollable-storybook-responsive.mjs";

const host = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const port = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const storybookUrl = `http://${host}:${port}`;

const server = await ensureStorybookServer({ host, port: Number(port) });

try {
  await verifySubmitWorkTextareaScrollableStories({ storybookUrl });
  console.log("Submit-work textarea scrollable browser verification passed.");
} finally {
  await server.stop();
}
