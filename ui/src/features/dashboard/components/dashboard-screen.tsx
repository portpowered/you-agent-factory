import { useAppLocale } from "../../../i18n";
import { DashboardBento } from "../../bento";
import { useDashboardBentoStore } from "../../bento/state/dashboardBentoStore";
import {
  DashboardExportDialog,
  DashboardHeader,
  DashboardStatusPanel,
} from "../../header";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { useDashboardSnapshot } from "../hooks/useDashboardSnapshot";

const DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-2";

export interface DashboardScreenProps {
  locale?: string;
}

export function DashboardScreen({ locale }: DashboardScreenProps = {}) {
  const { locale: resolvedLocale } = useAppLocale(locale);
  const refreshToken = useDashboardBentoStore((state) => state.refreshToken);
  const { snapshot, isInitialLoading, error } = useDashboardSnapshot({
    refreshToken,
  });
  const messages = getHeaderControlsMessages(resolvedLocale);

  if (isInitialLoading) {
    return (
      <main className={DASHBOARD_SHELL_CLASS}>
        <DashboardStatusPanel
          locale={resolvedLocale}
          title={messages.loadingDashboardTitle}
        />
      </main>
    );
  }

  if (error instanceof Error) {
    return (
      <main className={DASHBOARD_SHELL_CLASS}>
        <DashboardStatusPanel
          detail={error.message}
          locale={resolvedLocale}
          title={messages.dashboardUnavailableTitle}
          tone="error"
        />
      </main>
    );
  }

  if (!snapshot) {
    return null;
  }

  return (
    <main className={DASHBOARD_SHELL_CLASS}>
      <DashboardHeader locale={locale} />
      <DashboardBento locale={locale} />
      <DashboardExportDialog locale={locale} />
    </main>
  );
}
