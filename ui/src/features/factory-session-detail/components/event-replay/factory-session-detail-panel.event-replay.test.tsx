import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import { formatDateTime } from "../../../../i18n/formatters";
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
            'data: {"id":"evt-2","type":"ORCHESTRATOR_PHASE_CHANGED","context":{"sequence":2,"tick":2,"eventTime":"2026-06-25T10:00:01Z","sessionId":"dur-sess-js-success-002","sessionSequence":2,"phaseName":"review"},"payload":{"phase":"review","progressSummary":"Review work scheduled."}}',
            "",
            'data: {"id":"evt-3","type":"DISPATCH_QUEUED","context":{"sequence":3,"tick":3,"eventTime":"2026-06-25T10:00:02Z","sessionId":"dur-sess-js-success-002","sessionSequence":3,"phaseName":"review","dispatchId":"dispatch-1","workIds":["work-1","work-2"]},"payload":{"dispatchKind":"JAVASCRIPT_AGENT","label":"Draft release notes","queuePosition":1}}',
            "",
            'data: {"id":"evt-4","type":"DISPATCH_RECONCILED","context":{"sequence":4,"tick":4,"eventTime":"2026-06-25T10:00:03Z","sessionId":"dur-sess-js-success-002","sessionSequence":4,"phaseName":"review","dispatchId":"dispatch-1"},"payload":{"reconciledStatus":"COMPLETED","resultArtifactRef":{"id":"artifact-release-notes","kind":"FINAL_RESULT","label":"Release notes"},"artifactIds":["artifact-release-notes"]}}',
            "",
            'data: {"id":"evt-5","type":"SESSION_COMPLETED","context":{"sequence":5,"tick":5,"eventTime":"2026-06-25T10:00:05Z","sessionId":"dur-sess-js-success-002","sessionSequence":5,"phaseName":"review"},"payload":{"finalStatus":"SUCCEEDED","completedAt":"2026-06-25T10:00:05Z","artifactIds":["artifact-release-notes"]}}',
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
        screen.getByText("Showing 5 Factory Events."),
      ).toBeTruthy();
    });

    expect(replayTrigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Session started")).toBeTruthy();
    expect(screen.getByText("Phase changed")).toBeTruthy();
    expect(screen.getByText("Review work scheduled.")).toBeTruthy();
    expect(screen.getByText("Dispatch queued")).toBeTruthy();
    expect(screen.getByText("Draft release notes · Queue position 1")).toBeTruthy();
    expect(screen.getByText("Dispatch reconciled")).toBeTruthy();
    expect(screen.getByText("Dispatch status completed")).toBeTruthy();
    expect(screen.getAllByText(/artifact-release-notes/).length).toBeGreaterThan(0);
    expect(screen.getByText("Session completed")).toBeTruthy();
    expect(screen.getByText("Lifecycle status succeeded")).toBeTruthy();
    expect(screen.getByText("Dispatch Queued")).toBeTruthy();
    expect(
      screen.getByText(
        "Phase review · Dispatch dispatch-1 · 2 related work items",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Session event 4 · Tick 4")).toBeTruthy();
    expect(
      screen.getAllByText(
        formatDateTime("2026-06-25T10:00:03Z", "en", {
          timeZone: "UTC",
        }),
      ).length,
    ).toBeTruthy();
  });

  it("surfaces failed and warning replay cues inside the bounded timeline", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/dur-sess-js-warning-003")) {
        return jsonResponse({
          artifactRefs: [
            {
              id: "artifact-release-verification-log",
              kind: "FINAL_RESULT",
              label: "Release verification log",
              visibility: "PRIVATE",
            },
          ],
          dialect: "you-workflow-v1",
          lifecycle: {
            finishedAt: "2026-06-25T11:10:00Z",
            startedAt: "2026-06-25T11:00:00Z",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          progress: {
            completedDispatches: 0,
            failedDispatches: 1,
            inFlightDispatches: 0,
            totalDispatches: 1,
          },
          resolvedSource: {
            kind: "WORKFLOW_FILE",
            sourceRef: "workflow/.claude/workflows/release-verify.yaml",
            sourceHash: "sha256:release-verify",
          },
          resultSummary: {
            artifactRefs: [
              {
                id: "artifact-release-verification-log",
                kind: "FINAL_RESULT",
                label: "Release verification log",
                visibility: "PRIVATE",
              },
            ],
            resultStatus: "FAILED_WITH_PARTIAL",
            summary: "Release verification failed after checkpoint recovery.",
          },
          sessionId: "dur-sess-js-warning-003",
          status: "FAILED",
          usage: { resources: [] },
        });
      }
      if (url.endsWith("/factory-sessions/dur-sess-js-warning-003/events")) {
        return new Response(
          [
            'data: {"id":"evt-w1","type":"JAVASCRIPT_CHECKPOINT_REF","context":{"sequence":1,"tick":1,"eventTime":"2026-06-25T11:00:01Z","sessionId":"dur-sess-js-warning-003","sessionSequence":1,"phaseName":"verify","checkpointId":"checkpoint-9"},"payload":{"label":"Checkpoint before publish","warnings":[{"code":"CHECKPOINT_STALE","message":"Checkpoint is older than the latest source hash."}]}}',
            "",
            'data: {"id":"evt-w2","type":"DISPATCH_INTERRUPTED","context":{"sequence":2,"tick":2,"eventTime":"2026-06-25T11:00:02Z","sessionId":"dur-sess-js-warning-003","sessionSequence":2,"phaseName":"verify","dispatchId":"dispatch-verify"},"payload":{"reason":"Provider session timed out","observedStatus":"RUNNING","interruptedAt":"2026-06-25T11:00:02Z","retryPlanned":true}}',
            "",
            'data: {"id":"evt-w3","type":"SESSION_COMPLETED","context":{"sequence":3,"tick":3,"eventTime":"2026-06-25T11:00:05Z","sessionId":"dur-sess-js-warning-003","sessionSequence":3,"phaseName":"verify"},"payload":{"finalStatus":"FAILED","completedAt":"2026-06-25T11:00:05Z","failureDetail":{"message":"Release verification failed."}}}',
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
      <FactorySessionDetailPanel sessionID="dur-sess-js-warning-003" />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(screen.getByText("Checkpoint recorded")).toBeTruthy();
    });

    expect(screen.getByText("Checkpoint before publish")).toBeTruthy();
    expect(screen.getByText("Dispatch interrupted")).toBeTruthy();
    expect(screen.getByText("Provider session timed out · Retry planned")).toBeTruthy();
    expect(screen.getByText("Session completed")).toBeTruthy();
    expect(screen.getByText("Release verification failed.")).toBeTruthy();
    expect(
      screen.getByText("Phase verify · Checkpoint checkpoint-9"),
    ).toBeTruthy();
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
