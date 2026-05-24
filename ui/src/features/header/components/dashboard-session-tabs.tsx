import {
  type KeyboardEvent as ReactKeyboardEvent,
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
import { DashboardHeaderActionButton } from "./dashboard-header-action-button";
import { OpenSessionDialog } from "./dashboard-session-tabs-open-dialog";
import {
  normalizeFactorySessionsError,
  sessionCloseLabel,
  sessionPanelID,
  sessionTabID,
  sessionTabLabel,
} from "../lib/dashboard-session-tabs-utils";
import { getHeaderControlsMessages } from "../messages/header-controls";
import {
  type DashboardSessionTabsState,
  useDashboardSessionTabsState,
} from "../hooks/use-dashboard-session-tabs-state";

const SESSION_TABS_SHELL_CLASS = "grid min-w-0 flex-1 gap-2";
const SESSION_TABS_ROW_CLASS = "flex min-w-0 items-center gap-1.5";
const SESSION_TAB_LIST_CLASS = "flex min-w-0 flex-1 items-end gap-1 overflow-x-auto pb-1";
const SESSION_TAB_ITEM_CLASS =
  "group relative flex min-w-0 shrink-0 items-stretch rounded-t-xl rounded-b-md border border-b-0 transition-colors";
const SESSION_TAB_BUTTON_CLASS =
  "min-w-0 flex-1 rounded-bl-md rounded-tl-xl px-3 py-2 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring";
const SESSION_TAB_CLOSE_BUTTON_CLASS =
  "rounded-br-md rounded-tr-xl px-2.5 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring";
const SESSION_TAB_ACTIVE_CLASS =
  "z-10 border-af-border-strong bg-af-surface-raised text-af-text shadow-af-card";
const SESSION_TAB_INACTIVE_CLASS =
  "border-af-border bg-af-surface-subtle text-af-text-muted hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text";
const SESSION_DIALOG_ERROR_CLASS =
  "rounded-xl border border-af-danger-border bg-af-danger-surface px-3 py-2 text-sm text-af-danger-text";

export function DashboardSessionTabs({
  hideOpenButton = false,
  locale,
  state,
}: {
  hideOpenButton?: boolean;
  locale: string;
  state?: DashboardSessionTabsState;
}) {
  const fallbackState = useDashboardSessionTabsState();
  const sessionTabsState = state ?? fallbackState;

  return (
    <DashboardSessionTabsView
      hideOpenButton={hideOpenButton}
      locale={locale}
      state={sessionTabsState}
    />
  );
}

function DashboardSessionTabsView({
  hideOpenButton,
  locale,
  state,
}: {
  hideOpenButton: boolean;
  locale: string;
  state: DashboardSessionTabsState;
}) {
  const messages = getHeaderControlsMessages(locale);
  const streamStatus = useDashboardStreamStore(
    (storeState) => storeState.streamState.status,
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
    setSelectedTargetValue,
    validateFolderMutation,
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
            onSelectSession={setActiveSessionID}
            sessions={sessions}
            streamStatus={streamStatus}
          />
          {hideOpenButton ? null : (
            <OpenSessionButton
              label={messages.openSessionButtonLabel}
              onClick={() => {
                setDialogOpen(true);
              }}
            />
          )}
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

function OpenSessionButton({
  label,
  onClick,
}: {
  label: string;
  onClick: () => void;
}) {
  return (
    <DashboardHeaderActionButton
      aria-haspopup="dialog"
      aria-label={label}
      className="self-center"
      compact
      onClick={onClick}
    >
      <span aria-hidden="true" className="text-lg leading-none">+</span>
    </DashboardHeaderActionButton>
  );
}

function SessionTabsContent({
  activeSession,
  closingSessionID,
  error,
  isPending,
  messages,
  onCloseSession,
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
  onRetry: () => void;
  onSelectSession: (sessionID: string) => void;
  sessions: FactorySessionSummary[];
  streamStatus: DashboardStreamState["status"];
}) {
  const sessionButtonRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const sessionTabsID = useId();

  if (isPending) {
    return (
      <p className={cn("text-sm text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
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
      <p className={cn("text-sm text-af-text", DASHBOARD_BODY_TEXT_CLASS)}>
        {messages.sessionsEmptyTitle}
      </p>
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
                    onSelectSession(sessions.at(-1)?.id ?? session.id);
                    focusSessionButton(sessions.length - 1);
                    return;
                  case "Delete":
                  case "Backspace":
                    event.preventDefault();
                    onCloseSession(session.id);
                    return;
                  default:
                    return;
                }
              }}
              onClick={() => {
                onSelectSession(session.id);
              }}
              closeDisabled={closingSessionID === session.id}
              messages={messages}
              onClose={() => {
                onCloseSession(session.id);
              }}
              session={session}
              streamStatus={streamStatus}
              tabID={sessionTabID(sessionTabsID, session.id)}
            />
          ))}
        </div>
      </nav>
      {activeSession ? (
        <div
          aria-labelledby={sessionTabID(sessionTabsID, activeSession.id)}
          id={sessionPanelID(sessionTabsID, activeSession.id)}
          role="tabpanel"
        />
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
      <p className={cn("text-sm text-af-text", DASHBOARD_BODY_TEXT_CLASS)}>
        {label}
      </p>
      <Button onClick={onRetry} size="sm" tone="outline">
        {messages.retrySessionsLabel}
      </Button>
    </div>
  );
}
function SessionTabButton({
  active,
  buttonRef,
  closeDisabled,
  controlsID,
  messages,
  onClick,
  onClose,
  onKeyDown,
  session,
  streamStatus,
  tabID,
}: {
  active: boolean;
  buttonRef: (element: HTMLButtonElement | null) => void;
  closeDisabled: boolean;
  controlsID: string;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onClick: () => void;
  onClose: () => void;
  onKeyDown: (event: ReactKeyboardEvent<HTMLButtonElement>) => void;
  session: FactorySessionSummary;
  streamStatus: DashboardStreamState["status"];
  tabID: string;
}) {
  const label = sessionTabLabel(session);
  return (
    <div
      className={cn(
        SESSION_TAB_ITEM_CLASS,
        active ? SESSION_TAB_ACTIVE_CLASS : SESSION_TAB_INACTIVE_CLASS,
      )}
      title={`${session.folderPath} (${session.id})`}
    >
      <button
        aria-controls={controlsID}
        aria-label={label}
        aria-selected={active}
        className={SESSION_TAB_BUTTON_CLASS}
        id={tabID}
        onClick={onClick}
        onKeyDown={onKeyDown}
        ref={buttonRef}
        role="tab"
        tabIndex={active ? 0 : -1}
        type="button"
      >
        <span className="flex min-w-0 items-center gap-2">
          <SessionTabStatusIndicator status={streamStatus} />
          <span className="truncate text-sm font-semibold">{label}</span>
        </span>
        <span className="block truncate text-[11px] text-af-text-subtle">
          {session.project || session.folderPath}
        </span>
      </button>
      <button
        aria-label={sessionCloseLabel(session, messages)}
        className={cn(
          SESSION_TAB_CLOSE_BUTTON_CLASS,
          active
            ? "text-af-text-subtle hover:text-af-text"
            : "text-af-text-disabled group-hover:text-af-text-muted",
        )}
        disabled={closeDisabled}
        onClick={onClose}
        type="button"
      >
        {closeDisabled ? messages.closingSessionButtonLabel : "×"}
      </button>
    </div>
  );
}

function SessionTabStatusIndicator({
  status,
}: {
  status: DashboardStreamState["status"];
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "relative inline-flex size-2.5 shrink-0 rounded-full",
        status === "live" && "bg-af-success",
        status === "connecting" && "bg-af-accent",
        status === "offline" && "bg-af-danger",
      )}
    >
      {status === "live" ? (
        <span className="absolute inset-0 animate-ping rounded-full bg-af-success-surface" />
      ) : null}
    </span>
  );
}
