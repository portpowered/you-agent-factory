import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { mockFactoryDocumentSave } from "../../../../testing/factory-document-save-mocks";
import * as factoryDocumentSaveHooks from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { useDashboardSessionStore } from "../../../dashboard/state/dashboardSessionStore";
import type { EditableDocConfigurationState } from "../lib/detail-card-types";
import { useSaveEditableDocConfiguration } from "./use-save-editable-doc-configuration";

describe("useSaveEditableDocConfiguration", () => {
  beforeEach(() => {
    useDashboardSessionStore.setState({ selectedSessionID: "~default" });
    vi.restoreAllMocks();
  });

  it("saves doc edits and refreshes selection after rename", async () => {
    const markChangesSaved = vi.fn();
    const onDocRenamed = vi.fn();
    const saveMutation = mockFactoryDocumentSave({
      mode: "success",
      resolvedDocument: {
        name: "Current Factory",
        supportingFiles: {
          bundledFiles: [
            {
              content: { encoding: "utf-8", inline: "# Guide\n" },
              targetPath: "factory/docs/guide.md",
              type: "DOC",
            },
          ],
        },
        version: {
          logical: "8",
          physical: "2026-05-23T15:52:00.001Z",
        },
        workTypes: [],
      },
    });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { result } = renderHook(
      () =>
        useSaveEditableDocConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            markChangesSaved,
          }),
          onDocRenamed,
          scopeKey: "factory/docs/overview.md",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.save();
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });
    expect(markChangesSaved).toHaveBeenCalledTimes(1);
    expect(onDocRenamed).toHaveBeenCalledWith("factory/docs/guide.md");
  });

  it("no-ops when save is invoked without a ready editable configuration", async () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { result } = renderHook(
      () =>
        useSaveEditableDocConfiguration({
          editableConfigurationState: { status: "loading" },
          scopeKey: "factory/docs/overview.md",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.save();
    });

    expect(saveMutation.saveAsync).not.toHaveBeenCalled();
  });
});

function buildReadyEditableConfigurationState(
  overrides?: Partial<{
    isDirty: boolean;
    markChangesSaved: () => void;
  }>,
): EditableDocConfigurationState {
  return {
    baseVersion: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    canSave: true,
    draft: {
      fileName: "guide.md",
      inlineContent: "# Guide\n",
      originalExtension: ".md",
    },
    hasValidationErrors: false,
    initialValues: {
      fileName: "overview.md",
      inlineContent: "# Overview\n",
      targetPath: "factory/docs/overview.md",
    },
    isDirty: overrides?.isDirty ?? true,
    markChangesSaved: overrides?.markChangesSaved ?? vi.fn(),
    onFileNameChange: vi.fn(),
    onInlineContentChange: vi.fn(),
    onResetToLatest: vi.fn(),
    originalTargetPath: "factory/docs/overview.md",
    overwriteFieldNames: [],
    pendingFactoryDefinition: {
      name: "Current Factory",
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Guide\n" },
            targetPath: "factory/docs/guide.md",
            type: "DOC",
          },
        ],
      },
      workTypes: [],
    },
    pendingTargetPath: "factory/docs/guide.md",
    savedFactoryDefinition: {
      name: "Current Factory",
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Overview\n" },
            targetPath: "factory/docs/overview.md",
            type: "DOC",
          },
        ],
      },
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      workTypes: [],
    },
    status: "ready",
    validationErrors: {},
  };
}

function createQueryClientWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });

  return function QueryClientWrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}
