import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";

import {
  DashboardSessionTestProvider,
  type DashboardSessionTestProviderProps,
} from "./dashboard-session-test-provider";

export function wrapWithDashboardSessionTest(
  children: ReactNode,
  options: Omit<DashboardSessionTestProviderProps, "children"> = {},
): ReactElement {
  return (
    <DashboardSessionTestProvider {...options}>
      {children}
    </DashboardSessionTestProvider>
  );
}

export function renderWithDashboardSessionTest(
  ui: ReactElement,
  options: Omit<DashboardSessionTestProviderProps, "children"> = {},
): ReturnType<typeof render> {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <DashboardSessionTestProvider {...options}>
        {ui}
      </DashboardSessionTestProvider>
    </QueryClientProvider>,
  );
}
