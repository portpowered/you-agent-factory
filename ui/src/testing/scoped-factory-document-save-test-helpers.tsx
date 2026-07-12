import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import type { ScopedFactoryDocumentSaveRequest } from "../features/current-selection/base/hooks/useScopedFactoryDocumentSave";
import { useDashboardSessionStore } from "../features/dashboard/state/dashboardSessionStore";
import { DashboardSessionStoreTestProvider } from "./dashboard-session-test-provider";

export const defaultScopedFactoryDocumentSaveRequest: ScopedFactoryDocumentSaveRequest =
  {
    baseVersion: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    factory: {
      name: "Current Factory",
      workers: [],
      workstations: [],
    },
    previousFactory: {
      name: "Current Factory",
      workers: [],
      workstations: [],
    },
    scopeKey: "review:transition:Review",
  };

export function seedScopedFactoryDocumentSaveTestSession() {
  useDashboardSessionStore.setState({ selectedSessionID: "~default" });
}

export function createScopedFactoryDocumentSaveQueryClientWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });

  return function ScopedFactoryDocumentSaveQueryClientWrapper({
    children,
  }: {
    children: ReactNode;
  }) {
    return (
      <QueryClientProvider client={queryClient}>
        <DashboardSessionStoreTestProvider>
          {children}
        </DashboardSessionStoreTestProvider>
      </QueryClientProvider>
    );
  };
}
