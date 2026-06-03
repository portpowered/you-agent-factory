import type { ReactNode } from "react";

import { DashboardIconButtonShell } from "../../../components/ui";
import { cn } from "../../../lib/cn";

export type FactoryGraphEditorNoticeTone = "danger" | "neutral" | "warning";

const NOTICE_TONE_CLASS: Record<FactoryGraphEditorNoticeTone, string> = {
  danger: "border-af-danger-border bg-error-container text-on-error-container",
  neutral: "border-outline bg-surface-container-low text-on-surface-variant",
  warning:
    "border-af-warning-border bg-warning-container text-on-warning-container",
};

export function FactoryGraphEditorNotice({
  children,
  dismissLabel,
  onDismiss,
  title,
  tone = "neutral",
}: {
  children: ReactNode;
  dismissLabel?: string;
  onDismiss?: () => void;
  title: string;
  tone?: FactoryGraphEditorNoticeTone;
}) {
  return (
    <section
      className={cn(
        "grid gap-1 rounded-2xl border p-4",
        NOTICE_TONE_CLASS[tone],
      )}
      role={tone === "danger" ? "alert" : "status"}
    >
      <div className="flex items-start justify-between gap-3">
        <h3 className="m-0 text-sm font-semibold">{title}</h3>
        {onDismiss && dismissLabel ? (
          <DashboardIconButtonShell
            aria-label={dismissLabel}
            className="h-9 w-9 shrink-0"
            onClick={onDismiss}
            tone="ghost"
            type="button"
          >
            <FactoryGraphEditorNoticeDismissIcon />
          </DashboardIconButtonShell>
        ) : null}
      </div>
      <div className="m-0 text-sm leading-6">{children}</div>
    </section>
  );
}

function FactoryGraphEditorNoticeDismissIcon() {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="18"
      viewBox="0 0 24 24"
      width="18"
    >
      <path
        d="M6 6l12 12M18 6L6 18"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
      />
    </svg>
  );
}
