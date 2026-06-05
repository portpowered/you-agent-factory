import { forwardRef } from "react";

import { cn } from "../../lib/cn";
import { Button, type ButtonProps } from "./button";

const DASHBOARD_ICON_BUTTON_SHELL_BASE_CLASS = "relative shrink-0";
const DASHBOARD_ICON_BUTTON_SHELL_SIZE_CLASS = "h-10 w-10 rounded-lg";
const DASHBOARD_ICON_BUTTON_SHELL_TONE_CLASS = {
  dangerGhost:
    "text-on-surface-subtle hover:border-af-danger-border hover:bg-error-container hover:text-on-error-container",
} as const;

type DashboardIconButtonShellTone =
  | ButtonProps["tone"]
  | keyof typeof DASHBOARD_ICON_BUTTON_SHELL_TONE_CLASS;

export interface DashboardIconButtonShellProps
  extends Omit<ButtonProps, "size" | "tone"> {
  tone?: DashboardIconButtonShellTone;
}

export const DashboardIconButtonShell = forwardRef<
  HTMLButtonElement,
  DashboardIconButtonShellProps
>(function DashboardIconButtonShell(
  { className, tone = "outline", ...props },
  ref,
) {
  const buttonTone =
    tone === "dangerGhost" ? "ghost" : tone;
  const toneClassName =
    tone === "dangerGhost"
      ? DASHBOARD_ICON_BUTTON_SHELL_TONE_CLASS.dangerGhost
      : undefined;

  return (
    <Button
      className={cn(
        DASHBOARD_ICON_BUTTON_SHELL_BASE_CLASS,
        DASHBOARD_ICON_BUTTON_SHELL_SIZE_CLASS,
        toneClassName,
        className,
      )}
      ref={ref}
      size="icon"
      tone={buttonTone}
      {...props}
    />
  );
});
