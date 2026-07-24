import { QueryClient } from "@tanstack/react-query";
import { act, waitFor } from "@testing-library/react";
import { vi } from "vitest";

import {
  type CurrentFactoryDocument,
  saveFactoryForSessionDocument,
} from "../../../api/current-factory-definition";
import {
  defaultGraphDocumentScopeKey,
  renderEditableFactoryGraphHook,
} from "../../../testing/editable-factory-graph-hook-test-helpers";
import {
  createHookTestGraphEditorDraftState,
  type MockGraphEditorDraftState,
} from "../../../testing/graph-editor-harness";
import { currentFactoryDocumentQueryKey } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { currentFactoryDocument } from "../lib/draft/factory-graph-draft.test-helpers";
import { createEmptyFactoryGraphDraft } from "../lib/draft/factory-graph-draft-types";

const hookState = vi.hoisted(() => ({
  draftState: {} as MockGraphEditorDraftState,
}));

vi.mock("./factory-graph-draft-hook", () => ({
  useFactoryGraphDraftState: () => hookState.draftState,
}));

vi.mock("../../../api/current-factory-definition", async () => {
  const actual = await vi.importActual(
    "../../../api/current-factory-definition",
  );

  return {
    ...actual,
    saveFactoryForSessionDocument: vi.fn(),
  };
});

beforeEach(() => {
  vi.mocked(saveFactoryForSessionDocument).mockReset();
  useDashboardSessionStore.setState({ selectedSessionID: "~default" });
  hookState.draftState = createHookTestGraphEditorDraftState();
  hookState.draftState.hasChanges = true;
  hookState.draftState.draft = {
    ...createEmptyFactoryGraphDraft(),
    additions: {
      ...createEmptyFactoryGraphDraft().additions,
      resources: [
        {
          capacity: 1,
          name: "review-slot",
        },
      ],
    },
  };
});

describe("useEditableFactoryGraph save query cache", () => {
  it("updates the current factory document query cache exactly once on successful save", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    const documentQueryKey = currentFactoryDocumentQueryKey("~default");
    const savedDocument: CurrentFactoryDocument = {
      ...currentFactoryDocument,
      resources: [
        ...(currentFactoryDocument.resources ?? []),
        {
          capacity: 1,
          name: "review-slot",
        },
      ],
      version: {
        logical: "6",
        physical: "2026-05-31T12:00:00Z",
      },
    };
    vi.mocked(saveFactoryForSessionDocument).mockResolvedValue(savedDocument);

    const setQueryData = vi.spyOn(queryClient, "setQueryData");
    const { result } = renderEditableFactoryGraphHook(
      {
        currentFactoryDocument: currentFactoryDocument,
        factoryDocumentScopeKey: defaultGraphDocumentScopeKey,
      },
      queryClient,
    );

    await act(async () => {
      await result.current.actions.save();
    });

    await waitFor(() => {
      expect(result.current.saveState.documentSave).toEqual({
        status: "success",
      });
    });

    const documentCacheUpdates = setQueryData.mock.calls.filter(
      ([queryKey]) =>
        JSON.stringify(queryKey) === JSON.stringify(documentQueryKey),
    );
    expect(documentCacheUpdates).toHaveLength(1);
    expect(documentCacheUpdates[0]?.[1]).toEqual(savedDocument);
    expect(queryClient.getQueryData(documentQueryKey)).toEqual(savedDocument);
  });
});
