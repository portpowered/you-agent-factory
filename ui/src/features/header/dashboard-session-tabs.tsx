import {
  type ChangeEvent,
  type FormEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  useEffect,
  useId,
  useRef,
} from "react";

import type { DashboardStreamState } from "../../api/dashboard/types";
import {
  type FactorySessionSummary,
  type FactorySessionTarget,
  FactorySessionsAPIError,
} from "../../api/factory-sessions";
import { DEFAULT_FACTORY_SESSION_ID } from "../../api/session-routing";
import { Button, Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, Input } from "../../components/ui";
import { cn } from "../../lib/cn";
import { DASHBOARD_BODY_TEXT_CLASS } from "../../components/ui/dashboard-typography";
import { useDashboardStreamStore } from "../dashboard/state/dashboardStreamStore";
import { DashboardHeaderActionButton } from "./components/dashboard-header-action-button";
import { getHeaderControlsMessages } from "./messages/header-controls";
import {
  type DashboardSessionTabsState,
  useDashboardSessionTabsState,
} from "./use-dashboard-session-tabs-state";

const SESSION_TABS_SHELL_CLASS = "grid min-w-0 flex-1 gap-2";
const SESSION_TABS_ROW_CLASS = "flex min-w-0 items-center gap-1.5";
const SESSION_TAB_LIST_CLASS = "flex min-w-0 flex-1 items-end gap-1 overflow-x-auto pb-1";
const SESSION_SECTION_LABEL_CLASS =
  "text-xs uppercase tracking-[0.18em] text-af-ink/52";
const SESSION_TAB_ITEM_CLASS =
  "group relative flex min-w-0 shrink-0 items-stretch rounded-t-xl rounded-b-md border border-b-0 transition-colors";
const SESSION_TAB_BUTTON_CLASS =
  "min-w-0 flex-1 rounded-l-[inherit] px-3 py-2 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25";
const SESSION_TAB_CLOSE_BUTTON_CLASS =
  "rounded-r-[inherit] px-2.5 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25";
const SESSION_TAB_ACTIVE_CLASS =
  "z-10 border-af-overlay/14 bg-af-surface/72 text-af-ink shadow-[0_-1px_0_rgba(255,255,255,0.08)_inset]";
const SESSION_TAB_INACTIVE_CLASS =
  "border-af-overlay/14 bg-af-surface/72 text-af-ink/74 hover:border-af-overlay/18 hover:bg-af-surface/72 hover:text-af-ink";
const SESSION_DIALOG_ERROR_CLASS =
  "rounded-xl border border-af-danger/32 bg-af-danger/8 px-3 py-2 text-sm text-af-ink";
const SESSION_TARGET_LIST_CLASS = "grid gap-2 sm:grid-cols-2";
const SESSION_TARGET_BUTTON_CLASS =
  "flex min-h-11 flex-col items-start justify-center rounded-xl border border-af-overlay/12 bg-af-overlay/4 px-3 py-2 text-left text-sm text-af-ink/82 transition-colors hover:border-af-accent/30 hover:bg-af-overlay/8 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25";

export function DashboardSessionTabs({
  hideOpenButton = false,
  locale,
  state,
}: {
  hideOpenButton?: boolean;
  locale: string;
  state?: DashboardSessionTabsState;
}) {
  const messages = getHeaderControlsMessages(locale);
  const streamStatus = useDashboardStreamStore(
    (state) => state.streamState.status,
  );
  const sessionTabsState = state ?? useDashboardSessionTabsState();
  const {
    activeSession,
    activeSessionID,
    closeError,
    closeSessionMutation,
    dialogError,
    dialogOpen,
    discoveredTargets,
    folderPath,
    handleCloseSession,
    handleInspectFolder,
    handleOpenTarget,
    openSessionMutation,
    resetDialogState,
    sessions,
    sessionsQuery,
    setActiveSessionID,
    setDialogOpen,
    setFolderPath,
  } = sessionTabsState;

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
          folderPath={folderPath}
          isPending={openSessionMutation.isPending}
          messages={messages}
          onChangeFolderPath={setFolderPath}
          onInspectFolder={handleInspectFolder}
          onOpenTarget={handleOpenTarget}
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
      <p className={cn("text-sm text-af-ink/76", DASHBOARD_BODY_TEXT_CLASS)}>
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

function SessionTabButton({
  active,
  buttonRef,
  controlsID,
  closeDisabled,
  messages,
  onKeyDown,
  onClick,
  onClose,
  session,
  streamStatus,
  tabID,
}: {
  active: boolean;
  buttonRef: (element: HTMLButtonElement | null) => void;
  controlsID: string;
  closeDisabled: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onKeyDown: (event: ReactKeyboardEvent<HTMLButtonElement>) => void;
  onClick: () => void;
  onClose: () => void;
  session: FactorySessionSummary;
  streamStatus: DashboardStreamState["status"];
  tabID: string;
}) {
  const label = sessionTabLabel(session);
  const statusLabel = sessionStreamStatusLabel(streamStatus, messages);
  return (
    <div
      className={cn(
        SESSION_TAB_ITEM_CLASS,
        active ? SESSION_TAB_ACTIVE_CLASS : SESSION_TAB_INACTIVE_CLASS,
      )}
      title={`${session.folderPath} (${session.id}) · ${statusLabel}`}
    >
      <button
        aria-controls={controlsID}
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
        <span className="block truncate text-[11px] text-af-ink/54">
          {session.project || session.folderPath}
        </span>
      </button>
      <button
        aria-label={sessionCloseLabel(session, messages)}
        className={cn(
          SESSION_TAB_CLOSE_BUTTON_CLASS,
          active
            ? "text-af-ink/58 hover:text-af-ink"
            : "text-af-ink/42 group-hover:text-af-ink/68",
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

function OpenSessionDialog({
  dialogError,
  discoveredTargets,
  folderPath,
  isPending,
  messages,
  onChangeFolderPath,
  onInspectFolder,
  onOpenTarget,
}: {
  dialogError: FactorySessionsAPIError | null;
  discoveredTargets: FactorySessionTarget[];
  folderPath: string;
  isPending: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onChangeFolderPath: (value: string) => void;
  onInspectFolder: (event: FormEvent<HTMLFormElement>) => void;
  onOpenTarget: (target: FactorySessionTarget) => void;
}) {
  const folderFieldID = useId();
  const folderPickerInputRef = useRef<HTMLInputElement | null>(null);

  async function handleOpenFolderPicker() {
    const directoryHandle = await pickDirectoryHandle();
    if (directoryHandle) {
      const selectedPath = readDirectoryHandlePath(directoryHandle);
      if (selectedPath) {
        onChangeFolderPath(selectedPath);
        return;
      }
    }

    folderPickerInputRef.current?.click();
  }

  function handleSelectFolder(event: ChangeEvent<HTMLInputElement>) {
    const nextFolderPath = extractSelectedFolderPath(event.target.files);
    if (nextFolderPath) {
      onChangeFolderPath(nextFolderPath);
    }
    event.target.value = "";
  }

  return (
    <DialogContent closeDisabled={isPending}>
      <DialogHeader>
        <DialogTitle>{messages.openSessionDialogTitle}</DialogTitle>
        <DialogDescription>
          {messages.openSessionDialogDescription}
        </DialogDescription>
      </DialogHeader>
      <form className="grid gap-4" onSubmit={onInspectFolder}>
        <div className="grid gap-2">
          <label className={SESSION_SECTION_LABEL_CLASS} htmlFor={folderFieldID}>
            {messages.sessionFolderFieldLabel}
          </label>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input
              autoFocus
              className="flex-1"
              disabled={isPending}
              id={folderFieldID}
              onChange={(event) => {
                onChangeFolderPath(event.target.value);
              }}
              placeholder={messages.sessionFolderFieldPlaceholder}
              value={folderPath}
            />
            <Button
              disabled={isPending}
              onClick={handleOpenFolderPicker}
              size="sm"
              tone="outline"
              type="button"
            >
              {messages.browseSessionFolderButtonLabel}
            </Button>
          </div>
          <input
            {...({ directory: "", webkitdirectory: "" } as Record<string, string>)}
            className="sr-only"
            disabled={isPending}
            onChange={handleSelectFolder}
            ref={folderPickerInputRef}
            tabIndex={-1}
            type="file"
          />
        </div>
        {dialogError ? (
          <p className={SESSION_DIALOG_ERROR_CLASS} role="alert">
            {dialogError.message}
          </p>
        ) : null}
        <div className="flex justify-end">
          <Button disabled={isPending} type="submit">
            {isPending
              ? messages.openSessionSubmitPendingLabel
              : messages.openSessionSubmitLabel}
          </Button>
        </div>
      </form>
      {discoveredTargets.length > 0 ? (
        <SessionTargetPicker
          isPending={isPending}
          messages={messages}
          onOpenTarget={onOpenTarget}
          targets={discoveredTargets}
        />
      ) : null}
    </DialogContent>
  );
}

function SessionTargetPicker({
  isPending,
  messages,
  onOpenTarget,
  targets,
}: {
  isPending: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onOpenTarget: (target: FactorySessionTarget) => void;
  targets: FactorySessionTarget[];
}) {
  return (
    <section aria-label={messages.targetPickerTitle} className="grid gap-3">
      <div className="grid gap-1">
        <p className={SESSION_SECTION_LABEL_CLASS}>{messages.targetPickerTitle}</p>
        <p className={cn("text-sm text-af-ink/72", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.targetPickerHint}
        </p>
      </div>
      <div className={SESSION_TARGET_LIST_CLASS}>
        {targets.map((target) => (
          <button
            className={SESSION_TARGET_BUTTON_CLASS}
            disabled={isPending}
            key={`${target.ref.kind}:${target.ref.name ?? ""}:${target.factoryDir}`}
            onClick={() => {
              onOpenTarget(target);
            }}
            type="button"
          >
            <span className="font-semibold text-af-ink">{target.label}</span>
            <span className="truncate text-xs text-af-ink/58">
              {target.project || target.factoryDir}
            </span>
          </button>
        ))}
      </div>
    </section>
  );
}

function sessionTabLabel(session: FactorySessionSummary): string {
  const folderName = basename(session.folderPath) || session.project || "factory";
  const targetName =
    session.target.kind === "default" ? "default" : session.target.name || "named";
  return `${folderName} / ${targetName}`;
}

function sessionCloseLabel(
  session: FactorySessionSummary,
  messages: ReturnType<typeof getHeaderControlsMessages>,
): string {
  return messages.sessionTabCloseLabelTemplate.replace(
    "{{sessionLabel}}",
    sessionTabLabel(session),
  );
}

function sessionStreamStatusLabel(
  status: DashboardStreamState["status"],
  messages: ReturnType<typeof getHeaderControlsMessages>,
): string {
  if (status === "live") {
    return messages.streamStatusLiveLabel;
  }
  if (status === "offline") {
    return messages.streamStatusOfflineLabel;
  }

  return messages.streamStatusConnectingLabel;
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
        <span className="absolute inset-0 animate-ping rounded-full bg-af-success opacity-35" />
      ) : null}
    </span>
  );
}

function basename(path: string): string {
  const segments = path.split(/[\\/]/).filter((segment) => segment.length > 0);
  return segments[segments.length - 1] ?? "";
}

function sessionTabID(sessionTabsID: string, sessionID: string): string {
  return `${sessionTabsID}-tab-${sessionDOMIDFragment(sessionID)}`;
}

function sessionPanelID(sessionTabsID: string, sessionID: string): string {
  return `${sessionTabsID}-panel-${sessionDOMIDFragment(sessionID)}`;
}

function sessionDOMIDFragment(value: string): string {
  return value.replace(/[^a-zA-Z0-9_-]+/g, "-");
}

function normalizeFactorySessionsError(error: unknown): FactorySessionsAPIError {
  if (error instanceof FactorySessionsAPIError) {
    return error;
  }
  return new FactorySessionsAPIError(
    "The dashboard could not complete the factory session request.",
    {
      code: "INTERNAL_ERROR",
      responseBody: error,
    },
  );
}

function extractSelectedFolderPath(
  files: FileList | File[] | null,
): string | null {
  const firstFile = firstSelectedFile(files);
  if (!firstFile) {
    return null;
  }

  const absolutePath = readSelectedFilePath(firstFile);
  if (absolutePath) {
    return dirname(absolutePath);
  }
  return null;
}

async function pickDirectoryHandle(): Promise<FileSystemDirectoryHandle | null> {
  const browserWithDirectoryPicker = globalThis as typeof globalThis & {
    showDirectoryPicker?: () => Promise<FileSystemDirectoryHandle>;
  };
  if (typeof browserWithDirectoryPicker.showDirectoryPicker !== "function") {
    return null;
  }

  try {
    return await browserWithDirectoryPicker.showDirectoryPicker();
  } catch (error) {
    if (isDirectoryPickerAbortError(error)) {
      return null;
    }
    throw error;
  }
}

function firstSelectedFile(files: FileList | File[] | null): File | null {
  if (!files) {
    return null;
  }

  if (Array.isArray(files)) {
    return files[0] ?? null;
  }

  return files.item(0);
}

function readSelectedFilePath(file: File): string | null {
  const pathValue = (file as File & { path?: string }).path;
  return typeof pathValue === "string" && isAbsolutePath(pathValue)
    ? pathValue
    : null;
}

function readDirectoryHandlePath(handle: FileSystemDirectoryHandle): string | null {
  const pathValue = (handle as FileSystemDirectoryHandle & { path?: string }).path;
  if (typeof pathValue === "string" && isAbsolutePath(pathValue)) {
    return pathValue;
  }
  return null;
}

function isDirectoryPickerAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

function isAbsolutePath(path: string): boolean {
  return path.startsWith("/") || /^[a-zA-Z]:[\\/]/.test(path);
}

function dirname(path: string): string {
  return path.replace(/[\\/][^\\/]+$/, "");
}
