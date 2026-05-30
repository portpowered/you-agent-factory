import { useEffect, useState } from "react";

import type { FactoryImportSaveChoice } from "../../../api/named-factory";
import { FactoryGraphEditorAddEntityDialog } from "../../factory-graph-editor/components/factory-graph-editor-add-dialog";
import { FactoryGraphEditorConfirmationDialog } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { FactoryGraphEditorLeaveDialog } from "../../factory-graph-editor/components/factory-graph-editor-leave-dialog";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { FactoryImportPreviewDialog } from "../../import/public";
import { useFactoryImportActivationTarget } from "../../import/hooks/use-factory-import-activation-target";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import { GraphImportErrorPanel } from "./react-flow-current-activity-card-import";

export function CurrentActivityGraphEditorDialogs({
  editor,
  imports,
  locale,
  readyImportPreviewState,
  sessionID,
  shouldRenderImportPreviewDialog,
}: {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  imports: CurrentActivityImportController;
  locale?: string;
  readyImportPreviewState: Extract<
    CurrentActivityImportController["importPreviewState"],
    { status: "ready" }
  > | null;
  sessionID?: string | null;
  shouldRenderImportPreviewDialog: boolean;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const [importSaveChoice, setImportSaveChoice] =
    useState<FactoryImportSaveChoice>("REPLACE_CURRENT");
  const importActivationTarget = useFactoryImportActivationTarget({
    enabled: shouldRenderImportPreviewDialog && readyImportPreviewState !== null,
    preferredFactoryName: readyImportPreviewState?.value?.factory?.name,
    sessionID,
  });

  useEffect(() => {
    if (readyImportPreviewState) {
      setImportSaveChoice("REPLACE_CURRENT");
    }
  }, [
    readyImportPreviewState?.file?.name,
    readyImportPreviewState?.value?.factory?.name,
  ]);

  return (
    <>
      {shouldRenderImportPreviewDialog && readyImportPreviewState ? (
        <FactoryImportPreviewDialog
          activationState={imports.activationState}
          createTargetFactoryName={importActivationTarget.createTargetFactoryName}
          currentFactoryName={importActivationTarget.currentFactoryName}
          importSaveChoice={importSaveChoice}
          locale={locale}
          onCancel={() => {
            imports.clearActivationError();
            imports.closeImportPreview();
          }}
          onConfirm={() => {
            void imports.activateImport(
              readyImportPreviewState.value,
              importSaveChoice,
            );
          }}
          onImportSaveChoiceChange={setImportSaveChoice}
          previewState={readyImportPreviewState}
        />
      ) : null}
      {imports.dropState.status === "error" ? (
        <GraphImportErrorPanel
          error={imports.dropState.error}
          fileName={imports.dropState.fileName}
          locale={locale}
          onDismiss={imports.clearError}
        />
      ) : null}
      <FactoryGraphEditorLeaveDialog
        canSave={editor.canSaveDraft}
        isOpen={editor.leaveDialogOpen}
        isSaving={editor.saveEditableDefinition.status === "pending"}
        locale={locale}
        onCancel={() => {
          if (editor.saveEditableDefinition.status !== "pending") {
            editor.setIsConfirmingLeaveEditor(false);
          }
        }}
        onDiscard={editor.handleDiscardEditorChanges}
        onSave={() => {
          void editor.handleSaveBeforeLeavingEditor();
        }}
      />
      <FactoryGraphEditorConfirmationDialog
        cancelLabel={messages.leaveDialogKeepEditing}
        confirmLabel={messages.saveConfirmAction}
        description={editor.saveSummary.description}
        isBusy={editor.saveEditableDefinition.status === "pending"}
        isOpen={editor.isConfirmingSave}
        onCancel={() => {
          if (editor.saveEditableDefinition.status !== "pending") {
            editor.setIsConfirmingSave(false);
          }
        }}
        onConfirm={() => {
          void editor.handleSaveDraft();
        }}
        title={messages.saveConfirmTitle}
      />
      <FactoryGraphEditorAddEntityDialog
        currentFactoryDefinition={editor.currentFactoryDefinition}
        draft={editor.addEntityDraft}
        errors={editor.addEntityErrors}
        isOpen={editor.addEntityDraft !== null}
        locale={locale}
        onChange={(draft) => {
          editor.setAddEntityDraft(draft);
          editor.setAddEntityErrors({});
        }}
        onClose={() => {
          editor.setAddEntityDraft(null);
          editor.setAddEntityErrors({});
        }}
        onSubmit={editor.handleAddEntitySubmit}
      />
    </>
  );
}
