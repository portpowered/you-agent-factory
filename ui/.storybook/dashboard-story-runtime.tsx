import type { Decorator } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkstationRequest,
} from "../src/api/dashboard";
import type { FactoryEvent } from "../src/api/events";
import { resetSelectionHistoryStore } from "../src/features/current-selection/base/public";
import { resetDashboardSessionStore } from "../src/features/dashboard/state/dashboardSessionStore";
import { useFactoryTimelineStore } from "../src/features/timeline/state/factoryTimelineStore";
import type { WorldState } from "../src/features/timeline/state/factoryTimelineStore";
import { DashboardSessionTestProvider } from "../src/testing/dashboard-session-test-provider";

const DASHBOARD_STORYBOOK_BASE_PATH = "/dashboard/ui/";
type FetchLike = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

interface DashboardApiMockParameters {
  eventSourceMocks?: DashboardEventSourceMock[];
  fetchMocks?: DashboardFetchMock[];
  sessionID?: string | null;
  snapshot?: DashboardSnapshot;
  timelineSnapshots?: DashboardSnapshot[];
  timelineEvents?: FactoryEvent[];
  tracesByWorkID?: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

interface DashboardEventSourceMock {
  events?: FactoryEvent[];
  path: string;
  snapshot?: DashboardSnapshot;
  tracesByWorkID?: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

interface DashboardFetchMock {
  method?: string;
  path: string;
  response:
    | DashboardFetchMockResponse
    | ((
        input: RequestInfo | URL,
        init?: RequestInit,
      ) => DashboardFetchMockResponse | Promise<DashboardFetchMockResponse>);
}

interface DashboardFetchMockResponse {
  body?: BodyInit | null | Record<string, unknown>;
  headers?: HeadersInit;
  status?: number;
  statusText?: string;
}

class DashboardStoryEventSource {
  private readonly listeners = new Map<string, EventListener[]>();

  public constructor(url: string) {
    queueMicrotask(() => {
      const matchedMock = findEventSourceMock(installedEventSourceMocks, url);
      if (matchedMock?.snapshot) {
        seedDashboardStorySnapshot(
          matchedMock.snapshot,
          matchedMock.tracesByWorkID ?? {},
          matchedMock.workstationRequestsByDispatchID ?? {},
        );
      }
      this.onopen?.(new Event("open"));
      for (const event of matchedMock?.events ?? []) {
        this.emit("message", event);
      }
    });
  }

  public onerror: ((event: Event) => void) | null = null;
  public onopen: ((event: Event) => void) | null = null;

  public addEventListener(type: string, listener: EventListener): void {
    const listeners = this.listeners.get(type) ?? [];
    listeners.push(listener);
    this.listeners.set(type, listeners);
  }

  public close(): void {}

  private emit(type: string, data: unknown): void {
    const event = new MessageEvent(type, {
      data: JSON.stringify(data),
    });
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}

let originalFetch: FetchLike | null = null;
let originalEventSource: typeof EventSource | undefined;
let installedEventSourceMocks: readonly DashboardEventSourceMock[] = [];

function captureBrowserRuntime(): void {
  originalFetch ??= window.fetch.bind(window);
  originalEventSource ??= window.EventSource;
}

function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
}

function requestPath(input: RequestInfo | URL): string {
  if (typeof input === "string") {
    return input.startsWith("http") ? new URL(input).pathname : input;
  }

  if (input instanceof URL) {
    return input.pathname;
  }

  return input.url.startsWith("http") ? new URL(input.url).pathname : input.url;
}

function requestMethod(init?: RequestInit): string {
  return (init?.method ?? "GET").toUpperCase();
}

function normalizeURLPath(url: string): string {
  if (url.startsWith("http://") || url.startsWith("https://")) {
    return new URL(url).pathname;
  }

  return url;
}

function findFetchMock(
  fetchMocks: readonly DashboardFetchMock[],
  path: string,
  method: string,
): DashboardFetchMock | undefined {
  return fetchMocks.find(
    (fetchMock) =>
      fetchMock.path === path &&
      (fetchMock.method === undefined ||
        fetchMock.method.toUpperCase() === method),
  );
}

function findEventSourceMock(
  eventSourceMocks: readonly DashboardEventSourceMock[],
  url: string,
): DashboardEventSourceMock | undefined {
  const path = normalizeURLPath(url);
  return eventSourceMocks.find(
    (eventSourceMock) => eventSourceMock.path === path,
  );
}

function buildFetchMockResponse(
  mockResponse: DashboardFetchMockResponse,
): Response {
  const headers = new Headers(mockResponse.headers);
  let responseBody = mockResponse.body ?? null;

  if (
    responseBody !== null &&
    typeof responseBody === "object" &&
    !(responseBody instanceof Blob) &&
    !(responseBody instanceof FormData) &&
    !(responseBody instanceof URLSearchParams) &&
    !(responseBody instanceof ArrayBuffer) &&
    !ArrayBuffer.isView(responseBody)
  ) {
    responseBody = JSON.stringify(responseBody);
    if (!headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }
  }

  return new Response(responseBody, {
    headers,
    status: mockResponse.status ?? 200,
    statusText: mockResponse.statusText,
  });
}

function seedDashboardStorySnapshot(
  snapshot: DashboardSnapshot,
  tracesByWorkID: Record<string, DashboardTrace>,
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>,
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

function seedDashboardStorySnapshots(
  snapshots: readonly DashboardSnapshot[],
  tracesByWorkID: Record<string, DashboardTrace>,
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>,
): void {
  if (snapshots.length === 0) {
    useFactoryTimelineStore.getState().reset();
    return;
  }

  const latestTick = Math.max(
    ...snapshots.map((snapshot) => snapshot.tick_count),
  );

  useFactoryTimelineStore.setState({
    events: [],
    latestTick,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: latestTick,
    worldViewCache: Object.fromEntries(
      snapshots.map(
        (snapshot) =>
          [
            snapshot.tick_count,
            timelineSnapshot(
              snapshot,
              snapshot.tick_count === latestTick ? tracesByWorkID : {},
              snapshot.tick_count === latestTick
                ? workstationRequestsByDispatchID
                : {},
            ),
          ] as const,
      ),
    ),
  });
}

function timelineSnapshot(
  snapshot: DashboardSnapshot,
  tracesByWorkID: Record<string, DashboardTrace>,
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>,
): WorldState {
  return {
    ...snapshot,
    relationsByWorkID: {},
    tracesByWorkID,
    workstationRequestsByDispatchID,
    workRequestsByID: {},
  };
}

function resetDashboardStoryStores(): void {
  resetDashboardSessionStore();
  resetSelectionHistoryStore();
  useFactoryTimelineStore.getState().reset();
}

function installDashboardApiMock(
  parameters: DashboardApiMockParameters | undefined,
): void {
  captureBrowserRuntime();
  resetDashboardStoryStores();
  const fetchMocks = parameters?.fetchMocks ?? [];
  installedEventSourceMocks = parameters?.eventSourceMocks ?? [];
  const hasTimelineData =
    parameters?.snapshot != null ||
    parameters?.timelineSnapshots != null ||
    parameters?.timelineEvents != null;

  if (
    !hasTimelineData &&
    fetchMocks.length === 0 &&
    installedEventSourceMocks.length === 0
  ) {
    window.fetch = originalFetch ?? window.fetch;
    window.EventSource = originalEventSource;
    return;
  }

  const tracesByWorkID = parameters.tracesByWorkID ?? {};
  const workstationRequestsByDispatchID =
    parameters.workstationRequestsByDispatchID ?? {};
  if (parameters.timelineEvents) {
    useFactoryTimelineStore.getState().replaceEvents(parameters.timelineEvents);
  } else if (parameters.timelineSnapshots) {
    seedDashboardStorySnapshots(
      parameters.timelineSnapshots,
      tracesByWorkID,
      workstationRequestsByDispatchID,
    );
  } else if (parameters.snapshot) {
    seedDashboardStorySnapshot(
      parameters.snapshot,
      tracesByWorkID,
      workstationRequestsByDispatchID,
    );
  }

  window.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = requestPath(input);
    const method = requestMethod(init);
    if (path.startsWith(DASHBOARD_STORYBOOK_BASE_PATH)) {
      throw new Error(`unexpected dashboard Storybook fetch for ${path}`);
    }

    const matchedFetchMock = findFetchMock(fetchMocks, path, method);
    if (matchedFetchMock) {
      const mockResponse =
        typeof matchedFetchMock.response === "function"
          ? await matchedFetchMock.response(input, init)
          : matchedFetchMock.response;

      return buildFetchMockResponse(mockResponse);
    }

    return (originalFetch ?? window.fetch)(input, init);
  };

  window.EventSource =
    DashboardStoryEventSource as unknown as typeof EventSource;
}

function StorybookDashboardRuntime({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={createQueryClient()}>
      {children}
    </QueryClientProvider>
  );
}

export const withDashboardStoryRuntime: Decorator = (Story, context) => {
  const dashboardApi = context.parameters.dashboardApi as
    | DashboardApiMockParameters
    | undefined;

  installDashboardApiMock(dashboardApi);

  return (
    <StorybookDashboardRuntime>
      <DashboardSessionTestProvider sessionID={dashboardApi?.sessionID}>
        <Story />
      </DashboardSessionTestProvider>
    </StorybookDashboardRuntime>
  );
};
