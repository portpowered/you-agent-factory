import { useAppLocale } from "../../../i18n";
import { DashboardBento } from "../../bento/public";
import { useDashboardBentoStore } from "../../bento/state/dashboardBentoStore";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import {
  DashboardExportDialog,
  DashboardHeader,
  DashboardStatusPanel,
} from "../../header/public";
import { useDashboardSnapshot } from "../hooks/useDashboardSnapshot";
import { useDashboardWorldView } from "../hooks/useDashboardWorldView";
import { DashboardSessionProvider } from "../session/dashboard-session-provider";
import { DashboardSessionLifecycleBanner } from "./dashboard-session-lifecycle-banner";

const DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-2";

export interface DashboardScreenProps {
  locale?: string;
}

export function DashboardScreen({ locale }: DashboardScreenProps = {}) {
  return (
    <DashboardSessionProvider>
      <DashboardScreenContent locale={locale} />
    </DashboardSessionProvider>
  );
}

function DashboardScreenContent({ locale }: DashboardScreenProps = {}) {
  const { locale: resolvedLocale } = useAppLocale(locale);
  const refreshToken = useDashboardBentoStore((state) => state.refreshToken);
  const { snapshot, isInitialLoading, error, streamState } = useDashboardSnapshot({
    locale: resolvedLocale,
    refreshToken,
  });
  useDashboardWorldView();
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
      <DashboardHeader locale={locale} />

      <DashboardSessionLifecycleBanner
        bracket={snapshot.runtime?.session?.bracket}
        factoryState={snapshot.factory_state}
        locale={resolvedLocale}
        streamState={streamState}
      />

      <DashboardBento locale={locale} />
      <DashboardExportDialog locale={locale} />
    </main>
  );
}
