// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: live-vs-durable regression keeps both inspection paths in one harness.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { getFactorySessionDetailMessages } from "../messages/factory-session-detail";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";

const APPROVED_CUSTOMER_TERMS = [
  "Factory Session",
  "Dispatch",
  "Artifact",
  "Provider Session",
] as const;

describe("Factory Session detail live vs durable coexistence regression", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps live JavaScript session detail on the runtime-backed path", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/session-beta")) {
        return jsonResponse({
          id: "session-beta",
          runtime: {
            artifacts: [],
            dispatches: [],
            javascript: {
              checkpoints: [],
              childDispatchCounts: {
                completed: 4,
                queued: 1,
                running: 2,
              },
              phase: "review",
              phases: ["plan", "review"],
              scriptStatus: "IDLE",
            },
            orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
            status: "IDLE",
          },
        });
      }
      if (url.endsWith("/factory-sessions/session-beta/result")) {
        return jsonResponse({
          resultArtifactRef: {
            id: "artifact-final",
            kind: "FINAL_RESULT",
            visibility: "CUSTOMER",
          },
          sessionId: "session-beta",
          status: "IDLE",
        });
      }
      if (url.endsWith("/factory-sessions/session-beta/partial-result")) {
        return jsonResponse({
          partialResultArtifactRef: {
            id: "artifact-partial",
            kind: "CHILD_RESULT",
            visibility: "CUSTOMER",
          },
          phase: "review",
          sessionId: "session-beta",
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    expect(screen.getByText("Loading factory session runtime…")).toBeTruthy();

    await waitFor(() => {
      expect(
        screen.getByText("Dynamic workflow (JavaScript factory session)"),
      ).toBeTruthy();
    });

    expect(screen.getByText("artifact-final · FINAL_RESULT")).toBeTruthy();
    expect(screen.getByText("artifact-partial · CHILD_RESULT")).toBeTruthy();
    expect(screen.queryByText("Factory Session complete")).toBeNull();
    expect(screen.queryByText("Loading Factory Session detail")).toBeNull();
    expect(vi.mocked(globalThis.fetch)).toHaveBeenCalledWith(
      "/factory-sessions/session-beta/result",
      expect.objectContaining({ method: "GET" }),
    );
    expect(vi.mocked(globalThis.fetch)).not.toHaveBeenCalledWith(
      "/factory-sessions/session-beta/results?mode=final",
      expect.anything(),
    );
  });

  it("keeps durable session detail on durable reads without live runtime affordances", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/factory-sessions/dur-sess-petri-success-001") {
        return jsonResponse({
          orchestratorKind: "PETRI",
          progress: {
            completedDispatches: 1,
            totalDispatches: 1,
          },
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
          dispatches: [
            {
              dispatchKind: "PETRI_TRANSITION",
              id: "disp-petri-success-001",
              label: "plan-task",
              status: "COMPLETED",
            },
          ],
          sessionId: "dur-sess-petri-success-001",
        });
      }
      if (url.endsWith("/artifacts")) {
        return jsonResponse({
          artifacts: [
            {
              dispatchId: "disp-petri-success-001",
              id: "art-petri-final-001",
              kind: "FINAL_RESULT",
              label: "Triage summary",
              retrievalRef: {
                href: "/factory-sessions/dur-sess-petri-success-001/artifacts/art-petri-final-001",
                method: "GET",
              },
              visibility: "PUBLIC",
            },
          ],
          sessionId: "dur-sess-petri-success-001",
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-petri-success-001" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Factory Session complete")).toBeTruthy();
    });

    expect(screen.getByText("Ticket triaged and resolved.")).toBeTruthy();
    expect(screen.getByText("Dispatches")).toBeTruthy();
    expect(screen.getByText("Artifacts")).toBeTruthy();
    expect(
      screen.queryByText("Dynamic workflow (JavaScript factory session)"),
    ).toBeNull();
    expect(screen.queryByText("Loading factory session runtime…")).toBeNull();
    expect(vi.mocked(globalThis.fetch)).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-petri-success-001/results?mode=final",
      expect.objectContaining({ method: "GET" }),
    );
    expect(vi.mocked(globalThis.fetch)).not.toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-petri-success-001/result",
      expect.anything(),
    );
  });

  it("uses approved customer-facing terminology on durable detail copy", () => {
    const messages = getFactorySessionDetailMessages("en");
    const durableCopy = [
      messages.durableLoadingTitle,
      messages.durableLoadingState,
      messages.durableMissingTitle,
      messages.durableMissingState,
      messages.durableErrorTitle,
      messages.durableErrorState,
      messages.durablePartialTitle,
      messages.durablePartialState,
      messages.durableTerminalTitle,
      messages.durableTerminalState,
      messages.dispatchesHeading,
      messages.dispatchesEmptyState,
      messages.artifactsHeading,
      messages.artifactsEmptyState,
      messages.providerSessionHeading,
      messages.inspectArtifactAction,
      messages.inspectProviderSessionAction,
    ].join(" ");

    for (const term of APPROVED_CUSTOMER_TERMS) {
      expect(durableCopy).toContain(term);
    }

    expect(durableCopy).not.toMatch(/\bruntime\b/i);
    expect(durableCopy).not.toMatch(/\btoken\b/i);
  });
});

function renderWithQueryClient(children: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>,
  );
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
    status,
  });
}
