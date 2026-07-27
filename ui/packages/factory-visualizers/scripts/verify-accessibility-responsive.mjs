import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import net from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { chromium } from "playwright";

const require = createRequire(import.meta.url);
const axePath = require.resolve("axe-core/axe.min.js");
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const port = Number.parseInt(
  process.env.FACTORY_VISUALIZERS_STORYBOOK_PORT ?? "3767",
  10,
);
const baseUrl = `http://127.0.0.1:${port}`;
const httpServer = path.resolve(
  packageRoot,
  "../../node_modules/http-server/bin/http-server",
);

const storyIds = [
  "factory-visualizers-factoryemulatorview--full",
  "factory-visualizers-factoryemulatorview--loading-initial",
  "factory-visualizers-factoryemulatorview--empty",
  "factory-visualizers-factoryemulatorview--terminal",
  "factory-visualizers-factoryemulatorview--host-failure",
];

await assertPortAvailable(port);
const server = spawn(
  process.execPath,
  [httpServer, "storybook-static", "-p", String(port), "-a", "127.0.0.1", "-s"],
  { cwd: packageRoot, stdio: "ignore", windowsHide: true },
);
let browser;

try {
  await waitForServer();
  browser = await chromium.launch({ headless: true });
  for (const viewport of [
    { height: 800, width: 360 },
    { height: 900, width: 1200 },
  ]) {
    const context = await browser.newContext({ viewport });
    const page = await context.newPage();
    for (const storyId of storyIds) {
      await openStory(page, storyId);
      await assertNoPageOverflow(page, storyId);
      await assertNoSeriousAccessibilityViolations(page, storyId);
    }
    await openStory(page, storyIds[0]);
    await page
      .locator('[data-current-activity-node-type="worker"]')
      .first()
      .waitFor({ state: "visible", timeout: 5_000 });
    await page
      .locator('[data-current-activity-node-type="workstation"]')
      .first()
      .waitFor({ state: "visible", timeout: 5_000 });
    await context.close();
  }
  process.stdout.write(
    "Factory visualizer accessibility and responsive browser checks passed.\n",
  );
} finally {
  await browser?.close();
  server.kill();
}

async function assertNoPageOverflow(page, storyId) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth + 1,
  );
  if (overflow)
    throw new Error(`[factory-visualizers-storybook] ${storyId} overflows`);
}

async function assertNoSeriousAccessibilityViolations(page, storyId) {
  await page.addScriptTag({ path: axePath });
  const results = await page.evaluate(async () =>
    window.axe.run(document, {
      resultTypes: ["violations"],
      rules: { "color-contrast": { enabled: false } },
    }),
  );
  const serious = results.violations.filter((violation) =>
    ["critical", "serious"].includes(violation.impact),
  );
  if (serious.length > 0)
    throw new Error(
      `[factory-visualizers-storybook] ${storyId} accessibility violations: ${serious.map((violation) => violation.id).join(", ")}`,
    );
}

async function assertPortAvailable(requestedPort) {
  await new Promise((resolve, reject) => {
    const probe = net.createServer();
    probe.once("error", reject);
    probe.listen(requestedPort, "127.0.0.1", () =>
      probe.close((error) => (error ? reject(error) : resolve())),
    );
  });
}

async function openStory(page, storyId) {
  await page.goto(`${baseUrl}/iframe.html?id=${storyId}`, {
    waitUntil: "networkidle",
  });
}

async function waitForServer() {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    try {
      const response = await fetch(baseUrl);
      if (response.ok) return;
    } catch {
      // The static server is still starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error("Factory visualizer Storybook server did not start.");
}
