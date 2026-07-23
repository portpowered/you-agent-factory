import type { HTMLAttributes, ReactNode } from "react";

import { cn } from "../utilities/cn";

const ACTION_ROW_CLASS =
  "flex min-w-0 flex-wrap items-center gap-2 max-md:justify-start";
const ACTION_ROW_SECTION_CLASS = "flex min-w-0 flex-wrap items-center gap-2";

export interface ActionRowProps
  extends Omit<HTMLAttributes<HTMLDivElement>, "children"> {
  actions?: ReactNode;
  actionsClassName?: string;
  statuses?: ReactNode;
  statusesClassName?: string;
}

export function ActionRow({
  actions,
  actionsClassName,
  className,
  statuses,
  statusesClassName,
  ...props
}: ActionRowProps) {
  const hasStatuses = statuses !== undefined && statuses !== null;
  const hasActions = actions !== undefined && actions !== null;

  if (!hasStatuses && !hasActions) {
    return null;
  }

  return (
    <div className={cn(ACTION_ROW_CLASS, className)} {...props}>
      {hasStatuses ? (
        <div
          className={cn(ACTION_ROW_SECTION_CLASS, statusesClassName)}
          data-action-row-section="statuses"
        >
          {statuses}
        </div>
      ) : null}
      {hasActions ? (
        <div
          className={cn(ACTION_ROW_SECTION_CLASS, actionsClassName)}
          data-action-row-section="actions"
        >
          {actions}
        </div>
      ) : null}
    </div>
  );
}
