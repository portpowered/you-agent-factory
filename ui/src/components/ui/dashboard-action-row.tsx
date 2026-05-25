import type { ReactNode } from "react";

import { cn } from "../../lib/cn";

const DASHBOARD_ACTION_ROW_CLASS =
  "flex flex-wrap items-center gap-2 max-md:justify-start";
const DASHBOARD_ACTION_ROW_SECTION_CLASS =
  "flex min-w-0 flex-wrap items-center gap-2";

export interface DashboardActionRowProps {
  actions?: ReactNode;
  actionsClassName?: string;
  className?: string;
  statuses?: ReactNode;
  statusesClassName?: string;
}

export function DashboardActionRow({
  actions,
  actionsClassName,
  className,
  statuses,
  statusesClassName,
}: DashboardActionRowProps) {
  const hasStatuses = statuses !== undefined && statuses !== null;
  const hasActions = actions !== undefined && actions !== null;

  if (!hasStatuses && !hasActions) {
    return null;
  }

  return (
    <div className={cn(DASHBOARD_ACTION_ROW_CLASS, className)}>
      {hasStatuses ? (
        <div
          className={cn(
            DASHBOARD_ACTION_ROW_SECTION_CLASS,
            statusesClassName,
          )}
          data-dashboard-action-row-section="statuses"
        >
          {statuses}
        </div>
      ) : null}
      {hasActions ? (
        <div
          className={cn(DASHBOARD_ACTION_ROW_SECTION_CLASS, actionsClassName)}
          data-dashboard-action-row-section="actions"
        >
          {actions}
        </div>
      ) : null}
    </div>
  );
}
