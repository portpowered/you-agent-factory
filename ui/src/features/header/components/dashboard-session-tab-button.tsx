import type { KeyboardEvent as ReactKeyboardEvent } from "react";

import type { DashboardStreamState } from "../../../api/dashboard/types";
import type { FactorySessionSummary } from "../../../api/factory-sessions";
import { cn } from "../../../lib/cn";
import {
  sessionCloseLabel,
  sessionTabLabel,
} from "../lib/dashboard-session-tabs-utils";
import type { getHeaderControlsMessages } from "../messages/header-controls";

const SESSION_TAB_ITEM_CLASS =
  "group relative flex min-h-0 h-full min-w-0 shrink-0 items-stretch self-stretch border border-b-0 transition-colors";
const SESSION_TAB_BUTTON_CLASS =
  "min-w-0 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25";
const SESSION_TAB_CLOSE_BUTTON_CLASS =
  "px-2.5 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25";
const SESSION_TAB_ACTIVE_CLASS =
  "z-10 -mb-px overflow-visible rounded-t-2xl rounded-b-none border-af-overlay/10 bg-af-surface/72 text-af-ink shadow-[0_8px_18px_rgb(from_var(--color-af-ink)_r_g_b_/_0.08)] before:pointer-events-none before:absolute before:-bottom-0.5 before:-left-4 before:h-4 before:w-4 before:rounded-full before:shadow-[8px_8px_0_0_var(--color-af-surface)] after:pointer-events-none after:absolute after:-right-4 after:-bottom-0.5 after:h-4 after:w-4 after:rounded-full after:shadow-[-8px_8px_0_0_var(--color-af-surface)]";
const SESSION_TAB_INACTIVE_CLASS =
  "rounded-t-xl rounded-b-none border-af-overlay/10 bg-af-surface/52 text-af-ink/74 hover:border-af-overlay/14 hover:bg-af-surface/60 hover:text-af-ink";
const SESSION_TAB_ACTIVE_BUTTON_CLASS =
  "flex min-w-0 flex-1 items-center rounded-tl-xl px-3 py-2";
const SESSION_TAB_INACTIVE_BUTTON_CLASS =
  "flex min-w-0 flex-1 items-center rounded-tl-xl px-3 py-2";
const SESSION_TAB_ACTIVE_CONTROLS_CLASS =
  "flex items-stretch";
const SESSION_TAB_ACTIVE_CONTROL_BUTTON_CLASS =
  "flex items-center justify-center px-2.5 text-af-ink/62 transition-colors hover:text-af-ink";
const SESSION_TAB_INACTIVE_CLOSE_BUTTON_CLASS = cn(
  "rounded-tr-xl text-af-ink/34 transition-colors",
  "hover:bg-af-overlay/8 hover:text-af-ink/68",
  "group-focus-within:bg-af-overlay/10 group-focus-within:text-af-ink/76",
  "focus-visible:bg-af-overlay/10 focus-visible:text-af-ink",
);

export function SessionTabButton({
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
  const closeButtonLabel = closeDisabled
    ? messages.closingSessionButtonLabel
    : sessionCloseLabel(session, messages);
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
        aria-selected={active}
        className={cn(
          SESSION_TAB_BUTTON_CLASS,
          active ? SESSION_TAB_ACTIVE_BUTTON_CLASS : SESSION_TAB_INACTIVE_BUTTON_CLASS,
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
      </button>
      {active ? (
        <div className={SESSION_TAB_ACTIVE_CONTROLS_CLASS}>
          <button
            aria-label={closeButtonLabel}
            className={cn(
              SESSION_TAB_CLOSE_BUTTON_CLASS,
              SESSION_TAB_ACTIVE_CONTROL_BUTTON_CLASS,
            )}
            disabled={closeDisabled}
            onClick={onClose}
            type="button"
          >
            <SessionTabCloseIcon pending={closeDisabled} />
          </button>
        </div>
      ) : (
        <button
          aria-label={closeButtonLabel}
          className={cn(
            SESSION_TAB_CLOSE_BUTTON_CLASS,
            SESSION_TAB_INACTIVE_CLOSE_BUTTON_CLASS,
          )}
          disabled={closeDisabled}
          onClick={onClose}
          type="button"
        >
          <SessionTabCloseIcon pending={closeDisabled} />
        </button>
      )}
    </div>
  );
}

function SessionTabCloseIcon({
  pending,
}: {
  pending: boolean;
}) {
  if (!pending) {
    return <span aria-hidden="true">×</span>;
  }

  return (
    <svg
      aria-hidden="true"
      className="animate-spin"
      fill="none"
      height="14"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
      width="14"
    >
      <circle cx="12" cy="12" opacity="0.25" r="8" />
      <path d="M12 4a8 8 0 0 1 8 8" />
    </svg>
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
    />
  );
}
