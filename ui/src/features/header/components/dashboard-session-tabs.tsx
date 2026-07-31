import { Dialog } from "@you-agent-factory/components/overlays";
import { Button, Text } from "@you-agent-factory/components/primitives";
import {
  type DragEvent as ReactDragEvent,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import type { DashboardStreamState } from "../../../api/dashboard/types";
import type { FactorySessionSummary } from "../../../api/factory-sessions";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { AlertPanel } from "../../../components/ui/alert-panel";
import { useDashboardStreamStore } from "../../dashboard/state/dashboardStreamStore";
import {
  type DashboardSessionTabsState,
  useDashboardSessionTabsState,
} from "../hooks/use-dashboard-session-tabs-state";
import {
  normalizeFactorySessionsError,
  sessionPanelID,
  sessionTabID,
} from "../lib/dashboard-session-tabs-utils";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { OpenSessionButton, SessionTabButton } from "./dashboard-session-tab";
import { OpenSessionDialog } from "./dashboard-session-tabs-open-dialog";

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
    handleCancelInitConfirmation,
    handleChangeFolderPath,
    handleCloseSession,
    handleCreateNewFactory,
    handleInspectFolder,
    handleOpenTarget,
    moveSessionTab,
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
      <div className="grid min-w-0 max-w-full gap-2">
        <div className="flex min-w-0 max-w-full items-stretch gap-1">
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
            onReorderSession={moveSessionTab}
            onSelectSession={setActiveSessionID}
            sessions={sessions}
            streamStatus={streamStatus}
          />
        </div>
        {closeError ? (
          <AlertPanel role="alert" tone="danger">
            {closeError.message}
          </AlertPanel>
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
          isPending={openSessionMutation.isPending}
          isValidatePending={validateFolderMutation.isPending}
          messages={messages}
          onCancelInitConfirmation={handleCancelInitConfirmation}
          onChangeFolderPath={handleChangeFolderPath}
          onCreateNewFactory={() => {
            void handleCreateNewFactory();
          }}
          onInspectFolder={handleInspectFolder}
          onOpenTarget={handleOpenTarget}
          selectedTargetValue={selectedTargetValue}
        />
      </Dialog>
    </>
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: keeps tab focus, drag ordering, and empty/error states in one local renderer.
function SessionTabsContent({
  activeSession,
  closingSessionID,
  error,
  isPending,
  messages,
  onCloseSession,
  onOpenSession,
  onReorderSession,
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
  onReorderSession: (sessionID: string, targetIndex: number) => void;
  onRetry: () => void;
  onSelectSession: (sessionID: string) => void;
  sessions: FactorySessionSummary[];
  streamStatus: DashboardStreamState["status"];
}) {
  const sessionButtonRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const sessionTabsID = useId();
  const [draggedSessionID, setDraggedSessionID] = useState<string | null>(null);
  const [dropTargetIndex, setDropTargetIndex] = useState<number | null>(null);

  if (isPending) {
    return (
      <Text className="text-sm text-on-surface-variant">
        {messages.loadingSessionsLabel}
      </Text>
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
        <Text className="text-sm text-on-surface">
          {messages.sessionsEmptyTitle}
        </Text>
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

  function dropInsertionIndex(
    event: ReactDragEvent<HTMLDivElement>,
    index: number,
  ): number {
    const bounds = event.currentTarget.getBoundingClientRect();
    return event.clientX >= bounds.left + bounds.width / 2 ? index + 1 : index;
  }

  return (
    <>
      <nav
        aria-label={messages.sessionTabsLabel}
        className="min-w-0 overflow-x-auto overscroll-x-contain"
      >
        <div
          aria-orientation="horizontal"
          className="inline-flex h-full min-w-max items-stretch gap-1 pr-1"
          role="tablist"
        >
          {sessions.map((session, index) => (
            <SessionTabButton
              active={session.id === activeSession?.id}
              buttonRef={(element) => {
                sessionButtonRefs.current[index] = element;
              }}
              controlsID={sessionPanelID(sessionTabsID, session.id)}
              dragPreview={draggedSessionID === session.id}
              draggable={sessions.length > 1}
              dropIndicator={
                dropTargetIndex === index
                  ? "before"
                  : dropTargetIndex === index + 1
                    ? "after"
                    : null
              }
              key={session.id}
              onDragEnd={() => {
                setDraggedSessionID(null);
                setDropTargetIndex(null);
              }}
              onDragOver={(event) => {
                if (!draggedSessionID || draggedSessionID === session.id) {
                  return;
                }
                event.preventDefault();
                setDropTargetIndex(dropInsertionIndex(event, index));
              }}
              onDragStart={(event) => {
                setDraggedSessionID(session.id);
                setDropTargetIndex(index);
                event.dataTransfer.effectAllowed = "move";
                event.dataTransfer.setData("text/plain", session.id);
              }}
              onDrop={(event) => {
                if (!draggedSessionID) {
                  return;
                }
                event.preventDefault();
                const nextIndex =
                  dropTargetIndex ?? dropInsertionIndex(event, index);
                onReorderSession(draggedSessionID, nextIndex);
                setDraggedSessionID(null);
                setDropTargetIndex(null);
              }}
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
      <Text className="text-sm text-on-surface">{label}</Text>
      <Button onClick={onRetry} size="sm" tone="outline">
        {messages.retrySessionsLabel}
      </Button>
    </div>
  );
}
