import type { KeyboardEvent as ReactKeyboardEvent } from "react";

import type { DashboardStreamState } from "../../api/dashboard/types";
import type { FactorySessionSummary } from "../../api/factory-sessions";
import { cn } from "../../lib/cn";
import {
  sessionCloseLabel,
  sessionStreamToggleLabel,
  sessionTabLabel,
} from "./dashboard-session-tabs-utils";
import type { getHeaderControlsMessages } from "./messages/header-controls";

const SESSION_TAB_ITEM_CLASS =
  "group relative flex min-w-0 shrink-0 items-stretch border border-b-0 transition-colors";
const SESSION_TAB_BUTTON_CLASS =
  "min-w-0 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25";
const SESSION_TAB_CLOSE_BUTTON_CLASS =
  "px-2.5 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25";
const SESSION_TAB_ACTIVE_CLASS =
  "z-10 overflow-hidden rounded-t-xl rounded-b-md border-af-overlay/14 bg-af-surface/78 text-af-ink shadow-[0_-1px_0_rgba(255,255,255,0.08)_inset]";
const SESSION_TAB_INACTIVE_CLASS =
  "rounded-t-xl rounded-b-md border-af-overlay/14 bg-af-surface/72 text-af-ink/74 hover:border-af-overlay/18 hover:bg-af-surface/72 hover:text-af-ink";
const SESSION_TAB_ACTIVE_BUTTON_CLASS =
  "flex min-w-0 flex-1 items-center rounded-bl-md rounded-tl-xl px-3 py-2";
const SESSION_TAB_INACTIVE_BUTTON_CLASS =
  "flex min-w-0 flex-1 items-center rounded-bl-md rounded-tl-xl px-3 py-2";
const SESSION_TAB_ACTIVE_CONTROLS_CLASS =
  "flex items-stretch border-l border-af-overlay/14 bg-af-surface/82";
const SESSION_TAB_ACTIVE_CONTROL_BUTTON_CLASS =
  "flex items-center justify-center px-2.5 text-af-ink/62 transition-colors hover:text-af-ink";
const SESSION_TAB_ACTIVE_TOGGLE_BUTTON_CLASS =
  "border-r border-af-overlay/12";
const SESSION_TAB_INACTIVE_CLOSE_BUTTON_CLASS = cn(
  "rounded-br-md rounded-tr-xl text-af-ink/34 transition-colors",
  "hover:bg-af-overlay/8 hover:text-af-ink/68",
  "group-focus-within:bg-af-overlay/10 group-focus-within:text-af-ink/76",
  "focus-visible:bg-af-overlay/10 focus-visible:text-af-ink",
);

export function SessionTabButton({
  active,
  buttonRef,
  controlsID,
  closeDisabled,
  isStreamPaused,
  messages,
  onKeyDown,
  onClick,
  onClose,
  onToggleStreamPaused,
  session,
  streamStatus,
  tabID,
}: {
  active: boolean;
  buttonRef: (element: HTMLButtonElement | null) => void;
  controlsID: string;
  closeDisabled: boolean;
  isStreamPaused: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onKeyDown: (event: ReactKeyboardEvent<HTMLButtonElement>) => void;
  onClick: () => void;
  onClose: () => void;
  onToggleStreamPaused: () => void;
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
            aria-label={sessionStreamToggleLabel(session, isStreamPaused, messages)}
            aria-pressed={isStreamPaused}
            className={cn(
              SESSION_TAB_CLOSE_BUTTON_CLASS,
              SESSION_TAB_ACTIVE_CONTROL_BUTTON_CLASS,
              SESSION_TAB_ACTIVE_TOGGLE_BUTTON_CLASS,
            )}
            onClick={onToggleStreamPaused}
            type="button"
          >
            <SessionStreamToggleIcon paused={isStreamPaused} />
          </button>
          <button
            aria-label={sessionCloseLabel(session, messages)}
            className={cn(
              SESSION_TAB_CLOSE_BUTTON_CLASS,
              SESSION_TAB_ACTIVE_CONTROL_BUTTON_CLASS,
            )}
            disabled={closeDisabled}
            onClick={onClose}
            type="button"
          >
            {closeDisabled ? messages.closingSessionButtonLabel : "×"}
          </button>
        </div>
      ) : (
        <button
          aria-label={sessionCloseLabel(session, messages)}
          className={cn(
            SESSION_TAB_CLOSE_BUTTON_CLASS,
            SESSION_TAB_INACTIVE_CLOSE_BUTTON_CLASS,
          )}
          disabled={closeDisabled}
          onClick={onClose}
          type="button"
        >
          {closeDisabled ? messages.closingSessionButtonLabel : "×"}
        </button>
      )}
    </div>
  );
}

function SessionStreamToggleIcon({
  paused,
}: {
  paused: boolean;
}) {
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
