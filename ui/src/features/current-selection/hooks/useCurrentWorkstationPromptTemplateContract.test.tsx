import "../../../../testing/bun-current-factory-prompt-template-api-mocks";
import { beforeEach, describe, expect, it } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { type PromptTemplateContract } from "../../../api/current-factory-prompt-template";
import { getCurrentFactoryWorkstationPromptTemplateContractMock } from "../../../../testing/bun-current-factory-prompt-template-api-mocks";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../../dashboard/state/dashboardSessionStore";
import {
  buildCurrentWorkstationPromptTemplateContractQueryKey,
  useCurrentWorkstationPromptTemplateContract,
} from "./useCurrentWorkstationPromptTemplateContract";

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
    resetDashboardSessionStore();
    getCurrentFactoryWorkstationPromptTemplateContractMock.mockReset();
  });

  it("builds a stable query key for the selected workstation", () => {
    expect(
      buildCurrentWorkstationPromptTemplateContractQueryKey("Review", null),
    ).toEqual([
      "current-workstation-prompt-template-contract",
      "~default",
      "Review",
    ]);
  });

  it("does not fetch while prompt help is disabled", () => {
    const { result } = renderHook(
      () => useCurrentWorkstationPromptTemplateContract("Review", false),
      { wrapper: createQueryClientWrapper() },
    );

    expect(
      getCurrentFactoryWorkstationPromptTemplateContractMock,
    ).not.toHaveBeenCalled();
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
    getCurrentFactoryWorkstationPromptTemplateContractMock.mockResolvedValue(
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

    expect(
      getCurrentFactoryWorkstationPromptTemplateContractMock,
    ).toHaveBeenCalledWith("Review", { sessionID: "~default" });
  });

  it("loads prompt-template contract data through the selected session", async () => {
    useDashboardSessionStore.setState({ selectedSessionID: "session-beta" });
    getCurrentFactoryWorkstationPromptTemplateContractMock.mockResolvedValue(
      promptTemplateContract,
    );

    const { result } = renderHook(
      () => useCurrentWorkstationPromptTemplateContract("Review"),
      { wrapper: createQueryClientWrapper() },
    );

    await waitFor(() => {
      expect(result.current.status).toBe("success");
    });

    expect(
      getCurrentFactoryWorkstationPromptTemplateContractMock,
    ).toHaveBeenCalledWith("Review", { sessionID: "session-beta" });
    expect(result.current.data).toBe(promptTemplateContract);
  });

  it("surfaces typed API failures from the prompt-template contract query", async () => {
    getCurrentFactoryWorkstationPromptTemplateContractMock.mockRejectedValue({
      code: "NOT_FOUND",
      message: "Current factory workstation not found.",
      name: "CurrentFactoryPromptTemplateAPIError",
    });

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
