import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PropsWithChildren } from "react";

import * as factorySessionsAPI from "../../../api/factory-sessions";
import { FactorySessionSyncPreflightReasonCode } from "../../../api/generated/openapi";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { createReplayHarness } from "../../../testing/replay-harness";
import { useDashboardBentoStore } from "../../bento/state/dashboardBentoStore";
import {
  persistTimelineCheckpoint,
} from "../../timeline/public/checkpoint-persistence";
import { useFactoryTimelineStore } from "../../timeline/public/store";
import { emptyReplayWorldState } from "../../timeline/state/timeline/replayWorldStateSupport";
import { createMaterializedWorkOutcomeState } from "../../work-outcome/public/materializer";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../state/dashboardStreamStore";
import { DashboardScreen } from "./dashboard-screen";

const replayHarness = createReplayHarness();
const RESOLVED_DEFAULT_SESSION_UUID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";

function resolvedDefaultStreamIdentity() {
  return {
    backendScopeID: "backend-scope-a",
    factorySessionID: RESOLVED_DEFAULT_SESSION_UUID,
    logicalSessionKeyID: "logical-default",
    streamGenerationID: "2026-06-26T00:00:00Z",
  };
}

function StatusPanelProbe({
  detail,
  title,
}: {
  detail?: string;
  title: string;
}) {
  return (
    <section>
      <h1>{title}</h1>
      {detail ? <p>{detail}</p> : null}
    </section>
  );
}

vi.mock("../../bento/components/dashboard-bento", () => ({
  DashboardBento: () => <section>Dashboard bento</section>,
}));

vi.mock("../../header/components/dashboard-export-dialog", () => ({
  DashboardExportDialog: () => <div>Dashboard export dialog</div>,
}));

vi.mock("../../header/components/dashboard-header", () => ({
  DashboardHeader: () => <header>Dashboard header</header>,
}));

vi.mock("../../header/components/dashboard-status-panel", () => ({
  DashboardStatusPanel: ({
    detail,
    title,
  }: {
    detail?: string;
    title: string;
  }) => <StatusPanelProbe detail={detail} title={title} />,
}));

function indexedDBRequest<T>(
  result: T,
  beforeSuccess?: () => void,
  afterSuccess?: () => void,
) {
  const request = {
    error: null,
    onblocked: null,
    onerror: null,
    onsuccess: null,
    onupgradeneeded: null,
    result,
  } as unknown as IDBRequest<T> & {
    onblocked?: ((event: Event) => void) | null;
    onupgradeneeded?: ((event: IDBVersionChangeEvent) => void) | null;
  };

  window.setTimeout(() => {
    beforeSuccess?.();
    request.onsuccess?.({} as Event);
    afterSuccess?.();
  }, 0);

  return request;
}

function installIndexedDBTestDouble() {
  const records = new Map<string, unknown>();
  const database = {
    close: () => {},
    createObjectStore: () => undefined,
    objectStoreNames: {
      contains: () => true,
    },
    transaction: () => {
      const transaction = {
        oncomplete: null,
        objectStore: () => ({
          delete: (key: string) =>
            indexedDBRequest(
              undefined,
              () => {
                records.delete(key);
              },
              () =>
                (transaction.oncomplete as ((event: Event) => void) | null)?.(
                  {} as Event,
                ),
            ),
          get: (key: string) => indexedDBRequest(records.get(key)),
          getAll: () => indexedDBRequest([...records.values()]),
          put: (value: { sessionID: string; storageKey?: string }) =>
            indexedDBRequest(
              value.storageKey ?? value.sessionID,
              () => {
                records.set(value.storageKey ?? value.sessionID, value);
              },
              () =>
                (transaction.oncomplete as ((event: Event) => void) | null)?.(
                  {} as Event,
                ),
            ),
        }),
      };
      return transaction;
    },
  };
  const indexedDB = {
    open: () => {
      const request = indexedDBRequest(database);
      window.setTimeout(
        () => request.onupgradeneeded?.({} as IDBVersionChangeEvent),
        0,
      );
      return request;
    },
  };

  Object.defineProperty(window, "indexedDB", {
    configurable: true,
    value: indexedDB,
  });
}

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: the browser-visible stale-cursor retry flow shares one integration-style harness.
describe("DashboardScreen stale-cursor retry", () => {
  let queryClient: QueryClient;
  let getFactorySessionSyncPreflightSpy: ReturnType<typeof vi.spyOn>;
  let listFactorySessionsSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    replayHarness.install();
    installIndexedDBTestDouble();
    queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    getFactorySessionSyncPreflightSpy = vi
      .spyOn(factorySessionsAPI, "getFactorySessionSyncPreflight")
      .mockResolvedValue({
        backendScopeId: "backend-scope-a",
        checkpointReusable: true,
        factorySessionId: RESOLVED_DEFAULT_SESSION_UUID,
        logicalSessionKeyId: "logical-default",
        reasonCode: FactorySessionSyncPreflightReasonCode.ok,
        reconnectCursor: {
          afterEventId: "checkpoint-event-7",
          afterSequence: 7,
          provided: true,
          validForStreamGeneration: true,
        },
        requestedSessionId: RESOLVED_DEFAULT_SESSION_UUID,
        streamGenerationId: "2026-06-26T00:00:00Z",
      });
    listFactorySessionsSpy = vi
      .spyOn(factorySessionsAPI, "listFactorySessions")
      .mockResolvedValue([
        {
          factoryDir: "/workspace/root",
          folderPath: "/workspace/root",
          id: RESOLVED_DEFAULT_SESSION_UUID,
          isDefault: true,
          project: "root",
          target: { kind: "default" },
        },
      ]);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (
          url.includes(
            `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events`,
          )
        ) {
          return new Response(
            JSON.stringify({
              factorySessionId: RESOLVED_DEFAULT_SESSION_UUID,
              outcome: "CURSOR_STALE",
              retry: {
                omitAfterEventId: true,
                omitAfterSequence: true,
              },
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 200,
            },
          );
        }

        throw new Error(`unexpected fetch for ${url}`);
      }),
    );
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
      sessionTabOrder: [],
    });
    useDashboardStreamStore.setState({
      resolvedStreamIdentity: null,
      streamState: createDefaultDashboardStreamState(),
    });
    useFactoryTimelineStore.getState().reset();
  });

  afterEach(() => {
    replayHarness.reset();
    getFactorySessionSyncPreflightSpy.mockRestore();
    listFactorySessionsSpy.mockRestore();
    vi.unstubAllGlobals();
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
      sessionTabOrder: [],
    });
    useDashboardStreamStore.setState({
      resolvedStreamIdentity: null,
      streamState: createDefaultDashboardStreamState(),
    });
    useFactoryTimelineStore.getState().reset();
  });

  it("retries a recovery-failed session stream without replaying the stale checkpoint cursor", async () => {
    const user = userEvent.setup();

    await persistTimelineCheckpoint(
      window.indexedDB,
      {
        afterEventId: "checkpoint-event-7",
        afterSequence: 7,
        materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
      },
      resolvedDefaultStreamIdentity(),
    );

    render(<DashboardScreen />, {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events?after_event_id=checkpoint-event-7&after_sequence=7`,
    );

    const initialStream = replayHarness.getStreams()[0];
    if (!initialStream) {
      throw new Error("expected initial stale-cursor stream to open");
    }

    act(() => {
      initialStream.onerror?.(new Event("error"));
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(2);
    });
    expect(replayHarness.getStreams()[1]?.url).toBe(
      `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events`,
    );

    const replayStream = replayHarness.getStreams()[1];
    if (!replayStream) {
      throw new Error("expected cursor-free replay stream to open");
    }

    act(() => {
      replayStream.onerror?.(new Event("error"));
    });

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Retry session stream" }),
      ).toBeTruthy();
    });

    await user.click(
      screen.getByRole("button", { name: "Retry session stream" }),
    );

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(3);
    });
    expect(replayHarness.getStreams()[2]?.url).toBe(
      `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events`,
    );
  });
});
