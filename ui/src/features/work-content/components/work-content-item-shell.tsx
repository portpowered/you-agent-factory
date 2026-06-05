import type { ReactNode } from "react";
import { DashboardLabel, SurfacePanel } from "../../../components/ui";

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
    <SurfacePanel asChild className="grid gap-3" radius="lg">
      <li>
        <div className="flex items-start justify-between gap-3">
          <div className="grid gap-1">
            <DashboardLabel>{itemTypeLabel}</DashboardLabel>
          </div>
          {headerActions}
        </div>
        {children}
      </li>
    </SurfacePanel>
  );
}
