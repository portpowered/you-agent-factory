import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import {
  type CanonicalFactoryDefinition,
  CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
  getCurrentFactoryDefinition,
  getCurrentFactoryDocument,
  saveCurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import { sessionFactoryOperatorErrorMessages } from "../../../api/session-factory/operator-errors";
import { factoryRuntimeNotIdleTarget } from "../../../testing/factory-validation-target-fixtures";
import { syncCurrentFactoryDefinition } from "../../dashboard/lib/dashboard-event-stream";
import { DashboardSessionProvider } from "../../dashboard/session/dashboard-session-provider";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
  useCurrentFactoryDefinition,
  useCurrentFactoryDocument,
  useSaveCurrentFactory,
} from "./useCurrentFactoryDefinition";

vi.mock("../../../api/current-factory-definition", async () => {
  const actual = await vi.importActual(
    "../../../api/current-factory-definition",
  );

  return {
    ...actual,
    getCurrentFactoryDefinition: vi.fn(),
    getCurrentFactoryDocument: vi.fn(),
    saveCurrentFactoryDocument: vi.fn(),
  };
});

const editableFactoryDefinition: CanonicalFactoryDefinition = {
  name: "Current Factory",
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workstations: [
    {
      body: "Summarize before review.",
      inputs: [
        {
          state: "queued",
          workType: "task",
        },
      ],
      name: "Draft",
      outputs: [
        {
          state: "reviewed",
          workType: "task",
        },
      ],
      promptFile: "prompts/draft.md",
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
  workTypes: [],
};

beforeEach(() => {
  vi.mocked(getCurrentFactoryDefinition).mockReset();
  vi.mocked(getCurrentFactoryDocument).mockReset();
  vi.mocked(saveCurrentFactoryDocument).mockReset();
  useDashboardSessionStore.setState({ selectedSessionID: "~default" });
});

describe("useCurrentFactoryDefinition", () => {
  it("does not fetch while workstation editing is disabled", () => {
    const { result } = renderHook(() => useCurrentFactoryDefinition(false), {
      wrapper: createQueryClientWrapper(),
    });

    expect(getCurrentFactoryDefinition).not.toHaveBeenCalled();
    expect(result.current).toMatchObject({
      data: undefined,
      error: null,
      isFetching: false,
      isPending: true,
      status: "pending",
    });
  });

  it("returns the validated editable current factory definition on success", async () => {
    vi.mocked(getCurrentFactoryDefinition).mockResolvedValue(
      editableFactoryDefinition,
    );

    const { result } = renderHook(() => useCurrentFactoryDefinition(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        data: editableFactoryDefinition,
        error: null,
        isPending: false,
        status: "success",
      });
    });
  });

  it("loads the selected non-default session definition instead of the default alias", async () => {
    useDashboardSessionStore.setState({ selectedSessionID: "session-2" });
    vi.mocked(getCurrentFactoryDefinition).mockResolvedValue(
      editableFactoryDefinition,
    );

    renderHook(() => useCurrentFactoryDefinition(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(getCurrentFactoryDefinition).toHaveBeenCalledWith({
        sessionID: "session-2",
      });
    });
  });

  it("refetches when the selected session tab changes", async () => {
    vi.mocked(getCurrentFactoryDefinition).mockResolvedValue(
      editableFactoryDefinition,
    );

    const { rerender } = renderHook(() => useCurrentFactoryDefinition(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(getCurrentFactoryDefinition).toHaveBeenCalledWith({
        sessionID: "~default",
      });
    });

    vi.mocked(getCurrentFactoryDefinition).mockClear();

    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID("session-2");
    });

    rerender();

    await waitFor(() => {
      expect(getCurrentFactoryDefinition).toHaveBeenCalledWith({
        sessionID: "session-2",
      });
    });
  });

  it("exposes actionable typed errors when the current definition is not editable", async () => {
    vi.mocked(getCurrentFactoryDefinition).mockRejectedValue({
      code: "INVALID_FACTORY_DEFINITION",
      message: "The current factory definition is malformed.",
      name: "CurrentFactoryDefinitionError",
    });

    const { result } = renderHook(() => useCurrentFactoryDefinition(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        data: undefined,
        error: {
          code: "INVALID_FACTORY_DEFINITION",
          message: "The current factory definition is malformed.",
        },
        isPending: false,
        status: "error",
      });
    });
  });

  it("returns the editable current-factory document with version metadata", async () => {
    const editableFactoryDefinitionDocument: CurrentFactoryDocument = {
      ...editableFactoryDefinition,
      version: {
        logical: "4",
        physical: "2026-05-18T14:48:00Z",
      },
    };
    vi.mocked(getCurrentFactoryDocument).mockResolvedValue(
      editableFactoryDefinitionDocument,
    );

    const { result } = renderHook(() => useCurrentFactoryDocument(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        data: editableFactoryDefinitionDocument,
        error: null,
        isPending: false,
        status: "success",
      });
    });
  });

  it("loads the editable document for the selected non-default session", async () => {
    useDashboardSessionStore.setState({ selectedSessionID: "session-2" });
    vi.mocked(getCurrentFactoryDocument).mockResolvedValue({
      ...editableFactoryDefinition,
      version: {
        logical: "5",
        physical: "2026-05-18T14:49:00Z",
      },
    });

    renderHook(() => useCurrentFactoryDocument(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(getCurrentFactoryDocument).toHaveBeenCalledWith({
        sessionID: "session-2",
      });
    });
  });
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: save/convergence cases share one query-client harness.
describe("useSaveCurrentFactory", () => {
  it("saves the editable current-factory document and refreshes both query caches", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          gcTime: 0,
          retry: false,
        },
      },
    });
    const savedDocument: CurrentFactoryDocument = {
      ...editableFactoryDefinition,
      metadata: {
        owner: "graph-editor",
      },
      version: {
        logical: "8",
        physical: "2026-05-27T08:00:00Z",
      },
    };
    vi.mocked(saveCurrentFactoryDocument).mockResolvedValue(savedDocument);
    useDashboardSessionStore.setState({ selectedSessionID: "session-2" });

    const { result } = renderHook(
      () => ({
        definitionKey: currentFactoryDefinitionQueryKey("session-2"),
        documentKey: currentFactoryDocumentQueryKey("session-2"),
        save: useSaveCurrentFactory(),
      }),
      {
        wrapper: createQueryClientWrapper(queryClient),
      },
    );

    await result.current.save.mutateAsync({
      baseVersion: {
        logical: "7",
        physical: "2026-05-27T07:59:00Z",
      },
      factoryDefinition: editableFactoryDefinition,
    });

    expect(saveCurrentFactoryDocument).toHaveBeenCalledWith(
      {
        baseVersion: {
          logical: "7",
          physical: "2026-05-27T07:59:00Z",
        },
        factoryDefinition: editableFactoryDefinition,
      },
      {
        sessionID: "session-2",
      },
    );
    expect(queryClient.getQueryData(result.current.documentKey)).toEqual(
      savedDocument,
    );
    expect(queryClient.getQueryData(result.current.definitionKey)).toEqual(
      savedDocument,
    );
  });

  it("does not refetch the document query after a successful save", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          gcTime: 0,
          retry: false,
        },
      },
    });
    const savedDocument: CurrentFactoryDocument = {
      ...editableFactoryDefinition,
      version: {
        logical: "8",
        physical: "2026-05-27T08:00:00Z",
      },
    };
    vi.mocked(saveCurrentFactoryDocument).mockResolvedValue(savedDocument);
    useDashboardSessionStore.setState({ selectedSessionID: "session-2" });

    const { result } = renderHook(() => useSaveCurrentFactory(), {
      wrapper: createQueryClientWrapper(queryClient),
    });

    await result.current.mutateAsync({
      baseVersion: {
        logical: "7",
        physical: "2026-05-27T07:59:00Z",
      },
      factoryDefinition: editableFactoryDefinition,
    });

    expect(getCurrentFactoryDocument).not.toHaveBeenCalled();
  });

  it("converges document cache on FACTORY_CHANGE with version without a document GET", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          gcTime: 0,
          retry: false,
        },
      },
    });
    const savedDocument: CurrentFactoryDocument = {
      ...editableFactoryDefinition,
      version: {
        logical: "8",
        physical: "2026-05-27T08:00:00Z",
      },
    };
    vi.mocked(saveCurrentFactoryDocument).mockResolvedValue(savedDocument);
    useDashboardSessionStore.setState({ selectedSessionID: "session-2" });

    const { result } = renderHook(
      () => ({
        documentKey: currentFactoryDocumentQueryKey("session-2"),
        save: useSaveCurrentFactory(),
      }),
      {
        wrapper: createQueryClientWrapper(queryClient),
      },
    );

    await result.current.save.mutateAsync({
      baseVersion: {
        logical: "7",
        physical: "2026-05-27T07:59:00Z",
      },
      factoryDefinition: editableFactoryDefinition,
    });

    expect(getCurrentFactoryDocument).not.toHaveBeenCalled();

    syncCurrentFactoryDefinition(
      queryClient,
      {
        context: { eventTime: "2026-05-27T08:00:01Z", sequence: 9, tick: 9 },
        id: "factory-event/factory-change/9",
        payload: {
          factory: {
            ...editableFactoryDefinition,
            version: {
              logical: "9",
              physical: "2026-05-27T08:00:01Z",
            },
          },
        },
        type: FACTORY_EVENT_TYPES.factoryChange,
      },
      "session-2",
    );

    expect(getCurrentFactoryDocument).not.toHaveBeenCalled();
    expect(queryClient.getQueryData(result.current.documentKey)).toMatchObject({
      version: {
        logical: "9",
        physical: "2026-05-27T08:00:01Z",
      },
    });
  });

  it("preserves current-factory save API errors through the mutation", async () => {
    const error = new CurrentFactoryDefinitionError(
      sessionFactoryOperatorErrorMessages.STALE_FACTORY_VERSION,
      {
        code: "STALE_FACTORY_VERSION",
        status: 409,
      },
    );
    vi.mocked(saveCurrentFactoryDocument).mockRejectedValue(error);

    const { result } = renderHook(() => useSaveCurrentFactory(), {
      wrapper: createQueryClientWrapper(),
    });

    await expect(
      result.current.mutateAsync({
        baseVersion: {
          logical: "7",
          physical: "2026-05-27T07:59:00Z",
        },
        factoryDefinition: editableFactoryDefinition,
      }),
    ).rejects.toEqual(error);
  });

  it("preserves not-idle operator-facing save failures through the mutation", async () => {
    const error = new CurrentFactoryDefinitionError(
      sessionFactoryOperatorErrorMessages.FACTORY_NOT_IDLE,
      {
        code: "FACTORY_NOT_IDLE",
        status: 409,
        targets: [factoryRuntimeNotIdleTarget()],
      },
    );
    vi.mocked(saveCurrentFactoryDocument).mockRejectedValue(error);

    const { result } = renderHook(() => useSaveCurrentFactory(), {
      wrapper: createQueryClientWrapper(),
    });

    await expect(
      result.current.mutateAsync({
        baseVersion: {
          logical: "7",
          physical: "2026-05-27T07:59:00Z",
        },
        factoryDefinition: editableFactoryDefinition,
      }),
    ).rejects.toMatchObject({
      code: "FACTORY_NOT_IDLE",
      message: sessionFactoryOperatorErrorMessages.FACTORY_NOT_IDLE,
    });
  });
});

function createQueryClientWrapper(
  queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: 0,
        retry: false,
      },
    },
  }),
): ({ children }: { children: ReactNode }) => ReactNode {
  return function QueryClientWrapper({
    children,
  }: {
    children: ReactNode;
  }): ReactNode {
    return (
      <QueryClientProvider client={queryClient}>
        <DashboardSessionProvider>{children}</DashboardSessionProvider>
      </QueryClientProvider>
    );
  };
}
