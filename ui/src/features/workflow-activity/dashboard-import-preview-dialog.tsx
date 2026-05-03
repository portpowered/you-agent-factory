import type { CurrentActivityImportController } from "./current-activity-import-controller";
import { FactoryImportPreviewDialog } from "./react-flow-current-activity-card-import";

interface DashboardImportPreviewDialogProps {
  importController: CurrentActivityImportController;
}

export function DashboardImportPreviewDialog({
  importController,
}: DashboardImportPreviewDialogProps) {
  const readyImportPreviewState =
    importController.importPreviewState.status === "ready"
      ? importController.importPreviewState
      : null;

  if (!readyImportPreviewState) {
    return null;
  }

  return (
    <FactoryImportPreviewDialog
      activationState={importController.activationState}
      onCancel={() => {
        importController.clearActivationError();
        importController.closeImportPreview();
      }}
      onConfirm={() => {
        void importController.activateImport(readyImportPreviewState.value);
      }}
      previewState={readyImportPreviewState}
    />
  );
}
