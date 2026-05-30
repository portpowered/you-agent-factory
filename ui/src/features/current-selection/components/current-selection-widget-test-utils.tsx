import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, type RenderResult } from "@testing-library/react";
import type { ReactNode } from "react";

import { DashboardSessionTestProvider } from "../../../testing/dashboard-session-test-provider";

export function createCurrentSelectionWidgetQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  });
}

export function wrapCurrentSelectionWidgetView(
  queryClient: QueryClient,
  view: ReactNode,
): ReactNode {
  return (
    <QueryClientProvider client={queryClient}>
      <DashboardSessionTestProvider>{view}</DashboardSessionTestProvider>
    </QueryClientProvider>
  );
}

export function renderWithQueryClient(view: ReactNode): RenderResult {
  return renderWithExistingQueryClient(
    createCurrentSelectionWidgetQueryClient(),
    view,
  );
}

export function renderWithExistingQueryClient(
  queryClient: QueryClient,
  view: ReactNode,
): RenderResult {
  return render(view, {
    wrapper: ({ children }) =>
      wrapCurrentSelectionWidgetView(queryClient, children),
  });
}
