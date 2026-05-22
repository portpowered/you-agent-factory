import { useAppLocale } from "../../../i18n";
import { ExportFactoryDialog, useCurrentFactoryExport } from "../../export";
import { useExportDialogStore } from "../../export/state/exportDialogStore";

export interface DashboardExportDialogProps {
  locale?: string;
}

export function DashboardExportDialog({ locale }: DashboardExportDialogProps) {
  const { locale: resolvedLocale } = useAppLocale(locale);
  const closeExportDialog = useExportDialogStore(
    (state) => state.closeExportDialog,
  );
  const isExportDialogOpen = useExportDialogStore(
    (state) => state.isExportDialogOpen,
  );
  const { currentFactoryExport, isPreparing } =
    useCurrentFactoryExport(isExportDialogOpen);

  return (
    <ExportFactoryDialog
      factory={
        currentFactoryExport.ok ? currentFactoryExport.factoryDefinition : null
      }
      initialFactoryName={
        currentFactoryExport.ok
          ? currentFactoryExport.factoryDefinition.name
          : "you-agent-factory"
      }
      isOpen={isExportDialogOpen}
      isPreparing={isPreparing}
      locale={resolvedLocale}
      onClose={closeExportDialog}
      preparationFailure={currentFactoryExport.ok ? null : currentFactoryExport}
    />
  );
}
