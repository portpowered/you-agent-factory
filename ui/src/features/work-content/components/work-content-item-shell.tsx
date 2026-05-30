import type { ReactNode } from "react";
import { DASHBOARD_SUPPORTING_LABEL_CLASS } from "../../../components/ui/dashboard-typography";

export function WorkContentItemShell({
  children,
  headerActions,
  itemTypeLabel,
}: {
  children: ReactNode;
  headerActions?: ReactNode;
  itemTypeLabel: string;
}) {
  return (
    <li className="grid gap-3 rounded-lg border-af-border border bg-af-panel p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="grid gap-1">
          <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{itemTypeLabel}</span>
        </div>
        {headerActions}
      </div>
      {children}
    </li>
  );
}
