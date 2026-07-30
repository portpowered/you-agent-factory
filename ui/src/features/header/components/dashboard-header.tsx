import { useAppLocale } from "../../../i18n";
import { useDashboardSessionTabsState } from "../hooks/use-dashboard-session-tabs-state";
import { DashboardGeneralHeader } from "./dashboard-general-header";

export interface DashboardHeaderProps {
  locale?: string;
}

export function DashboardHeader({ locale }: DashboardHeaderProps) {
  const { locale: resolvedLocale, setLocale } = useAppLocale(locale);
  const sessionTabsState = useDashboardSessionTabsState();

  return (
    <DashboardGeneralHeader
      locale={resolvedLocale}
      onChangeLocale={setLocale}
      sessionTabsState={sessionTabsState}
    />
  );
}
