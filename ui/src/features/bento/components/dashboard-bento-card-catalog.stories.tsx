// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: Keeps the PRD-required top-level bento card catalog in one Storybook sidebar group.
import { useEffect, useState, type ReactNode } from "react";
import { expect, userEvent, within } from "storybook/test";

import type {
  DashboardProviderSessionAttempt,
  DashboardTrace,
} from "../../../api/dashboard/types";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  semanticWorkflowDashboardSnapshot,
} from "../../../components/dashboard/test-fixtures";
import type { AgentBentoLayoutItem } from "../../../components/ui";
import "../../../styles.css";
import {
  CurrentSelectionWidget,
  useCurrentSelection,
  useCurrentSelectionDetails,
  useSelectedProviderSessionState,
} from "../../current-selection/public";
import { ProviderSessionWidget } from "../../provider-session-detail/public";
import { SubmitWorkWidget } from "../../submit-work/public";
import { TerminalWorkWidget } from "../../terminal-work/public";
import {
  TraceDrilldownWidget,
  type useTraceDrilldown,
} from "../../trace-drilldown/public";
import { WorkOutcomeWidget } from "../../work-outcome/public";
import type { WorkChartModel } from "../../work-outcome/lib/trends";
import { WorkTotalsWidget } from "../../work-totals/public";
import {
  useCurrentActivityImportController,
  WorkflowActivityWidget,
} from "../../workflow-activity/public";
import {
  DASHBOARD_WIDGET_IDS,
  DEFAULT_DASHBOARD_LAYOUT,
} from "../hooks/dashboardLayoutSchema";
import { getDashboardWidgetPickerAvailability } from "../lib/dashboard-widget-picker";
import { AgentBentoLayout } from "./agent-bento";
import { InlineAddWidgetCard } from "./inline-add-widget-card";

const STORY_NOW = Date.parse("2026-04-08T12:05:00Z");
const providerSessionID = "sess-bento-card-catalog";

const populatedProviderSession = {
  dispatchID: "dispatch-review-active",
  id: providerSessionID,
  kind: "session_id",
  provider: "codex",
} as const;

const storyTrace: DashboardTrace = {
  trace_id: "trace-active-story",
  work_ids: ["work-active-story"],
  transition_ids: ["plan", "implement"],
  workstation_sequence: ["Plan", "Implement"],
  work_items: [
    {
      display_name: "Active Story",
      work_id: "work-active-story",
      work_type_id: "story",
    },
    {
      display_name: "Implemented Story",
      work_id: "work-implemented-story",
      work_type_id: "story",
    },
  ],
  dispatches: [
    {
      dispatch_id: "dispatch-review-active",
      duration_millis: 1000,
      end_time: "2026-04-08T12:00:01Z",
      input_items: [
        {
          display_name: "Active Story",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      outcome: "ACCEPTED",
      output_items: [
        {
          display_name: "Implemented Story",
          work_id: "work-implemented-story",
          work_type_id: "story",
        },
      ],
      start_time: "2026-04-08T12:00:00Z",
      transition_id: "implement",
      workstation_name: "Implement",
    },
  ],
};

const completedAttempt: DashboardProviderSessionAttempt = {
  dispatch_id: "dispatch-complete-story",
  outcome: "ACCEPTED",
  provider_session: {
    id: "sess-complete-story",
    kind: "session_id",
    provider: "codex",
  },
  transition_id: "complete",
  workstation_name: "Complete",
  work_items: [{ display_name: "Done Story", work_id: "work-done-story" }],
};

const failedAttempt: DashboardProviderSessionAttempt = {
  dispatch_id: "dispatch-repair-story",
  failure_message: "Provider returned a repair failure.",
  outcome: "FAILED",
  provider_session: {
    id: "sess-failed-story",
    kind: "session_id",
    provider: "codex",
  },
  transition_id: "repair",
  workstation_name: "Repair",
  work_items: [{ display_name: "Failed Story", work_id: "work-failed-story" }],
};

const workOutcomeModel: WorkChartModel = {
  delta: {
    completed: 4,
    failed: 2,
    inFlight: 2,
    queued: 1,
  },
  failureGroups: [{ count: 2, label: "Work type: story" }],
  points: [
    { label: "Tick 2", observedAt: 1000, order: 0, tick: 2 },
    { label: "Tick 5", observedAt: 2000, order: 1, tick: 5 },
    { label: "Tick 8", observedAt: 3000, order: 2, tick: 8 },
  ],
  rangeID: "session",
  rangeLabel: "Session",
  samples: [
    {
      completedCount: 2,
      dispatchedCount: 4,
      failedByWorkType: { story: 0 },
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 1,
      observedAt: 1000,
      queuedCount: 3,
      tick: 2,
    },
    {
      completedCount: 4,
      dispatchedCount: 7,
      failedByWorkType: { story: 1 },
      failedCount: 1,
      failedWorkLabels: ["Review rejected"],
      inFlightCount: 2,
      observedAt: 2000,
      queuedCount: 2,
      tick: 5,
    },
    {
      completedCount: 6,
      dispatchedCount: 10,
      failedByWorkType: { story: 2 },
      failedCount: 2,
      failedWorkLabels: ["Review rejected", "Repair failed"],
      inFlightCount: 3,
      observedAt: 3000,
      queuedCount: 4,
      tick: 8,
    },
  ],
  series: [
    {
      key: "queued",
      label: "Queued",
      points: [
        { label: "Queued: 3", observedAt: 1000, order: 0, value: 3 },
        { label: "Queued: 2", observedAt: 2000, order: 1, value: 2 },
        { label: "Queued: 4", observedAt: 3000, order: 2, value: 4 },
      ],
      unit: "count",
    },
    {
      key: "inFlight",
      label: "In-flight",
      points: [
        { label: "In-flight: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "In-flight: 2", observedAt: 2000, order: 1, value: 2 },
        { label: "In-flight: 3", observedAt: 3000, order: 2, value: 3 },
      ],
      unit: "count",
    },
    {
      key: "completed",
      label: "Completed",
      points: [
        { label: "Completed: 2", observedAt: 1000, order: 0, value: 2 },
        { label: "Completed: 4", observedAt: 2000, order: 1, value: 4 },
        { label: "Completed: 6", observedAt: 3000, order: 2, value: 6 },
      ],
      unit: "count",
    },
    {
      key: "failed",
      label: "Failed/retried",
      points: [
        { label: "Failed: 0", observedAt: 1000, order: 0, value: 0 },
        { label: "Failed: 1", observedAt: 2000, order: 1, value: 1 },
        { label: "Failed: 2", observedAt: 3000, order: 2, value: 2 },
      ],
      unit: "count",
    },
  ],
};

const providerSessionFetchMock = {
  method: "GET",
  path: `/provider-sessions/detail?id=${providerSessionID}&kind=session_id&provider=codex`,
  response: {
    body: {
      parse: {
        eventCount: 2,
        functionCalls: [],
        lineCount: 2,
        malformedLineCount: 0,
        parseErrors: [],
        reasoning: [],
        tokenUsage: {
          cachedInputTokens: 0,
          inputTokens: 14,
          outputTokens: 8,
          reasoningOutputTokens: 0,
          totalTokens: 22,
        },
        turns: [
          {
            eventCount: 2,
            functionCallCount: 0,
            index: 1,
            reasoningCount: 0,
            responseItemCount: 1,
            startedAt: "2026-04-08T12:00:00Z",
          },
        ],
        unknownEventCount: 0,
        unknownEvents: [],
      },
      providerSession: {
        id: providerSessionID,
        kind: "session_id",
        provider: "codex",
      },
      source: {
        modifiedAt: "2026-04-08T12:00:01Z",
        relativePath: "2026/04/08/session.jsonl",
        sizeBytes: 2048,
      },
      transcript: [
        {
          order: 1,
          text: "Inspect the current story and summarize reviewer concerns.",
          turnIndex: 1,
          type: "user_message",
        },
        {
          order: 2,
          text: "The bento card catalog is ready for isolated review.",
          turnIndex: 1,
          type: "assistant_message",
        },
      ],
    },
  },
};

function layoutFor(
  widgetType: string,
  overrides: Partial<AgentBentoLayoutItem> = {},
): AgentBentoLayoutItem {
  const defaultItem = DEFAULT_DASHBOARD_LAYOUT.find(
    (item) => item.widgetType === widgetType,
  );

  return {
    h: defaultItem?.h ?? 5,
    id: defaultItem?.id ?? `${widgetType}::story`,
    minH: defaultItem?.minH,
    minW: defaultItem?.minW,
    w: defaultItem?.w ?? 6,
    widgetType,
    x: 0,
    y: 0,
    ...overrides,
  };
}

function renderCardFrame({
  children,
  initialWidth = 960,
  layout,
}: {
  children: ReactNode;
  initialWidth?: number;
  layout: AgentBentoLayoutItem;
}) {
  return (
    <div style={{ maxWidth: `${initialWidth}px`, padding: "1rem", width: "100%" }}>
      <AgentBentoLayout
        cards={[
          {
            children,
            id: layout.id,
            widgetType: layout.widgetType,
          },
        ]}
        initialWidth={initialWidth}
        layout={[layout]}
        responsiveMode="interactive"
      />
    </div>
  );
}

function WorkflowGraphCardStory() {
  const currentSelection = useCurrentSelection({
    sessionID: DEFAULT_FACTORY_SESSION_ID,
    snapshot: semanticWorkflowDashboardSnapshot,
    workstationRequestsByDispatchID:
      semanticWorkflowDashboardSnapshot.runtime
        .workstation_requests_by_dispatch_id,
  });
  const importController = useCurrentActivityImportController({
    onFactoryActivated: () => undefined,
  });

  return renderCardFrame({
    children: (
      <WorkflowActivityWidget
        importController={importController}
        now={STORY_NOW}
        onSelectStateNode={currentSelection.selectStateNode}
        onSelectWorkID={currentSelection.selectWorkByID}
        onSelectWorkstation={currentSelection.selectWorkstation}
        selection={currentSelection.selection}
        snapshot={semanticWorkflowDashboardSnapshot}
        widgetInstanceID="work-graph::story"
      />
    ),
    initialWidth: 1080,
    layout: layoutFor(DASHBOARD_WIDGET_IDS.workGraph, {
      h: 8,
      id: "work-graph::story",
      w: 12,
    }),
  });
}

function CurrentSelectionCardStory() {
  const currentSelection = useCurrentSelection({
    sessionID: DEFAULT_FACTORY_SESSION_ID,
    snapshot: semanticWorkflowDashboardSnapshot,
    workstationRequestsByDispatchID:
      semanticWorkflowDashboardSnapshot.runtime
        .workstation_requests_by_dispatch_id,
  });
  const providerSessionState =
    useSelectedProviderSessionState(currentSelection);
  const details = useCurrentSelectionDetails({
    currentSelection,
    selectedTrace: storyTrace,
    snapshot: semanticWorkflowDashboardSnapshot,
    workstationRequestsByDispatchID:
      semanticWorkflowDashboardSnapshot.runtime
        .workstation_requests_by_dispatch_id,
  });

  useEffect(() => {
    currentSelection.selectWorkstation("implement");
  }, [currentSelection.selectWorkstation]);

  return renderCardFrame({
    children: (
      <CurrentSelectionWidget
        activeTraceID={storyTrace.trace_id}
        currentSelection={currentSelection}
        failedWorkDetailsByWorkID={
          semanticWorkflowDashboardSnapshot.runtime.session
            .failed_work_details_by_work_id
        }
        now={STORY_NOW}
        onSelectProviderSession={providerSessionState.setSelectedProviderSession}
        onSelectTraceID={() => undefined}
        selectedProviderSessionKey={
          providerSessionState.selectedProviderSessionKey
        }
        selectedTrace={storyTrace}
        selectedWorkExecutionDetails={details.selectedWorkExecutionDetails}
        selectedWorkRelationshipGraph={details.selectedWorkRelationshipGraph}
        widgetId="current-selection::story"
      />
    ),
    layout: layoutFor(DASHBOARD_WIDGET_IDS.currentSelection, {
      h: 5,
      id: "current-selection::story",
      w: 6,
    }),
  });
}

function InlineAddWidgetCardStory() {
  const [pickerOpen, setPickerOpen] = useState(false);

  return renderCardFrame({
    children: (
      <InlineAddWidgetCard
        onPickerOpenChange={setPickerOpen}
        onSelectWidget={() => undefined}
        pickerAvailability={getDashboardWidgetPickerAvailability([
          layoutFor(DASHBOARD_WIDGET_IDS.addWidget),
        ])}
        pickerOpen={pickerOpen}
      />
    ),
    layout: layoutFor(DASHBOARD_WIDGET_IDS.addWidget, {
      h: 4,
      id: "add-widget::story",
      w: 5,
    }),
  });
}

export default {
  title: "you-agent-factory/Dashboard/Bento Cards",
  tags: ["test"],
};

export const WorkTotals = {
  render: () =>
    renderCardFrame({
      children: <WorkTotalsWidget snapshot={semanticWorkflowDashboardSnapshot} />,
      layout: layoutFor(DASHBOARD_WIDGET_IDS.workTotals, {
        h: 2,
        id: "work-totals::story",
        w: 6,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Work totals" });

    await expect(within(card).getByText("Completed")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Move Work totals" }),
    ).toBeVisible();
  },
};

export const WorkflowGraph = {
  render: () => <WorkflowGraphCardStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Factory graph" });

    await expect(
      await within(card).findByRole("button", {
        name: "Select Implement workstation",
      }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Move Factory graph" }),
    ).toBeVisible();
  },
};

export const CurrentSelection = {
  render: () => <CurrentSelectionCardStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Current selection",
    });

    await expect(within(card).getByText("Implement")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Move Current selection" }),
    ).toBeVisible();
  },
};

export const ProviderSession = {
  parameters: {
    dashboardApi: {
      fetchMocks: [providerSessionFetchMock],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () =>
    renderCardFrame({
      children: (
        <ProviderSessionWidget
          selectedProviderSession={populatedProviderSession}
          widgetId="provider-session::story"
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.providerSession, {
        h: 6,
        id: "provider-session::story",
        w: 6,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Provider session",
    });

    await expect(await within(card).findByText("Transcript")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Move Provider session" }),
    ).toBeVisible();
  },
};

export const TerminalWork = {
  render: () =>
    renderCardFrame({
      children: (
        <TerminalWorkWidget
          completedItems={[
            {
              attempts: [completedAttempt],
              label: "Done Story",
              traceWorkID: "work-done-story",
            },
          ]}
          failedItems={[
            {
              attempts: [failedAttempt],
              label: "Failed Story",
              traceWorkID: "work-failed-story",
            },
          ]}
          onSelectItem={() => undefined}
          selectedItem={{ label: "Failed Story", status: "failed" }}
          widgetId="terminal-work::story"
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.terminalWork, {
        h: 5,
        id: "terminal-work::story",
        w: 5,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Completed and failed work",
    });

    await expect(
      within(card).getByRole("button", { name: "Failed Story" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Move Completed and failed work" }),
    ).toBeVisible();
  },
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

    await expect(
      within(card).getByRole("img", { name: "Work outcome chart for Session" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Move Work outcome chart" }),
    ).toBeVisible();
  },
};

export const SubmitWork = {
  render: () =>
    renderCardFrame({
      children: (
        <SubmitWorkWidget
          submitWorkTypes={semanticWorkflowDashboardSnapshot.topology.submit_work_types}
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.submitWork, {
        h: 6,
        id: "submit-work::story",
        w: 5,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });

    await expect(
      within(card).getByRole("combobox", { name: "Work type" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Move Submit work" }),
    ).toBeVisible();
  },
};

export const TraceDrilldown = {
  render: () =>
    renderCardFrame({
      children: (
        <TraceDrilldownWidget
          state={
            {
              status: "ready",
              trace: storyTrace,
            } satisfies ReturnType<typeof useTraceDrilldown>["traceGridState"]
          }
          widgetId="trace::story"
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.trace, {
        h: 8,
        id: "trace::story",
        w: 8,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });

    await expect(within(card).getByText("trace-active-story")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Move Trace drill-down" }),
    ).toBeVisible();
  },
};

export const InlineAddWidget = {
  render: () => <InlineAddWidgetCardStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    const card = await canvas.findByRole("article", { name: "Add widget" });

    await expect(
      within(card).getByRole("button", { name: "Add widget" }),
    ).toBeVisible();
    await userEvent.click(
      within(card).getByRole("button", { name: "Add widget" }),
    );
    await expect(
      await page.findByRole("dialog", { name: "Add dashboard widget" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Move Add widget" }),
    ).toBeVisible();
  },
};
