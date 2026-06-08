import { ensureStorybookServer } from "./run-storybook-responsive-check.mjs";
import { verifyCheckboxConsistencyStories } from "./verify-checkbox-consistency-storybook-responsive.mjs";

const host = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? "127.0.0.1";
const port = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? "6008";
const storybookUrl = `http://${host}:${port}`;

const server = await ensureStorybookServer({ host, port: Number(port) });

try {
  await verifyCheckboxConsistencyStories({ storybookUrl });
  console.log("Checkbox consistency browser verification passed.");
} finally {
  await server.stop();
}
