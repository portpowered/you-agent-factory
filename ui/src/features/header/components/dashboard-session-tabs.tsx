import {
  useEffect,
  useId,
  useRef,
} from "react";
import type { DashboardStreamState } from "../../../api/dashboard/types";
import type { FactorySessionSummary } from "../../../api/factory-sessions";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { Button, Dialog } from "../../../components/ui";
import { cn } from "../../../lib/cn";
import { DASHBOARD_BODY_TEXT_CLASS } from "../../../components/ui/dashboard-typography";
import { useDashboardStreamStore } from "../../dashboard/state/dashboardStreamStore";
import { SessionTabButton } from "./dashboard-session-tab-button";
import { OpenSessionDialog } from "./dashboard-session-tabs-open-dialog";
import {
  normalizeFactorySessionsError,
  sessionPanelID,
  sessionTabID,
} from "../lib/dashboard-session-tabs-utils";
import { getHeaderControlsMessages } from "../messages/header-controls";
import {
  type DashboardSessionTabsState,
  useDashboardSessionTabsState,
} from "../hooks/use-dashboard-session-tabs-state";
const SESSION_TABS_SHELL_CLASS = "grid min-h-0 min-w-0 h-full flex-1 gap-2";
const SESSION_TABS_ROW_CLASS = "flex min-h-0 min-w-0 h-full w-full items-stretch gap-1.5";
const SESSION_TAB_LIST_CLASS =
  "flex min-h-0 min-w-0 h-full flex-1 items-stretch gap-1 overflow-x-auto overflow-y-visible px-3";
const OPEN_SESSION_TAB_BUTTON_CLASS = cn(
  "flex h-full shrink-0 items-center rounded-t-xl bg-transparent px-3 py-2 text-af-ink/58 transition-colors",
  "hover:bg-af-overlay/4 hover:text-af-ink",
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25",
);
const SESSION_DIALOG_ERROR_CLASS =
  "rounded-xl border border-af-danger/32 bg-af-danger/8 px-3 py-2 text-sm text-af-ink";

export function DashboardSessionTabs({
  locale,
  state,
}: {
  locale: string;
  state?: DashboardSessionTabsState;
}) {
  const fallbackState = useDashboardSessionTabsState();
  const sessionTabsState = state ?? fallbackState;

  return (
    <DashboardSessionTabsView
      locale={locale}
      state={sessionTabsState}
    />
  );
}

function DashboardSessionTabsView({
  locale,
  state,
}: {
  locale: string;
  state: DashboardSessionTabsState;
}) {
  const messages = getHeaderControlsMessages(locale);
  const streamStatus = useDashboardStreamStore(
    (state) => state.streamState.status,
  );
  const {
    activeSession,
    activeSessionID,
    closeError,
    closeSessionMutation,
    dialogError,
    dialogOpen,
    discoveredTargets,
    folderValidation,
    folderPath,
    handleChangeManualFactoryName,
    handleChangeFolderPath,
    handleCloseSession,
    handleInspectFolder,
    handleOpenTarget,
    manualFactoryName,
    openSessionMutation,
    resetDialogState,
    selectedTargetValue,
    sessions,
    sessionsQuery,
    setActiveSessionID,
    setDialogOpen,
    validateFolderMutation,
    setSelectedTargetValue,
    toggleSessionStreamPaused,
  } = state;

  useEffect(() => {
    if (sessions.length === 0) {
      return;
    }
    if (
      activeSessionID === "" ||
      !sessions.some((session) => session.id === activeSessionID)
    ) {
      setActiveSessionID(sessions[0]?.id ?? DEFAULT_FACTORY_SESSION_ID);
    }
  }, [activeSessionID, sessions, setActiveSessionID]);

  return (
    <>
      <div className={SESSION_TABS_SHELL_CLASS}>
        <div className={SESSION_TABS_ROW_CLASS}>
          <SessionTabsContent
            activeSession={activeSession}
            closingSessionID={
              closeSessionMutation.isPending ? closeSessionMutation.variables : null
            }
            error={sessionsQuery.isError ? sessionsQuery.error : null}
            isPending={sessionsQuery.isPending}
            messages={messages}
            onCloseSession={handleCloseSession}
            onRetry={() => {
              void sessionsQuery.refetch();
            }}
            onOpenSession={() => {
              setDialogOpen(true);
            }}
            onSelectSession={setActiveSessionID}
            sessions={sessions}
            streamStatus={streamStatus}
          />
        </div>
        {closeError ? (
          <p className={SESSION_DIALOG_ERROR_CLASS} role="alert">
            {closeError.message}
          </p>
        ) : null}
      </div>
      <Dialog
        onOpenChange={(open) => {
          setDialogOpen(open);
          if (!open) {
            resetDialogState();
          }
        }}
        open={dialogOpen}
      >
        <OpenSessionDialog
          dialogError={dialogError}
          discoveredTargets={discoveredTargets}
          folderValidation={folderValidation}
          folderPath={folderPath}
          isPending={
            openSessionMutation.isPending || validateFolderMutation.isPending
          }
          manualFactoryName={manualFactoryName}
          messages={messages}
          onChangeFolderPath={handleChangeFolderPath}
          onChangeManualFactoryName={handleChangeManualFactoryName}
          onInspectFolder={handleInspectFolder}
          onOpenTarget={handleOpenTarget}
          onSelectTarget={setSelectedTargetValue}
          selectedTargetValue={selectedTargetValue}
        />
      </Dialog>
    </>
  );
}

function SessionTabsContent({
  activeSession,
  closingSessionID,
  error,
  isPending,
  messages,
  onCloseSession,
  onOpenSession,
  onRetry,
  onSelectSession,
  sessions,
  streamStatus,
}: {
  activeSession: FactorySessionSummary | null;
  closingSessionID: string | null;
  error: unknown;
  isPending: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onCloseSession: (sessionID: string) => void;
  onOpenSession: () => void;
  onRetry: () => void;
  onSelectSession: (sessionID: string) => void;
  sessions: FactorySessionSummary[];
  streamStatus: DashboardStreamState["status"];
}) {
  const sessionButtonRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const sessionTabsID = useId();

  if (isPending) {
    return (
      <p className={cn("text-sm text-af-ink/68", DASHBOARD_BODY_TEXT_CLASS)}>
        {messages.loadingSessionsLabel}
      </p>
    );
  }
  if (error) {
    const sessionError = normalizeFactorySessionsError(error);
    return (
      <SessionErrorState
        label={
          sessionError.code === "NETWORK_ERROR"
            ? messages.sessionsOfflineTitle
            : messages.sessionsErrorTitle
        }
        messages={messages}
        onRetry={onRetry}
      />
    );
  }
  if (sessions.length === 0) {
    return (
      <>
        <p className={cn("text-sm text-af-ink/76", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.sessionsEmptyTitle}
        </p>
        <button
          aria-haspopup="dialog"
          aria-label={messages.openSessionButtonLabel}
          className={OPEN_SESSION_TAB_BUTTON_CLASS}
          onClick={onOpenSession}
          type="button"
        >
          <span aria-hidden="true" className="text-lg leading-none">+</span>
        </button>
      </>
    );
  }

  function focusSessionButton(index: number) {
    sessionButtonRefs.current[index]?.focus();
  }

  function moveSessionFocus(currentIndex: number, offset: number) {
    const nextIndex = (currentIndex + offset + sessions.length) % sessions.length;
    const nextSession = sessions[nextIndex];
    if (!nextSession) {
      return;
    }
    onSelectSession(nextSession.id);
    focusSessionButton(nextIndex);
  }

  return (
    <>
      <nav aria-label={messages.sessionTabsLabel} className="min-w-0 flex-1">
        <div
          aria-orientation="horizontal"
          className={SESSION_TAB_LIST_CLASS}
          role="tablist"
        >
          {sessions.map((session, index) => (
            <SessionTabButton
              active={session.id === activeSession?.id}
              buttonRef={(element) => {
                sessionButtonRefs.current[index] = element;
              }}
              controlsID={sessionPanelID(sessionTabsID, session.id)}
              key={session.id}
              onKeyDown={(event) => {
                switch (event.key) {
                  case "ArrowLeft":
                  case "ArrowUp":
                    event.preventDefault();
                    moveSessionFocus(index, -1);
                    return;
                  case "ArrowRight":
                  case "ArrowDown":
                    event.preventDefault();
                    moveSessionFocus(index, 1);
                    return;
                  case "Home":
                    event.preventDefault();
                    onSelectSession(sessions[0]?.id ?? session.id);
                    focusSessionButton(0);
                    return;
                  case "End":
                    event.preventDefault();
                    onSelectSession(
                      sessions[sessions.length - 1]?.id ?? session.id,
                    );
                    focusSessionButton(sessions.length - 1);
                    return;
                  default:
                    return;
                }
              }}
              onClick={() => {
                onSelectSession(session.id);
              }}
              onClose={() => {
                onCloseSession(session.id);
              }}
              closeDisabled={closingSessionID === session.id}
              messages={messages}
              session={session}
              streamStatus={streamStatus}
              tabID={sessionTabID(sessionTabsID, session.id)}
            />
          ))}
          <button
            aria-haspopup="dialog"
            aria-label={messages.openSessionButtonLabel}
            className={OPEN_SESSION_TAB_BUTTON_CLASS}
            onClick={onOpenSession}
            type="button"
          >
            <span aria-hidden="true" className="text-lg leading-none">+</span>
          </button>
        </div>
      </nav>
      {activeSession ? (
        <div
          aria-labelledby={sessionTabID(sessionTabsID, activeSession.id)}
          className="sr-only"
          id={sessionPanelID(sessionTabsID, activeSession.id)}
          role="tabpanel"
        >
          <p>{activeSession.folderPath}</p>
        </div>
      ) : null}
    </>
  );
}

function SessionErrorState({
  label,
  messages,
  onRetry,
}: {
  label: string;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onRetry: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <p className={cn("text-sm text-af-ink/76", DASHBOARD_BODY_TEXT_CLASS)}>
        {label}
      </p>
      <Button onClick={onRetry} size="sm" tone="outline">
        {messages.retrySessionsLabel}
      </Button>
    </div>
  );
}
