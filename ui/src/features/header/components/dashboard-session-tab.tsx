import type {
  DragEvent as ReactDragEvent,
  KeyboardEvent as ReactKeyboardEvent,
} from "react";

import type { DashboardStreamState } from "../../../api/dashboard/types";
import type { FactorySessionSummary } from "../../../api/factory-sessions";
import { cn } from "../../../lib/cn";
import {
  sessionCloseLabel,
  sessionTabLabel,
  sessionTabSecondaryPath,
} from "../lib/dashboard-session-tabs-utils";
import type { getHeaderControlsMessages } from "../messages/header-controls";

const SESSION_TAB_ITEM_CLASS =
  "group relative flex h-12 min-w-0 flex-1 basis-0 items-stretch self-stretch overflow-hidden transition-[opacity,colors,box-shadow]";
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
  onClick: () => void;
}) {
  return (
    <button
      aria-haspopup="dialog"
      aria-label={label}
      className={cn(
        "flex min-h-10 min-w-10 shrink-0 self-stretch items-center rounded-xl bg-transparent px-3 py-2 text-on-surface-variant transition-colors",
        "hover:bg-af-overlay-subtle hover:text-on-surface",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring",
      )}
      onClick={onClick}
      type="button"
    >
      <span aria-hidden="true" className="text-lg leading-none">
        +
      </span>
    </button>
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
  streamStatus,
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
  streamStatus: DashboardStreamState["status"];
  tabID: string;
}) {
  const label = sessionTabLabel(session);
  const secondaryPath = sessionTabSecondaryPath(session.folderPath);
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
          status === "live" && "bg-success",
          (status === "connecting" || status === "reconnecting") &&
            "bg-primary",
          (status === "offline" || status === "recovery_failed") && "bg-error",
        )}
      />
    </span>
  );
}
