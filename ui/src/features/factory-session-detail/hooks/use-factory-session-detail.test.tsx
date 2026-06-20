import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { useFactorySessionDetail } from "./use-factory-session-detail";

describe("useFactorySessionDetail", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("routes durable factory session reads through durable get and result surfaces", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/factory-sessions/dur-sess-petri-success-001") {
        return jsonResponse({
          orchestratorKind: "PETRI",
          resolvedSource: {
            kind: "FACTORY_ID",
            sourceRef: "factory/customer-support-triage",
          },
          resultSummary: {
            resultStatus: "FINAL",
            summary: "Ticket triaged and resolved.",
          },
          sessionId: "dur-sess-petri-success-001",
          status: "SUCCEEDED",
        });
      }
      if (url.endsWith("/results?mode=final")) {
        return jsonResponse({
          mode: "final",
          resultStatus: "FINAL",
          sessionId: "dur-sess-petri-success-001",
        });
      }
      if (url.endsWith("/results?mode=partial")) {
        return jsonResponse({
          mode: "partial",
          resultStatus: "PARTIAL",
          sessionId: "dur-sess-petri-success-001",
        });
      }
      if (url.endsWith("/dispatches")) {
        return jsonResponse({
          dispatches: [],
          sessionId: "dur-sess-petri-success-001",
        });
      }
      if (url.endsWith("/artifacts")) {
        return jsonResponse({
          artifacts: [],
          sessionId: "dur-sess-petri-success-001",
        });
      }
      return new Response("not found", { status: 404 });
    });

    const { result } = renderHook(
      () => useFactorySessionDetail("dur-sess-petri-success-001"),
      { wrapper: createQueryWrapper() },
    );

    await waitFor(() => {
      expect(result.current.status).toBe("success");
    });

    if (result.current.status !== "success") {
      throw new Error("expected durable factory session detail success");
    }

    expect(result.current.data.kind).toBe("durable");
    expect(result.current.data.session.status).toBe("SUCCEEDED");
    expect(result.current.data.durableResult?.resultStatus).toBe("FINAL");
    expect(result.current.data.durablePartialResult?.resultStatus).toBe(
      "PARTIAL",
    );
    expect(vi.mocked(globalThis.fetch)).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-petri-success-001/dispatches",
      expect.objectContaining({ method: "GET" }),
    );
    expect(vi.mocked(globalThis.fetch)).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-petri-success-001/artifacts",
      expect.objectContaining({ method: "GET" }),
    );
    expect(vi.mocked(globalThis.fetch)).not.toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-petri-success-001/result",
      expect.anything(),
    );
    expect(vi.mocked(globalThis.fetch)).not.toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-petri-success-001/partial-result",
      expect.anything(),
    );
  });

  it("routes live factory session reads through the live detail path", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/factory-sessions/session-beta") {
        return jsonResponse({
          id: "session-beta",
          runtime: {
            orchestratorKind: "PETRI",
            status: "IDLE",
          },
        });
      }
      return new Response("not found", { status: 404 });
    });

    const { result } = renderHook(
      () => useFactorySessionDetail("session-beta"),
      { wrapper: createQueryWrapper() },
    );

    await waitFor(() => {
      expect(result.current.status).toBe("success");
    });

    if (result.current.status !== "success") {
      throw new Error("expected live factory session detail success");
    }

    expect(result.current.data.kind).toBe("live");
    expect(result.current.data.session.id).toBe("session-beta");
    expect(vi.mocked(globalThis.fetch)).not.toHaveBeenCalledWith(
      "/factory-sessions/session-beta/results?mode=final",
      expect.anything(),
    );
  });
});

function createQueryWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return function QueryWrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
    status,
  });
}
