import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import {
  DashboardStatusPill,
  type DashboardStatusPillTone,
} from "../../../components/ui/dashboard-status-pill";
import { getExportDialogMessages } from "../../export/messages/export-dialog";
import type { DashboardSessionTabsState } from "../hooks/use-dashboard-session-tabs-state";
import { sessionStreamToggleLabel } from "../lib/dashboard-session-tabs-utils";
import { getDashboardSessionControlsMessages } from "../messages/dashboard-session-controls";

interface DashboardSessionControlsProps {
  factoryLifecycleStatus?: string | null;
  isExportDialogOpen: boolean;
  locale: string;
  onOpenExportDialog: () => void;
  sessionTabsState: DashboardSessionTabsState;
}

export function DashboardSessionControls({
  factoryLifecycleStatus,
  isExportDialogOpen,
  locale,
  onOpenExportDialog,
  sessionTabsState,
}: DashboardSessionControlsProps) {
  const activeSession = sessionTabsState.activeSession;
  const exportMessages = getExportDialogMessages(locale);
  const sessionControlMessages = getDashboardSessionControlsMessages(locale);
  const isSessionStreamPaused = activeSession
    ? sessionTabsState.isSessionStreamPaused(activeSession.id)
    : false;
  const streamToggleLabel = activeSession
    ? sessionStreamToggleLabel(
        activeSession,
        isSessionStreamPaused,
        sessionControlMessages,
      )
    : null;
  const lifecycleStatus = factoryLifecycleStatusLabel(
    factoryLifecycleStatus,
    sessionControlMessages,
  );

  return (
    <div className="ml-auto flex shrink-0 items-center gap-1.5">
      {lifecycleStatus ? (
        <DashboardStatusPill
          aria-label={lifecycleStatus.label}
          role="status"
          size="compact"
          tone={lifecycleStatus.tone}
        >
          {lifecycleStatus.label}
        </DashboardStatusPill>
      ) : null}
      {isSessionStreamPaused ? (
        <DashboardStatusPill
          aria-label={sessionControlMessages.liveDashboardUpdatesPausedLabel}
          role="status"
          size="compact"
          tone="warning"
        >
          {sessionControlMessages.liveDashboardUpdatesPausedLabel}
        </DashboardStatusPill>
      ) : null}
      {activeSession ? (
        <DashboardActionButton
          aria-label={streamToggleLabel ?? undefined}
          aria-pressed={isSessionStreamPaused}
          iconOnly
          onClick={() => {
            sessionTabsState.toggleSessionStreamPaused(activeSession.id);
          }}
          title={streamToggleLabel ?? undefined}
          tone="outline"
        >
          <SessionStreamToggleIcon paused={isSessionStreamPaused} />
        </DashboardActionButton>
      ) : null}
      <DashboardActionButton
        aria-expanded={isExportDialogOpen}
        aria-haspopup="dialog"
        aria-label={exportMessages.triggerLabel}
        iconOnly
        onClick={onOpenExportDialog}
        tone="outline"
      >
        <ExportButtonIcon />
      </DashboardActionButton>
    </div>
  );
}

function factoryLifecycleStatusLabel(
  status: string | null | undefined,
  messages: ReturnType<typeof getDashboardSessionControlsMessages>,
): { label: string; tone: DashboardStatusPillTone } | null {
  switch (status?.trim().toUpperCase()) {
    case "PAUSED":
      return {
        label: messages.factoryLifecyclePausedLabel,
        tone: "warning",
      };
    case "RUNNING":
      return {
        label: messages.factoryLifecycleRunningLabel,
        tone: "success",
      };
    default:
      return null;
  }
}

function ExportButtonIcon() {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="18"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
      width="18"
    >
      <path d="M14 5h5v5" />
      <path d="M10 14 19 5" />
      <path d="M19 13v5a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h5" />
    </svg>
  );
}

function SessionStreamToggleIcon({ paused }: { paused: boolean }) {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="16"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
      width="16"
    >
      {paused ? (
        <path d="M8 5.75 18 12 8 18.25v-12.5Z" />
      ) : (
        <>
          <path d="M9 5.75v12.5" />
          <path d="M15 5.75v12.5" />
        </>
      )}
    </svg>
  );
}
