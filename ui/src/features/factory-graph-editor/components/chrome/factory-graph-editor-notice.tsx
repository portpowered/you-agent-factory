import type { ReactNode } from "react";

import {
  AlertPanel,
  AlertPanelText,
} from "../../../../components/ui/alert-panel";
import { DashboardIconButtonShell } from "../../../../components/ui/dashboard-icon-button-shell";

export type FactoryGraphEditorNoticeTone = "danger" | "neutral" | "warning";

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
    <AlertPanel
      padding="default"
      radius="2xl"
      role={tone === "danger" ? "alert" : "status"}
      tone={tone}
    >
      <section>
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
        <AlertPanelText as="div">{children}</AlertPanelText>
      </section>
    </AlertPanel>
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
