import {
  DashboardStatusPill,
  type DashboardStatusPillTone,
} from "../../../components/ui/dashboard-status-pill";
import type { DashboardLayoutDiagnostic } from "../hooks/dashboardLayoutSchema";
import { getDashboardLayoutDiagnosticsMessages } from "../messages/dashboard-layout-diagnostics";

export interface DashboardLayoutDiagnosticsProps {
  diagnostics?: readonly DashboardLayoutDiagnostic[];
  locale?: string | null;
}

export function DashboardLayoutDiagnostics({
  diagnostics = [],
  locale,
}: DashboardLayoutDiagnosticsProps) {
  const messages = getDashboardLayoutDiagnosticsMessages(locale);
  const hasDiagnostics = diagnostics.length > 0;
  const hasStorageError = diagnostics.some(
    (diagnostic) => diagnostic.severity === "error",
  );
  const statusTone: DashboardStatusPillTone = hasStorageError
    ? "danger"
    : hasDiagnostics
      ? "warning"
      : "success";

  return (
    <section
      aria-atomic="true"
      aria-label={messages.regionLabel}
      aria-live={hasStorageError ? "assertive" : "polite"}
      className="mb-3 flex min-w-0 flex-col gap-1.5 rounded-lg border border-outline bg-surface-container-low px-3 py-2"
      data-dashboard-layout-diagnostics={hasDiagnostics ? "issue" : "empty"}
      data-testid="dashboard-layout-diagnostics"
      role={hasStorageError ? "alert" : "status"}
    >
      <div className="flex min-w-0 items-center gap-2">
        <DashboardStatusPill size="compact" tone={statusTone}>
          {hasDiagnostics
            ? hasStorageError
              ? messages.storageLabel
              : messages.repairSummary
            : messages.successLabel}
        </DashboardStatusPill>
        <span className="min-w-0 text-sm text-on-surface-variant">
          {!hasDiagnostics
            ? messages.emptyLabel
            : hasStorageError
              ? messages.storageSummary
              : messages.repairDetails}
        </span>
      </div>
      {hasDiagnostics ? (
        <ul className="m-0 list-disc space-y-1 pl-5 text-sm text-on-surface-variant">
          {diagnostics.map((diagnostic) => (
            <li key={diagnostic.code}>
              {messages.diagnosticLabel(diagnostic.code, diagnostic.count)}
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  );
}
