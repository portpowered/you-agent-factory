// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction lint/nursery/noExcessiveLinesPerFile: existing provider-session detail coverage stayed intact during sibling-feature extraction.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
import { formatDateTime } from "../../../i18n/formatters";
import { ProviderSessionDetailPanel } from "./provider-session-detail-panel";

const SELECTED_SESSION = {
  dispatchID: "dispatch-review-active",
  id: "sess_active",
  kind: "session_id",
  provider: "codex",
} as const;

describe("ProviderSessionDetailPanel", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function expandDisclosure(name: string) {
    const button = screen.queryByRole("button", { name });
    if (button) {
      fireEvent.click(button);
    }
  }

  it("loads Cursor session details when provider is cursor", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          providerSession: {
            id: "cursor_sess_01",
            kind: "session_id",
            provider: "cursor",
          },
          source: {
            relativePath: "store.db",
            sizeBytes: 4096,
          },
          transcript: [
            {
              lineNumber: 1,
              order: 1,
              text: "Hello from Cursor",
              type: "assistant_message",
            },
          ],
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel
        selectedProviderSession={{
          dispatchID: "dispatch-cursor-active",
          id: "cursor_sess_01",
          kind: "session_id",
          provider: "cursor",
        }}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Transcript" })).toBeTruthy();
    });

    expect(screen.getAllByText("cursor_sess_01").length).toBeGreaterThan(0);
    expandDisclosure("Expand Transcript");
    expandDisclosure("Expand Assistant");
    expect(screen.getByText("Hello from Cursor")).toBeTruthy();
    expect(vi.mocked(globalThis.fetch)).toHaveBeenCalledWith(
      "/provider-sessions/detail?id=cursor_sess_01&kind=session_id&provider=cursor",
      { method: "GET" },
    );
  });

  it("shows an explicit loading state while session details are being fetched", () => {
    vi.mocked(globalThis.fetch).mockReturnValue(new Promise(() => undefined));

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    expect(screen.getByRole("status").textContent).toContain(
      "Loading session details...",
    );
    expect(screen.getByText("sess_active")).toBeTruthy();
  });

  it("renders zh-CN provider-session loading and missing states", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "NOT_FOUND",
          message: "Missing provider session.",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 404,
          statusText: "Not Found",
        },
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel
        locale="zh-CN"
        selectedProviderSession={SELECTED_SESSION}
      />,
    );

    expect(screen.getByRole("status").textContent).toContain(
      "正在加载会话详情...",
    );
    expect(screen.getAllByText("sess_active").length).toBeGreaterThan(0);
    expect(screen.queryByText("session_id")).toBeNull();

    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toContain(
        "无法在服务器上找到所选 provider session。",
      );
    });
  });

  it("keeps selected-session preview focused on the session id for unknown kinds", () => {
    vi.mocked(globalThis.fetch).mockReturnValue(new Promise(() => undefined));

    renderWithQueryClient(
      <ProviderSessionDetailPanel
        locale="zh-CN"
        selectedProviderSession={{
          ...SELECTED_SESSION,
          kind: "mystery_kind",
        }}
      />,
    );

    expect(screen.getAllByText("sess_active").length).toBeGreaterThan(0);
    expect(screen.queryByText("mystery_kind")).toBeNull();
  });

  it("shows a not-found state when the session file is missing", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "NOT_FOUND",
          message: "Missing provider session.",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 404,
          statusText: "Not Found",
        },
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toContain(
        "The selected provider session could not be found on the server.",
      );
    });
  });

  it("shows an empty-state message for session files without events", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 0,
            functionCalls: [],
            lineCount: 0,
            malformedLineCount: 0,
            parseErrors: [],
            reasoning: [],
            turns: [],
            unknownEventCount: 0,
            unknownEvents: [],
          },
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toContain(
        "The selected session did not contain any parseable event records.",
      );
    });
    expect(
      screen.getByText("2026/05/18/rollout-sess_active.jsonl"),
    ).toBeTruthy();
  });

  it("shows parse-error diagnostics when no session events can be parsed", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 0,
            functionCalls: [],
            lineCount: 1,
            malformedLineCount: 1,
            parseErrors: [
              {
                lineNumber: 1,
                message: "invalid character '}' after object key:value pair",
              },
            ],
            reasoning: [],
            turns: [],
            unknownEventCount: 0,
            unknownEvents: [],
          },
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("alert").textContent).toContain(
        "The selected session could not be parsed. Review the malformed-line diagnostics below.",
      );
    });
    expect(
      screen.getByRole("heading", { name: "Maintainer Diagnostics" }),
    ).toBeTruthy();
    expandDisclosure("Expand Maintainer Diagnostics");
    expect(
      screen.getByText("invalid character '}' after object key:value pair"),
    ).toBeTruthy();
  });

  it("shows an empty-transcript state when parsing succeeds without transcript entries", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 2,
            functionCalls: [],
            lineCount: 2,
            malformedLineCount: 0,
            parseErrors: [],
            reasoning: [],
            turns: [
              {
                eventCount: 2,
                functionCallCount: 0,
                index: 1,
                reasoningCount: 0,
                responseItemCount: 0,
                startedAt: "2026-05-18T14:10:00Z",
              },
            ],
            unknownEventCount: 2,
            unknownEvents: [
              {
                lineNumber: 1,
                payloadType: "mystery_payload",
                type: "mystery_event",
              },
              {
                lineNumber: 2,
                payloadType: "mystery_payload",
                type: "mystery_event",
              },
            ],
          },
          transcript: [],
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toContain(
        "The selected session was parsed, but it did not contain any transcript-visible entries.",
      );
    });
    expect(
      screen.getByRole("heading", { name: "Session Analysis" }),
    ).toBeTruthy();
    expandDisclosure("Expand Session Analysis");
    expect(screen.getByRole("heading", { name: "Token Usage" })).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Maintainer Diagnostics" }),
    ).toBeTruthy();
    expandDisclosure("Expand Maintainer Diagnostics");
    expect(screen.getByText("Unknown event on line 1")).toBeTruthy();
    expect(screen.getByText("Unknown event on line 2")).toBeTruthy();
  });

  it("renders parsed source metadata, turns, token usage, and diagnostics", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 6,
            functionCalls: [
              {
                arguments: '{"path":"pkg/api"}',
                callId: "call_1",
                name: "list_dir",
                order: 1,
                output: '{"entries":3}',
                status: "completed",
                turnIndex: 1,
                type: "function_call",
              },
            ],
            lineCount: 7,
            malformedLineCount: 1,
            parseErrors: [
              {
                lineNumber: 7,
                message: "unexpected end of JSON input",
              },
            ],
            reasoning: [
              {
                order: 1,
                sourceType: "reasoning",
                summary: "Inspect the provider-session diff before retrying.",
                text: "Investigate the failed response stream.",
                turnIndex: 1,
              },
            ],
            tokenUsage: {
              cachedInputTokens: 5,
              inputTokens: 120,
              outputTokens: 45,
              reasoningOutputTokens: 17,
              totalTokens: 182,
            },
            turns: [
              {
                eventCount: 4,
                functionCallCount: 1,
                index: 1,
                reasoningCount: 1,
                responseItemCount: 3,
                startedAt: "2026-05-18T14:10:00Z",
              },
            ],
            unknownEventCount: 1,
            unknownEvents: [
              {
                lineNumber: 6,
                payloadType: "mystery_payload",
                type: "mystery_event",
              },
            ],
          },
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "Session Analysis" }),
      ).toBeTruthy();
    });

    expect(
      screen.getByText("2026/05/18/rollout-sess_active.jsonl"),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Session Analysis" }),
    ).toBeTruthy();
    expandDisclosure("Expand Session Analysis");
    expandDisclosure("Expand Execution Turns");
    expect(screen.getByText("Turn 1")).toBeTruthy();
    expect(screen.queryByText("list_dir")).toBeNull();
    expect(screen.queryByRole("heading", { name: "Reasoning" })).toBeNull();
    expect(
      screen.getByRole("heading", { name: "Maintainer Diagnostics" }),
    ).toBeTruthy();
    expandDisclosure("Expand Token Usage");
    expandDisclosure("Expand Maintainer Diagnostics");
    expect(screen.getByText("182")).toBeTruthy();
    expect(screen.getByText("unexpected end of JSON input")).toBeTruthy();
    expect(screen.getByText("Unknown event on line 6")).toBeTruthy();
  });

  it("renders chronological transcript entries with expandable tool payloads", async () => {
    const longArguments =
      '{"path":"pkg/api/provider_session_details.go","mode":"diff","note":"' +
      "x".repeat(360) +
      '"}';
    const longOutput = [
      "Chunk ID: exec-123",
      "Wall time: 0.6289 seconds",
      "Process exited with code 0",
      "Original token count: 22",
      "Output:",
      "provider-session parsing verified successfully",
      `details:${"y".repeat(360)}`,
    ].join("\n");

    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          transcript: [
            {
              lineNumber: 1,
              order: 1,
              text: "Summarize the rollout state for this work item.",
              timestamp: "2026-05-18T14:10:00Z",
              turnIndex: 1,
              type: "user_message",
            },
            {
              lineNumber: 2,
              order: 2,
              text: "The failing edge is in provider-session parsing.",
              timestamp: "2026-05-18T14:10:02Z",
              turnIndex: 1,
              type: "assistant_message",
            },
            {
              encrypted: true,
              lineNumber: 3,
              order: 3,
              sourceType: "reasoning",
              timestamp: "2026-05-18T14:10:03Z",
              turnIndex: 1,
              type: "reasoning",
            },
            {
              arguments: longArguments,
              callId: "call_tool_1",
              lineNumber: 4,
              name: "read_file",
              order: 4,
              status: "completed",
              timestamp: "2026-05-18T14:10:04Z",
              turnIndex: 1,
              type: "tool_call",
            },
            {
              callId: "call_tool_1",
              lineNumber: 5,
              name: "exec_command",
              order: 5,
              output: longOutput,
              status: "completed",
              timestamp: "2026-05-18T14:10:05Z",
              turnIndex: 1,
              type: "tool_output",
            },
            {
              lineNumber: 6,
              order: 6,
              sourceType: "task_started",
              summary: "Retry attempt scheduled.",
              timestamp: "2026-05-18T14:10:06Z",
              turnIndex: 1,
              type: "system_event",
            },
          ],
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Transcript" })).toBeTruthy();
    });

    const transcriptTimestamp = formatDateTime("2026-05-18T14:10:04Z");

    expandDisclosure("Expand Transcript");
    expandDisclosure("Expand User");
    expandDisclosure("Expand Assistant");
    expandDisclosure("Expand Reasoning");
    expandDisclosure("Expand read_file");
    expandDisclosure("Expand call_tool_1");
    expandDisclosure("Expand task_started");

    expect(
      screen.getByText("Summarize the rollout state for this work item."),
    ).toBeTruthy();
    expect(
      screen.getByText("The failing edge is in provider-session parsing."),
    ).toBeTruthy();
    expect(screen.getAllByText("Encrypted Reasoning").length).toBeGreaterThan(
      0,
    );
    expect(
      screen.getByText(
        "Reasoning occurred for this step, but plaintext content is intentionally unavailable.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Encrypted reasoning content only.")).toBeTruthy();
    expect(screen.getByText("read_file")).toBeTruthy();
    expect(screen.getByText("call_tool_1")).toBeTruthy();
    expect(screen.getByText("Retry attempt scheduled.")).toBeTruthy();
    expect(screen.getByText("Command Result")).toBeTruthy();
    expect(screen.getByText("Exit Code")).toBeTruthy();
    expect(screen.getAllByText("0").length).toBeGreaterThan(0);
    expect(screen.getByText("Wall Time")).toBeTruthy();
    expect(screen.getByText("0.6289 seconds")).toBeTruthy();
    expect(screen.getAllByText("Summary").length).toBeGreaterThan(0);
    expect(
      screen.getByText("provider-session parsing verified successfully"),
    ).toBeTruthy();
    expect(screen.getByText("Order 4 / Turn 1")).toBeTruthy();
    expect(screen.getByText("Session Line 4")).toBeTruthy();
    expect(
      screen
        .getAllByTitle("2026-05-18T14:10:04Z")
        .some((element) => element.textContent === transcriptTimestamp),
    ).toBe(true);
    expect(screen.queryByText("Raw ISO Timestamp")).toBeNull();

    expect(screen.getByText(longArguments)).toBeTruthy();
    expect(
      screen.getByText((content) => content.includes("Chunk ID: exec-123")),
    ).toBeTruthy();
    expect(
      screen.getAllByText((content) =>
        content.includes("provider-session parsing verified successfully"),
      ).length,
    ).toBeGreaterThan(0);
  });

  it("uses the provider-session sans stack for customer-facing content while preserving monospace raw blocks", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          transcript: [
            {
              arguments: '{"path":"pkg/api/provider_session_details.go"}',
              callId: "call_tool_1",
              lineNumber: 1,
              name: "read_file",
              order: 1,
              status: "completed",
              timestamp: "2026-05-18T14:10:04Z",
              turnIndex: 1,
              type: "tool_call",
            },
          ],
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Transcript" })).toBeTruthy();
    });

    const panel = screen.getByLabelText("Selected Session Details");
    expect(panel.className).toContain("af-provider-session-sans");

    const transcriptHeading = screen.getByRole("heading", {
      name: "Transcript",
    });
    expect(panel.contains(transcriptHeading)).toBe(true);

    expandDisclosure("Expand Transcript");
    expandDisclosure("Expand read_file");

    const rawArguments = screen.getByText(
      '{"path":"pkg/api/provider_session_details.go"}',
    );
    expect(rawArguments.tagName).toBe("PRE");
    expect(rawArguments.className).toContain("text-code-medium");
  });

  it("formats source, transcript, and turn timestamps in the local timezone while preserving raw source and turn ISO access", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 2,
            functionCalls: [],
            lineCount: 2,
            malformedLineCount: 0,
            parseErrors: [],
            reasoning: [],
            turns: [
              {
                eventCount: 2,
                functionCallCount: 0,
                index: 1,
                reasoningCount: 0,
                responseItemCount: 2,
                startedAt: "2026-05-18T14:10:00Z",
              },
            ],
            unknownEventCount: 0,
            unknownEvents: [],
          },
          source: {
            modifiedAt: "2026-05-18T14:09:59Z",
            relativePath: "2026/05/18/rollout-sess_active.jsonl",
            sizeBytes: 2048,
          },
          transcript: [
            {
              lineNumber: 1,
              order: 1,
              text: "Summarize the rollout state for this work item.",
              timestamp: "2026-05-18T14:10:01Z",
              turnIndex: 1,
              type: "user_message",
            },
          ],
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Transcript" })).toBeTruthy();
    });

    const modifiedAt = formatDateTime("2026-05-18T14:09:59Z");
    const turnStartedAt = formatDateTime("2026-05-18T14:10:00Z");
    const transcriptTimestamp = formatDateTime("2026-05-18T14:10:01Z");

    expandDisclosure("Expand Source File");
    expandDisclosure("Expand Transcript");
    expandDisclosure("Expand Session Analysis");
    expandDisclosure("Expand Execution Turns");

    expect(
      screen
        .getAllByTitle("2026-05-18T14:09:59Z")
        .some((element) => element.textContent === modifiedAt),
    ).toBe(true);
    expect(
      screen
        .getAllByTitle("2026-05-18T14:10:00Z")
        .some((element) => element.textContent === turnStartedAt),
    ).toBe(true);
    expect(
      screen
        .getAllByTitle("2026-05-18T14:10:01Z")
        .some((element) => element.textContent === transcriptTimestamp),
    ).toBe(true);

    expect(screen.queryByText("2026-05-18T14:10:01Z")).toBeNull();
  });

  it("rerenders provider-session timestamps for zh-CN without changing raw values or payload text", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 2,
            functionCalls: [],
            lineCount: 2,
            malformedLineCount: 0,
            parseErrors: [],
            reasoning: [],
            turns: [
              {
                eventCount: 2,
                functionCallCount: 0,
                index: 1,
                reasoningCount: 0,
                responseItemCount: 2,
                startedAt: "2026-05-18T14:10:00Z",
              },
            ],
            unknownEventCount: 0,
            unknownEvents: [],
          },
          source: {
            modifiedAt: "2026-05-18T14:09:59Z",
            relativePath: "2026/05/18/rollout-sess_active.jsonl",
            sizeBytes: 2048,
          },
          transcript: [
            {
              lineNumber: 1,
              order: 1,
              text: "Summarize the rollout state for this work item.",
              timestamp: "2026-05-18T14:10:01Z",
              turnIndex: 1,
              type: "user_message",
            },
          ],
        }),
      ),
    );

    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <ProviderSessionDetailPanel
          selectedProviderSession={SELECTED_SESSION}
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Transcript" })).toBeTruthy();
    });

    expandDisclosure("Expand Transcript");
    expect(
      screen
        .getAllByTitle("2026-05-18T14:10:01Z")
        .some(
          (element) =>
            element.textContent ===
            formatDateTime("2026-05-18T14:10:01Z", "en"),
        ),
    ).toBe(true);
    expandDisclosure("Expand User");
    expect(
      screen.getByText("Summarize the rollout state for this work item."),
    ).toBeTruthy();

    rerender(
      <QueryClientProvider client={queryClient}>
        <ProviderSessionDetailPanel
          locale="zh-CN"
          selectedProviderSession={SELECTED_SESSION}
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "会话记录" })).toBeTruthy();
    });

    expandDisclosure("展开会话记录");
    expect(
      screen
        .getAllByTitle("2026-05-18T14:10:01Z")
        .some(
          (element) =>
            element.textContent ===
            formatDateTime("2026-05-18T14:10:01Z", "zh-CN"),
        ),
    ).toBe(true);
    const zhUserExpandButton = screen.queryByRole("button", {
      name: "展开用户",
    });
    if (zhUserExpandButton) {
      fireEvent.click(zhUserExpandButton);
    }
    expect(
      screen.getByText("Summarize the rollout state for this work item."),
    ).toBeTruthy();

    expect(screen.queryByText("2026-05-18T14:10:01Z")).toBeNull();
    expect(screen.getByText("sess_active")).toBeTruthy();
  });

  it("shows localized fallback copy for invalid and missing provider-session timestamps without leaking raw invalid values", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 3,
            functionCalls: [],
            lineCount: 3,
            malformedLineCount: 0,
            parseErrors: [],
            reasoning: [],
            turns: [
              {
                eventCount: 1,
                functionCallCount: 0,
                index: 1,
                reasoningCount: 0,
                responseItemCount: 1,
                startedAt: " definitely-not-a-date ",
              },
              {
                eventCount: 1,
                functionCallCount: 0,
                index: 2,
                reasoningCount: 0,
                responseItemCount: 1,
                startedAt: null,
              },
            ],
            unknownEventCount: 0,
            unknownEvents: [],
          },
          source: {
            modifiedAt: " not-a-real-timestamp ",
            relativePath: "2026/05/18/rollout-sess_active.jsonl",
            sizeBytes: 2048,
          },
          transcript: [
            {
              lineNumber: 1,
              order: 1,
              text: "First row has an invalid timestamp.",
              timestamp: " definitely-not-a-date ",
              turnIndex: 1,
              type: "user_message",
            },
            {
              lineNumber: 2,
              order: 2,
              text: "Second row omitted the timestamp.",
              turnIndex: 1,
              type: "assistant_message",
            },
          ],
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel
        locale="zh-CN"
        selectedProviderSession={SELECTED_SESSION}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "会话记录" })).toBeTruthy();
    });

    expandDisclosure("展开会话记录");
    expandDisclosure("展开会话分析");
    expandDisclosure("展开执行轮次");

    expect(screen.getAllByText("不可用").length).toBeGreaterThan(0);
    expect(screen.getAllByText("无时间戳").length).toBeGreaterThan(0);
    expect(screen.queryByText(" definitely-not-a-date ")).toBeNull();
    expect(screen.queryByTitle(" definitely-not-a-date ")).toBeNull();
    expect(screen.queryByText("原始 ISO 时间戳")).toBeNull();
  });

  it("renders the transcript before summary sections in the success state", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 4,
            functionCalls: [
              {
                arguments: '{"path":"pkg/api"}',
                callId: "call_1",
                name: "list_dir",
                order: 3,
                output: '{"entries":3}',
                status: "completed",
                turnIndex: 1,
                type: "function_call",
              },
            ],
            lineCount: 4,
            malformedLineCount: 0,
            parseErrors: [],
            reasoning: [
              {
                order: 2,
                sourceType: "reasoning",
                summary: "Trace the transcript chronology first.",
                turnIndex: 1,
              },
            ],
            tokenUsage: {
              cachedInputTokens: 0,
              inputTokens: 12,
              outputTokens: 8,
              reasoningOutputTokens: 2,
              totalTokens: 22,
            },
            turns: [
              {
                eventCount: 4,
                functionCallCount: 1,
                index: 1,
                reasoningCount: 1,
                responseItemCount: 3,
                startedAt: "2026-05-18T14:10:00Z",
              },
            ],
            unknownEventCount: 0,
            unknownEvents: [],
          },
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Transcript" })).toBeTruthy();
    });

    expandDisclosure("Expand Session Analysis");
    const headingNames = screen
      .getAllByRole("heading")
      .map((heading) => heading.textContent ?? "");
    const transcriptIndex = headingNames.indexOf("Transcript");
    const supportingHeadings = [
      "Session Analysis",
      "Token Usage",
      "Turns",
      "Maintainer Diagnostics",
    ];

    expect(transcriptIndex).toBeGreaterThan(-1);
    for (const headingName of supportingHeadings) {
      const headingIndex = headingNames.indexOf(headingName);
      if (headingIndex !== -1) {
        expect(transcriptIndex).toBeLessThan(headingIndex);
      }
    }
  });

  it("keeps reasoning and tool bodies in the transcript without duplicating them in summaries", async () => {
    const reasoningSummary =
      "Inspect the provider-session diff before retrying.";
    const reasoningText = "Investigate the failed response stream.";
    const callArguments = '{"path":"pkg/api/provider_session_details.go"}';
    const callOutput = '{"lines":128}';

    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 5,
            functionCalls: [
              {
                arguments: callArguments,
                callId: "call_1",
                name: "read_file",
                order: 4,
                output: callOutput,
                status: "completed",
                turnIndex: 1,
                type: "function_call",
              },
            ],
            lineCount: 5,
            malformedLineCount: 0,
            parseErrors: [],
            reasoning: [
              {
                order: 3,
                sourceType: "reasoning",
                summary: reasoningSummary,
                text: reasoningText,
                turnIndex: 1,
              },
            ],
            turns: [
              {
                eventCount: 5,
                functionCallCount: 1,
                index: 1,
                reasoningCount: 1,
                responseItemCount: 4,
                startedAt: "2026-05-18T14:10:00Z",
              },
            ],
            unknownEventCount: 0,
            unknownEvents: [],
          },
          transcript: [
            {
              order: 1,
              text: "Summarize the rollout state for this work item.",
              turnIndex: 1,
              type: "user_message",
            },
            {
              lineNumber: 2,
              order: 2,
              text: "The current failure is isolated to provider-session parsing.",
              timestamp: "2026-05-18T14:10:02Z",
              turnIndex: 1,
              type: "assistant_message",
            },
            {
              lineNumber: 3,
              order: 3,
              sourceType: "reasoning",
              summary: reasoningSummary,
              text: reasoningText,
              timestamp: "2026-05-18T14:10:03Z",
              turnIndex: 1,
              type: "reasoning",
            },
            {
              arguments: callArguments,
              callId: "call_1",
              lineNumber: 4,
              name: "read_file",
              order: 4,
              status: "completed",
              timestamp: "2026-05-18T14:10:04Z",
              turnIndex: 1,
              type: "tool_call",
            },
            {
              callId: "call_1",
              lineNumber: 5,
              order: 5,
              output: callOutput,
              status: "completed",
              timestamp: "2026-05-18T14:10:05Z",
              turnIndex: 1,
              type: "tool_output",
            },
          ],
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Transcript" })).toBeTruthy();
    });

    expandDisclosure("Expand Transcript");
    expandDisclosure("Expand Reasoning");
    expandDisclosure("Expand read_file");
    expandDisclosure("Expand call_1");

    expect(screen.getAllByText(reasoningSummary)).toHaveLength(1);
    expect(screen.getAllByText(reasoningText)).toHaveLength(1);
    expect(screen.getAllByText(callArguments)).toHaveLength(1);
    expect(screen.getAllByText(callOutput)).toHaveLength(1);
    expect(screen.getAllByText("read_file")).toHaveLength(1);
    expect(screen.getAllByText("Order 4 / Turn 1").length).toBeGreaterThan(0);
  });

  it("renders provider-session detail labels and templates from the zh-CN catalog", async () => {
    const execCommandOutput = [
      "Chunk ID: exec-456",
      "Wall time: 0.3000 seconds",
      "Process exited with code 0",
      "Original token count: 12",
      "Output:",
      "命令执行成功",
    ].join("\n");

    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 2,
            functionCalls: [
              {
                arguments: '{"path":"pkg/api"}',
                callId: null,
                name: "list_dir",
                order: 3,
                output: '{"entries":3}',
                status: "completed",
                turnIndex: 2,
                type: "function_call",
              },
            ],
            lineCount: 3,
            malformedLineCount: 1,
            parseErrors: [
              {
                lineNumber: 3,
                message: "unexpected end of JSON input",
              },
            ],
            reasoning: [
              {
                encrypted: true,
                order: 4,
                sourceType: "reasoning",
                turnIndex: 2,
              },
            ],
            tokenUsage: {
              cachedInputTokens: 5,
              inputTokens: 120,
              outputTokens: 45,
              reasoningOutputTokens: 17,
              totalTokens: 182,
            },
            turns: [
              {
                eventCount: 4,
                functionCallCount: 1,
                index: 2,
                reasoningCount: 1,
                responseItemCount: 3,
                startedAt: null,
              },
            ],
            unknownEventCount: 1,
            unknownEvents: [
              {
                lineNumber: 2,
                payloadType: "mystery_payload",
                type: "mystery_event",
              },
            ],
          },
          transcript: [
            {
              lineNumber: 1,
              order: 1,
              text: "请总结当前状态。",
              timestamp: "2026-05-18T14:10:01Z",
              turnIndex: 2,
              type: "user_message",
            },
            {
              lineNumber: 2,
              order: 2,
              name: "exec_command",
              output: execCommandOutput,
              timestamp: "2026-05-18T14:10:02Z",
              turnIndex: 2,
              type: "tool_output",
            },
          ],
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel
        locale="zh-CN"
        selectedProviderSession={SELECTED_SESSION}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "会话分析" })).toBeTruthy();
    });
    expandDisclosure("展开会话分析");
    expandDisclosure("展开令牌用量");
    expandDisclosure("展开执行轮次");
    expandDisclosure("展开维护诊断");
    expandDisclosure("展开会话记录");

    expect(screen.getByRole("heading", { name: "已选会话详情" })).toBeTruthy();
    expect(screen.getByText("会话 ID")).toBeTruthy();
    expect(screen.getAllByText("sess_active").length).toBeGreaterThan(0);
    expect(screen.getAllByText("用户").length).toBeGreaterThan(0);
    expect(screen.getAllByText("工具输出").length).toBeGreaterThan(0);
    expect(screen.getByText("输入")).toBeTruthy();
    expect(screen.getByText("缓存输入")).toBeTruthy();
    expect(screen.getByText("轮次 2")).toBeTruthy();
    expect(screen.getByText("无时间戳")).toBeTruthy();
    expect(screen.queryByText("顺序 3 / 轮次 2")).toBeNull();
    expect(screen.queryByText("调用 ID")).toBeNull();
    expect(screen.queryByText("加密推理")).toBeNull();
    expandDisclosure("展开工具输出");
    expect(screen.getByText("命令结果")).toBeTruthy();
    expect(screen.getByText("退出代码")).toBeTruthy();
    expect(screen.getByText("耗时")).toBeTruthy();
    expect(screen.getByText("摘要")).toBeTruthy();
    expect(screen.getByText("命令执行成功")).toBeTruthy();
    expect(
      screen.queryByText(
        "此步骤确实发生了推理，但明文内容会被有意隐藏，无法直接查看。",
      ),
    ).toBeNull();
    expect(screen.getByText("第 3 行")).toBeTruthy();
    expect(screen.getByText("第 2 行的未知事件")).toBeTruthy();
  });

  it("keeps zh-CN transcript prose on the provider-session sans stack", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          transcript: [
            {
              lineNumber: 1,
              order: 1,
              text: "请总结当前状态。",
              timestamp: "2026-05-18T14:10:01Z",
              turnIndex: 1,
              type: "user_message",
            },
          ],
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel
        locale="zh-CN"
        selectedProviderSession={SELECTED_SESSION}
      />,
    );

    const panel = await screen.findByLabelText("已选会话详情");
    expect(panel.className).toContain("af-provider-session-sans");
  });

  it("holds the zh-CN transcript reading model together across hierarchy, localization, timestamps, encrypted reasoning, and exec_command formatting", async () => {
    const execCommandOutput = [
      "Chunk ID: exec-789",
      "Wall time: 0.4821 seconds",
      "Process exited with code 0",
      "Original token count: 16",
      "Output:",
      "命令执行成功",
      `details:${"z".repeat(360)}`,
    ].join("\n");

    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 5,
            functionCalls: [
              {
                arguments: '{"path":"pkg/api/provider_session_details.go"}',
                callId: "call_exec_1",
                name: "exec_command",
                order: 4,
                output: execCommandOutput,
                status: "completed",
                turnIndex: 2,
                type: "function_call",
              },
            ],
            lineCount: 5,
            malformedLineCount: 1,
            parseErrors: [
              {
                lineNumber: 6,
                message: "unexpected end of JSON input",
              },
            ],
            reasoning: [
              {
                encrypted: true,
                order: 3,
                sourceType: "reasoning",
                turnIndex: 2,
              },
            ],
            turns: [
              {
                eventCount: 5,
                functionCallCount: 1,
                index: 2,
                reasoningCount: 1,
                responseItemCount: 4,
                startedAt: "2026-05-18T14:10:00Z",
              },
            ],
            unknownEventCount: 1,
            unknownEvents: [
              {
                lineNumber: 7,
                payloadType: "mystery_payload",
                type: "mystery_event",
              },
            ],
          },
          source: {
            modifiedAt: "2026-05-18T14:09:59Z",
            relativePath: "2026/05/18/rollout-sess_active.jsonl",
            sizeBytes: 2048,
          },
          transcript: [
            {
              lineNumber: 1,
              order: 1,
              text: "请总结当前状态。",
              timestamp: "2026-05-18T14:10:01Z",
              turnIndex: 2,
              type: "user_message",
            },
            {
              lineNumber: 2,
              order: 2,
              text: "当前故障集中在 provider-session 解析。",
              timestamp: "2026-05-18T14:10:02Z",
              turnIndex: 2,
              type: "assistant_message",
            },
            {
              encrypted: true,
              lineNumber: 3,
              order: 3,
              sourceType: "reasoning",
              timestamp: "2026-05-18T14:10:03Z",
              turnIndex: 2,
              type: "reasoning",
            },
            {
              callId: "call_exec_1",
              lineNumber: 4,
              name: "exec_command",
              order: 4,
              output: execCommandOutput,
              status: "completed",
              timestamp: "2026-05-18T14:10:04Z",
              turnIndex: 2,
              type: "tool_output",
            },
            {
              lineNumber: 5,
              order: 5,
              sourceType: "task_started",
              summary: "重试已计划。",
              timestamp: "2026-05-18T14:10:05Z",
              turnIndex: 2,
              type: "system_event",
            },
          ],
        }),
      ),
    );

    renderWithQueryClient(
      <ProviderSessionDetailPanel
        locale="zh-CN"
        selectedProviderSession={SELECTED_SESSION}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "会话记录" })).toBeTruthy();
    });

    expandDisclosure("展开会话记录");
    expandDisclosure("展开会话分析");
    expandDisclosure("展开维护诊断");

    const headingNames = screen
      .getAllByRole("heading")
      .map((heading) => heading.textContent ?? "");
    const transcriptIndex = headingNames.indexOf("会话记录");
    const analysisIndex = headingNames.indexOf("会话分析");

    expect(transcriptIndex).toBeGreaterThan(-1);
    expect(analysisIndex).toBeGreaterThan(transcriptIndex);
    expect(screen.getByText("会话 ID")).toBeTruthy();
    expect(screen.getAllByText("用户").length).toBeGreaterThan(0);
    expect(screen.getAllByText("助手").length).toBeGreaterThan(0);
    expect(screen.getAllByText("call_exec_1").length).toBeGreaterThan(0);
    expandDisclosure("展开助手");
    expandDisclosure("展开推理");
    expandDisclosure("展开call_exec_1");
    expect(screen.getAllByText("加密推理").length).toBeGreaterThan(0);
    expect(
      screen.getAllByText(
        "此步骤确实发生了推理，但明文内容会被有意隐藏，无法直接查看。",
      ).length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("命令结果")).toBeTruthy();
    expect(screen.getByText("退出代码")).toBeTruthy();
    expect(screen.getByText("0.4821秒钟")).toBeTruthy();
    expect(screen.getByText("命令执行成功")).toBeTruthy();
    expect(screen.getByText("unexpected end of JSON input")).toBeTruthy();
    expect(screen.getByText("第 7 行的未知事件")).toBeTruthy();
    expect(
      screen
        .getAllByTitle("2026-05-18T14:10:04Z")
        .some(
          (element) =>
            element.textContent ===
            formatDateTime("2026-05-18T14:10:04Z", "zh-CN"),
        ),
    ).toBe(true);

    expect(screen.queryByText("2026-05-18T14:10:04Z")).toBeNull();

    expect(
      screen.getByText((content) => content.includes("Chunk ID: exec-789")),
    ).toBeTruthy();
    expect(screen.getAllByText("命令执行成功").length).toBeGreaterThan(0);
  });
});

function buildProviderSessionDetailResponse(
  overrides: Partial<ProviderSessionDetailResponse> & {
    parse: Partial<ProviderSessionDetailResponse["parse"]>;
  },
): ProviderSessionDetailResponse {
  return {
    ...overrides,
    parse: {
      eventCount: 1,
      functionCalls: [],
      lineCount: 1,
      malformedLineCount: 0,
      parseErrors: [],
      reasoning: [],
      turns: [],
      unknownEventCount: 0,
      unknownEvents: [],
      ...overrides.parse,
    },
    providerSession: {
      id: SELECTED_SESSION.id,
      kind: SELECTED_SESSION.kind,
      provider: SELECTED_SESSION.provider,
      ...overrides.providerSession,
    },
    source: {
      relativePath: "2026/05/18/rollout-sess_active.jsonl",
      sizeBytes: 2048,
      ...overrides.source,
    },
    transcript: overrides.transcript ?? [
      {
        order: 1,
        text: "Summarize the failing provider session.",
        turnIndex: 1,
        type: "user_message",
      },
    ],
  };
}

function jsonResponse(body: ProviderSessionDetailResponse) {
  return new Response(JSON.stringify(body), {
    headers: {
      "Content-Type": "application/json",
    },
    status: 200,
    statusText: "OK",
  });
}

function renderWithQueryClient(view: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{view}</QueryClientProvider>,
  );
}
