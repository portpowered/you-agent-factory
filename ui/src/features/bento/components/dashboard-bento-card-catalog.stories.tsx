// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: Keeps the PRD-required top-level bento card catalog in one Storybook sidebar group.
import { type ReactNode, useEffect, useRef, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";

import type {
  DashboardProviderSessionAttempt,
  DashboardSnapshot,
  DashboardTrace,
} from "../../../api/dashboard/types";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import "../../../styles.css";
import { expectNoPageHorizontalOverflow } from "../../../stories/dashboardStorySupport";
import { CurrentSelectionWidget } from "../../current-selection/public";
import { useCurrentSelection } from "../../current-selection/hooks/useCurrentSelection";
import { useCurrentSelectionDetails } from "../../current-selection/hooks/useCurrentSelectionDetails";
import { useSelectedProviderSessionState } from "../../current-selection/work-selection/public";
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
import {
  WorkflowActivityWidget,
} from "../../workflow-activity/public";
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

const STORY_NOW = Date.parse("2026-04-08T12:05:00Z");
const providerSessionID = "sess-bento-card-catalog";
const providerSessionLoadingID = "sess-bento-card-loading";
const providerSessionEmptyID = "sess-bento-card-empty";
const providerSessionErrorID = "sess-bento-card-error";

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

const emptyWorkOutcomeModel: WorkChartModel = {
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

const emptyDashboardSnapshot: DashboardSnapshot = {
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

const providerSessionLoadingFetchMock = {
  method: "GET",
  path: providerSessionDetailPath(providerSessionLoadingID),
  response: () => new Promise<never>(() => undefined),
};

const providerSessionEmptyFetchMock = {
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

const providerSessionErrorFetchMock = {
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

const editableConfigurationPromptTemplateContract = {
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

function buildEditableConfigurationDocument(prompt = "Review the latest story changes before approval.") {
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

function promptTemplateValidationResponse(init?: RequestInit) {
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

function CurrentSelectionEditableConfigurationStory({
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
      <div style={{ display: "flex", flexWrap: "wrap", gap: "0.75rem", marginBottom: "1rem" }}>
        <button onClick={() => currentSelection.selectWorkstation("review")} type="button">
          Select Review workstation
        </button>
        <button onClick={() => currentSelection.selectWorkstation("plan")} type="button">
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
                selectedWorkExecutionDetails={details.selectedWorkExecutionDetails}
                selectedWorkRelationshipGraph={details.selectedWorkRelationshipGraph}
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

async function expectEditableConfigurationStoryFlow(
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
  await expect(
    within(card).getByText("Available variables"),
  ).toBeVisible();

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
    within(card).queryByDisplayValue("Use {{ .WorkID }} for browser verification."),
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

function InlineAddWidgetCardStory() {
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

function SubmitWorkInteractiveStory() {
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

function TraceDrilldownInteractiveStory() {
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

function renderProviderSessionStateCard({
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

function renderTraceStateCard(
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

function renderWorkOutcomeStateCard({
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

function renderSubmitWorkStatusCard({
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

function DashboardBentoResponsiveStory() {
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

export default {
  title: "you-agent-factory/Dashboard/Bento Cards",
  tags: ["test"],
};

export const WorkTotals = {
  render: () =>
    renderCardFrame({
      children: (
        <WorkTotalsWidget snapshot={semanticWorkflowDashboardSnapshot} />
      ),
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

export const WorkTotalsEmpty = {
  render: () =>
    renderCardFrame({
      children: <WorkTotalsWidget snapshot={emptyDashboardSnapshot} />,
      layout: layoutFor(DASHBOARD_WIDGET_IDS.workTotals, {
        h: 2,
        id: "work-totals-empty::story",
        w: 6,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Work totals" });

    await expect(within(card).getByLabelText("Completed: 0")).toBeVisible();
    await expect(within(card).getByLabelText("Failed: 0")).toBeVisible();
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
    await userEvent.click(
      await within(card).findByRole("button", {
        name: "Select Implement workstation",
      }),
    );
    await expect(
      await within(card).findByRole("button", {
        name: "Select Implement workstation",
      }),
    ).toHaveAttribute("aria-pressed", "true");
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

export const CurrentSelectionEditableConfigurationDesktop = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions/~default/factory",
          response: {
            body: buildEditableConfigurationDocument(),
          },
        },
        {
          method: "GET",
          path: "/factory-sessions/~default/factory/workstations/Review/prompt-template-contract",
          response: {
            body: editableConfigurationPromptTemplateContract,
          },
        },
        {
          method: "POST",
          path: "/factory-sessions/~default/factory/workstations/Review/prompt-template-validation",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: promptTemplateValidationResponse(init),
          }),
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => <CurrentSelectionEditableConfigurationStory width={960} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectEditableConfigurationStoryFlow(canvasElement);
  },
};

export const CurrentSelectionEditableConfigurationNarrow = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions/~default/factory",
          response: {
            body: buildEditableConfigurationDocument(),
          },
        },
        {
          method: "GET",
          path: "/factory-sessions/~default/factory/workstations/Review/prompt-template-contract",
          response: {
            body: editableConfigurationPromptTemplateContract,
          },
        },
        {
          method: "POST",
          path: "/factory-sessions/~default/factory/workstations/Review/prompt-template-validation",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: promptTemplateValidationResponse(init),
          }),
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => <CurrentSelectionEditableConfigurationStory width={360} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectEditableConfigurationStoryFlow(canvasElement);
    expectNoPageHorizontalOverflow(canvasElement);
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

export const ProviderSessionLoading = {
  ...renderProviderSessionStateCard({
    fetchMock: providerSessionLoadingFetchMock,
    sessionID: providerSessionLoadingID,
  }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Provider session",
    });

    await expect(await within(card).findByRole("status")).toHaveTextContent(
      "Loading session details...",
    );
  },
};

export const ProviderSessionEmpty = {
  render: () =>
    renderCardFrame({
      children: (
        <ProviderSessionWidget
          selectedProviderSession={null}
          widgetId="provider-session-empty::story"
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.providerSession, {
        h: 6,
        id: "provider-session-empty::story",
        w: 6,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Provider session",
    });

    await expect(
      within(card).getByText(
        "Select a provider session from work-item or workstation history to inspect session details.",
      ),
    ).toBeVisible();
  },
};

export const ProviderSessionEmptyFile = {
  ...renderProviderSessionStateCard({
    fetchMock: providerSessionEmptyFetchMock,
    sessionID: providerSessionEmptyID,
  }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Provider session",
    });

    await waitFor(() => {
      expect(within(card).getByRole("status")).toHaveTextContent(
        "The selected session file did not contain any Codex event records.",
      );
    });
  },
};

export const ProviderSessionError = {
  ...renderProviderSessionStateCard({
    fetchMock: providerSessionErrorFetchMock,
    sessionID: providerSessionErrorID,
  }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Provider session",
    });

    await expect(await within(card).findByRole("alert")).toHaveTextContent(
      "Storybook provider-session failure",
    );
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

export const TerminalWorkEmpty = {
  render: () =>
    renderCardFrame({
      children: (
        <TerminalWorkWidget
          completedItems={[]}
          failedItems={[]}
          onSelectItem={() => undefined}
          selectedItem={null}
          widgetId="terminal-work-empty::story"
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.terminalWork, {
        h: 5,
        id: "terminal-work-empty::story",
        w: 5,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Completed and failed work",
    });

    await expect(
      within(card).getByText("No completed work recorded yet."),
    ).toBeVisible();
    await expect(
      within(card).getByText("No failed work recorded yet."),
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

export const SubmitWork = {
  render: () =>
    renderCardFrame({
      children: (
        <SubmitWorkWidget
          submitWorkTypes={
            semanticWorkflowDashboardSnapshot.topology.submit_work_types
          }
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

export const SubmitWorkInteractive = {
  render: () => <SubmitWorkInteractiveStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });

    await expect(
      within(card).getByRole("button", {
        name: "Remove Submit work widget from dashboard",
      }),
    ).toBeVisible();
    await userEvent.selectOptions(
      within(card).getByRole("combobox", { name: "Work type" }),
      "story",
    );
    await userEvent.type(
      within(card).getByRole("textbox", { name: "Request name" }),
      "Interactive coverage",
    );
    await userEvent.type(
      within(card).getByRole("textbox", { name: "Text item 1" }),
      "Verify the bento card interaction path.",
    );
    await userEvent.click(
      within(card).getByRole("button", { name: "Submit work" }),
    );

    await expect(await within(card).findByRole("status")).toHaveTextContent(
      "Submitted Interactive coverage as story.",
    );
  },
};

export const SubmitWorkEmpty = {
  render: () =>
    renderSubmitWorkStatusCard({
      status: "empty",
      storyID: "submit-work-empty::story",
      submitWorkTypeNames: [],
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });

    await expect(
      within(card).getByText(
        "No work types are available to submit right now.",
      ),
    ).toBeVisible();
    await expect(
      within(card).getByRole("button", { name: "Submit work" }),
    ).toBeDisabled();
  },
};

export const SubmitWorkSubmitting = {
  render: () =>
    renderSubmitWorkStatusCard({
      isSubmitting: true,
      status: "submitting",
      storyID: "submit-work-submitting::story",
      submitWorkTypeNames: ["story"],
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });

    await expect(
      within(card).getByRole("button", { name: "Submitting..." }),
    ).toHaveAttribute("aria-busy", "true");
    await expect(
      within(card).getByText("Submitting work to the selected factory."),
    ).toBeVisible();
  },
};

export const SubmitWorkError = {
  render: () =>
    renderSubmitWorkStatusCard({
      status: "error",
      storyID: "submit-work-error::story",
      submitWorkTypeNames: ["story"],
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });

    await expect(await within(card).findByRole("alert")).toHaveTextContent(
      "Submission failed because the factory rejected the request.",
    );
    await expect(
      within(card).getByText("At least one text or file item is required."),
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

export const TraceDrilldownInteractive = {
  render: () => <TraceDrilldownInteractiveStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });

    await userEvent.click(within(card).getByRole("button", { name: "Expand" }));
    await userEvent.click(
      within(card).getAllByRole("button", { name: /Active Story/ })[0],
    );

    await expect(await canvas.findByRole("status")).toHaveTextContent(
      "Selected trace work item: work-active-story",
    );
  },
};

export const TraceDrilldownLoading = {
  render: () =>
    renderTraceStateCard({
      status: "loading",
      workID: "work-loading-story",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });

    await expect(within(card).getByText("Loading trace")).toBeVisible();
    await expect(
      within(card).getByText(
        "Reconstructing dispatch history for work-loading-story.",
      ),
    ).toBeVisible();
  },
};

export const TraceDrilldownEmpty = {
  render: () =>
    renderTraceStateCard({
      status: "empty",
      workID: "work-empty-story",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });

    await expect(
      within(card).getByText("Trace history unavailable"),
    ).toBeVisible();
    await expect(
      within(card).getByText(
        "No retained dispatch history is currently available for this work item.",
      ),
    ).toBeVisible();
  },
};

export const TraceDrilldownError = {
  render: () =>
    renderTraceStateCard({
      message: "Trace history request failed.",
      status: "error",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });

    await expect(within(card).getByText("Trace lookup failed")).toBeVisible();
    await expect(
      within(card).getByText("Trace history request failed."),
    ).toBeVisible();
  },
};

export const InlineAddWidget = {
  render: () => <InlineAddWidgetCardStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    const card = await canvas.findByRole("article", { name: "Add widget" });
    const addWidgetButton = within(card).getByRole("button", {
      name: "Add widget",
    });

    await expect(addWidgetButton).toBeVisible();
    addWidgetButton.focus();
    await expect(addWidgetButton).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    const dialog = await page.findByRole("dialog", {
      name: "Add dashboard widget",
    });

    await expect(dialog).toBeVisible();
    await expect(
      within(dialog).getByRole("button", {
        name: "Browse widgets: Current selection",
      }),
    ).toBeDisabled();
    await userEvent.click(
      within(dialog).getByRole("button", {
        name: "Browse widgets: Work totals",
      }),
    );
    await expect(await canvas.findByRole("status")).toHaveTextContent(
      "Selected widget: work-totals",
    );
    await expect(
      canvas.getByRole("button", { name: "Move Add widget" }),
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
