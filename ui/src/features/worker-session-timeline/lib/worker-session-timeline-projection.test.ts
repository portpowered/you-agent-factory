import {
  projectWorkerSessionTimeline,
  projectWorkerSessionTimelineEntry,
  type WorkerSessionEventRecord,
} from "./worker-session-timeline-projection";

type DraftRecord = Record<string, unknown>;

function record(
  position: number,
  payload: DraftRecord,
  overrides: Partial<WorkerSessionEventRecord> = {},
): WorkerSessionEventRecord {
  return {
    cursor: {
      position,
      workerSessionId: "worker-session-1",
      streamGenerationId: "generation-1",
    },
    position,
    sourceType: "worker_session",
    sourceId: "worker-session-1",
    sourceSequence: position,
    sourceEventId: `event-${position}`,
    schemaId: "workers.draft.v1",
    payload,
    ...overrides,
  };
}

function draft(
  kind: string,
  phase: string,
  payload: DraftRecord,
  overrides: DraftRecord = {},
): DraftRecord {
  return {
    kind,
    phase,
    provenance: {
      provider: "codex",
      delivery: "NATIVE_STREAM",
      representation: "SNAPSHOT",
    },
    payload,
    ...overrides,
  };
}

describe("Worker Session lifecycle projection", () => {
  it("projects lifecycle identity, attempts, retry reasons, and continuation links", () => {
    const opening = record(
      1,
      draft(
        "SESSION",
        "STARTED",
        {
          status: "STARTING",
          workerSessionId: "worker-session-1",
          attemptId: "attempt-1",
          attempt: 1,
          attemptReason: "INITIAL",
          model: "gpt-5",
          providerSelection: {
            modelProvider: "openai",
            executorProvider: "ACP",
          },
        },
        { dispatchId: "attempt-1" },
      ),
    );
    const retry = record(
      2,
      draft("SESSION", "UPDATED", {
        status: "RETRYING",
        workerSessionId: "worker-session-1",
        attemptId: "attempt-2",
        attempt: 2,
        attemptReason: "RETRY",
        lineage: {
          previousDispatchId: "attempt-1",
          previousAttemptId: "attempt-1",
        },
      }),
      { sourceType: "worker_session_attempt", sourceEventId: "retry" },
    );
    const continuation = record(
      3,
      draft("SESSION", "UPDATED", {
        status: "COMPLETED",
        workerSessionId: "worker-session-1",
        continuation: { provider: "codex", kind: "session_id", id: "thread-1" },
        lineage: { successorWorkerSessionId: "worker-session-2" },
      }),
      { sourceType: "worker_session_lineage", sourceEventId: "successor" },
    );
    const terminal = record(
      4,
      draft("SESSION", "COMPLETED", { status: "COMPLETED" }),
    );

    const entries = projectWorkerSessionTimeline([
      terminal,
      continuation,
      retry,
      opening,
    ]);

    expect(entries.map((entry) => entry.canonical.position)).toEqual([
      1, 2, 3, 4,
    ]);
    expect(entries[0]?.identity).toEqual({
      provider: "codex",
      modelProvider: "openai",
      executorProvider: "ACP",
      model: "gpt-5",
    });
    expect(entries[0]?.attempt).toEqual({
      id: "attempt-1",
      number: 1,
      reason: "INITIAL",
      dispatchId: "attempt-1",
      status: "STARTING",
    });
    expect(entries[1]?.attempt?.reason).toBe("RETRY");
    expect(entries[1]?.continuation).toEqual({
      previousDispatchId: "attempt-1",
      previousAttemptId: "attempt-1",
    });
    expect(entries[2]?.continuation).toEqual({
      providerSession: {
        provider: "codex",
        kind: "session_id",
        id: "thread-1",
      },
      successorWorkerSessionId: "worker-session-2",
    });
    expect(entries[3]?.terminal).toEqual({
      outcome: "SUCCESS",
      status: "COMPLETED",
    });
  });
});

describe("Worker Session content projection", () => {
  it("projects user and assistant messages plus reasoning snapshots and deltas", () => {
    const entries = projectWorkerSessionTimeline([
      record(
        1,
        draft("MESSAGE", "COMPLETED", {
          role: "user",
          contentBlocks: [{ kind: "TEXT", text: "Please inspect the build." }],
        }),
        { sourceEventId: "user-message" },
      ),
      record(
        2,
        draft(
          "MESSAGE",
          "DELTA",
          {
            contentBlockIndex: 0,
            contentBlockKind: "TEXT",
            textDelta: "Build is ",
          },
          { itemId: "message-1" },
        ),
        { sourceEventId: "assistant-delta-1" },
      ),
      record(
        3,
        draft(
          "MESSAGE",
          "COMPLETED",
          {
            role: "assistant",
            contentBlocks: [{ kind: "TEXT", text: "Build is green." }],
          },
          { itemId: "message-1" },
        ),
        { sourceEventId: "assistant-snapshot" },
      ),
      record(
        4,
        draft("REASONING", "DELTA", {
          summaryDelta: "Checking the test output.",
        }),
      ),
      record(
        5,
        draft("REASONING", "COMPLETED", {
          summary: "The test output is complete.",
        }),
      ),
    ]);

    expect(entries.map((entry) => entry.category)).toEqual([
      "message",
      "message",
      "message",
      "reasoning",
      "reasoning",
    ]);
    expect(entries[0]?.message).toEqual({
      role: "user",
      contentBlocks: [{ kind: "TEXT", text: "Please inspect the build." }],
      text: "Please inspect the build.",
    });
    expect(entries[1]?.message).toEqual({
      delta: {
        contentBlockIndex: 0,
        contentBlockKind: "TEXT",
        textDelta: "Build is ",
      },
    });
    expect(entries[2]?.itemId).toBe("message-1");
    expect(entries[2]?.message?.text).toBe("Build is green.");
    expect(entries[3]?.reasoning).toEqual({
      representation: "DELTA",
      summaryDelta: "Checking the test output.",
    });
    expect(entries[4]?.reasoning).toEqual({
      representation: "SNAPSHOT",
      summary: "The test output is complete.",
    });
  });
});

describe("Worker Session tool projection", () => {
  it("projects tool lifecycle, output, and usage", () => {
    const entries = projectWorkerSessionTimeline([
      record(
        6,
        draft("TOOL", "STARTED", {
          toolCallId: "tool-1",
          toolName: "command_execution",
          status: "in_progress",
          argumentsSummary: { command: "go test ./..." },
        }),
      ),
      record(
        7,
        draft("TOOL", "DELTA", {
          toolCallId: "tool-1",
          outputDelta: "ok",
        }),
      ),
      record(
        8,
        draft("TOOL", "COMPLETED", {
          toolCallId: "tool-1",
          toolName: "command_execution",
          status: "completed",
          resultSummary: { exitCode: 0, output: "ok" },
        }),
      ),
      record(
        9,
        draft("USAGE", "UPDATED", {
          cacheWriteTokens: 3,
          inputTokens: 10,
          outputTokens: 8,
          reasoningOutputTokens: 2,
          totalTokens: 20,
          model: "gpt-5",
        }),
      ),
      record(
        10,
        draft("PROGRESS", "UPDATED", {
          label: "compile",
          message: "building the package",
          percentComplete: 42,
        }),
      ),
    ]);

    expect(entries.map((entry) => entry.category)).toEqual([
      "tool",
      "tool",
      "tool",
      "usage",
      "progress",
    ]);
    expect(entries[0]?.tool?.argumentsSummary).toEqual({
      command: "go test ./...",
    });
    expect(entries[1]?.tool).toEqual({
      toolCallId: "tool-1",
      outputDelta: "ok",
    });
    expect(entries[2]?.tool?.resultSummary).toEqual({
      exitCode: 0,
      output: "ok",
    });
    expect(entries[3]?.usage).toEqual({
      cacheWriteTokens: 3,
      inputTokens: 10,
      outputTokens: 8,
      reasoningOutputTokens: 2,
      totalTokens: 20,
      model: "gpt-5",
    });
    expect(entries[3]?.identity?.model).toBe("gpt-5");
    expect(entries[4]?.progress).toEqual({
      label: "compile",
      message: "building the package",
      percentComplete: 42,
    });
  });
});

describe("Worker Session terminal projection", () => {
  it("keeps terminal failure and cancellation separate from success", () => {
    const failed = projectWorkerSessionTimelineEntry(
      record(
        1,
        draft("SESSION", "FAILED", {
          status: "FAILED",
          failureCause: "PROVIDER_FAILURE",
          failureDetail: "The provider stopped responding.",
          agentRunFailureClass: "provider_timeout",
        }),
      ),
    );
    const canceled = projectWorkerSessionTimelineEntry(
      record(2, draft("SESSION", "CANCELED", { status: "CANCELED" })),
    );

    expect(failed.terminal).toEqual({
      outcome: "FAILURE",
      status: "FAILED",
      failure: {
        kind: "PROVIDER_FAILURE",
        code: "provider_timeout",
        message: "The provider stopped responding.",
      },
    });
    expect(failed.failure?.message).toBe("The provider stopped responding.");
    expect(canceled.terminal).toEqual({
      outcome: "CANCELED",
      status: "CANCELED",
    });
  });
});

describe("Worker Session forward compatibility", () => {
  it("retains unknown schemas as bounded generic entries without payload dumps", () => {
    const payload = Object.fromEntries(
      Array.from({ length: 20 }, (_, index) => [
        `field-${index.toString().padStart(2, "0")}`,
        index === 0 ? "do-not-dump-this-secret" : { nested: index },
      ]),
    );
    const unknown = projectWorkerSessionTimelineEntry(
      record(1, payload, {
        schemaId: "provider.future.v9",
        sourceType: "provider_observation",
        sourceId: "provider-source-1",
      }),
    );

    expect(unknown.category).toBe("generic");
    expect(unknown.kind).toBe("UNKNOWN");
    expect(unknown.generic).toEqual({
      schemaId: "provider.future.v9",
      sourceType: "provider_observation",
      sourceId: "provider-source-1",
      sourceSequence: 1,
      payloadKeys: [
        "field-00",
        "field-01",
        "field-02",
        "field-03",
        "field-04",
        "field-05",
        "field-06",
        "field-07",
        "field-08",
        "field-09",
        "field-10",
        "field-11",
        "field-12",
        "field-13",
        "field-14",
        "field-15",
      ],
      payloadKeyCount: 20,
      payloadKeysTruncated: true,
    });
    expect(JSON.stringify(unknown)).not.toContain("do-not-dump-this-secret");
  });
});

describe("Worker Session malformed optional facts", () => {
  it("does not infer malformed optional facts or mutate canonical input", () => {
    const input = record(
      2,
      draft("MESSAGE", "COMPLETED", {
        role: 42,
        contentBlocks: [
          { text: "missing kind" },
          { kind: "TEXT", text: "safe" },
        ],
        providerSelection: { modelProvider: "" },
        inputTokens: "10",
        totalTokens: null,
        continuation: { provider: "codex", kind: "session_id" },
      }),
    );
    const before = JSON.stringify(input);
    const output = projectWorkerSessionTimeline([input]);

    expect(output[0]?.message).toEqual({
      contentBlocks: [{ kind: "TEXT", text: "safe" }],
      text: "safe",
    });
    expect(output[0]?.identity).toEqual({ provider: "codex" });
    expect(output[0]?.usage).toBeUndefined();
    expect(output[0]?.continuation).toBeUndefined();
    expect(JSON.stringify(input)).toBe(before);
  });
});

describe("Worker Session legacy lifecycle projection", () => {
  it("maps the retained legacy lifecycle schema without making it a second source", () => {
    const legacy = record(
      1,
      { status: "COMPLETED", failureDetail: "" },
      { schemaId: "worker_session.completed" },
    );

    const [entry] = projectWorkerSessionTimeline([legacy]);

    expect(entry?.category).toBe("session");
    expect(entry?.kind).toBe("SESSION");
    expect(entry?.phase).toBe("COMPLETED");
    expect(entry?.terminal?.outcome).toBe("SUCCESS");
  });
});
