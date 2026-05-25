import { expect, within } from "storybook/test";

import { expectNoPageHorizontalOverflow } from "./dashboardStoryTestUtils";

export async function expectCompactedTopDashboardSection(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);
  const toolbar = await canvas.findByRole("region", {
    name: "dashboard summary",
  });
  const board = await canvas.findByRole("region", {
    name: "you-agent-factory bento board",
  });
  const workTotals = await canvas.findByRole("article", {
    name: "Work totals",
  });
  const graphCard = await canvas.findByRole("article", {
    name: "Factory graph",
  });
  const slider = within(toolbar).getByRole("slider", {
    name: "Timeline tick",
  });
  const progressText = within(toolbar).getByText("5/5");
  const languageButton = within(toolbar).getByRole("button", {
    name: "Change language",
  });
  const exportButton = within(toolbar).getByRole("button", {
    name: "Export PNG",
  });
  const streamStatus = within(toolbar).getByRole("status", {
    name: /You Agent Factory event stream (connecting|live)/,
  });

  await expect(toolbar).toBeVisible();
  await expect(workTotals).toBeVisible();
  await expect(graphCard).toBeVisible();
  await expect(
    within(graphCard).getByRole("region", { name: "Work graph viewport" }),
  ).toBeVisible();
  expect(board.contains(workTotals)).toBe(true);
  expect(board.contains(graphCard)).toBe(true);
  expect(slider).toBeVisible();
  expect(progressText).toBeVisible();
  expect(
    within(toolbar).queryByRole("button", { name: "Return to current tick" }),
  ).toBeNull();
  expect(languageButton).toBeVisible();
  expect(exportButton).toBeVisible();
  expect(streamStatus).toBeVisible();

  languageButton.focus();
  expect(canvasElement.ownerDocument.activeElement).toBe(languageButton);
  exportButton.focus();
  expect(canvasElement.ownerDocument.activeElement).toBe(exportButton);

  expectNoPageHorizontalOverflow(canvasElement);
}
