import { DashboardBento } from "../bento";
import { useDashboardBentoStore } from "../bento/state/dashboardBentoStore";
import {
  DashboardExportDialog,
  DashboardHeader,
  DashboardStatusPanel,
} from "../header";
import { getHeaderControlsMessages } from "../header/messages/header-controls";
import { useDashboardSnapshot } from "./useDashboardSnapshot";

const DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-2";

export interface DashboardScreenProps {
  locale?: string;
}

export function DashboardScreen({ locale }: DashboardScreenProps = {}) {
  const refreshToken = useDashboardBentoStore((state) => state.refreshToken);
  const { snapshot, isInitialLoading, error } = useDashboardSnapshot({
    refreshToken,
  });
  const messages = getHeaderControlsMessages(locale);

  if (isInitialLoading) {
    return (
      <main className={DASHBOARD_SHELL_CLASS}>
        <DashboardStatusPanel
          locale={locale}
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
          locale={locale}
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
      <DashboardBento />
      <DashboardExportDialog locale={locale} />
    </main>
  );
}
