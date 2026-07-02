import { screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";
import {
  jsonResponse,
  renderWithQueryClient,
} from "./test-support/factory-session-detail-panel.test-helpers";

describe("FactorySessionDetailPanel durable summary", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows durable JavaScript session summary from shared typed session data", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/dur-sess-js-run-n-001")) {
        return jsonResponse({
          dialect: "you-workflow-v1",
          lifecycle: {
            startedAt: "2026-06-08T14:00:00Z",
            updatedAt: "2026-06-08T14:05:00Z",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          phase: "verify",
          phaseSummaries: [{ dispatchCount: 1, phase: "plan" }],
          progress: {
            completedDispatches: 1,
            failedDispatches: 0,
            inFlightDispatches: 1,
            totalDispatches: 3,
          },
          resolvedSource: {
            kind: "WORKFLOW_NAME",
            sourceRef: "workflow/release-train",
            sourceHash: "sha256:js-workflow-release-train",
          },
          sessionId: "dur-sess-js-run-n-001",
          status: "RUNNING",
          usage: { resources: [] },
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-run-n-001" />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    expect(screen.getAllByText("Running").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("verify")).toBeTruthy();
    expect(screen.queryAllByText("Idle")).toHaveLength(0);

    const fetchUrls = vi.mocked(globalThis.fetch).mock.calls.map(([input]) =>
      String(input),
    );
    expect(fetchUrls).toHaveLength(1);
    expect(fetchUrls[0]).toContain("/factory-sessions/dur-sess-js-run-n-001");
    expect(fetchUrls.some((url) => url.includes("/dispatches"))).toBe(false);
    expect(fetchUrls.some((url) => url.includes("/results?mode=final"))).toBe(
      false,
    );
    expect(fetchUrls.some((url) => url.includes("/results?mode=partial"))).toBe(
      false,
    );
  });

  it("shows durable JavaScript result and summary inspection details", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/dur-sess-js-success-002")) {
        return jsonResponse({
          artifactRefs: [
            {
              id: "art-js-success-001",
              kind: "FINAL_RESULT",
              label: "Docs refresh output",
              visibility: "PUBLIC",
            },
          ],
          dialect: "you-workflow-v1",
          lifecycle: {
            finishedAt: "2026-06-08T13:10:00Z",
            startedAt: "2026-06-08T13:00:02Z",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          progress: {
            completedDispatches: 2,
            failedDispatches: 0,
            inFlightDispatches: 0,
            totalDispatches: 2,
          },
          resolvedSource: {
            kind: "WORKFLOW_FILE",
            sourceRef: "workflow/.claude/workflows/docs-refresh.yaml",
            sourceHash: "sha256:js-workflow-docs-refresh",
          },
          resultSummary: {
            resultStatus: "FINAL",
            summary: "Documentation refresh complete.",
          },
          sessionId: "dur-sess-js-success-002",
          status: "SUCCEEDED",
          usage: { resources: [] },
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-success-002" />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    expect(screen.getAllByText("Succeeded").length).toBeGreaterThanOrEqual(1);

    const fetchUrls = vi.mocked(globalThis.fetch).mock.calls.map(([input]) =>
      String(input),
    );
    expect(
      fetchUrls.some((url) => url.includes("/results?mode=partial")),
    ).toBe(false);
  });
});
