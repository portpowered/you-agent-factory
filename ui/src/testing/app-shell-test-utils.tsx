import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, type Mock, vi } from "vitest";
import { App } from "../App";
import type {
  DashboardSnapshot,
  DashboardTopology,
  DashboardTrace,
  DashboardWorkstationRequest,
} from "../api/dashboard";
import type { FactoryEvent } from "../api/events";
import type { FactorySessionSummary } from "../api/factory-sessions/api";
import { DEFAULT_FACTORY_SESSION_ID } from "../api/session-routing";
import {
  buildDashboardSnapshotFixture,
  mediumBranchingDashboardTopology,
} from "../components/dashboard/fixtures";
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
import type { FactoryPngImportValue } from "../features/import/lib/factory-png-import";
import { useFactoryTimelineStore } from "../features/timeline/state/factoryTimelineStore";
import {
  defaultFactorySessionSummary,
  fetchRequestPath,
  MockEventSource,
} from "./app-shell-session-stream-test-utils";
import { buildDashboardTestGraphLayout } from "./app-shell-test-graph-layout";
import {
  seedTimelineSnapshot,
  seedTimelineSnapshots,
} from "./app-shell-timeline-seed-utils";
import { DashboardSessionTestProvider } from "./dashboard-session-test-provider";

export {
  renderWithDashboardSessionTest,
  wrapWithDashboardSessionTest,
} from "./dashboard-session-test-utils";

vi.mock("../features/flowchart/lib/layout", async () => {
  const actual = await vi.importActual("../features/flowchart/lib/layout");

  return {
    ...actual,
    buildGraphLayout: async (topology: DashboardTopology) =>
      buildDashboardTestGraphLayout(topology),
  };
});

vi.mock("../features/current-factory-definition/public", async () => {
  const actual = await vi.importActual(
    "../features/current-factory-definition/public",
  );

  return {
    ...actual,
    useCurrentFactoryDocument: vi.fn(),
  };
});

export {
  fetchRequestPath,
  MockEventSource,
} from "./app-shell-session-stream-test-utils";

interface RenderAppOptions {
  browserLanguage?: string | null;
  browserLanguages?: readonly string[] | null;
  factorySessions?: FactorySessionSummary[];
  initialLocale?: string | null;
  locationSearch?: string | null;
  sessionID?: string | null;
  seedTimelineFromSnapshot?: boolean;
  snapshot: DashboardSnapshot;
  timelineEvents?: FactoryEvent[];
  timelineSnapshots?: DashboardSnapshot[];
  traceFixtures?: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

type FetchMock = Mock<
  (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
>;

interface RenderAppResult extends ReturnType<typeof render> {
  fetchMock: FetchMock;
}

type CurrentFactoryDocumentResult = ReturnType<
  typeof useCurrentFactoryDocument
>;

const terminalBaseSnapshot = semanticWorkflowDashboardSnapshot;
const queryClients: QueryClient[] = [];
let restoreBrowserTestShims: (() => void) | null = null;
export const baselineSnapshot = buildDashboardSnapshotFixture(
  mediumBranchingDashboardTopology,
);

export const activeSnapshot = semanticWorkflowDashboardSnapshot;

export const terminalSnapshot = {
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
          work_id: "work-failed-story",
          work_type_id: "story",
        },
      ],
      "story:complete": [
        {
          display_name: "Done Story",
          trace_id: "trace-done-story",
          work_id: "work-complete",
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
              work_id: "work-complete",
              work_type_id: "story",
            },
          ],
        },
      ],
    },
  },
} satisfies DashboardSnapshot;

export const importedFactorySnapshot = (() => {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);

  snapshot.factory_state = "Imported factory active";
  snapshot.tick_count = semanticWorkflowDashboardSnapshot.tick_count + 1;

  return snapshot;
})();

export async function waitForDashboardShell(): Promise<void> {
  await screen.findByRole("heading", { name: "U" });
}

export function renderApp({
  browserLanguage,
  browserLanguages,
  factorySessions,
  initialLocale,
  locationSearch,
  seedTimelineFromSnapshot = true,
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

  const fetchMock: FetchMock = vi
    .fn()
    .mockImplementation(async (input: RequestInfo | URL) => {
      const path = fetchRequestPath(input);

      if (path === "/factory-sessions") {
        return jsonResponse({
          sessions: factorySessions ?? [defaultFactorySessionSummary],
        });
      }

      throw new Error(`unexpected fetch for ${path}`);
    });

  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("EventSource", MockEventSource);
  reloadDashboardLayoutFromStorage();
  if (timelineEvents) {
    useFactoryTimelineStore.getState().replaceEvents(timelineEvents);
  } else if (timelineSnapshots) {
    seedTimelineSnapshots(timelineSnapshots);
  } else if (seedTimelineFromSnapshot) {
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

function fetchCallPaths(fetchMock: ReturnType<typeof vi.fn>) {
  return fetchMock.mock.calls.map(([input]) =>
    typeof input === "string"
      ? input
      : input instanceof URL
        ? `${input.pathname}${input.search}`
        : input.url,
  );
}

export function nonPromptTemplateFetchPaths(
  fetchMock: ReturnType<typeof vi.fn>,
) {
  return fetchCallPaths(fetchMock).filter(
    (path) =>
      !path.includes("/prompt-template-contract") &&
      path !== "/factory-sessions",
  );
}

export function jsonResponse(
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

export function createFactoryImportValue(): FactoryPngImportValue {
  return {
    factory: {
      name: "Dropped Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    },
    previewImageSrc: "blob:factory-preview",
    revokePreviewImageSrc: vi.fn(),
    schemaVersion: "portos.agent-factory.png.v1",
  };
}

export function createFileDropTransfer(files: File[]): {
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

export function mockCurrentFactoryDocument(
  result: CurrentFactoryDocumentResult,
): void {
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

export { DEFAULT_FACTORY_SESSION_ID };
