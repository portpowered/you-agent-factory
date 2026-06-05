import { useAppLocale } from "../../../i18n";
import { useExportDialogStore } from "../../export/state/exportDialogStore";
import { useDashboardSessionTabsState } from "../hooks/use-dashboard-session-tabs-state";
import { DashboardGeneralHeader } from "./dashboard-general-header";

export interface DashboardHeaderProps {
  locale?: string;
}

export function DashboardHeader({ locale }: DashboardHeaderProps) {
  const { locale: resolvedLocale, setLocale } = useAppLocale(locale);
  const sessionTabsState = useDashboardSessionTabsState();
  const isExportDialogOpen = useExportDialogStore(
    (state) => state.isExportDialogOpen,
  );
  const openExportDialog = useExportDialogStore(
    (state) => state.openExportDialog,
  );

  return (
    <DashboardGeneralHeader
      isExportDialogOpen={isExportDialogOpen}
      locale={resolvedLocale}
      onChangeLocale={setLocale}
      onOpenExportDialog={openExportDialog}
      sessionTabsState={sessionTabsState}
    />
  );
}
