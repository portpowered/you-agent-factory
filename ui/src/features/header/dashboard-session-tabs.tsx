import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  type FormEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";

import {
  type FactorySessionSummary,
  type FactorySessionTarget,
  FactorySessionsAPIError,
  listFactorySessions,
  openFactorySession,
} from "../../api/factory-sessions";
import { DEFAULT_FACTORY_SESSION_ID } from "../../api/session-routing";
import { Button, Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, Input } from "../../components/ui";
import { cn } from "../../lib/cn";
import { DASHBOARD_BODY_TEXT_CLASS, DASHBOARD_SUPPORTING_LABELS_CLASS } from "../../components/ui/dashboard-typography";
import { useDashboardSessionStore } from "../dashboard/state/dashboardSessionStore";
import { getHeaderControlsMessages } from "./messages/header-controls";

const FACTORY_SESSIONS_QUERY_KEY = ["factory-sessions"] as const;

const SESSION_TABS_SHELL_CLASS = "mt-3 flex w-full flex-col gap-3 border-t border-af-overlay/10 pt-3";
const SESSION_HEADER_ROW_CLASS = "flex items-center justify-between gap-3";
const SESSION_SECTION_LABEL_CLASS = cn(
  "text-xs uppercase tracking-[0.18em] text-af-ink/52",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const SESSION_TAB_LIST_CLASS = "flex gap-2 overflow-x-auto pb-1";
const SESSION_TAB_BUTTON_CLASS =
  "min-w-0 shrink-0 rounded-2xl border px-3 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25";
const SESSION_TAB_ACTIVE_CLASS =
  "border-af-accent/42 bg-af-accent/12 text-af-ink";
const SESSION_TAB_INACTIVE_CLASS =
  "border-af-overlay/12 bg-af-overlay/4 text-af-ink/76 hover:border-af-overlay/24 hover:bg-af-overlay/8 hover:text-af-ink";
const SESSION_DIALOG_ERROR_CLASS =
  "rounded-xl border border-af-danger/32 bg-af-danger/8 px-3 py-2 text-sm text-af-ink";
const SESSION_TARGET_LIST_CLASS = "grid gap-2 sm:grid-cols-2";
const SESSION_TARGET_BUTTON_CLASS =
  "flex min-h-11 flex-col items-start justify-center rounded-xl border border-af-overlay/12 bg-af-overlay/4 px-3 py-2 text-left text-sm text-af-ink/82 transition-colors hover:border-af-accent/30 hover:bg-af-overlay/8 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25";

export function DashboardSessionTabs({ locale }: { locale: string }) {
  const messages = getHeaderControlsMessages(locale);
  const queryClient = useQueryClient();
  const sessionsQuery = useQuery({
    queryKey: FACTORY_SESSIONS_QUERY_KEY,
    queryFn: () => listFactorySessions(),
  });
  const openSessionMutation = useMutation({
    mutationFn: (input: Parameters<typeof openFactorySession>[0]) =>
      openFactorySession(input),
  });
  const activeSessionID = useDashboardSessionStore(
    (state) => state.selectedSessionID,
  );
  const setActiveSessionID = useDashboardSessionStore(
    (state) => state.setSelectedSessionID,
  );
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogError, setDialogError] = useState<FactorySessionsAPIError | null>(null);
  const [folderPath, setFolderPath] = useState("");
  const [discoveredTargets, setDiscoveredTargets] = useState<FactorySessionTarget[]>([]);

  const sessions = sessionsQuery.data ?? [];
  const activeSession = useMemo(
    () =>
      sessions.find((session) => session.id === activeSessionID) ??
      sessions[0] ??
      null,
    [activeSessionID, sessions],
  );

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

  async function handleInspectFolder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setDialogError(null);
    setDiscoveredTargets([]);

    try {
      const response = await openSessionMutation.mutateAsync({
        folderPath,
      });
      if (response.session) {
        await finishOpeningSession(response.session.id);
        return;
      }
      setDiscoveredTargets(response.targets ?? []);
    } catch (error) {
      setDialogError(normalizeFactorySessionsError(error));
    }
  }

  async function handleOpenTarget(target: FactorySessionTarget) {
    setDialogError(null);
    try {
      const response = await openSessionMutation.mutateAsync({
        folderPath,
        target: target.ref,
      });
      if (response.session) {
        await finishOpeningSession(response.session.id);
      }
    } catch (error) {
      setDialogError(normalizeFactorySessionsError(error));
    }
  }

  async function finishOpeningSession(sessionID: string) {
    await queryClient.invalidateQueries({
      queryKey: FACTORY_SESSIONS_QUERY_KEY,
    });
    setActiveSessionID(sessionID);
    resetDialogState();
    setDialogOpen(false);
  }

  function resetDialogState() {
    setDialogError(null);
    setDiscoveredTargets([]);
    setFolderPath("");
  }

  return (
    <>
      <div className={SESSION_TABS_SHELL_CLASS}>
        <div className={SESSION_HEADER_ROW_CLASS}>
          <p className={SESSION_SECTION_LABEL_CLASS}>{messages.sessionTabsLabel}</p>
          <OpenSessionButton
            label={messages.openSessionButtonLabel}
            onClick={() => {
              setDialogOpen(true);
            }}
          />
        </div>
        <SessionTabsContent
          activeSession={activeSession}
          error={sessionsQuery.isError ? sessionsQuery.error : null}
          isPending={sessionsQuery.isPending}
          messages={messages}
          onRetry={() => {
            void sessionsQuery.refetch();
          }}
          onSelectSession={setActiveSessionID}
          sessions={sessions}
        />
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
    <Button aria-haspopup="dialog" onClick={onClick} size="sm" tone="secondary">
      <span aria-hidden="true">+</span>
      <span>{label}</span>
    </Button>
  );
}

function SessionTabsContent({
  activeSession,
  error,
  isPending,
  messages,
  onRetry,
  onSelectSession,
  sessions,
}: {
  activeSession: FactorySessionSummary | null;
  error: unknown;
  isPending: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onRetry: () => void;
  onSelectSession: (sessionID: string) => void;
  sessions: FactorySessionSummary[];
}) {
  const sessionButtonRefs = useRef<Array<HTMLButtonElement | null>>([]);

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
      <nav aria-label={messages.sessionTabsLabel} className={SESSION_TAB_LIST_CLASS}>
        {sessions.map((session, index) => (
          <SessionTabButton
            active={session.id === activeSession?.id}
            key={session.id}
            buttonRef={(element) => {
              sessionButtonRefs.current[index] = element;
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
            session={session}
          />
        ))}
      </nav>
      {activeSession ? (
        <p className={cn("text-xs text-af-ink/58", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.activeSessionPathLabel}: {activeSession.folderPath}
        </p>
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
  onKeyDown,
  onClick,
  session,
}: {
  active: boolean;
  buttonRef: (element: HTMLButtonElement | null) => void;
  onKeyDown: (event: ReactKeyboardEvent<HTMLButtonElement>) => void;
  onClick: () => void;
  session: FactorySessionSummary;
}) {
  const label = sessionTabLabel(session);
  return (
    <button
      aria-pressed={active}
      className={cn(
        SESSION_TAB_BUTTON_CLASS,
        active ? SESSION_TAB_ACTIVE_CLASS : SESSION_TAB_INACTIVE_CLASS,
      )}
      onClick={onClick}
      onKeyDown={onKeyDown}
      ref={buttonRef}
      title={`${session.folderPath} (${session.id})`}
      type="button"
    >
      <span className="block truncate text-sm font-semibold">{label}</span>
      <span className="block truncate text-xs text-af-ink/58">
        {session.project || session.folderPath}
      </span>
    </button>
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
          <Input
            autoFocus
            disabled={isPending}
            id={folderFieldID}
            onChange={(event) => {
              onChangeFolderPath(event.target.value);
            }}
            placeholder={messages.sessionFolderFieldPlaceholder}
            value={folderPath}
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

function basename(path: string): string {
  const segments = path.split(/[\\/]/).filter((segment) => segment.length > 0);
  return segments[segments.length - 1] ?? "";
}

function normalizeFactorySessionsError(error: unknown): FactorySessionsAPIError {
  if (error instanceof FactorySessionsAPIError) {
    return error;
  }
  if (error instanceof Error) {
    // hardcoded-ui-copy-exception: non-product-diagnostic
    return new FactorySessionsAPIError(error.message, {
      code: "INTERNAL_ERROR",
    });
  }
  // hardcoded-ui-copy-exception: non-product-diagnostic
  return new FactorySessionsAPIError("Factory session request failed.", {
    code: "INTERNAL_ERROR",
  });
}
