import type { HTMLAttributes, ReactNode } from "react";

import { cn } from "../../lib/cn";

export const DASHBOARD_PANEL_SHELL_CLASS =
  "rounded-lg border border-af-border bg-af-surface-raised text-af-text shadow-af-card";

type DashboardPanelShellElement = "article" | "section";

interface DashboardPanelShellProps extends HTMLAttributes<HTMLElement> {
  as?: DashboardPanelShellElement;
  children: ReactNode;
  shellKind?: string;
}

export function DashboardPanelShell({
  as: Component = "section",
  children,
  className = "",
  shellKind = "panel",
  ...props
}: DashboardPanelShellProps) {
  return (
    <Component
      className={cn(DASHBOARD_PANEL_SHELL_CLASS, className)}
      data-dashboard-panel-shell={shellKind}
      {...props}
    >
      {children}
    </Component>
  );
}
