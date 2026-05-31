import { QueryClient } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";

import { FACTORY_EVENT_TYPES } from "../../../api/events";
import {
  type CurrentFactoryDocument,
  CURRENT_FACTORY_EDITOR_SAVE_MODE,
  getCurrentFactoryDocument,
  saveFactoryForSessionDocument,
} from "../../../api/current-factory-definition";
import { syncCurrentFactoryDefinition } from "../../dashboard/lib/dashboard-event-stream";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "./useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "./useFactoryDocumentSave";
import {
  createFactoryDocumentSaveQueryClientWrapper,
  editableFactoryDefinition,
} from "./useFactoryDocumentSave.test-helpers";

vi.mock("../../../api/current-factory-definition", async () => {
  const actual = await vi.importActual(
    "../../../api/current-factory-definition",
  );

  return {
    ...actual,
    getCurrentFactoryDocument: vi.fn(),
    saveFactoryForSessionDocument: vi.fn(),
  };
});

beforeEach(() => {
  vi.mocked(getCurrentFactoryDocument).mockReset();
  vi.mocked(saveFactoryForSessionDocument).mockReset();
  useDashboardSessionStore.setState({ selectedSessionID: "~default" });
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: cache and transport cases share one query-client harness.
describe("useFactoryDocumentSave", () => {
  it("saves with REPLACE_CURRENT by default and updates both query caches", async () => {
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
      version: {
        logical: "8",
        physical: "2026-05-27T08:00:00Z",
      },
    };
    vi.mocked(saveFactoryForSessionDocument).mockResolvedValue(savedDocument);
    useDashboardSessionStore.setState({ selectedSessionID: "session-2" });

    const { result } = renderHook(
      () => ({
        definitionKey: currentFactoryDefinitionQueryKey("session-2"),
        documentKey: currentFactoryDocumentQueryKey("session-2"),
        save: useFactoryDocumentSave(),
      }),
      {
        wrapper: createFactoryDocumentSaveQueryClientWrapper(queryClient),
      },
    );

    await result.current.save.saveAsync({
      baseVersion: {
        logical: "7",
        physical: "2026-05-27T07:59:00Z",
      },
      factory: editableFactoryDefinition,
    });

    expect(saveFactoryForSessionDocument).toHaveBeenCalledWith(
      {
        baseVersion: {
          logical: "7",
          physical: "2026-05-27T07:59:00Z",
        },
        factoryDefinition: editableFactoryDefinition,
        mode: CURRENT_FACTORY_EDITOR_SAVE_MODE,
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

  it("uses an explicit sessionID override for transport and cache keys", async () => {
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
      version: {
        logical: "3",
        physical: "2026-05-27T09:00:00Z",
      },
    };
    vi.mocked(saveFactoryForSessionDocument).mockResolvedValue(savedDocument);
    useDashboardSessionStore.setState({ selectedSessionID: "session-default" });

    const { result } = renderHook(
      () => ({
        definitionKey: currentFactoryDefinitionQueryKey("session-override"),
        documentKey: currentFactoryDocumentQueryKey("session-override"),
        save: useFactoryDocumentSave(),
      }),
      {
        wrapper: createFactoryDocumentSaveQueryClientWrapper(queryClient),
      },
    );

    await result.current.save.saveAsync({
      factory: editableFactoryDefinition,
      sessionID: "session-override",
    });

    expect(saveFactoryForSessionDocument).toHaveBeenCalledWith(
      expect.objectContaining({
        factoryDefinition: editableFactoryDefinition,
        mode: CURRENT_FACTORY_EDITOR_SAVE_MODE,
      }),
      {
        sessionID: "session-override",
      },
    );
    expect(queryClient.getQueryData(result.current.documentKey)).toEqual(
      savedDocument,
    );
    expect(queryClient.getQueryData(result.current.definitionKey)).toEqual(
      savedDocument,
    );
  });

  it("passes an explicit save mode to the session factory adapter", async () => {
    vi.mocked(saveFactoryForSessionDocument).mockResolvedValue({
      ...editableFactoryDefinition,
      version: {
        logical: "2",
        physical: "2026-05-27T10:00:00Z",
      },
    });

    const { result } = renderHook(() => useFactoryDocumentSave(), {
      wrapper: createFactoryDocumentSaveQueryClientWrapper(),
    });

    await result.current.saveAsync({
      factory: editableFactoryDefinition,
      mode: "UPSERT_NAMED_AND_ACTIVATE",
    });

    expect(saveFactoryForSessionDocument).toHaveBeenCalledWith(
      expect.objectContaining({
        mode: "UPSERT_NAMED_AND_ACTIVATE",
      }),
      expect.any(Object),
    );
  });

  it("does not mutate the caller factory object when the adapter adjusts version", async () => {
    const factoryBeforeSave = {
      ...editableFactoryDefinition,
      version: {
        logical: "1",
        physical: "2026-05-27T06:00:00Z",
      },
    };
    const factorySnapshot = structuredClone(factoryBeforeSave);
    vi.mocked(saveFactoryForSessionDocument).mockImplementation(
      async (input) => {
        input.factoryDefinition.version = {
          logical: "99",
          physical: "2026-05-27T11:00:00Z",
        };

        return {
          ...input.factoryDefinition,
          version: input.factoryDefinition.version,
        };
      },
    );

    const { result } = renderHook(() => useFactoryDocumentSave(), {
      wrapper: createFactoryDocumentSaveQueryClientWrapper(),
    });

    await result.current.saveAsync({
      factory: factoryBeforeSave,
    });

    expect(factoryBeforeSave).toEqual(factorySnapshot);
  });

  it("converges document cache on FACTORY_CHANGE with version without a document GET", async () => {
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
      version: {
        logical: "8",
        physical: "2026-05-27T08:00:00Z",
      },
    };
    vi.mocked(saveFactoryForSessionDocument).mockResolvedValue(savedDocument);
    useDashboardSessionStore.setState({ selectedSessionID: "session-2" });

    const { result } = renderHook(
      () => ({
        documentKey: currentFactoryDocumentQueryKey("session-2"),
        save: useFactoryDocumentSave(),
      }),
      {
        wrapper: createFactoryDocumentSaveQueryClientWrapper(queryClient),
      },
    );

    await result.current.save.saveAsync({
      baseVersion: {
        logical: "7",
        physical: "2026-05-27T07:59:00Z",
      },
      factory: editableFactoryDefinition,
    });

    expect(getCurrentFactoryDocument).not.toHaveBeenCalled();

    syncCurrentFactoryDefinition(
      queryClient,
      {
        context: { eventTime: "2026-05-27T08:00:01Z", sequence: 9, tick: 9 },
        id: "factory-event/factory-change/9",
        payload: {
          factory: {
            ...editableFactoryDefinition,
            version: {
              logical: "9",
              physical: "2026-05-27T08:00:01Z",
            },
          },
        },
        type: FACTORY_EVENT_TYPES.factoryChange,
      },
      "session-2",
    );

    expect(getCurrentFactoryDocument).not.toHaveBeenCalled();
    expect(queryClient.getQueryData(result.current.documentKey)).toMatchObject({
      version: {
        logical: "9",
        physical: "2026-05-27T08:00:01Z",
      },
    });
  });
});
