import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PropsWithChildren } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { createReplayHarness } from "../../../testing/replay-harness";
import { useDashboardBentoStore } from "../../bento/state/dashboardBentoStore";
import {
  persistTimelineCheckpoint,
  useFactoryTimelineStore,
} from "../../timeline/public";
import { emptyReplayWorldState } from "../../timeline/state/timeline/replayWorldStateSupport";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../state/dashboardStreamStore";
import { DashboardScreen } from "./dashboard-screen";

const replayHarness = createReplayHarness();

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

vi.mock("../../bento/public", () => ({
  DashboardBento: () => <section>Dashboard bento</section>,
}));

vi.mock("../../header/public", () => ({
  DashboardExportDialog: () => <div>Dashboard export dialog</div>,
  DashboardHeader: () => <header>Dashboard header</header>,
  DashboardStatusPanel: ({
    detail,
    title,
  }: {
    detail?: string;
    title: string;
  }) => <StatusPanelProbe detail={detail} title={title} />,
}));

function indexedDBRequest<T>(result: T, beforeSuccess?: () => void) {
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
    transaction: () => ({
      objectStore: () => ({
        delete: (key: string) =>
          indexedDBRequest(undefined, () => {
            records.delete(key);
          }),
        get: (key: string) => indexedDBRequest(records.get(key)),
        put: (value: { sessionID: string }) =>
          indexedDBRequest(value.sessionID, () => {
            records.set(value.sessionID, value);
          }),
      }),
    }),
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

describe("DashboardScreen stale-cursor retry", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    replayHarness.install();
    installIndexedDBTestDouble();
    queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            factorySessionId: DEFAULT_FACTORY_SESSION_ID,
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
        ),
      ),
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
      streamState: createDefaultDashboardStreamState(),
    });
    useFactoryTimelineStore.getState().reset();
  });

  afterEach(() => {
    replayHarness.reset();
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
      streamState: createDefaultDashboardStreamState(),
    });
    useFactoryTimelineStore.getState().reset();
  });

  it("retries a recovery-failed session stream without replaying the stale checkpoint cursor", async () => {
    const user = userEvent.setup();

    await persistTimelineCheckpoint(window.indexedDB, DEFAULT_FACTORY_SESSION_ID, {
      afterEventId: "checkpoint-event-7",
      afterSequence: 7,
      replayState: emptyReplayWorldState(7),
      selectedTick: 7,
    });

    render(<DashboardScreen />, {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events?after_event_id=checkpoint-event-7&after_sequence=7`,
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
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
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
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );
  });
});
