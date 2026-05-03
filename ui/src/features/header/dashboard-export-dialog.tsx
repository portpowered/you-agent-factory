import { ExportFactoryDialog, useCurrentFactoryExport } from "../export";
import { useExportDialogStore } from "../export/state/exportDialogStore";

export function DashboardExportDialog() {
  const closeExportDialog = useExportDialogStore((state) => state.closeExportDialog);
  const isExportDialogOpen = useExportDialogStore((state) => state.isExportDialogOpen);
  const { currentFactoryExport, isPreparing } = useCurrentFactoryExport(isExportDialogOpen);

  return (
    <ExportFactoryDialog
      factory={currentFactoryExport.ok ? currentFactoryExport.factoryDefinition : null}
      initialFactoryName={
        currentFactoryExport.ok ? currentFactoryExport.factoryDefinition.name : "agent-factory"
      }
      isOpen={isExportDialogOpen}
      isPreparing={isPreparing}
      onClose={closeExportDialog}
      preparationFailure={currentFactoryExport.ok ? null : currentFactoryExport}
    />
  );
}

