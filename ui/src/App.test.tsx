import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import type {
  DashboardRuntimeWorkstationRequest,
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "./api/dashboard";
import type { FactoryEvent } from "./api/events";
import { FACTORY_EVENT_TYPES } from "./api/events";
import {
  buildDashboardSnapshotFixture,
  mediumBranchingDashboardTopology,
  oneNodeDashboardTopology,
} from "./components/dashboard/fixtures";
import { installDashboardBrowserTestShims } from "./components/dashboard/test-browser-shims";
import {
  semanticWorkflowDashboardSnapshot,
  twentyNodeDashboardSnapshot,
} from "./components/dashboard/test-fixtures";
import { formatDurationMillis } from "./components/ui/formatters";
import { reloadDashboardLayoutFromStorage } from "./features/bento";
import { useDashboardBentoStore } from "./features/bento/state";
import { useCurrentEditableFactoryDefinition } from "./features/current-factory-definition";
import { resetSelectionHistoryStore } from "./features/current-selection/state";
import { DEFAULT_FACTORY_SESSION_ID } from "./api/session-routing";
import { useDashboardSessionStore } from "./features/dashboard/state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "./features/dashboard/state";
import { useExportDialogStore } from "./features/export/state";
import type { WorldState } from "./features/timeline/state";
import { useFactoryTimelineStore } from "./features/timeline/state";
import {
  TraceDrilldownWidget,
  useTraceDrilldown,
} from "./features/trace-drilldown";

vi.mock("./features/current-factory-definition", async () => {
  const actual = await vi.importActual("./features/current-factory-definition");

  return {
    ...actual,
    useCurrentEditableFactoryDefinition: vi.fn(),
  };
});

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
      const tracesByWorkID =
        state.worldViewCache[state.selectedTick]?.tracesByWorkID ?? {};
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
  browserLanguage?: string | null;
  browserLanguages?: readonly string[] | null;
  initialLocale?: string | null;
  locationSearch?: string | null;
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
const fanInResultWorkID = "work-result";
const fanInResultLabel = "Implemented Story";

const baselineSnapshot = buildDashboardSnapshotFixture(
  mediumBranchingDashboardTopology,
);
const activeSnapshot = semanticWorkflowDashboardSnapshot;
const _activeSnapshotWithoutTraceID =
  removeTraceIDsFromSnapshot(activeSnapshot);
const terminalBaseSnapshot = semanticWorkflowDashboardSnapshot;
const terminalSnapshot = {
  ...terminalBaseSnapshot,
  tick_count: 4,
  runtime: {
    ...terminalBaseSnapshot.runtime,
    place_occupancy_work_items_by_place_id: {
      ...(terminalBaseSnapshot.runtime.place_occupancy_work_items_by_place_id ??
        {}),
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
const { edges: _omittedEdges, ...oneNodeTopologyWithoutEdges } =
  oneNodeDashboardTopology;
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

const _completedTraceSnapshot: DashboardTrace = {
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

const _failedTraceSnapshot: DashboardTrace = {
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
    throw new Error(
      `expected ${label} state to be rendered in a React Flow node`,
    );
  }

  return node;
}

function expectStateNodeDotCount(label: string, count: number): void {
  const stateNode = getStateNodeByLabel(label);

  expect(
    stateNode.querySelectorAll("[data-state-work-progress-dot]"),
  ).toHaveLength(count);
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

function _expectRenderedWorkstationRequest(
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
    expectDefinitionValue(
      section,
      "requestTime",
      expected.request.request_time,
    );
  }
  if (expected.request.started_at) {
    expectDefinitionValue(section, "startedAt", expected.request.started_at);
  }
  if (expected.request.working_directory) {
    expectDefinitionValue(
      section,
      "workingDirectory",
      expected.request.working_directory,
    );
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
      expectDefinitionValue(
        section,
        "errorClass",
        expected.response.error_class,
      );
    }
    if (expected.response.failure_reason) {
      expectDefinitionValue(
        section,
        "failureReason",
        expected.response.failure_reason,
      );
    }
    if (expected.response.failure_message) {
      expectDefinitionValue(
        section,
        "failureMessage",
        expected.response.failure_message,
      );
    }
    if (expected.response.response_text) {
      expect(
        within(section).getByText(expected.response.response_text),
      ).toBeTruthy();
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
    within(section).getByText(
      "The workstation request has not produced a response yet.",
    ),
  ).toBeTruthy();
}

function removeTraceIDFromWorkItem(
  workItem: DashboardWorkItemRef,
): DashboardWorkItemRef {
  const withoutTraceID: DashboardWorkItemRef = { work_id: workItem.work_id };
  if (workItem.display_name) {
    withoutTraceID.display_name = workItem.display_name;
  }
  if (workItem.work_type_id) {
    withoutTraceID.work_type_id = workItem.work_type_id;
  }
  return withoutTraceID;
}

function removeTraceIDsFromSnapshot(
  snapshot: DashboardSnapshot,
): DashboardSnapshot {
  return {
    ...snapshot,
    runtime: {
      ...snapshot.runtime,
      active_executions_by_dispatch_id: Object.fromEntries(
        Object.entries(
          snapshot.runtime.active_executions_by_dispatch_id ?? {},
        ).map(([dispatchID, execution]) => [
          dispatchID,
          {
            ...execution,
            trace_ids: [],
            work_items: execution.work_items?.map(removeTraceIDFromWorkItem),
          },
        ]),
      ),
      current_work_items_by_place_id: Object.fromEntries(
        Object.entries(
          snapshot.runtime.current_work_items_by_place_id ?? {},
        ).map(([placeID, workItems]) => [
          placeID,
          workItems.map(removeTraceIDFromWorkItem),
        ]),
      ),
      session: {
        ...snapshot.runtime.session,
        provider_sessions: snapshot.runtime.session.provider_sessions?.map(
          (attempt) => ({
            ...attempt,
            work_items: attempt.work_items?.map(removeTraceIDFromWorkItem),
          }),
        ),
      },
      workstation_activity_by_node_id: Object.fromEntries(
        Object.entries(
          snapshot.runtime.workstation_activity_by_node_id ?? {},
        ).map(([nodeID, activity]) => [
          nodeID,
          {
            ...activity,
            active_work_items: activity.active_work_items?.map(
              removeTraceIDFromWorkItem,
            ),
            trace_ids: [],
          },
        ]),
      ),
    },
  };
}

const traceFanInReviewWorkstation = {
  id: "review",
  inputs: [{ state: "new", workType: "story" }],
  name: "Review",
  outputs: [{ state: "review", workType: "story" }],
  worker: "reviewer",
} as const;

const traceFanInCompleteWorkstation = {
  id: "complete",
  inputs: [{ state: "review", workType: "story" }],
  name: "Complete",
  outputs: [{ state: "active", workType: "story" }],
  worker: "completer",
} as const;

function buildTraceFanInTimelineEvents(): FactoryEvent[] {
  return [
    factoryEvent(
      "trace-fan-in-1",
      1,
      FACTORY_EVENT_TYPES.initialStructureRequest,
      {
        factory: {
          workTypes: [
            {
              name: "story",
              states: [
                { name: "new", type: "INITIAL" },
                { name: "review", type: "PROCESSING" },
                { name: "active", type: "PROCESSING" },
              ],
            },
          ],
          workstations: [
            traceFanInReviewWorkstation,
            traceFanInCompleteWorkstation,
          ],
        },
      },
    ),
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
    factoryEvent(
      "trace-legacy-1",
      1,
      FACTORY_EVENT_TYPES.initialStructureRequest,
      {
        factory: {
          workTypes: [
            {
              name: "story",
              states: [
                { name: "new", type: "INITIAL" },
                { name: "review", type: "PROCESSING" },
                { name: "active", type: "PROCESSING" },
              ],
            },
          ],
          workstations: [
            traceFanInReviewWorkstation,
            traceFanInCompleteWorkstation,
          ],
        },
      },
    ),
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
  factoryEvent(
    "timeline-zero-1",
    0,
    FACTORY_EVENT_TYPES.initialStructureRequest,
    {
      factory: {
        workTypes: [
          {
            name: "story",
            states: [
              { name: "new", type: "INITIAL" },
              { name: "review", type: "PROCESSING" },
            ],
          },
        ],
        workstations: [
          {
            id: "review",
            inputs: [{ state: "new", workType: "story" }],
            name: "Review",
            outputs: [{ state: "review", workType: "story" }],
            worker: "reviewer",
          },
        ],
      },
    },
  ),
];

const queryClients: QueryClient[] = [];
let restoreBrowserTestShims: (() => void) | null = null;

function timelineSnapshot(
  snapshot: DashboardSnapshot,
  tracesByWorkID: Record<string, DashboardTrace> = {},
  workstationRequestsByDispatchID: Record<
    string,
    DashboardWorkstationRequest
  > = {},
): WorldState {
  return {
    ...snapshot,
    relationsByWorkID: {},
    tracesByWorkID,
    workstationRequestsByDispatchID,
    workRequestsByID: {},
  };
}

function seedTimelineSnapshot(
  snapshot: DashboardSnapshot,
  tracesByWorkID: Record<string, DashboardTrace> = {},
  workstationRequestsByDispatchID: Record<
    string,
    DashboardWorkstationRequest
  > = {},
): void {
  useFactoryTimelineStore.setState({
    events: [],
    latestTick: snapshot.tick_count,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: snapshot.tick_count,
    worldViewCache: {
      [snapshot.tick_count]: timelineSnapshot(
        snapshot,
        tracesByWorkID,
        workstationRequestsByDispatchID,
      ),
    },
  });
}

function seedTimelineSnapshots(snapshots: DashboardSnapshot[]): void {
  const worldViewCache = Object.fromEntries(
    snapshots.map(
      (snapshot) =>
        [
          snapshot.tick_count,
          timelineSnapshot(snapshot) satisfies WorldState,
        ] as const,
    ),
  );
  const latestTick = Math.max(
    ...snapshots.map((snapshot) => snapshot.tick_count),
  );

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
  browserLanguage,
  browserLanguages,
  initialLocale,
  locationSearch,
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

  const fetchMock = vi
    .fn()
    .mockImplementation(async (input: RequestInfo | URL) => {
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
  reloadDashboardLayoutFromStorage();
  if (timelineEvents) {
    useFactoryTimelineStore.getState().replaceEvents(timelineEvents);
  } else if (timelineSnapshots) {
    seedTimelineSnapshots(timelineSnapshots);
  } else {
    seedTimelineSnapshot(
      snapshot,
      traceFixtures,
      workstationRequestsByDispatchID,
    );
  }

  const result = render(
    <QueryClientProvider client={queryClient}>
      <App
        browserLanguage={browserLanguage}
        browserLanguages={browserLanguages}
        initialLocale={initialLocale}
        locationSearch={locationSearch}
      />
    </QueryClientProvider>,
  );

  return { ...result, fetchMock };
}

function fetchCallPaths(fetchMock: ReturnType<typeof vi.fn>) {
  return fetchMock.mock.calls.map(([input]) =>
    typeof input === "string"
      ? input
      : input instanceof URL
        ? `${input.pathname}${input.search}`
        : input.url,
  );
}

function nonPromptTemplateFetchPaths(fetchMock: ReturnType<typeof vi.fn>) {
  return fetchCallPaths(fetchMock).filter(
    (path) =>
      !path.includes("/prompt-template-contract") &&
      path !== "/factory-sessions",
  );
}

function submitWorkCardControls() {
  const dashboardGrid = screen.getByRole("region", {
    name: "you-agent-factory bento board",
  });
  const submitWorkCard = within(dashboardGrid).getByRole("article", {
    name: "Submit work",
  });
  const submitWorkScope = within(submitWorkCard);

  return {
    requestName: submitWorkScope.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    }),
    requestText: submitWorkScope.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Request",
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

function jsonResponse(
  body: unknown,
  status = 200,
  statusText?: string,
): Response {
  return new Response(JSON.stringify(body), {
    headers: {
      "Content-Type": "application/json",
    },
    status,
    statusText,
  });
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

function _resizeDashboardViewport(width: number): void {
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

function _getDispatchHistoryCard(
  container: HTMLElement,
  dispatchId: string,
): HTMLElement {
  const dispatchBadge = within(container).getByText(dispatchId);
  const card = dispatchBadge.closest("article");

  if (!(card instanceof HTMLElement)) {
    throw new Error(`expected dispatch history card for ${dispatchId}`);
  }

  return card;
}

function registerAppDashboardTestLifecycle(): void {
  beforeEach(() => {
    window.localStorage.clear();
    MockEventSource.instances = [];
    restoreBrowserTestShims = installDashboardBrowserTestShims();
    resetSelectionHistoryStore();
    useDashboardSessionStore.setState({
      selectedSessionID: "~default",
    });
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue({
      data: undefined,
      error: null,
      failureCount: 0,
      failureReason: null,
      fetchStatus: "idle",
      isError: false,
      isFetched: false,
      isFetchedAfterMount: false,
      isFetching: false,
      isInitialLoading: false,
      isLoading: false,
      isLoadingError: false,
      isPaused: false,
      isPending: true,
      isPlaceholderData: false,
      isRefetchError: false,
      isRefetching: false,
      isStale: true,
      isSuccess: false,
      promise: Promise.resolve(undefined),
      refetch: vi.fn(),
      status: "pending",
    } as never);
  });

  afterEach(() => {
    for (const queryClient of queryClients.splice(0)) {
      queryClient.clear();
    }
    cleanup();
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
    useExportDialogStore.setState({
      isExportDialogOpen: false,
    });
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useDashboardSessionStore.setState({
      selectedSessionID: "~default",
    });
    useFactoryTimelineStore.getState().reset();
    resetSelectionHistoryStore();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });
}

function registerAppFollowUpTestLifecycle(): void {
  beforeEach(() => {
    window.localStorage.clear();
    MockEventSource.instances = [];
    restoreBrowserTestShims = installDashboardBrowserTestShims();
    useDashboardSessionStore.setState({
      selectedSessionID: "~default",
    });
  });

  afterEach(() => {
    for (const queryClient of queryClients.splice(0)) {
      queryClient.clear();
    }
    cleanup();
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
    useExportDialogStore.setState({
      isExportDialogOpen: false,
    });
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useDashboardSessionStore.setState({
      selectedSessionID: "~default",
    });
    useFactoryTimelineStore.getState().reset();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });
}

describe("App dashboard layout and graph behavior", () => {
  registerAppDashboardTestLifecycle();

  it("renders backend tick-zero initial structure instead of staying in loading state", async () => {
    renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: tickZeroInitialStructureRequestEvents,
    });

    expect(
      await screen.findByRole("heading", { name: "you-agent-factory" }),
    ).toBeTruthy();
    expect(screen.queryByText("Loading dashboard")).toBeNull();
    expect(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    ).toBeTruthy();
    expect(
      (
        screen.getByRole("slider", {
          name: "Timeline tick",
        }) as HTMLInputElement
      ).value,
    ).toBe("0");
    expect(screen.getByText("Waiting for more ticks")).toBeTruthy();
  });

  it("starts with full-width totals above a full-width Factory graph card", async () => {
    renderApp({ snapshot: baselineSnapshot });

    await screen.findByRole("heading", { name: "you-agent-factory" });

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const workTotals = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-totals"]',
    );
    const workflowActivity = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-graph"]',
    );
    if (!workTotals || !workflowActivity) {
      throw new Error(
        "expected totals and workflow cards to render in the dashboard grid",
      );
    }

    expect(workTotals.dataset.layoutSignature).toContain(
      "work-totals:0:0:12:2",
    );
    expect(workflowActivity.dataset.layoutSignature).toContain(
      "work-graph:0:2:12:8",
    );
    expect(
      within(screen.getByLabelText("work totals")).getByText("In progress"),
    ).toBeTruthy();
    expect(
      within(screen.getByLabelText("work totals")).getByText("Completed"),
    ).toBeTruthy();
    expect(
      within(screen.getByLabelText("work totals")).getByText("Failed"),
    ).toBeTruthy();
    expect(
      within(screen.getByLabelText("work totals")).getByText("Dispatched"),
    ).toBeTruthy();
  });

  it("migrates the stored dashboard baseline to the compacted factory graph height", async () => {
    window.localStorage.setItem(
      "agent-factory.dashboard.layout.v2",
      JSON.stringify([
        { h: 2, id: "work-totals", w: 12, x: 0, y: 0 },
        { h: 10, id: "work-graph", w: 12, x: 0, y: 2 },
        { h: 5, id: "current-selection", w: 4, x: 0, y: 12 },
        { h: 5, id: "terminal-work", w: 4, x: 4, y: 12 },
        { h: 6, id: "work-outcome-chart", w: 4, x: 8, y: 12 },
        { h: 6, id: "submit-work", w: 4, x: 8, y: 18 },
        { h: 9, id: "trace", w: 8, x: 0, y: 18 },
      ]),
    );

    renderApp({ snapshot: activeSnapshot });

    await screen.findByRole("heading", { name: "you-agent-factory" });

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const workflowActivity = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-graph"]',
    );
    const currentSelection = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="current-selection"]',
    );
    const trace = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="trace"]',
    );

    expect(workflowActivity?.dataset.layoutSignature).toContain(
      "work-graph:0:2:12:8",
    );
    expect(currentSelection?.dataset.layoutSignature).toContain(
      "current-selection:0:10:4:5",
    );
    expect(trace?.dataset.layoutSignature).toContain("trace:0:15:8:9");
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

    await screen.findByRole("heading", { name: "you-agent-factory" });

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const currentSelection = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="current-selection"]',
    );
    const legacySelection = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-info"], [data-bento-card-id="workstation-info"], [data-bento-card-id="terminal-summary"]',
    );

    expect(currentSelection).toBeTruthy();
    expect(legacySelection).toBeNull();
    expect(currentSelection?.dataset.layoutSignature).toMatch(
      /current-selection:7:\d+:5:6/,
    );
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

    await screen.findByRole("heading", { name: "you-agent-factory" });

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const workOutcome = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="work-outcome-chart"]',
    );
    const legacyCharts = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="completion-trend"], [data-bento-card-id="failure-trend"]',
    );

    expect(workOutcome).toBeTruthy();
    expect(legacyCharts).toBeNull();
    expect(workOutcome?.dataset.layoutSignature).toMatch(
      /work-outcome-chart:7:\d+:5:5/,
    );
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

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const trace = await within(dashboardGrid).findByRole("article", {
      name: "Trace drill-down",
    });
    const hiddenTrendCards = dashboardGrid.querySelector<HTMLElement>(
      '[data-bento-card-id="rework-trend"], [data-bento-card-id="timing-trend"]',
    );

    expect(hiddenTrendCards).toBeNull();
    expect(
      within(dashboardGrid).queryByRole("article", {
        name: "Retry and rework trend",
      }),
    ).toBeNull();
    expect(
      within(dashboardGrid).queryByRole("article", { name: "Timing trend" }),
    ).toBeNull();
    const layoutSignature =
      trace.closest<HTMLElement>("[data-bento-card-id]")?.dataset
        .layoutSignature ?? "";
    expect(layoutSignature).not.toContain("rework-trend");
    expect(layoutSignature).not.toContain("timing-trend");
    expect(layoutSignature).toMatch(/trace:\d+:\d+:\d+:\d+/);
  });

  it("renders distinct graph semantics for topology places, active work, and retry outcomes", async () => {
    renderApp({ snapshot: activeSnapshot });

    expect(
      (await screen.findAllByText("dispatch-review-active")).length,
    ).toBeGreaterThan(0);
    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: /Select .* workstation/ }),
      ).toHaveLength(5);
    });
    expect(screen.queryByText("Workstation Definition")).toBeNull();
    expect(screen.queryByText("State Position")).toBeNull();
    expect(screen.getByLabelText("agent-slot:available")).toBeTruthy();
    expect(screen.getByLabelText("2 resource tokens")).toBeTruthy();
    expect(screen.getByText("quality-gate:ready")).toBeTruthy();
    expect(screen.getByLabelText("1 constraint token")).toBeTruthy();
    const constraintArticle = screen
      .getByText("quality-gate:ready")
      .closest("article");

    expect(
      screen
        .getByRole("img", { name: "Resource" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("resource");
    expect(
      screen
        .getByRole("img", { name: "Constraint" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("constraint");
    expect(constraintArticle?.textContent).not.toContain("Constraint");
    expect(screen.queryByText("Active Work")).toBeNull();
    expectStateNodeDotCount("story:ready", 3);
    expect(getStateNodeByLabel("story:blocked")).toBeTruthy();
    expect(getStateNodeByLabel("story:complete")).toBeTruthy();
  });

  it("renders a valid single-workstation topology when the API omits empty edges", async () => {
    renderApp({ snapshot: singleNodeSnapshotWithoutEdges });

    expect(
      await screen.findByRole("heading", { name: "you-agent-factory" }),
    ).toBeTruthy();
    expect(
      await screen.findByRole("button", { name: "Select Intake workstation" }),
    ).toBeTruthy();
    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(currentSelection).toBeTruthy();
    fireEvent.click(
      within(currentSelection).getByRole("button", { name: "Expand" }),
    );
    expect(
      within(currentSelection).getByText(
        "No workstation runs have been recorded for this workstation yet.",
      ),
    ).toBeTruthy();
  });

  it("uses React Flow controls for work graph zoom interaction", async () => {
    renderApp({ snapshot: baselineSnapshot });

    await screen.findByRole("heading", { name: "you-agent-factory" });

    const workGraphViewport = screen.getByRole("region", {
      name: "Work graph viewport",
    });
    expect(workGraphViewport).toBeTruthy();
    const flowViewport = document.querySelector<HTMLElement>(
      ".react-flow__viewport",
    );
    const initialTransform = flowViewport?.style.transform;

    fireEvent.click(
      within(workGraphViewport).getByRole("button", { name: "Zoom In" }),
    );

    await waitFor(() => {
      expect(flowViewport?.style.transform).not.toBe(initialTransform);
    });
  });

  it("renders and interacts with a 20-node workflow through React Flow", async () => {
    renderApp({ snapshot: twentyNodeSnapshot });

    await screen.findByRole("heading", { name: "you-agent-factory" });

    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: /Select .* workstation/ }),
      ).toHaveLength(20);
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
    expect(
      screen.getByRole("heading", { name: "Current selection" }),
    ).toBeTruthy();
  });
});

describe("App dashboard follow-up flows", () => {
  registerAppFollowUpTestLifecycle();

  it("renders the submit-work card alongside the existing dashboard widgets", async () => {
    renderApp({ snapshot: terminalSnapshot });

    await screen.findByRole("heading", { name: "you-agent-factory" });

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });

    expect(
      within(dashboardGrid).getByRole("article", { name: "Submit work" }),
    ).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("article", { name: "Current selection" }),
    ).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("article", { name: "Trace drill-down" }),
    ).toBeTruthy();
    expect(
      within(dashboardGrid).getByRole("article", { name: "Factory graph" }),
    ).toBeTruthy();
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

    await screen.findByRole("heading", { name: "you-agent-factory" });

    expect(screen.getByRole("button", { name: "Export PNG" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Export PNG" }));

    const exportDialog = await screen.findByRole("dialog", {
      name: "Export factory",
    });
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
        new Response(JSON.stringify({ traceId: "trace-submit-story" }), {
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

    await screen.findByRole("heading", { name: "you-agent-factory" });

    const {
      requestName,
      requestText,
      submitButton,
      submitWorkScope,
      workType,
    } = submitWorkCardControls();

    expect(Array.from(workType.options, (option) => option.value)).toContain(
      "story",
    );
    expect(submitButton.disabled).toBe(true);
    expect(
      submitWorkScope.getByText(
        "Choose a work type and enter a request name to continue.",
      ),
    ).toBeTruthy();
    expect(
      submitWorkScope.queryByText(
        "Optional. Leave this blank to submit an empty request.",
      ),
    ).toBeNull();

    fireEvent.change(workType, { target: { value: "story" } });
    expect(submitButton.disabled).toBe(true);
    expect(
      submitWorkScope.getByText("Enter a request name to continue."),
    ).toBeTruthy();
    fireEvent.change(requestName, {
      target: { value: "Dashboard smoke request" },
    });
    fireEvent.change(requestText, {
      target: { value: "Review the failed dashboard submission smoke." },
    });

    fireEvent.click(submitButton);

    expect(
      await submitWorkScope.findByText(
        "Your request was submitted. Trace ID: trace-submit-story.",
      ),
    ).toBeTruthy();
    const submitCalls = nonPromptTemplateFetchPaths(fetchMock);
    expect(submitCalls).toEqual([
      `/factories/${DEFAULT_FACTORY_SESSION_ID}/work`,
    ]);
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body))).toEqual({
      name: "Dashboard smoke request",
      payload: "Review the failed dashboard submission smoke.",
      workTypeName: "story",
    });
    expect(workType.value).toBe("story");
    expect(requestName.value).toBe("");
    expect(requestText.value).toBe("");
    expect(submitButton.disabled).toBe(true);

    fireEvent.change(requestName, {
      target: { value: "Retry dashboard request" },
    });
    expect(submitButton.disabled).toBe(false);
    fireEvent.change(requestText, {
      target: {
        value: "Retry the broken submission from the dashboard shell.",
      },
    });

    fireEvent.click(submitButton);

    expect(
      await submitWorkScope.findByText("work_type_name is required"),
    ).toBeTruthy();
    expect(submitCalls).toEqual([
      `/factories/${DEFAULT_FACTORY_SESSION_ID}/work`,
    ]);
    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([
      `/factories/${DEFAULT_FACTORY_SESSION_ID}/work`,
      `/factories/${DEFAULT_FACTORY_SESSION_ID}/work`,
    ]);
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body))).toEqual({
      name: "Retry dashboard request",
      payload: "Retry the broken submission from the dashboard shell.",
      workTypeName: "story",
    });
    expect(workType.value).toBe("story");
    expect(requestName.value).toBe("Retry dashboard request");
    expect(requestText.value).toBe(
      "Retry the broken submission from the dashboard shell.",
    );
  });

  it("submits configured work through POST /work from the dashboard shell", async () => {
    const { fetchMock } = renderApp({ snapshot: activeSnapshot });
    fetchMock.mockImplementation(
      async () =>
        new Response(JSON.stringify({ traceId: "trace-submit-story" }), {
          headers: {
            "Content-Type": "application/json",
          },
          status: 201,
        }),
    );

    await screen.findByRole("heading", { name: "you-agent-factory" });

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const submitWorkCard = within(dashboardGrid).getByRole("article", {
      name: "Submit work",
    });
    const submitWorkScope = within(submitWorkCard);
    const workType = submitWorkScope.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });
    const requestName = submitWorkScope.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    });
    const requestText = submitWorkScope.getByRole<HTMLTextAreaElement>(
      "textbox",
      {
        name: "Request",
      },
    );

    expect(Array.from(workType.options, (option) => option.value)).toContain(
      "story",
    );
    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, {
      target: { value: "Dashboard smoke request" },
    });
    fireEvent.change(requestText, {
      target: { value: "Review the failed dashboard submission smoke." },
    });
    fireEvent.click(
      submitWorkScope.getByRole("button", { name: "Submit work" }),
    );

    expect(
      await submitWorkScope.findByText(
        "Your request was submitted. Trace ID: trace-submit-story.",
      ),
    ).toBeTruthy();
    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([
      `/factories/${DEFAULT_FACTORY_SESSION_ID}/work`,
    ]);
    expect(fetchMock.mock.calls.at(-1)?.[1]).toMatchObject({
      method: "POST",
    });
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body))).toEqual({
      name: "Dashboard smoke request",
      payload: "Review the failed dashboard submission smoke.",
      workTypeName: "story",
    });
    expect(workType.value).toBe("story");
    expect(requestName.value).toBe("");
    expect(requestText.value).toBe("");
  });

  it("submits an empty payload through POST /work from the dashboard shell when request name is present", async () => {
    const { fetchMock } = renderApp({ snapshot: activeSnapshot });
    fetchMock.mockImplementation(
      async () =>
        new Response(JSON.stringify({ traceId: "trace-submit-story" }), {
          headers: {
            "Content-Type": "application/json",
          },
          status: 201,
        }),
    );

    await screen.findByRole("heading", { name: "you-agent-factory" });

    const { requestName, submitButton, submitWorkScope, workType } =
      submitWorkCardControls();

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, {
      target: { value: "Dashboard empty payload request" },
    });
    expect(submitButton.disabled).toBe(false);
    fireEvent.click(submitButton);

    expect(
      await submitWorkScope.findByText(
        "Your request was submitted. Trace ID: trace-submit-story.",
      ),
    ).toBeTruthy();
    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([
      `/factories/${DEFAULT_FACTORY_SESSION_ID}/work`,
    ]);
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body))).toEqual({
      name: "Dashboard empty payload request",
      payload: "",
      workTypeName: "story",
    });
  });

  it("preserves the selected work type and request after a dashboard-shell submit failure", async () => {
    const { fetchMock } = renderApp({ snapshot: activeSnapshot });
    fetchMock.mockImplementation(
      async () =>
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

    await screen.findByRole("heading", { name: "you-agent-factory" });

    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });
    const submitWorkCard = within(dashboardGrid).getByRole("article", {
      name: "Submit work",
    });
    const submitWorkScope = within(submitWorkCard);
    const workType = submitWorkScope.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });
    const requestName = submitWorkScope.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    });
    const requestText = submitWorkScope.getByRole<HTMLTextAreaElement>(
      "textbox",
      {
        name: "Request",
      },
    );

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, {
      target: { value: "Retry dashboard request" },
    });
    fireEvent.change(requestText, {
      target: {
        value: "Retry the broken submission from the dashboard shell.",
      },
    });
    fireEvent.click(
      submitWorkScope.getByRole("button", { name: "Submit work" }),
    );

    expect(
      await submitWorkScope.findByText("work_type_name is required"),
    ).toBeTruthy();
    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([
      `/factories/${DEFAULT_FACTORY_SESSION_ID}/work`,
    ]);
    expect(workType.value).toBe("story");
    expect(requestName.value).toBe("Retry dashboard request");
    expect(requestText.value).toBe(
      "Retry the broken submission from the dashboard shell.",
    );
  });

  it("shows workstation-scoped workstation runs on the free-floating cards", async () => {
    renderApp({ snapshot: activeSnapshot });

    await screen.findByRole("heading", { name: "you-agent-factory" });

    fireEvent.click(
      await screen.findByRole("button", { name: "Select Review workstation" }),
    );

    const workstationInfo = await screen.findByRole("article", {
      name: "Current selection",
    });
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
    expect(
      within(workstationInfo).queryByText(
        /codex \/ session_id \/ sess-active-story/,
      ),
    ).toBeNull();
    const expandButton = within(workstationInfo).getByRole("button", {
      name: "Expand",
    });
    expect(expandButton.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(expandButton);
    await waitFor(() => {
      expect(
        within(workstationInfo).getAllByText(activeWorkLabel).length,
      ).toBeGreaterThan(0);
      expect(
        within(workstationInfo).getByText(
          /codex \/ session_id \/ sess-active-story/,
        ),
      ).toBeTruthy();
      expect(within(workstationInfo).getByText("Repeated work")).toBeTruthy();
      expect(
        within(workstationInfo).getByText("Raw outcome: REJECTED"),
      ).toBeTruthy();
    });

    fireEvent.click(
      await screen.findByRole("button", {
        name: "Select Implement workstation",
      }),
    );

    const implementInfo = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(implementInfo).getByText(
        "No active work is running on this workstation.",
      ),
    ).toBeTruthy();
    expect(within(implementInfo).queryByText("Retry Story")).toBeNull();
    fireEvent.click(
      within(implementInfo).getByRole("button", { name: "Expand" }),
    );
    await waitFor(() => {
      expect(within(implementInfo).getByText("Retry Story")).toBeTruthy();
      expect(
        within(implementInfo).getByText("Session log unavailable"),
      ).toBeTruthy();
    });
  });

  it("smoke tests predecessor-aware trace drill-down from streamed events through selected work resolution", async () => {
    renderTraceDrilldownHarness({
      selectedWorkID: fanInResultWorkID,
      timelineEvents: buildTraceFanInTimelineEvents(),
    });

    const snapshot = useFactoryTimelineStore.getState().worldViewCache[8];
    expect(
      snapshot?.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-implement"
      ]?.request?.input_work_items,
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
      snapshot?.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-implement"
      ]?.response?.output_work_items,
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
    expect(
      snapshot?.tracesByWorkID[fanInResultWorkID]?.dispatches.map(
        (dispatch) => dispatch.dispatch_id,
      ),
    ).toEqual(["dispatch-plan", "dispatch-implement"]);

    const traceCard = await screen.findByRole("article", {
      name: "Trace drill-down",
    });
    expect(
      await within(traceCard).findByText("Trace dispatch grid"),
    ).toBeTruthy();
    expect(
      await within(traceCard).findByRole("region", {
        name: "Dispatch relationship graph",
      }),
    ).toBeTruthy();
    await waitFor(() => {
      expect(within(traceCard).getByText("dispatch-plan")).toBeTruthy();
      expect(within(traceCard).getByText("dispatch-research")).toBeTruthy();
      expect(within(traceCard).getByText("dispatch-implement")).toBeTruthy();
    });
    expect(
      within(traceCard).getAllByText(/Reviewed Story/).length,
    ).toBeGreaterThan(0);
    expect(
      within(traceCard).getAllByText(/Research Context/).length,
    ).toBeGreaterThan(0);
    expect(
      within(traceCard).getAllByText(new RegExp(fanInResultLabel)).length,
    ).toBeGreaterThan(0);
  });

  it("smoke tests legacy trace drill-down fallback from streamed events without predecessor metadata", async () => {
    renderTraceDrilldownHarness({
      selectedWorkID: "work-legacy-done",
      timelineEvents: buildLegacyTraceTimelineEvents(),
    });

    const traceCard = await screen.findByRole("article", {
      name: "Trace drill-down",
    });
    expect(
      await within(traceCard).findByText("Trace dispatch grid"),
    ).toBeTruthy();
    expect(
      await within(traceCard).findByRole("region", {
        name: "Dispatch relationship graph",
      }),
    ).toBeTruthy();
    await waitFor(() => {
      expect(
        within(traceCard).getByText("dispatch-legacy-review"),
      ).toBeTruthy();
      expect(
        within(traceCard).getByText("dispatch-legacy-complete"),
      ).toBeTruthy();
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

    fireEvent.click(
      (await screen.findAllByRole("button", { name: /Active Story/ }))[0],
    );
    await screen.findByText("Trace dispatch grid");

    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([]);
  });

  it("updates completed and failed totals from the live stream", async () => {
    renderApp({ snapshot: baselineSnapshot });

    await screen.findByRole("heading", { name: "you-agent-factory" });

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
        within(
          within(workTotals)
            .getByText("Completed")
            .closest("article") as HTMLElement,
        ).getByText("3"),
      ).toBeTruthy();
      expect(
        within(
          within(workTotals)
            .getByText("Failed")
            .closest("article") as HTMLElement,
        ).getByText("1"),
      ).toBeTruthy();
      expect(
        screen.getByRole("status", {
          name: "you-agent-factory event stream live",
        }),
      ).toBeTruthy();
    });
  });
});
