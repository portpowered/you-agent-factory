import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  act,
  waitFor,
  within,
} from "@testing-library/react";

import { App } from "./App";
import * as factoryPngExportModule from "./features/export/factory-png-export";
import * as factoryPngImportModule from "./features/import/factory-png-import";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
} from "./components/dashboard";
import {
  buildDashboardSnapshotFixture,
  dashboardWorkstationRequestFixtures,
  failureAnalysisTimelineEvents,
  graphStateSmokeTimelineEvents,
  mediumBranchingDashboardTopology,
  oneNodeDashboardTopology,
  resourceCountAvailablePlaceID,
  resourceCountBackendWorldViewCountsByTick,
  resourceCountTimelineEvents,
  scriptDashboardIntegrationBackendWorkstationRequestsByDispatchID,
  scriptDashboardIntegrationFixtureIDs,
  scriptDashboardIntegrationTimelineEvents,
  runtimeDetailsBackendWorkstationRequestsByDispatchID,
  runtimeDetailsFixtureIDs,
  runtimeDetailsTimelineEvents,
} from "./components/dashboard/fixtures";
import { formatDurationMillis } from "./components/dashboard/formatters";
import {
  semanticWorkflowDashboardSnapshot,
  twentyNodeDashboardSnapshot,
} from "./components/dashboard/test-fixtures";
import { installDashboardBrowserTestShims } from "./components/dashboard/test-browser-shims";
import type {
  DashboardRuntimeWorkstationRequest,
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "./api/dashboard";
import { FACTORY_EVENT_TYPES } from "./api/events";
import type { FactoryEvent } from "./api/events";
import type { NamedFactoryValue } from "./api/named-factory";
import type { FactoryPngImportValue } from "./features/import";
import { TraceDrilldownWidget, useTraceDrilldown } from "./features/trace-drilldown";
import { buildFactoryTimelineSnapshot, useFactoryTimelineStore } from "./state/factoryTimelineStore";
import type { FactoryTimelineSnapshot } from "./state/factoryTimelineStore";

class MockEventSource {
  public static instances: MockEventSource[] = [];

  public onerror: ((event: Event) => void) | null = null;
  public onopen: ((event: Event) => void) | null = null;

  private readonly listeners = new Map<string, EventListener[]>();

  constructor(public readonly url: string) {
    MockEventSource.instances.push(this);
  }

  public addEventListener(type: string, listener: EventListener): void {
    const existing = this.listeners.get(type) ?? [];
    existing.push(listener);
    this.listeners.set(type, existing);
  }

  public close(): void {}

  public emit(type: string, data: unknown): void {
    if (type === "snapshot") {
      const state = useFactoryTimelineStore.getState();
      const tracesByWorkID = state.worldViewCache[state.selectedTick]?.tracesByWorkID ?? {};
      seedTimelineSnapshot(data as DashboardSnapshot, tracesByWorkID);
    }

    const event = new MessageEvent(type, {
      data: JSON.stringify(data),
    });

    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}

interface RenderAppOptions {
  snapshot: DashboardSnapshot;
  timelineEvents?: FactoryEvent[];
  timelineSnapshots?: DashboardSnapshot[];
  traceFixtures?: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

function TraceDrilldownTestHarness({
  selectedWorkID,
}: {
  selectedWorkID: string;
}) {
  const { traceGridState } = useTraceDrilldown(selectedWorkID);

  return <TraceDrilldownWidget state={traceGridState} />;
}

const activeWorkID = "work-active-story";
const completedWorkID = "work-complete";
const failedWorkID = "work-failed-story";
const activeWorkLabel = "Active Story";
const eventTimelineWorkID = "work-event-story";
const eventTimelineTraceID = "trace-event-story";
const eventTimelineDispatchID = "dispatch-event-story";
const ONE_PIXEL_PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP9KobjigAAAABJRU5ErkJggg==";
const fanInResultWorkID = "work-result";
const fanInResultLabel = "Implemented Story";

const baselineSnapshot = buildDashboardSnapshotFixture(mediumBranchingDashboardTopology);
const activeSnapshot = semanticWorkflowDashboardSnapshot;
const activeSnapshotWithoutTraceID = removeTraceIDsFromSnapshot(activeSnapshot);
const terminalBaseSnapshot = semanticWorkflowDashboardSnapshot;
const terminalSnapshot = {
  ...terminalBaseSnapshot,
  tick_count: 4,
  runtime: {
    ...terminalBaseSnapshot.runtime,
    place_occupancy_work_items_by_place_id: {
      ...(terminalBaseSnapshot.runtime.place_occupancy_work_items_by_place_id ?? {}),
      "story:blocked": [
        {
          display_name: "Failed Story",
          trace_id: "trace-failed-story",
          work_id: failedWorkID,
          work_type_id: "story",
        },
      ],
      "story:complete": [
        {
          display_name: "Done Story",
          trace_id: "trace-done-story",
          work_id: completedWorkID,
          work_type_id: "story",
        },
      ],
    },
    place_token_counts: {
      ...(terminalBaseSnapshot.runtime.place_token_counts ?? {}),
      "story:blocked": 1,
      "story:complete": 1,
    },
    session: {
      ...terminalBaseSnapshot.runtime.session,
      completed_count: 1,
      completed_work_labels: ["Done Story"],
      provider_sessions: [
        ...(terminalBaseSnapshot.runtime.session.provider_sessions ?? []),
        {
          dispatch_id: "dispatch-complete",
          outcome: "ACCEPTED",
          provider_session: {
            id: "sess-done-story",
            kind: "session_id",
            provider: "codex",
          },
          transition_id: "complete",
          workstation_name: "Complete",
          work_items: [
            {
              display_name: "Done Story",
              trace_id: "trace-done-story",
              work_id: completedWorkID,
              work_type_id: "story",
            },
          ],
        },
      ],
    },
  },
} satisfies DashboardSnapshot;
const historicalTimelineSnapshot = {
  ...baselineSnapshot,
  tick_count: 1,
} satisfies DashboardSnapshot;
const importedFactorySnapshot = (() => {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);

  snapshot.factory_state = "Imported factory active";
  snapshot.tick_count = semanticWorkflowDashboardSnapshot.tick_count + 1;
  snapshot.topology.workstation_nodes_by_id.review.workstation_name = "Imported Review";

  return snapshot;
})();

const { edges: _omittedEdges, ...oneNodeTopologyWithoutEdges } = oneNodeDashboardTopology;
const singleNodeSnapshotWithoutEdges = {
  ...buildDashboardSnapshotFixture(oneNodeDashboardTopology),
  topology: oneNodeTopologyWithoutEdges,
} as unknown as DashboardSnapshot;

const twentyNodeSnapshot = twentyNodeDashboardSnapshot;

const traceSnapshot: DashboardTrace = {
  trace_id: "trace-active-story",
  work_ids: [activeWorkID],
  transition_ids: ["plan", "review"],
  workstation_sequence: ["Plan", "Review"],
  dispatches: [
    {
      dispatch_id: "dispatch-review-active",
      transition_id: "plan",
      workstation_name: "Plan",
      outcome: "ACCEPTED",
      provider_session: {
        provider: "codex",
        kind: "session_id",
        id: "sess-active-story",
      },
      start_time: "2026-04-08T12:00:00Z",
      end_time: "2026-04-08T12:00:01Z",
      duration_millis: 1000,
      consumed_tokens: [
        {
          token_id: "tok-plan-in",
          place_id: "story:init",
          work_id: activeWorkID,
          work_type_id: "story",
          trace_id: "trace-active-story",
          created_at: "2026-04-08T11:59:58Z",
          entered_at: "2026-04-08T11:59:59Z",
        },
      ],
      output_mutations: [
        {
          type: "MOVE",
          token_id: "tok-plan-in",
          from_place: "story:init",
          to_place: "story:ready",
          resulting_token: {
            token_id: "tok-plan-out",
            place_id: "story:ready",
            work_id: activeWorkID,
            work_type_id: "story",
            trace_id: "trace-active-story",
            created_at: "2026-04-08T12:00:01Z",
            entered_at: "2026-04-08T12:00:01Z",
          },
        },
      ],
    },
  ],
};

const reworkTraceSnapshot: DashboardTrace = {
  ...traceSnapshot,
  transition_ids: ["plan", "review", "plan"],
  workstation_sequence: ["Plan", "Review", "Plan"],
  dispatches: [
    ...traceSnapshot.dispatches,
    {
      dispatch_id: "dispatch-review-rejected",
      transition_id: "review",
      workstation_name: "Review",
      outcome: "REJECTED",
      start_time: "2026-04-08T12:00:01Z",
      end_time: "2026-04-08T12:03:13Z",
      duration_millis: 192_000,
      consumed_tokens: [],
      output_mutations: [
        {
          type: "MOVE",
          token_id: "tok-review-in",
          from_place: "story:implemented",
          to_place: "story:ready",
          reason: "review rejected story",
        },
      ],
    },
  ],
};

const completedTraceSnapshot: DashboardTrace = {
  ...traceSnapshot,
  trace_id: "trace-done-story",
  work_ids: [completedWorkID],
  workstation_sequence: ["Complete"],
  dispatches: [
    {
      ...traceSnapshot.dispatches[0],
      dispatch_id: "dispatch-done-story",
      workstation_name: "Complete",
    },
  ],
};

const failedTraceSnapshot: DashboardTrace = {
  ...traceSnapshot,
  trace_id: "trace-failed-story",
  work_ids: ["work-failed-story"],
  workstation_sequence: ["Review", "Failure"],
  dispatches: [
    {
      ...traceSnapshot.dispatches[0],
      dispatch_id: "dispatch-failed-story",
      outcome: "FAILED",
      workstation_name: "Failure",
    },
  ],
};

function factoryEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-04-16T12:00:0${tick}Z`,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

function withFactoryEventContext(
  event: FactoryEvent,
  context: Partial<FactoryEvent["context"]>,
): FactoryEvent {
  return {
    ...event,
    context: {
      ...event.context,
      ...context,
    },
  };
}

function getStateNodeByLabel(label: string): HTMLElement {
  const button = screen.getByRole("button", { name: `Select ${label} state` });
  const node = button.closest(".react-flow__node");

  if (!(node instanceof HTMLElement)) {
    throw new Error(`expected ${label} state to be rendered in a React Flow node`);
  }

  return node;
}

function getWorkstationNodeByLabel(label: string): HTMLElement {
  const button = screen.getByRole("button", { name: `Select ${label} workstation` });
  const node = button.closest(".react-flow__node");

  if (!(node instanceof HTMLElement)) {
    throw new Error(`expected ${label} workstation to be rendered in a React Flow node`);
  }

  return node;
}

function expectFixedReviewWorkstationDimensions(): void {
  const reviewNode = getWorkstationNodeByLabel("Review");

  expect(reviewNode.getAttribute("style")).toContain("width: 156px");
  expect(reviewNode.getAttribute("style")).toContain("height: 196px");
}

function expectStateNodeDotCount(label: string, count: number): void {
  const stateNode = getStateNodeByLabel(label);

  expect(stateNode.querySelectorAll("[data-state-work-progress-dot]")).toHaveLength(count);
}

function expectRenderedResourceCountMatchesBackendWorldView(tick: number): void {
  const expectedCount =
    resourceCountBackendWorldViewCountsByTick[tick]?.[resourceCountAvailablePlaceID] ?? 0;

  expect(screen.getByLabelText(resourceCountAvailablePlaceID)).toBeTruthy();
  expect(screen.getByLabelText(`${expectedCount} resource tokens`).textContent?.trim()).toBe(
    String(expectedCount),
  );
}

function expectDefinitionValue(
  section: HTMLElement,
  label: string,
  expectedValue: string,
): void {
  const term = within(section).getByText(label, { selector: "dt" });
  const row = term.closest("div");

  if (!(row instanceof HTMLElement)) {
    throw new Error(`expected definition row for ${label}`);
  }

  expect(within(row).getByText(expectedValue)).toBeTruthy();
}

function workstationRequestSection(selection: HTMLElement): HTMLElement {
  const section = selection.querySelector("[aria-label='Workstation request']");

  if (!(section instanceof HTMLElement)) {
    throw new Error("expected workstation request section to be rendered");
  }

  return section;
}

function expectRenderedWorkstationRequest(
  selection: HTMLElement,
  expected: DashboardRuntimeWorkstationRequest,
): void {
  const section = workstationRequestSection(selection);

  expectDefinitionValue(
    section,
    "dispatchedCount",
    String(expected.counts.dispatched_count),
  );
  expectDefinitionValue(
    section,
    "respondedCount",
    String(expected.counts.responded_count),
  );
  expectDefinitionValue(
    section,
    "erroredCount",
    String(expected.counts.errored_count),
  );

  if (expected.request.request_time) {
    expectDefinitionValue(section, "requestTime", expected.request.request_time);
  }
  if (expected.request.started_at) {
    expectDefinitionValue(section, "startedAt", expected.request.started_at);
  }
  if (expected.request.working_directory) {
    expectDefinitionValue(section, "workingDirectory", expected.request.working_directory);
  }
  if (expected.request.worktree) {
    expectDefinitionValue(section, "worktree", expected.request.worktree);
  }
  if (expected.request.prompt) {
    expect(within(section).getByText(expected.request.prompt)).toBeTruthy();
  }

  if (expected.response) {
    if (expected.response.outcome) {
      expectDefinitionValue(section, "outcome", expected.response.outcome);
    }
    if (expected.response.duration_millis !== undefined) {
      expectDefinitionValue(
        section,
        "duration",
        formatDurationMillis(expected.response.duration_millis),
      );
    }
    if (expected.response.error_class) {
      expectDefinitionValue(section, "errorClass", expected.response.error_class);
    }
    if (expected.response.failure_reason) {
      expectDefinitionValue(section, "failureReason", expected.response.failure_reason);
    }
    if (expected.response.failure_message) {
      expectDefinitionValue(section, "failureMessage", expected.response.failure_message);
    }
    if (expected.response.response_text) {
      expect(within(section).getByText(expected.response.response_text)).toBeTruthy();
    } else {
      expect(
        within(section).getByText(
          "Provider response text is not available on the workstation request projection.",
        ),
      ).toBeTruthy();
    }
    return;
  }

  expect(
    within(section).getByText("The workstation request has not produced a response yet."),
  ).toBeTruthy();
}

function removeTraceIDFromWorkItem(workItem: DashboardWorkItemRef): DashboardWorkItemRef {
  const withoutTraceID: DashboardWorkItemRef = { work_id: workItem.work_id };
  if (workItem.display_name) {
    withoutTraceID.display_name = workItem.display_name;
  }
  if (workItem.work_type_id) {
    withoutTraceID.work_type_id = workItem.work_type_id;
  }
  return withoutTraceID;
}

function removeTraceIDsFromSnapshot(snapshot: DashboardSnapshot): DashboardSnapshot {
  return {
    ...snapshot,
    runtime: {
      ...snapshot.runtime,
      active_executions_by_dispatch_id: Object.fromEntries(
        Object.entries(snapshot.runtime.active_executions_by_dispatch_id ?? {}).map(
          ([dispatchID, execution]) => [
            dispatchID,
            {
              ...execution,
              trace_ids: [],
              work_items: execution.work_items?.map(removeTraceIDFromWorkItem),
            },
          ],
        ),
      ),
      current_work_items_by_place_id: Object.fromEntries(
        Object.entries(snapshot.runtime.current_work_items_by_place_id ?? {}).map(
          ([placeID, workItems]) => [placeID, workItems.map(removeTraceIDFromWorkItem)],
        ),
      ),
      session: {
        ...snapshot.runtime.session,
        provider_sessions: snapshot.runtime.session.provider_sessions?.map((attempt) => ({
          ...attempt,
          work_items: attempt.work_items?.map(removeTraceIDFromWorkItem),
        })),
      },
      workstation_activity_by_node_id: Object.fromEntries(
        Object.entries(snapshot.runtime.workstation_activity_by_node_id ?? {}).map(
          ([nodeID, activity]) => [
            nodeID,
            {
              ...activity,
              active_work_items: activity.active_work_items?.map(removeTraceIDFromWorkItem),
              trace_ids: [],
            },
          ],
        ),
      ),
    },
  };
}

function expectSeparatedStateMarkerZones(label: string, count: number): void {
  const stateNode = getStateNodeByLabel(label);
  const labelZone = stateNode.querySelector("[data-state-label-zone]");
  const markerZone = stateNode.querySelector("[data-state-marker-zone]");

  expect(labelZone).toBeTruthy();
  expect(markerZone).toBeTruthy();
  expect(labelZone?.textContent).not.toContain(`${count} active`);
  expect(markerZone?.textContent).not.toContain("story");
  expect(markerZone?.querySelectorAll("[data-state-work-progress-dot]")).toHaveLength(count);
}

const selectedTickTimelineEvents: FactoryEvent[] = [
  factoryEvent("timeline-1", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
    factory: {
      work_types: [{
        name: "story",
        states: [
          { name: "new", type: "INITIAL" },
          { name: "review", type: "PROCESSING" },
          { name: "done", type: "TERMINAL" },
        ],
      }],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", work_type: "story" }],
          name: "Review",
          outputs: [{ state: "review", work_type: "story" }],
          worker: "reviewer",
        },
      ],
    },
  }),
  factoryEvent("timeline-2", 2, FACTORY_EVENT_TYPES.workRequest, {
    type: "FACTORY_REQUEST_BATCH",
    works: [{
      name: "Event Story",
      trace_id: eventTimelineTraceID,
      work_id: eventTimelineWorkID,
      work_type_id: "story",
    }],
  }),
  factoryEvent("timeline-3", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
    dispatchId: eventTimelineDispatchID,
    inputs: [
      {
        name: "Event Story",
        trace_id: eventTimelineTraceID,
        work_id: eventTimelineWorkID,
        work_type_id: "story",
      },
    ],
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", work_type: "story" }],
      name: "Review",
      outputs: [{ state: "review", work_type: "story" }],
      worker: "reviewer",
    },
  }),
  factoryEvent("timeline-4", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
    dispatchId: eventTimelineDispatchID,
    durationMillis: 1500,
    outcome: "ACCEPTED",
    outputWork: [
      {
        name: "Event Story",
        trace_id: eventTimelineTraceID,
        work_id: eventTimelineWorkID,
        work_type_id: "story",
      },
    ],
    providerSession: {
      id: "sess-event-story",
      kind: "session_id",
      provider: "codex",
    },
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", work_type: "story" }],
      name: "Review",
      outputs: [{ state: "done", work_type: "story" }],
      worker: "reviewer",
    },
  }),
];
selectedTickTimelineEvents[1].context.requestId = "request-event-story";
selectedTickTimelineEvents[1].context.traceIds = [eventTimelineTraceID];
selectedTickTimelineEvents[1].context.workIds = [eventTimelineWorkID];
selectedTickTimelineEvents[2].context.dispatchId = eventTimelineDispatchID;
selectedTickTimelineEvents[2].context.traceIds = [eventTimelineTraceID];
selectedTickTimelineEvents[2].context.workIds = [eventTimelineWorkID];
selectedTickTimelineEvents[3].context.dispatchId = eventTimelineDispatchID;
selectedTickTimelineEvents[3].context.traceIds = [eventTimelineTraceID];
selectedTickTimelineEvents[3].context.workIds = [eventTimelineWorkID];

const traceFanInReviewWorkstation = {
  id: "review",
  inputs: [{ state: "new", work_type: "story" }],
  name: "Review",
  outputs: [{ state: "review", work_type: "story" }],
  worker: "reviewer",
} as const;

const traceFanInCompleteWorkstation = {
  id: "complete",
  inputs: [{ state: "review", work_type: "story" }],
  name: "Complete",
  outputs: [{ state: "active", work_type: "story" }],
  worker: "completer",
} as const;

function buildTraceFanInTimelineEvents(): FactoryEvent[] {
  return [
    factoryEvent("trace-fan-in-1", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
      factory: {
        work_types: [{
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "active", type: "PROCESSING" },
          ],
        }],
        workstations: [traceFanInReviewWorkstation, traceFanInCompleteWorkstation],
      },
    }),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-2", 2, FACTORY_EVENT_TYPES.workRequest, {
        source: "api",
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Plan Input",
            trace_id: "chain-a",
            work_id: "work-plan-input",
            work_type_id: "story",
          },
          {
            current_chaining_trace_id: "chain-b",
            name: "Research Input",
            trace_id: "chain-b",
            work_id: "work-research-input",
            work_type_id: "story",
          },
        ],
      }),
      {
        requestId: "request-chain",
        traceIds: ["chain-a", "chain-b"],
        workIds: ["work-plan-input", "work-research-input"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-3", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
        current_chaining_trace_id: "chain-a",
        dispatchId: "dispatch-plan",
        inputs: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Plan Input",
            trace_id: "chain-a",
            work_id: "work-plan-input",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: traceFanInReviewWorkstation,
      }),
      {
        dispatchId: "dispatch-plan",
        traceIds: ["chain-a"],
        workIds: ["work-plan-input"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-4", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
        current_chaining_trace_id: "chain-a",
        dispatchId: "dispatch-plan",
        durationMillis: 450,
        outcome: "ACCEPTED",
        outputWork: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Reviewed Story",
            trace_id: "chain-a",
            work_id: "work-reviewed-story",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: traceFanInReviewWorkstation,
      }),
      {
        dispatchId: "dispatch-plan",
        traceIds: ["chain-a"],
        workIds: ["work-plan-input"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-5", 5, FACTORY_EVENT_TYPES.dispatchRequest, {
        current_chaining_trace_id: "chain-b",
        dispatchId: "dispatch-research",
        inputs: [
          {
            current_chaining_trace_id: "chain-b",
            name: "Research Input",
            trace_id: "chain-b",
            work_id: "work-research-input",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: traceFanInReviewWorkstation,
      }),
      {
        dispatchId: "dispatch-research",
        traceIds: ["chain-b"],
        workIds: ["work-research-input"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-6", 6, FACTORY_EVENT_TYPES.dispatchResponse, {
        current_chaining_trace_id: "chain-b",
        dispatchId: "dispatch-research",
        durationMillis: 420,
        outcome: "ACCEPTED",
        outputWork: [
          {
            current_chaining_trace_id: "chain-b",
            name: "Research Context",
            trace_id: "chain-b",
            work_id: "work-research-context",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: traceFanInReviewWorkstation,
      }),
      {
        dispatchId: "dispatch-research",
        traceIds: ["chain-b"],
        workIds: ["work-research-input"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-7", 7, FACTORY_EVENT_TYPES.dispatchRequest, {
        current_chaining_trace_id: "chain-a",
        dispatchId: "dispatch-implement",
        inputs: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Reviewed Story",
            trace_id: "chain-a",
            work_id: "work-reviewed-story",
            work_type_id: "story",
          },
          {
            current_chaining_trace_id: "chain-b",
            name: "Research Context",
            trace_id: "chain-b",
            work_id: "work-research-context",
            work_type_id: "story",
          },
        ],
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        transitionId: "complete",
        workstation: traceFanInCompleteWorkstation,
      }),
      {
        dispatchId: "dispatch-implement",
        traceIds: ["chain-a", "chain-b"],
        workIds: ["work-reviewed-story", "work-research-context"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-8", 8, FACTORY_EVENT_TYPES.dispatchResponse, {
        current_chaining_trace_id: "chain-a",
        dispatchId: "dispatch-implement",
        durationMillis: 900,
        outcome: "ACCEPTED",
        outputWork: [
          {
            current_chaining_trace_id: "chain-a",
            name: fanInResultLabel,
            previous_chaining_trace_ids: ["chain-a", "chain-b"],
            trace_id: "chain-a",
            work_id: fanInResultWorkID,
            work_type_id: "story",
          },
        ],
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        transitionId: "complete",
        workstation: traceFanInCompleteWorkstation,
      }),
      {
        dispatchId: "dispatch-implement",
        traceIds: ["chain-a", "chain-b"],
        workIds: ["work-reviewed-story", "work-research-context"],
      },
    ),
  ];
}

function buildLegacyTraceTimelineEvents(): FactoryEvent[] {
  return [
    factoryEvent("trace-legacy-1", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
      factory: {
        work_types: [{
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "active", type: "PROCESSING" },
          ],
        }],
        workstations: [traceFanInReviewWorkstation, traceFanInCompleteWorkstation],
      },
    }),
    withFactoryEventContext(
      factoryEvent("trace-legacy-2", 2, FACTORY_EVENT_TYPES.workRequest, {
        source: "api",
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "Legacy Story",
            trace_id: "trace-legacy",
            work_id: "work-legacy",
            work_type_id: "story",
          },
        ],
      }),
      {
        requestId: "request-legacy",
        traceIds: ["trace-legacy"],
        workIds: ["work-legacy"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-legacy-3", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
        dispatchId: "dispatch-legacy-review",
        inputs: [
          {
            name: "Legacy Story",
            trace_id: "trace-legacy",
            work_id: "work-legacy",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: traceFanInReviewWorkstation,
      }),
      {
        dispatchId: "dispatch-legacy-review",
        traceIds: ["trace-legacy"],
        workIds: ["work-legacy"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-legacy-4", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
        dispatchId: "dispatch-legacy-review",
        durationMillis: 360,
        outcome: "ACCEPTED",
        outputWork: [
          {
            name: "Legacy Review",
            trace_id: "trace-legacy",
            work_id: "work-legacy-reviewed",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: traceFanInReviewWorkstation,
      }),
      {
        dispatchId: "dispatch-legacy-review",
        traceIds: ["trace-legacy"],
        workIds: ["work-legacy"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-legacy-5", 5, FACTORY_EVENT_TYPES.dispatchRequest, {
        dispatchId: "dispatch-legacy-complete",
        inputs: [
          {
            name: "Legacy Review",
            trace_id: "trace-legacy",
            work_id: "work-legacy-reviewed",
            work_type_id: "story",
          },
        ],
        transitionId: "complete",
        workstation: traceFanInCompleteWorkstation,
      }),
      {
        dispatchId: "dispatch-legacy-complete",
        traceIds: ["trace-legacy"],
        workIds: ["work-legacy-reviewed"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-legacy-6", 6, FACTORY_EVENT_TYPES.dispatchResponse, {
        dispatchId: "dispatch-legacy-complete",
        durationMillis: 640,
        outcome: "ACCEPTED",
        outputWork: [
          {
            name: "Legacy Done",
            trace_id: "trace-legacy",
            work_id: "work-legacy-done",
            work_type_id: "story",
          },
        ],
        transitionId: "complete",
        workstation: traceFanInCompleteWorkstation,
      }),
      {
        dispatchId: "dispatch-legacy-complete",
        traceIds: ["trace-legacy"],
        workIds: ["work-legacy-reviewed"],
      },
    ),
  ];
}

const tickZeroInitialStructureRequestEvents: FactoryEvent[] = [
  factoryEvent("timeline-zero-1", 0, FACTORY_EVENT_TYPES.initialStructureRequest, {
    factory: {
      work_types: [{
        name: "story",
        states: [
          { name: "new", type: "INITIAL" },
          { name: "review", type: "PROCESSING" },
        ],
      }],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", work_type: "story" }],
          name: "Review",
          outputs: [{ state: "review", work_type: "story" }],
          worker: "reviewer",
        },
      ],
    },
  }),
];

const exportTimelineEvents: FactoryEvent[] = [
  factoryEvent("export-run-request", 0, FACTORY_EVENT_TYPES.runRequest, {
    factory: {
      project: "semantic-workflow",
      source_directory: "/work/factories/semantic-workflow",
      workers: [
        {
          model_provider: "codex",
          name: "reviewer",
          provider: "script_wrap",
          type: "MODEL_WORKER",
        },
      ],
      work_types: [{
        name: "story",
        states: [
          { name: "new", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
      }],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", work_type: "story" }],
          name: "Review",
          on_failure: { state: "done", work_type: "story" },
          outputs: [{ state: "done", work_type: "story" }],
          worker: "reviewer",
        },
      ],
    },
    recordedAt: "2026-04-16T12:00:00Z",
  }),
];
const currentNamedFactoryExportResponse = {
  factory: {
    metadata: {
      contractSource: "current-factory-api",
    },
    project: "authored-current-factory",
    workers: [
      {
        executorProvider: "script_wrap",
        modelProvider: "codex",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [{
      name: "story",
      states: [
        { name: "new", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    }],
    workstations: [
      {
        id: "review",
        inputs: [{ state: "new", workType: "story" }],
        name: "Review",
        onFailure: { state: "done", workType: "story" },
        outputs: [{ state: "done", workType: "story" }],
        type: "MODEL_WORKSTATION",
        worker: "reviewer",
      },
    ],
  },
  name: "semantic-workflow",
} satisfies NamedFactoryValue;

const queryClients: QueryClient[] = [];
let restoreBrowserTestShims: (() => void) | null = null;

function seedTimelineSnapshot(
  snapshot: DashboardSnapshot,
  tracesByWorkID: Record<string, DashboardTrace> = {},
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest> = {},
): void {
  useFactoryTimelineStore.setState({
    events: [],
    latestTick: snapshot.tick_count,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: snapshot.tick_count,
    worldViewCache: {
      [snapshot.tick_count]: {
        dashboard: snapshot,
        relationsByWorkID: {},
        tracesByWorkID,
        workstationRequestsByDispatchID,
        workRequestsByID: {},
      },
    },
  });
}

function seedTimelineSnapshots(snapshots: DashboardSnapshot[]): void {
  const worldViewCache = Object.fromEntries(
    snapshots.map(
      (snapshot) =>
        [
          snapshot.tick_count,
          {
            dashboard: snapshot,
            relationsByWorkID: {},
            tracesByWorkID: {},
            workstationRequestsByDispatchID: {},
            workRequestsByID: {},
          } satisfies FactoryTimelineSnapshot,
        ] as const,
    ),
  );
  const latestTick = Math.max(...snapshots.map((snapshot) => snapshot.tick_count));

  useFactoryTimelineStore.setState({
    events: [],
    latestTick,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: latestTick,
    worldViewCache,
  });
}

function renderApp({
  snapshot,
  timelineEvents,
  timelineSnapshots,
  traceFixtures = {},
  workstationRequestsByDispatchID = {},
}: RenderAppOptions) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
  queryClients.push(queryClient);

  const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
    const path =
      typeof input === "string"
        ? input
        : input instanceof URL
          ? `${input.pathname}${input.search}`
          : input.url;

    throw new Error(`unexpected fetch for ${path}`);
  });

  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("EventSource", MockEventSource);
  if (timelineEvents) {
    useFactoryTimelineStore.getState().replaceEvents(timelineEvents);
  } else if (timelineSnapshots) {
    seedTimelineSnapshots(timelineSnapshots);
  } else {
    seedTimelineSnapshot(snapshot, traceFixtures, workstationRequestsByDispatchID);
  }

  const result = render(
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>,
  );

  return { ...result, fetchMock };
}

function submitWorkCardControls() {
  const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
  const submitWorkCard = within(dashboardGrid).getByRole("article", { name: "Submit work" });
  const submitWorkScope = within(submitWorkCard);

  return {
    requestName: submitWorkScope.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    }),
    requestText: submitWorkScope.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Request text",
    }),
    submitButton: submitWorkScope.getByRole<HTMLButtonElement>("button", {
      name: "Submit work",
    }),
    submitWorkScope,
    workType: submitWorkScope.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    }),
  };
}

function fromBase64(value: string): Uint8Array {
  return Uint8Array.from(atob(value), (character) => character.charCodeAt(0));
}

function jsonResponse(body: unknown, status = 200, statusText?: string): Response {
  return new Response(JSON.stringify(body), {
    headers: {
      "Content-Type": "application/json",
    },
    status,
    statusText,
  });
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const copy = new Uint8Array(bytes.byteLength);
  copy.set(bytes);
  return copy.buffer;
}

function exportImageFile(): File {
  return new File(
    [toArrayBuffer(fromBase64(ONE_PIXEL_PNG_BASE64))],
    "cover.png",
    { type: "image/png" },
  );
}


function createDeferredPromise<T>(): {
  promise: Promise<T>;
  reject: (reason?: unknown) => void;
  resolve: (value: T) => void;
} {
  let reject!: (reason?: unknown) => void;
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    reject = rejectPromise;
    resolve = resolvePromise;
  });

  return {
    promise,
    reject,
    resolve,
  };
}

function installExportDownloadProbe(): {
  getDownloadedBlob: () => Blob | null;
  getDownloadedFilename: () => string;
  restore: () => void;
} {
  class MockExportOffscreenCanvas {
    public constructor(
      public readonly width: number,
      public readonly height: number,
    ) {}

    public getContext(_contextID: "2d"): OffscreenCanvasRenderingContext2D {
      return {
        drawImage() {},
      } as unknown as OffscreenCanvasRenderingContext2D;
    }

    public async convertToBlob(): Promise<Blob> {
      return new Blob([toArrayBuffer(fromBase64(ONE_PIXEL_PNG_BASE64))], {
        type: "image/png",
      });
    }
  }

  const originalCreateObjectURL = window.URL.createObjectURL;
  const originalRevokeObjectURL = window.URL.revokeObjectURL;
  const originalCreateImageBitmap = globalThis.createImageBitmap;
  const originalOffscreenCanvas = globalThis.OffscreenCanvas;
  const originalClick = HTMLAnchorElement.prototype.click;
  let downloadedBlob: Blob | null = null;
  let downloadedFilename = "";

  window.URL.createObjectURL = ((blob: Blob) => {
    downloadedBlob = blob;
    return "blob:app-test-export";
  }) as typeof URL.createObjectURL;
  window.URL.revokeObjectURL = (() => {}) as typeof URL.revokeObjectURL;
  globalThis.createImageBitmap = (async () => ({
    close: () => {},
    height: 1,
    width: 1,
  })) as typeof createImageBitmap;
  globalThis.OffscreenCanvas =
    MockExportOffscreenCanvas as unknown as typeof OffscreenCanvas;
  HTMLAnchorElement.prototype.click = function click(): void {
    downloadedFilename = this.download;
  };

  return {
    getDownloadedBlob: () => downloadedBlob,
    getDownloadedFilename: () => downloadedFilename,
    restore: () => {
      window.URL.createObjectURL = originalCreateObjectURL;
      window.URL.revokeObjectURL = originalRevokeObjectURL;
      globalThis.createImageBitmap = originalCreateImageBitmap;
      globalThis.OffscreenCanvas = originalOffscreenCanvas;
      HTMLAnchorElement.prototype.click = originalClick;
    },
  };
}
function renderTraceDrilldownHarness({
  selectedWorkID,
  timelineEvents,
}: {
  selectedWorkID: string;
  timelineEvents: FactoryEvent[];
}) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
  queryClients.push(queryClient);
  useFactoryTimelineStore.getState().replaceEvents(timelineEvents);

  return render(
    <QueryClientProvider client={queryClient}>
      <TraceDrilldownTestHarness selectedWorkID={selectedWorkID} />
    </QueryClientProvider>,
  );
}

function createFactoryImportValue(): FactoryPngImportValue {
  return {
    envelope: {
      factory: {
        workTypes: [],
        workers: [],
        workstations: [],
      },
      name: "Dropped Factory",
      schemaVersion: "portos.agent-factory.png.v1",
    },
    factory: {
      workTypes: [],
      workers: [],
      workstations: [],
    },
    factoryName: "Dropped Factory",
    namedFactory: {
      factory: {
        workTypes: [],
        workers: [],
        workstations: [],
      },
      name: "Dropped Factory",
    },
    previewImageSrc: "blob:factory-preview",
    revokePreviewImageSrc: vi.fn(),
  };
}

function createFileDropTransfer(files: File[]): {
  dataTransfer: {
    dropEffect: string;
    files: File[];
    types: string[];
  };
} {
  return {
    dataTransfer: {
      dropEffect: "none",
      files,
      types: ["Files"],
    },
  };
}

function resizeDashboardViewport(width: number): void {
  Object.defineProperty(window, "innerWidth", {
    configurable: true,
    value: width,
    writable: true,
  });
  Object.defineProperty(window, "innerHeight", {
    configurable: true,
    value: width < 720 ? 720 : 900,
    writable: true,
  });
  window.dispatchEvent(new Event("resize"));
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function selectWorkstationRequest(dispatchId: string): Promise<void> {
  fireEvent.click(await screen.findByRole("button", { name: "Select Review workstation" }));
  const currentSelection = await screen.findByRole("article", { name: "Current selection" });
  const requestHistorySection = within(currentSelection)
    .getByRole("heading", { name: "Request history" })
    .closest("section");
  expect(requestHistorySection).toBeTruthy();
  fireEvent.click(within(requestHistorySection!).getByRole("button", { name: "Expand" }));
  fireEvent.click(
    within(requestHistorySection!).getByRole("button", {
      name: new RegExp(`\\(${escapeRegExp(dispatchId)}\\)$`),
    }),
  );
}

function getDispatchHistoryCard(container: HTMLElement, dispatchId: string): HTMLElement {
  const dispatchBadge = within(container).getByText(dispatchId);
  const card = dispatchBadge.closest("article");

  if (!(card instanceof HTMLElement)) {
    throw new Error(`expected dispatch history card for ${dispatchId}`);
  }

  return card;
}

describe("App", () => {
  beforeEach(() => {
    window.localStorage.clear();
    MockEventSource.instances = [];
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    for (const queryClient of queryClients.splice(0)) {
      queryClient.clear();
    }
    cleanup();
    useFactoryTimelineStore.getState().reset();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("renders the operator graph for an empty runtime snapshot", async () => {
    renderApp({ snapshot: baselineSnapshot });

    expect(
      await screen.findByRole("heading", { name: "Agent Factory" }),
    ).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Factory graph" })).toBeTruthy();
    expect(screen.getByText("In progress")).toBeTruthy();
    expect(await screen.findByRole("button", { name: "Select Plan workstation" })).toBeTruthy();
    expect(await screen.findByRole("button", { name: "Select Implement workstation" })).toBeTruthy();
    expect(await screen.findByRole("button", { name: "Select Review workstation" })).toBeTruthy();
    expect(screen.queryByText("Idle")).toBeNull();
    expect(screen.queryByText("Live Workstation Dashboard")).toBeNull();
    expect(
      screen.queryByText(/Reconstruction-first workflow graph with live workstation overlays/i),
    ).toBeNull();
    expect(screen.queryByRole("heading", { name: "Terminal summary" })).toBeNull();
  });

  it("smoke tests dropped factory import activation through preview confirmation and dashboard refresh", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    vi.spyOn(factoryPngImportModule, "readFactoryImportPng").mockResolvedValue({
      ok: true,
      value: importValue,
    });
    const { fetchMock } = renderApp({ snapshot: semanticWorkflowDashboardSnapshot });

    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path =
        typeof input === "string"
          ? input
          : input instanceof URL
            ? `${input.pathname}${input.search}`
            : input.url;

      if (path === "/factory") {
        return new Response(
          JSON.stringify({
            factory: importValue.factory,
            name: importValue.factoryName,
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
          },
        );
      }

      throw new Error(`unexpected fetch for ${path} (${init?.method ?? "GET"})`);
    });

    const viewport = await screen.findByRole("region", { name: "Work graph viewport" });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });
    expect(previewDialog.textContent).toContain("Dropped Factory");
    expect(previewDialog.textContent).toContain("factory-import.png");

    fireEvent.click(within(previewDialog).getByRole("button", { name: "Activate factory" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/factory",
        expect.objectContaining({
          body: JSON.stringify({
            factory: importValue.factory,
            name: importValue.factoryName,
          }),
          headers: {
            "Content-Type": "application/json",
          },
          method: "POST",
        }),
      );
    });
    await waitFor(() => {
      expect(MockEventSource.instances).toHaveLength(2);
    });
    expect(importValue.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog", { name: "Review factory import" })).toBeNull();

    const refreshedStream = MockEventSource.instances[1];
    if (!refreshedStream) {
      throw new Error("expected a refreshed dashboard stream after factory activation");
    }

    act(() => {
      refreshedStream.emit("snapshot", importedFactorySnapshot);
    });

    await waitFor(() => {
      expect(screen.getByText("Imported factory active")).toBeTruthy();
    });
    expect(await screen.findByRole("button", { name: "Select Imported Review workstation" }))
      .toBeTruthy();
    expect(screen.queryByRole("button", { name: "Select Review workstation" })).toBeNull();
  });

  it("smoke tests authored export and dropped import as one dashboard-shell roundtrip", async () => {
    const exportProbe = installExportDownloadProbe();
    const mockedExportResult = await factoryPngExportModule.writeFactoryExportPng({
      image: exportImageFile(),
      namedFactory: currentNamedFactoryExportResponse,
      rasterizeImageToPngBytes: async () => fromBase64(ONE_PIXEL_PNG_BASE64),
    });
    if (!mockedExportResult.ok) {
      throw new Error("expected the roundtrip export fixture to build successfully");
    }
    const writeFactoryExportPngSpy = vi
      .spyOn(factoryPngExportModule, "writeFactoryExportPng")
      .mockResolvedValue(mockedExportResult);
    const { fetchMock } = renderApp({
      snapshot: semanticWorkflowDashboardSnapshot,
      timelineEvents: exportTimelineEvents,
    });

    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path =
        typeof input === "string"
          ? input
          : input instanceof URL
            ? `${input.pathname}${input.search}`
            : input.url;

      if (path === "/factory/~current") {
        return jsonResponse(currentNamedFactoryExportResponse);
      }

      if (path === "/factory") {
        return jsonResponse(JSON.parse(String(init?.body)));
      }

      throw new Error(`unexpected fetch for ${path} (${init?.method ?? "GET"})`);
    });

    try {
      fireEvent.click(await screen.findByRole("button", { name: "Export PNG" }));

      const exportDialog = await screen.findByRole("dialog", { name: "Export factory" });
      await waitFor(() => {
        expect(
          (within(exportDialog).getByRole("button", { name: "Export PNG" }) as HTMLButtonElement)
            .disabled,
        ).toBe(false);
      });

      const imageInput = within(exportDialog).getByLabelText("Cover image") as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);
      fireEvent.click(within(exportDialog).getByRole("button", { name: "Export PNG" }));

      await waitFor(() => {
        expect(exportProbe.getDownloadedBlob()).not.toBeNull();
      });
      await waitFor(() => {
        expect(exportProbe.getDownloadedFilename()).toBe("semantic-workflow.png");
      });
      await waitFor(() => {
        expect(screen.queryByRole("dialog", { name: "Export factory" })).toBeNull();
      });

      const exportedBlob = exportProbe.getDownloadedBlob();
      if (!(exportedBlob instanceof Blob)) {
        throw new Error("expected the export flow to download a PNG blob");
      }

      const viewport = await screen.findByRole("region", { name: "Work graph viewport" });
      fireEvent.drop(
        viewport,
        createFileDropTransfer([
          new File([exportedBlob], exportProbe.getDownloadedFilename(), { type: "image/png" }),
        ]),
      );

      const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });
      expect(previewDialog.textContent).toContain(currentNamedFactoryExportResponse.name);
      expect(previewDialog.textContent).toContain("semantic-workflow.png");

      fireEvent.click(within(previewDialog).getByRole("button", { name: "Activate factory" }));

      await waitFor(() => {
        const activationCall = fetchMock.mock.calls.find(([url]) => url === "/factory");
        expect(activationCall).toBeDefined();
        expect(activationCall?.[1]).toEqual(
          expect.objectContaining({
            body: expect.any(String),
            headers: {
              "Content-Type": "application/json",
            },
            method: "POST",
          }),
        );
        expect(JSON.parse(String(activationCall?.[1]?.body))).toEqual(currentNamedFactoryExportResponse);
      });
      await waitFor(() => {
        expect(MockEventSource.instances).toHaveLength(2);
      });

      const refreshedStream = MockEventSource.instances[1];
      if (!refreshedStream) {
        throw new Error("expected a refreshed dashboard stream after factory activation");
      }

      act(() => {
        refreshedStream.emit("snapshot", importedFactorySnapshot);
      });

      await waitFor(() => {
        expect(screen.getByText("Imported factory active")).toBeTruthy();
      });
      expect(await screen.findByRole("button", { name: "Select Imported Review workstation" }))
        .toBeTruthy();
    } finally {
      writeFactoryExportPngSpy.mockRestore();
      exportProbe.restore();
    }
  });

  it("applies the shared typography helpers to the dashboard toolbar summary shell", async () => {
    renderApp({ snapshot: baselineSnapshot });

    const heading = await screen.findByRole("heading", { name: "Agent Factory" });
    const toolbar = screen.getByRole("region", { name: "dashboard summary" });
    const factoryStateLabel = screen.getByText("Factory state");
    const exportButton = screen.getByRole("button", { name: "Export PNG" });
    const summaryList = factoryStateLabel.closest("dl");

    if (!(summaryList instanceof HTMLDListElement)) {
      throw new Error("expected dashboard summary metadata list");
    }

    expect(heading.className).toContain(DASHBOARD_PAGE_HEADING_CLASS);
    expect(summaryList.className).toContain(DASHBOARD_BODY_TEXT_CLASS);
    expect(summaryList.className).toContain(DASHBOARD_SUPPORTING_LABELS_CLASS);
    expect(toolbar.textContent).toContain(baselineSnapshot.factory_state);
    expect(exportButton.getAttribute("aria-haspopup")).toBe("dialog");
  });

  it("opens the export dialog from the toolbar and dismisses it without dashboard side effects", async () => {
    const exportProbe = installExportDownloadProbe();
    const { fetchMock } = renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });
    fetchMock.mockResolvedValueOnce(jsonResponse(currentNamedFactoryExportResponse));

    try {
      fireEvent.click(await screen.findByRole("button", { name: "Export PNG" }));

      const dialog = await screen.findByRole("dialog", { name: "Export factory" });
      await waitFor(() => {
        expect(within(dialog).getByDisplayValue("semantic-workflow")).toBeTruthy();
      });
      expect(
        within(dialog).getByText(/without changing the live dashboard state/i),
      ).toBeTruthy();
      expect(within(dialog).getByRole("button", { name: "Cancel" })).toBeTruthy();
      fireEvent.change(within(dialog).getByLabelText("Factory name"), {
        target: { value: "Factory Poster" },
      });
      const imageInput = within(dialog).getByLabelText("Cover image") as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);
      expect(within(dialog).getByText("Selected image: cover.png")).toBeTruthy();

      fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));

      await waitFor(() => {
        expect(screen.queryByRole("dialog", { name: "Export factory" })).toBeNull();
      });
      expect(screen.getByRole("region", { name: "dashboard summary" })).toBeTruthy();
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(fetchMock.mock.calls[0]?.[0]).toBe("/factory/~current");
      expect(exportProbe.getDownloadedBlob()).toBeNull();
      expect(exportProbe.getDownloadedFilename()).toBe("");
    } finally {
      exportProbe.restore();
    }
  });

  it("waits for a fresh current-factory response before exporting after reopen", async () => {
    const refreshedCurrentNamedFactoryExportResponse = {
      ...currentNamedFactoryExportResponse,
      factory: {
        ...currentNamedFactoryExportResponse.factory,
        metadata: {
          ...currentNamedFactoryExportResponse.factory.metadata,
          contractSource: "refetched-current-factory-api",
        },
        project: "authored-refetched-factory",
      },
      name: "imported-workflow",
    } satisfies NamedFactoryValue;
    const refreshedCurrentFactoryResponse = createDeferredPromise<Response>();
    const writeFactoryExportPngSpy = vi
      .spyOn(factoryPngExportModule, "writeFactoryExportPng")
      .mockResolvedValue({
        blob: new Blob([toArrayBuffer(fromBase64(ONE_PIXEL_PNG_BASE64))], {
          type: "image/png",
        }),
        envelope: {
          ...refreshedCurrentNamedFactoryExportResponse,
          schemaVersion: "portos.agent-factory.png.v1",
        },
        ok: true,
      });
    const { fetchMock } = renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });
    let currentFactoryFetchCount = 0;

    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const path =
        typeof input === "string"
          ? input
          : input instanceof URL
            ? `${input.pathname}${input.search}`
            : input.url;

      if (path !== "/factory/~current") {
        throw new Error(`unexpected fetch for ${path}`);
      }

      currentFactoryFetchCount += 1;

      if (currentFactoryFetchCount === 1) {
        return jsonResponse(currentNamedFactoryExportResponse);
      }

      if (currentFactoryFetchCount === 2) {
        return refreshedCurrentFactoryResponse.promise;
      }

      throw new Error(`unexpected current factory fetch #${currentFactoryFetchCount}`);
    });

    try {
      fireEvent.click(await screen.findByRole("button", { name: "Export PNG" }));

      const firstDialog = await screen.findByRole("dialog", { name: "Export factory" });
      await waitFor(() => {
        expect(within(firstDialog).getByDisplayValue("semantic-workflow")).toBeTruthy();
      });
      expect(
        (within(firstDialog).getByRole("button", { name: "Export PNG" }) as HTMLButtonElement)
          .disabled,
      ).toBe(false);

      fireEvent.click(within(firstDialog).getByRole("button", { name: "Cancel" }));

      await waitFor(() => {
        expect(screen.queryByRole("dialog", { name: "Export factory" })).toBeNull();
      });

      fireEvent.click(screen.getByRole("button", { name: "Export PNG" }));

      const secondDialog = await screen.findByRole("dialog", { name: "Export factory" });
      expect(within(secondDialog).getByText("Loading the current authored factory definition."))
        .toBeTruthy();
      expect(
        (within(secondDialog).getByRole("button", { name: "Export PNG" }) as HTMLButtonElement)
          .disabled,
      ).toBe(true);
      expect(writeFactoryExportPngSpy).not.toHaveBeenCalled();

      await act(async () => {
        refreshedCurrentFactoryResponse.resolve(
          jsonResponse(refreshedCurrentNamedFactoryExportResponse),
        );
        await refreshedCurrentFactoryResponse.promise;
      });

      await waitFor(() => {
        expect(within(secondDialog).getByDisplayValue("imported-workflow")).toBeTruthy();
      });
      expect(
        (within(secondDialog).getByRole("button", { name: "Export PNG" }) as HTMLButtonElement)
          .disabled,
      ).toBe(false);

      const imageInput = within(secondDialog).getByLabelText("Cover image") as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);
      fireEvent.click(within(secondDialog).getByRole("button", { name: "Export PNG" }));

      await waitFor(() => {
        expect(writeFactoryExportPngSpy).toHaveBeenCalledWith({
          image: expect.any(File),
          namedFactory: refreshedCurrentNamedFactoryExportResponse,
        });
      });
      await waitFor(() => {
        expect(screen.queryByRole("dialog", { name: "Export factory" })).toBeNull();
      });
    } finally {
      writeFactoryExportPngSpy.mockRestore();
    }
  });

  it("does not download after cancelling an export that is still in flight", async () => {
    const exportProbe = installExportDownloadProbe();
    const factoryPngExportModule = await import("./features/export/factory-png-export");
    const pendingExport = createDeferredPromise<
      Awaited<ReturnType<typeof factoryPngExportModule.writeFactoryExportPng>>
    >();
    const writeFactoryExportPngSpy = vi
      .spyOn(factoryPngExportModule, "writeFactoryExportPng")
      .mockReturnValue(pendingExport.promise);
    const { fetchMock } = renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });
    fetchMock.mockResolvedValueOnce(jsonResponse(currentNamedFactoryExportResponse));

    try {
      fireEvent.click(await screen.findByRole("button", { name: "Export PNG" }));

      const dialog = await screen.findByRole("dialog", { name: "Export factory" });
      await waitFor(() => {
        expect((within(dialog).getByRole("button", { name: "Export PNG" }) as HTMLButtonElement).disabled).toBe(false);
      });
      fireEvent.change(within(dialog).getByLabelText("Factory name"), {
        target: { value: "Factory Poster" },
      });
      const imageInput = within(dialog).getByLabelText("Cover image") as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);
      fireEvent.click(within(dialog).getByRole("button", { name: "Export PNG" }));

      expect(
        (within(dialog).getByRole("button", { name: "Exporting..." }) as HTMLButtonElement)
          .disabled,
      ).toBe(true);

      fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));

      await waitFor(() => {
        expect(screen.queryByRole("dialog", { name: "Export factory" })).toBeNull();
      });

      await act(async () => {
        pendingExport.resolve({
          blob: new Blob([toArrayBuffer(fromBase64(ONE_PIXEL_PNG_BASE64))], {
            type: "image/png",
          }),
          envelope: {
            factory: currentNamedFactoryExportResponse.factory,
            name: "Factory Poster",
            schemaVersion: "portos.agent-factory.png.v1",
          },
          ok: true,
        });
        await pendingExport.promise;
      });

      expect(writeFactoryExportPngSpy).toHaveBeenCalledTimes(1);
      expect(exportProbe.getDownloadedBlob()).toBeNull();
      expect(exportProbe.getDownloadedFilename()).toBe("");
    } finally {
      writeFactoryExportPngSpy.mockRestore();
      exportProbe.restore();
    }
  });

  it("validates the export fields and accepts the confirmed name plus selected image", async () => {
    const exportProbe = installExportDownloadProbe();
    const { fetchMock } = renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });
    fetchMock.mockResolvedValueOnce(jsonResponse(currentNamedFactoryExportResponse));

    try {
      fireEvent.click(await screen.findByRole("button", { name: "Export PNG" }));

      const dialog = await screen.findByRole("dialog", { name: "Export factory" });
      await waitFor(() => {
        expect((within(dialog).getByRole("button", { name: "Export PNG" }) as HTMLButtonElement).disabled).toBe(false);
      });
      const exportButton = within(dialog).getByRole("button", { name: "Export PNG" });
      fireEvent.click(exportButton);

      expect(
        within(dialog).getByText("Choose a cover image before exporting."),
      ).toBeTruthy();
      expect(exportProbe.getDownloadedBlob()).toBeNull();

      const nameInput = within(dialog).getByLabelText("Factory name");
      fireEvent.change(nameInput, { target: { value: "   " } });
      fireEvent.click(exportButton);

      expect(
        within(dialog).getByText("Enter a factory name before exporting."),
      ).toBeTruthy();
      expect(exportProbe.getDownloadedBlob()).toBeNull();

      fireEvent.change(nameInput, { target: { value: "Factory Poster" } });
      const imageInput = within(dialog).getByLabelText("Cover image") as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);

      expect(within(dialog).getByDisplayValue("Factory Poster")).toBeTruthy();
      expect(within(dialog).getByText("Selected image: cover.png")).toBeTruthy();
      expect(
        within(dialog).queryByText("Enter a factory name before exporting."),
      ).toBeNull();
      expect(
        within(dialog).queryByText("Choose a cover image before exporting."),
      ).toBeNull();
      expect(exportProbe.getDownloadedBlob()).toBeNull();
      expect(exportProbe.getDownloadedFilename()).toBe("");
    } finally {
      exportProbe.restore();
    }
  });

  it("exports the current named-factory API payload instead of the event timeline projection", async () => {
    const exportProbe = installExportDownloadProbe();
    const factoryPngExportModule = await import("./features/export/factory-png-export");
    const writeFactoryExportPngSpy = vi
      .spyOn(factoryPngExportModule, "writeFactoryExportPng")
      .mockResolvedValue({
        blob: new Blob([toArrayBuffer(fromBase64(ONE_PIXEL_PNG_BASE64))], {
          type: "image/png",
        }),
        envelope: {
          factory: currentNamedFactoryExportResponse.factory,
          name: "Factory Poster",
          schemaVersion: "portos.agent-factory.png.v1",
        },
        ok: true,
      });
    const { fetchMock } = renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: exportTimelineEvents,
    });
    fetchMock.mockResolvedValueOnce(jsonResponse(currentNamedFactoryExportResponse));

    try {
      fireEvent.click(await screen.findByRole("button", { name: "Export PNG" }));

      const dialog = await screen.findByRole("dialog", { name: "Export factory" });
      await waitFor(() => {
        expect((within(dialog).getByRole("button", { name: "Export PNG" }) as HTMLButtonElement).disabled).toBe(false);
      });
      fireEvent.change(within(dialog).getByLabelText("Factory name"), {
        target: { value: "Factory Poster" },
      });
      const imageInput = within(dialog).getByLabelText("Cover image") as HTMLInputElement;
      Object.defineProperty(imageInput, "files", {
        configurable: true,
        value: [exportImageFile()],
      });
      fireEvent.change(imageInput);
      fireEvent.click(within(dialog).getByRole("button", { name: "Export PNG" }));

      await waitFor(() => {
        expect(writeFactoryExportPngSpy).toHaveBeenCalledTimes(1);
      });
      expect(writeFactoryExportPngSpy).toHaveBeenCalledWith({
        image: expect.any(File),
        namedFactory: {
          ...currentNamedFactoryExportResponse,
          name: "Factory Poster",
        },
      });
      expect(exportProbe.getDownloadedFilename()).toBe("factory-poster.png");
    } finally {
      writeFactoryExportPngSpy.mockRestore();
      exportProbe.restore();
    }
  });

  it("disables the timeline control until at least two ticks are available", async () => {
    renderApp({ snapshot: historicalTimelineSnapshot });

    const slider = await screen.findByRole<HTMLInputElement>("slider", { name: "Timeline tick" });

    expect(slider.disabled).toBe(true);
    expect(screen.getByText("Waiting for more ticks")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Current" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it("renders a fixed historical tick from the timeline slider", async () => {
    renderApp({
      snapshot: terminalSnapshot,
      timelineSnapshots: [historicalTimelineSnapshot, terminalSnapshot],
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", { name: "Timeline tick" });
    expect(slider.value).toBe("4");
    expect(screen.getByText("Tick 4 of 4")).toBeTruthy();
    expect(within(screen.getByLabelText("work totals")).getAllByText("1").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Done Story" })).toBeTruthy();

    fireEvent.change(slider, { target: { value: "1" } });

    await waitFor(() => {
      expect(slider.value).toBe("1");
      expect(screen.getByText("Tick 1 of 4")).toBeTruthy();
      expect(screen.queryByRole("button", { name: "Done Story" })).toBeNull();
    });
    expect(screen.queryByText("sess-done-story")).toBeNull();
  });

  it("returns from a fixed timeline tick to the current factory view", async () => {
    renderApp({
      snapshot: terminalSnapshot,
      timelineSnapshots: [historicalTimelineSnapshot, terminalSnapshot],
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", { name: "Timeline tick" });
    fireEvent.change(slider, { target: { value: "1" } });

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Done Story" })).toBeNull();
    });

    fireEvent.click(screen.getByRole("button", { name: "Current" }));

    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("Tick 4 of 4")).toBeTruthy();
      expect(screen.getByRole("button", { name: "Done Story" })).toBeTruthy();
    });
  });

  it("renders totals and selection panels from the selected event tick", async () => {
      renderApp({
        snapshot: baselineSnapshot,
        timelineEvents: selectedTickTimelineEvents,
      });

      const slider = await screen.findByRole<HTMLInputElement>("slider", { name: "Timeline tick" });
      const totals = screen.getByLabelText("work totals");
      expect(slider.value).toBe("4");
      expect(screen.getByText("Tick 4 of 4")).toBeTruthy();
      expect(within(totals).getByText("Completed")).toBeTruthy();
      expect(within(totals).getAllByText("1").length).toBeGreaterThan(0);
      const eventSelection = await screen.findByRole("article", { name: "Current selection" });
      expect(eventSelection).toBeTruthy();
      expect(screen.getByRole("article", { name: "Trace drill-down" })).toBeTruthy();
      expect(screen.queryByText("Trace history unavailable")).toBeNull();

      fireEvent.change(slider, { target: { value: "3" } });

      await waitFor(() => {
        expect(slider.value).toBe("3");
        expect(screen.getByText("Tick 3 of 4")).toBeTruthy();
        expect(screen.queryByText("sess-event-story")).toBeNull();
        expect(screen.queryByRole("article", { name: "Event Story" })).toBeNull();
      });
      expect(
        within(totals).getByText("In progress").closest("article")?.textContent,
      ).toContain("1");

      fireEvent.change(slider, { target: { value: "2" } });

      await waitFor(() => {
        expect(screen.getByText("Tick 2 of 4")).toBeTruthy();
        expectStateNodeDotCount("story:new", 1);
      });
    });

  it.each([
    {
      label: "ready",
      requestProjection: dashboardWorkstationRequestFixtures.ready,
      verify: (currentSelection: HTMLElement) => {
        expect(within(currentSelection).getAllByText("request-ready-story").length).toBeGreaterThan(
          0,
        );
        expect(within(currentSelection).getByRole("heading", { name: "Request counts" })).toBeTruthy();
        expect(within(currentSelection).getByRole("heading", { name: "Response details" })).toBeTruthy();
        expect(
          within(currentSelection).getAllByText("Ready for the next workstation.").length,
        ).toBeGreaterThan(0);
      },
    },
    {
      label: "no-response",
      requestProjection: dashboardWorkstationRequestFixtures.noResponse,
      verify: (currentSelection: HTMLElement) => {
        expect(
          within(currentSelection).getByText(
            "Response text is not available for this workstation request yet.",
          ),
        ).toBeTruthy();
        expect(
          within(currentSelection).getByText(
            "Response metadata is not available for this workstation request yet.",
          ),
        ).toBeTruthy();
      },
    },
    {
      label: "rejected",
      requestProjection: dashboardWorkstationRequestFixtures.rejected,
      verify: (currentSelection: HTMLElement) => {
        expect(within(currentSelection).getAllByText("The active story needs revision before it can continue.").length).toBeGreaterThan(0);
        expect(within(currentSelection).getByRole("heading", { name: "Response details" })).toBeTruthy();
      },
    },
    {
      label: "errored",
      requestProjection: dashboardWorkstationRequestFixtures.errored,
      verify: (currentSelection: HTMLElement) => {
        expect(
          within(currentSelection).getByRole("heading", { name: "Error details" }),
        ).toBeTruthy();
        expect(within(currentSelection).getAllByText("provider_rate_limit").length).toBeGreaterThan(
          0,
        );
      },
    },
    {
      label: "script-success",
      requestProjection: dashboardWorkstationRequestFixtures.scriptSuccess,
      verify: (currentSelection: HTMLElement) => {
        expect(within(currentSelection).getAllByText("request-script-success-story").length).toBeGreaterThan(
          0,
        );
        expect(within(currentSelection).getByRole("heading", { name: "Request counts" })).toBeTruthy();
        expect(within(currentSelection).queryByRole("heading", { name: "Execution details" })).toBeNull();
      },
    },
    {
      label: "script-failed",
      requestProjection: dashboardWorkstationRequestFixtures.scriptFailed,
      verify: (currentSelection: HTMLElement) => {
        expect(within(currentSelection).getAllByText("request-script-failed-story").length).toBeGreaterThan(
          0,
        );
        expect(within(currentSelection).getByRole("heading", { name: "Error details" })).toBeTruthy();
        expect(within(currentSelection).getByText("script_timeout")).toBeTruthy();
      },
    },
  ])(
    "selects a workstation dispatch and routes $label request context through work-item details",
    async ({ requestProjection, verify }) => {
      renderApp({
        snapshot: activeSnapshot,
        workstationRequestsByDispatchID: {
          [requestProjection.dispatch_id]: requestProjection,
        },
      });

      await selectWorkstationRequest(requestProjection.dispatch_id);

      await waitFor(() => {
        const currentSelection = screen.getByRole("article", { name: "Current selection" });
        expect(
          within(currentSelection).getAllByText(requestProjection.dispatch_id).length,
        ).toBeGreaterThan(0);
        expect(within(currentSelection).queryByRole("heading", { name: "Active work" })).toBeNull();
        verify(currentSelection);
      });
    },
  );

  it("smoke tests /events replay rendering without the removed dashboard snapshot route", async () => {
    const { fetchMock } = renderApp({ snapshot: historicalTimelineSnapshot });

    const stream = MockEventSource.instances[0];
    if (!stream) {
      throw new Error("expected factory event stream to be opened");
    }
    expect(stream.url).toBe("/events");

    act(() => {
      for (const event of selectedTickTimelineEvents) {
        stream.emit("message", event);
      }
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", { name: "Timeline tick" });
    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("Tick 4 of 4")).toBeTruthy();
      expect(screen.getByRole("button", { name: "Select Review workstation" })).toBeTruthy();
    });
    expect(fetchMock).not.toHaveBeenCalled();

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("Tick 3 of 4")).toBeTruthy();
      expect(screen.queryByText("sess-event-story")).toBeNull();
      expect(
        within(screen.getByLabelText("work totals"))
          .getByText("In progress")
          .closest("article")?.textContent,
      ).toContain("1");
    });
  });

  it("smoke tests failure analysis from streamed events through fixed-tick rendering", async () => {
    const { fetchMock } = renderApp({ snapshot: historicalTimelineSnapshot });

    const stream = MockEventSource.instances[0];
    if (!stream) {
      throw new Error("expected factory event stream to be opened");
    }

    act(() => {
      for (const event of failureAnalysisTimelineEvents) {
        stream.emit("message", event);
      }
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", { name: "Timeline tick" });
    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("Tick 4 of 4")).toBeTruthy();
      expect(screen.getByRole("button", { name: "Blocked Analysis Story" })).toBeTruthy();
      expect(
        screen.getAllByText(/codex \/ session_id \/ sess-blocked-analysis/).length,
      ).toBeGreaterThan(0);
    });

    fireEvent.click(screen.getByRole("button", { name: "Blocked Analysis Story" }));

    const failedDetail = await screen.findByRole("article", { name: "Current selection" });
    expect(within(failedDetail).getAllByText("Failure reason").length).toBeGreaterThan(0);
    expect(within(failedDetail).getAllByText("provider_rate_limit").length).toBeGreaterThan(0);
    expect(within(failedDetail).getAllByText("Failure message").length).toBeGreaterThan(0);
    expect(
      within(failedDetail).getAllByText(
        "Provider rate limit exceeded while generating the analysis.",
      ).length,
    ).toBeTruthy();
    expect(
      within(failedDetail).queryByText(
        "Terminal summaries are reconstructed from retained runtime state.",
      ),
    ).toBeNull();

    fireEvent.click(await screen.findByRole("button", { name: "Select story:new state" }));

    const currentPositionDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(within(currentPositionDetail).getByText("Current work")).toBeTruthy();
    expect(within(currentPositionDetail).getByText("Queued Analysis Story")).toBeTruthy();
    expect(within(currentPositionDetail).getByText("work-queued-analysis")).toBeTruthy();

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("Tick 3 of 4")).toBeTruthy();
      expect(screen.queryByRole("button", { name: "Blocked Analysis Story" })).toBeNull();
      expect(screen.queryByText("provider_rate_limit")).toBeNull();
      expect(screen.queryByText("sess-blocked-analysis")).toBeNull();
      expect(screen.getByText("Queued Analysis Story")).toBeTruthy();
    });

    fireEvent.change(slider, { target: { value: "4" } });

    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("Tick 4 of 4")).toBeTruthy();
      expect(screen.getByRole("button", { name: "Blocked Analysis Story" })).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "Blocked Analysis Story" }));

    const fixedFailedDetail = await screen.findByRole("article", { name: "Current selection" });
    expect(within(fixedFailedDetail).getAllByText("provider_rate_limit").length).toBeGreaterThan(0);
    expect(
      within(fixedFailedDetail).getAllByText(
        "Provider rate limit exceeded while generating the analysis.",
      ).length,
    ).toBeGreaterThan(0);
    expect(fetchMock).not.toHaveBeenCalled();
  });


  it("smoke tests resource counts from streamed events against backend world-view counts", async () => {
    const { fetchMock } = renderApp({ snapshot: historicalTimelineSnapshot });

    const stream = MockEventSource.instances[0];
    if (!stream) {
      throw new Error("expected factory event stream to be opened");
    }

    act(() => {
      for (const event of resourceCountTimelineEvents) {
        stream.emit("message", event);
      }
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", { name: "Timeline tick" });
    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("Tick 4 of 4")).toBeTruthy();
      expectRenderedResourceCountMatchesBackendWorldView(4);
    });

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("Tick 3 of 4")).toBeTruthy();
      expectRenderedResourceCountMatchesBackendWorldView(3);
    });

    fireEvent.change(slider, { target: { value: "1" } });

    await waitFor(() => {
      expect(slider.value).toBe("1");
      expect(screen.getByText("Tick 1 of 4")).toBeTruthy();
      expectRenderedResourceCountMatchesBackendWorldView(1);
    });

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("smoke tests workstation-request runtime details against backend expectations", async () => {
    renderApp({
      snapshot: historicalTimelineSnapshot,
      timelineEvents: runtimeDetailsTimelineEvents,
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    expect(slider.value).toBe("11");
    expect(screen.getByText("Tick 11 of 11")).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: runtimeDetailsFixtureIDs.completedWorkLabel,
      }),
    ).toBeTruthy();
    expect(
      useFactoryTimelineStore.getState().worldViewCache[11]?.dashboard.runtime
        .workstation_requests_by_dispatch_id,
    ).toMatchObject(runtimeDetailsBackendWorkstationRequestsByDispatchID);

    fireEvent.click(
      screen.getByRole("button", {
        name: runtimeDetailsFixtureIDs.completedWorkLabel,
      }),
    );

    const completedSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(within(completedSelection).getByRole("heading", { name: "Request counts" })).toBeTruthy();
    expect(
      within(completedSelection).getAllByText(runtimeDetailsFixtureIDs.completedDispatchID).length,
    ).toBeTruthy();
    expect(
      within(completedSelection).getByText(runtimeDetailsFixtureIDs.completedProviderSessionID),
    ).toBeTruthy();
    expect(
      within(completedSelection).getByText(runtimeDetailsFixtureIDs.completedPromptSource),
    ).toBeTruthy();
    expect(
      within(completedSelection).getByRole("heading", { name: "Inference attempts" }),
    ).toBeTruthy();
    expect(within(completedSelection).queryByRole("link", { name: "Open trace" })).toBeNull();

    fireEvent.click(
      screen.getByRole("button", {
        name: runtimeDetailsFixtureIDs.failedWorkLabel,
      }),
    );

    const failedSelection = await screen.findByRole("article", { name: "Current selection" });
    expect(
      within(failedSelection).getAllByText(runtimeDetailsFixtureIDs.failedFailureReason).length,
    ).toBeGreaterThan(0);
    expect(
      within(failedSelection).getAllByText(runtimeDetailsFixtureIDs.failedFailureMessage).length,
    ).toBeGreaterThan(0);
    expect(within(failedSelection).getByRole("heading", { name: "Request counts" })).toBeTruthy();

    fireEvent.click(
      (await screen.findAllByRole("button", {
        name: new RegExp(runtimeDetailsFixtureIDs.activeWorkLabel),
      }))[0],
    );

    const pendingSelection = await screen.findByRole("article", { name: "Current selection" });
    expect(
      within(pendingSelection).getByRole("heading", { name: "Execution details" }),
    ).toBeTruthy();
    expect(
      within(pendingSelection).getAllByText(runtimeDetailsFixtureIDs.activeDispatchID).length,
    ).toBeGreaterThan(0);
    expectDefinitionValue(pendingSelection, "Workstation dispatches", "1");
    expect(
      within(pendingSelection).queryByText(
        "No workstation dispatch has been recorded yet for this work item.",
      ),
    ).toBeNull();
    expect(
      within(pendingSelection).getByText(
        "Prompt details are not available for this selected run yet.",
      ),
    ).toBeTruthy();
    expect(
      within(pendingSelection).getByText(
        "Provider session details are not available for this selected run yet.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText(runtimeDetailsFixtureIDs.unsafeSystemPromptBody)).toBeNull();
    expect(screen.queryByText(runtimeDetailsFixtureIDs.unsafeUserMessageBody)).toBeNull();
  });

  it("smoke tests mixed script and inference workstation-request history against backend expectations", async () => {
    renderApp({
      snapshot: historicalTimelineSnapshot,
      timelineEvents: scriptDashboardIntegrationTimelineEvents,
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    expect(slider.value).toBe("14");
    expect(screen.getByText("Tick 14 of 14")).toBeTruthy();
    expect(
      useFactoryTimelineStore.getState().worldViewCache[14]?.dashboard.runtime
        .workstation_requests_by_dispatch_id,
    ).toMatchObject(scriptDashboardIntegrationBackendWorkstationRequestsByDispatchID);

    async function selectReviewRequest(dispatchID: string): Promise<void> {
      fireEvent.click(
        screen.getByRole("button", {
          name: "Select Review workstation",
        }),
      );

      const workstationSelection = await screen.findByRole("article", {
        name: "Current selection",
      });
      const requestHistorySection = within(workstationSelection)
        .getByRole("heading", { name: "Request history" })
        .closest("section");
      if (!(requestHistorySection instanceof HTMLElement)) {
        throw new Error("expected request history section for script dashboard smoke");
      }

      fireEvent.click(within(requestHistorySection).getByRole("button", { name: "Expand" }));
      fireEvent.click(
        within(requestHistorySection).getByRole("button", {
          name: new RegExp(`\\(${dispatchID}\\)$`),
        }),
      );
    }

    await selectReviewRequest(scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID);

    const scriptSuccessSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(within(scriptSuccessSelection).getByRole("heading", { name: "Request counts" })).toBeTruthy();
    expect(
      within(scriptSuccessSelection).getAllByText(
        scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID,
      ).length,
    ).toBeGreaterThan(0);
    expect(within(scriptSuccessSelection).getByText("script success stdout")).toBeTruthy();
    expect(within(scriptSuccessSelection).getAllByText("SUCCEEDED").length).toBeGreaterThan(0);
    expect(
      within(scriptSuccessSelection).queryByRole("heading", { name: "Inference attempts" }),
    ).toBeNull();

    await selectReviewRequest(scriptDashboardIntegrationFixtureIDs.failedDispatchID);

    const scriptFailedSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(scriptFailedSelection).getAllByText(
        scriptDashboardIntegrationFixtureIDs.failedFailureReason,
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptFailedSelection).getAllByText(
        scriptDashboardIntegrationFixtureIDs.failedFailureMessage,
      ).length,
    ).toBeGreaterThan(0);
    expect(within(scriptFailedSelection).getAllByText("TIMEOUT").length).toBeGreaterThan(0);
    expect(within(scriptFailedSelection).getByText("script timed out")).toBeTruthy();
    expect(
      within(scriptFailedSelection).queryByRole("heading", { name: "Inference attempts" }),
    ).toBeNull();

    await selectReviewRequest(scriptDashboardIntegrationFixtureIDs.inferenceDispatchID);

    const inferenceSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(inferenceSelection).getByRole("heading", { name: "Inference attempts" }),
    ).toBeTruthy();
    expect(
      within(inferenceSelection).getByText(
        scriptDashboardIntegrationFixtureIDs.inferenceProviderSessionID,
      ),
    ).toBeTruthy();
    expect(
      within(inferenceSelection).getByText(
        scriptDashboardIntegrationFixtureIDs.inferencePromptSource,
      ),
    ).toBeTruthy();
    expect(
      within(inferenceSelection).getAllByText(
        scriptDashboardIntegrationFixtureIDs.inferenceResponseText,
      ).length,
    ).toBeGreaterThan(0);
  });

  it("smoke tests graph state across event replay, terminal selection, and tick changes", async () => {
    renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: graphStateSmokeTimelineEvents,
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", { name: "Timeline tick" });
    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });

    await waitFor(() => {
      expect(slider.value).toBe("9");
      expect(screen.getByText("Tick 9 of 9")).toBeTruthy();
      expectFixedReviewWorkstationDimensions();
      expect(
        getStateNodeByLabel("story:done").querySelector("[aria-label='2 active items']"),
      ).toBeTruthy();
      expect(
        getStateNodeByLabel("story:failed").querySelector("[aria-label='1 active item']"),
      ).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "Select story:done state" }));

    const completedDetail = await within(dashboardGrid).findByRole("article", {
      name: "Current selection",
    });
    expect(within(completedDetail).getByText("Current work")).toBeTruthy();
    expect(within(completedDetail).getByText("Completed Smoke Story One")).toBeTruthy();
    expect(within(completedDetail).getByText("work-smoke-complete-one")).toBeTruthy();
    expect(within(completedDetail).getByText("Completed Smoke Story Two")).toBeTruthy();
    expect(within(completedDetail).getByText("work-smoke-complete-two")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Select story:failed state" }));

    await waitFor(() => {
      const failedDetail = screen.getByRole("article", { name: "Current selection" });

      expect(within(failedDetail).getByText("Current work")).toBeTruthy();
      expect(within(failedDetail).getByText("Failed Smoke Story")).toBeTruthy();
      expect(within(failedDetail).getByText("work-smoke-failed")).toBeTruthy();
      expect(within(failedDetail).getByText("provider_rate_limit")).toBeTruthy();
      expect(
        within(failedDetail).queryByText("No current work is occupying this place."),
      ).toBeNull();
    });

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("Tick 3 of 9")).toBeTruthy();
      expect(screen.getByRole("button", { name: /Completed Smoke Story One/ })).toBeTruthy();
      expectFixedReviewWorkstationDimensions();
    });

    fireEvent.change(slider, { target: { value: "2" } });

    await waitFor(() => {
      expect(slider.value).toBe("2");
      expect(screen.getByText("Tick 2 of 9")).toBeTruthy();
      expectSeparatedStateMarkerZones("story:new", 3);
    });

    fireEvent.change(slider, { target: { value: "9" } });

    await waitFor(() => {
      expect(slider.value).toBe("9");
      expect(screen.getByText("Tick 9 of 9")).toBeTruthy();
      expectFixedReviewWorkstationDimensions();
      expect(
        getStateNodeByLabel("story:done").querySelector("[aria-label='2 active items']"),
      ).toBeTruthy();
    });
  });

  it("renders backend tick-zero initial structure instead of staying in loading state", async () => {
    renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: tickZeroInitialStructureRequestEvents,
    });

    expect(await screen.findByRole("heading", { name: "Agent Factory" })).toBeTruthy();
    expect(screen.queryByText("Loading dashboard")).toBeNull();
    expect(await screen.findByRole("button", { name: "Select Review workstation" })).toBeTruthy();
    expect((screen.getByRole("slider", { name: "Timeline tick" }) as HTMLInputElement).value).toBe(
      "0",
    );
    expect(screen.getByText("Waiting for more ticks")).toBeTruthy();
  });

  it("starts with full-width totals above a full-width Factory graph card", async () => {
    renderApp({ snapshot: baselineSnapshot });

    await screen.findByRole("heading", { name: "Agent Factory" });

    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    const workTotals = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-totals"]',
    );
    const workflowActivity = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-graph"]',
    );
    if (!workTotals || !workflowActivity) {
      throw new Error("expected totals and workflow cards to render in the dashboard grid");
    }

    expect(workTotals.dataset.layoutSignature).toContain("work-totals:0:0:12:2");
    expect(workflowActivity.dataset.layoutSignature).toContain("work-graph:0:2:12:10");
    expect(within(screen.getByLabelText("work totals")).getByText("In progress")).toBeTruthy();
    expect(within(screen.getByLabelText("work totals")).getByText("Completed")).toBeTruthy();
    expect(within(screen.getByLabelText("work totals")).getByText("Failed")).toBeTruthy();
    expect(within(screen.getByLabelText("work totals")).getByText("Dispatched")).toBeTruthy();
  });

  it("migrates legacy selection detail layout IDs into one current selection slot", async () => {
    window.localStorage.setItem(
      "agent-factory.dashboard.layout.v2",
      JSON.stringify([
        { h: 5, id: "work-totals", w: 12, x: 0, y: 0 },
        { h: 10, id: "work-graph", w: 12, x: 0, y: 2 },
        { h: 6, id: "work-info", w: 5, x: 7, y: 12 },
      ]),
    );

    renderApp({ snapshot: activeSnapshot });

    await screen.findByRole("heading", { name: "Agent Factory" });

    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    const currentSelection = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="current-selection"]',
    );
    const legacySelection = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-info"], [data-bento-card-id="workstation-info"], [data-bento-card-id="terminal-summary"]',
    );

    expect(currentSelection).toBeTruthy();
    expect(legacySelection).toBeNull();
    expect(currentSelection?.dataset.layoutSignature).toMatch(/current-selection:7:\d+:5:6/);
  });

  it("migrates stored completion and failure chart layout IDs into one work outcome chart slot", async () => {
    window.localStorage.setItem(
      "agent-factory.dashboard.layout.v2",
      JSON.stringify([
        { h: 5, id: "completion-trend", w: 5, x: 7, y: 12 },
        { h: 5, id: "failure-trend", w: 4, x: 0, y: 17 },
      ]),
    );

    renderApp({ snapshot: activeSnapshot });

    await screen.findByRole("heading", { name: "Agent Factory" });

    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    const workOutcome = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-outcome-chart"]',
    );
    const legacyCharts = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="completion-trend"], [data-bento-card-id="failure-trend"]',
    );

    expect(workOutcome).toBeTruthy();
    expect(legacyCharts).toBeNull();
    expect(workOutcome?.dataset.layoutSignature).toMatch(/work-outcome-chart:7:\d+:5:5/);
  });

  it("ignores stored retry, rework, and timing trend card IDs in the visible dashboard layout", async () => {
    window.localStorage.setItem(
      "agent-factory.dashboard.layout.v2",
      JSON.stringify([
        { h: 5, id: "rework-trend", w: 4, x: 0, y: 18 },
        { h: 5, id: "timing-trend", w: 4, x: 4, y: 18 },
        { h: 7, id: "trace", w: 4, x: 8, y: 18 },
      ]),
    );

    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: reworkTraceSnapshot,
      },
    });

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);

    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    const trace = await within(dashboardGrid).findByRole("article", {
      name: "Trace drill-down",
    });
    const hiddenTrendCards = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="rework-trend"], [data-bento-card-id="timing-trend"]',
    );

    expect(hiddenTrendCards).toBeNull();
    expect(within(dashboardGrid).queryByRole("article", { name: "Retry and rework trend" })).toBeNull();
    expect(within(dashboardGrid).queryByRole("article", { name: "Timing trend" })).toBeNull();
    const layoutSignature =
      trace.closest<HTMLElement>("[data-bento-card-id]")?.dataset.layoutSignature ?? "";
    expect(layoutSignature).not.toContain("rework-trend");
    expect(layoutSignature).not.toContain("timing-trend");
    expect(layoutSignature).toMatch(/trace:\d+:\d+:\d+:\d+/);
  });

  it("renders distinct graph semantics for topology places, active work, and retry outcomes", async () => {
    renderApp({ snapshot: activeSnapshot });

    expect((await screen.findAllByText("dispatch-review-active")).length).toBeGreaterThan(0);
    await waitFor(() => {
      expect(screen.getAllByRole("button", { name: /Select .* workstation/ })).toHaveLength(5);
    });
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(screen.queryByText("State Position")).toBeNull();
    expect(screen.getByLabelText("agent-slot:available")).toBeTruthy();
    expect(screen.getByLabelText("2 resource tokens")).toBeTruthy();
    expect(screen.getByText("quality-gate:ready")).toBeTruthy();
    expect(screen.getByLabelText("1 constraint token")).toBeTruthy();
    const constraintArticle = screen.getByText("quality-gate:ready").closest("article");

    expect(screen.getByRole("img", { name: "Resource" }).getAttribute("data-graph-semantic-icon"))
      .toBe("resource");
    expect(screen.getByRole("img", { name: "Constraint" }).getAttribute("data-graph-semantic-icon"))
      .toBe("constraint");
    expect(constraintArticle?.textContent).not.toContain("Constraint");
    expect(screen.queryByText("Active Work")).toBeNull();
    expectStateNodeDotCount("story:ready", 3);
    expect(getStateNodeByLabel("story:blocked")).toBeTruthy();
    expect(getStateNodeByLabel("story:complete")).toBeTruthy();
  });

  it("renders a valid single-workstation topology when the API omits empty edges", async () => {
    renderApp({ snapshot: singleNodeSnapshotWithoutEdges });

    expect(
      await screen.findByRole("heading", { name: "Agent Factory" }),
    ).toBeTruthy();
    expect(await screen.findByRole("button", { name: "Select Intake workstation" })).toBeTruthy();
    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    expect(currentSelection).toBeTruthy();
    fireEvent.click(within(currentSelection).getByRole("button", { name: "Expand" }));
    expect(
      within(currentSelection).getByText(
        "No workstation runs have been recorded for this workstation yet.",
      ),
    ).toBeTruthy();
  });

  it("uses React Flow controls for work graph zoom interaction", async () => {
    renderApp({ snapshot: baselineSnapshot });

    await screen.findByRole("heading", { name: "Agent Factory" });

    const workGraphViewport = screen.getByRole("region", { name: "Work graph viewport" });
    expect(workGraphViewport).toBeTruthy();
    const flowViewport = document.querySelector<HTMLElement>(".react-flow__viewport");
    const initialTransform = flowViewport?.style.transform;

    fireEvent.click(within(workGraphViewport).getByRole("button", { name: "Zoom In" }));

    await waitFor(() => {
      expect(flowViewport?.style.transform).not.toBe(initialTransform);
    });
  });

  it("renders and interacts with a 20-node workflow through React Flow", async () => {
    renderApp({ snapshot: twentyNodeSnapshot });

    await screen.findByRole("heading", { name: "Agent Factory" });

    await waitFor(() => {
      expect(screen.getAllByRole("button", { name: /Select .* workstation/ })).toHaveLength(20);
    });
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(screen.getAllByText("Station 1").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Station 20")).toBeTruthy();
    expect(getStateNodeByLabel("story:step-6")).toBeTruthy();

    expect(screen.getByRole("button", { name: "Zoom Out" })).toBeTruthy();

    const station20 = await screen.findByRole("button", {
      name: "Select Station 20 workstation",
    });
    fireEvent.click(station20);

    await waitFor(() => {
      expect(station20.getAttribute("aria-pressed")).toBe("true");
    });
    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
  });

  it("renders a trace drill-down for a selected work item", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    expect((await screen.findAllByText("dispatch-review-active")).length).toBeGreaterThan(0);
    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);

    const currentSelection = await screen.findByRole("article", { name: "Current selection" });
    expect(within(currentSelection).getByRole("heading", { name: "Execution details" })).toBeTruthy();
    expect(
      within(currentSelection).getByText("Prompt details are not available for this selected run yet."),
    ).toBeTruthy();
    expect(within(currentSelection).getByText("sess-active-story")).toBeTruthy();
    expect(within(currentSelection).getAllByText(/trace-active-story/).length).toBeGreaterThan(0);
    expect(document.querySelector("[data-bento-card-id='trace']")?.getAttribute("id")).toBe(
      "trace",
    );
    expect(
      within(currentSelection).getByRole("heading", { name: "Workstation dispatches" }),
    ).toBeTruthy();
    expect(within(currentSelection).getAllByText(/codex \/ session_id \/ sess-active-story/)[0]).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", { name: "Work session runs list" }),
    ).toBeNull();
    const traceCard = screen.getByRole("article", { name: "Trace drill-down" });
    expect(traceCard).toBeTruthy();
    expect(within(traceCard).getByText("Trace dispatch grid")).toBeTruthy();
    expect(within(traceCard).getByText("Accepted · 1s")).toBeTruthy();
    expect(within(traceCard).queryByText("Workstation run")).toBeNull();
    expect(within(traceCard).queryByText("Consumed tokens")).toBeNull();
    expect(within(traceCard).queryByText("Output mutations")).toBeNull();
  });

  it("renders one selected-work dispatch history list with active, accepted, rejected, and failed rows", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
      workstationRequestsByDispatchID: {
        [dashboardWorkstationRequestFixtures.noResponse.dispatch_id]:
          dashboardWorkstationRequestFixtures.noResponse,
        [dashboardWorkstationRequestFixtures.ready.dispatch_id]:
          dashboardWorkstationRequestFixtures.ready,
        [dashboardWorkstationRequestFixtures.rejected.dispatch_id]:
          dashboardWorkstationRequestFixtures.rejected,
        [dashboardWorkstationRequestFixtures.errored.dispatch_id]:
          dashboardWorkstationRequestFixtures.errored,
      },
    });

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);

    const currentSelection = await screen.findByRole("article", { name: "Current selection" });
    const dispatchHistory = within(currentSelection).getByRole("region", {
      name: "Workstation dispatches",
    });

    expect(within(currentSelection).getByRole("heading", { name: "Workstation dispatches" })).toBeTruthy();
    expect(within(currentSelection).queryByRole("heading", { name: "Work session runs list" })).toBeNull();
    expect(within(dispatchHistory).getByText("4 dispatches")).toBeTruthy();

    const pendingCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.noResponse.dispatch_id,
    );
    expect(
      within(pendingCard).getByText(
        "Review the active story while the provider response is still pending.",
      ),
    ).toBeTruthy();
    expect(within(pendingCard).getByText("No response yet for this dispatch.")).toBeTruthy();

    const readyCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.ready.dispatch_id,
    );
    expect(
      within(readyCard).getByText("Review the active story and decide whether it is ready."),
    ).toBeTruthy();
    expect(within(readyCard).getByText("Ready for the next workstation.")).toBeTruthy();

    const rejectedCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.rejected.dispatch_id,
    );
    expect(
      within(rejectedCard).getByText(
        "Review the active story and explain what needs to change before approval.",
      ),
    ).toBeTruthy();
    expect(
      within(rejectedCard).getByText(
        "The active story needs revision before it can continue.",
      ),
    ).toBeTruthy();

    const erroredCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.errored.dispatch_id,
    );
    expect(
      within(erroredCard).getByText("Review the blocked story and explain the failure."),
    ).toBeTruthy();
    expect(
      within(erroredCard).getByText("Provider rate limit exceeded while reviewing the story."),
    ).toBeTruthy();
    expect(
      within(erroredCard).getByText(
        "Response text is unavailable because this dispatch ended with an error.",
      ),
    ).toBeTruthy();
  });

  it("renders selected work item trace unavailable copy when no trace ID exists", async () => {
    renderApp({
      snapshot: activeSnapshotWithoutTraceID,
    });

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);

    const currentSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(within(currentSelection).getByRole("heading", { name: "Execution details" })).toBeTruthy();
    expect(await screen.findByRole("article", { name: "Trace drill-down" })).toBeTruthy();
    expect(await screen.findByText("Trace history unavailable")).toBeTruthy();
  });

  it("keeps workstation and work-item selection usable after React Flow zoom", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    await screen.findAllByText("dispatch-review-active");

    const workGraphViewport = screen.getByRole("region", { name: "Work graph viewport" });
    fireEvent.click(within(workGraphViewport).getByRole("button", { name: "Zoom In" }));

    fireEvent.click(await screen.findByRole("button", { name: "Select Plan workstation" }));
    await waitFor(() => {
      expect(screen.getAllByText("planner").length).toBeGreaterThanOrEqual(1);
    });
    expect(screen.getByText("Input work types")).toBeTruthy();

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);
    expect(await screen.findByText("Trace drill-down")).toBeTruthy();
  });

  it("separates workstation selection from active work selection", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    await screen.findAllByText("dispatch-review-active");

    const reviewButton = await screen.findByRole("button", {
      name: "Select Review workstation",
    });
    fireEvent.click(reviewButton);
    await waitFor(() => {
      expect(reviewButton.getAttribute("aria-pressed")).toBe("true");
    });
    const reviewNode = reviewButton.closest("[data-workstation-kind]");
    const updatedReviewNode = screen
      .getByRole("button", { name: "Select Review workstation" })
      .closest("[data-workstation-kind]");
    expect(updatedReviewNode?.getAttribute("data-selected-workstation") === "true").toBe(
      true,
    );
    expect(updatedReviewNode?.getAttribute("data-selected-work") === "true").toBe(false);
    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();

    const workButton = (await screen.findAllByRole("button", { name: /Active Story/ }))[0];
    fireEvent.click(workButton);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    });
    expect(reviewButton.getAttribute("aria-pressed")).toBe("false");
    expect(workButton.getAttribute("aria-pressed")).toBe("true");
    expect(reviewNode?.getAttribute("data-selected-workstation") === "true").toBe(false);
    expect(reviewNode?.getAttribute("data-selected-work") === "true").toBe(true);
    expect(workButton.getAttribute("data-selected") === "true").toBe(true);

    fireEvent.click(await screen.findByRole("button", { name: "Select Plan workstation" }));

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    });
    const planNode = screen
      .getByRole("button", { name: "Select Plan workstation" })
      .closest("[data-workstation-kind]");
    const restoredReviewNode = screen
      .getByRole("button", { name: "Select Review workstation" })
      .closest("[data-workstation-kind]");
    expect(planNode?.getAttribute("data-selected-workstation") === "true").toBe(true);
    expect(restoredReviewNode?.getAttribute("data-selected-work") === "true").toBe(false);
    expect(workButton.getAttribute("aria-pressed")).toBe("false");
  });

  it("shows active executions from the selected workstation instead of provider history", async () => {
    const reviewExecution =
      activeSnapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];

    expect(reviewExecution).toBeDefined();

    const snapshot = {
      ...activeSnapshot,
      runtime: {
        ...activeSnapshot.runtime,
        active_dispatch_ids: ["dispatch-review-active", "dispatch-plan-active"],
        active_executions_by_dispatch_id: {
          "dispatch-plan-active": {
            ...reviewExecution!,
            dispatch_id: "dispatch-plan-active",
            started_at: "2026-04-08T12:00:01Z",
            trace_ids: ["trace-plan-active"],
            transition_id: "plan",
            workstation_name: "Plan",
            workstation_node_id: "plan",
            work_items: [
              {
                display_name: "Plan Active",
                trace_id: "trace-plan-active",
                work_id: "work-plan-active",
                work_type_id: "story",
              },
            ],
          },
          "dispatch-review-active": {
            ...reviewExecution!,
            transition_id: "review",
            workstation_name: "Review",
            workstation_node_id: "legacy-review-node",
          },
        },
        active_workstation_node_ids: ["review", "plan"],
        session: {
          ...activeSnapshot.runtime.session,
          provider_sessions: [],
        },
      },
    } satisfies DashboardSnapshot;

    renderApp({ snapshot });

    fireEvent.click(await screen.findByRole("button", { name: "Select Review workstation" }));

    const reviewInfo = await screen.findByRole("article", { name: "Current selection" });
    await waitFor(() => {
      expect(within(reviewInfo).getByRole("heading", { name: "Active work" })).toBeTruthy();
      expect(within(reviewInfo).getByText(activeWorkLabel)).toBeTruthy();
      expect(within(reviewInfo).getByText(activeWorkID)).toBeTruthy();
      expect(within(reviewInfo).getAllByText("dispatch-review-active").length).toBeGreaterThan(0);
      expect(within(reviewInfo).queryByText("Plan Active")).toBeNull();
    });

    fireEvent.click(await screen.findByRole("button", { name: "Select Plan workstation" }));

    const planInfo = await screen.findByRole("article", { name: "Current selection" });
    await waitFor(() => {
      expect(within(planInfo).getByText("Plan Active")).toBeTruthy();
      expect(within(planInfo).getByText("work-plan-active")).toBeTruthy();
      expect(within(planInfo).getAllByText("dispatch-plan-active").length).toBeGreaterThan(0);
      expect(within(planInfo).queryByText(activeWorkLabel)).toBeNull();
    });

    fireEvent.click(await screen.findByRole("button", { name: "Select Implement workstation" }));

    const implementInfo = await screen.findByRole("article", { name: "Current selection" });
    await waitFor(() => {
      expect(
        within(implementInfo).getByText("No active work is running on this workstation."),
      ).toBeTruthy();
      expect(within(implementInfo).queryByText(activeWorkLabel)).toBeNull();
      expect(within(implementInfo).queryByText("Plan Active")).toBeNull();
    });
  });

  it("shows selected state node details from the graph", async () => {
    renderApp({ snapshot: activeSnapshot });

    const stateButton = await screen.findByRole("button", {
      name: "Select story:implemented state",
    });
    fireEvent.click(stateButton);

    const stateInfo = await screen.findByRole("article", { name: "Current selection" });
    const stateSelectionSlot = stateInfo.closest("[data-bento-card-id]");
    expect(stateButton.getAttribute("aria-pressed")).toBe("true");
    expect(stateSelectionSlot?.getAttribute("data-bento-card-id")).toBe("current-selection");
    expect(within(stateInfo).getByTitle("story:implemented")).toBeTruthy();
    expect(within(stateInfo).getByText("Count")).toBeTruthy();
    expect(within(stateInfo).getByText("Current work")).toBeTruthy();
    expect(within(stateInfo).getByText(activeWorkLabel)).toBeTruthy();
    expect(within(stateInfo).getByText(activeWorkID)).toBeTruthy();

    fireEvent.click(within(stateInfo).getByRole("button", { name: "Select work item Active Story" }));

    const selectedWorkInfo = await screen.findByRole("article", { name: "Current selection" });
    expect(within(selectedWorkInfo).getByRole("heading", { name: "Execution details" })).toBeTruthy();
    expect(within(selectedWorkInfo).getByText("trace-active-story")).toBeTruthy();

    fireEvent.click(await screen.findByRole("button", { name: "Select story:blocked state" }));

    const emptyStateInfo = await screen.findByRole("article", { name: "Current selection" });
    expect(within(emptyStateInfo).getAllByText("blocked").length).toBeGreaterThan(0);
    expect(within(emptyStateInfo).getByTitle("story:blocked")).toBeTruthy();
    expect(
      within(emptyStateInfo).getByText(
        "No work is recorded for this place at the selected tick.",
      ),
    ).toBeTruthy();

    fireEvent.click(await screen.findByRole("button", { name: "Select Review workstation" }));

    const workstationInfo = await screen.findByRole("article", { name: "Current selection" });
    const workstationSelectionSlot = workstationInfo.closest("[data-bento-card-id]");
    expect(workstationInfo).toBeTruthy();
    expect(workstationSelectionSlot).toBe(stateSelectionSlot);
    expect(within(workstationInfo).getByText("Input work types")).toBeTruthy();
  });

  it("keeps selection detail out of the workflow graph inspector layer", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    await screen.findAllByText("dispatch-review-active");

    expect(screen.getByRole("region", { name: "Work graph viewport" })).toBeTruthy();
    expect(screen.queryByRole("complementary", { name: "Workstation Info" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Collapse inspector" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Expand inspector" })).toBeNull();
    expect(screen.getByRole("article", { name: "Current selection" })).toBeTruthy();
  });

  it("renders selected work and traces on the shared dashboard grid", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);

    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    const workInfo = await within(dashboardGrid).findByRole("article", {
      name: "Current selection",
    });
    expect(workInfo).toBeTruthy();
    expect(screen.getByLabelText("Work graph viewport")).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("article", { name: "Completed and failed work" }),
    ).toBeTruthy();
    expect(within(dashboardGrid).getByRole("article", { name: "Trace drill-down" })).toBeTruthy();
    expect(within(dashboardGrid).getByText("Trace drill-down")).toBeTruthy();
    expect(await within(dashboardGrid).findByText("Trace dispatch grid")).toBeTruthy();
  });

  it("supports rearranging shared-grid widgets without replacing graph selection", async () => {
    renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);

    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    const traceWidget = await within(dashboardGrid).findByRole("article", { name: "Trace drill-down" });
    const traceGridItem = traceWidget.closest(".react-grid-item") as HTMLElement;
    const initialStyle = traceGridItem.getAttribute("style");

    fireEvent.mouseDown(within(traceWidget).getByRole("button", { name: "Move Trace drill-down" }), {
      button: 0,
      buttons: 1,
      clientX: 120,
      clientY: 40,
    });
    fireEvent.mouseMove(document, {
      buttons: 1,
      clientX: 360,
      clientY: 40,
    });
    fireEvent.mouseUp(document, {
      button: 0,
      clientX: 360,
      clientY: 40,
    });

    await waitFor(() => {
      expect(traceGridItem.getAttribute("style")).not.toBe(initialStyle);
    });
    const storedLayout = window.localStorage.getItem("agent-factory.dashboard.layout.v2");
    expect(storedLayout).toContain("\"id\":\"trace\"");

    const movedStyle = traceGridItem.getAttribute("style");
    const stream = MockEventSource.instances[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    act(() => {
      stream.emit("snapshot", {
        ...activeSnapshot,
        tick_count: activeSnapshot.tick_count + 1,
      } satisfies DashboardSnapshot);
    });

    await waitFor(() => {
      expect(traceGridItem.getAttribute("style")).toBe(movedStyle);
    });
    expect(
      (
        await screen.findByRole("button", { name: "Select Review workstation" })
      ).getAttribute("aria-pressed"),
    ).toBe("false");
    expect(await within(dashboardGrid).findByText("Trace dispatch grid")).toBeTruthy();
  });

  it("renders queued, in-flight, completed, and failed work in one ranged outcome chart", async () => {
    renderApp({ snapshot: baselineSnapshot });

    await screen.findByRole("heading", { name: "Agent Factory" });
    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    const trendWidget = await within(dashboardGrid).findByRole("article", {
      name: "Work outcome chart",
    });

    expect(within(trendWidget).queryByRole("combobox", { name: "Time range" })).toBeNull();
    expect(
      within(trendWidget).getByRole("img", { name: "Work outcome chart for Session" }),
    ).toBeTruthy();
    expect(within(trendWidget).queryByRole("list", { name: "Work outcome totals" })).toBeNull();
    expect(
      trendWidget.querySelector(
        `[data-axis-tick='x'][data-axis-tick-value='${baselineSnapshot.tick_count}']`,
      ),
    ).toBeTruthy();
    expect(trendWidget.querySelector("[data-axis-tick='y'][data-axis-tick-value='0']")).toBeTruthy();
    expect(trendWidget.querySelector("circle")).toBeNull();
    expect(within(trendWidget).getByText("Ticks")).toBeTruthy();
    expect(within(trendWidget).getByText("Work count")).toBeTruthy();

    const stream = MockEventSource.instances[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    act(() => {
      stream.emit("snapshot", {
        ...baselineSnapshot,
        tick_count: baselineSnapshot.tick_count + 1,
        runtime: {
          ...baselineSnapshot.runtime,
          in_flight_dispatch_count: 3,
          place_token_counts: {
            ...(baselineSnapshot.runtime.place_token_counts ?? {}),
            "story:init": 5,
          },
          session: {
            ...baselineSnapshot.runtime.session,
            completed_count: 4,
            dispatched_count: 7,
            failed_by_work_type: { story: 2 },
            failed_count: 2,
            failed_work_labels: ["Blocked Story", "Rejected Story"],
          },
        },
      } satisfies DashboardSnapshot);
    });

    await waitFor(() => {
      expect(trendWidget.querySelector("[data-axis-gridline='x']")).toBeTruthy();
      expect(trendWidget.querySelector("[data-axis-gridline='y']")).toBeTruthy();
      expect(trendWidget.querySelector("[data-chart-series='queued']")).toBeTruthy();
      expect(trendWidget.querySelector("[data-chart-series='completed']")).toBeTruthy();
    });

    expect(
      within(trendWidget).getByRole("img", { name: "Work outcome chart for Session" }),
    ).toBeTruthy();
  });

  it("updates work outcome chart values when toggling timeline mode between fixed and live snapshots", async () => {
    const historicalWorkOutcomeSnapshot = {
      ...baselineSnapshot,
      tick_count: 1,
      runtime: {
        ...baselineSnapshot.runtime,
        in_flight_dispatch_count: 1,
        place_token_counts: {
          ...(baselineSnapshot.runtime.place_token_counts ?? {}),
          "story:init": 2,
        },
        session: {
          ...baselineSnapshot.runtime.session,
          completed_count: 1,
          completed_work_labels: ["Old Story"],
          failed_count: 0,
          dispatched_count: 2,
          failed_by_work_type: {},
          failed_work_labels: [],
        },
      },
    };
    const liveWorkOutcomeSnapshot = {
      ...historicalWorkOutcomeSnapshot,
      tick_count: 4,
      runtime: {
        ...historicalWorkOutcomeSnapshot.runtime,
        in_flight_dispatch_count: 2,
        place_token_counts: {
          ...(historicalWorkOutcomeSnapshot.runtime.place_token_counts ?? {}),
          "story:init": 4,
        },
        session: {
          ...historicalWorkOutcomeSnapshot.runtime.session,
          completed_count: 8,
          completed_work_labels: ["Old Story", "New Story"],
          failed_count: 3,
          dispatched_count: 10,
          failed_by_work_type: { story: 3 },
          failed_work_labels: ["Blocked Story", "Rejected Story", "Reworked Story"],
        },
      },
    };

    renderApp({
      snapshot: historicalWorkOutcomeSnapshot,
      timelineSnapshots: [historicalWorkOutcomeSnapshot, liveWorkOutcomeSnapshot],
    });

    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    const trendWidget = await within(dashboardGrid).findByRole("article", {
      name: "Work outcome chart",
    });
    const slider = screen.getByRole<HTMLInputElement>("slider", { name: "Timeline tick" });

    expect(
      within(trendWidget).getByRole("region", { name: "Work outcome chart region" }),
    ).toBeTruthy();
    fireEvent.change(slider, { target: { value: "1" } });
    await waitFor(() => {
      expect(trendWidget.querySelector("[data-chart-series='completed']")).toBeTruthy();
      expect(trendWidget.querySelector("[data-axis-tick='x'][data-axis-tick-value='1']")).toBeTruthy();
      expect(trendWidget.querySelector("[data-axis-tick='x'][data-axis-tick-value='4']")).toBeNull();
    });

    fireEvent.change(slider, { target: { value: "4" } });
    await waitFor(() => {
      expect(trendWidget.querySelector("[data-chart-series='failed']")).toBeTruthy();
      expect(trendWidget.querySelector("[data-axis-tick='x'][data-axis-tick-value='1']")).toBeTruthy();
      expect(trendWidget.querySelector("[data-axis-tick='x'][data-axis-tick-value='4']")).toBeTruthy();
      expect(trendWidget.querySelector("[data-axis-gridline='y']")).toBeTruthy();
    });
  });

  it("keeps retry, rework, and timing trends hidden when selected trace data is available", async () => {
    renderApp({
      snapshot: terminalSnapshot,
      traceFixtures: {
        [activeWorkID]: reworkTraceSnapshot,
      },
    });

    await screen.findByRole("heading", { name: "Agent Factory" });
    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    expect(within(dashboardGrid).queryByRole("article", { name: "Failure trend" })).toBeNull();

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);

    const workDetail = screen.getByRole("region", { name: "Agent Factory bento board" });
    expect(await within(workDetail).findByRole("article", { name: "Current selection" })).toBeTruthy();
    expect(within(workDetail).getByRole("article", { name: "Trace drill-down" })).toBeTruthy();
    expect(within(workDetail).queryByRole("article", { name: "Retry and rework trend" })).toBeNull();
    expect(within(workDetail).queryByRole("article", { name: "Timing trend" })).toBeNull();
    expect(workDetail.querySelector('[data-bento-card-id="rework-trend"]')).toBeNull();
    expect(workDetail.querySelector('[data-bento-card-id="timing-trend"]')).toBeNull();
  });

  it.each([1366, 1024, 640])(
    "keeps the widget cards readable at %ipx viewport width",
    async (viewportWidth) => {
      resizeDashboardViewport(viewportWidth);
      renderApp({
        snapshot: terminalSnapshot,
        traceFixtures: {
          [activeWorkID]: reworkTraceSnapshot,
        },
      });

      fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);

      const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });

      const widgets = within(dashboardGrid).getAllByRole("article");
      const widgetNames = widgets.map((widget) => widget.getAttribute("aria-label") ?? "");

      expect(widgetNames).toContain("Work outcome chart");
      expect(widgetNames).toContain("Submit work");
      expect(widgetNames).not.toContain("Completion trend");
      expect(widgetNames).not.toContain("Failure trend");
      expect(widgetNames).not.toContain("Retry and rework trend");
      expect(widgetNames).not.toContain("Timing trend");
      expect(widgetNames).toContain("Completed and failed work");
      expect(widgetNames).toContain("Current selection");
      expect(widgetNames).toContain("Trace drill-down");
      const bentoItems = Array.from(
        dashboardGrid.querySelectorAll<HTMLElement>("[data-bento-card-id]"),
      );
      const cardIds = bentoItems.map((item) => item.dataset.bentoCardId);
      expect(cardIds).toContain("work-outcome-chart");
      expect(cardIds).toContain("submit-work");
      expect(cardIds).not.toContain("completion-trend");
      expect(cardIds).not.toContain("failure-trend");
      expect(cardIds).not.toContain("rework-trend");
      expect(cardIds).not.toContain("timing-trend");
      expect(cardIds).toContain("terminal-work");
      expect(cardIds).toContain("trace");
      expect(cardIds).toContain("current-selection");

      expect(
        within(dashboardGrid).getByRole("img", { name: "Work outcome chart for Session" }),
      ).toBeTruthy();
      expect(
        within(dashboardGrid).queryByRole("img", {
          name: `Timing trend for ${activeWorkID}`,
        }),
      ).toBeNull();
    },
  );

  it("smoke tests the composed bento dashboard at a narrow viewport", async () => {
    resizeDashboardViewport(640);
    renderApp({
      snapshot: terminalSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    await screen.findByRole("heading", { name: "Agent Factory" });
    expect(screen.getAllByRole("region", { name: "Agent Factory bento board" })).toHaveLength(1);
    expect(screen.getByRole("article", { name: "Factory graph" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "Work graph viewport" })).toBeTruthy();

    const activeWorkButton = (await screen.findAllByRole("button", { name: /Active Story/ }))[0];
    fireEvent.click(activeWorkButton);

    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    expect(within(dashboardGrid).getByRole("article", { name: "Work outcome chart" })).toBeTruthy();
    expect(within(dashboardGrid).getByRole("article", { name: "Submit work" })).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("img", { name: "Work outcome chart for Session" }),
    ).toBeTruthy();
    expect(within(dashboardGrid).getByRole("article", { name: "Trace drill-down" })).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("article", { name: "Completed and failed work" }),
    ).toBeTruthy();
    expect(await within(dashboardGrid).findByText("Trace dispatch grid")).toBeTruthy();
    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: /Active Story/ })[0]?.getAttribute("aria-pressed"),
      ).toBe("true");
    });

    const outcomeWidget = within(dashboardGrid).getByRole("article", {
      name: "Work outcome chart",
    });
    const outcomeGridItem = outcomeWidget.closest(".react-grid-item") as HTMLElement;
    const initialOutcomeStyle = outcomeGridItem.getAttribute("style");

    fireEvent.mouseDown(
      within(outcomeWidget).getByRole("button", { name: "Move Work outcome chart" }),
      {
        button: 0,
        buttons: 1,
        clientX: 120,
        clientY: 40,
      },
    );
    fireEvent.mouseMove(document, {
      buttons: 1,
      clientX: 360,
      clientY: 40,
    });
    fireEvent.mouseUp(document, {
      button: 0,
      clientX: 360,
      clientY: 40,
    });

    await waitFor(() => {
      expect(outcomeGridItem.getAttribute("style")).not.toBe(initialOutcomeStyle);
    });

    const terminalWidget = within(dashboardGrid).getByRole("article", {
      name: "Completed and failed work",
    });
    const completedRow = within(terminalWidget)
      .getByRole("heading", { name: "Completed" })
      .closest("section");
    const failedRow = within(terminalWidget)
      .getByRole("heading", { name: "Failed" })
      .closest("section");

    if (!(completedRow instanceof HTMLElement) || !(failedRow instanceof HTMLElement)) {
      throw new Error("expected completed and failed rows to render as terminal sections");
    }

    fireEvent.click(within(completedRow).getByRole("button", { name: "Collapse" }));
    fireEvent.click(within(failedRow).getByRole("button", { name: "Collapse" }));
    fireEvent.click(within(completedRow).getByRole("button", { name: "Expand" }));
    fireEvent.click(within(failedRow).getByRole("button", { name: "Expand" }));

    expect(within(completedRow).getByRole("button", { name: "Done Story" })).toBeTruthy();
    expect(within(failedRow).getByRole("button", { name: "Failed Story" })).toBeTruthy();
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
  });

  it("renders the submit-work card alongside the existing dashboard widgets", async () => {
    renderApp({ snapshot: terminalSnapshot });

    await screen.findByRole("heading", { name: "Agent Factory" });

    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });

    expect(within(dashboardGrid).getByRole("article", { name: "Submit work" })).toBeTruthy();
    expect(within(dashboardGrid).getByRole("article", { name: "Current selection" })).toBeTruthy();
    expect(within(dashboardGrid).getByRole("article", { name: "Trace drill-down" })).toBeTruthy();
    expect(within(dashboardGrid).getByRole("article", { name: "Factory graph" })).toBeTruthy();
    expect(
      dashboardGrid.querySelector('[data-bento-card-id="submit-work"]'),
    ).toBeTruthy();
  });

  it("keeps the export toolbar action available alongside the submit-work card", async () => {
    const { fetchMock } = renderApp({ snapshot: terminalSnapshot });
    fetchMock.mockResolvedValueOnce(
      jsonResponse(
        {
          code: "NOT_FOUND",
          family: "NOT_FOUND",
          message: "Current named factory not found.",
        },
        404,
        "Not Found",
      ),
    );

    await screen.findByRole("heading", { name: "Agent Factory" });

    expect(screen.getByRole("button", { name: "Export PNG" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Export PNG" }));

    const exportDialog = await screen.findByRole("dialog", { name: "Export factory" });
    await waitFor(() => {
      expect(
        within(exportDialog).getByText(
          "The current factory definition is not available yet. Wait for the current-factory API to expose the authored definition before exporting.",
        ),
      ).toBeTruthy();
    });
    expect(within(exportDialog).getByLabelText("Factory name")).toBeTruthy();
  });

  it("smokes the refreshed submit-work flow through the dashboard shell", async () => {
    const { fetchMock } = renderApp({ snapshot: activeSnapshot });
    fetchMock
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ trace_id: "trace-submit-story" }), {
          headers: {
            "Content-Type": "application/json",
          },
          status: 201,
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            code: "BAD_REQUEST",
            message: "work_type_name is required",
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 400,
            statusText: "Bad Request",
          },
        ),
      );

    await screen.findByRole("heading", { name: "Agent Factory" });

    const { requestName, requestText, submitButton, submitWorkScope, workType } =
      submitWorkCardControls();

    expect(Array.from(workType.options, (option) => option.value)).toContain("story");
    expect(submitButton.disabled).toBe(true);

    fireEvent.change(requestName, { target: { value: "Dashboard smoke request" } });
    expect(submitButton.disabled).toBe(true);
    fireEvent.change(requestText, {
      target: { value: "Review the failed dashboard submission smoke." },
    });
    expect(submitButton.disabled).toBe(true);
    fireEvent.change(workType, { target: { value: "story" } });
    expect(submitButton.disabled).toBe(false);

    fireEvent.click(submitButton);

    expect(
      await submitWorkScope.findByText("Your request was submitted. Trace ID: trace-submit-story."),
    ).toBeTruthy();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/work");
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      name: "Dashboard smoke request",
      payload: "Review the failed dashboard submission smoke.",
      workTypeName: "story",
    });
    expect(requestName.value).toBe("");
    expect(requestText.value).toBe("");
    expect(submitButton.disabled).toBe(true);

    fireEvent.change(requestName, { target: { value: "Retry dashboard request" } });
    fireEvent.change(requestText, {
      target: { value: "Retry the broken submission from the dashboard shell." },
    });
    expect(submitButton.disabled).toBe(true);
    fireEvent.change(workType, { target: { value: "story" } });
    expect(submitButton.disabled).toBe(false);

    fireEvent.click(submitButton);

    expect(await submitWorkScope.findByText("work_type_name is required")).toBeTruthy();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/work");
    expect(JSON.parse(String(fetchMock.mock.calls[1]?.[1]?.body))).toEqual({
      name: "Retry dashboard request",
      payload: "Retry the broken submission from the dashboard shell.",
      workTypeName: "story",
    });
    expect(workType.value).toBe("story");
    expect(requestName.value).toBe("Retry dashboard request");
    expect(requestText.value).toBe("Retry the broken submission from the dashboard shell.");
  });

  it("submits configured work through POST /work from the dashboard shell", async () => {
    const { fetchMock } = renderApp({ snapshot: activeSnapshot });
    fetchMock.mockImplementation(async () =>
      new Response(JSON.stringify({ trace_id: "trace-submit-story" }), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 201,
      }),
    );

    await screen.findByRole("heading", { name: "Agent Factory" });

    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    const submitWorkCard = within(dashboardGrid).getByRole("article", { name: "Submit work" });
    const submitWorkScope = within(submitWorkCard);
    const workType = submitWorkScope.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });
    const requestName = submitWorkScope.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    });
    const requestText = submitWorkScope.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Request text",
    });

    expect(Array.from(workType.options, (option) => option.value)).toContain("story");
    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, { target: { value: "Dashboard smoke request" } });
    fireEvent.change(requestText, {
      target: { value: "Review the failed dashboard submission smoke." },
    });
    fireEvent.click(submitWorkScope.getByRole("button", { name: "Submit work" }));

    expect(
      await submitWorkScope.findByText("Your request was submitted. Trace ID: trace-submit-story."),
    ).toBeTruthy();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/work");
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: "POST",
    });
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      name: "Dashboard smoke request",
      payload: "Review the failed dashboard submission smoke.",
      workTypeName: "story",
    });
    expect(requestName.value).toBe("");
  });

  it("preserves the selected work type and request after a dashboard-shell submit failure", async () => {
    const { fetchMock } = renderApp({ snapshot: activeSnapshot });
    fetchMock.mockImplementation(async () =>
      new Response(
        JSON.stringify({
          code: "BAD_REQUEST",
          message: "work_type_name is required",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 400,
          statusText: "Bad Request",
        },
      ),
    );

    await screen.findByRole("heading", { name: "Agent Factory" });

    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    const submitWorkCard = within(dashboardGrid).getByRole("article", { name: "Submit work" });
    const submitWorkScope = within(submitWorkCard);
    const workType = submitWorkScope.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });
    const requestName = submitWorkScope.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    });
    const requestText = submitWorkScope.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Request text",
    });

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, { target: { value: "Retry dashboard request" } });
    fireEvent.change(requestText, {
      target: { value: "Retry the broken submission from the dashboard shell." },
    });
    fireEvent.click(submitWorkScope.getByRole("button", { name: "Submit work" }));

    expect(await submitWorkScope.findByText("work_type_name is required")).toBeTruthy();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/work");
    expect(workType.value).toBe("story");
    expect(requestName.value).toBe("Retry dashboard request");
    expect(requestText.value).toBe("Retry the broken submission from the dashboard shell.");
  });

  it("opens completed and failed work summaries and updates the trace card", async () => {
    renderApp({
      snapshot: terminalSnapshot,
      traceFixtures: {
        [completedWorkID]: completedTraceSnapshot,
        [failedWorkID]: failedTraceSnapshot,
      },
    });

    await screen.findByRole("heading", { name: "Agent Factory" });
    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });
    fireEvent.click(within(dashboardGrid).getByRole("button", { name: "Done Story" }));

    const completedDetail = await screen.findByRole("article", { name: "Current selection" });
    expect(within(completedDetail).getByText("Done Story")).toBeTruthy();
    expect(within(completedDetail).getByRole("heading", { name: "Execution details" })).toBeTruthy();
    expect(within(completedDetail).queryByText("Failure reason")).toBeNull();
    expect(completedDetail).toBeTruthy();
    expect(await within(dashboardGrid).findByText("dispatch-done-story")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Failed Story" }));

    const failedDetail = await screen.findByRole("article", { name: "Current selection" });
    expect(within(failedDetail).getByText("Failed Story")).toBeTruthy();
    expect(within(failedDetail).getAllByText(/FAILED|Failed/).length).toBeGreaterThanOrEqual(1);
    expect(within(failedDetail).getAllByText("Failure reason").length).toBeGreaterThan(0);
    expect(within(failedDetail).getAllByText("provider_rate_limit").length).toBeGreaterThan(0);
    expect(within(failedDetail).getByText("Failure message")).toBeTruthy();
    expect(
      within(failedDetail).getByText("Provider rate limit exceeded while generating the repair."),
    ).toBeTruthy();
    expect(
      within(failedDetail).queryByText(
        "Terminal summaries are reconstructed from retained runtime state.",
      ),
    ).toBeNull();
    expect(await within(dashboardGrid).findByText("dispatch-failed-story")).toBeTruthy();
  });

  it("shows terminal and failed state occupancy in current-selection details", async () => {
    renderApp({
      snapshot: terminalSnapshot,
      timelineSnapshots: [historicalTimelineSnapshot, terminalSnapshot],
    });

    await screen.findByRole("heading", { name: "Agent Factory" });
    const dashboardGrid = screen.getByRole("region", { name: "Agent Factory bento board" });

    fireEvent.click(await screen.findByRole("button", { name: "Select story:complete state" }));

    const completedDetail = await within(dashboardGrid).findByRole("article", {
      name: "Current selection",
    });
    expect(within(completedDetail).getByTitle("story:complete")).toBeTruthy();
    expect(within(completedDetail).getByText("Count")).toBeTruthy();
    expect(within(completedDetail).getByText("Current work")).toBeTruthy();
    expect(within(completedDetail).getByText("Done Story")).toBeTruthy();
    expect(within(completedDetail).getByText(completedWorkID)).toBeTruthy();
    expect(
      within(completedDetail).queryByText("No current work is occupying this place."),
    ).toBeNull();

    fireEvent.click(within(completedDetail).getByRole("button", { name: "Select work item Done Story" }));

    const completedWorkDetail = await within(dashboardGrid).findByRole("article", {
      name: "Current selection",
    });
    expect(within(completedWorkDetail).getByText("Done Story")).toBeTruthy();
    expect(within(completedWorkDetail).getByRole("heading", { name: "Execution details" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Select story:blocked state" }));

    await waitFor(() => {
      const failedDetail = screen.getByRole("article", { name: "Current selection" });

      expect(within(failedDetail).getAllByText("blocked").length).toBeGreaterThan(0);
      expect(within(failedDetail).getByText("Count")).toBeTruthy();
      expect(within(failedDetail).getByText("Current work")).toBeTruthy();
      expect(within(failedDetail).getByText("Failed Story")).toBeTruthy();
      expect(within(failedDetail).getByText(failedWorkID)).toBeTruthy();
      expect(within(failedDetail).getAllByText("Failure reason").length).toBeGreaterThan(0);
      expect(within(failedDetail).getAllByText("provider_rate_limit").length).toBeGreaterThan(0);
      expect(within(failedDetail).getByText("Failure message")).toBeTruthy();
      expect(
        within(failedDetail).getByText(
          "Provider rate limit exceeded while generating the repair.",
        ),
      ).toBeTruthy();
      expect(
        within(failedDetail).queryByText("No current work is occupying this place."),
      ).toBeNull();
    });
  });

  it("shows workstation-scoped workstation runs on the free-floating cards", async () => {
    renderApp({ snapshot: activeSnapshot });

    await screen.findByRole("heading", { name: "Agent Factory" });

    fireEvent.click(await screen.findByRole("button", { name: "Select Review workstation" }));

    const workstationInfo = await screen.findByRole("article", { name: "Current selection" });
    const activeWorkHeading = within(workstationInfo).getByRole("heading", {
      name: "Active work",
    });
    const runHistoryHeading = within(workstationInfo).getByRole("heading", {
      name: "Run history",
    });
    expect(
      activeWorkHeading.compareDocumentPosition(runHistoryHeading) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(within(workstationInfo).getByText("Active Story")).toBeTruthy();
    expect(within(workstationInfo).queryByText(/codex \/ session_id \/ sess-active-story/)).toBeNull();
    const expandButton = within(workstationInfo).getByRole("button", { name: "Expand" });
    expect(expandButton.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(expandButton);
    await waitFor(() => {
      expect(within(workstationInfo).getAllByText(activeWorkLabel).length).toBeGreaterThan(0);
      expect(within(workstationInfo).getByText(/codex \/ session_id \/ sess-active-story/)).toBeTruthy();
      expect(within(workstationInfo).getByText("Repeated work")).toBeTruthy();
      expect(within(workstationInfo).getByText("Raw outcome: REJECTED")).toBeTruthy();
    });

    fireEvent.click(await screen.findByRole("button", { name: "Select Implement workstation" }));

    const implementInfo = await screen.findByRole("article", { name: "Current selection" });
    expect(
      within(implementInfo).getByText("No active work is running on this workstation."),
    ).toBeTruthy();
    expect(within(implementInfo).queryByText("Retry Story")).toBeNull();
    fireEvent.click(within(implementInfo).getByRole("button", { name: "Expand" }));
    await waitFor(() => {
      expect(within(implementInfo).getByText("Retry Story")).toBeTruthy();
      expect(within(implementInfo).getByText("Session log unavailable")).toBeTruthy();
    });
  });

  it("shows an explicit unavailable state when no retained trace history exists", async () => {
    renderApp({
      snapshot: activeSnapshot,
    });

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);

    expect(await screen.findByText("Trace history unavailable")).toBeTruthy();
    expect(
      screen.getByText("No retained dispatch history is currently available for this work item."),
    ).toBeTruthy();
  });

  it("smoke tests predecessor-aware trace drill-down from streamed events through selected work resolution", async () => {
    renderTraceDrilldownHarness({
      selectedWorkID: fanInResultWorkID,
      timelineEvents: buildTraceFanInTimelineEvents(),
    });

    const snapshot = useFactoryTimelineStore.getState().worldViewCache[8];
    expect(
      snapshot?.dashboard.runtime.workstation_requests_by_dispatch_id?.["dispatch-implement"]?.request?.input_work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-b",
        display_name: "Research Context",
        trace_id: "chain-b",
        work_id: "work-research-context",
        work_type_id: "story",
      },
      {
        current_chaining_trace_id: "chain-a",
        display_name: "Reviewed Story",
        trace_id: "chain-a",
        work_id: "work-reviewed-story",
        work_type_id: "story",
      },
    ]);
    expect(
      snapshot?.dashboard.runtime.workstation_requests_by_dispatch_id?.["dispatch-implement"]?.response?.output_work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-a",
        display_name: fanInResultLabel,
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        trace_id: "chain-a",
        work_id: fanInResultWorkID,
        work_type_id: "story",
      },
    ]);
    expect(snapshot?.tracesByWorkID[fanInResultWorkID]?.dispatches.map((dispatch) => dispatch.dispatch_id)).toEqual([
      "dispatch-plan",
      "dispatch-implement",
    ]);

    const traceCard = await screen.findByRole("article", { name: "Trace drill-down" });
    expect(await within(traceCard).findByText("Trace dispatch grid")).toBeTruthy();
    expect(await within(traceCard).findByRole("region", { name: "Dispatch relationship graph" })).toBeTruthy();
    await waitFor(() => {
      expect(within(traceCard).getByText("dispatch-plan")).toBeTruthy();
      expect(within(traceCard).getByText("dispatch-research")).toBeTruthy();
      expect(within(traceCard).getByText("dispatch-implement")).toBeTruthy();
    });
    expect(within(traceCard).getAllByText(/Reviewed Story/).length).toBeGreaterThan(0);
    expect(within(traceCard).getAllByText(/Research Context/).length).toBeGreaterThan(0);
    expect(within(traceCard).getAllByText(new RegExp(fanInResultLabel)).length).toBeGreaterThan(0);
  });

  it("smoke tests legacy trace drill-down fallback from streamed events without predecessor metadata", async () => {
    renderTraceDrilldownHarness({
      selectedWorkID: "work-legacy-done",
      timelineEvents: buildLegacyTraceTimelineEvents(),
    });

    const traceCard = await screen.findByRole("article", { name: "Trace drill-down" });
    expect(await within(traceCard).findByText("Trace dispatch grid")).toBeTruthy();
    expect(await within(traceCard).findByRole("region", { name: "Dispatch relationship graph" })).toBeTruthy();
    await waitFor(() => {
      expect(within(traceCard).getByText("dispatch-legacy-review")).toBeTruthy();
      expect(within(traceCard).getByText("dispatch-legacy-complete")).toBeTruthy();
    });
    expect(within(traceCard).queryByText("dispatch-research")).toBeNull();
  });

  it("resolves trace drill-down from selected-tick events without fetching current trace state", async () => {
    const { fetchMock } = renderApp({
      snapshot: activeSnapshot,
      traceFixtures: {
        [activeWorkID]: traceSnapshot,
      },
    });

    fireEvent.click((await screen.findAllByRole("button", { name: /Active Story/ }))[0]);
    await screen.findByText("Trace dispatch grid");

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("updates completed and failed totals from the live stream", async () => {
    renderApp({ snapshot: baselineSnapshot });

    await screen.findByRole("heading", { name: "Agent Factory" });

    const stream = MockEventSource.instances[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    act(() => {
      stream.onopen?.(new Event("open"));
      stream.emit("snapshot", {
        ...baselineSnapshot,
        runtime: {
          ...baselineSnapshot.runtime,
          session: {
            ...baselineSnapshot.runtime.session,
            completed_count: 3,
            failed_count: 1,
            completed_work_labels: ["work-complete"],
            failed_work_labels: ["work-failed"],
          },
        },
      } satisfies DashboardSnapshot);
    });

    await waitFor(() => {
      const workTotals = screen.getByLabelText("work totals");
      expect(
        within(within(workTotals).getByText("Completed").closest("article") as HTMLElement).getByText("3"),
      ).toBeTruthy();
      expect(
        within(within(workTotals).getByText("Failed").closest("article") as HTMLElement).getByText("1"),
      ).toBeTruthy();
      expect(screen.getByText("Factory event stream connected.")).toBeTruthy();
    });
  });
});
