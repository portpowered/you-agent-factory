import { render, screen, within } from "@testing-library/react";
import { afterAll, describe, expect, it } from "bun:test";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import {
  expectSingleWorkOutcomeCardHeader,
  expectWorkOutcomeCardFillsChartBody,
} from "../lib/work-outcome-card-header-contract";
import { getWorkOutcomeMessages } from "../messages/work-outcome";
import { WorkChartCard } from "./d3-information-card";
import {
  sparseWorkChartModel,
  zeroValuedFailedSeriesModel,
} from "./work-chart/contracts/work-chart.test.fixtures";

describe("WorkChartCard owner contract", () => {
  const restoreBrowserShims = installDashboardBrowserTestShims();

  afterAll(() => {
    restoreBrowserShims();
  });

  it("composes the chart into one bento frame without a duplicate title", () => {
    const messages = getWorkOutcomeMessages();
    render(
      <WorkChartCard
        headerAction={<button type="button">Remove chart</button>}
        model={sparseWorkChartModel}
        widgetId="work-outcome-chart"
      />,
    );

    const card = screen.getByRole("article", {
      name: messages.chart.cardTitle,
    });
    expectSingleWorkOutcomeCardHeader(card, {
      cardRegionLabel: messages.chart.cardRegionLabel,
      cardTitle: messages.chart.cardTitle,
      headerActionLabel: "Remove chart",
    });
    expectWorkOutcomeCardFillsChartBody(card, messages.chart.cardRegionLabel);

    const chart = within(card).getByRole("img", {
      name: messages.chart.ariaLabel(sparseWorkChartModel.rangeLabel),
    });
    expect(chart.getAttribute("data-work-chart-presentation")).toBe("embedded");
    expect(
      within(card).queryAllByRole("heading", {
        name: messages.chart.cardTitle,
      }),
    ).toHaveLength(1);
  });

  it("passes localized chart copy and outcome series to the chart owner", () => {
    render(
      <WorkChartCard locale="zh-CN" model={zeroValuedFailedSeriesModel} />,
    );

    const chart = screen.getByRole("img", { name: "15m 的工作结果图表" });
    expect(within(chart).getByText("排队中")).toBeTruthy();
    expect(within(chart).getByText("进行中")).toBeTruthy();
    expect(within(chart).getByText("已完成")).toBeTruthy();
    expect(within(chart).getByText("失败/重试")).toBeTruthy();
    expect(within(chart).getByText("刻度")).toBeTruthy();
  });

  it("forwards non-ready state through the persistent card region", () => {
    const messages = getWorkOutcomeMessages();
    render(
      <WorkChartCard
        chartState={{ status: "loading" }}
        model={sparseWorkChartModel}
      />,
    );

    const card = screen.getByRole("article", {
      name: messages.chart.cardTitle,
    });
    expect(
      within(card).getByLabelText(messages.chart.cardRegionLabel),
    ).toBeTruthy();
    expect(within(card).getByRole("status")).toBeTruthy();
    expect(within(card).getByText("Loading work outcome samples")).toBeTruthy();
    expect(within(card).queryByRole("img")).toBeNull();
  });
});
