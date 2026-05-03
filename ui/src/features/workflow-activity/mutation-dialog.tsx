import { useId, type ReactNode } from "react";

import { cx } from "../../lib/cx";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard/typography";
import { EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS } from "../../components/dashboard/widget-board";

const DIALOG_OVERLAY_CLASS =
  "z-50 flex items-center justify-center bg-af-canvas/78 p-4 backdrop-blur-sm";
const DIALOG_PANEL_CLASS =
  "w-full overflow-hidden rounded-[1.6rem] border border-af-overlay/12 bg-af-surface/96 shadow-af-panel";
const DIALOG_HEADER_CLASS = "flex items-start justify-between gap-4";
const DIALOG_TITLE_CLASS = cx("m-0", DASHBOARD_SECTION_HEADING_CLASS);
const DIALOG_DESCRIPTION_CLASS = cx("m-0", DASHBOARD_BODY_TEXT_CLASS);
const DIALOG_EYEBROW_CLASS = cx(
  "mb-0 text-xs font-bold uppercase tracking-[0.16em] text-af-accent",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const DIALOG_CONTENT_CLASS = "grid gap-5 p-5 max-[900px]:p-4";
const DIALOG_CONTENT_WITH_MEDIA_CLASS =
  "min-[901px]:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]";
const DIALOG_MAIN_CLASS = "grid content-start gap-4";
const DIALOG_CLOSE_BUTTON_CLASS =
  "inline-flex h-10 w-10 items-center justify-center rounded-full border border-af-overlay/12 bg-af-overlay/4 text-af-ink/72 outline-af-accent transition hover:bg-af-overlay/10 hover:text-af-ink focus-visible:outline-2 focus-visible:outline-offset-2 disabled:cursor-not-allowed disabled:opacity-60";
const DIALOG_FOOTER_CLASS = "flex flex-wrap justify-end gap-3";

const MESSAGE_PANEL_TONE_CLASS = {
  error: "border-af-danger/30 bg-af-danger/8 text-af-danger-ink",
  neutral: "border-af-overlay/10 bg-af-overlay/4 text-af-ink/82",
} as const;

export interface DashboardMutationDialogProps {
  children: ReactNode;
  closeDisabled?: boolean;
  closeLabel?: string;
  description?: ReactNode;
  footer?: ReactNode;
  media?: ReactNode;
  onClose?: () => void;
  overlayClassName?: string;
  showCloseButton?: boolean;
  title: string;
}

export interface DashboardMessagePanelProps {
  action?: ReactNode;
  ariaLive?: "assertive" | "off" | "polite";
  children: ReactNode;
  className?: string;
  compact?: boolean;
  role?: "alert" | "status";
  title: string;
  tone?: "error" | "neutral";
}

export function DashboardMutationDialog({
  children,
  closeDisabled = false,
  closeLabel = "Close dialog",
  description,
  footer,
  media,
  onClose,
  overlayClassName = "fixed inset-0 px-5 py-6",
  showCloseButton = true,
  title,
}: DashboardMutationDialogProps) {
  const canClose = onClose !== undefined && !closeDisabled;
  const titleId = useId();
  const descriptionId = useId();

  return (
    <div className={cx(DIALOG_OVERLAY_CLASS, "relative", overlayClassName)}>
      {canClose ? (
        <button
          aria-label={closeLabel}
          className="absolute inset-0"
          onClick={onClose}
          type="button"
        />
      ) : null}
      <section
        aria-describedby={description ? descriptionId : undefined}
        aria-labelledby={titleId}
        aria-modal="true"
        className={DIALOG_PANEL_CLASS}
        role="dialog"
      >
        <div
          className={cx(DIALOG_CONTENT_CLASS, media ? DIALOG_CONTENT_WITH_MEDIA_CLASS : undefined)}
        >
          {media ? <div>{media}</div> : null}

          <div className={DIALOG_MAIN_CLASS}>
            <header className={DIALOG_HEADER_CLASS}>
              <div className="grid gap-2">
                <p className={DIALOG_EYEBROW_CLASS}>Mutation flow</p>
                <h2 className={DIALOG_TITLE_CLASS} id={titleId}>
                  {title}
                </h2>
                {description ? (
                  <p className={DIALOG_DESCRIPTION_CLASS} id={descriptionId}>
                    {description}
                  </p>
                ) : null}
              </div>

              {showCloseButton && onClose ? (
                <button
                  aria-label={closeLabel}
                  className={DIALOG_CLOSE_BUTTON_CLASS}
                  disabled={closeDisabled}
                  onClick={onClose}
                  type="button"
                >
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
                    <path d="M6 6l12 12" />
                    <path d="M18 6L6 18" />
                  </svg>
                </button>
              ) : null}
            </header>

            {children}
            {footer ? <div className={DIALOG_FOOTER_CLASS}>{footer}</div> : null}
          </div>
        </div>
      </section>
    </div>
  );
}

export function DashboardMessagePanel({
  action,
  ariaLive,
  children,
  className,
  compact = false,
  role,
  title,
  tone = "neutral",
}: DashboardMessagePanelProps) {
  return (
    <div
      aria-live={ariaLive}
      className={cx(
        EMPTY_STATE_CLASS,
        compact && EMPTY_STATE_COMPACT_CLASS,
        MESSAGE_PANEL_TONE_CLASS[tone],
        className,
      )}
      role={role}
    >
      <div className="flex items-start justify-between gap-4">
        <div className="grid gap-1">
          <h3>{title}</h3>
          <div className={cx("m-0 text-sm", DASHBOARD_SUPPORTING_TEXT_CLASS)}>{children}</div>
        </div>
        {action}
      </div>
    </div>
  );
}

