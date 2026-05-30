import { expect, within } from "storybook/test";

import "../../../styles.css";
import {
  DashboardBentoResponsiveStory,
  expectBentoHeaderDragSurface,
  HeaderConsistencyStory,
  providerSessionFetchMock,
  semanticWorkflowDashboardSnapshot,
} from "./dashboard-bento-story-shared";

export default {
  title: "you-agent-factory/Dashboard/Bento Cards",
  tags: ["test"],
};

export const HeaderConsistencyVerification = {
  parameters: {
    dashboardApi: {
      fetchMocks: [providerSessionFetchMock],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => <HeaderConsistencyStory initialWidth={1180} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    for (const cardName of [
      "Work totals",
      "Provider session",
      "Add widget",
      "Submit work",
    ] as const) {
      const card = await canvas.findByRole("article", { name: cardName });
      expectBentoHeaderDragSurface(card, cardName);
    }

    const submitWorkCard = await canvas.findByRole("article", {
      name: "Submit work",
    });
    await expect(
      within(submitWorkCard).getByRole("button", {
        name: "Remove Submit work widget from dashboard",
      }),
    ).toBeVisible();
  },
};

export const ResponsiveVerification = {
  parameters: {
    dashboardApi: {
      fetchMocks: [providerSessionFetchMock],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => <DashboardBentoResponsiveStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("article", { name: "Work totals" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("article", { name: "Factory graph" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("article", { name: "Current selection" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("article", { name: "Provider session" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("article", { name: "Submit work" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("article", { name: "Work outcome chart" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("article", { name: "Trace drill-down" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("article", {
        name: "Completed and failed work",
      }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("article", { name: "Add widget" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("button", { name: "Submit work" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("img", {
        name: "Work outcome chart for Session",
      }),
    ).toBeVisible();
    await expect(await canvas.findByText("trace-active-story")).toBeVisible();
    await expect(await canvas.findByText("Transcript")).toBeVisible();
  },
};
