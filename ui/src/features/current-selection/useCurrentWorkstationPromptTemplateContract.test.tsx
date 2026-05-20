import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  getCurrentFactoryWorkstationPromptTemplateContract,
  type PromptTemplateContract,
} from "../../api/current-factory-prompt-template";
import {
  buildCurrentWorkstationPromptTemplateContractQueryKey,
  useCurrentWorkstationPromptTemplateContract,
} from "./useCurrentWorkstationPromptTemplateContract";

vi.mock("../../api/current-factory-prompt-template", async () => {
  const actual = await vi.importActual(
    "../../api/current-factory-prompt-template",
  );

  return {
    ...actual,
    getCurrentFactoryWorkstationPromptTemplateContract: vi.fn(),
  };
});

const promptTemplateContract: PromptTemplateContract = {
  availableVariables: [
    {
      category: "ROOT",
      description: "Current workstation prompt context.",
      example: "{{ .Prompt }}",
      path: ".Prompt",
    },
  ],
  inputCount: 1,
  unavailableAccessPatterns: [],
};

describe("useCurrentWorkstationPromptTemplateContract", () => {
  beforeEach(() => {
    vi.mocked(getCurrentFactoryWorkstationPromptTemplateContract).mockReset();
  });

  it("builds a stable query key for the selected workstation", () => {
    expect(
      buildCurrentWorkstationPromptTemplateContractQueryKey("Review"),
    ).toEqual(["current-workstation-prompt-template-contract", "Review"]);
  });

  it("does not fetch while prompt help is disabled", () => {
    const { result } = renderHook(
      () => useCurrentWorkstationPromptTemplateContract("Review", false),
      { wrapper: createQueryClientWrapper() },
    );

    expect(getCurrentFactoryWorkstationPromptTemplateContract).not.toHaveBeenCalled();
    expect(result.current).toMatchObject({
      data: undefined,
      error: null,
      isFetching: false,
      isPending: true,
      status: "pending",
    });
  });

  it("rejects manual refetches without a selected workstation", async () => {
    const { result } = renderHook(
      () => useCurrentWorkstationPromptTemplateContract(undefined),
      { wrapper: createQueryClientWrapper() },
    );

    await expect(result.current.refetch()).resolves.toMatchObject({
      error: new Error("workstationName is required"),
      status: "error",
    });
  });

  it("loads the prompt-template contract for the selected workstation", async () => {
    vi.mocked(getCurrentFactoryWorkstationPromptTemplateContract).mockResolvedValue(
      promptTemplateContract,
    );

    const { result } = renderHook(
      () => useCurrentWorkstationPromptTemplateContract("Review"),
      { wrapper: createQueryClientWrapper() },
    );

    await waitFor(() => {
      expect(result.current).toMatchObject({
        data: promptTemplateContract,
        error: null,
        isPending: false,
        status: "success",
      });
    });

    expect(getCurrentFactoryWorkstationPromptTemplateContract).toHaveBeenCalledWith(
      "Review",
    );
  });

  it("surfaces typed API failures from the prompt-template contract query", async () => {
    vi.mocked(getCurrentFactoryWorkstationPromptTemplateContract).mockRejectedValue(
      {
        code: "NOT_FOUND",
        message: "Current factory workstation not found.",
        name: "CurrentFactoryPromptTemplateAPIError",
      },
    );

    const { result } = renderHook(
      () => useCurrentWorkstationPromptTemplateContract("Review"),
      { wrapper: createQueryClientWrapper() },
    );

    await waitFor(() => {
      expect(result.current).toMatchObject({
        data: undefined,
        error: {
          code: "NOT_FOUND",
          message: "Current factory workstation not found.",
        },
        isPending: false,
        status: "error",
      });
    });
  });
});

function createQueryClientWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: 0,
        retry: false,
      },
    },
  });

  return function QueryClientWrapper({
    children,
  }: {
    children: ReactNode;
  }): ReactNode {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}
