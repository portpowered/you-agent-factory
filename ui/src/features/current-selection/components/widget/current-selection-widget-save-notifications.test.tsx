import { render, screen } from "@testing-library/react";

import { CurrentSelectionWidgetSaveNotifications } from "./current-selection-widget-save-notifications";

vi.mock("../../base/public", () => ({
  CurrentSelectionSaveNotifications: ({
    entityKind,
    messages,
  }: {
    entityKind: string;
    messages: { saveSuccessDescription: string };
  }) => (
    <div data-testid={`save-notification-${entityKind}`}>
      {messages.saveSuccessDescription}
    </div>
  ),
}));

vi.mock(
  "../../base/components/save/current-selection-graph-draft-conflict-notifications",
  () => ({
    CurrentSelectionGraphDraftConflictNotifications: () => null,
  }),
);

vi.mock(
  "../../../workflow-activity/state/factory-graph-topology-editor-bridge",
  () => ({
    useFactoryGraphTopologyEditorBridge: (
      selector: (state: { graphDraftHasPendingChanges: boolean }) => unknown,
    ) => selector({ graphDraftHasPendingChanges: false }),
  }),
);

function buildIdleSaveHook() {
  return {
    lastSuccessfulSaveWasTopologyAffecting: false,
    saveAttemptRevision: 0,
    saveMutationError: null,
  };
}

describe("CurrentSelectionWidgetSaveNotifications", () => {
  it("renders doc save notifications with the pending target path display label", () => {
    render(
      <CurrentSelectionWidgetSaveNotifications
        docSave={buildIdleSaveHook() as never}
        docSaveState={{ status: "success" }}
        editableConfigurationState={null}
        editableDocConfigurationState={
          {
            draft: {
              fileName: "guide.md",
              inlineContent: "# Guide\n",
              originalExtension: ".md",
            },
            pendingTargetPath: "factory/docs/guide.md",
            status: "ready",
          } as never
        }
        editableResourceConfigurationState={null}
        editableWorkStateConfigurationState={null}
        editableWorkerConfigurationState={null}
        editableWorkTypeConfigurationState={null}
        resourceSave={buildIdleSaveHook() as never}
        resourceSaveState={{ status: "idle" }}
        selectedDocTargetPath="factory/docs/overview.md"
        selectedNode={null}
        selectedResourceName={null}
        selectedWorkerName={null}
        selectedWorkTypeName={null}
        selection={{ kind: "doc", targetPath: "factory/docs/guide.md" }}
        workStatePlaceId={null}
        workStateSave={buildIdleSaveHook() as never}
        workerSave={buildIdleSaveHook() as never}
        workerSaveState={{ status: "idle" }}
        workstationSave={buildIdleSaveHook() as never}
        workstationSaveState={{ status: "idle" }}
        workTypeSave={buildIdleSaveHook() as never}
      />,
    );

    expect(screen.getByTestId("save-notification-doc").textContent).toContain(
      "guide.md was updated in the running factory definition.",
    );
  });

  it("falls back to the selected doc path when editable configuration is unavailable", () => {
    render(
      <CurrentSelectionWidgetSaveNotifications
        docSave={buildIdleSaveHook() as never}
        docSaveState={{ status: "success" }}
        editableConfigurationState={null}
        editableDocConfigurationState={{ status: "loading" }}
        editableResourceConfigurationState={null}
        editableWorkStateConfigurationState={null}
        editableWorkerConfigurationState={null}
        editableWorkTypeConfigurationState={null}
        resourceSave={buildIdleSaveHook() as never}
        resourceSaveState={{ status: "idle" }}
        selectedDocTargetPath="factory/docs/overview.md"
        selectedNode={null}
        selectedResourceName={null}
        selectedWorkerName={null}
        selectedWorkTypeName={null}
        selection={{ kind: "doc", targetPath: "factory/docs/overview.md" }}
        workStatePlaceId={null}
        workStateSave={buildIdleSaveHook() as never}
        workerSave={buildIdleSaveHook() as never}
        workerSaveState={{ status: "idle" }}
        workstationSave={buildIdleSaveHook() as never}
        workstationSaveState={{ status: "idle" }}
        workTypeSave={buildIdleSaveHook() as never}
      />,
    );

    expect(screen.getByTestId("save-notification-doc").textContent).toContain(
      "overview.md was updated in the running factory definition.",
    );
  });
});
