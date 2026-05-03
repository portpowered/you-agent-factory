import "./styles.css";

import { DashboardBento } from "./features/bento";
import {
  DashboardExportDialog,
  DashboardHeader,
  DashboardStatusPanel,
} from "./features/header";
import { useDashboardSnapshot } from "./hooks/dashboard/useDashboard";
import { useDashboardAppStore } from "./state/dashboardAppStore";

const DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-5 max-[720px]:p-4";

export function App() {
  const refreshToken = useDashboardAppStore((state) => state.refreshToken);
  const { snapshot, streamState, isInitialLoading, error } = useDashboardSnapshot({
    refreshToken,
  });

  if (isInitialLoading) {
    return (
      <main className={DASHBOARD_SHELL_CLASS}>
        <DashboardStatusPanel title="Loading dashboard" />
      </main>
    );
  }

  if (error instanceof Error) {
    return (
      <main className={DASHBOARD_SHELL_CLASS}>
        <DashboardStatusPanel
          detail={error.message}
          title="Dashboard unavailable"
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
      <DashboardHeader snapshot={snapshot} streamState={streamState} />
      <DashboardBento snapshot={snapshot} />
      <DashboardExportDialog />
    </main>
  );
}
