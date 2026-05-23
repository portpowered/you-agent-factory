import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  getCurrentFactoryDefinition,
  getCurrentFactoryDocument,
  type CanonicalFactoryDefinition,
  type CurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import {
  useCurrentFactoryDefinition,
  useCurrentFactoryDocument,
} from "./useCurrentFactoryDefinition";

vi.mock("../../../api/current-factory-definition", async () => {
  const actual = await vi.importActual("../../../api/current-factory-definition");

  return {
    ...actual,
    getCurrentFactoryDefinition: vi.fn(),
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

describe("useCurrentFactoryDefinition", () => {
  beforeEach(() => {
    vi.mocked(getCurrentFactoryDefinition).mockReset();
    vi.mocked(getCurrentFactoryDocument).mockReset();
    useDashboardSessionStore.setState({ selectedSessionID: "~default" });
  });

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
    vi.mocked(getCurrentFactoryDefinition).mockResolvedValue(editableFactoryDefinition);

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
        logical: 4,
        physical: "2026-05-18T14:48:00Z",
      },
    };
    vi.mocked(getCurrentFactoryDocument).mockResolvedValue(
      editableFactoryDefinitionDocument,
    );

    const { result } = renderHook(
      () => useCurrentFactoryDocument(),
      {
        wrapper: createQueryClientWrapper(),
      },
    );

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
        logical: 5,
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

function createQueryClientWrapper(): ({ children }: { children: ReactNode }) => ReactNode {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: 0,
        retry: false,
      },
    },
  });

  return function QueryClientWrapper({ children }: { children: ReactNode }): ReactNode {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}
