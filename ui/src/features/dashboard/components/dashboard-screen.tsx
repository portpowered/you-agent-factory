import { Button } from "../../../components/ui";
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
import { useDashboardWorldView } from "../hooks/useDashboardWorldView";
import { getDashboardRecoveryMessages } from "../messages/dashboard-recovery";
import { DashboardSessionProvider } from "../session/dashboard-session-provider";

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
  const incrementRefreshToken = useDashboardBentoStore(
    (state) => state.incrementRefreshToken,
  );
  const refreshToken = useDashboardBentoStore((state) => state.refreshToken);
  const { snapshot, isInitialLoading, error, preflightRecovery, streamState } =
    useDashboardSnapshot({
      locale: resolvedLocale,
      refreshToken,
    });
  useDashboardWorldView();
  const messages = getHeaderControlsMessages(resolvedLocale);
  const recoveryMessages = getDashboardRecoveryMessages(resolvedLocale);

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

  if (preflightRecovery) {
    const recoveryCopy = copyForPreflightRecovery(
      preflightRecovery,
      recoveryMessages,
    );

    return (
      <main className={DASHBOARD_SHELL_CLASS}>
        <DashboardHeader locale={locale} />
        <DashboardStatusPanel
          actions={
            <Button
              aria-label={recoveryMessages.preflightRetryAction}
              onClick={incrementRefreshToken}
              tone="outline"
            >
              {recoveryMessages.preflightRetryAction}
            </Button>
          }
          detail={recoveryCopy.detail}
          locale={resolvedLocale}
          title={recoveryCopy.title}
          tone="error"
        />
      </main>
    );
  }

  if (error instanceof Error) {
    if (streamState.status === "recovery_failed") {
      return (
        <main className={DASHBOARD_SHELL_CLASS}>
          <DashboardStatusPanel
            detail={recoveryMessages.recoveryFailedDetail}
            locale={resolvedLocale}
            title={recoveryMessages.recoveryFailedTitle}
            tone="error"
          />
          <div className="flex flex-wrap gap-3">
            <Button onClick={incrementRefreshToken}>
              {recoveryMessages.recoveryFailedRetryLabel}
            </Button>
            <Button
              onClick={() => {
                window.location.reload();
              }}
              tone="outline"
            >
              {recoveryMessages.recoveryFailedRefreshLabel}
            </Button>
          </div>
        </main>
      );
    }
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
      <DashboardBento locale={locale} />
      <DashboardExportDialog locale={locale} />
    </main>
  );
}

function copyForPreflightRecovery(
  recovery: NonNullable<
    ReturnType<typeof useDashboardSnapshot>["preflightRecovery"]
  >,
  messages: ReturnType<typeof getDashboardRecoveryMessages>,
): { detail: string; title: string } {
  if (recovery.reasonCode === "session_not_found") {
    return {
      detail: messages.sessionNotFoundDetailTemplate.replace(
        "{{sessionId}}",
        recovery.requestedSessionId,
      ),
      title: messages.sessionNotFoundTitle,
    };
  }

  if (recovery.reasonCode === "logical_session_unresolved") {
    return {
      detail: messages.logicalSessionUnresolvedDetail,
      title: messages.logicalSessionUnresolvedTitle,
    };
  }

  return {
    detail: messages.unknownRecoveryDetail,
    title: messages.unknownRecoveryTitle,
  };
}
