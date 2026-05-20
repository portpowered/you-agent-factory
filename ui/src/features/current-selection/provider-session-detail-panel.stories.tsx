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
