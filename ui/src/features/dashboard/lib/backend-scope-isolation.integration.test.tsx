import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { vi } from "vitest";

import {
  type CurrentFactoryDocument,
  getCurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { DashboardSessionStoreTestProvider } from "../../../testing/dashboard-session-test-provider";
import {
  currentFactoryDefinitionQueryKey,
  useCurrentFactoryDefinition,
} from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { StreamDerivedCacheIdentity } from "../../timeline/public/stream-identity";
import { useDashboardSessionLifecycle } from "../hooks/useDashboardSessionLifecycle";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";

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
const RESOLVED_DEFAULT_SESSION_UUID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";

function streamIdentityForScope(
  backendScopeID: string,
): StreamDerivedCacheIdentity {
  return {
    backendScopeID,
    factorySessionID: RESOLVED_DEFAULT_SESSION_UUID,
    logicalSessionKeyID: "logical-default",
    streamGenerationID: "2026-06-26T00:00:00Z",
  };
}

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
        <DashboardSessionStoreTestProvider>
          {children}
        </DashboardSessionStoreTestProvider>
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
    useDashboardStreamStore.getState().resetStreamState();
  });

  it("resolves runtime queries from separate namespaces per backendScopeID", async () => {
    const streamIdentityA = streamIdentityForScope(BACKEND_SCOPE_A);
    const streamIdentityB = streamIdentityForScope(BACKEND_SCOPE_B);

    vi.mocked(getCurrentFactoryDocument)
      .mockResolvedValueOnce(factoryDocumentForScope(BACKEND_SCOPE_A))
      .mockResolvedValueOnce(factoryDocumentForScope(BACKEND_SCOPE_B));

    useDashboardStreamStore.setState({
      backendRuntimeCacheScope: BACKEND_SCOPE_A,
      resolvedStreamIdentity: streamIdentityA,
    });

    const { result, rerender } = renderHook(
      () => useCurrentFactoryDefinition(),
      {
        wrapper: createWrapper(queryClient),
      },
    );

    await waitFor(() => {
      expect(result.current.data?.workers[0]?.name).toBe(
        `${BACKEND_SCOPE_A}-worker`,
      );
    });
    expect(
      queryClient.getQueryData(
        currentFactoryDefinitionQueryKey(
          DEFAULT_FACTORY_SESSION_ID,
          streamIdentityA,
        ),
      ),
    ).toEqual(factoryDocumentForScope(BACKEND_SCOPE_A));

    act(() => {
      useDashboardStreamStore.setState({
        backendRuntimeCacheScope: BACKEND_SCOPE_B,
        resolvedStreamIdentity: streamIdentityB,
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
          streamIdentityB,
        ),
      ),
    ).toEqual(factoryDocumentForScope(BACKEND_SCOPE_B));
  });

  it("clears runtime-derived queries when backendRuntimeCacheScope changes", () => {
    const streamIdentityA = streamIdentityForScope(BACKEND_SCOPE_A);
    const streamIdentityB = streamIdentityForScope(BACKEND_SCOPE_B);

    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(
        DEFAULT_FACTORY_SESSION_ID,
        streamIdentityA,
      ),
      { workers: [{ name: "scope-a-only" }] },
    );
    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(
        DEFAULT_FACTORY_SESSION_ID,
        streamIdentityB,
      ),
      { workers: [{ name: "scope-b-only" }] },
    );

    useDashboardStreamStore.setState({
      backendRuntimeCacheScope: BACKEND_SCOPE_A,
      resolvedStreamIdentity: streamIdentityA,
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
        resolvedStreamIdentity: streamIdentityB,
      });
    });

    expect(
      queryClient.getQueryData(
        currentFactoryDefinitionQueryKey(
          DEFAULT_FACTORY_SESSION_ID,
          streamIdentityA,
        ),
      ),
    ).toBeUndefined();
    expect(
      queryClient.getQueryData(
        currentFactoryDefinitionQueryKey(
          DEFAULT_FACTORY_SESSION_ID,
          streamIdentityB,
        ),
      ),
    ).toBeUndefined();
  });
});
