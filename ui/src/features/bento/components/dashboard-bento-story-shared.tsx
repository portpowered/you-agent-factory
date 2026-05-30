// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: Shared fixtures and render helpers for split bento card catalog stories.
import { type ReactNode, useEffect, useRef, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";

import type {
  DashboardProviderSessionAttempt,
  DashboardSnapshot,
  DashboardTrace,
} from "../../../api/dashboard/types";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  currentSelectionWorkContentsDashboardSnapshot,
  semanticWorkflowDashboardSnapshot,
} from "../../../components/dashboard/test-fixtures";

export {
  currentSelectionWorkContentsDashboardSnapshot,
  semanticWorkflowDashboardSnapshot,
} from "../../../components/dashboard/test-fixtures";

import { useCurrentSelection } from "../../current-selection/hooks/useCurrentSelection";
import { useCurrentSelectionDetails } from "../../current-selection/hooks/useCurrentSelectionDetails";
import { CurrentSelectionWidget } from "../../current-selection/public";
import { useSelectedProviderSessionState } from "../../current-selection/work-selection/hooks/useSelectedProviderSessionState";
import { InlineAddWidgetCard } from "../../dashboard-add-card/components/inline-add-widget-card";
import { ProviderSessionWidget } from "../../provider-session-detail/public";
import {
  SubmitWorkCard,
  type SubmitWorkDraft,
  type SubmitWorkStatus,
} from "../../submit-work/components/submit-work-card";
import { SubmitWorkWidget } from "../../submit-work/public";
import { TerminalWorkWidget } from "../../terminal-work/public";
import type { useTraceDrilldown } from "../../trace-drilldown/hooks/useTraceDrilldown";
import { TraceDrilldownWidget } from "../../trace-drilldown/public";
import type { WorkChartModel } from "../../work-outcome/lib/trends";
import { WorkOutcomeWidget } from "../../work-outcome/public";
import { WorkTotalsWidget } from "../../work-totals/public";
import { useCurrentActivityImportController } from "../../workflow-activity/hooks/current-activity-import-controller";
import { WorkflowActivityWidget } from "../../workflow-activity/public";
import {
  DASHBOARD_WIDGET_IDS,
  DEFAULT_DASHBOARD_LAYOUT,
} from "../hooks/dashboardLayoutSchema";
import {
  type DashboardWidgetPickerWidgetType,
  getDashboardWidgetPickerAvailability,
} from "../lib/dashboard-widget-picker";
import {
  AgentBentoLayout,
  type AgentBentoLayoutCard,
  type AgentBentoLayoutItem,
} from "./agent-bento";
import { DashboardWidgetRemoveButton } from "./dashboard-widget-remove-button";

export function expectBentoHeaderDragSurface(card: HTMLElement, title: string) {
  const header = card.querySelector("header");
  expect(header).toBeTruthy();
  expect(header?.getAttribute("data-bento-drag-handle")).toBe("true");
  expect(header?.className).toContain("cursor-grab");
  expect(
    within(card).queryByRole("button", { name: `Move ${title}` }),
  ).toBeNull();
  expect(
    within(card).getByRole("heading", { level: 3, name: title }),
  ).toBeVisible();
}

export const STORY_NOW = Date.parse("2026-04-08T12:05:00Z");
export const providerSessionID = "sess-bento-card-catalog";
export const providerSessionLoadingID = "sess-bento-card-loading";
export const providerSessionEmptyID = "sess-bento-card-empty";
export const providerSessionErrorID = "sess-bento-card-error";

export const populatedProviderSession = {
  dispatchID: "dispatch-review-active",
  id: providerSessionID,
  kind: "session_id",
  provider: "codex",
} as const;

export const storyTrace: DashboardTrace = {
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

export const completedAttempt: DashboardProviderSessionAttempt = {
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

export const failedAttempt: DashboardProviderSessionAttempt = {
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

export const workOutcomeModel: WorkChartModel = {
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

export const emptyWorkOutcomeModel: WorkChartModel = {
  delta: {
    completed: 0,
    failed: 0,
    inFlight: 0,
    queued: 0,
  },
  failureGroups: [],
  points: [],
  rangeID: "session",
  rangeLabel: "Session",
  samples: [],
  series: [],
};

export const emptyDashboardSnapshot: DashboardSnapshot = {
  ...semanticWorkflowDashboardSnapshot,
  runtime: {
    ...semanticWorkflowDashboardSnapshot.runtime,
    in_flight_dispatch_count: 0,
    session: {
      ...semanticWorkflowDashboardSnapshot.runtime.session,
      completed_count: 0,
      dispatched_count: 0,
      failed_count: 0,
    },
  },
};

export const providerSessionFetchMock = {
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

function providerSessionDetailPath(sessionID: string): string {
  return `/provider-sessions/detail?id=${sessionID}&kind=session_id&provider=codex`;
}

function providerSessionRef(sessionID: string) {
  return {
    dispatchID: `dispatch-${sessionID}`,
    id: sessionID,
    kind: "session_id",
    provider: "codex",
  } as const;
}

export const providerSessionLoadingFetchMock = {
  method: "GET",
  path: providerSessionDetailPath(providerSessionLoadingID),
  response: () => new Promise<never>(() => undefined),
};

export const providerSessionEmptyFetchMock = {
  method: "GET",
  path: providerSessionDetailPath(providerSessionEmptyID),
  response: {
    body: {
      parse: {
        eventCount: 0,
        functionCalls: [],
        lineCount: 0,
        malformedLineCount: 0,
        parseErrors: [],
        reasoning: [],
        tokenUsage: null,
        turns: [],
        unknownEventCount: 0,
        unknownEvents: [],
      },
      providerSession: {
        id: providerSessionEmptyID,
        kind: "session_id",
        provider: "codex",
      },
      source: {
        modifiedAt: "2026-04-08T12:00:01Z",
        relativePath: "2026/04/08/empty-session.jsonl",
        sizeBytes: 0,
      },
      transcript: [],
    },
  },
};

export const providerSessionErrorFetchMock = {
  method: "GET",
  path: providerSessionDetailPath(providerSessionErrorID),
  response: {
    body: {
      code: "INTERNAL_ERROR",
      message: "Storybook provider-session failure",
    },
    status: 500,
    statusText: "Internal Server Error",
  },
};

export const editableConfigurationPromptTemplateContract = {
  availableVariables: [
    {
      category: "ROOT",
      description: "The current work item identifier.",
      example: "{{ .WorkID }}",
      path: ".WorkID",
    },
    {
      category: "INPUT",
      description: "Payload for the first authored input.",
      example: "{{ (index .Inputs 0).Payload }}",
      path: ".Inputs[0].Payload",
    },
  ],
  inputCount: 1,
  unavailableAccessPatterns: [
    {
      example: "{{ (index .Inputs 1).Payload }}",
      path: ".Inputs[1].Payload",
      reason: "Only input 0 is available for this workstation.",
    },
  ],
};

export function buildEditableConfigurationDocument(
  prompt = "Review the latest story changes before approval.",
) {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-20T10:00:00Z",
    },
    workers: [
      {
        model: "gpt-5.5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5.6",
        name: "planner",
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [],
    workstations: [
      {
        body: prompt,
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: "reviewer",
      },
    ],
  };
}

export function promptTemplateValidationResponse(init?: RequestInit) {
  if (typeof init?.body !== "string") {
    return {
      diagnostics: [],
      valid: true,
    };
  }

  const requestBody = JSON.parse(init.body) as { prompt?: unknown };
  const prompt =
    typeof requestBody.prompt === "string" ? requestBody.prompt.trim() : "";

  if (prompt.includes(".Inputs[1]")) {
    return {
      diagnostics: [
        {
          kind: "UNAVAILABLE_VARIABLE",
          message: "Only input 0 is available.",
          path: ".Inputs[1]",
          sourceText: "(index .Inputs 1)",
          startOffset: 7,
          endOffset: 24,
        },
      ],
      valid: false,
    };
  }

  return {
    diagnostics: [],
    valid: true,
  };
}

export function layoutFor(
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

export function renderCardFrame({
  children,
  initialWidth = 960,
  layout,
}: {
  children: ReactNode;
  initialWidth?: number;
  layout: AgentBentoLayoutItem;
}) {
  return (
    <div
      style={{ maxWidth: `${initialWidth}px`, padding: "1rem", width: "100%" }}
    >
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

export function WorkflowGraphCardStory() {
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
        onSelectWorker={currentSelection.selectWorker}
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

export function CurrentSelectionWorkContentsCardStory() {
  const currentSelection = useCurrentSelection({
    sessionID: DEFAULT_FACTORY_SESSION_ID,
    snapshot: currentSelectionWorkContentsDashboardSnapshot,
    workstationRequestsByDispatchID:
      currentSelectionWorkContentsDashboardSnapshot.runtime
        .workstation_requests_by_dispatch_id,
  });
  const providerSessionState =
    useSelectedProviderSessionState(currentSelection);
  const details = useCurrentSelectionDetails({
    currentSelection,
    selectedTrace: storyTrace,
    snapshot: currentSelectionWorkContentsDashboardSnapshot,
    workstationRequestsByDispatchID:
      currentSelectionWorkContentsDashboardSnapshot.runtime
        .workstation_requests_by_dispatch_id,
  });

  useEffect(() => {
    currentSelection.selectWorkByID("work-active-story");
  }, [currentSelection.selectWorkByID]);

  return renderCardFrame({
    children: (
      <CurrentSelectionWidget
        activeTraceID={storyTrace.trace_id}
        currentSelection={currentSelection}
        failedWorkDetailsByWorkID={
          currentSelectionWorkContentsDashboardSnapshot.runtime.session
            .failed_work_details_by_work_id
        }
        now={STORY_NOW}
        onSelectProviderSession={
          providerSessionState.setSelectedProviderSession
        }
        onSelectTraceID={() => undefined}
        selectedProviderSessionKey={
          providerSessionState.selectedProviderSessionKey
        }
        selectedTrace={storyTrace}
        selectedWorkExecutionDetails={details.selectedWorkExecutionDetails}
        selectedWorkRelationshipGraph={details.selectedWorkRelationshipGraph}
        widgetId="current-selection-work-contents::story"
      />
    ),
    layout: layoutFor(DASHBOARD_WIDGET_IDS.currentSelection, {
      h: 6,
      id: "current-selection-work-contents::story",
      w: 6,
    }),
  });
}

export function CurrentSelectionCardStory() {
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
        onSelectProviderSession={
          providerSessionState.setSelectedProviderSession
        }
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

export function CurrentSelectionEditableConfigurationStory({
  width,
}: {
  width: number;
}) {
  const initializedSelectionRef = useRef(false);
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
    if (initializedSelectionRef.current) {
      return;
    }

    initializedSelectionRef.current = true;
    currentSelection.selectWorkstation("review");
  }, [currentSelection.selectWorkstation]);

  return (
    <div style={{ maxWidth: `${width}px`, padding: "1rem", width: "100%" }}>
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          gap: "0.75rem",
          marginBottom: "1rem",
        }}
      >
        <button
          onClick={() => currentSelection.selectWorkstation("review")}
          type="button"
        >
          Select Review workstation
        </button>
        <button
          onClick={() => currentSelection.selectWorkstation("plan")}
          type="button"
        >
          Select Plan workstation
        </button>
      </div>
      <AgentBentoLayout
        cards={[
          {
            children: (
              <CurrentSelectionWidget
                activeTraceID={storyTrace.trace_id}
                currentSelection={currentSelection}
                failedWorkDetailsByWorkID={
                  semanticWorkflowDashboardSnapshot.runtime.session
                    .failed_work_details_by_work_id
                }
                now={STORY_NOW}
                onSelectProviderSession={
                  providerSessionState.setSelectedProviderSession
                }
                onSelectTraceID={() => undefined}
                selectedProviderSessionKey={
                  providerSessionState.selectedProviderSessionKey
                }
                selectedTrace={storyTrace}
                selectedWorkExecutionDetails={
                  details.selectedWorkExecutionDetails
                }
                selectedWorkRelationshipGraph={
                  details.selectedWorkRelationshipGraph
                }
                widgetId="current-selection-editable-configuration::story"
              />
            ),
            id: "current-selection-editable-configuration::story",
            widgetType: DASHBOARD_WIDGET_IDS.currentSelection,
          },
        ]}
        initialWidth={width}
        layout={[
          layoutFor(DASHBOARD_WIDGET_IDS.currentSelection, {
            h: 7,
            id: "current-selection-editable-configuration::story",
            w: 6,
          }),
        ]}
        responsiveMode="interactive"
      />
    </div>
  );
}

export async function expectEditableConfigurationStoryFlow(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);
  const card = await canvas.findByRole("article", {
    name: "Current selection",
  });

  await expect(within(card).getByText("Review")).toBeVisible();

  const expandButton = within(card).getByRole("button", {
    name: "Expand editable configuration",
  });
  await userEvent.click(expandButton);

  const promptField = await within(card).findByRole("textbox", {
    name: "Prompt",
  });
  await expect(promptField).toBeVisible();
  await expect(within(card).getByText("Available variables")).toBeVisible();

  await userEvent.click(promptField, { pointerEventsCheck: 0 });
  await userEvent.keyboard("{Control>}{KeyA}{/Control}");
  await userEvent.paste("Use {{ .WorkID }} for browser verification.");

  await waitFor(() => {
    expect(
      within(card).getByRole("button", { name: "Save changes" }),
    ).toBeEnabled();
  });

  await userEvent.click(
    canvas.getByRole("button", { name: "Select Plan workstation" }),
  );

  await waitFor(() => {
    expect(within(card).getByText("Plan", { selector: "p" })).toBeVisible();
  });
  await userEvent.click(
    within(card).getByRole("button", { name: "Expand editable configuration" }),
  );
  await expect(
    within(card).getByText(
      "This running factory definition does not expose editable worker and prompt values for the selected workstation.",
    ),
  ).toBeVisible();
  await expect(
    within(card).getByRole("button", { name: "Save changes" }),
  ).toBeDisabled();
  expect(
    within(card).queryByDisplayValue(
      "Use {{ .WorkID }} for browser verification.",
    ),
  ).toBeNull();

  await userEvent.click(
    canvas.getByRole("button", { name: "Select Review workstation" }),
  );
  await waitFor(() => {
    expect(within(card).getByText("Review", { selector: "p" })).toBeVisible();
  });
  await userEvent.click(
    within(card).getByRole("button", { name: "Expand editable configuration" }),
  );
  await expect(
    await within(card).findByRole("textbox", { name: "Prompt" }),
  ).toHaveValue("Review the latest story changes before approval.");
}

export function InlineAddWidgetCardStory() {
  const [pickerOpen, setPickerOpen] = useState(false);
  const [selectedWidgetType, setSelectedWidgetType] =
    useState<DashboardWidgetPickerWidgetType | null>(null);

  return (
    <div>
      {renderCardFrame({
        children: (
          <InlineAddWidgetCard
            onPickerOpenChange={setPickerOpen}
            onSelectWidget={setSelectedWidgetType}
            pickerAvailability={getDashboardWidgetPickerAvailability([
              layoutFor(DASHBOARD_WIDGET_IDS.addWidget),
              layoutFor(DASHBOARD_WIDGET_IDS.currentSelection),
            ])}
            pickerOpen={pickerOpen}
          />
        ),
        layout: layoutFor(DASHBOARD_WIDGET_IDS.addWidget, {
          h: 4,
          id: "add-widget::story",
          w: 5,
        }),
      })}
      <p role="status">
        {selectedWidgetType
          ? `Selected widget: ${selectedWidgetType}`
          : "No widget selected yet."}
      </p>
    </div>
  );
}

export function SubmitWorkInteractiveStory() {
  const [draft, setDraft] = useState<SubmitWorkDraft>({
    items: [
      {
        id: "submit-work-interactive-text-item",
        text: "",
        type: "text" as const,
      },
    ],
    requestName: "",
    workTypeName: "",
  });
  const [status, setStatus] = useState<SubmitWorkStatus>({
    kind: "guidance" as const,
    message: "Fill out the request and submit work.",
  });

  return renderCardFrame({
    children: (
      <SubmitWorkCard
        draft={draft}
        headerAction={
          <DashboardWidgetRemoveButton
            onClick={() => undefined}
            widgetTitle="Submit work"
          />
        }
        onAddItem={(type) => {
          setDraft((currentDraft) => ({
            ...currentDraft,
            items: [
              ...currentDraft.items,
              type === "text"
                ? {
                    id: `submit-work-interactive-text-item-${currentDraft.items.length + 1}`,
                    text: "",
                    type: "text",
                  }
                : {
                    id: `submit-work-interactive-file-item-${currentDraft.items.length + 1}`,
                    stagingStatus: "idle",
                    type,
                  },
            ],
          }));
        }}
        onItemTextChange={(itemId, value) => {
          setDraft((currentDraft) => ({
            ...currentDraft,
            items: currentDraft.items.map((item) =>
              item.id === itemId && item.type === "text"
                ? { ...item, text: value }
                : item,
            ),
          }));
        }}
        onRemoveItem={(itemId) => {
          setDraft((currentDraft) => ({
            ...currentDraft,
            items: currentDraft.items.filter((item) => item.id !== itemId),
          }));
        }}
        onRequestNameChange={(requestName) => {
          setDraft((currentDraft) => ({ ...currentDraft, requestName }));
        }}
        onStageFileItems={() => undefined}
        onSubmit={() => {
          setStatus({
            kind: "success",
            message: `Submitted ${draft.requestName} as ${draft.workTypeName}.`,
          });
        }}
        onWorkTypeNameChange={(workTypeName) => {
          setDraft((currentDraft) => ({ ...currentDraft, workTypeName }));
        }}
        status={status}
        submitWorkTypeNames={["story", "bug"]}
        widgetId="submit-work-interactive::story"
      />
    ),
    layout: layoutFor(DASHBOARD_WIDGET_IDS.submitWork, {
      h: 6,
      id: "submit-work-interactive::story",
      w: 5,
    }),
  });
}

export function TraceDrilldownInteractiveStory() {
  const [selectedWorkID, setSelectedWorkID] = useState<string | null>(null);

  return (
    <div>
      {renderCardFrame({
        children: (
          <TraceDrilldownWidget
            onSelectWorkID={setSelectedWorkID}
            state={
              {
                status: "ready",
                trace: storyTrace,
              } satisfies ReturnType<typeof useTraceDrilldown>["traceGridState"]
            }
            widgetId="trace-interactive::story"
          />
        ),
        layout: layoutFor(DASHBOARD_WIDGET_IDS.trace, {
          h: 8,
          id: "trace-interactive::story",
          w: 8,
        }),
      })}
      <p role="status">
        {selectedWorkID
          ? `Selected trace work item: ${selectedWorkID}`
          : "No trace work item selected."}
      </p>
    </div>
  );
}

export function renderProviderSessionStateCard({
  fetchMock,
  sessionID,
}: {
  fetchMock: unknown;
  sessionID: string;
}) {
  return {
    parameters: {
      dashboardApi: {
        fetchMocks: [fetchMock],
        snapshot: semanticWorkflowDashboardSnapshot,
      },
    },
    render: () =>
      renderCardFrame({
        children: (
          <ProviderSessionWidget
            selectedProviderSession={providerSessionRef(sessionID)}
            widgetId={`provider-session-${sessionID}::story`}
          />
        ),
        layout: layoutFor(DASHBOARD_WIDGET_IDS.providerSession, {
          h: 6,
          id: `provider-session-${sessionID}::story`,
          w: 6,
        }),
      }),
  };
}

export function renderTraceStateCard(
  state: ReturnType<typeof useTraceDrilldown>["traceGridState"],
) {
  return renderCardFrame({
    children: (
      <TraceDrilldownWidget
        state={state}
        widgetId={`trace-${state.status}::story`}
      />
    ),
    layout: layoutFor(DASHBOARD_WIDGET_IDS.trace, {
      h: 8,
      id: `trace-${state.status}::story`,
      w: 8,
    }),
  });
}

export function renderWorkOutcomeStateCard({
  chartState,
  model,
  storyID,
}: {
  chartState?: {
    message?: string;
    status: "error" | "loading";
    title?: string;
  };
  model: WorkChartModel;
  storyID: string;
}) {
  return renderCardFrame({
    children: (
      <WorkOutcomeWidget
        chartState={chartState}
        model={model}
        widgetId={storyID}
      />
    ),
    layout: layoutFor(DASHBOARD_WIDGET_IDS.workOutcomeChart, {
      h: 6,
      id: storyID,
      w: 6,
    }),
  });
}

export function renderSubmitWorkStatusCard({
  isSubmitting = false,
  status,
  storyID,
  submitWorkTypeNames,
}: {
  isSubmitting?: boolean;
  status: "empty" | "error" | "submitting" | "success";
  storyID: string;
  submitWorkTypeNames: string[];
}) {
  const isEmpty = status === "empty";
  const isError = status === "error";

  return renderCardFrame({
    children: (
      <SubmitWorkCard
        draft={{
          items: [
            {
              id: `${storyID}-text-item`,
              text: isEmpty ? "" : "Review the state coverage story.",
              type: "text",
            },
          ],
          requestName: isEmpty ? "" : "State coverage",
          workTypeName: isEmpty ? "" : "story",
        }}
        isSubmitting={isSubmitting}
        onAddItem={() => undefined}
        onItemTextChange={() => undefined}
        onRemoveItem={() => undefined}
        onRequestNameChange={() => undefined}
        onStageFileItems={() => undefined}
        onSubmit={() => undefined}
        onWorkTypeNameChange={() => undefined}
        status={
          status === "submitting"
            ? {
                kind: "submitting",
                message: "Submitting work to the selected factory.",
              }
            : status === "success"
              ? {
                  kind: "success",
                  message: "Work submitted successfully.",
                }
              : isError
                ? {
                    kind: "error",
                    message:
                      "Submission failed because the factory rejected the request.",
                  }
                : {
                    kind: "guidance",
                    message: "No work types are available to submit right now.",
                  }
        }
        submitWorkTypeNames={submitWorkTypeNames}
        validationErrors={
          isError
            ? {
                submissionItems: "At least one text or file item is required.",
              }
            : undefined
        }
        widgetId={storyID}
      />
    ),
    layout: layoutFor(DASHBOARD_WIDGET_IDS.submitWork, {
      h: 6,
      id: storyID,
      w: 5,
    }),
  });
}

function responsiveCatalogLayout(): AgentBentoLayoutItem[] {
  return [
    layoutFor(DASHBOARD_WIDGET_IDS.workTotals, {
      h: 2,
      id: "work-totals::responsive",
      w: 4,
      x: 0,
      y: 0,
    }),
    layoutFor(DASHBOARD_WIDGET_IDS.submitWork, {
      h: 6,
      id: "submit-work::responsive",
      w: 4,
      x: 4,
      y: 0,
    }),
    layoutFor(DASHBOARD_WIDGET_IDS.workOutcomeChart, {
      h: 6,
      id: "work-outcome-chart::responsive",
      w: 4,
      x: 8,
      y: 0,
    }),
    layoutFor(DASHBOARD_WIDGET_IDS.workGraph, {
      h: 7,
      id: "work-graph::responsive",
      w: 8,
      x: 0,
      y: 2,
    }),
    layoutFor(DASHBOARD_WIDGET_IDS.currentSelection, {
      h: 5,
      id: "current-selection::responsive",
      w: 4,
      x: 8,
      y: 6,
    }),
    layoutFor(DASHBOARD_WIDGET_IDS.providerSession, {
      h: 6,
      id: "provider-session::responsive",
      w: 4,
      x: 0,
      y: 9,
    }),
    layoutFor(DASHBOARD_WIDGET_IDS.terminalWork, {
      h: 5,
      id: "terminal-work::responsive",
      w: 4,
      x: 4,
      y: 9,
    }),
    layoutFor(DASHBOARD_WIDGET_IDS.trace, {
      h: 8,
      id: "trace::responsive",
      w: 4,
      x: 8,
      y: 11,
    }),
    layoutFor(DASHBOARD_WIDGET_IDS.addWidget, {
      h: 4,
      id: "add-widget::responsive",
      w: 4,
      x: 0,
      y: 15,
    }),
  ];
}

interface ResponsiveCatalogContext {
  currentSelection: ReturnType<typeof useCurrentSelection>;
  details: ReturnType<typeof useCurrentSelectionDetails>;
  importController: ReturnType<typeof useCurrentActivityImportController>;
  layout: AgentBentoLayoutItem[];
  pickerOpen: boolean;
  providerSessionState: ReturnType<typeof useSelectedProviderSessionState>;
  setPickerOpen: (open: boolean) => void;
}

function responsiveCatalogMetricCards(): AgentBentoLayoutCard[] {
  return [
    {
      children: (
        <WorkTotalsWidget snapshot={semanticWorkflowDashboardSnapshot} />
      ),
      id: "work-totals::responsive",
      widgetType: DASHBOARD_WIDGET_IDS.workTotals,
    },
    {
      children: (
        <SubmitWorkWidget
          submitWorkTypes={
            semanticWorkflowDashboardSnapshot.topology.submit_work_types
          }
        />
      ),
      id: "submit-work::responsive",
      widgetType: DASHBOARD_WIDGET_IDS.submitWork,
    },
    {
      children: (
        <WorkOutcomeWidget
          model={workOutcomeModel}
          widgetId="work-outcome-chart::responsive"
        />
      ),
      id: "work-outcome-chart::responsive",
      widgetType: DASHBOARD_WIDGET_IDS.workOutcomeChart,
    },
  ];
}

function responsiveCatalogSelectionCards({
  currentSelection,
  details,
  importController,
  providerSessionState,
}: ResponsiveCatalogContext): AgentBentoLayoutCard[] {
  return [
    {
      children: (
        <WorkflowActivityWidget
          importController={importController}
          now={STORY_NOW}
          onSelectStateNode={currentSelection.selectStateNode}
          onSelectWorkID={currentSelection.selectWorkByID}
          onSelectWorker={currentSelection.selectWorker}
          onSelectWorkstation={currentSelection.selectWorkstation}
          selection={currentSelection.selection}
          snapshot={semanticWorkflowDashboardSnapshot}
          widgetInstanceID="work-graph::responsive"
        />
      ),
      id: "work-graph::responsive",
      widgetType: DASHBOARD_WIDGET_IDS.workGraph,
    },
    {
      children: (
        <CurrentSelectionWidget
          activeTraceID={storyTrace.trace_id}
          currentSelection={currentSelection}
          failedWorkDetailsByWorkID={
            semanticWorkflowDashboardSnapshot.runtime.session
              .failed_work_details_by_work_id
          }
          now={STORY_NOW}
          onSelectProviderSession={
            providerSessionState.setSelectedProviderSession
          }
          onSelectTraceID={() => undefined}
          selectedProviderSessionKey={
            providerSessionState.selectedProviderSessionKey
          }
          selectedTrace={storyTrace}
          selectedWorkExecutionDetails={details.selectedWorkExecutionDetails}
          selectedWorkRelationshipGraph={details.selectedWorkRelationshipGraph}
          widgetId="current-selection::responsive"
        />
      ),
      id: "current-selection::responsive",
      widgetType: DASHBOARD_WIDGET_IDS.currentSelection,
    },
  ];
}

function responsiveCatalogDetailCards({
  layout,
  pickerOpen,
  setPickerOpen,
}: ResponsiveCatalogContext): AgentBentoLayoutCard[] {
  return [
    {
      children: (
        <ProviderSessionWidget
          selectedProviderSession={populatedProviderSession}
          widgetId="provider-session::responsive"
        />
      ),
      id: "provider-session::responsive",
      widgetType: DASHBOARD_WIDGET_IDS.providerSession,
    },
    {
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
          widgetId="terminal-work::responsive"
        />
      ),
      id: "terminal-work::responsive",
      widgetType: DASHBOARD_WIDGET_IDS.terminalWork,
    },
    {
      children: (
        <TraceDrilldownWidget
          state={
            {
              status: "ready",
              trace: storyTrace,
            } satisfies ReturnType<typeof useTraceDrilldown>["traceGridState"]
          }
          widgetId="trace::responsive"
        />
      ),
      id: "trace::responsive",
      widgetType: DASHBOARD_WIDGET_IDS.trace,
    },
    {
      children: (
        <InlineAddWidgetCard
          onPickerOpenChange={setPickerOpen}
          onSelectWidget={() => undefined}
          pickerAvailability={getDashboardWidgetPickerAvailability(layout)}
          pickerOpen={pickerOpen}
        />
      ),
      id: "add-widget::responsive",
      widgetType: DASHBOARD_WIDGET_IDS.addWidget,
    },
  ];
}

export function HeaderConsistencyStory({
  initialWidth,
}: {
  initialWidth: number;
}) {
  const [pickerOpen, setPickerOpen] = useState(false);
  const layout: AgentBentoLayoutItem[] = [
    layoutFor(DASHBOARD_WIDGET_IDS.workTotals, {
      h: 2,
      id: "work-totals::header-consistency",
      w: 4,
      x: 0,
      y: 0,
    }),
    layoutFor(DASHBOARD_WIDGET_IDS.providerSession, {
      h: 5,
      id: "provider-session::header-consistency",
      w: 5,
      x: 4,
      y: 0,
    }),
    layoutFor(DASHBOARD_WIDGET_IDS.addWidget, {
      h: 4,
      id: "add-widget::header-consistency",
      w: 3,
      x: 9,
      y: 0,
    }),
    layoutFor(DASHBOARD_WIDGET_IDS.submitWork, {
      h: 6,
      id: "submit-work::header-consistency",
      w: 5,
      x: 0,
      y: 2,
    }),
  ];

  return (
    <div
      style={{ maxWidth: `${initialWidth}px`, padding: "1rem", width: "100%" }}
    >
      <AgentBentoLayout
        cards={[
          {
            children: (
              <WorkTotalsWidget snapshot={semanticWorkflowDashboardSnapshot} />
            ),
            id: "work-totals::header-consistency",
            widgetType: DASHBOARD_WIDGET_IDS.workTotals,
          },
          {
            children: (
              <ProviderSessionWidget
                selectedProviderSession={populatedProviderSession}
                widgetId="provider-session::header-consistency"
              />
            ),
            id: "provider-session::header-consistency",
            widgetType: DASHBOARD_WIDGET_IDS.providerSession,
          },
          {
            children: (
              <InlineAddWidgetCard
                onPickerOpenChange={setPickerOpen}
                onSelectWidget={() => undefined}
                pickerAvailability={getDashboardWidgetPickerAvailability(
                  layout,
                )}
                pickerOpen={pickerOpen}
              />
            ),
            id: "add-widget::header-consistency",
            widgetType: DASHBOARD_WIDGET_IDS.addWidget,
          },
          {
            children: (
              <SubmitWorkCard
                draft={{
                  items: [
                    {
                      id: "submit-work-header-consistency-text",
                      text: "Header consistency coverage",
                      type: "text",
                    },
                  ],
                  requestName: "Header consistency",
                  workTypeName: "story",
                }}
                headerAction={
                  <DashboardWidgetRemoveButton
                    onClick={() => undefined}
                    widgetTitle="Submit work"
                  />
                }
                onAddItem={() => undefined}
                onItemTextChange={() => undefined}
                onRemoveItem={() => undefined}
                onRequestNameChange={() => undefined}
                onStageFileItems={() => undefined}
                onSubmit={() => undefined}
                onWorkTypeNameChange={() => undefined}
                status={{
                  kind: "guidance",
                  message: "Header consistency story.",
                }}
                submitWorkTypeNames={["story"]}
                widgetId="submit-work::header-consistency"
              />
            ),
            id: "submit-work::header-consistency",
            widgetType: DASHBOARD_WIDGET_IDS.submitWork,
          },
        ]}
        initialWidth={initialWidth}
        layout={layout}
        responsiveMode="interactive"
      />
    </div>
  );
}

export function DashboardBentoResponsiveStory() {
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
  const [pickerOpen, setPickerOpen] = useState(false);
  const layout = responsiveCatalogLayout();
  const context = {
    currentSelection,
    details,
    importController,
    layout,
    pickerOpen,
    providerSessionState,
    setPickerOpen,
  };

  useEffect(() => {
    currentSelection.selectWorkstation("implement");
  }, [currentSelection.selectWorkstation]);

  return (
    <div style={{ boxSizing: "border-box", padding: "1rem", width: "100%" }}>
      <AgentBentoLayout
        cards={[
          ...responsiveCatalogMetricCards(),
          ...responsiveCatalogSelectionCards(context),
          ...responsiveCatalogDetailCards(context),
        ]}
        initialWidth={1180}
        layout={layout}
        responsiveMode="adaptive"
      />
    </div>
  );
}
