import type { ReactNode } from "react";

import { DashboardIconButtonShell } from "../../../components/ui";
import { cn } from "../../../lib/cn";

export type FactoryGraphEditorNoticeTone = "danger" | "neutral" | "warning";

const NOTICE_TONE_CLASS: Record<FactoryGraphEditorNoticeTone, string> = {
  danger: "border-af-danger-border bg-af-danger-surface text-af-danger-text",
  neutral: "border-af-border bg-af-surface-subtle text-af-text-muted",
  warning:
    "border-af-warning-border bg-af-warning-surface text-af-warning-text",
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
