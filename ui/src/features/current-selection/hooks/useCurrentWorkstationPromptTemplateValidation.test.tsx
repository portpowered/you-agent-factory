import "../../../../testing/bun-current-factory-prompt-template-api-mocks";
import { beforeEach, describe, expect, it } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { type PromptTemplateValidationResult } from "../../../api/current-factory-prompt-template";
import { validateCurrentFactoryWorkstationPromptTemplateMock } from "../../../../testing/bun-current-factory-prompt-template-api-mocks";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../../dashboard/state/dashboardSessionStore";
import {
  buildCurrentWorkstationPromptTemplateValidationQueryKey,
  useCurrentWorkstationPromptTemplateValidation,
} from "./useCurrentWorkstationPromptTemplateValidation";

const promptTemplateValidationResult: PromptTemplateValidationResult = {
  diagnostics: [],
  valid: true,
};

describe("useCurrentWorkstationPromptTemplateValidation", () => {
  beforeEach(() => {
    resetDashboardSessionStore();
    validateCurrentFactoryWorkstationPromptTemplateMock.mockReset();
  });

  it("builds a stable query key for one prompt draft", () => {
    expect(
      buildCurrentWorkstationPromptTemplateValidationQueryKey(
        "Review",
        "Use {{ .Prompt }}",
        null,
      ),
    ).toEqual([
      "current-workstation-prompt-template-validation",
      "~default",
      "Review",
      "Use {{ .Prompt }}",
    ]);
  });

  it("does not fetch validation while the draft is blank", () => {
    const { result } = renderHook(
      () => useCurrentWorkstationPromptTemplateValidation("Review", "   "),
      { wrapper: createQueryClientWrapper() },
    );

    expect(
      validateCurrentFactoryWorkstationPromptTemplateMock,
    ).not.toHaveBeenCalled();
    expect(result.current).toMatchObject({
      data: undefined,
      error: null,
      isFetching: false,
      isPending: true,
      status: "pending",
    });
  });

  it("rejects manual refetches without a selected workstation or prompt", async () => {
    const { result: missingWorkstation } = renderHook(
      () => useCurrentWorkstationPromptTemplateValidation(undefined, "prompt"),
      { wrapper: createQueryClientWrapper() },
    );
    const { result: missingPrompt } = renderHook(
      () => useCurrentWorkstationPromptTemplateValidation("Review", undefined),
      { wrapper: createQueryClientWrapper() },
    );

    await expect(missingWorkstation.current.refetch()).resolves.toMatchObject({
      error: new Error("workstationName is required"),
      status: "error",
    });
    await expect(missingPrompt.current.refetch()).resolves.toMatchObject({
      error: new Error("prompt is required"),
      status: "error",
    });
  });

  it("loads authoritative prompt validation for the active draft", async () => {
    validateCurrentFactoryWorkstationPromptTemplateMock.mockResolvedValue(
      promptTemplateValidationResult,
    );

    const { result } = renderHook(
      () =>
        useCurrentWorkstationPromptTemplateValidation(
          "Review",
          "Use {{ .Prompt }}",
        ),
      { wrapper: createQueryClientWrapper() },
    );

    await waitFor(() => {
      expect(result.current).toMatchObject({
        data: promptTemplateValidationResult,
        error: null,
        isPending: false,
        status: "success",
      });
    });

    expect(validateCurrentFactoryWorkstationPromptTemplateMock).toHaveBeenCalledWith(
      "Review",
      "Use {{ .Prompt }}",
      { sessionID: "~default" },
    );
  });

  it("validates the active prompt draft through the selected session", async () => {
    useDashboardSessionStore.setState({ selectedSessionID: "session-beta" });
    validateCurrentFactoryWorkstationPromptTemplateMock.mockResolvedValue(
      promptTemplateValidationResult,
    );

    const { result } = renderHook(
      () =>
        useCurrentWorkstationPromptTemplateValidation(
          "Review",
          "Use {{ .Prompt }}",
        ),
      { wrapper: createQueryClientWrapper() },
    );

    await waitFor(() => {
      expect(result.current.status).toBe("success");
    });

    expect(validateCurrentFactoryWorkstationPromptTemplateMock).toHaveBeenCalledWith(
      "Review",
      "Use {{ .Prompt }}",
      { sessionID: "session-beta" },
    );
  });

  it("surfaces typed validation API failures for the active draft", async () => {
    validateCurrentFactoryWorkstationPromptTemplateMock.mockRejectedValue({
      code: "BAD_REQUEST",
      message: "Prompt validation request was rejected.",
      name: "CurrentFactoryPromptTemplateAPIError",
    });

    const { result } = renderHook(
      () =>
        useCurrentWorkstationPromptTemplateValidation(
          "Review",
          "Use {{ .Prompt }}",
        ),
      { wrapper: createQueryClientWrapper() },
    );

    await waitFor(() => {
      expect(result.current).toMatchObject({
        data: undefined,
        error: {
          code: "BAD_REQUEST",
          message: "Prompt validation request was rejected.",
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
