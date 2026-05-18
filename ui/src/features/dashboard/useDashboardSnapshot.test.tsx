import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";

import type { DashboardSnapshot } from "../../api/dashboard/types";
import { FACTORY_EVENT_TYPES } from "../../api/events";
import { createReplayHarness } from "../../testing/replay-harness";
import { CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY } from "../current-factory-definition";
import { FACTORY_TIMELINE_DEBUG_STORAGE_KEY } from "../timeline/state/factoryTimelineDebug";
import {
  type WorldState,
  useFactoryTimelineStore,
} from "../timeline/state/factoryTimelineStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "./state/dashboardStreamStore";
import { useDashboardSnapshot } from "./useDashboardSnapshot";

const replayHarness = createReplayHarness();

const SEEDED_SNAPSHOT: DashboardSnapshot = {
  factory_state: "IDLE",
  runtime: {
    in_flight_dispatch_count: 0,
    session: {
      completed_count: 0,
      dispatched_count: 0,
      failed_count: 0,
      has_data: true,
    },
  },
  tick_count: 3,
  topology: {
    edges: [],
    workstation_node_ids: [],
    workstation_nodes_by_id: {},
  },
  uptime_seconds: 12,
};

const REFRESHED_SNAPSHOT: DashboardSnapshot = {
  ...SEEDED_SNAPSHOT,
  factory_state: "RUNNING",
  tick_count: 1,
  uptime_seconds: 1,
};

function timelineSnapshot(snapshot: DashboardSnapshot): WorldState {
  return {
    ...snapshot,
    relationsByWorkID: {},
    tracesByWorkID: {},
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
  };
}

const CANONICAL_SELECTED_TICK_EVENTS = [
  {
    context: {
      eventTime: "2026-04-25T20:00:01Z",
      sequence: 1,
      tick: 1,
    },
    id: "event-1",
    payload: {
      factory: {
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
            outputs: [{ state: "done", workType: "story" }],
            worker: "reviewer",
          },
        ],
        workers: [
          {
            model: "gpt-5.4",
            modelProvider: "codex",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
      },
    },
    type: FACTORY_EVENT_TYPES.initialStructureRequest,
  },
  {
    context: {
      eventTime: "2026-04-25T20:00:02Z",
      requestId: "request-story-1",
      sequence: 2,
      tick: 2,
      traceIds: ["trace-story-1"],
      workIds: ["work-story-1"],
    },
    id: "event-2",
    payload: {
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          name: "Canonical Story",
          trace_id: "trace-story-1",
          work_id: "work-story-1",
          work_type_name: "story",
        },
      ],
    },
    type: FACTORY_EVENT_TYPES.workRequest,
  },
  {
    context: {
      dispatchId: "dispatch-story-1",
      eventTime: "2026-04-25T20:00:03Z",
      requestId: "request-story-1",
      sequence: 3,
      tick: 3,
      traceIds: ["trace-story-1"],
      workIds: ["work-story-1"],
    },
    id: "event-3",
    payload: {
      inputs: [
        {
          workId: "work-story-1",
        },
      ],
      transitionId: "review",
    },
    type: FACTORY_EVENT_TYPES.dispatchRequest,
  },
  {
    context: {
      dispatchId: "dispatch-story-1",
      eventTime: "2026-04-25T20:00:04Z",
      requestId: "request-story-1",
      sequence: 4,
      tick: 4,
      traceIds: ["trace-story-1"],
      workIds: ["work-story-1"],
    },
    id: "event-4",
    payload: {
      attempt: 1,
      inferenceRequestId: "dispatch-story-1/inference/1",
      prompt: "Review the canonical story.",
      workingDirectory: "/work/story",
      worktree: "/work/story/.worktree",
    },
    type: FACTORY_EVENT_TYPES.inferenceRequest,
  },
  {
    context: {
      dispatchId: "dispatch-story-1",
      eventTime: "2026-04-25T20:00:05Z",
      requestId: "request-story-1",
      sequence: 5,
      tick: 5,
      traceIds: ["trace-story-1"],
      workIds: ["work-story-1"],
    },
    id: "event-5",
    payload: {
      attempt: 1,
      diagnostics: {
        provider: {
          model: "gpt-5.4",
          provider: "codex",
          responseMetadata: {
            provider_session_id: "session-story-1",
          },
        },
      },
      durationMillis: 850,
      inferenceRequestId: "dispatch-story-1/inference/1",
      outcome: "SUCCEEDED",
      providerSession: {
        id: "session-story-1",
        kind: "session_id",
        provider: "codex",
      },
      response: "Canonical review complete.",
    },
    type: FACTORY_EVENT_TYPES.inferenceResponse,
  },
  {
    context: {
      dispatchId: "dispatch-story-1",
      eventTime: "2026-04-25T20:00:06Z",
      requestId: "request-story-1",
      sequence: 6,
      tick: 6,
      traceIds: ["trace-story-1"],
      workIds: ["work-story-1"],
    },
    id: "event-6",
    payload: {
      durationMillis: 850,
      outcome: "ACCEPTED",
      outputWork: [
        {
          name: "Canonical Story",
          state: "done",
          trace_id: "trace-story-1",
          work_id: "work-story-1",
          work_type_name: "story",
        },
      ],
      transitionId: "review",
    },
    type: FACTORY_EVENT_TYPES.dispatchResponse,
  },
];

async function emitCanonicalSelectedTickEvents(
  stream: { emit: (type: string, data: unknown) => void },
  count = CANONICAL_SELECTED_TICK_EVENTS.length,
): Promise<void> {
  await act(async () => {
    for (const event of CANONICAL_SELECTED_TICK_EVENTS.slice(0, count)) {
      stream.emit("message", event);
    }
    await new Promise<void>((resolve) => {
      window.setTimeout(() => resolve(), 20);
    });
  });
}

describe("useDashboardSnapshot", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    replayHarness.install();
    queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useFactoryTimelineStore.setState({
      events: [],
      latestTick: SEEDED_SNAPSHOT.tick_count,
      mode: "current",
      receivedEventIDs: [],
      selectedTick: SEEDED_SNAPSHOT.tick_count,
      worldViewCache: {
        [SEEDED_SNAPSHOT.tick_count]: timelineSnapshot(SEEDED_SNAPSHOT),
      },
    });
  });

  afterEach(() => {
    replayHarness.reset();
    window.history.replaceState({}, "", "/");
    window.localStorage.removeItem(FACTORY_TIMELINE_DEBUG_STORAGE_KEY);
    vi.unstubAllGlobals();
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useFactoryTimelineStore.getState().reset();
  });

  it("keeps the seeded snapshot on first mount and reopens the stream after refresh", async () => {
    const { result, rerender } = renderHook(
      ({ refreshToken }: { refreshToken: number }) => useDashboardSnapshot({ refreshToken }),
      { initialProps: { refreshToken: 0 }, wrapper: createWrapper(queryClient) },
    );

    expect(result.current.snapshot?.tick_count).toBe(SEEDED_SNAPSHOT.tick_count);
    expect(replayHarness.getStreams()).toHaveLength(1);
    expect(replayHarness.getStreams()[0]?.url).toBe("/events");

    act(() => {
      rerender({ refreshToken: 1 });
    });

    await waitFor(() => {
      expect(result.current.isInitialLoading).toBe(true);
    });
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    expect(replayHarness.getStreams()).toHaveLength(2);

    act(() => {
      replayHarness.emitSnapshot(REFRESHED_SNAPSHOT);
    });

    await waitFor(() => {
      expect(useFactoryTimelineStore.getState().selectedTick).toBe(REFRESHED_SNAPSHOT.tick_count);
    });
  });

  it("reduces raw canonical /events messages into the current timeline projection", async () => {
    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    expect(replayHarness.getStreams()).toHaveLength(1);

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    await emitCanonicalSelectedTickEvents(stream);

    await waitFor(() => {
      expect(useFactoryTimelineStore.getState().latestTick).toBe(6);
    });

    const snapshot =
      useFactoryTimelineStore.getState().worldViewCache[
        useFactoryTimelineStore.getState().selectedTick
      ];
    expect(
      snapshot?.workstationRequestsByDispatchID["dispatch-story-1"],
    ).toMatchObject({
      dispatch_id: "dispatch-story-1",
      request_view: {
        input_work_items: [
          {
            display_name: "Canonical Story",
            trace_id: "trace-story-1",
            work_id: "work-story-1",
            work_type_id: "story",
          },
        ],
      },
      response_view: {
        duration_millis: 850,
        outcome: "ACCEPTED",
      },
      inference_attempts: [
        expect.objectContaining({
          prompt: "Review the canonical story.",
          response: "Canonical review complete.",
        }),
      ],
      workstation_name: "Review",
    });
    expect(snapshot?.runtime.session.provider_sessions).toMatchObject([
      {
        dispatch_id: "dispatch-story-1",
        outcome: "ACCEPTED",
        provider_session: {
          id: "session-story-1",
          kind: "session_id",
          provider: "codex",
        },
        transition_id: "review",
        workstation_name: "Review",
        work_items: [
          {
            display_name: "Canonical Story",
            trace_id: "trace-story-1",
            work_id: "work-story-1",
            work_type_id: "story",
          },
        ],
      },
    ]);
  });

  it("keeps fixed selected-tick request details stable while later streamed responses advance current mode", async () => {
    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    await emitCanonicalSelectedTickEvents(stream, 4);

    await waitFor(() => {
      expect(useFactoryTimelineStore.getState().latestTick).toBe(4);
    });

    useFactoryTimelineStore.getState().selectTick(4);

    await emitCanonicalSelectedTickEvents(stream, 6);

    await waitFor(() => {
      expect(useFactoryTimelineStore.getState().latestTick).toBe(6);
    });

    expect(useFactoryTimelineStore.getState().mode).toBe("fixed");
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(4);
    expect(
      useFactoryTimelineStore.getState().worldViewCache[4]?.workstationRequestsByDispatchID[
        "dispatch-story-1"
      ],
    ).toMatchObject({
      dispatch_id: "dispatch-story-1",
      request_view: {
        input_work_items: [
          {
            display_name: "Canonical Story",
            trace_id: "trace-story-1",
            work_id: "work-story-1",
            work_type_id: "story",
          },
        ],
      },
      inference_attempts: [
        expect.objectContaining({
          prompt: "Review the canonical story.",
        }),
      ],
      workstation_name: "Review",
    });
    expect(
      useFactoryTimelineStore.getState().worldViewCache[4]?.workstationRequestsByDispatchID[
        "dispatch-story-1"
      ]?.response_view,
    ).toBeUndefined();

    useFactoryTimelineStore.getState().setCurrentMode();

    await waitFor(() => {
      expect(useFactoryTimelineStore.getState().selectedTick).toBe(6);
    });

    expect(
      useFactoryTimelineStore.getState().worldViewCache[6]?.workstationRequestsByDispatchID[
        "dispatch-story-1"
      ],
    ).toMatchObject({
      response_view: {
        outcome: "ACCEPTED",
      },
      inference_attempts: [
        expect.objectContaining({
          response: "Canonical review complete.",
        }),
      ],
    });
  });

  it("can compact streamed event text and persist a recoverable memory summary in debug mode", async () => {
    window.history.replaceState(
      {},
      "",
      "/?afCompactEventText=1&afMemoryDebug=1&afMaxEventTextChars=10",
    );

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    await emitCanonicalSelectedTickEvents(stream, 4);

    await waitFor(() => {
      expect(useFactoryTimelineStore.getState().latestTick).toBe(4);
    });

    const storedPrompt = (
      useFactoryTimelineStore.getState().events[3]?.payload as { prompt?: string }
    )?.prompt;
    expect(storedPrompt).toContain("[truncated ");
    expect(window.localStorage.getItem(FACTORY_TIMELINE_DEBUG_STORAGE_KEY)).toContain(
      "\"eventCount\": 4",
    );
    expect(window.__agentFactoryTimelineDebug__?.summarize().selectedTick).toBe(4);
  });

  it("hydrates the editable current-factory cache from a streamed factory-change event", async () => {
    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    await act(async () => {
      stream.emit("message", {
        context: {
          eventTime: "2026-05-17T15:00:00Z",
          sequence: 8,
          tick: 8,
        },
        id: "factory-event/factory-change/8",
        payload: {
          factory: {
            name: "factory",
            workers: [
              {
                model: "gpt-5.6",
                modelProvider: "CODEX",
                name: "reviewer",
                type: "MODEL_WORKER",
              },
            ],
            workTypes: [{
              name: "story",
              states: [{ name: "new", type: "INITIAL" }],
            }],
            workstations: [
              {
                body: "Updated prompt",
                id: "review",
                inputs: [{ state: "new", workType: "story" }],
                name: "Review",
                outputs: [],
                promptFile: "prompts/review.md",
                worker: "reviewer",
              },
            ],
          },
        },
        type: FACTORY_EVENT_TYPES.factoryChange,
      });
      await new Promise<void>((resolve) => {
        window.setTimeout(() => resolve(), 20);
      });
    });

    await waitFor(() => {
      expect(queryClient.getQueryData(CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY)).toMatchObject(
        {
          workers: [expect.objectContaining({ model: "gpt-5.6" })],
          workstations: [
            expect.objectContaining({
              body: "Updated prompt",
              promptFile: "prompts/review.md",
            }),
          ],
        },
      );
    });
  });
});

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}
