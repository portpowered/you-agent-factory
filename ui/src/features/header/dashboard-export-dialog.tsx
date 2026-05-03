import { ExportFactoryDialog, useCurrentFactoryExport } from "../export";
import { useDashboardAppStore } from "../../state/dashboardAppStore";

export function DashboardExportDialog() {
  const closeExportDialog = useDashboardAppStore((state) => state.closeExportDialog);
  const isExportDialogOpen = useDashboardAppStore((state) => state.isExportDialogOpen);
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
