import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  getCurrentEditableFactoryDefinition,
  type CanonicalFactoryDefinition,
} from "../../api/current-factory-definition";
import { useCurrentEditableFactoryDefinition } from "./useCurrentEditableFactoryDefinition";

vi.mock("../../api/current-factory-definition", async () => {
  const actual = await vi.importActual("../../api/current-factory-definition");

  return {
    ...actual,
    getCurrentEditableFactoryDefinition: vi.fn(),
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
