import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, vi, type Mock } from "bun:test";
import { App } from "../App";
import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkstationRequest,
} from "../api/dashboard";
import type { FactoryEvent } from "../api/events";
import type { FactorySessionSummary } from "../api/factory-sessions/api";
import { DEFAULT_FACTORY_SESSION_ID } from "../api/session-routing";
import { installDashboardBrowserTestShims } from "../components/dashboard/test-browser-shims";
import { semanticWorkflowDashboardSnapshot } from "../components/dashboard/test-fixtures";
import { reloadDashboardLayoutFromStorage } from "../features/bento/hooks/useDashboardLayout";
import { useDashboardBentoStore } from "../features/bento/state/dashboardBentoStore";
import { useCurrentFactoryDocument } from "../features/current-factory-definition/public";
import { resetSelectionHistoryStore } from "../features/current-selection/base/public";
import { useDashboardSessionStore } from "../features/dashboard/state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../features/dashboard/state/dashboardStreamStore";
import { useExportDialogStore } from "../features/export/state/exportDialogStore";
import {
  useFactoryTimelineStore,
  type WorldState,
} from "../features/timeline/state/factoryTimelineStore";
import { DashboardSessionTestProvider } from "./dashboard-session-test-provider";
import {
  createFactoryImportValue,
  createFileDropTransfer,
  jsonResponse,
  nonPromptTemplateFetchPaths,
  type FetchMock,
} from "../../testing/app-shell-test-http-utils";

export {
  activeSnapshot,
  baselineSnapshot,
  importedFactorySnapshot,
  terminalSnapshot,
} from "./app-shell-test-snapshots";
export {
  createFactoryImportValue,
  createFileDropTransfer,
  jsonResponse,
  nonPromptTemplateFetchPaths,
};
export {
  renderWithDashboardSessionTest,
  wrapWithDashboardSessionTest,
} from "./dashboard-session-test-utils";

const isBunTestRuntime = typeof Bun !== "undefined";

const globalStubStack: Array<{
  key: keyof typeof globalThis;
  previous: unknown;
  previousWindow: unknown;
}> = [];

function stubGlobal<K extends keyof typeof globalThis>(
  key: K,
  value: (typeof globalThis)[K],
): void {
  const windowRef = globalThis.window as
    | (Window & typeof globalThis & Record<string, unknown>)
    | undefined;
  const previousWindowValue = windowRef?.[key as string];

  const globalTarget = globalThis as Record<string, unknown>;

  globalStubStack.push({
    key,
    previous: globalTarget[key as string],
    previousWindow: previousWindowValue,
  });
  globalTarget[key as string] = value;
  if (windowRef) {
    windowRef[key as string] = value;
  }
}

function restoreGlobalStubs(): void {
  const globalTarget = globalThis as Record<string, unknown>;
  const windowRef = globalThis.window as
    | (Window & typeof globalThis & Record<string, unknown>)
    | undefined;

  while (globalStubStack.length > 0) {
    const entry = globalStubStack.pop();
    if (!entry) {
      break;
    }
    const { key, previous, previousWindow } = entry;
    globalTarget[key as string] = previous;
    if (windowRef) {
      windowRef[key as string] = previousWindow;
    }
  }
}

export class MockEventSource {
  public static instances: MockEventSource[] = [];

  public onerror: ((event: Event) => void) | null = null;
  public onopen: ((event: Event) => void) | null = null;

  private readonly listeners = new Map<string, EventListener[]>();

  public constructor(public readonly url: string) {
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
  sessionID?: string | null;
  snapshot: DashboardSnapshot;
  timelineEvents?: FactoryEvent[];
  timelineSnapshots?: DashboardSnapshot[];
  traceFixtures?: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

interface RenderAppResult extends ReturnType<typeof render> {
  fetchMock: FetchMock;
}

type CurrentFactoryDocumentResult = ReturnType<
  typeof useCurrentFactoryDocument
>;

const queryClients: QueryClient[] = [];
let restoreBrowserTestShims: (() => void) | null = null;
const defaultFactorySessionSummary: FactorySessionSummary = {
  factoryDir: "/workspace/default",
  folderPath: "/workspace",
  id: DEFAULT_FACTORY_SESSION_ID,
  isDefault: true,
  project: "default",
  target: {
    kind: "default",
  },
};

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

export async function waitForDashboardShell(): Promise<void> {
  await screen.findByRole("heading", { name: "U" });
}

export function renderApp({
  browserLanguage,
  browserLanguages,
  initialLocale,
  locationSearch,
  sessionID,
  snapshot,
  timelineEvents,
  timelineSnapshots,
  traceFixtures = {},
  workstationRequestsByDispatchID = {},
}: RenderAppOptions): RenderAppResult {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
  queryClients.push(queryClient);

  const fetchMock: FetchMock = vi.fn(
    async (input: RequestInfo | URL) => {
      const path =
        typeof input === "string"
          ? input
          : input instanceof URL
            ? `${input.pathname}${input.search}`
            : input.url;

      if (path === "/factory-sessions") {
        return jsonResponse({
          sessions: [defaultFactorySessionSummary],
        });
      }

      throw new Error(`unexpected fetch for ${path}`);
    },
  );

  stubGlobal("fetch", fetchMock as typeof fetch);
  stubGlobal(
    "EventSource",
    MockEventSource as unknown as typeof EventSource,
  );
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
      <DashboardSessionTestProvider sessionID={sessionID}>
        <App
          browserLanguage={browserLanguage}
          browserLanguages={browserLanguages}
          initialLocale={initialLocale}
          locationSearch={locationSearch}
        />
      </DashboardSessionTestProvider>
    </QueryClientProvider>,
  );

  return { ...result, fetchMock };
}

export async function renderAppWithDashboardShell(
  options: RenderAppOptions,
): Promise<RenderAppResult> {
  const result = renderApp(options);
  await waitForDashboardShell();
  return result;
}

export function mockCurrentFactoryDocument(
  result: CurrentFactoryDocumentResult,
): void {
  if (isBunTestRuntime) {
    const useCurrentFactoryDocumentMock = globalThis.__useCurrentFactoryDocumentMock;
    if (!useCurrentFactoryDocumentMock) {
      throw new Error(
        "Bun app-shell mocks are not loaded; ensure ui/testing/bun-test.preload.ts runs before tests.",
      );
    }
    useCurrentFactoryDocumentMock.mockReturnValue(result as never);
    return;
  }

  vi.mocked(useCurrentFactoryDocument).mockReturnValue(result as never);
}

export function registerAppDashboardTestLifecycle(): void {
  beforeEach(() => {
    window.localStorage.clear();
    MockEventSource.instances = [];
    restoreBrowserTestShims = installDashboardBrowserTestShims();
    resetSelectionHistoryStore();
    useDashboardSessionStore.setState({
      selectedSessionID: "~default",
    });
    mockCurrentFactoryDocument({
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
      refetch: vi.fn(() => Promise.resolve(undefined)),
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
    restoreGlobalStubs();
  });
}

export { DEFAULT_FACTORY_SESSION_ID };
