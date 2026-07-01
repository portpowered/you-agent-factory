import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { vi } from "vitest";

import {
  getCurrentFactoryDocument,
  type CurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  currentFactoryDefinitionQueryKey,
  useCurrentFactoryDefinition,
} from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { DashboardSessionProvider } from "../session/dashboard-session-provider";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";
import { useDashboardSessionLifecycle } from "../hooks/useDashboardSessionLifecycle";

vi.mock("../../../api/current-factory-definition", async () => {
  const actual = await vi.importActual(
    "../../../api/current-factory-definition",
  );

  return {
    ...actual,
    getCurrentFactoryDocument: vi.fn(),
  };
});

const BACKEND_SCOPE_A = "backend-scope-a";
const BACKEND_SCOPE_B = "backend-scope-b";

function factoryDocumentForScope(scopeLabel: string): CurrentFactoryDocument {
  return {
    name: scopeLabel,
    version: { logical: "1", physical: "2026-06-26T00:00:00Z" },
    workers: [{ name: `${scopeLabel}-worker` }],
    workstations: [],
    workTypes: [],
  };
}

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        <DashboardSessionProvider>{children}</DashboardSessionProvider>
      </QueryClientProvider>
    );
  };
}

describe("backend scope isolation integration", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.mocked(getCurrentFactoryDocument).mockReset();
    queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    useDashboardStreamStore.setState({
      backendRuntimeCacheScope: null,
    });
  });

  it("resolves runtime queries from separate namespaces per backendScopeID", async () => {
    vi.mocked(getCurrentFactoryDocument)
      .mockResolvedValueOnce(factoryDocumentForScope(BACKEND_SCOPE_A))
      .mockResolvedValueOnce(factoryDocumentForScope(BACKEND_SCOPE_B));

    useDashboardStreamStore.setState({
      backendRuntimeCacheScope: BACKEND_SCOPE_A,
    });

    const { result, rerender } = renderHook(() => useCurrentFactoryDefinition(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(result.current.data?.workers[0]?.name).toBe(
        `${BACKEND_SCOPE_A}-worker`,
      );
    });
    expect(
      queryClient.getQueryData(
        currentFactoryDefinitionQueryKey(
          DEFAULT_FACTORY_SESSION_ID,
          BACKEND_SCOPE_A,
        ),
      ),
    ).toEqual(factoryDocumentForScope(BACKEND_SCOPE_A));

    act(() => {
      useDashboardStreamStore.setState({
        backendRuntimeCacheScope: BACKEND_SCOPE_B,
      });
    });
    rerender();

    await waitFor(() => {
      expect(result.current.data?.workers[0]?.name).toBe(
        `${BACKEND_SCOPE_B}-worker`,
      );
    });
    expect(getCurrentFactoryDocument).toHaveBeenCalledTimes(2);
    expect(result.current.data?.workers[0]?.name).not.toBe(
      `${BACKEND_SCOPE_A}-worker`,
    );
    expect(
      queryClient.getQueryData(
        currentFactoryDefinitionQueryKey(
          DEFAULT_FACTORY_SESSION_ID,
          BACKEND_SCOPE_B,
        ),
      ),
    ).toEqual(factoryDocumentForScope(BACKEND_SCOPE_B));
  });

  it("clears runtime-derived queries when backendRuntimeCacheScope changes", () => {
    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(DEFAULT_FACTORY_SESSION_ID, BACKEND_SCOPE_A),
      { workers: [{ name: "scope-a-only" }] },
    );
    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(DEFAULT_FACTORY_SESSION_ID, BACKEND_SCOPE_B),
      { workers: [{ name: "scope-b-only" }] },
    );

    useDashboardStreamStore.setState({
      backendRuntimeCacheScope: BACKEND_SCOPE_A,
    });

    renderHook(
      () =>
        useDashboardSessionLifecycle({
          sessionID: DEFAULT_FACTORY_SESSION_ID,
          refreshToken: 0,
        }),
      { wrapper: createWrapper(queryClient) },
    );

    act(() => {
      useDashboardStreamStore.setState({
        backendRuntimeCacheScope: BACKEND_SCOPE_B,
      });
    });

    expect(
      queryClient.getQueryData(
        currentFactoryDefinitionQueryKey(
          DEFAULT_FACTORY_SESSION_ID,
          BACKEND_SCOPE_A,
        ),
      ),
    ).toBeUndefined();
    expect(
      queryClient.getQueryData(
        currentFactoryDefinitionQueryKey(
          DEFAULT_FACTORY_SESSION_ID,
          BACKEND_SCOPE_B,
        ),
      ),
    ).toBeUndefined();
  });
});
