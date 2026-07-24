import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
  beforeEach(() => {
    vi.mocked(getFactorySessionDispatchDetail).mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("stays idle when no dispatch is selected", () => {
    const { result } = renderHook(
      () => useFactorySessionDispatchDetail("dur-sess-js-success-1", null),
      { wrapper: createWrapper() },
    );

    expect(getFactorySessionDispatchDetail).not.toHaveBeenCalled();
    expect(result.current).toEqual({ status: "idle" });
  });

  it("stays idle when the selected dispatch id is blank", () => {
    const { result } = renderHook(
      () => useFactorySessionDispatchDetail("dur-sess-js-success-1", "   "),
      { wrapper: createWrapper() },
    );

    expect(getFactorySessionDispatchDetail).not.toHaveBeenCalled();
    expect(result.current).toEqual({ status: "idle" });
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
});

describe("useFactorySessionDispatchDetail success and error states", () => {
  beforeEach(() => {
    vi.mocked(getFactorySessionDispatchDetail).mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("projects a successful durable dispatch detail read", async () => {
    vi.mocked(getFactorySessionDispatchDetail).mockResolvedValue({
      artifactIds: ["artifact-final-1"],
      dispatchKind: "JAVASCRIPT_AGENT",
      id: "dispatch-success-1",
      javascript: {
        executionMode: "live",
        taskKind: "AGENT",
        taskLabel: "Draft response",
      },
      orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
      providerSessionRefs: [
        {
          id: "sess_codex_1",
          kind: "session_id",
          provider: "codex",
        },
      ],
      sessionId: "dur-sess-js-success-1",
      status: "COMPLETED",
      statusTransitions: ["QUEUED", "RUNNING", "COMPLETED"],
    });

    const { result } = renderHook(
      () =>
        useFactorySessionDispatchDetail(
          "dur-sess-js-success-1",
          "dispatch-success-1",
        ),
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(result.current.status).toBe("success");
    });

    expect(getFactorySessionDispatchDetail).toHaveBeenCalledWith({
      dispatch_id: "dispatch-success-1",
      session_id: "dur-sess-js-success-1",
    });
    expect(result.current).toEqual({
      status: "success",
      data: expect.objectContaining({
        dispatchID: "dispatch-success-1",
        javascript: {
          executionMode: "live",
          taskKind: "AGENT",
          taskLabel: "Draft response",
        },
        providerSessionRefs: [
          {
            id: "sess_codex_1",
            kind: "session_id",
            provider: "codex",
          },
        ],
        status: "COMPLETED",
      }),
    });
  });

  it("returns not-found when the dispatch detail read is unavailable", async () => {
    vi.mocked(getFactorySessionDispatchDetail).mockRejectedValue(
      new FactorySessionsAPIError("Dispatch not found.", {
        code: "INTERNAL_ERROR",
        status: 404,
      }),
    );

    const { result } = renderHook(
      () =>
        useFactorySessionDispatchDetail(
          "dur-sess-js-failed-1",
          "dispatch-missing-1",
        ),
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(result.current.status).toBe("not-found");
    });
  });

  it("returns an error state when the dispatch detail read fails", async () => {
    vi.mocked(getFactorySessionDispatchDetail).mockRejectedValue(
      new FactorySessionsAPIError(
        "The factory sessions API rejected the request.",
        {
          code: "INTERNAL_ERROR",
          status: 500,
        },
      ),
    );

    const { result } = renderHook(
      () =>
        useFactorySessionDispatchDetail(
          "dur-sess-js-failed-1",
          "dispatch-failed-1",
        ),
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(result.current.status).toBe("error");
    });

    expect(result.current).toEqual({
      status: "error",
      message: "The factory sessions API rejected the request.",
    });
  });
});
