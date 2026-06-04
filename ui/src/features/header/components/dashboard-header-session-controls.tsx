import { getExportDialogMessages } from "../../export/messages/export-dialog";
import type { DashboardSessionTabsState } from "../hooks/use-dashboard-session-tabs-state";
import { sessionStreamToggleLabel } from "../lib/dashboard-session-tabs-utils";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardHeaderActionButton } from "./dashboard-header-action-button";

interface DashboardHeaderSessionControlsProps {
  isExportDialogOpen: boolean;
  locale: string;
  onOpenExportDialog: () => void;
  sessionTabsState: DashboardSessionTabsState;
}

export function DashboardHeaderSessionControls({
  isExportDialogOpen,
  locale,
  onOpenExportDialog,
  sessionTabsState,
}: DashboardHeaderSessionControlsProps) {
  const activeSession = sessionTabsState.activeSession;
  const exportMessages = getExportDialogMessages(locale);
  const headerMessages = getHeaderControlsMessages(locale);

  return (
    <div className="ml-auto flex shrink-0 items-center gap-1.5 pr-2">
      {activeSession ? (
        <DashboardHeaderActionButton
          aria-label={sessionStreamToggleLabel(
            activeSession,
            sessionTabsState.isSessionStreamPaused(activeSession.id),
            headerMessages,
          )}
          aria-pressed={sessionTabsState.isSessionStreamPaused(activeSession.id)}
          compact
          onClick={() => {
            sessionTabsState.toggleSessionStreamPaused(activeSession.id);
          }}
        >
          <SessionStreamToggleIcon
            paused={sessionTabsState.isSessionStreamPaused(activeSession.id)}
          />
        </DashboardHeaderActionButton>
      ) : null}
      <DashboardHeaderActionButton
        aria-expanded={isExportDialogOpen}
        aria-haspopup="dialog"
        aria-label={exportMessages.triggerLabel}
        compact
        onClick={onOpenExportDialog}
      >
        <ExportButtonIcon />
      </DashboardHeaderActionButton>
    </div>
  );
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
