import { describe, expect, it, mock } from "bun:test";
import { fireEvent, render, screen, within } from "@testing-library/react";

import type { WorkerSessionEventRecord } from "../../../api/worker-sessions";
import type { UseWorkerSessionTimelineResult } from "../hooks/useWorkerSessionTimeline";
import {
  projectWorkerSessionTimeline,
  type WorkerSessionTimelineEntry,
} from "../lib/worker-session-timeline-projection";
import { getWorkerSessionTimelineMessages } from "../messages/worker-session-timeline";
import { WorkerSessionTimelineContent } from "./worker-session-timeline";

const WORKER_SESSION_ID = "worker-session-ui-1";

describe("WorkerSessionTimelineContent states", () => {
  it("keeps loading and empty states distinct", () => {
    const messages = getWorkerSessionTimelineMessages("en");
    const { rerender } = render(
      <WorkerSessionTimelineContent
        state={timelineState({ status: "loading" })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );

    expect(screen.getAllByText(messages.loadingState).length).toBeGreaterThan(
      0,
    );
    expect(screen.getByRole("status")).toBeTruthy();

    rerender(
      <WorkerSessionTimelineContent
        state={timelineState({ status: "ready-empty" })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );
    expect(screen.getAllByText(messages.emptyState).length).toBeGreaterThan(0);
  });

  it("identifies live following and completed terminal delivery", () => {
    const messages = getWorkerSessionTimelineMessages("en");
    const { rerender } = render(
      <WorkerSessionTimelineContent
        state={timelineState({ status: "live" })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );
    expect(
      screen.getAllByText(messages.liveFollowingState).length,
    ).toBeGreaterThan(0);

    const terminalEntry = projectEntries([
      draftRecord(1, "SESSION", "COMPLETED", { status: "COMPLETED" }),
    ]);
    rerender(
      <WorkerSessionTimelineContent
        state={timelineState({ entries: terminalEntry, status: "completed" })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );
    expect(screen.getAllByText(messages.completedState).length).toBeGreaterThan(
      0,
    );
    expect(
      screen.getByText(messages.terminalOutcomeLabel("SUCCESS")),
    ).toBeTruthy();
  });

  it("preserves retained entries when the source fails", () => {
    render(
      <WorkerSessionTimelineContent
        state={timelineState({
          entries: projectEntries([
            draftRecord(1, "MESSAGE", "COMPLETED", {
              role: "assistant",
              contentBlocks: [{ kind: "TEXT", text: "retained" }],
            }),
          ]),
          sourceError: {
            code: "SOURCE_FAILURE",
            message: "retained history unavailable",
          },
          status: "source-error",
        })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "retained history unavailable",
    );
    expect(screen.getByText("Message · COMPLETED")).toBeTruthy();
  });
});

describe("WorkerSessionTimelineContent details", () => {
  it("renders canonical identity, attempts, continuation, content, usage, and terminal detail", () => {
    const entries = projectEntries([
      draftRecord(1, "SESSION", "STARTED", {
        attempt: 1,
        attemptId: "attempt-1",
        model: "gpt-5",
        providerSelection: {
          executorProvider: "ACP",
          modelProvider: "openai",
        },
        status: "STARTING",
      }),
      draftRecord(2, "MESSAGE", "COMPLETED", {
        contentBlocks: [{ kind: "TEXT", text: "Please inspect the build." }],
        role: "user",
      }),
      draftRecord(3, "REASONING", "DELTA", {
        summaryDelta: "Checking the build output.",
      }),
      draftRecord(4, "TOOL", "COMPLETED", {
        resultSummary: { exitCode: 0, output: "ok" },
        status: "completed",
        toolCallId: "tool-1",
        toolName: "command_execution",
      }),
      draftRecord(5, "USAGE", "UPDATED", {
        inputTokens: 10,
        model: "gpt-5",
        outputTokens: 8,
        totalTokens: 18,
      }),
      draftRecord(6, "SESSION", "COMPLETED", {
        continuation: {
          kind: "session_id",
          id: "provider-thread-1",
          provider: "codex",
        },
        lineage: {
          successorWorkerSessionId: "worker-session-ui-2",
        },
        status: "COMPLETED",
      }),
    ]);

    const messages = getWorkerSessionTimelineMessages("en");
    render(
      <WorkerSessionTimelineContent
        onNavigateToWorkerSession={mock(() => {})}
        state={timelineState({ entries, status: "completed" })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );

    expect(screen.getByText("openai")).toBeTruthy();
    expect(screen.getByText(messages.modelProviderLabel)).toBeTruthy();
    expect(screen.getAllByText("gpt-5").length).toBeGreaterThan(0);
    expect(
      screen.getAllByRole("button", { name: messages.detailsLabel(false) }),
    ).toHaveLength(entries.length);

    for (const button of screen.getAllByRole("button", {
      name: messages.detailsLabel(false),
    })) {
      fireEvent.click(button);
    }

    expect(
      screen.getAllByText("Please inspect the build.").length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("Checking the build output.")).toBeTruthy();
    expect(screen.getByText("command_execution")).toBeTruthy();
    expect(screen.getByText(/"exitCode"/)).toBeTruthy();
    expect(screen.getByText("18")).toBeTruthy();
    expect(screen.getByText("worker-session-ui-2")).toBeTruthy();
    expect(
      screen.getByText("codex / session_id / provider-thread-1"),
    ).toBeTruthy();

    const continuationHeading = screen.getByText(messages.continuationLabel);
    expect(continuationHeading).toBeTruthy();
    expect(
      within(continuationHeading.closest("section") as HTMLElement).getByRole(
        "button",
        { name: messages.openWorkerSessionLabel("worker-session-ui-2") },
      ),
    ).toBeTruthy();
  });
});

describe("WorkerSessionTimelineContent recording health", () => {
  it("keeps recording health separate from the worker terminal outcome", () => {
    const entries = projectEntries([
      draftRecord(1, "SESSION", "FAILED", {
        failureDetail: "Provider stopped responding.",
        status: "FAILED",
      }),
    ]);
    const messages = getWorkerSessionTimelineMessages("en");

    render(
      <WorkerSessionTimelineContent
        state={timelineState({
          entries,
          recordingHealth: "INCOMPLETE",
          recordingHealthReason: "RETAINED_HEAD_MOVED",
          status: "completed",
        })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );

    expect(
      screen.getByText(messages.recordingHealthLabel("INCOMPLETE")),
    ).toBeTruthy();
    expect(
      screen.getByText(messages.terminalOutcomeLabel("FAILURE")),
    ).toBeTruthy();
    expect(screen.getByText(messages.failureLabel)).toBeTruthy();
    expect(screen.getByText(/RETAINED_HEAD_MOVED/)).toBeTruthy();
    expect(screen.getByText("Provider stopped responding.")).toBeTruthy();
  });
});

function timelineState(
  overrides: Partial<UseWorkerSessionTimelineResult> = {},
): UseWorkerSessionTimelineResult {
  return {
    acknowledgedCursor: null,
    entries: [],
    isFollowing: false,
    records: [],
    recordingHealth: null,
    recordingHealthReason: null,
    reconnectCursor: null,
    replaySummary: null,
    retry: mock(() => {}),
    sourceError: null,
    status: "loading",
    streamGenerationId: null,
    terminalDelivery: null,
    ...overrides,
  };
}

function projectEntries(
  records: WorkerSessionEventRecord[],
): WorkerSessionTimelineEntry[] {
  return projectWorkerSessionTimeline(records);
}

function draftRecord(
  position: number,
  kind: string,
  phase: string,
  payload: Record<string, unknown>,
): WorkerSessionEventRecord {
  return {
    cursor: {
      position,
      streamGenerationId: "generation-ui",
      workerSessionId: WORKER_SESSION_ID,
    },
    payload: {
      kind,
      payload,
      phase,
      provenance: { provider: "codex" },
    },
    position,
    schemaId: "workers.draft.v1",
    sourceEventId: `ui-event-${position}`,
    sourceId: WORKER_SESSION_ID,
    sourceSequence: position,
    sourceType: "worker_session",
  };
}
