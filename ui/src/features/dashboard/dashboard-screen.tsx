import { resolveSupportedLocale } from "../../i18n";
import { DashboardBento } from "../bento";
import { useDashboardBentoStore } from "../bento/state/dashboardBentoStore";
import {
  DashboardExportDialog,
  DashboardHeader,
  DashboardStatusPanel,
} from "../header";
import { useDashboardSnapshot } from "./useDashboardSnapshot";

const DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-5 max-[720px]:p-4";

export function DashboardScreen() {
  const refreshToken = useDashboardBentoStore((state) => state.refreshToken);
  const { snapshot, isInitialLoading, error } = useDashboardSnapshot({
    refreshToken,
  });
  const locale = getDashboardLocale();

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
      <DashboardHeader locale={locale} />
      <DashboardBento locale={locale} />
      <DashboardExportDialog locale={locale} />
    </main>
  );
}

function getDashboardLocale(): string {
  if (typeof document !== "undefined" && document.documentElement.lang) {
    return resolveSupportedLocale(document.documentElement.lang);
  }

  if (typeof navigator !== "undefined") {
    const preferredLocale = navigator.languages[0] ?? navigator.language;
    return resolveSupportedLocale(preferredLocale);
  }

  return resolveSupportedLocale(undefined);
}
