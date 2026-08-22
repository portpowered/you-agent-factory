import type {
  DragEvent as ReactDragEvent,
  KeyboardEvent as ReactKeyboardEvent,
  MouseEvent as ReactMouseEvent,
} from "react";

import type { DashboardStreamState } from "../../../api/dashboard/types";
import type { FactorySessionSummary } from "../../../api/factory-sessions";
import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import { cn } from "../../../lib/cn";
import {
  sessionCloseLabel,
  sessionTabLabel,
  sessionTabSecondaryPath,
} from "../lib/dashboard-session-tabs-utils";
import type { getHeaderControlsMessages } from "../messages/header-controls";

const SESSION_TAB_ITEM_CLASS =
  "group relative flex h-12 min-w-40 max-w-72 flex-none items-stretch self-stretch overflow-hidden transition-[opacity,colors,box-shadow]";
const SESSION_TAB_BUTTON_CLASS =
  "min-w-0 flex-1 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring";
const SESSION_TAB_CLOSE_BUTTON_CLASS =
  "min-h-10 min-w-10 shrink-0 px-2.5 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring";
const SESSION_TAB_ACTIVE_CLASS = cn(
  "z-10 rounded-xl bg-surface-container-low text-on-surface",
);
const SESSION_TAB_INACTIVE_CLASS =
  "rounded-xl text-on-surface-variant hover:bg-af-overlay hover:text-on-surface";
const SESSION_TAB_ACTIVE_BUTTON_CLASS =
  "flex min-w-0 flex-1 flex-col items-start rounded-l-xl px-3 py-2";
const SESSION_TAB_INACTIVE_BUTTON_CLASS =
  "flex min-w-0 flex-1 flex-col items-start rounded-l-xl px-3 py-2";
const SESSION_TAB_ACTIVE_CONTROL_BUTTON_CLASS =
  "flex items-center justify-center text-on-surface-subtle transition-colors hover:text-on-surface";
const SESSION_TAB_INACTIVE_CLOSE_BUTTON_CLASS = cn(
  "rounded-tr-xl text-on-surface-disabled transition-colors",
  "group-hover:text-on-surface-variant",
  "hover:bg-af-overlay hover:text-on-surface-variant",
  "group-focus-within:bg-af-overlay-focus group-focus-within:text-on-surface",
  "focus-visible:bg-af-overlay-focus focus-visible:text-on-surface",
);

export function OpenSessionButton({
  label,
  onClick,
}: {
  label: string;
  onClick: (event: ReactMouseEvent<HTMLButtonElement>) => void;
}) {
  return (
    <DashboardActionButton
      aria-haspopup="dialog"
      aria-label={label}
      iconOnly
      onClick={onClick}
      title={label}
      tone="outline"
    >
      <OpenSessionButtonIcon />
    </DashboardActionButton>
  );
}

function OpenSessionButtonIcon() {
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
      <path d="M12 5v14" />
      <path d="M5 12h14" />
    </svg>
  );
}

export function SessionTabButton({
  active,
  buttonRef,
  closeDisabled,
  controlsID,
  dragPreview,
  draggable,
  dropIndicator,
  messages,
  onClick,
  onClose,
  onDragEnd,
  onDragOver,
  onDragStart,
  onDrop,
  onKeyDown,
  session,
  streamState,
  tabID,
}: {
  active: boolean;
  buttonRef: (element: HTMLButtonElement | null) => void;
  closeDisabled: boolean;
  controlsID: string;
  dragPreview: boolean;
  draggable: boolean;
  dropIndicator: "before" | "after" | null;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onClick: () => void;
  onClose: () => void;
  onDragEnd: () => void;
  onDragOver: (event: ReactDragEvent<HTMLDivElement>) => void;
  onDragStart: (event: ReactDragEvent<HTMLDivElement>) => void;
  onDrop: (event: ReactDragEvent<HTMLDivElement>) => void;
  onKeyDown: (event: ReactKeyboardEvent<HTMLButtonElement>) => void;
  session: FactorySessionSummary;
  streamState: DashboardStreamState;
  tabID: string;
}) {
  const label = sessionTabLabel(session);
  const secondaryPath = sessionTabSecondaryPath(session.folderPath);
  const streamStatusID = `${tabID}-stream-status`;
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: drag-and-drop is attached to the tab shell while the semantic tab control remains the inner button.
    <div
      className={cn(
        SESSION_TAB_ITEM_CLASS,
        active ? SESSION_TAB_ACTIVE_CLASS : SESSION_TAB_INACTIVE_CLASS,
        dropIndicator === "before" &&
          "shadow-[-2px_0_0_0_var(--color-primary)]",
        dropIndicator === "after" && "shadow-[2px_0_0_0_var(--color-primary)]",
        dragPreview && "bg-surface-container-low text-on-surface-disabled",
      )}
      draggable={draggable}
      onDragEnd={onDragEnd}
      onDragOver={onDragOver}
      onDragStart={onDragStart}
      onDrop={onDrop}
      data-dragging={dragPreview}
      title={`${session.folderPath} (${session.id})`}
    >
      <button
        aria-controls={controlsID}
        aria-describedby={streamStatusID}
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
          <SessionTabStatusIndicator
            id={streamStatusID}
            message={streamState.message}
            status={streamState.status}
          />
          <span className="truncate text-sm font-semibold">{label}</span>
        </span>
        <span
          className="block min-w-0 max-w-full truncate text-[11px] text-on-surface-subtle"
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
  id,
  message,
  status,
}: {
  id: string;
  message: string;
  status: DashboardStreamState["status"];
}) {
  return (
    <>
      <span
        aria-hidden="true"
        className="relative inline-flex size-2.5 shrink-0"
      >
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
            status === "live" && "bg-success",
            (status === "connecting" || status === "reconnecting") &&
              "bg-primary",
            (status === "offline" || status === "recovery_failed") &&
              "bg-error",
          )}
        />
      </span>
      <span aria-label={message} className="sr-only" id={id} role="img" />
    </>
  );
}
