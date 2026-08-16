import { mkdir } from "node:fs/promises";
import { join } from "node:path";

export async function captureEvidence(page, name) {
  const directory = process.env.AGENT_FACTORY_BROWSER_ARTIFACT_DIR;
  if (!directory) {
    return;
  }

  await mkdir(directory, { recursive: true });
  await page.screenshot({
    fullPage: true,
    path: join(directory, `${name}.png`),
  });
}

export async function selectGraphNodeWithMarquee(
  page,
  viewport,
  locator,
  nodeLabel,
) {
  const nodeBox = await locator.boundingBox();
  const viewportBox = await viewport.boundingBox();
  if (!nodeBox || !viewportBox) {
    throw new Error(
      "Could not measure the graph node and viewport for selection.",
    );
  }

  const startX = Math.max(viewportBox.x + 4, nodeBox.x - 20);
  const startY = Math.max(viewportBox.y + 4, nodeBox.y - 20);
  const endX = Math.min(
    viewportBox.x + viewportBox.width - 4,
    nodeBox.x + nodeBox.width + 20,
  );
  const endY = Math.min(
    viewportBox.y + viewportBox.height - 4,
    nodeBox.y + nodeBox.height + 20,
  );

  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(endX, endY, { steps: 5 });
  await page.mouse.up();
  await expectNodeSelected(page, locator, nodeLabel);
}

async function expectNodeSelected(page, locator, nodeLabel) {
  const handle = await locator.elementHandle();
  if (!handle) {
    throw new Error(
      `Could not resolve the ${nodeLabel} graph node for selection.`,
    );
  }

  try {
    await page.waitForFunction(
      (node) => node.classList.contains("selected"),
      handle,
      { timeout: 5_000 },
    );
  } catch (error) {
    const details = await locator.evaluate((node) => ({
      ariaSelected: node.getAttribute("aria-selected"),
      className: node.className,
      html: node.outerHTML.slice(0, 800),
    }));
    throw new Error(
      `${nodeLabel} node did not become selected: ${JSON.stringify(details)}`,
      {
        cause: error,
      },
    );
  }
}
