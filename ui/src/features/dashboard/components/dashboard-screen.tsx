import { useAppLocale } from "../../../i18n";
import { DashboardBento } from "../../bento/public";
import { useDashboardBentoStore } from "../../bento/state/dashboardBentoStore";
import {
  DashboardExportDialog,
  DashboardHeader,
  DashboardStatusPanel,
} from "../../header/public";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { useDashboardSnapshot } from "../hooks/useDashboardSnapshot";
import { DashboardSessionProvider } from "../session/dashboard-session-provider";

const DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-2";

export interface DashboardScreenProps {
  locale?: string;
}

export function DashboardScreen({ locale }: DashboardScreenProps = {}) {
  const { locale: resolvedLocale } = useAppLocale(locale);
  const refreshToken = useDashboardBentoStore((state) => state.refreshToken);
  const { snapshot, isInitialLoading, error } = useDashboardSnapshot({
    locale: resolvedLocale,
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
    return (
      <main className={DASHBOARD_SHELL_CLASS}>
        <DashboardHeader locale={locale} />
        <DashboardStatusPanel
          locale={resolvedLocale}
          title={messages.sessionsEmptyTitle}
        />
      </main>
    );
  }

  return (
    <main className={DASHBOARD_SHELL_CLASS}>
      <DashboardSessionProvider>
        <DashboardHeader locale={locale} />
        <DashboardBento locale={locale} />
        <DashboardExportDialog locale={locale} />
      </DashboardSessionProvider>
    </main>
  );
}
