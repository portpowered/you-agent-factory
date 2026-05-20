import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { RenderResult } from "@testing-library/react";
import { cleanup, render } from "@testing-library/react";
import type { Mock } from "vitest";
import { afterEach, beforeEach, vi } from "vitest";
import { App } from "./App";
import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "./api/dashboard";
import type { FactoryEvent } from "./api/events";
import { installDashboardBrowserTestShims } from "./components/dashboard/test-browser-shims";
import { useDashboardBentoStore } from "./features/bento/state/dashboardBentoStore";
import { reloadDashboardLayoutFromStorage } from "./features/bento/useDashboardLayout";
import { useCurrentEditableFactoryDefinition } from "./features/current-factory-definition";
import { resetSelectionHistoryStore } from "./features/current-selection/state/selectionHistoryStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "./features/dashboard/state/dashboardStreamStore";
import { useExportDialogStore } from "./features/export/state/exportDialogStore";
import type { WorldState } from "./features/timeline/state/factoryTimelineStore";
import { useFactoryTimelineStore } from "./features/timeline/state/factoryTimelineStore";

vi.mock("./features/current-factory-definition", async () => {
  const actual = await vi.importActual("./features/current-factory-definition");

  return {
    ...actual,
    useCurrentEditableFactoryDefinition: vi.fn(),
  };
});

export class MockEventSource {
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

export interface RenderAppOptions {
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

type AppDashboardFetchMock = Mock<
  (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
>;
type RenderAppResult = RenderResult & { fetchMock: AppDashboardFetchMock };

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

export function renderApp({
  browserLanguage,
  browserLanguages,
  initialLocale,
  locationSearch,
  snapshot,
  timelineEvents,
  timelineSnapshots,
  traceFixtures = {},
  workstationRequestsByDispatchID = {},
}: RenderAppOptions): RenderAppResult {
  const queryClient = createAppDashboardQueryClient();

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

export function registerAppDashboardTestLifecycle({
  resetSelectionHistory = true,
}: {
  resetSelectionHistory?: boolean;
} = {}): void {
  beforeEach(() => {
    window.localStorage.clear();
    MockEventSource.instances = [];
    restoreBrowserTestShims = installDashboardBrowserTestShims();
    if (resetSelectionHistory) {
      resetSelectionHistoryStore();
    }
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
    clearAppDashboardQueryClients();
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
    useFactoryTimelineStore.getState().reset();
    if (resetSelectionHistory) {
      resetSelectionHistoryStore();
    }
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });
}

export function requireValue<T>(
  value: T | null | undefined,
  message: string,
): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

export function removeTraceIDsFromSnapshot(
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

function createAppDashboardQueryClient(): QueryClient {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
  queryClients.push(queryClient);

  return queryClient;
}

function clearAppDashboardQueryClients(): void {
  for (const queryClient of queryClients.splice(0)) {
    queryClient.clear();
  }
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
