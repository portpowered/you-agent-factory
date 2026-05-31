import "../../../../../testing/bun-current-factory-prompt-template-api-mocks";
import { beforeEach, describe, expect, it } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import type { PromptTemplateValidationResult } from "../../../../api/current-factory-prompt-template";
import { validateCurrentFactoryWorkstationPromptTemplateMock } from "../../../../../testing/bun-current-factory-prompt-template-api-mocks";
import { DashboardSessionProvider } from "../../../dashboard/session/dashboard-session-provider";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../../../dashboard/state/dashboardSessionStore";
import {
  buildCurrentWorkstationPromptTemplateValidationQueryKey,
  useCurrentWorkstationPromptTemplateValidation,
} from "./useCurrentWorkstationPromptTemplateValidation";

const promptTemplateValidationResult: PromptTemplateValidationResult = {
  diagnostics: [],
  valid: true,
};

function resetValidationHookTestState(): void {
  resetDashboardSessionStore();
  validateCurrentFactoryWorkstationPromptTemplateMock.mockReset();
}

describe("useCurrentWorkstationPromptTemplateValidation query key", () => {
  beforeEach(resetValidationHookTestState);

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
});

describe("useCurrentWorkstationPromptTemplateValidation fetch gating", () => {
  beforeEach(resetValidationHookTestState);

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
});

describe("useCurrentWorkstationPromptTemplateValidation refetch guards", () => {
  beforeEach(resetValidationHookTestState);

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
});

describe("useCurrentWorkstationPromptTemplateValidation success", () => {
  beforeEach(resetValidationHookTestState);

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
});

describe("useCurrentWorkstationPromptTemplateValidation API failures", () => {
  beforeEach(resetValidationHookTestState);

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

  it("surfaces not-found failures from the prompt-template validation query", async () => {
    validateCurrentFactoryWorkstationPromptTemplateMock.mockRejectedValue({
      code: "NOT_FOUND",
      message: "Current factory workstation not found.",
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
          code: "NOT_FOUND",
          message: "Current factory workstation not found.",
        },
        isPending: false,
        status: "error",
      });
    });
  });

  it("surfaces network failures from the prompt-template validation query", async () => {
    validateCurrentFactoryWorkstationPromptTemplateMock.mockRejectedValue({
      code: "NETWORK_ERROR",
      message:
        "The dashboard could not reach the current factory prompt-template validation API.",
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
          code: "NETWORK_ERROR",
          message:
            "The dashboard could not reach the current factory prompt-template validation API.",
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
      <QueryClientProvider client={queryClient}>
        <DashboardSessionProvider>{children}</DashboardSessionProvider>
      </QueryClientProvider>
    );
  };
}
