import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  getCurrentEditableFactoryDefinition,
  getCurrentEditableFactoryDefinitionDocument,
  type CanonicalFactoryDefinition,
  type EditableFactoryDefinitionDocument,
} from "../../../api/current-factory-definition";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import {
  useCurrentEditableFactoryDefinition,
  useCurrentEditableFactoryDefinitionDocument,
} from "./useCurrentEditableFactoryDefinition";

vi.mock("../../../api/current-factory-definition", async () => {
  const actual = await vi.importActual("../../../api/current-factory-definition");

  return {
    ...actual,
    getCurrentEditableFactoryDefinition: vi.fn(),
    getCurrentEditableFactoryDefinitionDocument: vi.fn(),
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

describe("useCurrentEditableFactoryDefinition", () => {
  beforeEach(() => {
    vi.mocked(getCurrentEditableFactoryDefinition).mockReset();
    vi.mocked(getCurrentEditableFactoryDefinitionDocument).mockReset();
    useDashboardSessionStore.setState({ selectedSessionID: "~default" });
  });

  it("does not fetch while workstation editing is disabled", () => {
    const { result } = renderHook(() => useCurrentEditableFactoryDefinition(false), {
      wrapper: createQueryClientWrapper(),
    });

    expect(getCurrentEditableFactoryDefinition).not.toHaveBeenCalled();
    expect(result.current).toMatchObject({
      data: undefined,
      error: null,
      isFetching: false,
      isPending: true,
      status: "pending",
    });
  });

  it("returns the validated editable current factory definition on success", async () => {
    vi.mocked(getCurrentEditableFactoryDefinition).mockResolvedValue(editableFactoryDefinition);

    const { result } = renderHook(() => useCurrentEditableFactoryDefinition(), {
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
    vi.mocked(getCurrentEditableFactoryDefinition).mockResolvedValue(
      editableFactoryDefinition,
    );

    renderHook(() => useCurrentEditableFactoryDefinition(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(getCurrentEditableFactoryDefinition).toHaveBeenCalledWith({
        sessionID: "session-2",
      });
    });
  });

  it("exposes actionable typed errors when the current definition is not editable", async () => {
    vi.mocked(getCurrentEditableFactoryDefinition).mockRejectedValue({
      code: "INVALID_FACTORY_DEFINITION",
      message: "The current factory definition is malformed.",
      name: "CurrentEditableFactoryDefinitionError",
    });

    const { result } = renderHook(() => useCurrentEditableFactoryDefinition(), {
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
    const editableFactoryDefinitionDocument: EditableFactoryDefinitionDocument =
      {
        factoryDefinition: editableFactoryDefinition,
        version: {
          logical: 4,
          physical: "2026-05-18T14:48:00Z",
        },
      };
    vi.mocked(getCurrentEditableFactoryDefinitionDocument).mockResolvedValue(
      editableFactoryDefinitionDocument,
    );

    const { result } = renderHook(
      () => useCurrentEditableFactoryDefinitionDocument(),
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
    vi.mocked(getCurrentEditableFactoryDefinitionDocument).mockResolvedValue({
      factoryDefinition: editableFactoryDefinition,
      version: {
        logical: 5,
        physical: "2026-05-18T14:49:00Z",
      },
    });

    renderHook(() => useCurrentEditableFactoryDefinitionDocument(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(getCurrentEditableFactoryDefinitionDocument).toHaveBeenCalledWith({
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
