import { expect, waitFor, within } from "storybook/test";

import "../../../styles.css";
import {
  expectWorkChartAxisLabelsVisible,
  expectWorkChartCompactLegendContract,
  expectWorkChartLegendClearOfCardTitle,
} from "../../work-outcome/lib/work-chart-legend-contract";
import { expectSingleWorkOutcomeCardHeader } from "../../work-outcome/lib/work-outcome-card-header-contract";
import { getWorkOutcomeMessages } from "../../work-outcome/messages/work-outcome";
import { WorkOutcomeWidget } from "../../work-outcome/components/work-outcome-widget";
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

async function expectWorkOutcomeChartLegendAndAxisContract(
  card: HTMLElement,
  chart: HTMLElement,
): Promise<void> {
  const chartMessages = getWorkOutcomeMessages().chart;

  await waitFor(() => {
    expectWorkChartCompactLegendContract(chart);
    expectWorkChartAxisLabelsVisible(chart, {
      xAxisLabel: chartMessages.xAxisLabel,
      yAxisLabel: chartMessages.yAxisLabel,
    });
    expectWorkChartLegendClearOfCardTitle(card);
  });
}

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
    const chartMessages = getWorkOutcomeMessages().chart;

    await expect(chart).toBeVisible();
    expectBentoHeaderDragSurface(card, "Work outcome chart");
    expectSingleWorkOutcomeCardHeader(card, {
      cardRegionLabel: chartMessages.cardRegionLabel,
      cardTitle: chartMessages.cardTitle,
    });
    await expectWorkOutcomeChartLegendAndAxisContract(card, chart);
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
    const chartMessages = getWorkOutcomeMessages().chart;

    expectSingleWorkOutcomeCardHeader(card, {
      cardRegionLabel: chartMessages.cardRegionLabel,
      cardTitle: chartMessages.cardTitle,
    });
    await expectWorkOutcomeChartLegendAndAxisContract(card, chart);

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
    const chartMessages = getWorkOutcomeMessages().chart;
    expectSingleWorkOutcomeCardHeader(card, {
      cardRegionLabel: chartMessages.cardRegionLabel,
      cardTitle: chartMessages.cardTitle,
    });
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
    const chartMessages = getWorkOutcomeMessages().chart;
    expectSingleWorkOutcomeCardHeader(card, {
      cardRegionLabel: chartMessages.cardRegionLabel,
      cardTitle: chartMessages.cardTitle,
    });
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
    const chartMessages = getWorkOutcomeMessages().chart;
    expectSingleWorkOutcomeCardHeader(card, {
      cardRegionLabel: chartMessages.cardRegionLabel,
      cardTitle: chartMessages.cardTitle,
    });
  },
};
