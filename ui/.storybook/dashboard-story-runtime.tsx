import type { Decorator } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkstationRequest,
} from "../src/api/dashboard";
import type { FactoryEvent } from "../src/api/events";
import { resetSelectionHistoryStore } from "../src/features/current-selection/state/selectionHistoryStore";
import {
  useFactoryTimelineStore,
  type WorldState,
} from "../src/features/timeline/state/factoryTimelineStore";

const DASHBOARD_STORYBOOK_BASE_PATH = "/dashboard/ui/";
type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

interface DashboardApiMockParameters {
  fetchMocks?: DashboardFetchMock[];
  snapshot?: DashboardSnapshot;
  timelineSnapshots?: DashboardSnapshot[];
  timelineEvents?: FactoryEvent[];
  tracesByWorkID?: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

interface DashboardFetchMock {
  method?: string;
  path: string;
  response:
    | DashboardFetchMockResponse
    | ((input: RequestInfo | URL, init?: RequestInit) => DashboardFetchMockResponse | Promise<DashboardFetchMockResponse>);
}

interface DashboardFetchMockResponse {
  body?: BodyInit | null | Record<string, unknown>;
  headers?: HeadersInit;
  status?: number;
  statusText?: string;
}

class DashboardStoryEventSource {
  public onerror: ((event: Event) => void) | null = null;
  public onopen: ((event: Event) => void) | null = null;

  public constructor() {
    queueMicrotask(() => {
      this.onopen?.(new Event("open"));
    });
  }

  public addEventListener(): void {}

  public close(): void {}
}

let originalFetch: FetchLike | null = null;
let originalEventSource: typeof EventSource | undefined;

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

function findFetchMock(
  fetchMocks: readonly DashboardFetchMock[],
  path: string,
  method: string,
): DashboardFetchMock | undefined {
  return fetchMocks.find(
    (fetchMock) =>
      fetchMock.path === path &&
      (fetchMock.method === undefined || fetchMock.method.toUpperCase() === method),
  );
}

function buildFetchMockResponse(mockResponse: DashboardFetchMockResponse): Response {
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

  const latestTick = Math.max(...snapshots.map((snapshot) => snapshot.tick_count));

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
              snapshot.tick_count === latestTick ? workstationRequestsByDispatchID : {},
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
  resetSelectionHistoryStore();
  useFactoryTimelineStore.getState().reset();
}

function installDashboardApiMock(parameters: DashboardApiMockParameters | undefined): void {
  captureBrowserRuntime();
  resetDashboardStoryStores();

  if (!parameters?.snapshot && !parameters?.timelineSnapshots && !parameters?.timelineEvents) {
    window.fetch = originalFetch ?? window.fetch;
    window.EventSource = originalEventSource;
    return;
  }

  const fetchMocks = parameters.fetchMocks ?? [];
  const tracesByWorkID = parameters.tracesByWorkID ?? {};
  const workstationRequestsByDispatchID = parameters.workstationRequestsByDispatchID ?? {};
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

  window.EventSource = DashboardStoryEventSource as unknown as typeof EventSource;
}

function StorybookDashboardRuntime({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={createQueryClient()}>{children}</QueryClientProvider>
  );
}

export const withDashboardStoryRuntime: Decorator = (Story, context) => {
  installDashboardApiMock(
    context.parameters.dashboardApi as DashboardApiMockParameters | undefined,
  );

  return (
    <StorybookDashboardRuntime>
      <Story />
    </StorybookDashboardRuntime>
  );
};
