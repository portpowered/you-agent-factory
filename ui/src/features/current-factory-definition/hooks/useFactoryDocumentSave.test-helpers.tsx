import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { DashboardSessionProvider } from "../../dashboard/session/dashboard-session-provider";

export const editableFactoryDefinition: CanonicalFactoryDefinition = {
  name: "Current Factory",
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workstations: [
    {
      body: "Summarize before review.",
      inputs: [
        {
          state: "queued",
          workType: "task",
        },
      ],
      name: "Draft",
      outputs: [
        {
          state: "reviewed",
          workType: "task",
        },
      ],
      promptFile: "prompts/draft.md",
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
  workTypes: [],
};

export function createFactoryDocumentSaveQueryClientWrapper(
  queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: 0,
        retry: false,
      },
    },
  }),
) {
  return function QueryClientWrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <DashboardSessionProvider>{children}</DashboardSessionProvider>
      </QueryClientProvider>
    );
  };
}
