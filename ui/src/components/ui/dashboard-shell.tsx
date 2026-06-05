import type { HTMLAttributes, ReactNode } from "react";

import { cn } from "../../lib/cn";

const DASHBOARD_PANEL_SHELL_CLASS =
  "rounded-lg border border-outline bg-surface-container-high text-on-surface shadow-af-card";
const DASHBOARD_PANEL_SHELL_INSET_CLASS =
  "p-layout-inset-card md:p-layout-inset-card-relaxed";

type DashboardPanelShellElement = "article" | "section";

interface DashboardPanelShellProps extends HTMLAttributes<HTMLElement> {
  as?: DashboardPanelShellElement;
  children: ReactNode;
  inset?: boolean;
  shellKind?: string;
}

export function DashboardPanelShell({
  as: Component = "section",
  children,
  className = "",
  inset = false,
  shellKind = "panel",
  ...props
}: DashboardPanelShellProps) {
  return (
    <Component
      className={cn(
        DASHBOARD_PANEL_SHELL_CLASS,
        inset && DASHBOARD_PANEL_SHELL_INSET_CLASS,
        className,
      )}
      data-dashboard-panel-shell={shellKind}
      {...props}
    >
      {children}
    </Component>
  );
}
