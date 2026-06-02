import {
  assertPortAvailable,
  HOST,
  PORT,
  spawnBun,
  stopServer,
  waitForStorybookReady,
} from "./run-storybook-ci.mjs";
import { createServerExitPromise } from "./run-storybook-responsive-check.mjs";
import { verifyChooseFileStories } from "./verify-choose-file-storybook-responsive.mjs";

const host = process.env.AGENT_FACTORY_STORYBOOK_HOST ?? HOST;
const port = process.env.AGENT_FACTORY_STORYBOOK_PORT ?? PORT;
const storybookUrl = `http://${host}:${port}`;

await assertPortAvailable(host, port);

const server = spawnBun([
  "x",
  "--no-install",
  "http-server",
  "storybook-static",
  "-p",
  port,
  "-a",
  host,
  "-s",
]);

try {
  await waitForStorybookReady({ serverExit: createServerExitPromise(server) });
  await verifyChooseFileStories({ storybookUrl });
  console.log("Unified choose-file browser verification passed.");
} finally {
  await stopServer(server);
}
