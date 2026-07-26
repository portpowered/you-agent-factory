import { Button } from "@you-agent-factory/components/primitives";
import { useAppLocale } from "../../../i18n";
import { DashboardBento } from "../../bento/components/dashboard-bento";
import { useDashboardBentoStore } from "../../bento/state/dashboardBentoStore";
import { getHeaderControlsMessages } from "../../header/messages/header-controls";
import { DashboardExportDialog } from "../../header/components/dashboard-export-dialog";
import { DashboardHeader } from "../../header/components/dashboard-header";
import { DashboardStatusPanel } from "../../header/components/dashboard-status-panel";
import { useDashboardSnapshot } from "../hooks/useDashboardSnapshot";
import { useDashboardWorldView } from "../hooks/useDashboardWorldView";
import { getDashboardRecoveryMessages } from "../messages/dashboard-recovery";
import {
  type DashboardSessionDiscoveryState,
  DashboardSessionProvider,
} from "../session/dashboard-session-provider";

// Phone gutters stay within the four-CSS-pixel viewport budget. Desktop spacing
// resumes with the dashboard's established medium breakpoint.
const DASHBOARD_SHELL_CLASS = "min-h-screen overflow-x-hidden p-1 md:p-2";

export interface DashboardScreenProps {
  locale?: string;
}

export function DashboardScreen({ locale }: DashboardScreenProps = {}) {
  return (
    <DashboardSessionProvider
      renderDiscoveryState={(state) => (
        <DashboardSessionDiscoveryStatus locale={locale} state={state} />
      )}
    >
      <DashboardScreenContent locale={locale} />
    </DashboardSessionProvider>
  );
}

function DashboardSessionDiscoveryStatus({
  locale,
  state,
}: {
  locale?: string;
  state: DashboardSessionDiscoveryState;
}) {
  const { locale: resolvedLocale } = useAppLocale(locale);
  const messages = getHeaderControlsMessages(resolvedLocale);

  if (state.status === "loading") {
    return (
      <main className={DASHBOARD_SHELL_CLASS}>
        <DashboardStatusPanel
          locale={resolvedLocale}
          title={messages.loadingSessionsLabel}
        />
      </main>
    );
  }

  if (state.status === "error") {
    return (
      <main className={DASHBOARD_SHELL_CLASS}>
        <DashboardStatusPanel
          actions={
            <Button onClick={state.retry} tone="outline">
              {messages.retrySessionsLabel}
            </Button>
          }
          detail={
            state.error instanceof Error ? state.error.message : undefined
          }
          locale={resolvedLocale}
          title={messages.sessionsErrorTitle}
          tone="error"
        />
      </main>
    );
  }

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

function DashboardScreenContent({ locale }: DashboardScreenProps = {}) {
  const { locale: resolvedLocale } = useAppLocale(locale);
  const incrementRefreshToken = useDashboardBentoStore(
    (state) => state.incrementRefreshToken,
  );
  const refreshToken = useDashboardBentoStore((state) => state.refreshToken);
  const {
    snapshot,
    isInitialLoading,
    error,
    preflightRecovery,
    preflightStatus,
    streamState,
    workOutcomeStreamIdentity,
  } = useDashboardSnapshot({
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

      <DashboardBento
        locale={locale}
        workOutcomeStream={{
          identity: workOutcomeStreamIdentity ?? null,
          status: preflightStatus === "success" ? "ready" : "loading",
        }}
      />
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
