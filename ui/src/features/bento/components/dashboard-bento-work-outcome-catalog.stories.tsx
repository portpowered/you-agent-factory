import { expect, within } from "storybook/test";

import "../../../styles.css";
import {
  expectWorkChartAxisLabelsVisible,
  expectWorkChartCompactLegendContract,
  expectWorkChartLegendClearOfCardTitle,
} from "../../work-outcome/lib/work-chart-legend-story-contract";
import { WorkOutcomeWidget } from "../../work-outcome/public";
import { DASHBOARD_WIDGET_IDS } from "../hooks/dashboardLayoutSchema";
import {
  emptyWorkOutcomeModel,
  expectBentoHeaderDragSurface,
  layoutFor,
  renderCardFrame,
  renderWorkOutcomeStateCard,
  workOutcomeModel,
} from "./dashboard-bento-story-shared";

export default {
  title: "you-agent-factory/Dashboard/Bento Cards",
  tags: ["test"],
};

export const WorkOutcomeChart = {
  render: () =>
    renderCardFrame({
      children: (
        <WorkOutcomeWidget
          model={workOutcomeModel}
          widgetId="work-outcome-chart::story"
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.workOutcomeChart, {
        h: 6,
        id: "work-outcome-chart::story",
        w: 6,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });
    const chart = within(card).getByRole("img", {
      name: "Work outcome chart for Session",
    });

    await expect(chart).toBeVisible();
    expectBentoHeaderDragSurface(card, "Work outcome chart");
    expectWorkChartCompactLegendContract(chart);
    expectWorkChartAxisLabelsVisible(chart);
    expectWorkChartLegendClearOfCardTitle(card);
  },
};

export const WorkOutcomeChartNarrow = {
  render: () =>
    renderCardFrame({
      children: (
        <WorkOutcomeWidget
          model={workOutcomeModel}
          widgetId="work-outcome-chart-narrow::story"
        />
      ),
      initialWidth: 360,
      layout: layoutFor(DASHBOARD_WIDGET_IDS.workOutcomeChart, {
        h: 6,
        id: "work-outcome-chart-narrow::story",
        w: 6,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });
    const chart = within(card).getByRole("img", {
      name: "Work outcome chart for Session",
    });

    expectWorkChartCompactLegendContract(chart);
    expectWorkChartAxisLabelsVisible(chart);
    expectWorkChartLegendClearOfCardTitle(card);

    const frame = canvasElement.firstElementChild;
    if (!(frame instanceof HTMLElement)) {
      throw new Error("expected catalog story frame");
    }
    expect(frame.getBoundingClientRect().width).toBeLessThanOrEqual(360);
    expect(frame.scrollWidth <= frame.clientWidth + 1).toBe(true);
  },
};

export const WorkOutcomeChartLoading = {
  render: () =>
    renderWorkOutcomeStateCard({
      chartState: { status: "loading" },
      model: emptyWorkOutcomeModel,
      storyID: "work-outcome-chart-loading::story",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });

    await expect(await within(card).findByRole("status")).toHaveTextContent(
      "Loading work outcome samples",
    );
  },
};

export const WorkOutcomeChartEmpty = {
  render: () =>
    renderWorkOutcomeStateCard({
      model: emptyWorkOutcomeModel,
      storyID: "work-outcome-chart-empty::story",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });

    await expect(await within(card).findByRole("status")).toHaveTextContent(
      "No work outcome samples",
    );
  },
};

export const WorkOutcomeChartError = {
  render: () =>
    renderWorkOutcomeStateCard({
      chartState: { status: "error" },
      model: emptyWorkOutcomeModel,
      storyID: "work-outcome-chart-error::story",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });

    await expect(await within(card).findByRole("alert")).toHaveTextContent(
      "Work outcome chart unavailable",
    );
  },
};
