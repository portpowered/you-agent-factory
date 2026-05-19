import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import type { ProviderSessionDetailResponse } from "../../api/provider-session-details";
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

    expect(screen.getByText("Loading session details...")).toBeTruthy();
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

    expect(screen.getByText("正在加载会话详情...")).toBeTruthy();

    await waitFor(() => {
      expect(
        screen.getByText(
          "无法在已配置的 Codex sessions 目录下找到所选 provider-session 文件。",
        ),
      ).toBeTruthy();
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
      expect(
        screen.getByText(
          "The selected provider-session file could not be found under the configured Codex sessions directory.",
        ),
      ).toBeTruthy();
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
      expect(
        screen.getByText(
          "The selected session file did not contain any Codex event records.",
        ),
      ).toBeTruthy();
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
      expect(
        screen.getByText(
          "The selected session file could not be parsed into Codex events. Review the malformed-line diagnostics below.",
        ),
      ).toBeTruthy();
    });
    expect(
      screen.getByText("invalid character '}' after object key:value pair"),
    ).toBeTruthy();
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
    expect(
      screen.getByText("Inspect the provider-session diff before retrying."),
    ).toBeTruthy();
    expect(screen.getByText("182")).toBeTruthy();
    expect(screen.getByText("unexpected end of JSON input")).toBeTruthy();
    expect(screen.getByText("Unknown event on line 6")).toBeTruthy();
  });
});

function buildProviderSessionDetailResponse(
  overrides: Partial<ProviderSessionDetailResponse> & {
    parse: Partial<ProviderSessionDetailResponse["parse"]>;
  },
): ProviderSessionDetailResponse {
  return {
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
