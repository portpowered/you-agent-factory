import { Button, type ButtonProps } from "../../components/ui/button";
import { cn } from "../../lib/cn";

const DASHBOARD_HEADER_ACTION_BUTTON_CLASS = "shrink-0";
const DASHBOARD_HEADER_ACTION_BUTTON_COMPACT_CLASS =
  "h-10 w-10 rounded-lg px-0 py-0";

export interface DashboardHeaderActionButtonProps extends ButtonProps {
  compact?: boolean;
}

export function DashboardHeaderActionButton({
  className,
  compact = false,
  size = "icon",
  tone = "outline",
  ...props
}: DashboardHeaderActionButtonProps) {
  return (
    <Button
      className={cn(
        DASHBOARD_HEADER_ACTION_BUTTON_CLASS,
        compact && DASHBOARD_HEADER_ACTION_BUTTON_COMPACT_CLASS,
        className,
      )}
      data-dashboard-header-action="neutral"
      size={size}
      tone={tone}
      {...props}
    />
  );
}
