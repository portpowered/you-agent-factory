import { type ReactNode, useId } from "react";
import {
  AlertPanel,
  Button,
  DashboardHeading,
  DashboardLabel,
  DashboardText,
} from "../../../components/ui";
import { cn } from "../../../lib/cn";
import { getWorkflowActivityGraphImportMessages } from "../messages/graph-import";

const DIALOG_OVERLAY_CLASS =
  "z-50 flex items-center justify-center bg-af-overlay-strong p-4 backdrop-blur-sm";
const DIALOG_CONTENT_CLASS = "grid gap-layout-block p-4 md:p-5";
const DIALOG_CONTENT_WITH_MEDIA_CLASS =
  "lg:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]";

export interface DashboardMutationDialogProps {
  children: ReactNode;
  closeDisabled?: boolean;
  closeLabel?: string;
  description?: ReactNode;
  footer?: ReactNode;
  flowLabel?: string;
  locale?: string;
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
  closeLabel,
  description,
  footer,
  flowLabel,
  locale,
  media,
  onClose,
  // hardcoded-ui-copy-exception: non-product-diagnostic
  overlayClassName = "fixed inset-0 px-5 py-6",
  showCloseButton = true,
  title,
}: DashboardMutationDialogProps) {
  const messages = getWorkflowActivityGraphImportMessages(locale);
  const resolvedCloseLabel = closeLabel ?? messages.dialogCloseLabel;
  const resolvedFlowLabel = flowLabel ?? messages.dialogFlowLabel;
  const canClose = onClose !== undefined && !closeDisabled;
  const titleId = useId();
  const descriptionId = useId();

  return (
    <div
      className={cn(
        DIALOG_OVERLAY_CLASS,
        "pointer-events-none relative",
        overlayClassName,
      )}
    >
      {canClose ? (
        <button
          aria-label={resolvedCloseLabel}
          className="pointer-events-auto absolute inset-0"
          onClick={onClose}
          type="button"
        />
      ) : null}
      <section
        aria-describedby={description ? descriptionId : undefined}
        aria-labelledby={titleId}
        aria-modal="true"
        className="pointer-events-auto relative z-10 w-full overflow-hidden rounded-3xl border border-outline bg-surface-container-high shadow-af-panel"
        role="dialog"
      >
        <div
          className={cn(
            DIALOG_CONTENT_CLASS,
            media ? DIALOG_CONTENT_WITH_MEDIA_CLASS : undefined,
          )}
        >
          {media ? <div>{media}</div> : null}

          <div className="grid content-start gap-4">
            <header className="flex items-start justify-between gap-4">
              <div className="grid gap-2">
                <DashboardLabel
                  as="p"
                  className="mb-0 text-xs font-bold uppercase tracking-[0.16em] text-primary"
                >
                  {resolvedFlowLabel}
                </DashboardLabel>
                <DashboardHeading
                  as="h2"
                  className="m-0"
                  id={titleId}
                >
                  {title}
                </DashboardHeading>
                {description ? (
                  <DashboardText className="m-0" id={descriptionId}>
                    {description}
                  </DashboardText>
                ) : null}
              </div>

              {showCloseButton && onClose ? (
                <Button
                  aria-label={resolvedCloseLabel}
                  className="h-10 min-h-10 w-10 rounded-full bg-surface-container-low text-on-surface-variant"
                  disabled={closeDisabled}
                  onClick={onClose}
                  size="icon"
                  tone="outline"
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
                </Button>
              ) : null}
            </header>

            {children}
            {footer ? (
              <div className="flex flex-wrap justify-end gap-3">{footer}</div>
            ) : null}
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
    <AlertPanel
      aria-live={ariaLive}
      className={className}
      compact={compact}
      role={role}
      tone={tone === "error" ? "danger" : "neutral"}
      variant="empty"
    >
      <div className="flex items-start justify-between gap-4">
        <div className="grid gap-1">
          <DashboardHeading as="h3">{title}</DashboardHeading>
          <DashboardText as="div" className="m-0 text-sm" variant="supporting">
            {children}
          </DashboardText>
        </div>
        {action}
      </div>
    </AlertPanel>
  );
}
