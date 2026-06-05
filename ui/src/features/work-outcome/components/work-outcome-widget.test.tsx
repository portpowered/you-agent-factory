import { render, screen } from "@testing-library/react";

import type { WorkChartModel } from "../lib/trends";
import {
  expectSingleWorkOutcomeCardHeader,
  expectWorkOutcomeCardFillsChartBody,
} from "../lib/work-outcome-card-header-contract";
import { getWorkOutcomeMessages } from "../messages/work-outcome";
import { WorkOutcomeWidget } from "./work-outcome-widget";

const emptyTrend: WorkChartModel = {
  delta: {
    completed: 0,
    failed: 0,
    inFlight: 0,
    queued: 0,
  },
  failureGroups: [],
  points: [],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [],
  series: [],
};

const populatedTrend: WorkChartModel = {
  delta: {
    completed: 3,
    failed: 1,
    inFlight: 2,
    queued: 1,
  },
  failureGroups: [],
  points: [
    { label: "Tick 7", observedAt: 1000, order: 0, tick: 7 },
    { label: "Tick 9", observedAt: 2000, order: 1, tick: 9 },
  ],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [
    {
      completedCount: 2,
      dispatchedCount: 4,
      failedByWorkType: {},
      failedCount: 1,
      failedWorkLabels: [],
      inFlightCount: 1,
      observedAt: 1000,
      queuedCount: 3,
      tick: 7,
    },
    {
      completedCount: 5,
      dispatchedCount: 8,
      failedByWorkType: {},
      failedCount: 2,
      failedWorkLabels: [],
      inFlightCount: 2,
      observedAt: 2000,
      queuedCount: 1,
      tick: 9,
    },
  ],
  series: [
    {
      key: "queued",
      label: "Queued",
      unit: "count",
      points: [
        { label: "Queued: 3", observedAt: 1000, order: 0, value: 3 },
        { label: "Queued: 1", observedAt: 2000, order: 1, value: 1 },
      ],
    },
    {
      key: "inFlight",
      label: "In-flight",
      unit: "count",
      points: [
        { label: "In-flight: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "In-flight: 2", observedAt: 2000, order: 1, value: 2 },
      ],
    },
    {
      key: "completed",
      label: "Completed",
      unit: "count",
      points: [
        { label: "Completed: 2", observedAt: 1000, order: 0, value: 2 },
        { label: "Completed: 5", observedAt: 2000, order: 1, value: 5 },
      ],
    },
    {
      key: "failed",
      label: "Failed/retried",
      unit: "count",
      points: [
        { label: "Failed: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "Failed: 2", observedAt: 2000, order: 1, value: 2 },
      ],
    },
  ],
};

describe("WorkOutcomeWidget", () => {
  it("lets the chart card inherit the dashboard grid height instead of imposing an oversized shell minimum", () => {
    render(<WorkOutcomeWidget model={emptyTrend} />);

    const card = screen.getByRole("article", { name: "Work outcome chart" });
    expect(card.className).toContain("h-full");
    expect(card.className).toContain("min-h-0");
    expect(card.className).not.toContain("min-h-72");
  });

  it("routes the localized card title and header action through the sole bento header", () => {
    const messages = getWorkOutcomeMessages();
    render(
      <WorkOutcomeWidget
        headerAction={<button type="button">Remove chart</button>}
        model={populatedTrend}
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
  });

  it("keeps the localized card title in the bento header for zh-CN", () => {
    const messages = getWorkOutcomeMessages("zh-CN");
    render(
      <WorkOutcomeWidget
        locale="zh-CN"
        model={populatedTrend}
        widgetId="work-outcome-chart"
      />,
    );

    const card = screen.getByRole("article", {
      name: messages.chart.cardTitle,
    });
    expectSingleWorkOutcomeCardHeader(card, {
      cardRegionLabel: messages.chart.cardRegionLabel,
      cardTitle: messages.chart.cardTitle,
    });
  });
});
