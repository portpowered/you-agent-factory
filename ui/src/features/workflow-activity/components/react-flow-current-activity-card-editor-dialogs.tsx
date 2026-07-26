import { isFactoryDocumentSaveSubmitting } from "../../current-selection/base/hooks/factory-document-save-types";
import { FactoryGraphEditorAddEntityDialog } from "../../factory-graph-editor/components/add-dialog/factory-graph-editor-add-dialog";
import { FactoryGraphEditorConfirmationDialog } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import { FactoryGraphEditorLeaveDialog } from "../../factory-graph-editor/components/dialogs/factory-graph-editor-leave-dialog";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { FactoryImportPreviewDialog } from "../../import/components/dashboard-import-preview-dialog";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { CurrentActivityGraphCardViewModel } from "../hooks/use-current-activity-graph-card-view-model";
import { GraphImportErrorPanel } from "./react-flow-current-activity-card-import";

export function CurrentActivityGraphEditorDialogs({
  currentSessionFactoryName,
  discardEditorChanges,
  viewModel,
  imports,
  locale,
  readyImportPreviewState,
  shouldRenderImportPreviewDialog,
}: {
  currentSessionFactoryName: string;
  discardEditorChanges?: () => void;
  viewModel: CurrentActivityGraphCardViewModel;
  imports: CurrentActivityImportController;
  locale?: string;
  readyImportPreviewState: Extract<
    CurrentActivityImportController["importPreviewState"],
    { status: "ready" }
  > | null;
  shouldRenderImportPreviewDialog: boolean;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const addControls = viewModel.addControls;
  const isSaving = viewModel.status.isSaving;
  const leaveControls = viewModel.leaveControls;
  const removalControls = viewModel.removalControls;
  const saveControls = viewModel.saveControls;
  const isSaveBusy =
    isSaving || isFactoryDocumentSaveSubmitting(saveControls.feedback);

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
        canSave={saveControls.canSave}
        isOpen={leaveControls.isOpen}
        isSaving={isSaving}
        locale={locale}
        onCancel={() => {
          if (!isSaving) {
            leaveControls.cancel();
          }
        }}
        onDiscard={discardEditorChanges ?? leaveControls.discardChanges}
        onSave={() => {
          void saveControls.saveBeforeLeaving();
        }}
      />
      <FactoryGraphEditorConfirmationDialog
        cancelLabel={messages.leaveDialogKeepEditing}
        confirmLabel={saveControls.summary.confirmActionLabel}
        description={saveControls.summary.description}
        isBusy={isSaveBusy}
        isOpen={saveControls.isConfirming}
        onCancel={() => {
          if (!isSaveBusy) {
            saveControls.cancelConfirmation();
          }
        }}
        onConfirm={() => {
          void saveControls.confirmSave();
        }}
        title={messages.saveConfirmTitle}
      />
      <FactoryGraphEditorConfirmationDialog
        cancelLabel={messages.leaveDialogKeepEditing}
        confirmLabel={
          removalControls.pendingIntent?.confirmLabel ??
          messages.removalFallbackConfirmLabel
        }
        confirmTone="destructive"
        description={
          removalControls.pendingIntent?.confirmDescription ??
          messages.removalFallbackConfirmDescription
        }
        isOpen={Boolean(
          removalControls.pendingIntent &&
            !removalControls.pendingIntent.ineligibleReason,
        )}
        onCancel={removalControls.cancel}
        onConfirm={removalControls.confirm}
        title={
          removalControls.pendingIntent?.title ?? messages.removalFallbackTitle
        }
      />
      <FactoryGraphEditorAddEntityDialog
        currentFactoryDefinition={addControls.currentFactoryDefinition}
        draft={addControls.draft}
        errors={addControls.errors}
        isOpen={addControls.isDialogOpen}
        locale={locale}
        onChange={(draft) => {
          addControls.updateDraft(draft);
        }}
        onClose={addControls.closeDialog}
        onSubmit={addControls.submit}
      />
    </>
  );
}
