import { Button, type ButtonProps } from "../../components/ui/button";
import { cn } from "../../lib/cn";

const DASHBOARD_HEADER_ACTION_BUTTON_CLASS = "shrink-0";

export function DashboardHeaderActionButton({
  className,
  size = "icon",
  tone = "outline",
  ...props
}: ButtonProps) {
  return (
    <Button
      className={cn(DASHBOARD_HEADER_ACTION_BUTTON_CLASS, className)}
      data-dashboard-header-action="neutral"
      size={size}
      tone={tone}
      {...props}
    />
  );
}
