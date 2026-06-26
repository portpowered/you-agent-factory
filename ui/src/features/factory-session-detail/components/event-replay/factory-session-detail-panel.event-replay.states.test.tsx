import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  jsonResponse,
  renderWithQueryClient,
} from "../factory-session-detail-panel.test-helpers";

describe("FactorySessionDetailPanel event replay disclosure loading and empty states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders an explicit loading state while durable replay is being read", async () => {
    let resolveReplayResponse: ((value: Response) => void) | undefined;
    const replayResponse = new Promise<Response>((resolve) => {
      resolveReplayResponse = resolve;
    });

    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/dur-sess-js-success-002")) {
        return jsonResponse(
          buildSuccessfulDurableSession("dur-sess-js-success-002"),
        );
      }
      if (url.endsWith("/factory-sessions/dur-sess-js-success-002/events")) {
        return replayResponse;
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
    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(
        screen.getByText("Loading durable Factory Event replay…"),
      ).toBeTruthy();
    });

    resolveReplayResponse?.(
      new Response("", {
        headers: {
          "Content-Type": "text/event-stream",
        },
        status: 200,
      }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "No durable Factory Events are available for this session.",
        ),
      ).toBeTruthy();
    });
  });

  it("renders an explicit empty state when durable replay has no events in scope", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/dur-sess-js-success-002")) {
        return jsonResponse(
          buildSuccessfulDurableSession("dur-sess-js-success-002"),
        );
      }
      if (url.endsWith("/factory-sessions/dur-sess-js-success-002/events")) {
        return new Response(
          [": keepalive", "", "event: ping", "data: ignored", ""].join("\n"),
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
    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "No durable Factory Events are available for this session.",
        ),
      ).toBeTruthy();
    });
  });
});

describe("FactorySessionDetailPanel event replay disclosure unavailable and error states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders an explicit unavailable state when durable replay is omitted for the session", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/dur-sess-js-warning-003")) {
        return jsonResponse(
          buildWarningDurableSession("dur-sess-js-warning-003"),
        );
      }
      if (url.endsWith("/factory-sessions/dur-sess-js-warning-003/events")) {
        return new Response("not found", { status: 404 });
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
      expect(
        screen.getByText(
          "Durable Factory Event replay is unavailable for this session.",
        ),
      ).toBeTruthy();
    });

    expect(screen.getByText("Partial result ref")).toBeTruthy();
  });

  it("renders an explicit error state when the durable replay read fails", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/dur-sess-js-warning-003")) {
        return jsonResponse(
          buildWarningDurableSession("dur-sess-js-warning-003"),
        );
      }
      if (url.endsWith("/factory-sessions/dur-sess-js-warning-003/events")) {
        return new Response(
          JSON.stringify({
            code: "INTERNAL_ERROR",
            message: "replay boom",
          }),
          {
            headers: { "Content-Type": "application/json" },
            status: 500,
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
      expect(screen.getByText("replay boom")).toBeTruthy();
    });

    expect(screen.getByText("Artifacts")).toBeTruthy();
  });
});

function buildSuccessfulDurableSession(sessionId: string) {
  return {
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
      completedDispatches: 0,
      failedDispatches: 0,
      inFlightDispatches: 0,
      totalDispatches: 0,
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
    sessionId,
    status: "SUCCEEDED",
    usage: { resources: [] },
  };
}

function buildWarningDurableSession(sessionId: string) {
  return {
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
    sessionId,
    status: "FAILED",
    usage: { resources: [] },
  };
}
