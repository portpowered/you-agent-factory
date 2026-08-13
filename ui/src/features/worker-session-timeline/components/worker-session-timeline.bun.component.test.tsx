// biome-ignore lint/style/noExcessiveLinesPerFile: timeline state, detail, window, and target cases share one deterministic harness.
import { describe, expect, it, mock } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";

import type {
  WorkerSessionEventRecord,
  WorkerSessionObservation,
} from "../../../api/worker-sessions";
import type { UseWorkerSessionTimelineResult } from "../hooks/useWorkerSessionTimeline";
import {
  projectWorkerSessionTimeline,
  type WorkerSessionTimelineEntry,
} from "../lib/worker-session-timeline-projection";
import { getWorkerSessionTimelineMessages } from "../messages/worker-session-timeline";
import { WorkerSessionTimelineContent } from "./worker-session-timeline";
import { WorkerSessionTimelineWidget } from "./worker-session-timeline-widget";

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

describe("WorkerSessionTimelineContent bounded windows", () => {
  it("mounts one bounded window and returns to the live tail with pending activity", () => {
    const messages = getWorkerSessionTimelineMessages("en");
    const { container, rerender } = render(
      <WorkerSessionTimelineContent
        state={timelineState({
          entries: manyEntries(201),
          isFollowing: true,
          status: "live",
        })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );

    expect(timelineRows(container)).toHaveLength(200);
    expect(
      screen.getByText(messages.windowRangeLabel(2, 201, 201)),
    ).toBeTruthy();
    expect(timelineRows(container)[0]).toHaveAttribute(
      "data-worker-session-timeline-entry-position",
      "2",
    );

    const earlierButton = requireTimelineButton(
      container,
      messages.earlierEventsAction,
    );
    fireEvent.click(earlierButton);
    expect(timelineRows(container)).toHaveLength(200);
    expect(timelineRows(container)[0]).toHaveAttribute(
      "data-worker-session-timeline-entry-position",
      "1",
    );

    rerender(
      <WorkerSessionTimelineContent
        state={timelineState({
          entries: manyEntries(203),
          isFollowing: true,
          status: "live",
        })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );
    expect(timelineRows(container)).toHaveLength(200);
    const newActivityButton = requireTimelineButton(
      container,
      messages.newActivityAction(2),
    );
    expect(newActivityButton).toBeTruthy();
    expect(timelineRows(container)[0]).toHaveAttribute(
      "data-worker-session-timeline-entry-position",
      "1",
    );

    fireEvent.click(newActivityButton);
    expect(timelineRows(container)[0]).toHaveAttribute(
      "data-worker-session-timeline-entry-position",
      "4",
    );
    expect(
      findTimelineButton(container, messages.newActivityAction(2)),
    ).toBeNull();
  });
});

describe("WorkerSessionTimelineContent live focus", () => {
  it("does not move focus when a live record appends", () => {
    const retainedRecord = draftRecord(1, "MESSAGE", "COMPLETED", {
      contentBlocks: [{ kind: "TEXT", text: "retained" }],
      role: "assistant",
    });
    const { container, rerender } = render(
      <WorkerSessionTimelineContent
        state={timelineState({
          entries: projectEntries([retainedRecord]),
          isFollowing: true,
          status: "live",
        })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );
    const detailsButton = container.querySelector<HTMLButtonElement>(
      "button[aria-expanded]",
    );
    if (!detailsButton) {
      throw new Error("expected a focusable details button");
    }
    detailsButton.focus();

    rerender(
      <WorkerSessionTimelineContent
        state={timelineState({
          entries: projectEntries([
            retainedRecord,
            draftRecord(2, "PROGRESS", "UPDATED", { status: "RUNNING" }),
          ]),
          isFollowing: true,
          status: "live",
        })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );

    expect(container.ownerDocument.activeElement).toBe(detailsButton);
  });
});

describe("WorkerSessionTimelineContent large details", () => {
  it("uses semantic row positions and a keyboard-operable disclosure for large bodies", () => {
    const messages = getWorkerSessionTimelineMessages("en");
    const longText = "long body ".repeat(500);
    render(
      <WorkerSessionTimelineContent
        state={timelineState({
          entries: projectEntries([
            draftRecord(1, "MESSAGE", "COMPLETED", {
              contentBlocks: [{ kind: "TEXT", text: longText }],
              role: "assistant",
            }),
          ]),
          status: "completed",
        })}
        workerSessionID={WORKER_SESSION_ID}
      />,
    );

    const row = screen.getByRole("listitem");
    expect(row).toHaveAttribute("aria-posinset", "1");
    expect(row).toHaveAttribute("aria-setsize", "1");
    fireEvent.click(
      screen.getByRole("button", { name: messages.detailsLabel(false) }),
    );

    const boundedContent = document.querySelector(
      "details[data-worker-session-timeline-bounded-content='true']",
    );
    if (!(boundedContent instanceof HTMLDetailsElement)) {
      throw new Error("expected a native bounded content disclosure");
    }
    expect(boundedContent.open).toBe(false);
    const boundedSummary = boundedContent.querySelector("summary");
    if (!boundedSummary) {
      throw new Error("expected a bounded content summary");
    }
    fireEvent.click(boundedSummary);
    expect(boundedContent.open).toBe(true);
    expect(boundedSummary.textContent).toBe(messages.collapseContentAction);
    expect(boundedContent.textContent).toContain(
      `${longText.slice(0, 4_000)}…`,
    );
  });
});

describe("WorkerSessionTimelineWidget target selection", () => {
  it("selects a Worker Session target from the selected Work before rendering its timeline", async () => {
    const loadWorkerSessionTargets = async () => [observation("worker-1")];

    render(
      <WorkerSessionTimelineWidget
        enabled={false}
        factorySessionID="factory-1"
        loadWorkerSessionTargets={loadWorkerSessionTargets}
        stateOverride={timelineState({ status: "ready-empty" })}
        workID="work-1"
        workerSessionID={null}
      />,
      { wrapper: createQueryWrapper() },
    );

    const messages = getWorkerSessionTimelineMessages("en");
    await waitFor(() => {
      expect(
        screen.getByRole("combobox", {
          name: messages.sessionTargetSelectLabel,
        }),
      ).toBeTruthy();
    });
    expect(screen.getByText(messages.timelineTitle)).toBeTruthy();
  });

  it("keeps the explicit empty target state separate from the timeline", () => {
    const messages = getWorkerSessionTimelineMessages("en");

    render(
      <WorkerSessionTimelineWidget
        enabled={false}
        factorySessionID="factory-1"
        stateOverride={timelineState({ status: "ready-empty" })}
        workerSessionID={null}
      />,
      { wrapper: createQueryWrapper() },
    );

    expect(screen.getByText(messages.workSelectionRequired)).toBeTruthy();
    expect(screen.queryByText(messages.emptyState)).toBeNull();
  });
});

function createQueryWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function QueryWrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

function observation(workerSessionId: string): WorkerSessionObservation {
  return {
    attemptId: "attempt-1",
    direct: false,
    durationBasis: "UNAVAILABLE",
    durationMillis: null,
    endedAt: null,
    parse: { errors: [], ignored: 0 },
    providerSessionAvailable: false,
    startedAt: null,
    state: "RUNNING",
    transcript: "AVAILABLE",
    turnId: null,
    workIds: ["work-1"],
    workerSessionId,
  } as WorkerSessionObservation;
}

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

function manyEntries(count: number): WorkerSessionTimelineEntry[] {
  return Array.from({ length: count }, (_, index) => ({
    canonical: {
      cursor: {
        position: index + 1,
        streamGenerationId: "generation-ui",
        workerSessionId: WORKER_SESSION_ID,
      },
      position: index + 1,
      schemaId: "workers.draft.v1",
      sourceEventId: `ui-event-${index + 1}`,
      sourceId: WORKER_SESSION_ID,
      sourceSequence: index + 1,
      sourceType: "worker_session",
    },
    category: "progress",
    key: `ui-event-${index + 1}`,
    kind: "PROGRESS",
    phase: "UPDATED",
  }));
}

function timelineRows(container: HTMLElement): Element[] {
  return Array.from(
    container.querySelectorAll(
      "li[data-worker-session-timeline-entry-position]",
    ),
  );
}

function findTimelineButton(
  container: HTMLElement,
  label: string,
): HTMLButtonElement | null {
  return (
    Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(
      (button) => button.textContent === label,
    ) ?? null
  );
}

function requireTimelineButton(
  container: HTMLElement,
  label: string,
): HTMLButtonElement {
  const button = findTimelineButton(container, label);
  if (!button) {
    throw new Error(`expected timeline button: ${label}`);
  }
  return button;
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
