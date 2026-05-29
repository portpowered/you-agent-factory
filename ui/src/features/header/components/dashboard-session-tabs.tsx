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
import { DASHBOARD_BODY_TEXT_CLASS } from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import { useDashboardStreamStore } from "../../dashboard/state/dashboardStreamStore";
import {
  type DashboardSessionTabsState,
  useDashboardSessionTabsState,
} from "../hooks/use-dashboard-session-tabs-state";
import {
  normalizeFactorySessionsError,
  sessionCloseLabel,
  sessionPanelID,
  sessionTabID,
  sessionTabLabel,
  sessionTabSecondaryPath,
} from "../lib/dashboard-session-tabs-utils";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { OpenSessionDialog } from "./dashboard-session-tabs-open-dialog";

const SESSION_TABS_SHELL_CLASS = "grid min-w-0 max-w-full flex-1 gap-2";
const SESSION_TABS_ROW_CLASS =
  "flex min-w-0 max-w-full items-stretch gap-1 overflow-hidden";
const SESSION_TAB_LIST_CLASS =
  "flex h-full min-w-full w-max items-left gap-1";
const SESSION_TAB_ITEM_CLASS =
  "group relative flex min-h-0 h-full min-w-0 shrink-0 items-stretch self-stretch transition-colors";
const SESSION_TAB_BUTTON_CLASS =
  "min-w-0 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring";
const SESSION_TAB_CLOSE_BUTTON_CLASS =
  "px-2.5 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring";
const OPEN_SESSION_TAB_BUTTON_CLASS = cn(
  "flex shrink-0 self-stretch items-center rounded-t-2xl bg-transparent px-3 py-2 text-af-text-muted transition-colors",
  "hover:bg-af-overlay-subtle hover:text-af-text",
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring",
);
const SESSION_TAB_ACTIVE_CLASS = cn(
  "z-10 -mb-0.5 overflow-visible rounded-t-2xl rounded-b-none bg-af-surface-subtle text-af-text",
  "before:pointer-events-none before:absolute before:-left-4 before:-bottom-0 before:h-4 before:w-4 before:bg-[radial-gradient(circle_at_top_left,transparent_1rem,var(--color-af-surface-subtle)_1rem)]",
  "after:pointer-events-none after:absolute after:-right-4 after:-bottom-0 after:h-4 after:w-4 after:bg-[radial-gradient(circle_at_top_right,transparent_1rem,var(--color-af-surface-subtle)_1rem)]",
);
const SESSION_TAB_INACTIVE_CLASS =
  "rounded-t-xl rounded-b-none text-af-text-muted hover:bg-af-overlay hover:text-af-text";
const SESSION_TAB_ACTIVE_BUTTON_CLASS =
  "flex min-w-0 flex-1 flex-col items-start rounded-tl-xl px-3 py-2";
const SESSION_TAB_INACTIVE_BUTTON_CLASS =
  "flex min-w-0 flex-1 flex-col items-start rounded-tl-xl px-3 py-2";
const SESSION_TAB_ACTIVE_CONTROL_BUTTON_CLASS =
  "flex items-center justify-center text-af-text-subtle transition-colors hover:text-af-text";
const SESSION_TAB_INACTIVE_CLOSE_BUTTON_CLASS = cn(
  "rounded-tr-xl text-af-text-disabled transition-colors",
  "group-hover:text-af-text-muted",
  "hover:bg-af-overlay hover:text-af-text-muted",
  "group-focus-within:bg-af-overlay-focus group-focus-within:text-af-text",
  "focus-visible:bg-af-overlay-focus focus-visible:text-af-text",
);
const SESSION_DIALOG_ERROR_CLASS =
  "rounded-xl border border-af-danger-border bg-af-danger-surface px-3 py-2 text-sm text-af-danger-text";

export function DashboardSessionTabs({
  locale,
  state,
}: {
  locale: string;
  state?: DashboardSessionTabsState;
}) {
  const fallbackState = useDashboardSessionTabsState();
  const sessionTabsState = state ?? fallbackState;

  return <DashboardSessionTabsView locale={locale} state={sessionTabsState} />;
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
    handleChangeFolderPath,
    handleCloseSession,
    handleInspectFolder,
    handleOpenTarget,
    openSessionMutation,
    resetDialogState,
    selectedTargetValue,
    sessions,
    sessionsQuery,
    setActiveSessionID,
    setDialogOpen,
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
              closeSessionMutation.isPending
                ? closeSessionMutation.variables
                : null
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
          messages={messages}
          onChangeFolderPath={handleChangeFolderPath}
          onInspectFolder={handleInspectFolder}
          onOpenTarget={handleOpenTarget}
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
    <button
      aria-haspopup="dialog"
      aria-label={label}
      className={OPEN_SESSION_TAB_BUTTON_CLASS}
      onClick={onClick}
      type="button"
    >
      <span aria-hidden="true" className="text-lg leading-none">
        +
      </span>
    </button>
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: existing session tab orchestration stays intact; this change only adjusts the status indicator styling.
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
      <p
        className={cn("text-sm text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}
      >
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
        <p className={cn("text-sm text-af-text", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.sessionsEmptyTitle}
        </p>
        <OpenSessionButton
          label={messages.openSessionButtonLabel}
          onClick={onOpenSession}
        />
      </>
    );
  }

  function focusSessionButton(index: number) {
    sessionButtonRefs.current[index]?.focus();
  }

  function moveSessionFocus(currentIndex: number, offset: number) {
    const nextIndex =
      (currentIndex + offset + sessions.length) % sessions.length;
    const nextSession = sessions[nextIndex];
    if (!nextSession) {
      return;
    }
    onSelectSession(nextSession.id);
    focusSessionButton(nextIndex);
  }

  return (
    <>
      <nav
        aria-label={messages.sessionTabsLabel}
        className="min-w-0 overflow-x-auto overflow-y-hidden"
      >
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
      <OpenSessionButton
        label={messages.openSessionButtonLabel}
        onClick={onOpenSession}
      />
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
  const secondaryPath = sessionTabSecondaryPath(session.folderPath);
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
        className={cn(
          SESSION_TAB_BUTTON_CLASS,
          active
            ? SESSION_TAB_ACTIVE_BUTTON_CLASS
            : SESSION_TAB_INACTIVE_BUTTON_CLASS,
        )}
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
        <span
          className="block truncate text-[11px] text-af-text-subtle"
          title={session.folderPath}
        >
          {secondaryPath}
        </span>
      </button>
      <button
        aria-label={sessionCloseLabel(session, messages)}
        className={cn(
          SESSION_TAB_CLOSE_BUTTON_CLASS,
          active
            ? SESSION_TAB_ACTIVE_CONTROL_BUTTON_CLASS
            : SESSION_TAB_INACTIVE_CLOSE_BUTTON_CLASS,
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
    <span aria-hidden="true" className="relative inline-flex size-2.5 shrink-0">
      {status === "live" ? (
        <span
          className={cn(
            "absolute -inset-1 animate-ping rounded-full",
            "bg-[var(--color-af-session-live-ping)]",
          )}
          data-testid="dashboard-session-live-ping"
        />
      ) : null}
      <span
        className={cn(
          "absolute inset-0 rounded-full",
          status === "live" && "bg-af-success",
          status === "connecting" && "bg-af-accent",
          status === "offline" && "bg-af-danger",
        )}
      />
    </span>
  );
}
