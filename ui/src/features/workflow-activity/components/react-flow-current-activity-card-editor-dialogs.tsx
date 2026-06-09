import { isFactoryDocumentSaveSubmitting } from "../../current-selection/base/hooks/factory-document-save-types";
import { FactoryGraphEditorAddEntityDialog } from "../../factory-graph-editor/components/add-dialog/factory-graph-editor-add-dialog";
import { FactoryGraphEditorConfirmationDialog } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import { FactoryGraphEditorLeaveDialog } from "../../factory-graph-editor/components/dialogs/factory-graph-editor-leave-dialog";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { FactoryImportPreviewDialog } from "../../import/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import type { CurrentActivityGraphCardViewModel } from "../hooks/use-current-activity-graph-card-view-model";
import { GraphImportErrorPanel } from "./react-flow-current-activity-card-import";

export function CurrentActivityGraphEditorDialogs({
  currentSessionFactoryName,
  discardEditorChanges,
  editor,
  viewModel,
  imports,
  locale,
  readyImportPreviewState,
  shouldRenderImportPreviewDialog,
}: {
  currentSessionFactoryName: string;
  discardEditorChanges?: () => void;
  editor?: ReturnType<typeof useCurrentActivityGraphEditor>;
  viewModel?: CurrentActivityGraphCardViewModel;
  imports: CurrentActivityImportController;
  locale?: string;
  readyImportPreviewState: Extract<
    CurrentActivityImportController["importPreviewState"],
    { status: "ready" }
  > | null;
  shouldRenderImportPreviewDialog: boolean;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const model = viewModel ?? editor;
  if (!model) {
    return null;
  }
  return (
    <>
      {shouldRenderImportPreviewDialog && readyImportPreviewState ? (
        <FactoryImportPreviewDialog
          activationState={imports.activationState}
          currentSessionFactoryName={currentSessionFactoryName}
          locale={locale}
          onCancel={() => {
            imports.clearActivationError();
            imports.closeImportPreview();
          }}
          onConfirm={(input) => {
            void imports.activateImport(input);
          }}
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
        canSave={model.canSaveDraft}
        isOpen={model.leaveDialogOpen}
        isSaving={model.saveEditableDefinition.isPending}
        locale={locale}
        onCancel={() => {
          if (!model.saveEditableDefinition.isPending) {
            model.setIsConfirmingLeaveEditor(false);
          }
        }}
        onDiscard={
          discardEditorChanges ?? model.handleDiscardEditorChanges
        }
        onSave={() => {
          void model.handleSaveBeforeLeavingEditor();
        }}
      />
      <FactoryGraphEditorConfirmationDialog
        cancelLabel={messages.leaveDialogKeepEditing}
        confirmLabel={model.saveSummary.confirmActionLabel}
        description={model.saveSummary.description}
        isBusy={
          model.saveEditableDefinition.isPending ||
          isFactoryDocumentSaveSubmitting(model.documentSave)
        }
        isOpen={model.isConfirmingSave}
        onCancel={() => {
          if (
            !model.saveEditableDefinition.isPending &&
            !isFactoryDocumentSaveSubmitting(model.documentSave)
          ) {
            model.cancelSaveConfirmation();
          }
        }}
        onConfirm={() => {
          void model.handleSaveDraft();
        }}
        title={messages.saveConfirmTitle}
      />
      <FactoryGraphEditorConfirmationDialog
        cancelLabel={messages.leaveDialogKeepEditing}
        confirmLabel={
          model.pendingRemovalIntent?.confirmLabel ??
          messages.removalFallbackConfirmLabel
        }
        confirmTone="destructive"
        description={
          model.pendingRemovalIntent?.confirmDescription ??
          messages.removalFallbackConfirmDescription
        }
        isOpen={Boolean(
          model.pendingRemovalIntent &&
            !model.pendingRemovalIntent.ineligibleReason,
        )}
        onCancel={model.handleCancelRemoval}
        onConfirm={model.handleConfirmRemoval}
        title={
          model.pendingRemovalIntent?.title ?? messages.removalFallbackTitle
        }
      />
      <FactoryGraphEditorAddEntityDialog
        currentFactoryDefinition={model.currentFactoryDefinition}
        draft={model.addEntityDraft}
        errors={model.addEntityErrors}
        isOpen={model.addEntityDraft !== null}
        locale={locale}
        onChange={(draft) => {
          model.setAddEntityDraft(draft);
          model.setAddEntityErrors({});
        }}
        onClose={() => {
          model.setAddEntityDraft(null);
          model.setAddEntityErrors({});
        }}
        onSubmit={model.handleAddEntitySubmit}
      />
    </>
  );
}
