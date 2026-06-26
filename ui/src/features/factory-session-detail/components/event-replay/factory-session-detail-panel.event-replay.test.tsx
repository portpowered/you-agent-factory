import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  jsonResponse,
  renderWithQueryClient,
} from "../factory-session-detail-panel.test-helpers";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: focused event replay coverage keeps one fetch harness and assertion seam.
describe("FactorySessionDetailPanel event replay disclosure", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("reveals bounded durable Factory Event replay inline for durable JavaScript sessions", async () => {
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
      if (url.endsWith("/factory-sessions/dur-sess-js-success-002/events")) {
        return new Response(
          [
            'data: {"id":"evt-1","type":"SESSION_STARTED","context":{"sequence":1,"tick":1,"eventTime":"2026-06-25T10:00:00Z","sessionId":"dur-sess-js-success-002","sessionSequence":1,"phaseName":"plan"},"payload":{"startedAt":"2026-06-25T10:00:00Z"}}',
            "",
            'data: {"id":"evt-2","type":"DISPATCH_QUEUED","context":{"sequence":2,"tick":2,"eventTime":"2026-06-25T10:00:02Z","sessionId":"dur-sess-js-success-002","sessionSequence":2,"phaseName":"review","dispatchId":"dispatch-1","workIds":["work-1","work-2"]},"payload":{"dispatchKind":"JAVASCRIPT_AGENT"}}',
            "",
          ].join("\n"),
          {
            headers: {
              "Content-Type": "text/event-stream",
            },
            status: 200,
          },
        );
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-success-002" />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    const user = userEvent.setup();
    const replayTrigger = screen.getByRole("button", {
      name: "Expand Factory Event replay",
    });

    expect(replayTrigger.getAttribute("aria-expanded")).toBe("false");

    await user.click(replayTrigger);

    await waitFor(() => {
      expect(
        screen.getByText("Showing 2 Factory Events."),
      ).toBeTruthy();
    });

    expect(replayTrigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Session Started")).toBeTruthy();
    expect(screen.getByText("Dispatch Queued")).toBeTruthy();
    expect(
      screen.getByText(
        "Phase review · Dispatch dispatch-1 · 2 related work items",
      ),
    ).toBeTruthy();
    expect(screen.getByText("2026-06-25T10:00:02Z")).toBeTruthy();
  });

  it("keeps replay disclosure out of non-durable session detail views", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse({
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root",
        id: "session-beta",
        isDefault: false,
        project: "beta",
        runtime: {
          artifacts: [],
          dispatches: [],
          javascript: {
            checkpoints: [],
            childDispatchCounts: {
              completed: 0,
              queued: 0,
              running: 0,
            },
            phase: "review",
            phases: ["review"],
            scriptStatus: "IDLE",
          },
          lifecycle: {
            startedAt: "2026-06-08T14:00:00Z",
            updatedAt: "2026-06-08T14:05:00Z",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          progress: {
            categories: {},
            factoryState: "RUNNING",
            inFlightCount: 0,
            totalTokens: 0,
          },
          status: "IDLE",
          usage: { resources: [] },
        },
        target: { kind: "named", name: "beta" },
      }),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    expect(
      screen.queryByRole("button", { name: "Expand Factory Event replay" }),
    ).toBeNull();
  });
});
