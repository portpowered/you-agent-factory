// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction lint/nursery/noExcessiveLinesPerFile: existing provider-session detail coverage stayed intact during sibling-feature extraction.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
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

  it("shows an explicit loading state while session details are being fetched", () => {
    vi.mocked(globalThis.fetch).mockReturnValue(new Promise(() => undefined));

    renderWithQueryClient(
      <ProviderSessionDetailPanel selectedProviderSession={SELECTED_SESSION} />,
    );

    expect(screen.getByRole("status").textContent).toContain(
      "Loading session details...",
    );
    expect(screen.getByText("dispatch-review-active")).toBeTruthy();
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

    expect(screen.getByRole("status").textContent).toContain("正在加载会话详情...");

    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toContain(
        "无法在已配置的 Codex sessions 目录下找到所选 provider-session 文件。",
      );
    });
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
        "The selected provider-session file could not be found under the configured Codex sessions directory.",
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
        "The selected session file did not contain any Codex event records.",
      );
    });
    expect(screen.getByText("2026/05/18/rollout-sess_active.jsonl")).toBeTruthy();
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
        "The selected session file could not be parsed into Codex events. Review the malformed-line diagnostics below.",
      );
    });
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
    expect(screen.getByRole("heading", { name: "Parse diagnostics" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Token usage" })).toBeTruthy();
    expect(screen.getByText("Unknown event on line 1")).toBeTruthy();
    expect(screen.getByText("Unknown event on line 2")).toBeTruthy();
  });

  it("renders parsed source metadata, turns, function calls, reasoning, token usage, and diagnostics", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 6,
            functionCalls: [
              {
                arguments: "{\"path\":\"pkg/api\"}",
                callId: "call_1",
                name: "list_dir",
                order: 1,
                output: "{\"entries\":3}",
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
      expect(screen.getByRole("heading", { name: "Token usage" })).toBeTruthy();
    });

    expect(screen.getByText("2026/05/18/rollout-sess_active.jsonl")).toBeTruthy();
    expect(screen.getByText("Turn 1")).toBeTruthy();
    expect(screen.getByText("list_dir")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Reasoning" })).toBeTruthy();
    expect(screen.getByText("182")).toBeTruthy();
    expect(screen.getByText("unexpected end of JSON input")).toBeTruthy();
    expect(screen.getByText("Unknown event on line 6")).toBeTruthy();
  });

  it("renders chronological transcript entries with expandable tool payloads", async () => {
    const longArguments =
      "{\"path\":\"pkg/api/provider_session_details.go\",\"mode\":\"diff\",\"note\":\"" +
      "x".repeat(360) +
      "\"}";
    const longOutput =
      "{\"summary\":\"provider session parsed\",\"details\":\"" +
      "y".repeat(360) +
      "\"}";

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

    expect(screen.getByText("Summarize the rollout state for this work item.")).toBeTruthy();
    expect(screen.getByText("The failing edge is in provider-session parsing.")).toBeTruthy();
    expect(screen.getByText("Encrypted reasoning content only.")).toBeTruthy();
    expect(screen.getByText("read_file")).toBeTruthy();
    expect(screen.getByText("call_tool_1")).toBeTruthy();
    expect(screen.getByText("Retry attempt scheduled.")).toBeTruthy();
    expect(
      screen.getByText("Order 4 / Turn 1 / 2026-05-18T14:10:04Z / Session line 4"),
    ).toBeTruthy();

    const expandArguments = screen.getByRole("button", {
      name: "Expand Arguments",
    });
    expect(expandArguments.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByText(`${longArguments.slice(0, 320)}…`)).toBeTruthy();

    fireEvent.click(expandArguments);

    expect(screen.getByRole("button", { name: "Collapse Arguments" }).getAttribute("aria-expanded")).toBe(
      "true",
    );
    expect(screen.getByText(longArguments)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Expand Output" }));
    expect(screen.getByText(longOutput)).toBeTruthy();
  });

  it("renders the transcript before summary sections in the success state", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 4,
            functionCalls: [
              {
                arguments: "{\"path\":\"pkg/api\"}",
                callId: "call_1",
                name: "list_dir",
                order: 3,
                output: "{\"entries\":3}",
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

    const headingNames = screen
      .getAllByRole("heading")
      .map((heading) => heading.textContent ?? "");
    const transcriptIndex = headingNames.indexOf("Transcript");
    const supportingHeadings = [
      "Token usage",
      "Turns",
      "Function calls",
      "Reasoning",
      "Parse diagnostics",
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
    const reasoningSummary = "Inspect the provider-session diff before retrying.";
    const reasoningText = "Investigate the failed response stream.";
    const callArguments = "{\"path\":\"pkg/api/provider_session_details.go\"}";
    const callOutput = "{\"lines\":128}";

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
      expect(screen.getByRole("heading", { name: "Reasoning" })).toBeTruthy();
    });

    expect(screen.getAllByText(reasoningSummary)).toHaveLength(1);
    expect(screen.getAllByText(reasoningText)).toHaveLength(1);
    expect(screen.getAllByText(callArguments)).toHaveLength(1);
    expect(screen.getAllByText(callOutput)).toHaveLength(1);
    expect(screen.getAllByText("read_file")).toHaveLength(2);
    expect(screen.getAllByText("Order 4 / Turn 1").length).toBeGreaterThan(0);
  });

  it("renders provider-session detail labels and templates from the zh-CN catalog", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse(
        buildProviderSessionDetailResponse({
          parse: {
            eventCount: 2,
            functionCalls: [
              {
                arguments: "{\"path\":\"pkg/api\"}",
                callId: null,
                name: "list_dir",
                order: 3,
                output: "{\"entries\":3}",
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
      expect(screen.getByRole("heading", { name: "Token 用量" })).toBeTruthy();
    });

    expect(screen.getByText("输入")).toBeTruthy();
    expect(screen.getByText("缓存输入")).toBeTruthy();
    expect(screen.getByText("轮次 2")).toBeTruthy();
    expect(screen.getByText("无时间戳")).toBeTruthy();
    expect(screen.getAllByText("顺序 3 / 轮次 2").length).toBeGreaterThan(0);
    expect(screen.getByText("调用 ID")).toBeTruthy();
    expect(screen.getAllByText("不可用").length).toBeGreaterThan(0);
    expect(screen.getByText("仅包含加密的推理内容。")).toBeTruthy();
    expect(screen.getByText("第 3 行")).toBeTruthy();
    expect(screen.getByText("第 2 行的未知事件")).toBeTruthy();
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
