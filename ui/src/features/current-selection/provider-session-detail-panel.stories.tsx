import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { ProviderSessionDetailPanel } from "./components/provider-session-detail-panel";

const providerSessionVerificationSessionID =
  "019e44f4-580e-7f32-981e-1e54ec6907d6";
const selectedProviderSession = {
  dispatchID: "dispatch-review-active",
  id: providerSessionVerificationSessionID,
  kind: "session_id",
  provider: "codex",
} as const;

export default {
  title: "Infinite You/Current Selection/Provider Session Detail Panel",
  component: ProviderSessionDetailPanel,
};

export const TimestampPrefixedSessionSuccess = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/provider-sessions/detail?id=${providerSessionVerificationSessionID}&kind=session_id&provider=codex`,
          response: {
            body: {
              parse: {
                eventCount: 3,
                functionCalls: [],
                lineCount: 3,
                malformedLineCount: 0,
                parseErrors: [],
                reasoning: [],
                tokenUsage: {
                  cachedInputTokens: 0,
                  inputTokens: 18,
                  outputTokens: 9,
                  reasoningOutputTokens: 0,
                  totalTokens: 27,
                },
                turns: [
                  {
                    eventCount: 3,
                    functionCallCount: 0,
                    index: 1,
                    reasoningCount: 0,
                    responseItemCount: 1,
                    startedAt: "2026-05-20T17:35:24Z",
                  },
                ],
                unknownEventCount: 0,
                unknownEvents: [],
              },
              providerSession: {
                id: providerSessionVerificationSessionID,
                kind: "session_id",
                provider: "codex",
              },
              source: {
                modifiedAt: "2026-05-20T17:35:24Z",
                relativePath:
                  "2026/05/20/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl",
                sizeBytes: 2048,
              },
              transcript: [
                {
                  order: 1,
                  text: "Summarize the rollout state for this work item.",
                  turnIndex: 1,
                  type: "user_message",
                },
              ],
            },
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          gcTime: Infinity,
          retry: false,
        },
      },
    });

    return (
      <div style={{ maxWidth: "100%", width: "960px" }}>
        <QueryClientProvider client={queryClient}>
          <ProviderSessionDetailPanel
            selectedProviderSession={selectedProviderSession}
          />
        </QueryClientProvider>
      </div>
    );
  },
};

export const MixedTranscript = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/provider-sessions/detail?id=${providerSessionVerificationSessionID}&kind=session_id&provider=codex`,
          response: {
            body: {
              parse: {
                eventCount: 6,
                functionCalls: [
                  {
                    arguments: "{\"path\":\"pkg/api/provider_session_details.go\"}",
                    callId: "call_1",
                    name: "read_file",
                    order: 4,
                    output: "{\"lines\":128}",
                    status: "completed",
                    turnIndex: 1,
                    type: "function_call",
                  },
                ],
                lineCount: 6,
                malformedLineCount: 0,
                parseErrors: [],
                reasoning: [
                  {
                    order: 3,
                    sourceType: "reasoning",
                    summary: "Inspect the parser branch before retrying.",
                    turnIndex: 1,
                  },
                ],
                tokenUsage: {
                  cachedInputTokens: 0,
                  inputTokens: 32,
                  outputTokens: 18,
                  reasoningOutputTokens: 6,
                  totalTokens: 56,
                },
                turns: [
                  {
                    eventCount: 6,
                    functionCallCount: 1,
                    index: 1,
                    reasoningCount: 1,
                    responseItemCount: 4,
                    startedAt: "2026-05-20T17:35:24Z",
                  },
                ],
                unknownEventCount: 0,
                unknownEvents: [],
              },
              providerSession: {
                id: providerSessionVerificationSessionID,
                kind: "session_id",
                provider: "codex",
              },
              source: {
                modifiedAt: "2026-05-20T17:35:24Z",
                relativePath:
                  "2026/05/20/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl",
                sizeBytes: 4096,
              },
              transcript: [
                {
                  lineNumber: 1,
                  order: 1,
                  text: "Summarize the rollout state for this work item.",
                  timestamp: "2026-05-20T17:35:24Z",
                  turnIndex: 1,
                  type: "user_message",
                },
                {
                  lineNumber: 2,
                  order: 2,
                  text: "The current failure is isolated to provider-session parsing.",
                  timestamp: "2026-05-20T17:35:25Z",
                  turnIndex: 1,
                  type: "assistant_message",
                },
                {
                  lineNumber: 3,
                  order: 3,
                  sourceType: "reasoning",
                  summary: "Inspect the parser branch before retrying.",
                  timestamp: "2026-05-20T17:35:26Z",
                  turnIndex: 1,
                  type: "reasoning",
                },
                {
                  arguments: "{\"path\":\"pkg/api/provider_session_details.go\"}",
                  callId: "call_1",
                  lineNumber: 4,
                  name: "read_file",
                  order: 4,
                  status: "completed",
                  timestamp: "2026-05-20T17:35:27Z",
                  turnIndex: 1,
                  type: "tool_call",
                },
                {
                  callId: "call_1",
                  lineNumber: 5,
                  order: 5,
                  output: "{\"lines\":128}",
                  status: "completed",
                  timestamp: "2026-05-20T17:35:28Z",
                  turnIndex: 1,
                  type: "tool_output",
                },
                {
                  lineNumber: 6,
                  order: 6,
                  sourceType: "task_started",
                  summary: "Retry attempt scheduled.",
                  timestamp: "2026-05-20T17:35:29Z",
                  turnIndex: 1,
                  type: "system_event",
                },
              ],
            },
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: TimestampPrefixedSessionSuccess.render,
};
