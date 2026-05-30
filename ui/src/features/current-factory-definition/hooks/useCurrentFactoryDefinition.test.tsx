import "../../../../testing/bun-current-factory-definition-api-mocks";
import { beforeEach, describe, expect, it } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import {
  getCurrentFactoryDefinitionMock,
  getCurrentFactoryDocumentMock,
  saveCurrentFactoryDocumentMock,
} from "../../../../testing/bun-current-factory-definition-api-mocks";
import { DashboardSessionProvider } from "../../dashboard/session/dashboard-session-provider";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";

const {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
  useCurrentFactoryDefinition,
  useCurrentFactoryDocument,
  useSaveCurrentFactory,
} = await import("./useCurrentFactoryDefinition");

const editableFactoryDefinition: CanonicalFactoryDefinition = {
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

beforeEach(() => {
  getCurrentFactoryDefinitionMock.mockReset();
  getCurrentFactoryDocumentMock.mockReset();
  saveCurrentFactoryDocumentMock.mockReset();
  useDashboardSessionStore.setState({ selectedSessionID: "~default" });
});

describe("useCurrentFactoryDefinition", () => {
  it("does not fetch while workstation editing is disabled", () => {
    const { result } = renderHook(() => useCurrentFactoryDefinition(false), {
      wrapper: createQueryClientWrapper(),
    });

    expect(getCurrentFactoryDefinitionMock).not.toHaveBeenCalled();
    expect(result.current).toMatchObject({
      data: undefined,
      error: null,
      isFetching: false,
      isPending: true,
      status: "pending",
    });
  });

  it("returns the validated editable current factory definition on success", async () => {
    getCurrentFactoryDefinitionMock.mockResolvedValue(editableFactoryDefinition);

    const { result } = renderHook(() => useCurrentFactoryDefinition(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        data: editableFactoryDefinition,
        error: null,
        isPending: false,
        status: "success",
      });
    });
  });

  it("loads the selected non-default session definition instead of the default alias", async () => {
    useDashboardSessionStore.setState({ selectedSessionID: "session-2" });
    getCurrentFactoryDefinitionMock.mockResolvedValue(editableFactoryDefinition);

    renderHook(() => useCurrentFactoryDefinition(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(getCurrentFactoryDefinitionMock).toHaveBeenCalledWith({
        sessionID: "session-2",
      });
    });
  });

  it("refetches when the selected session tab changes", async () => {
    getCurrentFactoryDefinitionMock.mockResolvedValue(editableFactoryDefinition);

    const { rerender } = renderHook(() => useCurrentFactoryDefinition(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(getCurrentFactoryDefinitionMock).toHaveBeenCalledWith({
        sessionID: "~default",
      });
    });

    getCurrentFactoryDefinitionMock.mockClear();

    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID("session-2");
    });

    rerender();

    await waitFor(() => {
      expect(getCurrentFactoryDefinitionMock).toHaveBeenCalledWith({
        sessionID: "session-2",
      });
    });
  });

  it("exposes actionable typed errors when the current definition is not editable", async () => {
    getCurrentFactoryDefinitionMock.mockRejectedValue({
      code: "INVALID_FACTORY_DEFINITION",
      message: "The current factory definition is malformed.",
      name: "CurrentFactoryDefinitionError",
    });

    const { result } = renderHook(() => useCurrentFactoryDefinition(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        data: undefined,
        error: {
          code: "INVALID_FACTORY_DEFINITION",
          message: "The current factory definition is malformed.",
        },
        isPending: false,
        status: "error",
      });
    });
  });

  it("returns the editable current-factory document with version metadata", async () => {
    const editableFactoryDefinitionDocument: CurrentFactoryDocument = {
      ...editableFactoryDefinition,
      version: {
        logical: "4",
        physical: "2026-05-18T14:48:00Z",
      },
    };
    getCurrentFactoryDocumentMock.mockResolvedValue(
      editableFactoryDefinitionDocument,
    );

    const { result } = renderHook(() => useCurrentFactoryDocument(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        data: editableFactoryDefinitionDocument,
        error: null,
        isPending: false,
        status: "success",
      });
    });
  });

  it("loads the editable document for the selected non-default session", async () => {
    useDashboardSessionStore.setState({ selectedSessionID: "session-2" });
    getCurrentFactoryDocumentMock.mockResolvedValue({
      ...editableFactoryDefinition,
      version: {
        logical: "5",
        physical: "2026-05-18T14:49:00Z",
      },
    });

    renderHook(() => useCurrentFactoryDocument(), {
      wrapper: createQueryClientWrapper(),
    });

    await waitFor(() => {
      expect(getCurrentFactoryDocumentMock).toHaveBeenCalledWith({
        sessionID: "session-2",
      });
    });
  });
});

describe("useSaveCurrentFactory", () => {
  it("saves the editable current-factory document and refreshes both query caches", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          gcTime: 0,
          retry: false,
        },
      },
    });
    const savedDocument: CurrentFactoryDocument = {
      ...editableFactoryDefinition,
      metadata: {
        owner: "graph-editor",
      },
      version: {
        logical: "8",
        physical: "2026-05-27T08:00:00Z",
      },
    };
    saveCurrentFactoryDocumentMock.mockResolvedValue(savedDocument);
    useDashboardSessionStore.setState({ selectedSessionID: "session-2" });

    const { result } = renderHook(
      () => ({
        definitionKey: currentFactoryDefinitionQueryKey("session-2"),
        documentKey: currentFactoryDocumentQueryKey("session-2"),
        save: useSaveCurrentFactory(),
      }),
      {
        wrapper: createQueryClientWrapper(queryClient),
      },
    );

    await result.current.save.mutateAsync({
      baseVersion: {
        logical: "7",
        physical: "2026-05-27T07:59:00Z",
      },
      factoryDefinition: editableFactoryDefinition,
    });

    expect(saveCurrentFactoryDocumentMock).toHaveBeenCalledWith(
      {
        baseVersion: {
          logical: "7",
          physical: "2026-05-27T07:59:00Z",
        },
        factoryDefinition: editableFactoryDefinition,
      },
      {
        sessionID: "session-2",
      },
    );
    expect(queryClient.getQueryData(result.current.documentKey)).toEqual(
      savedDocument,
    );
    expect(queryClient.getQueryData(result.current.definitionKey)).toEqual(
      savedDocument,
    );
  });

  it("preserves current-factory save API errors through the mutation", async () => {
    const error = {
      code: "STALE_FACTORY_VERSION",
      message: "The editable definition is stale.",
      name: "CurrentFactoryDefinitionError",
    };
    saveCurrentFactoryDocumentMock.mockRejectedValue(error);

    const { result } = renderHook(() => useSaveCurrentFactory(), {
      wrapper: createQueryClientWrapper(),
    });

    await expect(
      result.current.mutateAsync({
        baseVersion: {
          logical: "7",
          physical: "2026-05-27T07:59:00Z",
        },
        factoryDefinition: editableFactoryDefinition,
      }),
    ).rejects.toEqual(error);
  });
});

function createQueryClientWrapper(
  queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: 0,
        retry: false,
      },
    },
  }),
): ({ children }: { children: ReactNode }) => ReactNode {
  return function QueryClientWrapper({
    children,
  }: {
    children: ReactNode;
  }): ReactNode {
    return (
      <QueryClientProvider client={queryClient}>
        <DashboardSessionProvider>{children}</DashboardSessionProvider>
      </QueryClientProvider>
    );
  };
}
