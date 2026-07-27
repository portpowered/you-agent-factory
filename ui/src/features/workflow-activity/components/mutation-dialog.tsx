import { type ReactNode, useId } from "react";
import { SurfacePanel } from "@you-agent-factory/components/layout";
import { Heading, Label, Text } from "@you-agent-factory/components/primitives";
import { AlertPanel, AlertPanelText } from "../../../components/ui/alert-panel";
import { Button } from "../../../components/ui/button";
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
      <SurfacePanel
        aria-describedby={description ? descriptionId : undefined}
        aria-labelledby={titleId}
        aria-modal="true"
        asChild
        className="pointer-events-auto relative z-10 w-full overflow-hidden shadow-af-panel"
        padding="none"
        radius="3xl"
        role="dialog"
      >
        <section>
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
                  <Label
                    as="p"
                    className="mb-0 text-xs font-bold uppercase tracking-[0.16em] text-primary"
                  >
                    {resolvedFlowLabel}
                  </Label>
                  <Heading as="h2" className="m-0" id={titleId}>
                    {title}
                  </Heading>
                  {description ? (
                    <Text className="m-0" id={descriptionId}>
                      {description}
                    </Text>
                  ) : null}
                </div>

                {showCloseButton && onClose ? (
                  <Button
                    aria-label={resolvedCloseLabel}
                    className="text-on-surface-variant"
                    disabled={closeDisabled}
                    onClick={onClose}
                    size="iconPill"
                    tone="secondary"
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
      </SurfacePanel>
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
          <Heading as="h3">{title}</Heading>
          <AlertPanelText as="div" variant="supporting">
            {children}
          </AlertPanelText>
        </div>
        {action}
      </div>
    </AlertPanel>
  );
}
