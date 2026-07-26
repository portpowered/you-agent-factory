import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, vi } from "vitest";
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
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
  useCurrentFactoryDocument,
} from "../features/current-factory-definition/hooks/useCurrentFactoryDefinition";
import { resetSelectionHistoryStore } from "../features/current-selection/base/public";
import { useDashboardSessionStore } from "../features/dashboard/state/dashboardSessionStore";
import { useDashboardStreamStore } from "../features/dashboard/state/dashboardStreamStore";
import { useExportDialogStore } from "../features/export/state/exportDialogStore";
import type { FactoryPngImportValue } from "../features/import/lib/factory-png-import";
import { useFactoryTimelineStore } from "../features/timeline/state/factoryTimelineStore";
import {
  chainRenderAppFetchMock,
  type FetchMock,
  type RenderAppFetchOverride,
} from "./app-shell-fetch-test-utils";
import {
  buildAppShellStreamIdentity,
  handleFactorySessionPreflightRequest,
} from "./app-shell-session-preflight-test-utils";
import {
  defaultFactorySessionSummary,
  fetchRequestPath,
  MockEventSource,
} from "./app-shell-session-stream-test-utils";
import { buildDashboardTestGraphLayout } from "./app-shell-test-graph-layout";
import { AppShellSeededApp } from "./app-shell-timeline-seeder";
import {
  DashboardSessionStoreTestProvider,
  DashboardSessionTestProvider,
} from "./dashboard-session-test-provider";
import {
  isSessionFactoryRequest,
  mockGetSessionFactory,
  sessionFactoryDocumentFromSnapshot,
} from "./session-factory-mocks";

export {
  chainRenderAppFetchMock,
  type FetchMock,
  fetchCallPaths,
  jsonResponse,
  lastFetchCallBody,
  nonPromptTemplateFetchPaths,
  type RenderAppFetchOverride,
} from "./app-shell-fetch-test-utils";

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

const currentFactoryDocumentMock = vi.hoisted(() => ({
  actual: null as typeof useCurrentFactoryDocument | null,
}));

vi.mock(
  "../features/current-factory-definition/hooks/useCurrentFactoryDefinition",
  async (importOriginal) => {
    const actual =
      await importOriginal<
        typeof import("../features/current-factory-definition/hooks/useCurrentFactoryDefinition")
      >();
    currentFactoryDocumentMock.actual = actual.useCurrentFactoryDocument;

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(actual.useCurrentFactoryDocument),
    };
  },
);

export {
  fetchRequestPath,
  MockEventSource,
} from "./app-shell-session-stream-test-utils";

export interface RenderAppOptions {
  app: ReactNode;
  browserLanguage?: string | null;
  browserLanguages?: readonly string[] | null;
  factorySessions?: FactorySessionSummary[];
  fetchOverride?: RenderAppFetchOverride;
  initialLocale?: string | null;
  locationSearch?: string | null;
  sessionID?: string | null;
  seedTimelineFromSnapshot?: boolean;
  seedCurrentFactoryDocument?: boolean;
  snapshot: DashboardSnapshot;
  timelineEvents?: FactoryEvent[];
  timelineSnapshots?: DashboardSnapshot[];
  traceFixtures?: Record<string, DashboardTrace>;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

export interface RenderAppResult extends ReturnType<typeof render> {
  fetchMock: FetchMock;
}

function resolveTimelineSessionSummary(
  sessions: readonly FactorySessionSummary[],
  controlledSessionID: string | null,
): FactorySessionSummary {
  const summary =
    sessions.find((session) => session.id === controlledSessionID) ??
    sessions[0];
  if (!summary) {
    throw new Error(
      "expected at least one factory session summary for timeline seeding",
    );
  }
  return summary;
}

function wrapAppForDashboardSession(
  app: ReactNode,
  sessionID: string | null | undefined,
  controlledSessionID: string | null,
): ReactNode {
  return sessionID === undefined ? (
    <DashboardSessionStoreTestProvider
      resolvedDefaultSessionID={controlledSessionID ?? undefined}
    >
      {app}
    </DashboardSessionStoreTestProvider>
  ) : (
    <DashboardSessionTestProvider sessionID={controlledSessionID}>
      {app}
    </DashboardSessionTestProvider>
  );
}

const queryClients: QueryClient[] = [];
let restoreBrowserTestShims: (() => void) | null = null;
export const baselineSnapshot = buildDashboardSnapshotFixture(
  mediumBranchingDashboardTopology,
);
export const terminalSnapshot = {
  ...semanticWorkflowDashboardSnapshot,
  tick_count: 4,
  runtime: {
    ...semanticWorkflowDashboardSnapshot.runtime,
    place_occupancy_work_items_by_place_id: {
      ...(semanticWorkflowDashboardSnapshot.runtime
        .place_occupancy_work_items_by_place_id ?? {}),
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
      ...(semanticWorkflowDashboardSnapshot.runtime.place_token_counts ?? {}),
      "story:blocked": 1,
      "story:complete": 1,
    },
    session: {
      ...semanticWorkflowDashboardSnapshot.runtime.session,
      completed_count: 1,
      completed_work_labels: ["Done Story"],
      provider_sessions: [
        ...(semanticWorkflowDashboardSnapshot.runtime.session
          .provider_sessions ?? []),
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

export async function settleAppShellDashboardEffects(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
  });
}

export async function waitForAppShellWorkGraphReady(): Promise<void> {
  const graphViewport = await screen.findByRole("region", {
    name: "Work graph viewport",
  });
  await waitFor(
    () => {
      expect(graphViewport.querySelector(".react-flow__node")).toBeTruthy();
    },
    { timeout: 15_000 },
  );
  await settleAppShellDashboardEffects();
}

export async function waitForDashboardShell(): Promise<void> {
  await screen.findByRole("heading", { name: "U" });
  await settleAppShellDashboardEffects();
}

export function renderApp({
  app: renderedApp,
  browserLanguage,
  browserLanguages,
  factorySessions,
  fetchOverride,
  initialLocale,
  locationSearch,
  seedTimelineFromSnapshot = true,
  seedCurrentFactoryDocument = true,
  sessionID,
  snapshot,
  timelineEvents,
  timelineSnapshots,
  traceFixtures = {},
  workstationRequestsByDispatchID = {},
}: RenderAppOptions): RenderAppResult {
  const availableFactorySessions = factorySessions ?? [
    defaultFactorySessionSummary,
  ];
  const controlledSessionID =
    sessionID === undefined
      ? (availableFactorySessions.find((session) => session.isDefault)?.id ??
        DEFAULT_FACTORY_SESSION_ID)
      : sessionID;
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
  queryClients.push(queryClient);
  if (seedCurrentFactoryDocument) {
    const resolvedSessionID = controlledSessionID ?? DEFAULT_FACTORY_SESSION_ID;
    const sessionSummary =
      availableFactorySessions.find(
        (session) => session.id === resolvedSessionID,
      ) ?? availableFactorySessions[0];
    if (!sessionSummary) {
      throw new Error(
        "expected at least one factory session summary for app-shell seeding",
      );
    }
    const currentFactoryDocument = sessionFactoryDocumentFromSnapshot(snapshot);
    const streamIdentity = buildAppShellStreamIdentity(
      sessionSummary,
      snapshot,
    );

    queryClient.setQueryData(
      currentFactoryDocumentQueryKey(resolvedSessionID, streamIdentity),
      currentFactoryDocument,
    );
    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(resolvedSessionID, streamIdentity),
      currentFactoryDocument,
    );
    queryClient.setQueryData(
      currentFactoryDocumentQueryKey(resolvedSessionID),
      currentFactoryDocument,
    );
    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(resolvedSessionID),
      currentFactoryDocument,
    );
  }

  const fetchMock: FetchMock = vi
    .fn()
    .mockImplementation(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = fetchRequestPath(input);
        const method = (init?.method ?? "GET").toUpperCase();
        const preflightResponse = handleFactorySessionPreflightRequest({
          availableFactorySessions,
          method,
          path,
          snapshot,
        });
        if (preflightResponse) {
          return preflightResponse;
        }

        if (
          method === "GET" &&
          isSessionFactoryRequest(path, method, sessionID ?? undefined)
        ) {
          return mockGetSessionFactory({
            document: sessionFactoryDocumentFromSnapshot(snapshot),
          });
        }

        throw new Error(`unexpected fetch for ${path}`);
      },
    );

  if (fetchOverride) {
    chainRenderAppFetchMock(fetchMock, fetchOverride);
  }

  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("EventSource", MockEventSource);
  reloadDashboardLayoutFromStorage();
  const sessionSummary = resolveTimelineSessionSummary(
    availableFactorySessions,
    controlledSessionID,
  );
  const app = (
    <AppShellSeededApp
      browserLanguage={browserLanguage}
      browserLanguages={browserLanguages}
      identity={buildAppShellStreamIdentity(sessionSummary, snapshot)}
      initialLocale={initialLocale}
      locationSearch={locationSearch}
      seedTimelineFromSnapshot={seedTimelineFromSnapshot}
      snapshot={snapshot}
      timelineEvents={timelineEvents}
      timelineSnapshots={timelineSnapshots}
      traceFixtures={traceFixtures}
      workstationRequestsByDispatchID={workstationRequestsByDispatchID}
    >
      {renderedApp}
    </AppShellSeededApp>
  );
  const scopedApp = wrapAppForDashboardSession(
    app,
    sessionID,
    controlledSessionID,
  );
  const result = render(
    <QueryClientProvider client={queryClient}>{scopedApp}</QueryClientProvider>,
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

export function resetCurrentFactoryDocumentMock(): void {
  if (currentFactoryDocumentMock.actual == null) {
    throw new Error("useCurrentFactoryDocument mock was not initialized");
  }

  vi.mocked(useCurrentFactoryDocument).mockImplementation(
    currentFactoryDocumentMock.actual,
  );
}

export function registerAppDashboardTestLifecycle(): void {
  beforeEach(() => {
    MockEventSource.instances = [];
    restoreBrowserTestShims = installDashboardBrowserTestShims();
    window.localStorage.clear();
    resetSelectionHistoryStore();
    useDashboardSessionStore.setState({
      selectedSessionID: "~default",
    });
    useDashboardStreamStore.getState().resetStreamState();
    resetCurrentFactoryDocumentMock();
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
    useDashboardStreamStore.getState().resetStreamState();
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
