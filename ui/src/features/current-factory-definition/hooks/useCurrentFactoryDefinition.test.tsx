import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import {
  type CanonicalFactoryDefinition,
  type CurrentFactoryDocument,
  getCurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import { DashboardSessionStoreTestProvider } from "../../../testing/dashboard-session-test-provider";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import {
  useCurrentFactoryDefinition,
  useCurrentFactoryDocument,
} from "./useCurrentFactoryDefinition";

vi.mock("../../../api/current-factory-definition", async () => {
  const actual = await vi.importActual(
    "../../../api/current-factory-definition",
  );

  return {
    ...actual,
    getCurrentFactoryDocument: vi.fn(),
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
  vi.mocked(getCurrentFactoryDocument).mockReset();
  useDashboardSessionStore.setState({ selectedSessionID: "~default" });
});

describe("useCurrentFactoryDefinition", () => {
  it("does not fetch while workstation editing is disabled", () => {
    const { result } = renderHook(() => useCurrentFactoryDefinition(false), {
      wrapper: createQueryClientWrapper(),
    });

    expect(getCurrentFactoryDocument).not.toHaveBeenCalled();
    expect(result.current).toMatchObject({
      data: undefined,
      error: null,
      isFetching: false,
      isPending: true,
      status: "pending",
    });
  });

  it("returns the validated editable current factory definition on success", async () => {
    vi.mocked(getCurrentFactoryDocument).mockResolvedValue(
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
    vi.mocked(getCurrentFactoryDocument).mockResolvedValue(
      editableFactoryDefinition,
    );

    renderHook(() => useCurrentFactoryDefinition(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(getCurrentFactoryDocument).toHaveBeenCalledWith({
        sessionID: "session-2",
      });
    });
  });

  it("refetches when the selected session tab changes", async () => {
    vi.mocked(getCurrentFactoryDocument).mockResolvedValue(
      editableFactoryDefinition,
    );

    const { rerender } = renderHook(() => useCurrentFactoryDefinition(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(getCurrentFactoryDocument).toHaveBeenCalledWith({
        sessionID: "~default",
      });
    });

    vi.mocked(getCurrentFactoryDocument).mockClear();

    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID("session-2");
    });

    rerender();

    await waitFor(() => {
      expect(getCurrentFactoryDocument).toHaveBeenCalledWith({
        sessionID: "session-2",
      });
    });
  });

  it("exposes actionable typed errors when the current definition is not editable", async () => {
    vi.mocked(getCurrentFactoryDocument).mockRejectedValue({
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
        <DashboardSessionStoreTestProvider>
          {children}
        </DashboardSessionStoreTestProvider>
      </QueryClientProvider>
    );
  };
}
