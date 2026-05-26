import { forwardRef } from "react";

import { cn } from "../../lib/cn";
import { Button, type ButtonProps } from "./button";

const DASHBOARD_ICON_BUTTON_SHELL_BASE_CLASS = "relative shrink-0";
const DASHBOARD_ICON_BUTTON_SHELL_SIZE_CLASS = "h-10 w-10 rounded-lg";

export interface DashboardIconButtonShellProps extends Omit<ButtonProps, "size"> {}

export const DashboardIconButtonShell = forwardRef<
  HTMLButtonElement,
  DashboardIconButtonShellProps
>(function DashboardIconButtonShell(
  { className, tone = "outline", ...props },
  ref,
) {
  return (
    <Button
      className={cn(
        DASHBOARD_ICON_BUTTON_SHELL_BASE_CLASS,
        DASHBOARD_ICON_BUTTON_SHELL_SIZE_CLASS,
        className,
      )}
      ref={ref}
      size="icon"
      tone={tone}
      {...props}
    />
  );
});
