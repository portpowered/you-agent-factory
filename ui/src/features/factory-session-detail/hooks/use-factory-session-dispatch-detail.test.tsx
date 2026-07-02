import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FactorySessionsAPIError } from "../../../api/factory-sessions/api";
import { getFactorySessionDispatchDetail } from "../../../api/factory-sessions/dispatch-detail";
import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { useFactorySessionDispatchDetail } from "./use-factory-session-dispatch-detail";

vi.mock("../../../api/factory-sessions/dispatch-detail", async () => {
  const actual = await vi.importActual(
    "../../../api/factory-sessions/dispatch-detail",
  );
  return {
    ...actual,
    getFactorySessionDispatchDetail: vi.fn(),
  };
});

const successfulDispatch = {
  artifactIds: ["artifact-1"],
  dispatchKind: "JAVASCRIPT_VERIFY",
  id: "dispatch-success",
  javascript: {
    executionMode: "live",
    taskKind: "VERIFY",
    taskLabel: "verify-docs",
  },
  orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
  providerSessionRefs: [
    {
      id: "provider-session-1",
      kind: "session_id" as const,
      provider: "codex",
    },
  ],
  sessionId: "dur-sess-js-success-002",
  status: "COMPLETED",
};

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("useFactorySessionDispatchDetail", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns idle when no dispatch is selected", () => {
    const { result } = renderHook(
      () => useFactorySessionDispatchDetail("dur-sess-js-success-002", null),
      { wrapper: createWrapper() },
    );

    expect(result.current).toEqual({ status: "idle" });
    expect(getFactorySessionDispatchDetail).not.toHaveBeenCalled();
  });

  it("returns loading while dispatch detail is resolving", () => {
    vi.mocked(getFactorySessionDispatchDetail).mockImplementation(
      () => new Promise(() => undefined),
    );

    const { result } = renderHook(
      () =>
        useFactorySessionDispatchDetail(
          "dur-sess-js-success-002",
          "dispatch-success",
        ),
      { wrapper: createWrapper() },
    );

    expect(result.current).toEqual({ status: "loading" });
  });

  it("returns success with normalized live-provider inspection projection", async () => {
    vi.mocked(getFactorySessionDispatchDetail).mockResolvedValue(
      successfulDispatch,
    );

    const { result } = renderHook(
      () =>
        useFactorySessionDispatchDetail(
          "dur-sess-js-success-002",
          "dispatch-success",
        ),
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(result.current.status).toBe("success");
    });

    if (result.current.status !== "success") {
      throw new Error("expected success dispatch detail state");
    }

    expect(result.current.data).toMatchObject({
      dispatchID: "dispatch-success",
      javascript: {
        executionMode: "live",
        taskKind: "VERIFY",
        taskLabel: "verify-docs",
      },
      providerSessionRefs: [
        {
          id: "provider-session-1",
          kind: "session_id",
          provider: "codex",
        },
      ],
      sessionID: "dur-sess-js-success-002",
      status: "COMPLETED",
    });
  });

  it("returns not-found when dispatch detail is missing", async () => {
    vi.mocked(getFactorySessionDispatchDetail).mockRejectedValue(
      new FactorySessionsAPIError("Dispatch not found.", {
        code: "NOT_FOUND",
        status: 404,
      }),
    );

    const { result } = renderHook(
      () =>
        useFactorySessionDispatchDetail(
          "dur-sess-js-success-002",
          "dispatch-missing",
        ),
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(result.current).toEqual({ status: "not-found" });
    });
  });

  it("returns error when dispatch detail fails to load", async () => {
    vi.mocked(getFactorySessionDispatchDetail).mockRejectedValue(
      new FactorySessionsAPIError("Dispatch detail unavailable.", {
        code: "INTERNAL_ERROR",
        status: 500,
      }),
    );

    const { result } = renderHook(
      () =>
        useFactorySessionDispatchDetail(
          "dur-sess-js-success-002",
          "dispatch-error",
        ),
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(result.current).toEqual({
        message: "Dispatch detail unavailable.",
        status: "error",
      });
    });
  });
});
