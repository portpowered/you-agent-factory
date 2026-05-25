import type { ReactNode } from "react";
import { expect, within } from "storybook/test";

import "../../../styles.css";
import {
  AgentBentoLayout,
  type AgentBentoLayoutItem,
} from "../../bento/components/agent-bento";
import type { WorkChartModel } from "../lib/trends";
import { D3CompletionInformationCard } from "./d3-information-card";
import type { WorkChartState } from "./work-chart";
import { WorkOutcomeWidget } from "./work-outcome-widget";

const populatedTrend: WorkChartModel = {
  delta: {
    queued: 2,
    inFlight: 3,
    completed: 4,
    failed: 2,
  },
  failureGroups: [{ count: 2, label: "Work type: story" }],
  points: [
    {
      label: "Tick 10",
      observedAt: 1000,
      order: 0,
      tick: 10,
    },
    {
      label: "Tick 20",
      observedAt: 2000,
      order: 1,
      tick: 20,
    },
    {
      label: "Tick 40",
      observedAt: 3000,
      order: 2,
      tick: 40,
    },
  ],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [
    {
      completedCount: 2,
      dispatchedCount: 3,
      failedByWorkType: { story: 0 },
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 1,
      observedAt: 1000,
      queuedCount: 4,
      tick: 10,
    },
    {
      completedCount: 3,
      dispatchedCount: 5,
      failedByWorkType: { story: 1 },
      failedCount: 1,
      failedWorkLabels: ["story review rejected"],
      inFlightCount: 2,
      observedAt: 2000,
      queuedCount: 3,
      tick: 20,
    },
    {
      completedCount: 6,
      dispatchedCount: 9,
      failedByWorkType: { story: 2 },
      failedCount: 2,
      failedWorkLabels: ["story review rejected"],
      inFlightCount: 3,
      observedAt: 3000,
      queuedCount: 2,
      tick: 40,
    },
  ],
  series: [
    {
      key: "queued",
      label: "Queued",
      unit: "count",
      points: [
        { label: "Queued: 4", observedAt: 1000, order: 0, value: 4 },
        { label: "Queued: 3", observedAt: 2000, order: 1, value: 3 },
        { label: "Queued: 2", observedAt: 3000, order: 2, value: 2 },
      ],
    },
    {
      key: "inFlight",
      label: "In-flight",
      unit: "count",
      points: [
        { label: "In-flight: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "In-flight: 2", observedAt: 2000, order: 1, value: 2 },
        { label: "In-flight: 3", observedAt: 3000, order: 2, value: 3 },
      ],
    },
    {
      key: "completed",
      label: "Completed",
      unit: "count",
      points: [
        { label: "Completed: 2", observedAt: 1000, order: 0, value: 2 },
        { label: "Completed: 3", observedAt: 2000, order: 1, value: 3 },
        { label: "Completed: 6", observedAt: 3000, order: 2, value: 6 },
      ],
    },
    {
      key: "failed",
      label: "Failed/retried",
      unit: "count",
      points: [
        { label: "Failed: 0", observedAt: 1000, order: 0, value: 0 },
        { label: "Failed: 1", observedAt: 2000, order: 1, value: 1 },
        { label: "Failed: 2", observedAt: 3000, order: 2, value: 2 },
      ],
    },
  ],
};

const emptyTrend: WorkChartModel = {
  delta: {
    queued: 0,
    inFlight: 0,
    completed: 0,
    failed: 0,
  },
  failureGroups: [],
  points: [],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [],
  series: [],
};

const RESIZABLE_WORK_OUTCOME_LAYOUT: AgentBentoLayoutItem[] = [
  {
    h: 3,
    id: "work-outcome-chart",
    widgetType: "work-outcome-chart",
    w: 6,
    x: 0,
    y: 0,
  },
];

async function expectWorkOutcomeChartContract(card: HTMLElement): Promise<void> {
  const chart = within(card).getByRole("img", {
    name: "Work outcome chart for 15m",
  });

  expect(chart).toBeVisible();
  expect(within(card).getByText("Ticks")).toBeVisible();
  expect(within(card).getByText("Work count")).toBeVisible();
  expect(chart.getAttribute("data-work-chart-ready")).toBe("true");
  expect(chart.getAttribute("data-work-chart-visible-ticks")).toBe("10,20,40");
  expect(chart.className).toContain("px-5");
  expect(chart.className).toContain("pb-5");
  expect(chart.className).toContain("pt-4");
  expect(chart.className).toContain("sm:px-6");
  expect(chart.className).toContain("sm:pb-6");
  expect(chart.className).toContain("sm:pt-5");

  const overlay = chart.querySelector<HTMLElement>(
    "[data-work-chart-overlay='true']",
  );

  expect(overlay).not.toBeNull();
  expect(overlay?.className).toContain("px-5");
  expect(overlay?.className).toContain("pb-4");
  expect(overlay?.className).toContain("pt-4");
  expect(overlay?.className).toContain("sm:px-6");
  expect(overlay?.className).toContain("sm:pb-5");
  expect(overlay?.className).toContain("sm:pt-5");
  expect(
    chart.querySelector(".recharts-responsive-container"),
  ).not.toBeNull();
}

function expectNoOverflowInStoryShell(canvasElement: HTMLElement): void {
  const shell = canvasElement.querySelector<HTMLElement>("[data-story-shell]");

  expect(shell).not.toBeNull();
  expect(shell ? shell.getBoundingClientRect().width : 0).toBeLessThanOrEqual(
    360,
  );
  expect((shell?.scrollWidth ?? 0) <= (shell?.clientWidth ?? 0) + 1).toBe(true);
}

function renderWorkOutcomeStoryShell({
  children,
  height,
  maxWidth = "640px",
}: {
  children: ReactNode;
  height?: string;
  maxWidth?: string;
}) {
  return (
    <div
      data-story-shell="work-outcome"
      style={{ height, maxWidth, padding: "1rem", width: "100%" }}
    >
      {children}
    </div>
  );
}

function renderResizableWorkOutcomeStoryShell({
  chartState,
  maxWidth = "1180px",
  model,
}: {
  chartState?: WorkChartState;
  maxWidth?: string;
  model: WorkChartModel;
}) {
  return (
    <div
      data-story-shell="work-outcome"
      style={{ height: "48rem", maxWidth, padding: "1rem", width: "100%" }}
    >
      <AgentBentoLayout
        cards={[
          {
            id: "work-outcome-chart",
            widgetType: "work-outcome-chart",
            children: chartState ? (
              <D3CompletionInformationCard
                chartState={chartState}
                model={model}
                widgetId="work-outcome-chart-story"
              />
            ) : (
              <WorkOutcomeWidget
                model={model}
                widgetId="work-outcome-chart-story"
              />
            ),
          },
        ]}
        initialWidth={maxWidth === "360px" ? 360 : 1180}
        layout={RESIZABLE_WORK_OUTCOME_LAYOUT}
        responsiveMode="interactive"
      />
    </div>
  );
}

async function expectChartResizeChrome(
  canvasElement: HTMLElement,
  handleSelector: ".react-resizable-handle-s" | ".react-resizable-handle-se",
): Promise<void> {
  const canvas = within(canvasElement);
  const card = await canvas.findByRole("article", {
    name: "Work outcome chart",
  });
  const gridItem = card.closest<HTMLElement>("[data-bento-card-id='work-outcome-chart']");

  if (!(gridItem instanceof HTMLElement)) {
    throw new Error("expected work outcome grid item");
  }

  const resizeHandle = gridItem.querySelector<HTMLElement>(handleSelector);
  if (!(resizeHandle instanceof HTMLElement)) {
    throw new Error(`expected ${handleSelector} resize handle`);
  }
  expect(gridItem.querySelector(handleSelector)).not.toBeNull();
  await expectWorkOutcomeChartContract(card);
}

function expectCenteredStatusPanel(statePanel: HTMLElement): void {
  expect(statePanel.className).toContain("h-full");
  expect(statePanel.className).toContain("flex-1");
  expect(statePanel.className).toContain("justify-center");
  expect(statePanel).toBeVisible();
}

async function expectStatusPanelResizeChrome(
  canvasElement: HTMLElement,
  {
    expectedRole,
    handleSelector,
  }: {
    expectedRole: "alert" | "status";
    handleSelector: ".react-resizable-handle-s" | ".react-resizable-handle-se";
  },
): Promise<void> {
  const canvas = within(canvasElement);
  const card = await canvas.findByRole("article", {
    name: "Work outcome chart",
  });
  const gridItem = card.closest<HTMLElement>("[data-bento-card-id='work-outcome-chart']");

  if (!(gridItem instanceof HTMLElement)) {
    throw new Error("expected work outcome grid item");
  }

  const resizeHandle = gridItem.querySelector<HTMLElement>(handleSelector);
  const statePanel = within(card).getByRole(expectedRole);

  if (!(resizeHandle instanceof HTMLElement)) {
    throw new Error(`expected ${handleSelector} resize handle`);
  }

  expectCenteredStatusPanel(statePanel);
  expect(gridItem.querySelector(handleSelector)).not.toBeNull();
  expectCenteredStatusPanel(statePanel);
}

export default {
  title: "Agent Factory/Dashboard/Work Outcome Chart Card",
  component: D3CompletionInformationCard,
};

export const Populated = {
  render: () =>
    renderWorkOutcomeStoryShell({
      children: (
        <D3CompletionInformationCard
          model={populatedTrend}
          widgetId="work-outcome-chart-story"
        />
      ),
      height: "28rem",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });

    await expectWorkOutcomeChartContract(card);
  },
};

export const EmptyData = {
  render: () =>
    renderWorkOutcomeStoryShell({
      children: (
        <D3CompletionInformationCard
          model={emptyTrend}
          widgetId="work-outcome-chart-empty-story"
        />
      ),
      height: "28rem",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });

    const emptyState = within(card).getByRole("status");
    expectCenteredStatusPanel(emptyState);
  },
};

export const LoadingData = {
  render: () =>
    renderWorkOutcomeStoryShell({
      children: (
        <D3CompletionInformationCard
          chartState={{ status: "loading" }}
          model={emptyTrend}
          widgetId="work-outcome-chart-loading-story"
        />
      ),
      height: "32rem",
      maxWidth: "360px",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });

    const loadingState = within(card).getByRole("status");
    expectCenteredStatusPanel(loadingState);
    expectNoOverflowInStoryShell(canvasElement);
  },
};

export const ErrorState = {
  render: () =>
    renderWorkOutcomeStoryShell({
      children: (
        <D3CompletionInformationCard
          chartState={{ status: "error" }}
          model={emptyTrend}
          widgetId="work-outcome-chart-error-story"
        />
      ),
      height: "32rem",
      maxWidth: "360px",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });

    const alert = within(card).getByRole("alert");
    expectCenteredStatusPanel(alert);
    expectNoOverflowInStoryShell(canvasElement);
  },
};

export const ConstrainedWidth = {
  render: () =>
    renderWorkOutcomeStoryShell({
      children: (
        <D3CompletionInformationCard
          model={populatedTrend}
          widgetId="work-outcome-chart-narrow-story"
        />
      ),
      height: "28rem",
      maxWidth: "360px",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });

    await expectWorkOutcomeChartContract(card);
    expectNoOverflowInStoryShell(canvasElement);
  },
};

export const LocalizedZhCN = {
  render: () =>
    renderWorkOutcomeStoryShell({
      children: (
        <D3CompletionInformationCard
          locale="zh-CN"
          model={populatedTrend}
          widgetId="work-outcome-chart-zh-cn-story"
        />
      ),
      height: "28rem",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "工作结果图表" });
    const chart = within(card).getByRole("img", { name: "15m 的工作结果图表" });

    await expect(chart).toBeVisible();
    await expect(within(card).getByText("刻度")).toBeVisible();
    await expect(within(card).getByText("工作计数")).toBeVisible();
    expect(chart.getAttribute("data-work-chart-ready")).toBe("true");
  },
};

export const ResizableDesktop = {
  render: () =>
    renderResizableWorkOutcomeStoryShell({
      model: populatedTrend,
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectChartResizeChrome(
      canvasElement,
      ".react-resizable-handle-s",
    );
  },
};

export const ResizableConstrainedWidth = {
  render: () =>
    renderResizableWorkOutcomeStoryShell({
      maxWidth: "360px",
      model: populatedTrend,
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectChartResizeChrome(
      canvasElement,
      ".react-resizable-handle-se",
    );
    expectNoOverflowInStoryShell(canvasElement);
  },
};

export const ResizableEmptyState = {
  render: () =>
    renderResizableWorkOutcomeStoryShell({
      chartState: { status: "empty" },
      model: emptyTrend,
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectStatusPanelResizeChrome(canvasElement, {
      expectedRole: "status",
      handleSelector: ".react-resizable-handle-s",
    });
  },
};

export const ResizableLoadingStateConstrainedWidth = {
  render: () =>
    renderResizableWorkOutcomeStoryShell({
      chartState: { status: "loading" },
      maxWidth: "360px",
      model: emptyTrend,
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectStatusPanelResizeChrome(canvasElement, {
      expectedRole: "status",
      handleSelector: ".react-resizable-handle-se",
    });
    expectNoOverflowInStoryShell(canvasElement);
  },
};

export const ResizableErrorState = {
  render: () =>
    renderResizableWorkOutcomeStoryShell({
      chartState: { status: "error" },
      model: emptyTrend,
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectStatusPanelResizeChrome(canvasElement, {
      expectedRole: "alert",
      handleSelector: ".react-resizable-handle-s",
    });
  },
};
